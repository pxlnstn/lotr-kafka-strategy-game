// Package config loads the shared, code-free game configuration
// (config/units.conf and config/map.conf) into typed Go structs and builds
// the map graph used by movement, detection, and the analysis pipelines.
//
// Everything about a unit is config-driven. No unit id string literal appears
// in game logic - the code reads fields like cfg.Indestructible or cfg.Maia,
// never cfg.ID == "...".
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// UnitConfig holds the per-unit config. JSON keys match units.conf.
type UnitConfig struct {
	ID               string   `json:"id"`
	Name             string   `json:"name"`
	Class            string   `json:"class"`
	Side             string   `json:"side"`
	StartRegion      string   `json:"start"`
	Strength         int      `json:"strength"`
	Leadership       bool     `json:"leadership"`
	LeadershipBonus  int      `json:"leadershipBonus"`
	Indestructible   bool     `json:"indestructible"`
	DetectionRange   int      `json:"detectionRange"`
	Respawns         bool     `json:"respawns"`
	RespawnTurns     int      `json:"respawnTurns"`
	Maia             bool     `json:"maia"`
	MaiaAbilityPaths []string `json:"maiaAbilityPaths"`
	IgnoresFortress  bool     `json:"ignoresFortress"`
	CanFortify       bool     `json:"canFortify"`
	Cooldown         int      `json:"cooldown"`
}

// Region is a node on the map.
type Region struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Terrain      string `json:"terrain"`
	SpecialRole  string `json:"specialRole"`
	StartControl string `json:"startControl"`
	StartThreat  int    `json:"startThreat"`
}

// Path is a bidirectional edge on the map. cost = turns to traverse.
type Path struct {
	ID   string `json:"id"`
	From string `json:"from"`
	To   string `json:"to"`
	Cost int    `json:"cost"`
}

// gameSettings + units come from units.conf.
type unitsFile struct {
	HiddenUntilTurn     int          `json:"hiddenUntilTurn"`
	MaxTurns            int          `json:"maxTurns"`
	TurnDurationSeconds int          `json:"turnDurationSeconds"`
	Units               []UnitConfig `json:"units"`
}

// regions + paths come from map.conf.
type mapFile struct {
	Regions []Region `json:"regions"`
	Paths   []Path   `json:"paths"`
}

// Config is the fully-loaded, validated game configuration plus the derived
// lookup maps and graph. It is read-only after Load.
type Config struct {
	HiddenUntilTurn     int
	MaxTurns            int
	TurnDurationSeconds int

	Units   []UnitConfig
	Regions []Region
	Paths   []Path

	// Lookup maps keyed by id.
	UnitByID   map[string]UnitConfig
	RegionByID map[string]Region
	PathByID   map[string]Path

	Graph *Graph
}

// Load reads units.conf and map.conf from configDir, validates referential
// integrity, and builds the graph.
func Load(configDir string) (*Config, error) {
	var uf unitsFile
	if err := readJSON(filepath.Join(configDir, "units.conf"), &uf); err != nil {
		return nil, fmt.Errorf("loading units.conf: %w", err)
	}
	var mf mapFile
	if err := readJSON(filepath.Join(configDir, "map.conf"), &mf); err != nil {
		return nil, fmt.Errorf("loading map.conf: %w", err)
	}

	c := &Config{
		HiddenUntilTurn:     uf.HiddenUntilTurn,
		MaxTurns:            uf.MaxTurns,
		TurnDurationSeconds: uf.TurnDurationSeconds,
		Units:               uf.Units,
		Regions:             mf.Regions,
		Paths:               mf.Paths,
		UnitByID:            make(map[string]UnitConfig, len(uf.Units)),
		RegionByID:          make(map[string]Region, len(mf.Regions)),
		PathByID:            make(map[string]Path, len(mf.Paths)),
	}

	for _, r := range mf.Regions {
		if _, dup := c.RegionByID[r.ID]; dup {
			return nil, fmt.Errorf("duplicate region id %q", r.ID)
		}
		c.RegionByID[r.ID] = r
	}
	for _, p := range mf.Paths {
		if _, dup := c.PathByID[p.ID]; dup {
			return nil, fmt.Errorf("duplicate path id %q", p.ID)
		}
		if _, ok := c.RegionByID[p.From]; !ok {
			return nil, fmt.Errorf("path %q references unknown from-region %q", p.ID, p.From)
		}
		if _, ok := c.RegionByID[p.To]; !ok {
			return nil, fmt.Errorf("path %q references unknown to-region %q", p.ID, p.To)
		}
		c.PathByID[p.ID] = p
	}
	for _, u := range uf.Units {
		if _, dup := c.UnitByID[u.ID]; dup {
			return nil, fmt.Errorf("duplicate unit id %q", u.ID)
		}
		if _, ok := c.RegionByID[u.StartRegion]; !ok {
			return nil, fmt.Errorf("unit %q has unknown start region %q", u.ID, u.StartRegion)
		}
		for _, pid := range u.MaiaAbilityPaths {
			if _, ok := c.PathByID[pid]; !ok {
				return nil, fmt.Errorf("unit %q lists unknown maiaAbilityPath %q", u.ID, pid)
			}
		}
		c.UnitByID[u.ID] = u
	}

	c.Graph = BuildGraph(mf.Regions, mf.Paths)
	return c, nil
}

// Accessors used by order validation.

func (c *Config) UnitSide(id string) (string, bool) {
	u, ok := c.UnitByID[id]
	return u.Side, ok
}

func (c *Config) PathEndpoints(id string) (string, string, bool) {
	p, ok := c.PathByID[id]
	return p.From, p.To, ok
}

func (c *Config) Adjacent(a, b string) bool {
	return c.Graph.Adjacent(a, b)
}

func readJSON(path string, v any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}

// FindConfigDir walks up from the current working directory looking for a
// "config" folder containing units.conf. This lets both tests and binaries
// locate the shared config without an absolute path.
func FindConfigDir() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		candidate := filepath.Join(dir, "config")
		if _, err := os.Stat(filepath.Join(candidate, "units.conf")); err == nil {
			return candidate, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("could not locate config/units.conf walking up from working dir")
		}
		dir = parent
	}
}
