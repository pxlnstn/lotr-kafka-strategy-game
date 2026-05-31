package game

import (
	"testing"

	"rotme/internal/config"
)

// buildState seeds a validation snapshot from a fresh world, then lets each
// test tweak it.
func validationState(t *testing.T) (*config.Config, *ValidationState) {
	t.Helper()
	dir, err := config.FindConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	st := &ValidationState{
		Turn:      1,
		Units:     map[string]UnitView{},
		Paths:     map[string]PathView{},
		Regions:   map[string]RegionView{},
		SeenUnits: map[string]bool{},
	}
	for _, u := range cfg.Units {
		st.Units[u.ID] = UnitView{Side: u.Side, Region: u.StartRegion, Status: string(Active)}
	}
	for _, p := range cfg.Paths {
		st.Paths[p.ID] = PathView{Status: string(Open)}
	}
	for _, r := range cfg.Regions {
		st.Regions[r.ID] = RegionView{ControlledBy: r.StartControl}
	}
	return cfg, st
}

func TestValidate_WrongTurn(t *testing.T) {
	cfg, st := validationState(t)
	o := Order{OrderType: OrderAssignRoute, PlayerID: PlayerLight, UnitID: "ring-bearer", Turn: 5, PathIds: []string{"shire-to-bree"}}
	if got := Validate(o, st, cfg); got != ErrWrongTurn {
		t.Fatalf("got %q, want WRONG_TURN", got)
	}
}

func TestValidate_NotYourUnit(t *testing.T) {
	cfg, st := validationState(t)
	// Light player tries to order a Shadow unit.
	o := Order{OrderType: OrderBlockPath, PlayerID: PlayerLight, UnitID: "witch-king", Turn: 1, PathID: "minas-morgul-to-mordor"}
	if got := Validate(o, st, cfg); got != ErrNotYourUnit {
		t.Fatalf("got %q, want NOT_YOUR_UNIT", got)
	}
}

func TestValidate_InvalidPath(t *testing.T) {
	cfg, st := validationState(t)
	// shire-to-bree is fine, but the next leg is not connected to bree.
	o := Order{OrderType: OrderAssignRoute, PlayerID: PlayerLight, UnitID: "ring-bearer", Turn: 1,
		PathIds: []string{"shire-to-bree", "dead-marshes-to-mordor"}}
	if got := Validate(o, st, cfg); got != ErrInvalidPath {
		t.Fatalf("got %q, want INVALID_PATH", got)
	}
}

func TestValidate_PathBlocked(t *testing.T) {
	cfg, st := validationState(t)
	st.Paths["shire-to-bree"] = PathView{Status: string(Blocked)}
	o := Order{OrderType: OrderAssignRoute, PlayerID: PlayerLight, UnitID: "ring-bearer", Turn: 1, PathIds: []string{"shire-to-bree"}}
	if got := Validate(o, st, cfg); got != ErrPathBlocked {
		t.Fatalf("got %q, want PATH_BLOCKED", got)
	}
}

func TestValidate_UnitNotAdjacent(t *testing.T) {
	cfg, st := validationState(t)
	// nazgul-2 starts at minas-morgul, not an endpoint of shire-to-bree.
	o := Order{OrderType: OrderBlockPath, PlayerID: PlayerDark, UnitID: "nazgul-2", Turn: 1, PathID: "shire-to-bree"}
	if got := Validate(o, st, cfg); got != ErrUnitNotAdj {
		t.Fatalf("got %q, want UNIT_NOT_ADJACENT", got)
	}
}

func TestValidate_InvalidTarget(t *testing.T) {
	cfg, st := validationState(t)
	// Aragorn at bree attacking a non-adjacent region.
	o := Order{OrderType: OrderAttackRegion, PlayerID: PlayerLight, UnitID: "aragorn", Turn: 1, TargetRegion: "mordor"}
	if got := Validate(o, st, cfg); got != ErrInvalidTarget {
		t.Fatalf("got %q, want INVALID_TARGET", got)
	}
}

func TestValidate_OnCooldown(t *testing.T) {
	cfg, st := validationState(t)
	v := st.Units["gandalf"]
	v.Cooldown = 2
	st.Units["gandalf"] = v
	o := Order{OrderType: OrderMaiaAbility, PlayerID: PlayerLight, UnitID: "gandalf", Turn: 1, TargetPathID: "shire-to-bree"}
	if got := Validate(o, st, cfg); got != ErrOnCooldown {
		t.Fatalf("got %q, want ABILITY_ON_COOLDOWN", got)
	}
}

func TestValidate_DuplicateOrder(t *testing.T) {
	cfg, st := validationState(t)
	st.SeenUnits["aragorn"] = true
	o := Order{OrderType: OrderAssignRoute, PlayerID: PlayerLight, UnitID: "aragorn", Turn: 1, PathIds: []string{"bree-to-weathertop"}}
	if got := Validate(o, st, cfg); got != ErrDuplicateOrder {
		t.Fatalf("got %q, want DUPLICATE_UNIT_ORDER", got)
	}
}

func TestValidate_HappyPath(t *testing.T) {
	cfg, st := validationState(t)
	o := Order{OrderType: OrderAssignRoute, PlayerID: PlayerLight, UnitID: "ring-bearer", Turn: 1, PathIds: []string{"shire-to-bree", "bree-to-weathertop"}}
	if got := Validate(o, st, cfg); got != "" {
		t.Fatalf("valid order rejected with %q", got)
	}
}
