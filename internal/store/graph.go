package store

import (
	"context"
	"fmt"
)

// Neighbors returns the post IDs directly linked to postID via
// post_quotes, in either direction: posts postID quotes, and posts that
// quote postID. Resolution is board-aware, matching ParentText's join.
func (s *Store) Neighbors(ctx context.Context, postID int64) ([]int64, error) {
	rows, err := s.pool.Query(ctx, `
		-- outgoing: postID -> whoever it quotes
		SELECT p2.id
		FROM post_quotes pq
		JOIN posts p1 ON p1.id = pq.post_id
		JOIN threads t1 ON t1.id = p1.thread_id
		JOIN boards b1 ON b1.id = t1.board_id
		JOIN boards btgt ON btgt.site_id = b1.site_id
			AND btgt.code = CASE WHEN pq.board = '' THEN b1.code ELSE pq.board END
		JOIN threads t2 ON t2.board_id = btgt.id
		JOIN posts p2 ON p2.thread_id = t2.id AND p2.post_native_id = pq.quoted_post_native_id
		WHERE pq.post_id = $1
		UNION
		-- incoming: whoever quotes postID
		SELECT pq2.post_id
		FROM post_quotes pq2
		JOIN posts pself ON pself.id = $1
		JOIN threads tself ON tself.id = pself.thread_id
		JOIN boards bself ON bself.id = tself.board_id
		JOIN posts pq_post ON pq_post.id = pq2.post_id
		JOIN threads pq_thread ON pq_thread.id = pq_post.thread_id
		JOIN boards pq_board ON pq_board.id = pq_thread.board_id
		WHERE pq2.quoted_post_native_id = pself.post_native_id
		  AND (
		        (pq2.board = '' AND pq_board.site_id = bself.site_id AND pq_board.code = bself.code)
		        OR pq2.board = bself.code
		      )`, postID)
	if err != nil {
		return nil, fmt.Errorf("store: neighbors %d: %w", postID, err)
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// EmbeddingsFor returns existing content embeddings for postIDs that have
// one, for the given model version. Posts with no stored vector yet are
// simply absent from the returned map, not an error.
func (s *Store) EmbeddingsFor(ctx context.Context, postIDs []int64, modelVersion string) (map[int64][]float32, error) {
	if len(postIDs) == 0 {
		return map[int64][]float32{}, nil
	}
	placeholders := make([]string, len(postIDs))
	args := make([]any, len(postIDs)+1)
	args[0] = modelVersion
	for i, id := range postIDs {
		placeholders[i] = fmt.Sprintf("$%d", i+2)
		args[i+1] = id
	}
	q := fmt.Sprintf(`
		SELECT pe.post_id, pe.embedding::text
		FROM post_embeddings pe
		WHERE pe.model_version = $1 AND pe.post_id IN (%s)`, joinPlaceholders(placeholders))
	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("store: embeddings for: %w", err)
	}
	defer rows.Close()
	out := make(map[int64][]float32, len(postIDs))
	for rows.Next() {
		var id int64
		var vecStr string
		if err := rows.Scan(&id, &vecStr); err != nil {
			continue
		}
		vec, err := parseVector(vecStr)
		if err != nil {
			continue
		}
		out[id] = vec
	}
	return out, rows.Err()
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
