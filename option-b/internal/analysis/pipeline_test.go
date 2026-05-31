package analysis

import (
	"testing"

	"rotme/internal/config"
	"rotme/internal/game"
)

func world(t *testing.T) *game.World {
	t.Helper()
	dir, err := config.FindConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	return game.NewWorld(cfg)
}

func TestRouteRisk_KnownThreatAndSurveillance(t *testing.T) {
	w := world(t)
	// Keep nazgul far so proximity is zero for this check.
	for id, u := range w.Units {
		if w.Cfg.UnitByID[id].Class == "Nazgul" {
			u.Region = "mordor"
		}
	}
	// Route the-shire -> bree -> weathertop.
	route := []string{"shire-to-bree", "bree-to-weathertop"}
	w.Paths["shire-to-bree"].SurveillanceLevel = 2
	w.Paths["bree-to-weathertop"].Status = game.Threatened

	rr := ScoreRoute(w, "test", "the-shire", route)

	// regions entered: bree(threat 1), weathertop(threat 2) = 3
	// surveillance: 2*3 = 6
	// threatened path: +2
	// nazgul proximity: 0
	want := 3 + 6 + 2
	if rr.Score != want {
		t.Fatalf("score = %d, want %d (%+v)", rr.Score, want, rr)
	}
}

func TestRouteRisk_NazgulProximityAdds(t *testing.T) {
	w := world(t)
	for id, u := range w.Units {
		if w.Cfg.UnitByID[id].Class == "Nazgul" {
			u.Region = "mordor"
		}
	}
	route := []string{"shire-to-bree"}
	base := ScoreRoute(w, "base", "the-shire", route).Score

	// Put one nazgul within 2 hops of bree (weathertop is 1 hop from bree).
	w.Units["nazgul-2"].Region = "weathertop"
	withNazgul := ScoreRoute(w, "near", "the-shire", route).Score

	if withNazgul-base != 2 {
		t.Fatalf("one nazgul within 2 hops should add 2, got delta %d", withNazgul-base)
	}
}

func TestInterceptScore_Window(t *testing.T) {
	if s := InterceptScore(2, 5, 10); s <= 0 {
		t.Fatalf("positive window should score > 0, got %v", s)
	}
	if s := InterceptScore(8, 3, 10); s != 0 {
		t.Fatalf("negative window should score 0, got %v", s)
	}
}
