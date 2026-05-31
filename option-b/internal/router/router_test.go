package router

import (
	"sync"
	"testing"

	"rotme/internal/game"
)

// Run with: go test -race ./internal/router/

func TestSnapshotStrippedForDark(t *testing.T) {
	r := New(8)
	r.Broadcast(Snapshot{Turn: 5, RingBearerRegion: "weathertop"})

	light := <-r.Light
	dark := <-r.Dark
	if light.Snapshot.RingBearerRegion != "weathertop" {
		t.Fatalf("light should see real region, got %q", light.Snapshot.RingBearerRegion)
	}
	if dark.Snapshot.RingBearerRegion != "" {
		t.Fatalf("dark must see empty region, got %q", dark.Snapshot.RingBearerRegion)
	}
}

func TestRingBearerMovedNeverReachesDark(t *testing.T) {
	r := New(8)
	r.RouteEvent(game.Event{Type: game.EvRingBearerMoved, TrueRegion: "bree", Audience: game.AudLight})

	// Light gets it.
	select {
	case <-r.Light:
	default:
		t.Fatal("light should have received RingBearerMoved")
	}
	// Dark must have nothing.
	select {
	case msg := <-r.Dark:
		t.Fatalf("dark received a message it must not: %+v", msg)
	default:
	}
}

func TestDarkRingRegionAlwaysEmptyUnderConcurrency(t *testing.T) {
	c := &Cache{}
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(turn int) {
			defer wg.Done()
			c.Update(turn, "mount-doom", []string{"shire-to-bree"}, 1)
			c.RecordDetection(turn, "bree")
		}(i)
	}
	// Concurrent readers.
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if c.DarkRingRegion() != "" {
				t.Error("DarkView.RingBearerRegion must always be empty")
			}
		}()
	}
	wg.Wait()
	if c.DarkRingRegion() != "" {
		t.Fatal("DarkView.RingBearerRegion leaked the position")
	}
}
