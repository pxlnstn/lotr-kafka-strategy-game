package game

import (
	"testing"

	"rotme/internal/config"
)

func unit(strength int, opts ...func(*config.UnitConfig)) Combatant {
	c := config.UnitConfig{Strength: strength}
	for _, o := range opts {
		o(&c)
	}
	return Combatant{Cfg: c, Strength: strength}
}

func leader(bonus int) func(*config.UnitConfig) {
	return func(c *config.UnitConfig) { c.Leadership = true; c.LeadershipBonus = bonus }
}
func ignoresFortress() func(*config.UnitConfig) {
	return func(c *config.UnitConfig) { c.IgnoresFortress = true }
}
func indestructible() func(*config.UnitConfig) {
	return func(c *config.UnitConfig) { c.Indestructible = true }
}
func respawns(turns int) func(*config.UnitConfig) {
	return func(c *config.UnitConfig) { c.Respawns = true; c.RespawnTurns = turns }
}

func TestCombat_TieOnPlains(t *testing.T) {
	r := ResolveCombat([]Combatant{unit(5)}, []Combatant{unit(5)}, "PLAINS", false)
	if r.AttackerWon {
		t.Fatalf("5 vs 5 on plains should not win: %+v", r)
	}
}

func TestCombat_FortressDefends(t *testing.T) {
	r := ResolveCombat([]Combatant{unit(5)}, []Combatant{unit(5)}, "FORTRESS", false)
	if r.AttackerWon || r.DefenderPower != 7 {
		t.Fatalf("want defender power 7 and a hold, got %+v", r)
	}
}

func TestCombat_UrukIgnoresFortressTerrain(t *testing.T) {
	r := ResolveCombat([]Combatant{unit(5, ignoresFortress())}, []Combatant{unit(5)}, "FORTRESS", false)
	if r.AttackerWon || r.DefenderPower != 5 {
		t.Fatalf("uruk should face 5 (terrain skipped) -> tie, got %+v", r)
	}
}

func TestCombat_FortifiedStillStopsUruk(t *testing.T) {
	r := ResolveCombat([]Combatant{unit(5, ignoresFortress())}, []Combatant{unit(5)}, "FORTRESS", true)
	if r.AttackerWon || r.DefenderPower != 7 {
		t.Fatalf("fortification still applies (5+2), got %+v", r)
	}
}

func TestCombat_LeadershipBonus(t *testing.T) {
	att := []Combatant{unit(5, leader(1)), unit(3)}
	r := ResolveCombat(att, []Combatant{unit(5)}, "PLAINS", false)
	if !r.AttackerWon || r.AttackerPower != 9 {
		t.Fatalf("aragorn+gimli should be 9 and win, got %+v", r)
	}
}

func TestCombat_IndestructibleFloorsAtOne(t *testing.T) {
	u := &UnitState{Strength: 5, Status: Active}
	u.ApplyDamage(config.UnitConfig{Indestructible: true}, 100)
	if u.Strength != 1 || u.Status != Active {
		t.Fatalf("indestructible should floor at 1 and stay active, got str=%d status=%s", u.Strength, u.Status)
	}
}

func TestApplyDamage_RespawnAndDestroy(t *testing.T) {
	u := &UnitState{Strength: 3, Status: Active, Region: "minas-morgul"}
	u.ApplyDamage(config.UnitConfig{Respawns: true, RespawnTurns: 3}, 5)
	if u.Status != Respawning || u.RespawnTurns != 3 || u.Region != "" {
		t.Fatalf("respawning unit wrong: %+v", u)
	}

	v := &UnitState{Strength: 3, Status: Active, Region: "isengard"}
	v.ApplyDamage(config.UnitConfig{}, 5)
	if v.Status != Destroyed || v.Region != "" {
		t.Fatalf("non-respawning unit should be destroyed: %+v", v)
	}
}
