package search

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

type TrendAggregator struct {
	store  StoreAPI
	logger *slog.Logger
}

func NewTrendAggregator(st StoreAPI, logger *slog.Logger) *TrendAggregator {
	return &TrendAggregator{store: st, logger: logger}
}

func (t *TrendAggregator) RunOnce(ctx context.Context) error {
	t.logger.Debug("trends: aggregating")

	now := time.Now()
	windowStart := now.Truncate(time.Hour)
	windowEnd := windowStart.Add(time.Hour)

	// Aggregate total posts per board
	if err := t.aggregateTotalPosts(ctx, windowStart, windowEnd); err != nil {
		t.logger.Error("trends: total posts", "err", err)
	}

	// Aggregate top terms from post bodies using ts_stat
	if err := t.aggregateTermsFromFTS(ctx, windowStart, windowEnd); err != nil {
		t.logger.Error("trends: post terms", "err", err)
	}

	// Aggregate top terms from OCR text
	if err := t.aggregateTermsFromOCR(ctx, windowStart, windowEnd); err != nil {
		t.logger.Error("trends: ocr terms", "err", err)
	}

	// Aggregate top terms from transcripts
	if err := t.aggregateTermsFromTranscripts(ctx, windowStart, windowEnd); err != nil {
		t.logger.Error("trends: transcript terms", "err", err)
	}

	t.logger.Info("trends: aggregated", "window", windowStart.Format(time.RFC3339))
	return nil
}

func (t *TrendAggregator) aggregateTotalPosts(ctx context.Context, windowStart, windowEnd time.Time) error {
	err := t.store.Exec(ctx, `
		INSERT INTO trend_snapshots (board_id, site_id, term, count, window_start, window_end, interval)
		SELECT t.board_id, bo.site_id, 'total_posts', COUNT(DISTINCT p.id), $1, $2, 'hour'
		FROM posts p
		JOIN threads t ON t.id = p.thread_id
		JOIN boards bo ON bo.id = t.board_id
		WHERE p."timestamp" >= $1 AND p."timestamp" < $2
		GROUP BY t.board_id, bo.site_id
		ON CONFLICT (board_id, term, window_start, interval) DO UPDATE
			SET count = EXCLUDED.count,
			    site_id = EXCLUDED.site_id`,
		windowStart, windowEnd)
	return err
}

func (t *TrendAggregator) aggregateTermsFromFTS(ctx context.Context, windowStart, windowEnd time.Time) error {
	// Extract terms by scanning posts and counting words
	return t.aggregateTermsSimple(ctx, windowStart, windowEnd, "comment_parsed", "post")
}

func (t *TrendAggregator) aggregateTermsFromOCR(ctx context.Context, windowStart, windowEnd time.Time) error {
	return t.aggregateTermsSimple(ctx, windowStart, windowEnd, "text_cleaned", "ocr")
}

func (t *TrendAggregator) aggregateTermsFromTranscripts(ctx context.Context, windowStart, windowEnd time.Time) error {
	return t.aggregateTermsSimple(ctx, windowStart, windowEnd, "text", "transcript")
}

func (t *TrendAggregator) aggregateTermsSimple(ctx context.Context, windowStart, windowEnd time.Time, textColumn, source string) error {
	// Simpler approach: fetch posts in window, extract terms in Go, aggregate
	// This is less efficient but works without complex SQL

	var table string
	switch source {
	case "post":
		table = "posts"
	case "ocr":
		table = "post_ocr"
	case "transcript":
		table = "post_transcript"
	default:
		return fmt.Errorf("unknown source: %s", source)
	}

	rows, err := t.store.Query(ctx, fmt.Sprintf(`
		SELECT t.board_id, bo.site_id, p.%s
		FROM %s p
		JOIN threads t ON t.id = p.thread_id
		JOIN boards bo ON bo.id = t.board_id
		WHERE p."timestamp" >= $1 AND p."timestamp" < $2
		  AND p.%s IS NOT NULL AND p.%s != ''`, textColumn, table, textColumn, textColumn), windowStart, windowEnd)
	if err != nil {
		return err
	}
	defer rows.Close()

	// Aggregate terms per board
	type boardKey struct {
		boardID int64
		siteID  int64
	}
	termCounts := make(map[boardKey]map[string]int)

	for rows.Next() {
		var boardID, siteID int64
		var text string
		if err := rows.Scan(&boardID, &siteID, &text); err != nil {
			continue
		}

		key := boardKey{boardID: boardID, siteID: siteID}
		if termCounts[key] == nil {
			termCounts[key] = make(map[string]int)
		}

		terms := extractTerms(text)
		for _, term := range terms {
			termCounts[key][term]++
		}
	}

	// Store top terms per board
	for key, counts := range termCounts {
		// Get top 50 terms
		type termCount struct {
			term  string
			count int
		}
		var topTerms []termCount
		for term, count := range counts {
			if count > 1 {
				topTerms = append(topTerms, termCount{term, count})
			}
		}

		// Sort by count descending (simple bubble sort for small N)
		for i := 0; i < len(topTerms)-1; i++ {
			for j := i + 1; j < len(topTerms); j++ {
				if topTerms[j].count > topTerms[i].count {
					topTerms[i], topTerms[j] = topTerms[j], topTerms[i]
				}
			}
		}

		if len(topTerms) > 50 {
			topTerms = topTerms[:50]
		}

		// Batch insert
		for _, tc := range topTerms {
			err := t.store.Exec(ctx, `
				INSERT INTO trend_snapshots (board_id, site_id, term, count, window_start, window_end, interval)
				VALUES ($1, $2, $3, $4, $5, $6, 'hour')
				ON CONFLICT (board_id, term, window_start, interval) DO UPDATE
					SET count = EXCLUDED.count,
					    site_id = EXCLUDED.site_id`,
				key.boardID, key.siteID, tc.term, tc.count, windowStart, windowEnd)
			if err != nil {
				t.logger.Error("trends: insert term", "err", err)
			}
		}
	}

	return rows.Err()
}

func extractTerms(text string) []string {
	text = stripHTML(text)
	words := strings.FieldsFunc(text, func(r rune) bool {
		return !isLetterOrDigit(r)
	})

	stopWords := map[string]bool{
		"the": true, "a": true, "an": true, "and": true, "or": true,
		"but": true, "in": true, "on": true, "at": true, "to": true,
		"for": true, "of": true, "with": true, "by": true, "from": true,
		"is": true, "are": true, "was": true, "were": true, "be": true,
		"been": true, "being": true, "have": true, "has": true, "had": true,
		"do": true, "does": true, "did": true, "will": true, "would": true,
		"could": true, "should": true, "may": true, "might": true, "must": true,
		"can": true, "this": true, "that": true, "these": true, "those": true,
		"it": true, "its": true, "they": true, "them": true, "their": true,
		"i": true, "you": true, "he": true, "she": true, "we": true,
		"me": true, "him": true, "her": true, "us": true,
		"my": true, "your": true, "his": true, "our": true,
		"what": true, "when": true, "where": true, "who": true, "why": true,
		"how": true, "if": true, "then": true, "else": true, "than": true,
		"so": true, "as": true, "not": true, "no": true, "yes": true,
	}

	var terms []string
	for _, w := range words {
		w = strings.ToLower(w)
		if len(w) >= 3 && !stopWords[w] {
			terms = append(terms, w)
		}
	}
	return terms
}

func stripHTML(text string) string {
	result := strings.Builder{}
	inTag := false
	for _, r := range text {
		if r == '<' {
			inTag = true
		} else if r == '>' {
			inTag = false
		} else if !inTag {
			result.WriteRune(r)
		}
	}
	return result.String()
}

func isLetterOrDigit(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
}
