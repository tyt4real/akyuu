package embedder

import (
	"context"
)

// GraphStore is the slice of the store PPR needs, kept separate from the
// full Store type so this is testable against a fake.
type GraphStore interface {
	Neighbors(ctx context.Context, postID int64) ([]int64, error)
}

// Seed is one starting point for personalized PageRank: a post ID and its
// initial weight — typically its dense-retrieval similarity score.
type Seed struct {
	PostID int64
	Weight float32
}

// PPRConfig controls the bounded-subgraph power iteration. MaxHops keeps
// this tractable on a large archive: PPR runs only over the subgraph
// reachable from the seed set within MaxHops, not the whole graph.
type PPRConfig struct {
	MaxHops       int
	Alpha         float32 // probability of following an edge vs. restarting to a seed
	MaxIterations int
	Tolerance     float32 // L1 convergence threshold to stop early
}

func DefaultPPRConfig() PPRConfig {
	return PPRConfig{MaxHops: 2, Alpha: 0.6, MaxIterations: 30, Tolerance: 1e-4}
}

// PersonalizedPageRank computes PPR scores over the bounded subgraph
// around seeds. Higher Alpha weights graph structure more heavily;
// higher (1-Alpha) stays closer to the original dense-retrieval ranking.
func PersonalizedPageRank(ctx context.Context, gs GraphStore, seeds []Seed, cfg PPRConfig) (map[int64]float32, error) {
	if len(seeds) == 0 {
		return nil, nil
	}

	// 1. BFS-expand the subgraph from all seeds, up to cfg.MaxHops.
	adjacency := make(map[int64][]int64)
	visited := make(map[int64]bool)
	frontier := make([]int64, 0, len(seeds))
	for _, s := range seeds {
		frontier = append(frontier, s.PostID)
		visited[s.PostID] = true
	}
	for hop := 0; hop < cfg.MaxHops; hop++ {
		var next []int64
		for _, node := range frontier {
			neighbors, err := gs.Neighbors(ctx, node)
			if err != nil {
				return nil, err
			}
			adjacency[node] = neighbors
			for _, n := range neighbors {
				if !visited[n] {
					visited[n] = true
					next = append(next, n)
				}
			}
		}
		frontier = next
		if len(frontier) == 0 {
			break
		}
	}
	for node := range visited {
		if _, ok := adjacency[node]; !ok {
			neighbors, err := gs.Neighbors(ctx, node)
			if err != nil {
				return nil, err
			}
			adjacency[node] = neighbors
		}
	}

	// 2. Weight-normalized restart (personalization) vector.
	var totalWeight float32
	for _, s := range seeds {
		totalWeight += s.Weight
	}
	if totalWeight == 0 {
		return nil, nil
	}
	restart := make(map[int64]float32, len(seeds))
	for _, s := range seeds {
		restart[s.PostID] += s.Weight / totalWeight
	}

	// 3. Power iteration:
	//    pi_{t+1}(v) = alpha * sum_{u->v} pi_t(u)/deg(u) + (1-alpha) * restart(v)
	nodes := make([]int64, 0, len(visited))
	for n := range visited {
		nodes = append(nodes, n)
	}
	pi := make(map[int64]float32, len(nodes))
	for _, n := range nodes {
		pi[n] = restart[n]
	}
	for iter := 0; iter < cfg.MaxIterations; iter++ {
		next := make(map[int64]float32, len(nodes))
		for _, n := range nodes {
			next[n] = (1 - cfg.Alpha) * restart[n]
		}
		for u, neighbors := range adjacency {
			deg := len(neighbors)
			if deg == 0 {
				continue
			}
			share := cfg.Alpha * pi[u] / float32(deg)
			for _, v := range neighbors {
				next[v] += share
			}
		}
		var diff float32
		for _, n := range nodes {
			d := next[n] - pi[n]
			if d < 0 {
				d = -d
			}
			diff += d
		}
		pi = next
		if diff < cfg.Tolerance {
			break
		}
	}
	return pi, nil
}
