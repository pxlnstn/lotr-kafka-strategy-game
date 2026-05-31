package game

// Path transitions follow the state machine. Blocking records the
// unit holding the block; turn processing clears it when that unit leaves an
// endpoint.

func (p *PathState) Block(byUnit string) {
	if p.Status == Open || p.Status == Threatened {
		p.Status = Blocked
		p.BlockedBy = byUnit
	}
}

func (p *PathState) Threat() {
	if p.Status == Open {
		p.Status = Threatened
	}
}

func (p *PathState) Clear() {
	if p.Status == Threatened || p.Status == Blocked {
		p.Status = Open
		p.BlockedBy = ""
	}
}

// OpenTemporarily is Gandalf's effect: a blocked path opens for 2 turns.
func (p *PathState) OpenTemporarily() bool {
	if p.Status == Blocked {
		p.Status = TempOpen
		p.TempOpenTurns = 2
		return true
	}
	return false
}

func (p *PathState) Search() {
	if p.SurveillanceLevel < 3 {
		p.SurveillanceLevel++
	}
}

// Corrupt is Saruman's effect: surveillance pinned to 3, permanently.
func (p *PathState) Corrupt() {
	p.SurveillanceLevel = 3
}

// tickTempOpen runs at step 9. When the timer expires the path goes back to
// BLOCKED if a blocker is still present, otherwise OPEN.
func (p *PathState) tickTempOpen() {
	if p.Status != TempOpen {
		return
	}
	p.TempOpenTurns--
	if p.TempOpenTurns <= 0 {
		if p.BlockedBy != "" {
			p.Status = Blocked
		} else {
			p.Status = Open
		}
		p.TempOpenTurns = 0
	}
}

func (p *PathState) passable() bool {
	return p.Status == Open || p.Status == Threatened || p.Status == TempOpen
}
