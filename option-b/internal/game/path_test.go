package game

import "testing"

func TestPath_OpenToThreatened(t *testing.T) {
	p := &PathState{Status: Open}
	p.Threat()
	if p.Status != Threatened {
		t.Fatal(p.Status)
	}
}

func TestPath_ThreatenedToBlocked(t *testing.T) {
	p := &PathState{Status: Threatened}
	p.Block("nazgul-2")
	if p.Status != Blocked || p.BlockedBy != "nazgul-2" {
		t.Fatal(p)
	}
}

func TestPath_GandalfOpensBlocked(t *testing.T) {
	p := &PathState{Status: Blocked, BlockedBy: "nazgul-2"}
	if !p.OpenTemporarily() {
		t.Fatal("should open")
	}
	if p.Status != TempOpen || p.TempOpenTurns != 2 {
		t.Fatal(p)
	}
}

func TestPath_TempOpenRevertsToBlockedWhenBlockerPresent(t *testing.T) {
	p := &PathState{Status: TempOpen, TempOpenTurns: 1, BlockedBy: "nazgul-2"}
	p.tickTempOpen()
	if p.Status != Blocked {
		t.Fatal(p.Status)
	}
}

func TestPath_TempOpenRevertsToOpenWhenNoBlocker(t *testing.T) {
	p := &PathState{Status: TempOpen, TempOpenTurns: 1}
	p.tickTempOpen()
	if p.Status != Open {
		t.Fatal(p.Status)
	}
}

func TestPath_BlockedToOpen(t *testing.T) {
	p := &PathState{Status: Blocked, BlockedBy: "nazgul-2"}
	p.Clear()
	if p.Status != Open || p.BlockedBy != "" {
		t.Fatal(p)
	}
}

func TestPath_SearchCapsAtThree(t *testing.T) {
	p := &PathState{Status: Open}
	for i := 0; i < 5; i++ {
		p.Search()
	}
	if p.SurveillanceLevel != 3 {
		t.Fatal(p.SurveillanceLevel)
	}
}

func TestPath_SarumanCorrupts(t *testing.T) {
	p := &PathState{Status: Open, SurveillanceLevel: 0}
	p.Corrupt()
	if p.SurveillanceLevel != 3 {
		t.Fatal(p.SurveillanceLevel)
	}
}
