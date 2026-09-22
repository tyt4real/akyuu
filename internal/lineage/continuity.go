package lineage

import (
	"context"
	"log/slog"
	"math"
	"strings"
)

type ContinuityWorker struct {
	store          StoreAPI
	logger         *slog.Logger
	timeGapHours   float64
	titleThreshold float64
	opSimThreshold float64
}

func NewContinuityWorker(st StoreAPI, logger *slog.Logger, timeGapHours float64, titleThreshold, opSimThreshold float64) *ContinuityWorker {
	if timeGapHours <= 0 {
		timeGapHours = 48
	}
	if titleThreshold <= 0 {
		titleThreshold = 0.8
	}
	if opSimThreshold <= 0 {
		opSimThreshold = 0.75
	}
	return &ContinuityWorker{
		store:          st,
		logger:         logger,
		timeGapHours:   timeGapHours,
		titleThreshold: titleThreshold,
		opSimThreshold: opSimThreshold,
	}
}

type candidate struct {
	prevID, nextID int64
	boardID        int64
	prevSubject    string
	nextSubject    string
	prevOpText     string
	nextOpText     string
	prevEmb        []float32
	nextEmb        []float32
	timeGapHours   float64
}

func (w *ContinuityWorker) RunOnce(ctx context.Context) error {
	w.logger.Debug("continuity: detecting thread links")

	rows, err := w.store.Query(ctx, `
		WITH archived_threads AS (
			SELECT t.id, t.board_id, t.thread_native_id, t.subject, t.last_bump_time,
			       p.comment_parsed as op_text,
			       pe.embedding as op_embedding
			FROM threads t
			JOIN posts p ON p.thread_id = t.id AND p.post_native_id = (
				SELECT MIN(post_native_id) FROM posts WHERE thread_id = t.id
			)
			LEFT JOIN post_embeddings pe ON pe.post_id = p.id
			WHERE t.status = 'archived' 
			  AND t.last_bump_time > now() - interval '30 days'
		),
		new_threads AS (
			SELECT t.id, t.board_id, t.thread_native_id, t.subject, t.first_seen_at,
			       p.comment_parsed as op_text,
			       pe.embedding as op_embedding
			FROM threads t
			JOIN posts p ON p.thread_id = t.id AND p.post_native_id = (
				SELECT MIN(post_native_id) FROM posts WHERE thread_id = t.id
			)
			LEFT JOIN post_embeddings pe ON pe.post_id = p.id
			WHERE t.status = 'active'
			  AND t.first_seen_at > now() - interval '30 days'
		)
		SELECT 
			a.id as prev_id, a.board_id, a.subject as prev_subject, a.op_text as prev_op,
			a.op_embedding as prev_emb,
			n.id as next_id, n.subject as next_subject, n.op_text as next_op,
			n.op_embedding as next_emb,
			EXTRACT(EPOCH FROM (n.first_seen_at - a.last_bump_time))/3600 as time_gap_hours
		FROM archived_threads a
		JOIN new_threads n ON n.board_id = a.board_id
		WHERE n.first_seen_at > a.last_bump_time
		  AND EXTRACT(EPOCH FROM (n.first_seen_at - a.last_bump_time))/3600 <= $1
		ORDER BY a.board_id, time_gap_hours
		LIMIT 500`, w.timeGapHours)
	if err != nil {
		return err
	}
	defer rows.Close()

	var candidates []candidate
	for rows.Next() {
		var c candidate
		var prevSubj, nextSubj, prevOp, nextOp interface{}
		var prevEmb, nextEmb interface{}

		if err := rows.Scan(&c.prevID, &c.boardID, &prevSubj, &prevOp, &prevEmb,
			&c.nextID, &nextSubj, &nextOp, &nextEmb, &c.timeGapHours); err != nil {
			continue
		}

		if prevSubj != nil {
			c.prevSubject = prevSubj.(string)
		}
		if nextSubj != nil {
			c.nextSubject = nextSubj.(string)
		}
		if prevOp != nil {
			c.prevOpText = prevOp.(string)
		}
		if nextOp != nil {
			c.nextOpText = nextOp.(string)
		}

		candidates = append(candidates, c)
	}

	for _, c := range candidates {
		w.evaluatePair(ctx, c)
	}

	return nil
}

func (w *ContinuityWorker) evaluatePair(ctx context.Context, c candidate) {
	titleSim := titleSimilarity(c.prevSubject, c.nextSubject)
	if titleSim < w.titleThreshold {
		return
	}

	opSim := 0.0
	if len(c.prevEmb) > 0 && len(c.nextEmb) > 0 {
		opSim = cosineSimilarity(c.prevEmb, c.nextEmb)
	} else if c.prevOpText != "" && c.nextOpText != "" {
		opSim = textSimilarity(c.prevOpText, c.nextOpText)
	}

	if opSim < w.opSimThreshold {
		return
	}

	confidence := (titleSim + opSim) / 2.0
	method := "title+op"
	if len(c.prevEmb) == 0 {
		method = "title+op_text"
	}

	err := w.store.Exec(ctx, `
		INSERT INTO continuity_links (prev_thread_id, next_thread_id, confidence, method, title_similarity, op_similarity, time_gap_hours)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (prev_thread_id, next_thread_id) DO UPDATE
			SET confidence = EXCLUDED.confidence,
			    method = EXCLUDED.method,
			    title_similarity = EXCLUDED.title_similarity,
			    op_similarity = EXCLUDED.op_similarity,
			    time_gap_hours = EXCLUDED.time_gap_hours`,
		c.prevID, c.nextID, confidence, method, titleSim, opSim, c.timeGapHours)
	if err != nil {
		w.logger.Error("continuity: insert link", "err", err)
		return
	}

	err = w.store.Exec(ctx, `UPDATE threads SET continuity_next_id = $1 WHERE id = $2`, c.nextID, c.prevID)
	if err != nil {
		w.logger.Error("continuity: update next pointer", "err", err)
	}
	err = w.store.Exec(ctx, `UPDATE threads SET continuity_prev_id = $1 WHERE id = $2`, c.prevID, c.nextID)
	if err != nil {
		w.logger.Error("continuity: update prev pointer", "err", err)
	}

	w.logger.Info("continuity: linked threads", "prev", c.prevID, "next", c.nextID, "confidence", confidence)
}

func titleSimilarity(a, b string) float64 {
	a = strings.TrimSpace(strings.ToLower(a))
	b = strings.TrimSpace(strings.ToLower(b))

	if a == "" || b == "" {
		return 0
	}
	if a == b {
		return 1.0
	}

	if strings.HasPrefix(a, b) || strings.HasPrefix(b, a) {
		minLen := len(a)
		if len(b) < minLen {
			minLen = len(b)
		}
		maxLen := len(a)
		if len(b) > maxLen {
			maxLen = len(b)
		}
		return float64(minLen) / float64(maxLen)
	}

	return 1.0 - float64(levenshtein(a, b))/float64(max(len(a), len(b)))
}

func levenshtein(a, b string) int {
	if len(a) == 0 {
		return len(b)
	}
	if len(b) == 0 {
		return len(a)
	}

	prev := make([]int, len(b)+1)
	curr := make([]int, len(b)+1)
	for i := range prev {
		prev[i] = i
	}

	for i := 1; i <= len(a); i++ {
		curr[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 0
			if a[i-1] != b[j-1] {
				cost = 1
			}
			curr[j] = min(
				prev[j]+1,
				curr[j-1]+1,
				prev[j-1]+cost,
			)
		}
		prev, curr = curr, prev
	}
	return prev[len(b)]
}

func textSimilarity(a, b string) float64 {
	wordsA := strings.Fields(strings.ToLower(a))
	wordsB := strings.Fields(strings.ToLower(b))

	if len(wordsA) == 0 || len(wordsB) == 0 {
		return 0
	}

	setA := make(map[string]bool)
	for _, w := range wordsA {
		setA[w] = true
	}

	common := 0
	for _, w := range wordsB {
		if setA[w] {
			common++
		}
	}

	return float64(common) / float64(max(len(wordsA), len(wordsB)))
}

func cosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) {
		return 0
	}

	var dot, normA, normB float64
	for i := range a {
		dot += float64(a[i] * b[i])
		normA += float64(a[i] * a[i])
		normB += float64(b[i] * b[i])
	}

	if normA == 0 || normB == 0 {
		return 0
	}

	return dot / (float64(math.Sqrt(normA)) * math.Sqrt(normB))
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b, c int) int {
	if a < b && a < c {
		return a
	}
	if b < c {
		return b
	}
	return c
}
