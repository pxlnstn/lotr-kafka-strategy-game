package engine

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"rotme/internal/game"
	"rotme/internal/kafka"
)

func nowMs() int64 { return time.Now().UnixMilli() }

func (e *Engine) publish(ctx context.Context, topic, schemaName, key string, val any) {
	id, ok := e.schemaIDs[schemaName]
	if !ok {
		log.Printf("no schema id for %s", schemaName)
		return
	}
	if err := e.prod.Produce(ctx, topic, key, id, val); err != nil {
		log.Printf("produce %s: %v", topic, err)
	}
}

func (e *Engine) publishValidated(ctx context.Context, o game.Order, risk *int) {
	payload, _ := json.Marshal(o)
	msg := orderValidatedMsg{
		PlayerID: o.PlayerID, UnitID: o.UnitID, OrderType: o.OrderType,
		Payload: payload, Turn: o.Turn, Timestamp: nowMs(), RouteRiskScore: risk,
	}
	e.publish(ctx, topicOrdersValidated, "OrderValidated", o.UnitID, msg)
}

func (e *Engine) publishDLQ(ctx context.Context, o game.Order, code string, raw []byte) {
	d := dlqMsg{
		OriginalTopic: topicOrdersRaw, ErrorCode: code,
		ErrorMessage: "order rejected by validation", RawPayload: raw, Timestamp: nowMs(),
	}
	e.publish(ctx, topicDLQ, "DLQEntry", code, d)
}

func (e *Engine) publishEvents(ctx context.Context, events []game.Event) {
	for _, ev := range events {
		switch ev.Type {
		case game.EvGameOver:
			// Exactly-once: the GameOver event is written in a transaction so
			// it appears once even if the engine crashes mid-publish (K6).
			msg := gameOverMsg{Winner: ev.Winner, Cause: ev.Cause, Turn: ev.Turn, Timestamp: nowMs()}
			id := e.schemaIDs["GameOver"]
			if err := e.txn.ProduceInTxn(ctx, topicBroadcast, gameID, id, msg); err != nil {
				log.Printf("gameover txn: %v", err)
			}
		case "WorldStateSnapshot":
			// Published separately from the full world; ignore the marker.
		default:
			topic, schema, ok := routeTopic(ev.Type)
			if !ok {
				continue
			}
			e.publish(ctx, topic, schema, eventKey(ev), eventToDTO(ev))
		}
	}
}

func eventKey(ev game.Event) string {
	switch ev.Type {
	case game.EvUnitMoved:
		return ev.UnitID
	case game.EvPathStatusChanged, game.EvPathCorrupted:
		return ev.PathID
	case game.EvRegionControl, game.EvBattleResolved:
		return ev.RegionID
	case game.EvRingBearerMoved:
		return "light"
	case game.EvRingBearerDetected, game.EvRingBearerSpotted:
		return "dark"
	}
	return ""
}

func eventToDTO(ev game.Event) any {
	ts := nowMs()
	switch ev.Type {
	case game.EvUnitMoved:
		return unitMovedMsg{UnitID: ev.UnitID, From: ev.From, To: ev.To, Turn: ev.Turn, Timestamp: ts}
	case game.EvPathStatusChanged:
		return pathStatusChangedMsg{PathID: ev.PathID, NewStatus: ev.Status, SurveillanceLevel: ev.SurveillanceLevel, TempOpenTurns: ev.TempOpenTurns, Turn: ev.Turn, Timestamp: ts}
	case game.EvPathCorrupted:
		return pathCorruptedMsg{PathID: ev.PathID, Turn: ev.Turn, Timestamp: ts}
	case game.EvRegionControl:
		return regionControlMsg{RegionID: ev.RegionID, NewController: ev.NewController, Turn: ev.Turn, Timestamp: ts}
	case game.EvBattleResolved:
		return battleResolvedMsg{RegionID: ev.RegionID, AttackerWon: ev.AttackerWon, Turn: ev.Turn, Timestamp: ts}
	case game.EvRingBearerMoved:
		return ringMovedMsg{TrueRegion: ev.TrueRegion, Turn: ev.Turn, Timestamp: ts}
	case game.EvRingBearerDetected:
		return ringDetectedMsg{RegionID: ev.RegionID, Turn: ev.Turn, Timestamp: ts}
	case game.EvRingBearerSpotted:
		return ringSpottedMsg{PathID: ev.PathID, Turn: ev.Turn, Timestamp: ts}
	}
	return nil
}

func (e *Engine) buildSnapshot() worldSnapshotMsg {
	snap := worldSnapshotMsg{Timestamp: nowMs()}
	if e.world == nil {
		return snap
	}
	snap.Turn = e.world.Turn
	for _, r := range e.cfg.Regions {
		rs := e.world.Regions[r.ID]
		snap.Regions = append(snap.Regions, regionSnap{
			RegionID: r.ID, ControlledBy: rs.ControlledBy, ThreatLevel: rs.ThreatLevel, Fortified: rs.Fortified,
		})
	}
	for _, u := range e.cfg.Units {
		us := e.world.Units[u.ID]
		snap.Units = append(snap.Units, unitSnap{
			UnitID: u.ID, Region: us.Region, Strength: us.Strength, Status: string(us.Status),
		})
	}
	return snap
}

func (e *Engine) publishSnapshot(ctx context.Context) {
	e.publish(ctx, topicBroadcast, "WorldStateSnapshot", gameID, e.buildSnapshot())
}

func (e *Engine) publishSession(ctx context.Context) {
	phase := "IN_PROGRESS"
	var winner *string
	if e.world.GameOver {
		phase = "OVER"
		w := e.world.Winner
		winner = &w
	}
	blob, _ := json.Marshal(buildRecoverState(e.world))
	world := string(blob)
	msg := gameSessionMsg{GameID: gameID, Turn: e.world.Turn, Phase: phase, Winner: winner, WorldJSON: &world, Timestamp: nowMs()}
	e.publish(ctx, topicSession, "GameSession", gameID, msg)
}

// runValidation consumes raw orders, validates each against the latest world
// snapshot, and publishes to validated or the DLQ. This is Topology 1.
func (e *Engine) runValidation(ctx context.Context, cons *kafka.Consumer) {
	seen := map[string]bool{}
	curTurn := -1
	for {
		recs, err := cons.Poll(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			continue
		}
		for _, r := range recs {
			var m orderSubmittedMsg
			if _, err := cons.Decode(r.Value, &m); err != nil {
				continue
			}
			var o game.Order
			if err := json.Unmarshal(m.Payload, &o); err != nil {
				continue
			}
			// Control actions bypass validation and drive the core loop.
			switch o.OrderType {
			case ctrlGameStart:
				e.startCh <- gameID
				continue
			case ctrlTurnAdvance:
				e.advanceCh <- make(chan struct{})
				continue
			}
			st := e.valState.Load()
			if st == nil {
				continue // no game in progress yet
			}
			if st.Turn != curTurn {
				curTurn = st.Turn
				seen = map[string]bool{}
			}
			stc := *st
			stc.SeenUnits = seen
			if code := game.Validate(o, &stc, e.cfg); code != "" {
				e.publishDLQ(ctx, o, code, m.Payload)
				continue
			}
			seen[o.UnitID] = true
			e.publishValidated(ctx, o, nil)
		}
	}
}

func (e *Engine) runValidatedConsumer(ctx context.Context, cons *kafka.Consumer) {
	for {
		recs, err := cons.Poll(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			continue
		}
		for _, r := range recs {
			var m orderValidatedMsg
			if _, err := cons.Decode(r.Value, &m); err != nil {
				continue
			}
			var o game.Order
			if err := json.Unmarshal(m.Payload, &o); err != nil {
				continue
			}
			select {
			case e.validatedCh <- validatedOrder{O: o, Risk: m.RouteRiskScore}:
			case <-ctx.Done():
				return
			}
		}
	}
}
