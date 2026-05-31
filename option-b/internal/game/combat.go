package game

import "rotme/internal/config"

// Combatant is a unit taking part in a battle, with its config and current strength.
type Combatant struct {
	Cfg      config.UnitConfig
	Strength int
}

type CombatResult struct {
	AttackerWon   bool
	Damage        int
	AttackerPower int
	DefenderPower int
}

// ResolveCombat applies the formula. terrain is the defender region's
// terrain; fortified is whether that region has an active fortification.
func ResolveCombat(attackers, defenders []Combatant, terrain string, fortified bool) CombatResult {
	ap := sidePower(attackers)
	dp := sidePower(defenders)

	if !attackIgnoresFortress(attackers) {
		dp += terrainBonus(terrain)
	}
	if fortified {
		dp += 2
	}

	r := CombatResult{AttackerPower: ap, DefenderPower: dp}
	if ap > dp {
		r.AttackerWon = true
		r.Damage = ap - dp
	}
	return r
}

func sidePower(side []Combatant) int {
	total := 0
	for i := range side {
		total += effectiveStrength(side, i)
	}
	return total
}

// effectiveStrength adds the leadership bonus from every other leader on the
// same side. A leader does not buff itself.
func effectiveStrength(side []Combatant, i int) int {
	s := side[i].Strength
	for j := range side {
		if j == i {
			continue
		}
		if side[j].Cfg.Leadership {
			s += side[j].Cfg.LeadershipBonus
		}
	}
	return s
}

func terrainBonus(terrain string) int {
	switch terrain {
	case "FORTRESS":
		return 2
	case "MOUNTAINS":
		return 1
	default:
		return 0
	}
}

func attackIgnoresFortress(attackers []Combatant) bool {
	for _, a := range attackers {
		if a.Cfg.IgnoresFortress {
			return true
		}
	}
	return false
}

// ApplyDamage mutates the unit per the state machine.
func (u *UnitState) ApplyDamage(cfg config.UnitConfig, damage int) {
	raw := u.Strength - damage
	if cfg.Indestructible {
		u.Strength = max(1, raw)
		return
	}
	if raw <= 0 {
		u.Strength = 0
		u.Region = ""
		if cfg.Respawns {
			u.Status = Respawning
			u.RespawnTurns = cfg.RespawnTurns
		} else {
			u.Status = Destroyed
		}
		return
	}
	u.Strength = raw
}
