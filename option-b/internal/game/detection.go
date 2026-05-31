package game

import "rotme/internal/config"

type Detection struct {
	UnitID string
	Region string
	Turn   int
}

// RunDetection applies the formula at turn end. It sets the ring
// bearer's exposed flag and returns one Detection per hunter that found it.
// Suppressed while turn <= hidden-until-turn.
func RunDetection(w *World) []Detection {
	if w.Turn <= w.Cfg.HiddenUntilTurn {
		return nil
	}
	rb := w.Ring.TrueRegion
	if rb == "" {
		return nil
	}

	amp := 0
	if eyeActive(w) {
		amp = 1
	}

	var found []Detection
	for id, u := range w.Units {
		if u.Status != Active {
			continue
		}
		cfg := w.Cfg.UnitByID[id]
		if cfg.DetectionRange <= 0 { // only hunters detect; Nazgul are the units with range > 0
			continue
		}
		rng := cfg.DetectionRange + amp
		d := w.Cfg.Graph.HopDistance(u.Region, rb)
		if d >= 0 && d <= rng {
			w.Ring.Exposed = true
			w.Ring.LastDetectedTurn = w.Turn
			w.Ring.LastDetectedRegion = rb
			found = append(found, Detection{UnitID: id, Region: rb, Turn: w.Turn})
		}
	}
	return found
}

// eyeActive reports whether the Eye of Sauron amplifier is in effect: a SHADOW
// Maia that is indestructible (uniquely Sauron in config), still active, and
// standing in the volcanic shadow stronghold (Mordor). All identified by
// config traits, never by id.
func eyeActive(w *World) bool {
	for id, u := range w.Units {
		cfg := w.Cfg.UnitByID[id]
		if cfg.Side == SideShadow && cfg.Maia && cfg.Indestructible && u.Status == Active {
			if isMordor(w.Cfg.RegionByID[u.Region]) {
				return true
			}
		}
	}
	return false
}

func isMordor(r config.Region) bool {
	return r.SpecialRole == "SHADOW_STRONGHOLD" && r.Terrain == "VOLCANIC"
}
