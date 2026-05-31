package game

// ProcessTurn runs the fixed 13-step turn end over the orders
// validated for the current turn, mutating the world and returning the events
// to publish. Orders are assumed already validated upstream; steps re-check
// only what affects world consistency (blocker presence, win conditions).
func (w *World) ProcessTurn(orders []Order) []Event {
	var ev []Event

	// Step 2: routes.
	for _, o := range orders {
		switch o.OrderType {
		case OrderAssignRoute:
			w.AssignRoute(o.UnitID, o.PathIds)
		case OrderRedirect:
			w.AssignRoute(o.UnitID, o.NewPathIds)
		}
	}

	// Step 3: blocks and searches. First drop blocks whose holder has left,
	// then apply new ones, then flag compromised routes.
	for _, p := range w.Paths {
		if p.Status == Blocked && !w.blockerStillHolds(p) {
			p.Clear()
			ev = append(ev, pathEvent(w.Turn, p))
		}
	}
	newlyBlocked := map[string]bool{}
	for _, o := range orders {
		switch o.OrderType {
		case OrderBlockPath:
			p := w.Paths[o.PathID]
			// A unit cannot block a path while an enemy unit is holding one of
			// its endpoints: the chokepoint is contested. A FellowshipGuard at
			// the endpoint therefore stops a Nazgul from blocking.
			if p != nil && w.atEndpoint(o.UnitID, o.PathID) && !w.enemyHoldsEndpoint(o.UnitID, o.PathID) {
				p.Block(o.UnitID)
				if p.Status == Blocked {
					newlyBlocked[o.PathID] = true
				}
				ev = append(ev, pathEvent(w.Turn, p))
			}
		case OrderSearchPath:
			p := w.Paths[o.PathID]
			if p != nil {
				p.Search()
				ev = append(ev, pathEvent(w.Turn, p))
			}
		}
	}
	compromised := map[string]bool{}
	ringCompromised := false
	for pid := range newlyBlocked {
		for id, u := range w.Units {
			if u.Status == Active && u.RouteIdx < len(u.Route) && u.Route[u.RouteIdx] == pid {
				compromised[id] = true
				ev = append(ev, Event{Type: EvRouteCompromised, Turn: w.Turn, UnitID: id, PathID: pid, Audience: AudAll})
			}
		}
		if w.Ring.RouteIdx < len(w.Ring.Route) && w.Ring.Route[w.Ring.RouteIdx] == pid {
			ringCompromised = true
			ev = append(ev, Event{Type: EvRouteCompromised, Turn: w.Turn, UnitID: w.ringID, PathID: pid, Audience: AudLight})
		}
	}

	// Step 4: reinforcement and deployment (reposition the unit).
	for _, o := range orders {
		if o.OrderType == OrderReinforce || o.OrderType == OrderDeployNazgul {
			if u := w.Units[o.UnitID]; u != nil && u.Status == Active {
				if _, ok := w.Regions[o.TargetRegion]; ok {
					w.placeUnit(u, o.TargetRegion)
				}
			}
		}
	}

	// Step 5: fortify.
	for _, o := range orders {
		if o.OrderType == OrderFortify {
			cfg := w.Cfg.UnitByID[o.UnitID]
			u := w.Units[o.UnitID]
			if cfg.CanFortify && u != nil && u.Status == Active {
				r := w.Regions[u.Region]
				r.Fortified = true
				r.FortifyTurns = 2
			}
		}
	}

	// Step 6: Maia abilities. Dispatch by config, not id.
	for _, o := range orders {
		if o.OrderType == OrderMaiaAbility {
			ev = append(ev, w.maiaAbility(o)...)
		}
	}

	// Step 7: auto-advance everyone with a route, ring bearer included.
	for id, u := range w.Units {
		if w.isRing(id) || compromised[id] || u.Status != Active || len(u.Route) == 0 {
			continue
		}
		a := w.advanceUnit(id)
		if a.Outcome == Moved || a.Outcome == RouteComplete {
			if a.To != "" {
				ev = append(ev, Event{Type: EvUnitMoved, Turn: w.Turn, UnitID: id, From: a.From, To: a.To, PathID: a.PathID, Audience: AudAll})
			}
		}
	}
	if !ringCompromised {
		ev = append(ev, w.advanceRing()...)
	}

	// Step 8: combat.
	ev = append(ev, w.resolveAttacks(orders)...)

	// Step 9: temp-open timers.
	for _, p := range w.Paths {
		before := p.Status
		p.tickTempOpen()
		if p.Status != before {
			ev = append(ev, pathEvent(w.Turn, p))
		}
	}

	// Step 10: fortification timers.
	for _, r := range w.Regions {
		if r.Fortified {
			r.FortifyTurns--
			if r.FortifyTurns <= 0 {
				r.Fortified = false
				r.FortifyTurns = 0
			}
		}
	}

	// Step 11: respawn and cooldown counters.
	for id, u := range w.Units {
		if u.Status == Respawning {
			u.RespawnTurns--
			if u.RespawnTurns <= 0 {
				cfg := w.Cfg.UnitByID[id]
				u.Status = Active
				u.Strength = cfg.Strength
				u.RespawnTurns = 0
				w.placeUnit(u, cfg.StartRegion)
			}
		}
		if u.Cooldown > 0 {
			u.Cooldown--
		}
	}

	// Step 12: detection.
	for _, d := range RunDetection(w) {
		ev = append(ev, Event{Type: EvRingBearerDetected, Turn: w.Turn, UnitID: d.UnitID, RegionID: d.Region, Audience: AudDark})
	}

	// Step 13: win evaluation.
	ev = append(ev, w.evaluateWin(orders)...)
	ev = append(ev, Event{Type: "WorldStateSnapshot", Turn: w.Turn, Audience: AudAll})
	w.Ring.Exposed = false

	return ev
}

func (w *World) advanceRing() []Event {
	r := w.Ring
	if len(r.Route) == 0 || r.RouteIdx >= len(r.Route) {
		return nil
	}
	pid := r.Route[r.RouteIdx]
	ps := w.Paths[pid]
	if ps == nil || ps.Status == Blocked {
		return nil
	}
	to, ok := otherEnd(w.Cfg.PathByID[pid], r.TrueRegion)
	if !ok {
		return nil
	}
	r.TrueRegion = to
	r.RouteIdx++

	ev := []Event{{Type: EvRingBearerMoved, Turn: w.Turn, TrueRegion: to, Audience: AudLight}}
	if ps.SurveillanceLevel >= 1 && w.Turn > w.Cfg.HiddenUntilTurn {
		r.Exposed = true
		ev = append(ev, Event{Type: EvRingBearerSpotted, Turn: w.Turn, PathID: pid, Audience: AudDark})
	}
	return ev
}

func (w *World) maiaAbility(o Order) []Event {
	cfg := w.Cfg.UnitByID[o.UnitID]
	u := w.Units[o.UnitID]
	p := w.Paths[o.TargetPathID]
	if u == nil || p == nil || u.Status != Active {
		return nil
	}
	if !w.atEndpoint(o.UnitID, o.TargetPathID) {
		return nil
	}

	// A maia that lists ability paths corrupts; one that lists none opens.
	if len(cfg.MaiaAbilityPaths) > 0 {
		if w.SarumanDisabled || !contains(cfg.MaiaAbilityPaths, o.TargetPathID) {
			return nil
		}
		p.Corrupt()
		u.Cooldown = cfg.Cooldown
		return []Event{
			{Type: EvPathCorrupted, Turn: w.Turn, PathID: p.ID, Audience: AudAll},
			pathEvent(w.Turn, p),
		}
	}

	if !p.OpenTemporarily() {
		return nil
	}
	u.Cooldown = cfg.Cooldown
	return []Event{pathEvent(w.Turn, p)}
}

func (w *World) resolveAttacks(orders []Order) []Event {
	byRegion := map[string][]Order{}
	for _, o := range orders {
		if o.OrderType == OrderAttackRegion {
			byRegion[o.TargetRegion] = append(byRegion[o.TargetRegion], o)
		}
	}
	var ev []Event
	for region, group := range byRegion {
		reg := w.Regions[region]
		if reg == nil {
			continue
		}
		var attackers []Combatant
		var attackerSide string
		for _, o := range group {
			u := w.Units[o.UnitID]
			if u == nil || u.Status != Active {
				continue
			}
			cfg := w.Cfg.UnitByID[o.UnitID]
			if !w.Cfg.Graph.Adjacent(u.Region, region) {
				continue
			}
			attackers = append(attackers, Combatant{Cfg: cfg, Strength: u.Strength})
			attackerSide = cfg.Side
		}
		if len(attackers) == 0 {
			continue
		}

		var defenders []Combatant
		var defenderIDs []string
		for _, id := range reg.Units {
			cfg := w.Cfg.UnitByID[id]
			if cfg.Side != attackerSide && cfg.Side != SideNeutral {
				defenders = append(defenders, Combatant{Cfg: cfg, Strength: w.Units[id].Strength})
				defenderIDs = append(defenderIDs, id)
			}
		}

		terrain := w.Cfg.RegionByID[region].Terrain
		res := ResolveCombat(attackers, defenders, terrain, reg.Fortified)
		ev = append(ev, Event{Type: EvBattleResolved, Turn: w.Turn, RegionID: region, AttackerWon: res.AttackerWon, Audience: AudAll})

		if res.AttackerWon {
			for _, id := range defenderIDs {
				w.Units[id].ApplyDamage(w.Cfg.UnitByID[id], res.Damage)
				if w.Units[id].Status != Active {
					reg.Units = removeStr(reg.Units, id)
				}
			}
			reg.ControlledBy = attackerSide
			ev = append(ev, Event{Type: EvRegionControl, Turn: w.Turn, RegionID: region, NewController: attackerSide, Audience: AudAll})
			if region == w.sarumanHomeID && attackerSide == SideFree {
				w.SarumanDisabled = true
			}
		} else {
			for _, o := range group {
				if u := w.Units[o.UnitID]; u != nil && u.Status == Active {
					u.ApplyDamage(w.Cfg.UnitByID[o.UnitID], 1)
				}
			}
		}
	}
	return ev
}

func (w *World) evaluateWin(orders []Order) []Event {
	destroyRing := false
	for _, o := range orders {
		if o.OrderType == OrderDestroyRing {
			destroyRing = true
		}
	}

	ringAtDoom := w.Ring.TrueRegion == w.mountDoomID
	switch {
	case ringAtDoom && destroyRing && !w.shadowPresent(w.mountDoomID):
		return w.gameOver(SideFree, "RING_DESTROYED")
	case w.nazgulPresent(w.Ring.TrueRegion) && w.Ring.Exposed:
		return w.gameOver(SideShadow, "RING_CAPTURED")
	case w.Turn >= w.Cfg.MaxTurns:
		return w.gameOver("DRAW", "TURN_LIMIT")
	}
	return nil
}

func (w *World) gameOver(winner, cause string) []Event {
	w.GameOver = true
	w.Winner = winner
	return []Event{{Type: EvGameOver, Turn: w.Turn, Winner: winner, Cause: cause, Audience: AudAll}}
}

// --- small helpers ---

func (w *World) unitRegion(id string) string {
	if w.isRing(id) {
		return w.Ring.TrueRegion
	}
	if u := w.Units[id]; u != nil {
		return u.Region
	}
	return ""
}

func (w *World) atEndpoint(unitID, pathID string) bool {
	p, ok := w.Cfg.PathByID[pathID]
	if !ok {
		return false
	}
	r := w.unitRegion(unitID)
	return r == p.From || r == p.To
}

// enemyHoldsEndpoint reports whether an active enemy unit sits at either
// endpoint of the path, which contests a block attempt.
func (w *World) enemyHoldsEndpoint(blockerID, pathID string) bool {
	p, ok := w.Cfg.PathByID[pathID]
	if !ok {
		return false
	}
	side := w.Cfg.UnitByID[blockerID].Side
	for _, endpoint := range []string{p.From, p.To} {
		reg := w.Regions[endpoint]
		if reg == nil {
			continue
		}
		for _, id := range reg.Units {
			u := w.Units[id]
			if u != nil && u.Status == Active && opposed(w.Cfg.UnitByID[id].Side, side) {
				return true
			}
		}
	}
	return false
}

func opposed(a, b string) bool {
	return (a == SideFree && b == SideShadow) || (a == SideShadow && b == SideFree)
}

func (w *World) blockerStillHolds(p *PathState) bool {
	if p.BlockedBy == "" {
		return false
	}
	u := w.Units[p.BlockedBy]
	return u != nil && u.Status == Active && w.atEndpoint(p.BlockedBy, p.ID)
}

func (w *World) shadowPresent(region string) bool {
	for id, u := range w.Units {
		if u.Status == Active && u.Region == region && w.Cfg.UnitByID[id].Side == SideShadow {
			return true
		}
	}
	return false
}

func (w *World) nazgulPresent(region string) bool {
	for id, u := range w.Units {
		if u.Status == Active && u.Region == region && w.Cfg.UnitByID[id].Class == "Nazgul" {
			return true
		}
	}
	return false
}

func pathEvent(turn int, p *PathState) Event {
	return Event{
		Type: EvPathStatusChanged, Turn: turn, PathID: p.ID,
		Status: string(p.Status), SurveillanceLevel: p.SurveillanceLevel,
		TempOpenTurns: p.TempOpenTurns, Audience: AudAll,
	}
}

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}
