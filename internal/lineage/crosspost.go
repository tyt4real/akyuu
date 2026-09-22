package lineage

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

type CrosspostWorker struct {
	store       StoreAPI
	logger      *slog.Logger
	phashThresh int
}

func NewCrosspostWorker(st StoreAPI, logger *slog.Logger, phashThreshold int) *CrosspostWorker {
	if phashThreshold <= 0 {
		phashThreshold = 8
	}
	return &CrosspostWorker{
		store:       st,
		logger:      logger,
		phashThresh: phashThreshold,
	}
}

func (w *CrosspostWorker) RunOnce(ctx context.Context) error {
	w.logger.Debug("crosspost detection: starting scan")

	if err := w.detectExactMatches(ctx); err != nil {
		return fmt.Errorf("exact matches: %w", err)
	}

	if err := w.detectNearDuplicates(ctx); err != nil {
		return fmt.Errorf("near duplicates: %w", err)
	}

	return nil
}

func (w *CrosspostWorker) detectExactMatches(ctx context.Context) error {
	rows, err := w.store.Query(ctx, `
		SELECT 
			b.file_hash,
			COUNT(DISTINCT p.id) as post_count,
			COUNT(DISTINCT t.board_id) as board_count,
			COUNT(DISTINCT bo.site_id) as site_count,
			ARRAY_AGG(DISTINCT p.id) as post_ids,
			ARRAY_AGG(DISTINCT bo.site_id) as site_ids,
			ARRAY_AGG(DISTINCT t.board_id) as board_ids,
			MIN(p."timestamp") as first_seen,
			MAX(p."timestamp") as last_seen
		FROM blobs b
		JOIN files f ON f.file_hash = b.file_hash
		JOIN posts p ON p.id = f.post_id
		JOIN threads t ON t.id = p.thread_id
		JOIN boards bo ON bo.id = t.board_id
		WHERE b.file_hash IS NOT NULL
		GROUP BY b.file_hash
		HAVING COUNT(DISTINCT t.board_id) > 1 OR COUNT(DISTINCT bo.site_id) > 1
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var fileHash string
		var postCount, boardCount, siteCount int
		var postIDs, siteIDs, boardIDs []int64
		var firstSeen, lastSeen time.Time

		if err := rows.Scan(&fileHash, &postCount, &boardCount, &siteCount,
			&postIDs, &siteIDs, &boardIDs, &firstSeen, &lastSeen); err != nil {
			w.logger.Error("crosspost: scan exact match", "err", err)
			continue
		}

		matchType := "exact_sha256"
		err := w.store.Exec(ctx, `
			INSERT INTO crosspost_matches (blob_id, post_ids, site_ids, board_ids, match_type, first_seen_at, last_seen_at, spread_count)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			ON CONFLICT (blob_id, match_type) DO UPDATE
				SET post_ids = EXCLUDED.post_ids,
				    site_ids = EXCLUDED.site_ids,
				    board_ids = EXCLUDED.board_ids,
				    last_seen_at = EXCLUDED.last_seen_at,
				    spread_count = EXCLUDED.spread_count`,
			fileHash, postIDs, siteIDs, boardIDs, matchType, firstSeen, lastSeen, postCount)
		if err != nil {
			w.logger.Error("crosspost: upsert exact match", "hash", fileHash[:16]+"...", "err", err)
		}
	}

	return rows.Err()
}

func (w *CrosspostWorker) detectNearDuplicates(ctx context.Context) error {
	rows, err := w.store.Query(ctx, `
		SELECT file_hash, platform_phash
		FROM blobs
		WHERE platform_phash IS NOT NULL
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	type blobPHASH struct {
		hash  string
		phash string
	}
	var blobs []blobPHASH
	for rows.Next() {
		var b blobPHASH
		if err := rows.Scan(&b.hash, &b.phash); err != nil {
			continue
		}
		blobs = append(blobs, b)
	}

	if len(blobs) < 2 {
		return nil
	}

	w.logger.Debug("crosspost: near-dup scan", "blobs_with_phash", len(blobs))

	batchSize := 1000
	if len(blobs) > batchSize {
		blobs = blobs[:batchSize]
		w.logger.Debug("crosspost: limiting near-dup scan", "batch", batchSize)
	}

	seen := make(map[string]bool)

	for i, b1 := range blobs {
		if seen[b1.hash] {
			continue
		}

		var cluster []string
		cluster = append(cluster, b1.hash)
		seen[b1.hash] = true

		for j := i + 1; j < len(blobs); j++ {
			b2 := blobs[j]
			if seen[b2.hash] {
				continue
			}

			dist := hammingDistance(b1.phash, b2.phash)
			if dist <= w.phashThresh {
				cluster = append(cluster, b2.hash)
				seen[b2.hash] = true
			}
		}

		if len(cluster) > 1 {
			w.recordClusterMatch(ctx, cluster, "near_phash")
		}
	}

	return nil
}

func (w *CrosspostWorker) recordClusterMatch(ctx context.Context, blobHashes []string, matchType string) {
	if len(blobHashes) == 0 {
		return
	}

	placeholders := make([]string, len(blobHashes))
	args := make([]any, len(blobHashes))
	for i, h := range blobHashes {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
		args[i] = h
	}

	query := fmt.Sprintf(`
		SELECT 
			ARRAY_AGG(DISTINCT p.id) as post_ids,
			ARRAY_AGG(DISTINCT bo.site_id) as site_ids,
			ARRAY_AGG(DISTINCT t.board_id) as board_ids,
			MIN(p."timestamp") as first_seen,
			MAX(p."timestamp") as last_seen
		FROM blobs b
		JOIN files f ON f.file_hash = b.file_hash
		JOIN posts p ON p.id = f.post_id
		JOIN threads t ON t.id = p.thread_id
		JOIN boards bo ON bo.id = t.board_id
		WHERE b.file_hash IN (%s)
	`, joinPlaceholders(placeholders))

	var postIDs, siteIDs, boardIDs []int64
	var firstSeen, lastSeen time.Time

	err := w.store.QueryRow(ctx, query, args...).Scan(
		&postIDs, &siteIDs, &boardIDs, &firstSeen, &lastSeen)
	if err != nil {
		w.logger.Error("crosspost: query cluster", "err", err)
		return
	}

	if len(postIDs) == 0 {
		return
	}

	repHash := blobHashes[0]
	err = w.store.Exec(ctx, `
		INSERT INTO crosspost_matches (blob_id, post_ids, site_ids, board_ids, match_type, first_seen_at, last_seen_at, spread_count)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (blob_id, match_type) DO UPDATE
			SET post_ids = EXCLUDED.post_ids,
			    site_ids = EXCLUDED.site_ids,
			    board_ids = EXCLUDED.board_ids,
			    last_seen_at = EXCLUDED.last_seen_at,
			    spread_count = EXCLUDED.spread_count`,
		repHash, postIDs, siteIDs, boardIDs, matchType, firstSeen, lastSeen, len(postIDs))
	if err != nil {
		w.logger.Error("crosspost: upsert cluster", "err", err)
	}
}

func hammingDistance(a, b string) int {
	if len(a) != len(b) {
		return -1
	}

	dist := 0
	for i := 0; i < len(a); i++ {
		if a[i] != b[i] {
			da := decodeHex(a[i])
			db := decodeHex(b[i])
			xor := da ^ db
			for xor > 0 {
				dist += int(xor & 1)
				xor >>= 1
			}
		}
	}
	return dist
}

func decodeHex(c byte) byte {
	switch {
	case c >= '0' && c <= '9':
		return c - '0'
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10
	}
	return 0
}

func joinPlaceholders(ph []string) string {
	result := ""
	for i, p := range ph {
		if i > 0 {
			result += ","
		}
		result += p
	}
	return result
}
