package game

import (
	"testing"

	"rotme/internal/config"
)

func TestUnit_AssignRoute(t *testing.T) {
	w := newTestWorld(t)
	ok := w.AssignRoute("aragorn", []string{"bree-to-weathertop", "weathertop-to-rivendell"})
	if !ok || len(w.Units["aragorn"].Route) != 2 {
		t.Fatal("route not set")
	}
}

func TestUnit_EmptyRouteRejected(t *testing.T) {
	w := newTestWorld(t)
	if a := w.advanceUnit("aragorn"); a.Outcome != RouteRejected {
		t.Fatal(a.Outcome)
	}
}

func TestUnit_MovesOnOpenPath(t *testing.T) {
	w := newTestWorld(t)
	w.AssignRoute("aragorn", []string{"bree-to-weathertop", "weathertop-to-rivendell"})
	a := w.advanceUnit("aragorn")
	if a.Outcome != Moved || w.Units["aragorn"].Region != "weathertop" {
		t.Fatalf("%+v region=%s", a, w.Units["aragorn"].Region)
	}
}

func TestUnit_StaysOnBlockedPath(t *testing.T) {
	w := newTestWorld(t)
	w.Paths["bree-to-weathertop"].Status = Blocked
	w.AssignRoute("aragorn", []string{"bree-to-weathertop"})
	a := w.advanceUnit("aragorn")
	if a.Outcome != RouteBlocked || w.Units["aragorn"].Region != "bree" {
		t.Fatalf("%+v region=%s", a, w.Units["aragorn"].Region)
	}
}

func TestUnit_LastPathCompletes(t *testing.T) {
	w := newTestWorld(t)
	w.AssignRoute("aragorn", []string{"bree-to-weathertop"})
	a := w.advanceUnit("aragorn")
	if a.Outcome != RouteComplete || w.Units["aragorn"].Region != "weathertop" {
		t.Fatalf("%+v", a)
	}
}

func TestUnit_DamageReducesStrength(t *testing.T) {
	u := &UnitState{Strength: 5, Status: Active}
	u.ApplyDamage(config.UnitConfig{}, 2)
	if u.Strength != 3 || u.Status != Active {
		t.Fatal(u)
	}
}

func TestUnit_RespawningRejectsCommands(t *testing.T) {
	w := newTestWorld(t)
	w.Units["nazgul-2"].Status = Respawning
	if w.AssignRoute("nazgul-2", []string{"shire-to-bree"}) {
		t.Fatal("respawning unit should reject AssignRoute")
	}
	if a := w.advanceUnit("nazgul-2"); a.Outcome != RouteRejected {
		t.Fatal(a.Outcome)
	}
}

func TestUnit_DestroyedRejectsCommands(t *testing.T) {
	w := newTestWorld(t)
	w.Units["uruk-hai-legion"].Status = Destroyed
	if w.AssignRoute("uruk-hai-legion", []string{"fangorn-to-isengard"}) {
		t.Fatal("destroyed unit should reject AssignRoute")
	}
}
