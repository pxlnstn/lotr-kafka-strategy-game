package game

import (
	"testing"

	"rotme/internal/config"
)

func freshWorld(t *testing.T) *World {
	t.Helper()
	dir, err := config.FindConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	return NewWorld(cfg)
}

func hasEvent(ev []Event, typ string) *Event {
	for i := range ev {
		if ev[i].Type == typ {
			return &ev[i]
		}
	}
	return nil
}

func TestTurn_RingAdvancesLightOnly(t *testing.T) {
	w := freshWorld(t)
	ev := w.ProcessTurn([]Order{
		{OrderType: OrderAssignRoute, UnitID: "ring-bearer", PathIds: []string{"shire-to-bree"}},
	})
	if w.Ring.TrueRegion != "bree" {
		t.Fatalf("ring should be at bree, got %s", w.Ring.TrueRegion)
	}
	m := hasEvent(ev, EvRingBearerMoved)
	if m == nil || m.Audience != AudLight {
		t.Fatal("RingBearerMoved must be emitted to LIGHT only")
	}
}

func TestTurn_SurveillanceExposesRingBearer(t *testing.T) {
	w := freshWorld(t)
	w.Turn = 5 // past hidden start
	w.Paths["shire-to-bree"].SurveillanceLevel = 1
	ev := w.ProcessTurn([]Order{
		{OrderType: OrderAssignRoute, UnitID: "ring-bearer", PathIds: []string{"shire-to-bree"}},
	})
	if !w.Ring.Exposed && hasEvent(ev, EvRingBearerSpotted) == nil {
		// exposed is reset at end of turn, so assert via the emitted event
		t.Fatal("crossing a surveilled path should emit RingBearerSpotted")
	}
	if s := hasEvent(ev, EvRingBearerSpotted); s == nil || s.Audience != AudDark {
		t.Fatal("RingBearerSpotted must go to DARK only")
	}
}

func TestTurn_BlockRevertsWhenBlockerLeaves(t *testing.T) {
	w := freshWorld(t)
	// Aragorn starts at bree; move him away so he doesn't contest the block.
	w.placeUnit(w.Units["aragorn"], "rivendell")
	// Park a nazgul at an endpoint and block the path.
	w.placeUnit(w.Units["nazgul-2"], "bree")
	w.ProcessTurn([]Order{
		{OrderType: OrderBlockPath, UnitID: "nazgul-2", PathID: "shire-to-bree"},
	})
	if w.Paths["shire-to-bree"].Status != Blocked {
		t.Fatalf("path should be blocked, got %s", w.Paths["shire-to-bree"].Status)
	}
	// Move the blocker away; next turn the block must revert.
	w.placeUnit(w.Units["nazgul-2"], "weathertop")
	w.ProcessTurn(nil)
	if w.Paths["shire-to-bree"].Status != Open {
		t.Fatalf("block should revert to OPEN once blocker leaves, got %s", w.Paths["shire-to-bree"].Status)
	}
}

func TestTurn_GuardPreventsNazgulBlock(t *testing.T) {
	w := freshWorld(t)
	// Aragorn (a Fellowship guard) holds the-shire; a nazgul at bree tries to
	// block shire-to-bree. The block must fail while the guard is present.
	w.placeUnit(w.Units["aragorn"], "the-shire")
	w.placeUnit(w.Units["nazgul-2"], "bree")
	w.ProcessTurn([]Order{
		{OrderType: OrderBlockPath, UnitID: "nazgul-2", PathID: "shire-to-bree"},
	})
	if w.Paths["shire-to-bree"].Status == Blocked {
		t.Fatal("block should fail while a guard holds the endpoint")
	}

	// Move the guard away; now the block should take.
	w.placeUnit(w.Units["aragorn"], "rivendell")
	w.ProcessTurn([]Order{
		{OrderType: OrderBlockPath, UnitID: "nazgul-2", PathID: "shire-to-bree"},
	})
	if w.Paths["shire-to-bree"].Status != Blocked {
		t.Fatalf("block should succeed once the guard leaves, got %s", w.Paths["shire-to-bree"].Status)
	}
}

func TestTurn_MaiaDispatchByConfig(t *testing.T) {
	w := freshWorld(t)

	// Gandalf (no ability paths) opens a blocked path he stands at an endpoint of.
	// A nazgul actually holds the block from the other endpoint.
	w.placeUnit(w.Units["gandalf"], "bree")
	w.placeUnit(w.Units["nazgul-2"], "the-shire")
	w.Paths["shire-to-bree"].Status = Blocked
	w.Paths["shire-to-bree"].BlockedBy = "nazgul-2"
	w.ProcessTurn([]Order{
		{OrderType: OrderMaiaAbility, UnitID: "gandalf", TargetPathID: "shire-to-bree"},
	})
	if w.Paths["shire-to-bree"].Status != TempOpen {
		t.Fatalf("gandalf should temporarily open, got %s", w.Paths["shire-to-bree"].Status)
	}

	// Saruman (has ability paths) corrupts one of his listed paths.
	w2 := freshWorld(t)
	w2.placeUnit(w2.Units["saruman"], "fords-of-isen")
	ev := w2.ProcessTurn([]Order{
		{OrderType: OrderMaiaAbility, UnitID: "saruman", TargetPathID: "fords-of-isen-to-edoras"},
	})
	if w2.Paths["fords-of-isen-to-edoras"].SurveillanceLevel != 3 {
		t.Fatal("saruman should corrupt to surveillance 3")
	}
	if hasEvent(ev, EvPathCorrupted) == nil {
		t.Fatal("PathCorrupted should fire")
	}
}

func TestTurn_LightWinsAtMountDoom(t *testing.T) {
	w := freshWorld(t)
	w.Ring.TrueRegion = "mount-doom"
	// Make sure no shadow unit is sitting in mount-doom.
	for id, u := range w.Units {
		if w.Cfg.UnitByID[id].Side == SideShadow {
			u.Region = "minas-morgul"
		}
	}
	ev := w.ProcessTurn([]Order{
		{OrderType: OrderDestroyRing, UnitID: "ring-bearer"},
	})
	g := hasEvent(ev, EvGameOver)
	if g == nil || g.Winner != SideFree {
		t.Fatalf("light should win at mount-doom: %+v", g)
	}
}
