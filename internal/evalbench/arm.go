package evalbench

import (
	"context"
	"errors"
)

// ErrSearchFailed is returned when an arm's search function fails.
// The runner records this as a zero-hit rather than aborting the entire run.
var ErrSearchFailed = errors.New("search failed")

// Arm is one retrieval strategy under test. Every technique from Phase 2
// onward — context-augmentation, quantization, hybrid, graph-augmented —
// implements this, so Runner never branches on technique type.
type Arm struct {
	Name   string
	Search func(ctx context.Context, query string, k int) ([]Hit, error)
}

type Hit struct {
	PostID int64
	Score  float32
}
