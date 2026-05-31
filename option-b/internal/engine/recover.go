package engine

import (
	"rotme/internal/config"
	"rotme/internal/game"
)

// recoverState is the full world serialized into game.session so a newly
// promoted leader can restore everything, routes included. game.session is
// never forwarded to a browser, so putting the Ring Bearer's route here does
// not break information hiding.
type recoverState struct {
	Turn            int                      `json:"turn"`
	GameOver        bool                     `json:"gameOver"`
	Winner          string                   `json:"winner"`
	SarumanDisabled bool                     `json:"sarumanDisabled"`
	RingRegion      string                   `json:"ringRegion"`
	RingRoute       []string                 `json:"ringRoute"`
	RingRouteIdx    int                      `json:"ringRouteIdx"`
	RingExposed     bool                     `json:"ringExposed"`
	Units           map[string]recoverUnit   `json:"units"`
	Regions         map[string]recoverRegion `json:"regions"`
	Paths           map[string]recoverPath   `json:"paths"`
}

type recoverUnit struct {
	Region       string   `json:"region"`
	Strength     int      `json:"strength"`
	Status       string   `json:"status"`
	Route        []string `json:"route"`
	RouteIdx     int      `json:"routeIdx"`
	Cooldown     int      `json:"cooldown"`
	RespawnTurns int      `json:"respawnTurns"`
}

type recoverRegion struct {
	ControlledBy string `json:"controlledBy"`
	ThreatLevel  int    `json:"threatLevel"`
	Fortified    bool   `json:"fortified"`
	FortifyTurns int    `json:"fortifyTurns"`
}

type recoverPath struct {
	Status            string `json:"status"`
	SurveillanceLevel int    `json:"surveillanceLevel"`
	TempOpenTurns     int    `json:"tempOpenTurns"`
	BlockedBy         string `json:"blockedBy"`
}

func buildRecoverState(w *game.World) recoverState {
	rs := recoverState{
		Turn: w.Turn, GameOver: w.GameOver, Winner: w.Winner, SarumanDisabled: w.SarumanDisabled,
		RingRegion: w.Ring.TrueRegion, RingRoute: w.Ring.Route, RingRouteIdx: w.Ring.RouteIdx, RingExposed: w.Ring.Exposed,
		Units:   make(map[string]recoverUnit, len(w.Units)),
		Regions: make(map[string]recoverRegion, len(w.Regions)),
		Paths:   make(map[string]recoverPath, len(w.Paths)),
	}
	for id, u := range w.Units {
		rs.Units[id] = recoverUnit{u.Region, u.Strength, string(u.Status), u.Route, u.RouteIdx, u.Cooldown, u.RespawnTurns}
	}
	for id, r := range w.Regions {
		rs.Regions[id] = recoverRegion{r.ControlledBy, r.ThreatLevel, r.Fortified, r.FortifyTurns}
	}
	for id, p := range w.Paths {
		rs.Paths[id] = recoverPath{string(p.Status), p.SurveillanceLevel, p.TempOpenTurns, p.BlockedBy}
	}
	return rs
}

// buildWorldFromRecover rebuilds a World from a recoverState. The derived ids
// (ring, mount-doom, saruman home) come for free from NewWorld.
func buildWorldFromRecover(cfg *config.Config, rs recoverState) *game.World {
	w := game.NewWorld(cfg)
	w.Turn = rs.Turn
	w.GameOver = rs.GameOver
	w.Winner = rs.Winner
	w.SarumanDisabled = rs.SarumanDisabled
	w.Ring.TrueRegion = rs.RingRegion
	w.Ring.Route = rs.RingRoute
	w.Ring.RouteIdx = rs.RingRouteIdx
	w.Ring.Exposed = rs.RingExposed

	for id, ru := range rs.Units {
		if u := w.Units[id]; u != nil {
			u.Region = ru.Region
			u.Strength = ru.Strength
			u.Status = game.Status(ru.Status)
			u.Route = ru.Route
			u.RouteIdx = ru.RouteIdx
			u.Cooldown = ru.Cooldown
			u.RespawnTurns = ru.RespawnTurns
		}
	}
	for id, rr := range rs.Regions {
		if r := w.Regions[id]; r != nil {
			r.ControlledBy = rr.ControlledBy
			r.ThreatLevel = rr.ThreatLevel
			r.Fortified = rr.Fortified
			r.FortifyTurns = rr.FortifyTurns
			r.Units = nil
		}
	}
	for id, rp := range rs.Paths {
		if p := w.Paths[id]; p != nil {
			p.Status = game.PathStatus(rp.Status)
			p.SurveillanceLevel = rp.SurveillanceLevel
			p.TempOpenTurns = rp.TempOpenTurns
			p.BlockedBy = rp.BlockedBy
		}
	}
	// Rebuild each region's present-units list from the restored positions.
	for id, u := range w.Units {
		if u.Status == game.Active && u.Region != "" && cfg.UnitByID[id].Class != "RingBearer" {
			if r := w.Regions[u.Region]; r != nil {
				r.Units = append(r.Units, id)
			}
		}
	}
	return w
}
