// Package router is the one place information asymmetry is enforced. Every
// event and snapshot bound for a browser passes through here; the ring
// bearer's true position is removed before anything reaches the Dark Side.
package router

import (
	"sync"

	"rotme/internal/game"
)

// Snapshot is the world view sent to a browser. For the Dark Side,
// RingBearerRegion is blanked before delivery.
type Snapshot struct {
	Turn             int                         `json:"turn"`
	RingBearerRegion string                      `json:"ringBearerRegion"`
	Units            map[string]UnitView         `json:"units"`
	Regions          map[string]game.RegionState `json:"regions"`
	Paths            map[string]game.PathState   `json:"paths"`
}

type UnitView struct {
	ID       string `json:"id"`
	Region   string `json:"region"`
	Strength int    `json:"strength"`
	Status   string `json:"status"`
}

// Outbound is what a side's SSE channel carries: either a single event or a
// full snapshot.
type Outbound struct {
	Event    *game.Event
	Snapshot *Snapshot
}

type Router struct {
	Light chan Outbound
	Dark  chan Outbound
}

func New(buffer int) *Router {
	return &Router{
		Light: make(chan Outbound, buffer),
		Dark:  make(chan Outbound, buffer),
	}
}

// RouteEvent fans an event out by audience. LIGHT-only events (ring position)
// never touch the Dark channel; DARK-only events (detection) never touch the
// Light channel.
func (r *Router) RouteEvent(ev game.Event) {
	out := Outbound{Event: &ev}
	switch ev.Audience {
	case game.AudLight:
		r.Light <- out
	case game.AudDark:
		r.Dark <- out
	default:
		r.Light <- out
		r.Dark <- out
	}
}

// Broadcast sends a snapshot to both sides, stripping the ring bearer region
// from the Dark Side copy.
func (r *Router) Broadcast(s Snapshot) {
	light := s
	dark := stripForDark(s)
	r.Light <- Outbound{Snapshot: &light}
	r.Dark <- Outbound{Snapshot: &dark}
}

func stripForDark(s Snapshot) Snapshot {
	s.RingBearerRegion = ""
	return s
}

// Cache holds the per-side views. DarkView.RingBearerRegion is never set, by
// construction: Update writes the real region only to the Light view.
type Cache struct {
	mu    sync.RWMutex
	Turn  int
	Light LightView
	Dark  DarkView
}

type LightView struct {
	RingBearerRegion string
	AssignedRoute    []string
	RouteIdx         int
}

type DarkView struct {
	RingBearerRegion   string // always ""
	LastDetectedRegion string
	LastDetectedTurn   int
}

func (c *Cache) Update(turn int, ringRegion string, route []string, routeIdx int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Turn = turn
	c.Light.RingBearerRegion = ringRegion
	c.Light.AssignedRoute = route
	c.Light.RouteIdx = routeIdx
	// Dark.RingBearerRegion is deliberately not touched here. It stays "".
}

// RecordDetection updates only what the Dark Side is allowed to know: that a
// detection happened, never the live position.
func (c *Cache) RecordDetection(turn int, region string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Dark.LastDetectedRegion = region
	c.Dark.LastDetectedTurn = turn
}

func (c *Cache) DarkRingRegion() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.Dark.RingBearerRegion
}
