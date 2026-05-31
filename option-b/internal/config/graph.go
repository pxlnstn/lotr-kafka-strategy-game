package config

import "container/heap"

// Edge is one traversable connection out of a region.
type Edge struct {
	To     string
	PathID string
	Cost   int
}

// Graph is an undirected adjacency list built from the path list. All paths
// are bidirectional, so each path contributes two edges.
type Graph struct {
	adj map[string][]Edge
}

// BuildGraph constructs the undirected graph from regions and paths.
func BuildGraph(regions []Region, paths []Path) *Graph {
	g := &Graph{adj: make(map[string][]Edge, len(regions))}
	for _, r := range regions {
		g.adj[r.ID] = nil // ensure isolated nodes still appear
	}
	for _, p := range paths {
		g.adj[p.From] = append(g.adj[p.From], Edge{To: p.To, PathID: p.ID, Cost: p.Cost})
		g.adj[p.To] = append(g.adj[p.To], Edge{To: p.From, PathID: p.ID, Cost: p.Cost})
	}
	return g
}

// Neighbors returns the edges leaving region r.
func (g *Graph) Neighbors(r string) []Edge { return g.adj[r] }

// EdgeBetween returns the path directly connecting a and b, if one exists.
func (g *Graph) EdgeBetween(a, b string) (Edge, bool) {
	for _, e := range g.adj[a] {
		if e.To == b {
			return e, true
		}
	}
	return Edge{}, false
}

// Adjacent reports whether a single path connects a and b.
func (g *Graph) Adjacent(a, b string) bool {
	_, ok := g.EdgeBetween(a, b)
	return ok
}

// HopDistance is the minimum number of edges between a and b (unweighted BFS).
// Returns 0 when a == b, and -1 when b is unreachable from a.
//
// Detection range is measured in hops (edge count), not traversal cost.
func (g *Graph) HopDistance(a, b string) int {
	if a == b {
		return 0
	}
	visited := map[string]bool{a: true}
	frontier := []string{a}
	dist := 0
	for len(frontier) > 0 {
		dist++
		var next []string
		for _, cur := range frontier {
			for _, e := range g.adj[cur] {
				if e.To == b {
					return dist
				}
				if !visited[e.To] {
					visited[e.To] = true
					next = append(next, e.To)
				}
			}
		}
		frontier = next
	}
	return -1
}

// CostDistance is the minimum total traversal cost (turns) between a and b,
// using Dijkstra over edge costs. Returns 0 when a == b and -1 if unreachable.
//
// This is "turns to reach" - used by the interception pipeline.
func (g *Graph) CostDistance(a, b string) int {
	if a == b {
		return 0
	}
	const inf = int(^uint(0) >> 1)
	dist := map[string]int{a: 0}
	pq := &minPQ{{node: a, cost: 0}}
	for pq.Len() > 0 {
		cur := heap.Pop(pq).(pqItem)
		if cur.node == b {
			return cur.cost
		}
		if d, ok := dist[cur.node]; ok && cur.cost > d {
			continue // stale entry
		}
		for _, e := range g.adj[cur.node] {
			nd := cur.cost + e.Cost
			if old, ok := dist[e.To]; !ok || nd < old {
				dist[e.To] = nd
				heap.Push(pq, pqItem{node: e.To, cost: nd})
			}
		}
	}
	if d, ok := dist[b]; ok {
		return d
	}
	_ = inf
	return -1
}

// --- tiny priority queue for Dijkstra ---

type pqItem struct {
	node string
	cost int
}

type minPQ []pqItem

func (p minPQ) Len() int           { return len(p) }
func (p minPQ) Less(i, j int) bool { return p[i].cost < p[j].cost }
func (p minPQ) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }
func (p *minPQ) Push(x any)        { *p = append(*p, x.(pqItem)) }
func (p *minPQ) Pop() any {
	old := *p
	n := len(old)
	it := old[n-1]
	*p = old[:n-1]
	return it
}
