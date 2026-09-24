package evalbench

import (
	"math"
	"sort"
)

// RecallAtK computes recall@k for a set of hits against relevance judgments.
func RecallAtK(hits []Hit, relevant map[int64]int, k int) float64 {
	if len(relevant) == 0 {
		return 0
	}
	found := 0
	for i, h := range hits {
		if i >= k {
			break
		}
		if _, ok := relevant[h.PostID]; ok {
			found++
		}
	}
	return float64(found) / float64(len(relevant))
}

// NDCGAtK computes normalized discounted cumulative gain at k.
func NDCGAtK(hits []Hit, relevant map[int64]int, k int) float64 {
	dcg := 0.0
	for i, h := range hits {
		if i >= k {
			break
		}
		grade, ok := relevant[h.PostID]
		if !ok {
			continue
		}
		dcg += float64(grade) / math.Log2(float64(i)+2)
	}
	// ideal DCG: sort relevant grades descending, take top k
	grades := make([]int, 0, len(relevant))
	for _, g := range relevant {
		grades = append(grades, g)
	}
	sortDesc(grades)
	idcg := 0.0
	for i, g := range grades {
		if i >= k {
			break
		}
		idcg += float64(g) / math.Log2(float64(i)+2)
	}
	if idcg == 0 {
		return 0
	}
	return dcg / idcg
}

func sortDesc(xs []int) {
	for i := 1; i < len(xs); i++ {
		for j := i; j > 0 && xs[j-1] < xs[j]; j-- {
			xs[j-1], xs[j] = xs[j], xs[j-1]
		}
	}
}

// Percentile computes the p-th percentile of a slice of latencies.
func Percentile(latenciesMS []float64, p float64) float64 {
	if len(latenciesMS) == 0 {
		return 0
	}
	sorted := append([]float64(nil), latenciesMS...)
	sort.Float64s(sorted)
	idx := int(p / 100 * float64(len(sorted)-1))
	return sorted[idx]
}
