package embedder

import (
	"context"
	"fmt"
	"time"

	"akyuu/internal/config"

	modal "github.com/modal-labs/modal-client/go"
)

type ModalEmbedder struct {
	client       *modal.Client
	fn           *modal.Function
	modelVersion string
	dimensions   int
	logger       Logger
}

type Logger interface {
	Info(msg string, args ...any)
	Error(msg string, args ...any)
	Debug(msg string, args ...any)
	Warn(msg string, args ...any)
}

func NewModalEmbedder(cfg config.ModalConfig, logger Logger) (*ModalEmbedder, error) {
	mc, err := modal.NewClient()
	if err != nil {
		return nil, fmt.Errorf("create modal client: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	fn, err := mc.Functions.FromName(ctx, cfg.AppName, cfg.EmbedFunction, nil)
	if err != nil {
		return nil, fmt.Errorf("get modal function %q: %w", cfg.EmbedFunction, err)
	}

	return &ModalEmbedder{
		client:       mc,
		fn:           fn,
		modelVersion: "all-MiniLM-L6-v2",
		dimensions:   384,
		logger:       logger,
	}, nil
}

func (m *ModalEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	batches := [][]string{texts}
	result, err := m.fn.Remote(ctx, []any{batches}, nil)
	if err != nil {
		return nil, fmt.Errorf("modal embed_batch remote call: %w", err)
	}

	modalResult, ok := result.([]any)
	if !ok {
		return nil, fmt.Errorf("unexpected modal result type: %T", result)
	}

	if len(modalResult) == 0 {
		return nil, fmt.Errorf("modal returned empty result")
	}

	firstBatch, ok := modalResult[0].([]any)
	if !ok {
		return nil, fmt.Errorf("unexpected batch result type: %T", modalResult[0])
	}

	embeddings := make([][]float32, len(firstBatch))
	for i, vec := range firstBatch {
		floatSlice, ok := vec.([]any)
		if !ok {
			return nil, fmt.Errorf("unexpected vector type: %T", vec)
		}
		embeddings[i] = make([]float32, len(floatSlice))
		for j, v := range floatSlice {
			f, ok := v.(float64)
			if !ok {
				return nil, fmt.Errorf("unexpected vector element type: %T", v)
			}
			embeddings[i][j] = float32(f)
		}
	}

	return embeddings, nil
}

func (m *ModalEmbedder) Dimensions() int { return m.dimensions }

func (m *ModalEmbedder) ModelVersion() string { return m.modelVersion }

func (m *ModalEmbedder) Close() error {
	if m.client != nil {
		m.client.Close()
	}
	return nil
}
