package store

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"akyuu/internal/parser"
)

// PostEmbeddingRow is the minimal shape the embed worker needs from posts.
//
// (The Post model lives in models.go; these are the queries that feed it.)

// PendingEmbeddingBatch returns up to limit posts whose bodies still need
// embedding, oldest first. The embed worker drains this loop; because every
// such post is also marked non-pending when processed (either embedded or
// skipped), the same loop doubles as the one-time archive backfill.
func (s *Store) PendingEmbeddingBatch(ctx context.Context, limit int) ([]*Post, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, thread_id, post_native_id, "timestamp", coalesce(comment_parsed, '')
		FROM posts WHERE pending_embedding ORDER BY id LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("store: pending embedding batch: %w", err)
	}
	defer rows.Close()
	var out []*Post
	for rows.Next() {
		var p Post
		if err := rows.Scan(&p.ID, &p.ThreadID, &p.NativeID, &p.Timestamp, &p.CommentParsed); err != nil {
			return nil, err
		}
		out = append(out, &p)
	}
	return out, rows.Err()
}

// MarkEmbedding upserts a post's vector (idempotent on post_id) and clears the
// embedding flag in the same transaction.
func (s *Store) MarkEmbedding(ctx context.Context, postID int64, vec []float32, modelVersion, textHash string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("store: begin embedding tx: %w", err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `
		INSERT INTO post_embeddings (post_id, embedding, model_version, text_hash)
		VALUES ($1, $2::vector, $3, $4)
		ON CONFLICT (post_id) DO UPDATE
			SET embedding = EXCLUDED.embedding,
			    model_version = EXCLUDED.model_version,
			    text_hash = EXCLUDED.text_hash,
			    created_at = now()`,
		postID, formatVector(vec), modelVersion, textHash); err != nil {
		return fmt.Errorf("store: upsert embedding %d: %w", postID, err)
	}
	if _, err := tx.Exec(ctx, `UPDATE posts SET pending_embedding = FALSE WHERE id = $1`, postID); err != nil {
		return fmt.Errorf("store: clear embedding flag %d: %w", postID, err)
	}
	return tx.Commit(ctx)
}

// MarkEmbeddingSkipped clears the embedding flag without storing a vector: the
// post body is empty or too short to carry meaning. The post stays fully
// reachable by browsing its thread.
func (s *Store) MarkEmbeddingSkipped(ctx context.Context, postID int64) error {
	if _, err := s.pool.Exec(ctx,
		`UPDATE posts SET pending_embedding = FALSE WHERE id = $1`, postID); err != nil {
		return fmt.Errorf("store: skip embedding %d: %w", postID, err)
	}
	return nil
}

// SearchOpts scopes a semantic search query.
type SearchOpts struct {
	Site          string     // optional site name filter
	Board         string     // optional board code filter
	ThreadID      *int64     // optional thread-scope filter
	DateFrom      *time.Time // optional lower bound on post timestamp
	DateTo        *time.Time // optional upper bound on post timestamp
	HasAttachment *bool      // optional: only posts with a file row
	Limit         int        // result cap (clamped to 1..100, default 20)
}

// SearchResult is one semantic-search hit with enough context to jump into the
// thread. Full post content is read from the primary tables by the caller.
type SearchResult struct {
	PostID         int64
	Score          float32
	Site           string
	Board          string
	ThreadID       int64
	ThreadNativeID string
	Timestamp      *time.Time
	Excerpt        string
}

// SearchEmbeddings finds posts whose vectors are closest to the query vector,
// applying the same optional filters the archiver stores. Only rows produced
// by modelVersion are considered, so vectors from different models never mix.
func (s *Store) SearchEmbeddings(ctx context.Context, query []float32, modelVersion string, opts SearchOpts) ([]SearchResult, error) {
	limit := opts.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	rows, err := s.pool.Query(ctx, `
		SELECT pe.post_id,
		       1 - (pe.embedding <=> $1::vector) AS score,
		       st.name, b.code, t.id, t.thread_native_id, p."timestamp",
		       coalesce(p.comment_parsed, '')
		FROM post_embeddings pe
		JOIN posts p   ON p.id = pe.post_id
		JOIN threads t ON t.id = p.thread_id
		JOIN boards b  ON b.id = t.board_id
		JOIN sites st  ON st.id = b.site_id
		WHERE pe.model_version = $2
		  AND ($3::text IS NULL OR st.name = $3)
		  AND ($4::text IS NULL OR b.code = $4)
		  AND ($5::bigint IS NULL OR t.id = $5)
		  AND ($6::timestamptz IS NULL OR p."timestamp" >= $6)
		  AND ($7::timestamptz IS NULL OR p."timestamp" <= $7)
		  AND (
		        $8::boolean IS NULL
		        OR ($8 = TRUE  AND EXISTS (SELECT 1 FROM files f WHERE f.post_id = p.id))
		        OR ($8 = FALSE AND NOT EXISTS (SELECT 1 FROM files f WHERE f.post_id = p.id))
		      )
		ORDER BY pe.embedding <=> $1::vector
		LIMIT $9`,
		formatVector(query), modelVersion,
		nullIfEmpty(opts.Site), nullIfEmpty(opts.Board),
		opts.ThreadID, opts.DateFrom, opts.DateTo, opts.HasAttachment, limit)
	if err != nil {
		return nil, fmt.Errorf("store: search embeddings: %w", err)
	}
	defer rows.Close()
	var out []SearchResult
	for rows.Next() {
		var r SearchResult
		var commentParsed string
		if err := rows.Scan(&r.PostID, &r.Score, &r.Site, &r.Board, &r.ThreadID,
			&r.ThreadNativeID, &r.Timestamp, &commentParsed); err != nil {
			return nil, err
		}
		r.Excerpt = excerpt(commentParsed, 200)
		out = append(out, r)
	}
	return out, rows.Err()
}

// excerpt reduces an HTML comment to a plain-text snippet.
func excerpt(commentHTML string, maxLen int) string {
	text, err := parser.TextContent(commentHTML)
	if err != nil || text == "" {
		return ""
	}
	text = strings.Join(strings.Fields(text), " ")
	if len(text) <= maxLen {
		return text
	}
	return text[:maxLen] + "…"
}

// formatVector renders []float32 as the pgvector literal syntax "[a,b,...]".
// Vectors are always passed as $N::vector string literals, so no pgx type
// registration is needed and vector columns are never scanned back.
func formatVector(v []float32) string {
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

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
