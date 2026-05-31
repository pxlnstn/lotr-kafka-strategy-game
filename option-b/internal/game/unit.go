package game

import "rotme/internal/config"

const (
	Moved         = "MOVED"
	RouteBlocked  = "BLOCKED"
	RouteComplete = "COMPLETE"
	RouteRejected = "REJECTED"
)

type Advance struct {
	Outcome string
	PathID  string
	From    string
	To      string
}

// AssignRoute sets a unit's route. The ring bearer's route lives in its hidden
// state. Rejected if the unit is not active.
func (w *World) AssignRoute(id string, pathIds []string) bool {
	if w.isRing(id) {
		w.Ring.Route = append([]string(nil), pathIds...)
		w.Ring.RouteIdx = 0
		return true
	}
	u := w.Units[id]
	if u == nil || u.Status != Active {
		return false
	}
	u.Route = append([]string(nil), pathIds...)
	u.RouteIdx = 0
	return true
}

// advanceUnit moves a non-hidden unit one step along its route. The ring
// bearer is advanced separately in turn processing because its region is
// hidden.
func (w *World) advanceUnit(id string) Advance {
	u := w.Units[id]
	if u == nil || u.Status != Active {
		return Advance{Outcome: RouteRejected}
	}
	if len(u.Route) == 0 {
		return Advance{Outcome: RouteRejected}
	}
	if u.RouteIdx >= len(u.Route) {
		return Advance{Outcome: RouteComplete}
	}

	pid := u.Route[u.RouteIdx]
	ps := w.Paths[pid]
	if ps == nil {
		return Advance{Outcome: RouteRejected, PathID: pid}
	}
	if ps.Status == Blocked {
		return Advance{Outcome: RouteBlocked, PathID: pid, From: u.Region}
	}

	to, ok := otherEnd(w.Cfg.PathByID[pid], u.Region)
	if !ok {
		return Advance{Outcome: RouteRejected, PathID: pid}
	}
	from := u.Region
	w.placeUnit(u, to)
	u.RouteIdx++

	out := Moved
	if u.RouteIdx >= len(u.Route) {
		out = RouteComplete
	}
	return Advance{Outcome: out, PathID: pid, From: from, To: to}
}

func otherEnd(p config.Path, current string) (string, bool) {
	switch current {
	case p.From:
		return p.To, true
	case p.To:
		return p.From, true
	default:
		return "", false
	}
}

func (w *World) placeUnit(u *UnitState, to string) {
	if u.Region != "" {
		rs := w.Regions[u.Region]
		rs.Units = removeStr(rs.Units, u.ID)
	}
	u.Region = to
	w.Regions[to].Units = append(w.Regions[to].Units, u.ID)
}

func removeStr(s []string, v string) []string {
	for i, x := range s {
		if x == v {
			return append(s[:i], s[i+1:]...)
		}
	}
	return s
}
