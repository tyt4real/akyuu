package lineage

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

type Coordinator struct {
	store        StoreAPI
	logger       *slog.Logger
	crosspost    *CrosspostWorker
	lineage      *LineageWorker
	continuity   *ContinuityWorker
	replyGraph   *ReplyGraphWorker
	pollInterval time.Duration
}

type CoordinatorConfig struct {
	PollInterval      time.Duration
	CrosspostEnabled  bool
	LineageEnabled    bool
	ContinuityEnabled bool
	ReplyGraphEnabled bool

	CrosspostPHASHThreshold int
	LineageAlgo             string
	LineageThreshold        int
	LineageBatchSize        int

	ContinuityTimeGapHours   float64
	ContinuityTitleThreshold float64
	ContinuityOpSimThreshold float64
}

func NewCoordinator(st StoreAPI, cfg CoordinatorConfig, logger *slog.Logger) (*Coordinator, error) {
	var crosspost *CrosspostWorker
	var lineage *LineageWorker
	var continuity *ContinuityWorker
	var replyGraph *ReplyGraphWorker

	if cfg.CrosspostEnabled {
		crosspost = NewCrosspostWorker(st, logger, cfg.CrosspostPHASHThreshold)
	}

	if cfg.LineageEnabled {
		lineage = NewLineageWorker(st, logger, cfg.LineageAlgo, cfg.LineageThreshold, cfg.LineageBatchSize)
	}

	if cfg.ContinuityEnabled {
		continuity = NewContinuityWorker(st, logger, cfg.ContinuityTimeGapHours, cfg.ContinuityTitleThreshold, cfg.ContinuityOpSimThreshold)
	}

	if cfg.ReplyGraphEnabled {
		replyGraph = NewReplyGraphWorker(st, logger)
	}

	if crosspost == nil && lineage == nil && continuity == nil && replyGraph == nil {
		return nil, fmt.Errorf("no lineage workers enabled")
	}

	pollInterval := cfg.PollInterval
	if pollInterval == 0 {
		pollInterval = 5 * time.Minute
	}

	return &Coordinator{
		store:        st,
		logger:       logger,
		crosspost:    crosspost,
		lineage:      lineage,
		continuity:   continuity,
		replyGraph:   replyGraph,
		pollInterval: pollInterval,
	}, nil
}

func (c *Coordinator) Run(ctx context.Context) {
	c.logger.Info("lineage coordinator starting")
	defer c.logger.Info("lineage coordinator stopped")

	// Run once immediately
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

	if c.crosspost != nil {
		if err := c.crosspost.RunOnce(ctx); err != nil {
			c.logger.Error("crosspost run failed", "err", err)
		}
	}

	if c.lineage != nil {
		if err := c.lineage.RunOnce(ctx); err != nil {
			c.logger.Error("lineage run failed", "err", err)
		}
	}

	if c.continuity != nil {
		if err := c.continuity.RunOnce(ctx); err != nil {
			c.logger.Error("continuity run failed", "err", err)
		}
	}

	if c.replyGraph != nil {
		if err := c.replyGraph.RunOnce(ctx); err != nil {
			c.logger.Error("replygraph run failed", "err", err)
		}
	}

	c.logger.Debug("lineage cycle complete", "duration", time.Since(start))
}
