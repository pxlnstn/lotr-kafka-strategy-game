// Package api exposes the HTTP/SSE surface over the engine.
package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"rotme/internal/config"
	"rotme/internal/engine"
	"rotme/internal/game"
)

type Server struct {
	e *engine.Engine
}

func New(e *engine.Engine) *Server { return &Server{e: e} }

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.health)
	mux.HandleFunc("/game/start", s.gameStart)
	mux.HandleFunc("/game/state", s.gameState)
	mux.HandleFunc("/order", s.order)
	mux.HandleFunc("/orders/available", s.ordersAvailable)
	mux.HandleFunc("/analysis/routes", s.analysisRoutes)
	mux.HandleFunc("/analysis/intercept", s.analysisIntercept)
	mux.HandleFunc("/events", s.events)
	mux.HandleFunc("/map", s.gameMap)
	mux.HandleFunc("/turn/advance", s.advance) // admin/demo convenience

	uiDir := os.Getenv("UI_DIR")
	if uiDir == "" {
		uiDir = "../ui"
	}
	mux.Handle("/", noCache(http.FileServer(http.Dir(uiDir))))
	return withCORS(mux)
}

// noCache tells browsers not to cache the UI files, so a reload always serves
// the latest version.
func noCache(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		h.ServeHTTP(w, r)
	})
}

func (s *Server) gameMap(w http.ResponseWriter, r *http.Request) {
	cfg := s.e.Config()
	writeJSON(w, http.StatusOK, map[string]any{"regions": cfg.Regions, "paths": cfg.Paths, "units": cfg.Units})
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

func (s *Server) gameStart(w http.ResponseWriter, r *http.Request) {
	if err := s.e.SubmitControl(r.Context(), "GAME_START"); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "starting"})
}

func (s *Server) order(w http.ResponseWriter, r *http.Request) {
	var o game.Order
	if err := json.NewDecoder(r.Body).Decode(&o); err != nil {
		http.Error(w, "bad order json", http.StatusBadRequest)
		return
	}
	if err := s.e.SubmitRaw(r.Context(), o); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "accepted"})
}

func (s *Server) gameState(w http.ResponseWriter, r *http.Request) {
	// Served from the hub's Kafka-rebuilt cache, so any instance can answer.
	// The hub already blanks RingBearerRegion for the Dark Side.
	writeJSON(w, http.StatusOK, s.e.Hub().StateView(playerID(r)))
}

func (s *Server) ordersAvailable(w http.ResponseWriter, r *http.Request) {
	unitID := r.URL.Query().Get("unitId")
	pid := playerID(r)
	writeJSON(w, http.StatusOK, availableOrders(s.e.Hub().StateView(pid), s.e.Config(), unitID, pid))
}

func playerID(r *http.Request) string {
	if r.URL.Query().Get("playerId") == game.PlayerDark {
		return game.PlayerDark
	}
	return game.PlayerLight
}

func (s *Server) analysisRoutes(w http.ResponseWriter, r *http.Request) {
	// Light Side only.
	if r.URL.Query().Get("playerId") == game.PlayerDark {
		http.Error(w, "route analysis is light-side only", http.StatusForbidden)
		return
	}
	writeJSON(w, http.StatusOK, s.e.Hub().Analyze("routes"))
}

func (s *Server) analysisIntercept(w http.ResponseWriter, r *http.Request) {
	// Dark Side only.
	if r.URL.Query().Get("playerId") != game.PlayerDark {
		http.Error(w, "intercept analysis is dark-side only", http.StatusForbidden)
		return
	}
	writeJSON(w, http.StatusOK, s.e.Hub().Analyze("intercept"))
}

func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	side := game.PlayerLight
	if r.URL.Query().Get("playerId") == game.PlayerDark {
		side = game.PlayerDark
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	client := s.e.Hub().Subscribe(side)
	defer s.e.Hub().Unsubscribe(client)

	fmt.Fprintf(w, "retry: 3000\n\n")
	flusher.Flush()

	for {
		select {
		case msg, ok := <-client.Ch:
			if !ok {
				return
			}
			fmt.Fprintf(w, "data: %s\n\n", msg)
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

func (s *Server) advance(w http.ResponseWriter, r *http.Request) {
	if err := s.e.SubmitControl(r.Context(), "TURN_ADVANCE"); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "advancing"})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func withCORS(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		h.ServeHTTP(w, r)
	})
}

// OrderOption describes a legal order the UI can offer for a unit.
type OrderOption struct {
	OrderType string   `json:"orderType"`
	Targets   []string `json:"targets,omitempty"` // path ids or region ids, depending on type
}

func availableOrders(st engine.APIState, cfg *config.Config, unitID, playerID string) []OrderOption {
	u, ok := cfg.UnitByID[unitID]
	if !ok || u.Side != playerSide(playerID) {
		return []OrderOption{}
	}
	region := unitRegion(st, cfg, unitID)
	if region == "" {
		return []OrderOption{}
	}

	var adjPaths, adjRegions, enemyRegions []string
	mySide := u.Side
	for _, p := range cfg.Paths {
		if p.From == region || p.To == region {
			adjPaths = append(adjPaths, p.ID)
			other := p.To
			if other == region {
				other = p.From
			}
			adjRegions = append(adjRegions, other)
			if controllerOf(st, other) != mySide && controllerOf(st, other) != game.SideNeutral {
				enemyRegions = append(enemyRegions, other)
			}
		}
	}

	opts := []OrderOption{{OrderType: game.OrderAssignRoute, Targets: adjPaths}, {OrderType: game.OrderRedirect, Targets: adjPaths}}

	switch {
	case u.Class == "RingBearer":
		if region == mountDoomID(cfg) {
			opts = append(opts, OrderOption{OrderType: game.OrderDestroyRing})
		}
	case u.Maia:
		opts = append(opts, OrderOption{OrderType: game.OrderMaiaAbility, Targets: adjPaths})
	default:
		opts = append(opts,
			OrderOption{OrderType: game.OrderBlockPath, Targets: adjPaths},
			OrderOption{OrderType: game.OrderAttackRegion, Targets: enemyRegions},
			OrderOption{OrderType: game.OrderReinforce, Targets: adjRegions},
		)
		if mySide == game.SideShadow {
			opts = append(opts,
				OrderOption{OrderType: game.OrderSearchPath, Targets: adjPaths},
				OrderOption{OrderType: game.OrderDeployNazgul, Targets: adjRegions},
			)
		}
		if u.CanFortify {
			opts = append(opts, OrderOption{OrderType: game.OrderFortify})
		}
	}
	return opts
}

func playerSide(playerID string) string {
	if playerID == game.PlayerDark {
		return game.SideShadow
	}
	return game.SideFree
}

func unitRegion(st engine.APIState, cfg *config.Config, unitID string) string {
	if cfg.UnitByID[unitID].Class == "RingBearer" {
		return st.RingBearerRegion
	}
	for _, u := range st.Units {
		if u.UnitID == unitID {
			return u.Region
		}
	}
	return ""
}

func controllerOf(st engine.APIState, regionID string) string {
	for _, r := range st.Regions {
		if r.RegionID == regionID {
			return r.ControlledBy
		}
	}
	return game.SideNeutral
}

func mountDoomID(cfg *config.Config) string {
	for _, r := range cfg.Regions {
		if r.SpecialRole == "RING_DESTRUCTION_SITE" {
			return r.ID
		}
	}
	return ""
}
