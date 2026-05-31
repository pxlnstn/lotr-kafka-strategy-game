package game

import (
	"testing"

	"rotme/internal/config"
)

func newTestWorld(t *testing.T) *World {
	t.Helper()
	dir, err := config.FindConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	w := NewWorld(cfg)
	w.Turn = 4 // past the hidden start
	// Park Sauron away from Mordor so the amplifier is off unless a test wants it.
	w.Units["sauron"].Region = "dead-marshes"
	// Keep the other hunters far from the test area.
	w.Units["witch-king"].Region = "minas-morgul"
	w.Units["nazgul-3"].Region = "minas-morgul"
	w.Ring.TrueRegion = "the-shire"
	return w
}

// the-shire -> bree is 1 hop; the-shire -> weathertop is 2 hops.

func TestDetection_Range1At1Hop(t *testing.T) {
	w := newTestWorld(t)
	w.Units["nazgul-2"].Region = "bree" // range 1, 1 hop away
	if RunDetection(w); !w.Ring.Exposed {
		t.Fatal("range 1 at 1 hop should expose")
	}
}

func TestDetection_Range1At2Hops(t *testing.T) {
	w := newTestWorld(t)
	w.Units["nazgul-2"].Region = "weathertop" // range 1, 2 hops away
	if RunDetection(w); w.Ring.Exposed {
		t.Fatal("range 1 at 2 hops should not expose")
	}
}

func TestDetection_Range2At2Hops(t *testing.T) {
	w := newTestWorld(t)
	w.Units["witch-king"].Region = "weathertop" // range 2, 2 hops away
	if RunDetection(w); !w.Ring.Exposed {
		t.Fatal("range 2 at 2 hops should expose")
	}
}

func TestDetection_SauronAmplifier(t *testing.T) {
	w := newTestWorld(t)
	w.Units["sauron"].Region = "mordor"       // amplifier on
	w.Units["nazgul-2"].Region = "weathertop" // range 1 +1 = 2, 2 hops away
	if RunDetection(w); !w.Ring.Exposed {
		t.Fatal("sauron amplifier should push range 1 to 2 and expose")
	}
}

func TestDetection_SuppressedDuringHiddenStart(t *testing.T) {
	w := newTestWorld(t)
	w.Turn = 3 // hidden-until-turn
	w.Units["nazgul-2"].Region = "bree"
	if RunDetection(w); w.Ring.Exposed {
		t.Fatal("detection must be suppressed on turns <= 3")
	}
}
