package lineage

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

type LineageWorker struct {
	store     StoreAPI
	logger    *slog.Logger
	algo      string
	threshold int
	batchSize int
}

type blobInfo struct {
	hash      string
	phash     string
	width     int
	height    int
	firstSeen time.Time
	postIDs   []int64
}

func NewLineageWorker(st StoreAPI, logger *slog.Logger, algo string, threshold, batchSize int) *LineageWorker {
	if algo == "" {
		algo = "phash"
	}
	if threshold <= 0 {
		threshold = 8
	}
	if batchSize <= 0 {
		batchSize = 500
	}
	return &LineageWorker{
		store:     st,
		logger:    logger,
		algo:      algo,
		threshold: threshold,
		batchSize: batchSize,
	}
}

func (w *LineageWorker) RunOnce(ctx context.Context) error {
	w.logger.Debug("lineage: building clusters", "algo", w.algo)

	var lastBlobID string
	err := w.store.QueryRow(ctx, `
		SELECT last_blob_id FROM meme_lineage_state WHERE algo = $1`, w.algo).Scan(&lastBlobID)
	if err != nil {
		lastBlobID = ""
	}

	query := `
		SELECT b.file_hash, b.platform_phash, bp.width, bp.height, 
		       MIN(p."timestamp") as first_seen,
		       ARRAY_AGG(DISTINCT p.id) as post_ids
		FROM blobs b
		JOIN blob_phash bp ON bp.blob_id = b.file_hash AND bp.algo = $1
		JOIN files f ON f.file_hash = b.file_hash
		JOIN posts p ON p.id = f.post_id
		WHERE b.platform_phash IS NOT NULL
	`
	args := []any{w.algo}

	if lastBlobID != "" {
		query += " AND b.file_hash > $2"
		args = append(args, lastBlobID)
	}

	query += `
		GROUP BY b.file_hash, b.platform_phash, bp.width, bp.height
		ORDER BY b.file_hash
		LIMIT $` + fmt.Sprintf("%d", len(args)+1)
	args = append(args, w.batchSize)

	rows, err := w.store.Query(ctx, query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()

	var newBlobs []blobInfo
	for rows.Next() {
		var b blobInfo
		if err := rows.Scan(&b.hash, &b.phash, &b.width, &b.height, &b.firstSeen, &b.postIDs); err != nil {
			continue
		}
		newBlobs = append(newBlobs, b)
	}

	if len(newBlobs) == 0 {
		w.logger.Debug("lineage: no new blobs to process")
		return nil
	}

	lastProcessed := newBlobs[len(newBlobs)-1].hash
	err = w.store.Exec(ctx, `
		INSERT INTO meme_lineage_state (algo, last_blob_id, last_updated)
		VALUES ($1, $2, now())
		ON CONFLICT (algo) DO UPDATE
			SET last_blob_id = EXCLUDED.last_blob_id,
			    last_updated = now()`,
		w.algo, lastProcessed)
	if err != nil {
		w.logger.Error("lineage: update state", "err", err)
	}

	for _, b := range newBlobs {
		w.assignToCluster(ctx, b)
	}

	return nil
}

func (w *LineageWorker) assignToCluster(ctx context.Context, b blobInfo) {
	var newClusterID int64
	err := w.store.QueryRow(ctx, `
		INSERT INTO meme_clusters (algo, threshold, representative, blob_count, first_seen_at, last_seen_at)
		VALUES ($1, $2, $3, 1, $4, $5)
		RETURNING id`,
		w.algo, w.threshold, b.hash, b.firstSeen, b.firstSeen).Scan(&newClusterID)
	if err != nil {
		w.logger.Error("lineage: create cluster", "err", err)
		return
	}

	err = w.store.Exec(ctx, `
		INSERT INTO meme_cluster_blobs (cluster_id, blob_id, distance, generation, first_seen_post_id, first_seen_at)
		VALUES ($1, $2, 0, 0, $3, $4)
		ON CONFLICT DO NOTHING`,
		newClusterID, b.hash, firstOrZero(b.postIDs), b.firstSeen)
	if err != nil {
		w.logger.Error("lineage: add blob to cluster", "err", err)
	}

	for _, postID := range b.postIDs {
		var boardID, siteID int64
		w.store.QueryRow(ctx, `
			SELECT t.board_id, bo.site_id
			FROM posts p
			JOIN threads t ON t.id = p.thread_id
			JOIN boards bo ON bo.id = t.board_id
			WHERE p.id = $1`, postID).Scan(&boardID, &siteID)

		if boardID > 0 && siteID > 0 {
			err = w.store.Exec(ctx, `
				INSERT INTO meme_spread (cluster_id, board_id, site_id, post_id, blob_id, first_seen_at)
				VALUES ($1, $2, $3, $4, $5, $6)
				ON CONFLICT (cluster_id, post_id) DO NOTHING`,
				newClusterID, boardID, siteID, postID, b.hash, b.firstSeen)
			if err != nil {
				w.logger.Error("lineage: insert spread", "err", err)
			}
		}
	}
}

func firstOrZero(arr []int64) int64 {
	if len(arr) > 0 {
		return arr[0]
	}
	return 0
}
