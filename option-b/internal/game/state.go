package game

import "rotme/internal/config"

type Status string

const (
	Active     Status = "ACTIVE"
	Destroyed  Status = "DESTROYED"
	Respawning Status = "RESPAWNING"
)

type PathStatus string

const (
	Open       PathStatus = "OPEN"
	Threatened PathStatus = "THREATENED"
	Blocked    PathStatus = "BLOCKED"
	TempOpen   PathStatus = "TEMPORARILY_OPEN"
)

const (
	SideFree    = "FREE_PEOPLES"
	SideShadow  = "SHADOW"
	SideNeutral = "NEUTRAL"
)

type UnitState struct {
	ID           string
	Region       string // always "" for the ring bearer in shared state
	Strength     int
	Status       Status
	RespawnTurns int
	Route        []string // path ids still to traverse
	RouteIdx     int
	Cooldown     int
}

type RegionState struct {
	ID           string
	ControlledBy string
	ThreatLevel  int
	Fortified    bool
	FortifyTurns int
	Units        []string // ids of non-hidden units currently here
}

type PathState struct {
	ID                string
	Status            PathStatus
	SurveillanceLevel int
	TempOpenTurns     int
	BlockedBy         string // unit holding the block, "" if none
}

type RingBearerState struct {
	TrueRegion         string
	Exposed            bool
	Route              []string
	RouteIdx           int
	LastDetectedTurn   int // -1 if never
	LastDetectedRegion string
}

type World struct {
	Cfg     *config.Config
	Turn    int
	Units   map[string]*UnitState
	Regions map[string]*RegionState
	Paths   map[string]*PathState
	Ring    *RingBearerState

	SarumanDisabled bool
	GameOver        bool
	Winner          string // "", FREE_PEOPLES, SHADOW, or DRAW

	// Identifiers derived from config traits at startup, so game logic never
	// names a unit or region by literal id.
	ringID        string
	mountDoomID   string
	sarumanHomeID string
}

// NewWorld builds the starting world from config. Turn starts at 1.
func NewWorld(cfg *config.Config) *World {
	w := &World{
		Cfg:     cfg,
		Turn:    1,
		Units:   make(map[string]*UnitState, len(cfg.Units)),
		Regions: make(map[string]*RegionState, len(cfg.Regions)),
		Paths:   make(map[string]*PathState, len(cfg.Paths)),
		Ring:    &RingBearerState{LastDetectedTurn: -1},
	}

	for _, r := range cfg.Regions {
		w.Regions[r.ID] = &RegionState{
			ID:           r.ID,
			ControlledBy: r.StartControl,
			ThreatLevel:  r.StartThreat,
		}
	}
	for _, p := range cfg.Paths {
		w.Paths[p.ID] = &PathState{ID: p.ID, Status: Open}
	}
	for _, u := range cfg.Units {
		us := &UnitState{
			ID:       u.ID,
			Region:   u.StartRegion,
			Strength: u.Strength,
			Status:   Active,
		}
		if isRingBearer(u) {
			w.Ring.TrueRegion = u.StartRegion
			us.Region = "" // hidden in shared state
		} else {
			reg := w.Regions[u.StartRegion]
			reg.Units = append(reg.Units, u.ID)
		}
		w.Units[u.ID] = us
	}

	w.deriveIDs()
	return w
}

// deriveIDs resolves the special ids we need by config trait. A SHADOW maia
// that lists ability paths is the path-corrupting maia (Saruman); its start
// region is the stronghold whose fall disables it.
func (w *World) deriveIDs() {
	for _, u := range w.Cfg.Units {
		switch {
		case u.Class == "RingBearer":
			w.ringID = u.ID
		case u.Side == SideShadow && u.Maia && len(u.MaiaAbilityPaths) > 0:
			w.sarumanHomeID = u.StartRegion
		}
	}
	for _, r := range w.Cfg.Regions {
		if r.SpecialRole == "RING_DESTRUCTION_SITE" {
			w.mountDoomID = r.ID
		}
	}
}

func (w *World) isRing(id string) bool { return id == w.ringID }

// isRingBearer is the only place we describe the ring bearer, and we do it by
// config trait (the unit that carries the ring is the one whose start region
// is the ring-bearer start). We never test against a literal id.
func isRingBearer(u config.UnitConfig) bool {
	return u.Class == "RingBearer"
}
