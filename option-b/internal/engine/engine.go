package engine

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"sync/atomic"
	"time"

	"rotme/internal/analysis"
	"rotme/internal/config"
	"rotme/internal/game"
	"rotme/internal/kafka"
)

// Fixed game id so a new leader keys broadcast/session records the same way
// the previous one did, which is what lets the game survive failover.
const gameID = "rotme"

// Control "orders" carried on game.orders.raw so any instance can trigger them
// and the elected leader acts on them.
const (
	ctrlGameStart   = "GAME_START"
	ctrlTurnAdvance = "TURN_ADVANCE"
)

type Engine struct {
	cfg     *config.Config
	brokers []string
	srURL   string

	serde     *kafka.AvroSerde
	schemaIDs map[string]int
	prod      *kafka.Producer
	txn       *kafka.Producer

	instanceID string

	// Leader-only state, owned by the leader's core goroutine.
	world  *game.World
	buffer map[string]game.Order

	// Read by the validation worker (replaced wholesale, never mutated).
	valState atomic.Pointer[game.ValidationState]

	startCh     chan string
	validatedCh chan validatedOrder
	advanceCh   chan chan struct{}

	hub         *Hub
	turnSeconds int
}

type validatedOrder struct {
	O    game.Order
	Risk *int
}

func New(cfg *config.Config, brokers []string, srURL string, turnSeconds int) (*Engine, error) {
	serde := kafka.NewAvroSerde()
	reg := kafka.NewRegistry(srURL)
	ids, err := reg.LoadSchemas(serde, schemaSubjects)
	if err != nil {
		return nil, err
	}
	prod, err := kafka.NewProducer(brokers, serde)
	if err != nil {
		return nil, err
	}
	txn, err := kafka.NewTxnProducer(brokers, "rotme-gameover-"+hostname(), serde)
	if err != nil {
		return nil, err
	}
	e := &Engine{
		cfg: cfg, brokers: brokers, srURL: srURL,
		serde: serde, schemaIDs: ids, prod: prod, txn: txn,
		instanceID:  hostname(),
		buffer:      map[string]game.Order{},
		startCh:     make(chan string, 1),
		validatedCh: make(chan validatedOrder, 100),
		advanceCh:   make(chan chan struct{}, 4),
		turnSeconds: turnSeconds,
	}
	e.hub = newHub(cfg, brokers, serde, e.instanceID)
	return e, nil
}

func hostname() string {
	h, _ := os.Hostname()
	if h == "" {
		return "engine"
	}
	return h
}

// Run starts the read side (hub) on every instance and the leadership
// coordinator. The coordinator promotes exactly one instance to run the
// authoritative pipeline. Blocks until ctx is cancelled.
func (e *Engine) Run(ctx context.Context) error {
	go e.hub.run(ctx)
	e.runCoordinator(ctx)
	e.prod.Close()
	e.txn.Close()
	return ctx.Err()
}

// runLeader is the authoritative pipeline. It is started by the coordinator on
// promotion and torn down (via ctx) when leadership is lost.
func (e *Engine) runLeader(ctx context.Context) {
	e.promote()

	rawCons, err := kafka.NewConsumer(e.brokers, "rotme-validation", []string{topicOrdersRaw}, e.serde)
	if err != nil {
		log.Printf("leader: raw consumer: %v", err)
		return
	}
	valCons, err := kafka.NewConsumer(e.brokers, "rotme-engine", []string{topicOrdersValidated}, e.serde)
	if err != nil {
		log.Printf("leader: validated consumer: %v", err)
		rawCons.Close()
		return
	}
	go e.runValidation(ctx, rawCons)
	go e.runValidatedConsumer(ctx, valCons)

	ticker := time.NewTicker(time.Duration(e.turnSeconds) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			rawCons.Close()
			valCons.Close()
			return
		case id := <-e.startCh:
			e.startGame(id)
		case vo := <-e.validatedCh:
			e.handleValidated(ctx, vo)
		case ack := <-e.advanceCh:
			e.advanceTurn(ctx)
			close(ack)
		case <-ticker.C:
			if e.world != nil && !e.world.GameOver {
				e.advanceTurn(ctx)
			}
		}
	}
}

// promote recovers any in-progress game from the local Kafka-rebuilt cache so
// turn processing continues seamlessly after a failover.
func (e *Engine) promote() {
	if w := e.hub.RecoverWorld(); w != nil {
		e.world = w
		e.buffer = map[string]game.Order{}
		e.refreshValState()
		log.Printf("[%s] promoted; recovered game at turn %d", e.instanceID, e.world.Turn)
	} else {
		e.world = nil
		log.Printf("[%s] promoted; no game in progress", e.instanceID)
	}
}

func (e *Engine) startGame(id string) {
	e.world = game.NewWorld(e.cfg)
	e.buffer = map[string]game.Order{}
	e.refreshValState()
	e.publishSnapshot(context.Background())
	e.publishSession(context.Background())
	log.Printf("[%s] game started at turn %d", e.instanceID, e.world.Turn)
}

func (e *Engine) handleValidated(ctx context.Context, vo validatedOrder) {
	if e.world == nil {
		return
	}
	e.buffer[vo.O.UnitID] = vo.O
	if vo.Risk == nil && (vo.O.OrderType == game.OrderAssignRoute || vo.O.OrderType == game.OrderRedirect) {
		risk := e.computeRisk(vo.O)
		e.publishValidated(ctx, vo.O, &risk)
	}
}

func (e *Engine) advanceTurn(ctx context.Context) {
	if e.world == nil || e.world.GameOver {
		return
	}
	orders := make([]game.Order, 0, len(e.buffer))
	for _, o := range e.buffer {
		orders = append(orders, o)
	}
	events := e.world.ProcessTurn(orders)
	e.publishEvents(ctx, events)
	e.publishSnapshot(ctx)

	e.buffer = map[string]game.Order{}
	if !e.world.GameOver {
		e.world.Turn++
	}
	e.refreshValState()
	e.publishSession(ctx)
}

func (e *Engine) computeRisk(o game.Order) int {
	paths := o.PathIds
	if o.OrderType == game.OrderRedirect {
		paths = o.NewPathIds
	}
	start := ""
	if e.cfg.UnitByID[o.UnitID].Class == "RingBearer" {
		start = e.world.Ring.TrueRegion
	} else if u := e.world.Units[o.UnitID]; u != nil {
		start = u.Region
	}
	return analysis.ScoreRoute(e.world, o.OrderType, start, paths).Score
}

func (e *Engine) refreshValState() {
	st := &game.ValidationState{
		Turn:    e.world.Turn,
		Units:   make(map[string]game.UnitView, len(e.world.Units)),
		Paths:   make(map[string]game.PathView, len(e.world.Paths)),
		Regions: make(map[string]game.RegionView, len(e.world.Regions)),
	}
	for id, u := range e.world.Units {
		region := u.Region
		if e.cfg.UnitByID[id].Class == "RingBearer" {
			region = e.world.Ring.TrueRegion
		}
		st.Units[id] = game.UnitView{Side: e.cfg.UnitByID[id].Side, Region: region, Status: string(u.Status), Cooldown: u.Cooldown}
	}
	for id, p := range e.world.Paths {
		st.Paths[id] = game.PathView{Status: string(p.Status)}
	}
	for id, r := range e.world.Regions {
		st.Regions[id] = game.RegionView{ControlledBy: r.ControlledBy}
	}
	e.valState.Store(st)
}

// SubmitRaw publishes a player's order to game.orders.raw. Any instance can do
// this; only the leader consumes and processes it.
func (e *Engine) SubmitRaw(ctx context.Context, o game.Order) error {
	payload, _ := json.Marshal(o)
	msg := orderSubmittedMsg{
		PlayerID: o.PlayerID, UnitID: o.UnitID, OrderType: o.OrderType,
		Payload: payload, Turn: o.Turn, Timestamp: time.Now().UnixMilli(),
	}
	key := o.PlayerID
	if key == "" {
		key = "system"
	}
	return e.prod.Produce(ctx, topicOrdersRaw, key, e.schemaIDs["OrderSubmitted"], msg)
}

// SubmitControl sends a game-control action (start/advance) through Kafka.
func (e *Engine) SubmitControl(ctx context.Context, kind string) error {
	return e.SubmitRaw(ctx, game.Order{OrderType: kind, PlayerID: "system", UnitID: "system"})
}

func (e *Engine) Hub() *Hub              { return e.hub }
func (e *Engine) Config() *config.Config { return e.cfg }
