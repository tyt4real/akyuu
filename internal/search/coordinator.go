package search

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

type Coordinator struct {
	store        StoreAPI
	logger       *slog.Logger
	evaluator    *Evaluator
	trendAgg     *TrendAggregator
	pollInterval time.Duration
}

type CoordinatorConfig struct {
	PollInterval     time.Duration
	EvaluatorEnabled bool
	TrendsEnabled    bool
}

func NewCoordinator(st StoreAPI, emb Embedder, cfg CoordinatorConfig, logger *slog.Logger) (*Coordinator, error) {
	var evaluator *Evaluator
	var trendAgg *TrendAggregator

	if cfg.EvaluatorEnabled && emb != nil {
		evaluator = NewEvaluator(st, emb, logger)
	}

	if cfg.TrendsEnabled {
		trendAgg = NewTrendAggregator(st, logger)
	}

	if evaluator == nil && trendAgg == nil {
		return nil, fmt.Errorf("no search workers enabled")
	}

	pollInterval := cfg.PollInterval
	if pollInterval == 0 {
		pollInterval = 5 * time.Minute
	}

	return &Coordinator{
		store:        st,
		logger:       logger,
		evaluator:    evaluator,
		trendAgg:     trendAgg,
		pollInterval: pollInterval,
	}, nil
}

func (c *Coordinator) Run(ctx context.Context) {
	c.logger.Info("search coordinator starting")
	defer c.logger.Info("search coordinator stopped")

	c.runOnce(ctx)

	ticker := time.NewTicker(c.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.runOnce(ctx)
		}
	}
}

func (c *Coordinator) runOnce(ctx context.Context) {
	start := time.Now()

	if c.evaluator != nil {
		if err := c.evaluator.RunOnce(ctx); err != nil {
			c.logger.Error("evaluator run failed", "err", err)
		}
	}

	if c.trendAgg != nil {
		if err := c.trendAgg.RunOnce(ctx); err != nil {
			c.logger.Error("trend aggregator run failed", "err", err)
		}
	}

	c.logger.Debug("search cycle complete", "duration", time.Since(start))
}
