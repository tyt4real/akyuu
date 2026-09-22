package search

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"
)

// Evaluator evaluates saved searches and records hits.
type Evaluator struct {
	store    StoreAPI
	logger   *slog.Logger
	embedder Embedder
}

type SavedSearch struct {
	ID            int64
	Name          string
	Description   string
	DSLQuery      string
	DSLAST        json.RawMessage
	SQLWhere      string
	VectorParams  json.RawMessage
	SiteFilter    string
	BoardFilter   string
	IsPublic      bool
	LastEvaluated *time.Time
	EvalCount     int
}

type SearchHit struct {
	SavedSearchID int64
	PostID        int64
	Score         float32
	MatchedAt     time.Time
}

func NewEvaluator(st StoreAPI, emb Embedder, logger *slog.Logger) *Evaluator {
	return &Evaluator{
		store:    st,
		logger:   logger,
		embedder: emb,
	}
}

func (e *Evaluator) RunOnce(ctx context.Context) error {
	e.logger.Debug("search: evaluating saved searches")

	searches, err := e.getActiveSearches(ctx)
	if err != nil {
		return fmt.Errorf("get searches: %w", err)
	}

	for _, s := range searches {
		if err := e.evaluateSearch(ctx, s); err != nil {
			e.logger.Error("search: evaluate failed", "search", s.ID, "err", err)
		}
	}

	return nil
}

func (e *Evaluator) getActiveSearches(ctx context.Context) ([]*SavedSearch, error) {
	rows, err := e.store.Query(ctx, `
		SELECT id, name, description, dsl_query, dsl_ast, sql_where, vector_params,
		       site_filter, board_filter, is_public, last_evaluated, evaluation_count
		FROM saved_searches
		WHERE is_public = true OR is_public = false  -- all searches for now
		ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var searches []*SavedSearch
	for rows.Next() {
		var s SavedSearch
		var lastEval sql.NullTime
		if err := rows.Scan(&s.ID, &s.Name, &s.Description, &s.DSLQuery, &s.DSLAST,
			&s.SQLWhere, &s.VectorParams, &s.SiteFilter, &s.BoardFilter,
			&s.IsPublic, &lastEval, &s.EvalCount); err != nil {
			continue
		}
		if lastEval.Valid {
			s.LastEvaluated = &lastEval.Time
		}
		searches = append(searches, &s)
	}
	return searches, rows.Err()
}

func (e *Evaluator) evaluateSearch(ctx context.Context, s *SavedSearch) error {
	// Parse DSL if needed
	var ast DSLAST
	if len(s.DSLAST) > 0 {
		if err := json.Unmarshal(s.DSLAST, &ast); err != nil {
			return fmt.Errorf("parse ast: %w", err)
		}
	} else {
		p := NewParser(s.DSLQuery)
		parsed, err := p.Parse()
		if err != nil {
			return fmt.Errorf("parse dsl: %w", err)
		}
		ast = *parsed
	}

	// Build vector params if there's a semantic/vector filter
	var vectorParams map[string]any
	if s.VectorParams != nil {
		json.Unmarshal(s.VectorParams, &vectorParams)
	}

	// If query has semantic/vector field, embed it
	var queryVector []float32
	for _, f := range ast.Filters {
		if f.Field == "vector" || f.Field == "embedding" || f.Field == "semantic" {
			if text, ok := f.Value.(string); ok {
				vecs, err := e.embedder.EmbedBatch(ctx, []string{text})
				if err != nil {
					return fmt.Errorf("embed query: %w", err)
				}
				queryVector = vecs[0]
			}
			break
		}
	}

	// Execute search using the compiled SQL
	where, args, vp := ast.ToSQLWhere()

	// Merge vector params
	for k, v := range vp {
		if vectorParams == nil {
			vectorParams = make(map[string]any)
		}
		vectorParams[k] = v
	}

	// Add site/board filters
	if s.SiteFilter != "" {
		if where == "" {
			where = "WHERE "
		} else {
			where += " AND "
		}
		where += fmt.Sprintf("st.name = $%d", len(args)+1)
		args = append(args, s.SiteFilter)
	}
	if s.BoardFilter != "" {
		if where == "" {
			where = "WHERE "
		} else {
			where += " AND "
		}
		where += fmt.Sprintf("b.code = $%d", len(args)+1)
		args = append(args, s.BoardFilter)
	}

	// Build final query
	query := fmt.Sprintf(`
		SELECT p.id, p.thread_id, p.post_native_id, p."timestamp",
		       st.name as site, b.code as board, t.thread_native_id,
		       p.comment_parsed,
		       1 - (pe.embedding <=> $%d::vector) as score
		FROM posts p
		JOIN threads t ON t.id = p.thread_id
		JOIN boards b ON b.id = t.board_id
		JOIN sites st ON st.id = b.site_id
		LEFT JOIN post_embeddings pe ON pe.post_id = p.id AND pe.model_version = $%d
		%s
		ORDER BY pe.embedding <=> $%d::vector
		LIMIT 100`,
		len(args)+1, len(args)+1, where, len(args)+1)

	// If we have a vector query, add it
	if queryVector != nil {
		args = append(args, formatVector(queryVector))
	} else {
		// No vector - use FTS or skip
		return nil
	}

	rows, err := e.store.Query(ctx, query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()

	hitCount := 0
	newHits := []int64{}

	for rows.Next() {
		var hit SearchHit
		var commentParsed sql.NullString
		hit.SavedSearchID = s.ID

		if err := rows.Scan(&hit.PostID, &hit.PostID, &hit.PostID, &hit.MatchedAt,
			&hit.PostID, &hit.PostID, &hit.PostID, &commentParsed, &hit.Score); err != nil {
			continue
		}

		// Check if this hit is new (after last_evaluated)
		isNew := s.LastEvaluated == nil || hit.MatchedAt.After(*s.LastEvaluated)

		// Upsert search_hit
		err := e.store.Exec(ctx, `
			INSERT INTO search_hits (saved_search_id, post_id, score, matched_at)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (saved_search_id, post_id) DO UPDATE
				SET score = EXCLUDED.score,
				    matched_at = EXCLUDED.matched_at`,
			s.ID, hit.PostID, hit.Score, hit.MatchedAt)
		if err != nil {
			e.logger.Error("search: upsert hit", "err", err)
			continue
		}

		hitCount++
		if isNew {
			newHits = append(newHits, hit.PostID)
		}
	}

	// Update search metadata
	now := time.Now()
	err = e.store.Exec(ctx, `
		UPDATE saved_searches
		SET last_evaluated = $1, evaluation_count = evaluation_count + 1
		WHERE id = $2`, now, s.ID)
	if err != nil {
		return err
	}

	// Fire alerts if there are new hits
	if len(newHits) > 0 {
		e.fireAlerts(ctx, s, newHits)
	}

	e.logger.Debug("search: evaluated", "search", s.ID, "hits", hitCount, "new", len(newHits))
	return nil
}

func (e *Evaluator) fireAlerts(ctx context.Context, s *SavedSearch, newPostIDs []int64) {
	// Get alert configs for this search
	rows, err := e.store.Query(ctx, `
		SELECT id, channel, target, threshold, cooldown_minutes, last_triggered
		FROM search_alerts
		WHERE saved_search_id = $1 AND enabled = true`, s.ID)
	if err != nil {
		e.logger.Error("search: get alerts", "err", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var alertID int64
		var channel, target string
		var threshold int
		var cooldownMinutes int
		var lastTriggered sql.NullTime

		if err := rows.Scan(&alertID, &channel, &target, &threshold, &cooldownMinutes, &lastTriggered); err != nil {
			continue
		}

		// Check threshold
		if len(newPostIDs) < threshold {
			continue
		}

		// Check cooldown
		if lastTriggered.Valid && time.Since(lastTriggered.Time) < time.Duration(cooldownMinutes)*time.Minute {
			continue
		}

		// Fire alert
		e.dispatchAlert(ctx, alertID, channel, target, s, newPostIDs)
	}
}

func (e *Evaluator) dispatchAlert(ctx context.Context, alertID int64, channel, target string, s *SavedSearch, postIDs []int64) {
	payload := map[string]any{
		"alert_id":     alertID,
		"search_id":    s.ID,
		"search_name":  s.Name,
		"hit_count":    len(postIDs),
		"post_ids":     postIDs,
		"triggered_at": time.Now(),
	}

	payloadJSON, _ := json.Marshal(payload)

	status := "sent"
	var errMsg string

	switch channel {
	case "webhook":
		// HTTP POST to target URL
		// TODO: implement HTTP client
		errMsg = "webhook not implemented"
		status = "failed"
	case "log":
		e.logger.Info("SEARCH ALERT", "payload", string(payloadJSON))
	default:
		errMsg = "unknown channel: " + channel
		status = "failed"
	}

	// Record in alert_history
	_ = e.store.Exec(ctx, `
		INSERT INTO alert_history (alert_id, hit_count, new_post_ids, payload, status, error)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		alertID, len(postIDs), postIDs, payloadJSON, status, errMsg)

	// Update last_triggered
	_ = e.store.Exec(ctx, `
		UPDATE search_alerts SET last_triggered = now() WHERE id = $1`, alertID)
}

func formatVector(v []float32) string {
	// Same as embedder formatVector
	var b strings.Builder
	b.WriteByte('[')
	for i, f := range v {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(strconv.FormatFloat(float64(f), 'g', -1, 32))
	}
	b.WriteByte(']')
	return b.String()
}
