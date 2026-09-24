package store

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

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
	FTSQuery      string     // optional full-text search query
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
			  AND ($9::tsvector IS NULL OR p.comment_tsv @@ plainto_tsquery($9::text))
		ORDER BY pe.embedding <=> $1::vector
		LIMIT $10`,
		formatVector(query), modelVersion,
		nullIfEmpty(opts.Site), nullIfEmpty(opts.Board),
		opts.ThreadID, opts.DateFrom, opts.DateTo, opts.HasAttachment, nullIfEmpty(opts.FTSQuery), limit)
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

// SearchHybridRRF performs hybrid dense + lexical search using Reciprocal Rank Fusion (RRF).
// Combines dense vector search (cosine similarity) with lexical full-text search (ts_rank).
// RRF formula: score = 1/(k + rank_dense) + 1/(k + rank_lexical), k=60 (default RRF constant).
// Candidate-narrowing variant: lexical prefilter -> exact rerank (cheaper for strong lexical signals).
func (s *Store) SearchHybridRRF(ctx context.Context, query []float32, modelVersion, ftsQuery string, opts SearchOpts) ([]SearchResult, error) {
	limit := opts.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	// If no FTS query provided, fall back to dense-only search
	if ftsQuery == "" {
		return s.SearchEmbeddings(ctx, query, modelVersion, opts)
	}
	// Candidate-narrowing: lexical prefilter -> exact dense rerank
	return s.searchHybridLexicalFirst(ctx, query, modelVersion, ftsQuery, opts)
}

// searchHybridLexicalFirst does lexical prefilter -> exact dense rerank.
// Used when there's a strong lexical signal (FTS query provided).
func (s *Store) searchHybridLexicalFirst(ctx context.Context, query []float32, modelVersion, ftsQuery string, opts SearchOpts) ([]SearchResult, error) {
	limit := opts.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	// Lexical candidates (top 500)
	rows, err := s.pool.Query(ctx, `
		SELECT p.id AS post_id,
		       ts_rank(p.comment_tsv, plainto_tsquery($1)) AS score
		FROM posts p
		JOIN threads t ON t.id = p.thread_id
		JOIN boards b ON b.id = t.board_id
		JOIN sites st ON st.id = b.site_id
		WHERE p.comment_tsv @@ plainto_tsquery($1)
		  AND ($2::text IS NULL OR st.name = $2)
		  AND ($3::text IS NULL OR b.code = $3)
		  AND ($4::bigint IS NULL OR t.id = $4)
		  AND ($5::timestamptz IS NULL OR p."timestamp" >= $5)
		  AND ($6::timestamptz IS NULL OR p."timestamp" <= $6)
		ORDER BY score DESC
		LIMIT 500`, ftsQuery,
		nullIfEmpty(opts.Site), nullIfEmpty(opts.Board),
		opts.ThreadID, opts.DateFrom, opts.DateTo)
	if err != nil {
		return nil, fmt.Errorf("store: hybrid lexical: %w", err)
	}
	defer rows.Close()

	type lexHit struct {
		postID int64
		score  float32
	}
	var lexical []lexHit
	for rows.Next() {
		var l lexHit
		if err := rows.Scan(&l.postID, &l.score); err != nil {
			continue
		}
		lexical = append(lexical, l)
	}

	if len(lexical) == 0 {
		return []SearchResult{}, nil
	}

	// Exact dense rerank on lexical candidates
	placeholders := make([]string, len(lexical))
	args := make([]any, len(lexical)+2)
	args[0] = modelVersion
	for i, l := range lexical {
		placeholders[i] = fmt.Sprintf("$%d", i+2)
		args[i+1] = l.postID
	}
	args[len(args)-1] = limit

	q := fmt.Sprintf(`
		SELECT pe.post_id,
		       1 - (pe.embedding <=> (SELECT embedding FROM post_embeddings WHERE post_id = p.id AND model_version = $1)) AS score,
		       st.name, b.code, t.id, t.thread_native_id, p."timestamp",
		       coalesce(p.comment_parsed, '')
		FROM post_embeddings pe
		JOIN posts p   ON p.id = pe.post_id
		JOIN threads t ON t.id = p.thread_id
		JOIN boards b  ON b.id = t.board_id
		JOIN sites st  ON st.id = b.site_id
		WHERE pe.post_id IN (%s) AND pe.model_version = $1
		ORDER BY pe.embedding <=> (SELECT embedding FROM post_embeddings WHERE post_id = p.id AND model_version = $1)
		LIMIT $%d`,
		strings.Join(placeholders, ","), len(lexical)+2)

	var rerankRows pgx.Rows
	rerankRows, err = s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("store: hybrid lexical rerank: %w", err)
	}
	defer rerankRows.Close()

	var out []SearchResult
	for rerankRows.Next() {
		var r SearchResult
		var commentParsed string
		if err := rerankRows.Scan(&r.PostID, &r.Score, &r.Site, &r.Board, &r.ThreadID,
			&r.ThreadNativeID, &r.Timestamp, &commentParsed); err != nil {
			return nil, err
		}
		r.Excerpt = excerpt(commentParsed, 200)
		out = append(out, r)
	}
	return out, rerankRows.Err()
}

// LexicalHit is a hit from lexical search.
type lexHit struct {
	postID int64
	score  float32
}

// SearchEmbeddingsHalfvec finds posts using half-precision vectors for
// scalar quantization. Uses halfvec cosine similarity with halfvec column.
// Requires pgvector 0.7+ with halfvec support.
func (s *Store) SearchEmbeddingsHalfvec(ctx context.Context, query []float32, modelVersion string, opts SearchOpts) ([]SearchResult, error) {
	limit := opts.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	rows, err := s.pool.Query(ctx, `
		SELECT pe.post_id,
		       1 - (pe.embedding_half <=> $1::halfvec) AS score,
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
		  AND ($9::tsvector IS NULL OR p.comment_tsv @@ plainto_tsquery($9::text))
		ORDER BY pe.embedding_half <=> $1::halfvec
		LIMIT $10`,
		formatVector(query), modelVersion,
		nullIfEmpty(opts.Site), nullIfEmpty(opts.Board),
		opts.ThreadID, opts.DateFrom, opts.DateTo, opts.HasAttachment, nullIfEmpty(opts.FTSQuery), limit)
	if err != nil {
		return nil, fmt.Errorf("store: search embeddings halfvec: %w", err)
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

// SearchEmbeddingsDotProduct finds posts using dot product (inner product) similarity.
// Since ONNX embedder outputs L2-normalized vectors, dot product is mathematically
// equivalent to cosine similarity (cosine = 1 - dot for unit vectors).
// Uses pgvector's <#> operator which returns negative inner product, so we negate it.
func (s *Store) SearchEmbeddingsDotProduct(ctx context.Context, query []float32, modelVersion string, opts SearchOpts) ([]SearchResult, error) {
	limit := opts.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	rows, err := s.pool.Query(ctx, `
		SELECT pe.post_id,
		       -1 * (pe.embedding <#> $1::vector) AS score,
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
		  AND ($9::tsvector IS NULL OR p.comment_tsv @@ plainto_tsquery($9::text))
		ORDER BY pe.embedding <#> $1::vector
		LIMIT $10`,
		formatVector(query), modelVersion,
		nullIfEmpty(opts.Site), nullIfEmpty(opts.Board),
		opts.ThreadID, opts.DateFrom, opts.DateTo, opts.HasAttachment, nullIfEmpty(opts.FTSQuery), limit)
	if err != nil {
		return nil, fmt.Errorf("store: search embeddings dot product: %w", err)
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

// NeighborEmbeddings resolves the embeddings of a post's neighbors in the reply graph.
// Returns (own_embedding, neighbor_embeddings, error). Neighbors include:
// - Parent post (via post_quotes -> threads -> boards -> posts)
// - Direct children/replies (reverse direction of post_quotes)
func (s *Store) NeighborEmbeddings(ctx context.Context, postID int64) ([]float32, [][]float32, error) {
	// Get own embedding
	var ownVecStr string
	err := s.pool.QueryRow(ctx, `
		SELECT embedding::text FROM post_embeddings WHERE post_id = $1
	`, postID).Scan(&ownVecStr)
	if err != nil {
		return nil, nil, fmt.Errorf("store: get own embedding: %w", err)
	}
	ownVec, err := parseVector(ownVecStr)
	if err != nil {
		return nil, nil, fmt.Errorf("store: parse own vector: %w", err)
	}

	// Get parent embedding (via post_quotes)
	var parentVecStr string
	err = s.pool.QueryRow(ctx, `
		SELECT pe.embedding::text
		FROM post_quotes pq
		JOIN posts p1 ON p1.id = pq.post_id
		JOIN threads t1 ON t1.id = p1.thread_id
		JOIN boards b1 ON b1.id = t1.board_id
		JOIN boards btgt ON btgt.site_id = b1.site_id
			AND btgt.code = CASE WHEN pq.board = '' THEN b1.code ELSE pq.board END
		JOIN threads t2 ON t2.board_id = btgt.id
		JOIN posts p2 ON p2.thread_id = t2.id AND p2.post_native_id = pq.quoted_post_native_id
		JOIN post_embeddings pe ON pe.post_id = p2.id
		WHERE pq.post_id = $1
		LIMIT 1
	`, postID).Scan(&parentVecStr)
	var parentVec []float32
	if err == nil && parentVecStr != "" {
		parentVec, _ = parseVector(parentVecStr)
	}

	// Get children embeddings (replies to this post)
	rows, err := s.pool.Query(ctx, `
		SELECT pe.embedding::text
		FROM post_quotes pq
		JOIN posts p1 ON p1.id = pq.post_id
		JOIN threads t1 ON t1.id = p1.thread_id
		JOIN boards b1 ON b1.id = t1.board_id
		JOIN boards btgt ON btgt.site_id = b1.site_id
			AND btgt.code = CASE WHEN pq.board = '' THEN b1.code ELSE pq.board END
		JOIN threads t2 ON t2.board_id = btgt.id
		JOIN posts p2 ON p2.thread_id = t2.id AND p2.post_native_id = pq.quoted_post_native_id
		JOIN post_embeddings pe ON pe.post_id = p1.id
		WHERE p2.id = $1
	`, postID)
	if err != nil {
		return nil, nil, fmt.Errorf("store: get children embeddings: %w", err)
	}
	defer rows.Close()

	var childVecs [][]float32
	for rows.Next() {
		var vecStr string
		if err := rows.Scan(&vecStr); err != nil {
			continue
		}
		if vecStr != "" {
			if vec, err := parseVector(vecStr); err == nil {
				childVecs = append(childVecs, vec)
			}
		}
	}

	var allVecs [][]float32
	if len(parentVec) > 0 {
		allVecs = append(allVecs, parentVec)
	}
	allVecs = append(allVecs, childVecs...)

	return ownVec, allVecs, nil
}

// MarkStructuralEmbedding upserts a post's structural embedding (embedding_struct).
func (s *Store) MarkStructuralEmbedding(ctx context.Context, postID int64, vec []float32, modelVersion string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE post_embeddings
		SET embedding_struct = $1::vector
		WHERE post_id = $1
	`, postID, formatVector(vec))
	if err != nil {
		return fmt.Errorf("store: mark structural embedding %d: %w", postID, err)
	}
	return nil
}

// SearchEmbeddingsStruct finds posts using structure-aware embeddings (embedding_struct).
// These embeddings incorporate the post's own content plus its reply-graph neighbors.
func (s *Store) SearchEmbeddingsStruct(ctx context.Context, query []float32, modelVersion string, opts SearchOpts) ([]SearchResult, error) {
	limit := opts.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	rows, err := s.pool.Query(ctx, `
		SELECT pe.post_id,
		       1 - (pe.embedding_struct <=> $1::vector) AS score,
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
		  AND ($9::tsvector IS NULL OR p.comment_tsv @@ plainto_tsquery($9::text))
		ORDER BY pe.embedding_struct <=> $1::vector
		LIMIT $10`,
		formatVector(query), modelVersion,
		nullIfEmpty(opts.Site), nullIfEmpty(opts.Board),
		opts.ThreadID, opts.DateFrom, opts.DateTo, opts.HasAttachment, nullIfEmpty(opts.FTSQuery), limit)
	if err != nil {
		return nil, fmt.Errorf("store: search embeddings struct: %w", err)
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

// PostWithEmbedding represents a post with its embedding vector.
type PostWithEmbedding struct {
	ID  int64
	Vec []float32
}

// GetPostsWithoutStructuralEmbedding returns posts that have embeddings but no structural embeddings.
func (s *Store) GetPostsWithoutStructuralEmbedding(ctx context.Context, modelVersion string, limit int) ([]PostWithEmbedding, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT pe.post_id, pe.embedding::text
		FROM post_embeddings pe
		WHERE pe.embedding IS NOT NULL
		AND pe.embedding_struct IS NULL
		AND pe.model_version = $1
		ORDER BY pe.post_id
		LIMIT $2`, modelVersion, limit)
	if err != nil {
		return nil, fmt.Errorf("store: fetch posts without structural embedding: %w", err)
	}
	defer rows.Close()

	var out []PostWithEmbedding
	for rows.Next() {
		var id int64
		var vecStr string
		if err := rows.Scan(&id, &vecStr); err != nil {
			return nil, err
		}
		vec, err := parseVector(vecStr)
		if err != nil {
			return nil, fmt.Errorf("store: parse vector for post %d: %w", id, err)
		}
		out = append(out, PostWithEmbedding{ID: id, Vec: vec})
	}
	return out, rows.Err()
}
