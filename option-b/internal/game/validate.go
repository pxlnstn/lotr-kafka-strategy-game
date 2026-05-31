package game

// Order validation (Topology 1). Pure over a snapshot of the
// KTable state so it can be unit tested without Kafka and reused by the
// validation service.

const (
	PlayerLight = "light"
	PlayerDark  = "dark"
)

const (
	ErrWrongTurn      = "WRONG_TURN"
	ErrNotYourUnit    = "NOT_YOUR_UNIT"
	ErrInvalidPath    = "INVALID_PATH"
	ErrPathBlocked    = "PATH_BLOCKED"
	ErrUnitNotAdj     = "UNIT_NOT_ADJACENT"
	ErrInvalidTarget  = "INVALID_TARGET"
	ErrOnCooldown     = "ABILITY_ON_COOLDOWN"
	ErrDuplicateOrder = "DUPLICATE_UNIT_ORDER"
)

// UnitView is the slice of unit state validation needs.
type UnitView struct {
	Side     string
	Region   string
	Status   string
	Cooldown int
}

type PathView struct {
	Status string
}

type RegionView struct {
	ControlledBy string
}

// ValidationState is the KTable snapshot the validator reads.
type ValidationState struct {
	Turn      int
	Units     map[string]UnitView
	Paths     map[string]PathView
	Regions   map[string]RegionView
	SeenUnits map[string]bool // unit ids already ordered this turn
}

func playerSide(playerID string) string {
	if playerID == PlayerDark {
		return SideShadow
	}
	return SideFree
}

// Validate applies the eight rules in order and returns "" when the order is
// valid, or the error code of the first rule it breaks.
func Validate(o Order, st *ValidationState, cfg interface {
	UnitSide(id string) (string, bool)
	PathEndpoints(id string) (string, string, bool)
	Adjacent(a, b string) bool
}) string {
	// Rule 1: turn must match.
	if o.Turn != st.Turn {
		return ErrWrongTurn
	}
	// Rule 8: at most one order per unit per turn.
	if st.SeenUnits[o.UnitID] {
		return ErrDuplicateOrder
	}
	// Rule 2: the unit must belong to the submitting player's side.
	side, ok := cfg.UnitSide(o.UnitID)
	if !ok || side != playerSide(o.PlayerID) {
		return ErrNotYourUnit
	}

	switch o.OrderType {
	case OrderAssignRoute, OrderRedirect:
		paths := o.PathIds
		if o.OrderType == OrderRedirect {
			paths = o.NewPathIds
		}
		u := st.Units[o.UnitID]
		// Rule 4: the route must be a connected walk from the unit's region.
		cur := u.Region
		for _, pid := range paths {
			a, b, ok := cfg.PathEndpoints(pid)
			if !ok || (cur != a && cur != b) {
				return ErrInvalidPath
			}
			if cur == a {
				cur = b
			} else {
				cur = a
			}
		}
		// Rule 3: the first step must not be blocked.
		if len(paths) > 0 {
			if p, ok := st.Paths[paths[0]]; ok && p.Status == string(Blocked) {
				return ErrPathBlocked
			}
		}

	case OrderBlockPath, OrderSearchPath:
		// Rule 5: the unit must stand at an endpoint of the path.
		a, b, ok := cfg.PathEndpoints(o.PathID)
		if !ok {
			return ErrInvalidPath
		}
		r := st.Units[o.UnitID].Region
		if r != a && r != b {
			return ErrUnitNotAdj
		}

	case OrderAttackRegion:
		// Rule 6: target must be adjacent and enemy-controlled.
		r := st.Units[o.UnitID].Region
		if !cfg.Adjacent(r, o.TargetRegion) {
			return ErrInvalidTarget
		}
		reg, ok := st.Regions[o.TargetRegion]
		if !ok || reg.ControlledBy == side {
			return ErrInvalidTarget
		}

	case OrderMaiaAbility:
		// Rule 7: ability must be off cooldown.
		if st.Units[o.UnitID].Cooldown > 0 {
			return ErrOnCooldown
		}
	}

	return ""
}
