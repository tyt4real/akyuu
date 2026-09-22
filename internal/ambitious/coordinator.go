package ambitious

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

type Coordinator struct {
	store        StoreAPI
	logger       *slog.Logger
	stylometric  *StylometricWorker
	anomaly      *AnomalyWorker
	pollInterval time.Duration
}

type CoordinatorConfig struct {
	PollInterval       time.Duration
	StylometricEnabled bool
	AnomalyEnabled     bool

	StylometricMinPosts     int
	StylometricNGramMin     int
	StylometricNGramMax     int
	StylometricTopFeatures  int
	StylometricSimThreshold float64

	AnomalyVolumeZThreshold float64
	AnomalyBurstThreshold   float64
	AnomalyBurstWindowMin   int
	AnomalyLookbackHours    int
}

func NewCoordinator(st StoreAPI, cfg CoordinatorConfig, logger *slog.Logger) (*Coordinator, error) {
	var stylometric *StylometricWorker
	var anomaly *AnomalyWorker

	if cfg.StylometricEnabled {
		stylometric = NewStylometricWorker(st, logger,
			cfg.StylometricMinPosts,
			cfg.StylometricNGramMin,
			cfg.StylometricNGramMax,
			cfg.StylometricTopFeatures,
			cfg.StylometricSimThreshold)
	}

	if cfg.AnomalyEnabled {
		anomaly = NewAnomalyWorker(st, logger,
			cfg.AnomalyVolumeZThreshold,
			cfg.AnomalyBurstThreshold,
			cfg.AnomalyBurstWindowMin,
			cfg.AnomalyLookbackHours)
	}

	if stylometric == nil && anomaly == nil {
		return nil, fmt.Errorf("no ambitious workers enabled")
	}

	pollInterval := cfg.PollInterval
	if pollInterval == 0 {
		pollInterval = 15 * time.Minute
	}

	return &Coordinator{
		store:        st,
		logger:       logger,
		stylometric:  stylometric,
		anomaly:      anomaly,
		pollInterval: pollInterval,
	}, nil
}

func (c *Coordinator) Run(ctx context.Context) {
	c.logger.Info("ambitious coordinator starting")
	defer c.logger.Info("ambitious coordinator stopped")

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

	if c.stylometric != nil {
		if err := c.stylometric.RunOnce(ctx); err != nil {
			c.logger.Error("stylometric run failed", "err", err)
		}
	}

	if c.anomaly != nil {
		if err := c.anomaly.RunOnce(ctx); err != nil {
			c.logger.Error("anomaly run failed", "err", err)
		}
	}

	c.logger.Debug("ambitious cycle complete", "duration", time.Since(start))
}
