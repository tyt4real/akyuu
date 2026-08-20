package embedder

import (
	"errors"
	"fmt"
	"math"
)

var errEmptySequence = errors.New("embedder: sequence has no attended tokens")

// meanPool collapses a [batch, seq, hidden] tensor into [batch, hidden] by
// averaging the hidden states of tokens whose attention mask is set.
func meanPool(hiddenStates []float32, shape []int64, masks [][]int64) ([][]float32, error) {
	if len(shape) != 3 {
		return nil, fmt.Errorf("embedder: unexpected tensor shape %v", shape)
	}
	batch, seq, hid := int(shape[0]), int(shape[1]), int(shape[2])
	out := make([][]float32, batch)
	for b := 0; b < batch; b++ {
		var count float32
		acc := make([]float32, hid)
		for s := 0; s < seq; s++ {
			if masks[b][s] == 0 {
				continue
			}
			count++
			base := (b*seq + s) * hid
			for h := 0; h < hid; h++ {
				acc[h] += hiddenStates[base+h]
			}
		}
		if count == 0 {
			return nil, errEmptySequence
		}
		for h := 0; h < hid; h++ {
			acc[h] /= count
		}
		out[b] = normalize(acc)
	}
	return out, nil
}

// normalize L2-normalizes a vector in place and returns it.
func normalize(v []float32) []float32 {
	var sum float64
	for _, x := range v {
		sum += float64(x) * float64(x)
	}
	norm := math.Sqrt(sum)
	if norm == 0 {
		return v
	}
	for i := range v {
		v[i] = float32(float64(v[i]) / norm)
	}
	return v
}

// matrixToVector interprets a [batch, hidden] output directly.
func matrixToVector(data []float32, shape []int64) ([][]float32, error) {
	if len(shape) != 2 {
		return nil, fmt.Errorf("embedder: unexpected tensor shape %v", shape)
	}
	batch, hidden := int(shape[0]), int(shape[1])
	out := make([][]float32, batch)
	for b := 0; b < batch; b++ {
		row := make([]float32, hidden)
		copy(row, data[b*hidden:(b+1)*hidden])
		out[b] = normalize(row)
	}
	return out, nil
}
