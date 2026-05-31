// Package analysis computes the two derived views the browsers request:
// route risk for the Light Side and interception plans for the Dark Side.
// Functions here are pure over a game.World snapshot so they parallelise
// cleanly in the worker pipelines and are easy to test without Kafka.
package analysis

import (
	"rotme/internal/game"
)

type RouteRisk struct {
	Name            string   `json:"name"`
	PathIds         []string `json:"pathIds"`
	Score           int      `json:"score"`
	BlockedPaths    []string `json:"blockedPaths"`
	ThreatenedPaths []string `json:"threatenedPaths"`
}

type RankedRoutes struct {
	Routes      []RouteRisk `json:"routes"`
	Recommended string      `json:"recommended"`
	Warnings    []string    `json:"warnings"`
}

// ScoreRoute applies the risk formula to one route starting at
// `start`.
func ScoreRoute(w *game.World, name, start string, pathIds []string) RouteRisk {
	regions := regionsAlong(w, start, pathIds)

	score := 0
	for _, rid := range regions {
		if r := w.Regions[rid]; r != nil {
			score += r.ThreatLevel
		}
	}

	var blocked, threatened []string
	for _, pid := range pathIds {
		p := w.Paths[pid]
		if p == nil {
			continue
		}
		score += p.SurveillanceLevel * 3
		switch p.Status {
		case game.Blocked:
			score += 5
			blocked = append(blocked, pid)
		case game.Threatened:
			score += 2
			threatened = append(threatened, pid)
		}
	}

	score += nazgulProximity(w, append([]string{start}, regions...)) * 2

	return RouteRisk{
		Name: name, PathIds: pathIds, Score: score,
		BlockedPaths: blocked, ThreatenedPaths: threatened,
	}
}

// RankRoutes scores several candidate routes and picks the lowest-risk one.
func RankRoutes(w *game.World, start string, candidates map[string][]string) RankedRoutes {
	var out RankedRoutes
	best := ""
	bestScore := 1 << 30
	for name, pathIds := range candidates {
		rr := ScoreRoute(w, name, start, pathIds)
		out.Routes = append(out.Routes, rr)
		if rr.Score < bestScore {
			bestScore = rr.Score
			best = name
		}
		if len(rr.BlockedPaths) > 0 {
			out.Warnings = append(out.Warnings, name+" has blocked paths")
		}
	}
	out.Recommended = best
	return out
}

func nazgulProximity(w *game.World, routeRegions []string) int {
	count := 0
	for id, u := range w.Units {
		if u.Status != game.Active || w.Cfg.UnitByID[id].Class != "Nazgul" {
			continue
		}
		near := false
		for _, rid := range routeRegions {
			if d := w.Cfg.Graph.HopDistance(u.Region, rid); d >= 0 && d <= 2 {
				near = true
				break
			}
		}
		if near {
			count++
		}
	}
	return count
}

func regionsAlong(w *game.World, start string, pathIds []string) []string {
	var regions []string
	cur := start
	for _, pid := range pathIds {
		p, ok := w.Cfg.PathByID[pid]
		if !ok {
			break
		}
		switch cur {
		case p.From:
			cur = p.To
		case p.To:
			cur = p.From
		default:
			return regions
		}
		regions = append(regions, cur)
	}
	return regions
}
