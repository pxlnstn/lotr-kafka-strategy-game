package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"rotme/internal/analysis"
	"rotme/internal/config"
	"rotme/internal/game"
	"rotme/internal/kafka"
)

// SSEClient is one connected browser stream.
type SSEClient struct {
	Side string // "light" or "dark"
	Ch   chan []byte
}

type hubEvent struct {
	audience string
	payload  []byte
}

type analysisReq struct {
	kind  string
	reply chan any
}

type stateReq struct {
	side  string
	reply chan APIState
}

// Hub is the read side: it consumes the broadcast/event/ring topics, fans
// events out to SSE clients with information hiding, keeps a local world cache
// rebuilt from Kafka, and answers state/analysis requests. One select loop
// owns all of this state. Every instance runs a hub, so any
// instance can serve reads and any promoted leader can recover the world.
type Hub struct {
	cfg        *config.Config
	brokers    []string
	serde      *kafka.AvroSerde
	instanceID string

	register   chan *SSEClient
	unregister chan *SSEClient
	eventsCh   chan hubEvent
	snapshotCh chan worldSnapshotMsg
	analysisCh chan analysisReq
	stateCh    chan stateReq
	recoverCh  chan chan *game.World
	sessionCh  chan recoverState

	light  map[*SSEClient]bool
	dark   map[*SSEClient]bool
	closed chan struct{}

	cache              *game.World
	lastRecover        *recoverState
	started            bool
	gameOver           bool
	winner             string
	lightRingRegion    string
	lastDetectedRegion string

	canonical map[string][]string
}

func newHub(cfg *config.Config, brokers []string, serde *kafka.AvroSerde, instanceID string) *Hub {
	return &Hub{
		cfg: cfg, brokers: brokers, serde: serde, instanceID: instanceID,
		register:   make(chan *SSEClient),
		unregister: make(chan *SSEClient),
		eventsCh:   make(chan hubEvent, 100),
		snapshotCh: make(chan worldSnapshotMsg, 16),
		analysisCh: make(chan analysisReq, 16),
		stateCh:    make(chan stateReq, 16),
		recoverCh:  make(chan chan *game.World, 4),
		sessionCh:  make(chan recoverState, 16),
		closed:     make(chan struct{}),
		light:      map[*SSEClient]bool{},
		dark:       map[*SSEClient]bool{},
		cache:      game.NewWorld(cfg),
		canonical:  canonicalRoutePaths(cfg),
	}
}

func (h *Hub) run(ctx context.Context) {
	go h.consumeSnapshots(ctx)
	go h.consumeSession(ctx)
	go h.consumeEvents(ctx, topicEventsUnit, game.AudAll)
	go h.consumeEvents(ctx, topicEventsRegion, game.AudAll)
	go h.consumeEvents(ctx, topicEventsPath, game.AudAll)
	go h.consumeEvents(ctx, topicRingPosition, game.AudLight)
	go h.consumeEvents(ctx, topicRingDetection, game.AudDark)

	keepalive := time.NewTicker(20 * time.Second)
	defer keepalive.Stop()

	for {
		select {
		case <-ctx.Done():
			for c := range h.light {
				close(c.Ch)
			}
			for c := range h.dark {
				close(c.Ch)
			}
			close(h.closed)
			return

		case c := <-h.register:
			if c.Side == game.PlayerDark {
				h.dark[c] = true
			} else {
				h.light[c] = true
			}

		case c := <-h.unregister:
			h.drop(c)

		case ev := <-h.eventsCh:
			h.applyEventToCache(ev.payload)
			h.fanout(ev.audience, ev.payload)

		case snap := <-h.snapshotCh:
			h.applySnapshot(snap)
			payload, _ := json.Marshal(map[string]any{"topic": topicBroadcast, "data": snap})
			h.fanout(game.AudAll, payload)

		case req := <-h.analysisCh:
			req.reply <- h.analyze(req.kind)

		case sr := <-h.stateCh:
			sr.reply <- h.buildStateView(sr.side)

		case rr := <-h.recoverCh:
			rr <- h.cloneWorld()

		case rs := <-h.sessionCh:
			h.applySession(rs)

		case <-keepalive.C:
			h.fanout(game.AudAll, []byte(`{"topic":"keepalive"}`))
		}
	}
}

func (h *Hub) drop(c *SSEClient) {
	if h.light[c] {
		delete(h.light, c)
		close(c.Ch)
	} else if h.dark[c] {
		delete(h.dark, c)
		close(c.Ch)
	}
}

func (h *Hub) fanout(audience string, payload []byte) {
	send := func(set map[*SSEClient]bool) {
		for c := range set {
			select {
			case c.Ch <- payload:
			default:
			}
		}
	}
	switch audience {
	case game.AudLight:
		send(h.light)
	case game.AudDark:
		send(h.dark)
	default:
		send(h.light)
		send(h.dark)
	}
}

func (h *Hub) consumeEvents(ctx context.Context, topic, audience string) {
	group := fmt.Sprintf("sse-%s-%s", h.instanceID, topic)
	cons, err := kafka.NewConsumer(h.brokers, group, []string{topic}, h.serde)
	if err != nil {
		return
	}
	defer cons.Close()
	for {
		recs, err := cons.Poll(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			continue
		}
		for _, r := range recs {
			data, _, err := h.serde.DecodeGeneric(r.Value)
			if err != nil {
				continue
			}
			payload, _ := json.Marshal(map[string]any{"topic": topic, "data": data})
			select {
			case h.eventsCh <- hubEvent{audience: audience, payload: payload}:
			case <-ctx.Done():
				return
			}
		}
	}
}

// consumeSession reads the full-state recovery records from game.session.
// These are never forwarded to a browser, so they may carry the Ring Bearer's
// route. They give a promoted leader everything it needs to continue.
func (h *Hub) consumeSession(ctx context.Context) {
	group := fmt.Sprintf("sse-%s-session", h.instanceID)
	cons, err := kafka.NewConsumer(h.brokers, group, []string{topicSession}, h.serde)
	if err != nil {
		return
	}
	defer cons.Close()
	for {
		recs, err := cons.Poll(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			continue
		}
		for _, r := range recs {
			var m gameSessionMsg
			if _, err := cons.Decode(r.Value, &m); err != nil || m.WorldJSON == nil {
				continue
			}
			var rs recoverState
			if json.Unmarshal([]byte(*m.WorldJSON), &rs) != nil {
				continue
			}
			select {
			case h.sessionCh <- rs:
			case <-ctx.Done():
				return
			}
		}
	}
}

func (h *Hub) applySession(rs recoverState) {
	h.started = true
	h.lastRecover = &rs
	h.gameOver = rs.GameOver
	h.winner = rs.Winner
	h.lightRingRegion = rs.RingRegion
	h.cache = buildWorldFromRecover(h.cfg, rs)
}

func (h *Hub) consumeSnapshots(ctx context.Context) {
	group := fmt.Sprintf("sse-%s-broadcast", h.instanceID)
	cons, err := kafka.NewConsumer(h.brokers, group, []string{topicBroadcast}, h.serde)
	if err != nil {
		return
	}
	defer cons.Close()
	for {
		recs, err := cons.Poll(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			continue
		}
		for _, r := range recs {
			m, _, err := h.serde.DecodeGeneric(r.Value)
			if err != nil {
				continue
			}
			if _, isOver := m["winner"]; isOver {
				payload, _ := json.Marshal(map[string]any{"topic": "game.over", "data": m})
				select {
				case h.eventsCh <- hubEvent{audience: game.AudAll, payload: payload}:
				case <-ctx.Done():
					return
				}
				continue
			}
			var snap worldSnapshotMsg
			if _, err := h.serde.Decode(r.Value, &snap); err != nil || len(snap.Units) == 0 {
				continue
			}
			select {
			case h.snapshotCh <- snap:
			case <-ctx.Done():
				return
			}
		}
	}
}

// --- cache maintenance (loop-owned) ---

func (h *Hub) applySnapshot(snap worldSnapshotMsg) {
	h.started = true
	if snap.Turn == 1 {
		h.gameOver = false
		h.winner = ""
	}
	h.cache.Turn = snap.Turn
	for _, rs := range snap.Regions {
		if r := h.cache.Regions[rs.RegionID]; r != nil {
			r.ControlledBy = rs.ControlledBy
			r.ThreatLevel = rs.ThreatLevel
			r.Fortified = rs.Fortified
		}
	}
	for _, us := range snap.Units {
		if u := h.cache.Units[us.UnitID]; u != nil {
			u.Region = us.Region
			u.Strength = us.Strength
			u.Status = game.Status(us.Status)
		}
	}
	h.cache.Ring.TrueRegion = h.lightRingRegion
}

func (h *Hub) applyEventToCache(payload []byte) {
	var env struct {
		Topic string         `json:"topic"`
		Data  map[string]any `json:"data"`
	}
	if json.Unmarshal(payload, &env) != nil {
		return
	}
	switch env.Topic {
	case topicEventsPath:
		if pid, ok := env.Data["pathId"].(string); ok {
			if p := h.cache.Paths[pid]; p != nil {
				if s, ok := env.Data["newStatus"].(string); ok {
					p.Status = game.PathStatus(s)
				}
				if sl, ok := env.Data["surveillanceLevel"].(float64); ok {
					p.SurveillanceLevel = int(sl)
				}
			}
		}
	case topicRingPosition:
		if reg, ok := env.Data["trueRegion"].(string); ok {
			h.lightRingRegion = reg
			h.cache.Ring.TrueRegion = reg
		}
	case topicRingDetection:
		if reg, ok := env.Data["regionId"].(string); ok {
			h.lastDetectedRegion = reg
		}
	case "game.over":
		h.gameOver = true
		if w, ok := env.Data["winner"].(string); ok {
			h.winner = w
		}
	}
}

func (h *Hub) buildStateView(side string) APIState {
	s := APIState{Turn: h.cache.Turn, Started: h.started, GameOver: h.gameOver, Winner: h.winner}
	if side != game.PlayerDark {
		s.RingBearerRegion = h.lightRingRegion
	}
	for _, r := range h.cfg.Regions {
		rs := h.cache.Regions[r.ID]
		s.Regions = append(s.Regions, regionSnap{RegionID: r.ID, ControlledBy: rs.ControlledBy, ThreatLevel: rs.ThreatLevel, Fortified: rs.Fortified})
	}
	for _, u := range h.cfg.Units {
		us := h.cache.Units[u.ID]
		s.Units = append(s.Units, unitSnap{UnitID: u.ID, Region: us.Region, Strength: us.Strength, Status: string(us.Status)})
	}
	for _, p := range h.cfg.Paths {
		ps := h.cache.Paths[p.ID]
		s.Paths = append(s.Paths, pathSnap{PathID: p.ID, Status: string(ps.Status), SurveillanceLevel: ps.SurveillanceLevel, TempOpenTurns: ps.TempOpenTurns})
	}
	return s
}

// cloneWorld rebuilds the world for a newly promoted leader from the latest
// full-state record, or returns nil when no game is in progress. Because the
// record includes routes (units and the Ring Bearer), the promoted leader can
// keep advancing without anything being re-issued.
func (h *Hub) cloneWorld() *game.World {
	if h.lastRecover == nil {
		return nil
	}
	return buildWorldFromRecover(h.cfg, *h.lastRecover)
}

func (h *Hub) analyze(kind string) any {
	if kind == "intercept" {
		start := h.lastDetectedRegion
		if start == "" {
			start = h.cfg.UnitByID["ring-bearer"].StartRegion
		}
		return analysis.BuildInterceptPlan(h.cache, start, h.canonical["Fellowship"])
	}
	start := h.lightRingRegion
	if start == "" {
		start = h.cfg.UnitByID["ring-bearer"].StartRegion
	}
	return analysis.RankRoutes(h.cache, start, h.canonical)
}

// --- public, channel-backed accessors ---

func (h *Hub) Subscribe(side string) *SSEClient {
	c := &SSEClient{Side: side, Ch: make(chan []byte, 64)}
	h.register <- c
	return c
}

func (h *Hub) Unsubscribe(c *SSEClient) {
	select {
	case h.unregister <- c:
	case <-h.closed:
	}
}

func (h *Hub) Analyze(kind string) any {
	reply := make(chan any, 1)
	h.analysisCh <- analysisReq{kind: kind, reply: reply}
	return <-reply
}

func (h *Hub) StateView(side string) APIState {
	reply := make(chan APIState, 1)
	h.stateCh <- stateReq{side: side, reply: reply}
	return <-reply
}

func (h *Hub) RecoverWorld() *game.World {
	reply := make(chan *game.World, 1)
	h.recoverCh <- reply
	return <-reply
}

func canonicalRoutePaths(cfg *config.Config) map[string][]string {
	walks := map[string][]string{
		"Fellowship":        {"the-shire", "bree", "weathertop", "rivendell", "moria", "lothlorien", "emyn-muil", "ithilien", "cirith-ungol", "mount-doom"},
		"Northern Bypass":   {"the-shire", "bree", "rivendell", "lothlorien", "emyn-muil", "dead-marshes", "ithilien", "cirith-ungol", "mount-doom"},
		"Dark Route":        {"the-shire", "bree", "rivendell", "lothlorien", "emyn-muil", "dead-marshes", "mordor", "mount-doom"},
		"Southern Corridor": {"the-shire", "tharbad", "fords-of-isen", "edoras", "minas-tirith", "osgiliath", "minas-morgul", "cirith-ungol", "mount-doom"},
	}
	out := map[string][]string{}
	for name, regions := range walks {
		var pathIds []string
		for i := 0; i+1 < len(regions); i++ {
			if e, ok := cfg.Graph.EdgeBetween(regions[i], regions[i+1]); ok {
				pathIds = append(pathIds, e.PathID)
			}
		}
		out[name] = pathIds
	}
	return out
}
