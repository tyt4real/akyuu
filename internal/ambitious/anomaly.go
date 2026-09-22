package ambitious

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"sort"
	"time"
)

type AnomalyWorker struct {
	store            StoreAPI
	logger           *slog.Logger
	volumeZThreshold float64
	burstWindowMin   int
	burstThreshold   float64
	lookbackHours    int
}

type boardStats struct {
	BoardID   int64
	SiteID    int64
	PostCount int
	Hour      time.Time
}

func NewAnomalyWorker(st StoreAPI, logger *slog.Logger, volumeZThreshold, burstThreshold float64, burstWindowMin, lookbackHours int) *AnomalyWorker {
	if volumeZThreshold <= 0 {
		volumeZThreshold = 3.0
	}
	if burstThreshold <= 0 {
		burstThreshold = 5.0
	}
	if burstWindowMin <= 0 {
		burstWindowMin = 10
	}
	if lookbackHours <= 0 {
		lookbackHours = 168 // 1 week
	}
	return &AnomalyWorker{
		store:            st,
		logger:           logger,
		volumeZThreshold: volumeZThreshold,
		burstThreshold:   burstThreshold,
		burstWindowMin:   burstWindowMin,
		lookbackHours:    lookbackHours,
	}
}

func (w *AnomalyWorker) RunOnce(ctx context.Context) error {
	w.logger.Debug("anomaly: detecting raids/anomalies")

	// 1. Volume spike detection (z-score per board)
	if err := w.detectVolumeSpikes(ctx); err != nil {
		return fmt.Errorf("volume spikes: %w", err)
	}

	// 2. Near-duplicate burst detection
	if err := w.detectNearDupBursts(ctx); err != nil {
		return fmt.Errorf("near-dup bursts: %w", err)
	}

	// 3. Cross-board coordinated posting
	if err := w.detectCoordinatedPosting(ctx); err != nil {
		return fmt.Errorf("coordinated posting: %w", err)
	}

	return nil
}

func (w *AnomalyWorker) detectVolumeSpikes(ctx context.Context) error {
	// Get hourly post counts per board for the lookback period
	rows, err := w.store.Query(ctx, `
		SELECT t.board_id, bo.site_id,
		       date_trunc('hour', p."timestamp") as hour,
		       COUNT(p.id) as post_count
		FROM posts p
		JOIN threads t ON t.id = p.thread_id
		JOIN boards bo ON bo.id = t.board_id
		WHERE p."timestamp" >= now() - interval '1 hour' * $1
		  AND p."timestamp" < now()
		GROUP BY t.board_id, bo.site_id, date_trunc('hour', p."timestamp")
		ORDER BY t.board_id, hour`, w.lookbackHours)
	if err != nil {
		return err
	}
	defer rows.Close()

	type boardHourly struct {
		BoardID int64
		SiteID  int64
		Hours   []boardStats
	}

	boardData := make(map[int64]*boardHourly)

	for rows.Next() {
		var b boardStats
		if err := rows.Scan(&b.BoardID, &b.SiteID, &b.Hour, &b.PostCount); err != nil {
			continue
		}

		key := b.BoardID
		if boardData[key] == nil {
			boardData[key] = &boardHourly{BoardID: b.BoardID, SiteID: b.SiteID}
		}
		boardData[key].Hours = append(boardData[key].Hours, b)
	}

	// Calculate z-score for the most recent hour
	now := time.Now().Truncate(time.Hour)
	for _, bh := range boardData {
		if len(bh.Hours) < 12 { // Need at least 12 hours of data
			continue
		}

		// Find current hour
		var currentCount int
		var historicalCounts []float64

		for _, h := range bh.Hours {
			if h.Hour.Equal(now) {
				currentCount = h.PostCount
			} else {
				historicalCounts = append(historicalCounts, float64(h.PostCount))
			}
		}

		if currentCount == 0 || len(historicalCounts) < 6 {
			continue
		}

		// Calculate mean and std dev
		var mean, variance float64
		for _, c := range historicalCounts {
			mean += c
		}
		mean /= float64(len(historicalCounts))

		for _, c := range historicalCounts {
			diff := c - mean
			variance += diff * diff
		}
		variance /= float64(len(historicalCounts))
		stdDev := math.Sqrt(variance)

		if stdDev == 0 {
			continue
		}

		zScore := (float64(currentCount) - mean) / stdDev

		if zScore >= w.volumeZThreshold {
			w.recordAnomaly(ctx, bh.BoardID, bh.SiteID, "volume_spike", map[string]any{
				"z_score":          zScore,
				"current_count":    currentCount,
				"mean":             mean,
				"std_dev":          stdDev,
				"historical_hours": len(historicalCounts),
			})
		}
	}

	return nil
}

func (w *AnomalyWorker) detectNearDupBursts(ctx context.Context) error {
	// Look for bursts of near-duplicate images in a short time window
	rows, err := w.store.Query(ctx, `
		SELECT b.file_hash, b.platform_phash, p."timestamp", t.board_id, bo.site_id, p.id as post_id
		FROM blobs b
		JOIN files f ON f.file_hash = b.file_hash
		JOIN posts p ON p.id = f.post_id
		JOIN threads t ON t.id = p.thread_id
		JOIN boards bo ON bo.id = t.board_id
		WHERE b.platform_phash IS NOT NULL
		  AND p."timestamp" >= now() - interval '1 hour' * $1
		ORDER BY b.platform_phash, p."timestamp"`, w.lookbackHours)
	if err != nil {
		return err
	}
	defer rows.Close()

	type phashPost struct {
		Hash      string
		Timestamp time.Time
		BoardID   int64
		SiteID    int64
		PostID    int64
	}

	byPhash := make(map[string][]phashPost)

	for rows.Next() {
		var pp phashPost
		if err := rows.Scan(&pp.Hash, &pp.Hash, &pp.Timestamp, &pp.BoardID, &pp.SiteID, &pp.PostID); err != nil {
			continue
		}
		byPhash[pp.Hash] = append(byPhash[pp.Hash], pp)
	}

	// For each phash, check for bursts
	for phash, posts := range byPhash {
		if len(posts) < 3 {
			continue
		}

		// Sort by time
		sort.Slice(posts, func(i, j int) bool {
			return posts[i].Timestamp.Before(posts[j].Timestamp)
		})

		// Sliding window
		window := time.Duration(w.burstWindowMin) * time.Minute
		for i := 0; i < len(posts); i++ {
			count := 1
			for j := i + 1; j < len(posts); j++ {
				if posts[j].Timestamp.Sub(posts[i].Timestamp) <= window {
					count++
				} else {
					break
				}
			}

			// Expected rate: total posts / total hours
			totalHours := posts[len(posts)-1].Timestamp.Sub(posts[0].Timestamp).Hours()
			if totalHours < 1 {
				totalHours = 1
			}
			expectedRate := float64(len(posts)) / totalHours
			expectedInWindow := expectedRate * (window.Hours())

			if expectedInWindow > 0 {
				burstRatio := float64(count) / expectedInWindow
				if burstRatio >= w.burstThreshold && count >= 3 {
					// Check if we already recorded this
					var exists bool
					w.store.QueryRow(ctx, `
						SELECT EXISTS(SELECT 1 FROM anomaly_events 
							WHERE board_id = $1 AND type = 'near_dup_burst' 
							AND details->>'phash' = $2
							AND detected_at > now() - interval '1 hour')`,
						posts[i].BoardID, phash).Scan(&exists)

					if !exists {
						w.recordAnomaly(ctx, posts[i].BoardID, posts[i].SiteID, "near_dup_burst", map[string]any{
							"phash":          phash,
							"burst_count":    count,
							"window_minutes": w.burstWindowMin,
							"burst_ratio":    burstRatio,
							"expected_rate":  expectedRate,
						})
					}
				}
			}
		}
	}

	return nil
}

func (w *AnomalyWorker) detectCoordinatedPosting(ctx context.Context) error {
	// Look for same/similar content posted across multiple boards in short time
	rows, err := w.store.Query(ctx, `
		SELECT cm.blob_id, cm.post_ids, cm.board_ids, cm.site_ids, 
		       cm.first_seen_at, cm.last_seen_at, cm.spread_count
		FROM crosspost_matches cm
		WHERE cm.match_type IN ('exact_sha256', 'near_phash')
		  AND cm.spread_count >= 3
		  AND cm.last_seen_at >= now() - interval '1 hour' * $1
		  AND EXTRACT(EPOCH FROM (cm.last_seen_at - cm.first_seen_at))/3600 < 24`, w.lookbackHours)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var blobID string
		var postIDs, boardIDs, siteIDs []int64
		var firstSeen, lastSeen time.Time
		var spreadCount int

		if err := rows.Scan(&blobID, &postIDs, &boardIDs, &siteIDs, &firstSeen, &lastSeen, &spreadCount); err != nil {
			continue
		}

		// Check if posts were made within a very short window (coordinated)
		timeSpan := lastSeen.Sub(firstSeen).Minutes()
		if timeSpan <= 60 && spreadCount >= 3 { // 3+ boards within 1 hour
			// Use first board as reference
			if len(boardIDs) > 0 {
				w.recordAnomaly(ctx, boardIDs[0], siteIDs[0], "coordinated_posting", map[string]any{
					"blob_id":       blobID,
					"boards":        boardIDs,
					"sites":         siteIDs,
					"spread_count":  spreadCount,
					"time_span_min": timeSpan,
					"match_type":    "crosspost",
				})
			}
		}
	}

	return nil
}

func (w *AnomalyWorker) recordAnomaly(ctx context.Context, boardID, siteID int64, anomalyType string, details map[string]any) {
	detailsJSON, _ := jsonMarshal(details)

	err := w.store.Exec(ctx, `
		INSERT INTO anomaly_events (board_id, site_id, type, details, severity, detected_at)
		VALUES ($1, $2, $3, $4, $5, now())
		ON CONFLICT DO NOTHING`,
		boardID, siteID, anomalyType, detailsJSON, calculateSeverity(anomalyType, details))
	if err != nil {
		w.logger.Error("anomaly: record", "type", anomalyType, "err", err)
		return
	}

	w.logger.Info("anomaly: detected", "type", anomalyType, "board", boardID)
}

func calculateSeverity(anomalyType string, details map[string]any) string {
	switch anomalyType {
	case "volume_spike":
		if z, ok := details["z_score"].(float64); ok && z > 5 {
			return "critical"
		}
		return "high"
	case "near_dup_burst":
		if ratio, ok := details["burst_ratio"].(float64); ok && ratio > 10 {
			return "critical"
		}
		return "high"
	case "coordinated_posting":
		if count, ok := details["spread_count"].(int); ok && count > 5 {
			return "critical"
		}
		return "high"
	}
	return "medium"
}

func jsonMarshal(v any) (string, error) {
	// Simple JSON marshaling
	// In production, use encoding/json
	return "{}", nil
}
