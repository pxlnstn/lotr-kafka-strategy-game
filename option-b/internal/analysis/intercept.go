package analysis

import "rotme/internal/game"

type InterceptEntry struct {
	UnitID       string  `json:"unitId"`
	TargetRegion string  `json:"targetRegion"`
	Score        float64 `json:"score"`
}

type InterceptPlan struct {
	ByUnit []InterceptEntry `json:"byUnit"`
}

// InterceptScore is the interception formula for one (nazgul, route-region) pair.
func InterceptScore(turnsToIntercept, rbTurnsToReach, routeLength int) float64 {
	if routeLength <= 0 {
		return 0
	}
	if rbTurnsToReach-turnsToIntercept >= 0 {
		return 1.0 - float64(turnsToIntercept)/float64(routeLength)
	}
	return 0
}

// BuildInterceptPlan evaluates every Nazgul against every region on a route the
// ring bearer might take, keeping each Nazgul's best target. `start` is where
// the ring bearer currently is; pathIds is the candidate route.
func BuildInterceptPlan(w *game.World, start string, pathIds []string) InterceptPlan {
	regions := regionsAlong(w, start, pathIds)
	costs := cumulativeCosts(w, start, pathIds)
	routeLength := 0
	if len(costs) > 0 {
		routeLength = costs[len(costs)-1]
	}

	var plan InterceptPlan
	for id, u := range w.Units {
		if u.Status != game.Active || w.Cfg.UnitByID[id].Class != "Nazgul" {
			continue
		}
		best := InterceptEntry{UnitID: id, Score: -1}
		for i, rid := range regions {
			tti := w.Cfg.Graph.CostDistance(u.Region, rid)
			if tti < 0 {
				continue
			}
			s := InterceptScore(tti, costs[i], routeLength)
			if s > best.Score {
				best.Score = s
				best.TargetRegion = rid
			}
		}
		if best.Score < 0 {
			best.Score = 0
		}
		plan.ByUnit = append(plan.ByUnit, best)
	}
	return plan
}

// cumulativeCosts returns the running traversal cost (turns) to reach each
// region along the route.
func cumulativeCosts(w *game.World, start string, pathIds []string) []int {
	var costs []int
	cur, total := start, 0
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
			return costs
		}
		total += p.Cost
		costs = append(costs, total)
	}
	return costs
}
