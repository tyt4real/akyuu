package store

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"akyuu/internal/adapter"
)

// UpsertThreadPosts writes (or updates) all posts of a thread and their files
// and quote graph in one transaction. Platform-provided content hashes are used
// to bind file rows to existing blobs so cross-site dedup happens even before
// any bytes are downloaded.
func (s *Store) UpsertThreadPosts(ctx context.Context, threadID int64, posts []*adapter.Post) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("store: begin posts tx: %w", err)
	}
	defer tx.Rollback(ctx)

	for _, p := range posts {
		postID, err := upsertPost(ctx, tx, threadID, p)
		if err != nil {
			return err
		}
		if err := upsertQuotes(ctx, tx, postID, p.Quotes); err != nil {
			return err
		}
		for _, f := range p.Files {
			if err := upsertFile(ctx, tx, postID, f); err != nil {
				return err
			}
		}
	}
	return tx.Commit(ctx)
}

func upsertPost(ctx context.Context, tx pgx.Tx, threadID int64, p *adapter.Post) (int64, error) {
	var id int64
	ts := tsOrNull(p.Timestamp)
	parent := strings.TrimSpace(p.ParentID)
	if parent == "" || parent == "0" {
		parent = p.ThreadID
	}
	err := tx.QueryRow(ctx, `
		INSERT INTO posts (thread_id, post_native_id, "timestamp", author_name, tripcode,
		                   capcode, poster_id, comment_raw, comment_parsed, sage, country, flag,
		                   original_board, website, original_thread_number, original_attachment_link,
		                   pending_embedding)
		VALUES ($1, $2, $3, NULLIF($4,''), NULLIF($5,''), NULLIF($6,''), NULLIF($7,''),
		        $8, $9, $10, NULLIF($11,''), NULLIF($12,''), NULLIF($13,''), NULLIF($14,''), NULLIF($15,''), NULLIF($16,''), TRUE)
		ON CONFLICT (thread_id, post_native_id) DO UPDATE
			SET "timestamp" = EXCLUDED."timestamp",
			    author_name = EXCLUDED.author_name,
			    tripcode    = EXCLUDED.tripcode,
			    capcode     = EXCLUDED.capcode,
			    poster_id   = EXCLUDED.poster_id,
			    comment_raw = EXCLUDED.comment_raw,
			    comment_parsed = EXCLUDED.comment_parsed,
			    sage        = EXCLUDED.sage,
			    country     = EXCLUDED.country,
			    flag        = EXCLUDED.flag,
			    original_board = EXCLUDED.original_board,
			    website       = EXCLUDED.website,
			    original_thread_number = EXCLUDED.original_thread_number,
			    original_attachment_link = EXCLUDED.original_attachment_link,
			-- Only re-embed when the text actually changed; steady-state
			-- re-polls of unchanged posts stay embedded.
			    pending_embedding = (posts.comment_parsed IS DISTINCT FROM EXCLUDED.comment_parsed)
		RETURNING id`,
		threadID, p.NativeID, ts, p.Name, p.Tripcode, p.Capcode, p.PosterID,
		p.CommentRaw, p.CommentHTML, p.Sage, p.Country, p.Flag, p.OriginalBoard, p.Website, p.OriginalThread, p.OriginalLink).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("store: upsert post %s/%s: %w", p.ThreadID, p.NativeID, err)
	}
	return id, nil
}

func upsertQuotes(ctx context.Context, tx pgx.Tx, postID int64, quotes []adapter.QuoteRef) error {
	if _, err := tx.Exec(ctx, `DELETE FROM post_quotes WHERE post_id=$1`, postID); err != nil {
		return fmt.Errorf("store: clear quotes for post %d: %w", postID, err)
	}
	for _, q := range quotes {
		// board is '' for same-board refs (it is part of the composite PK, so
		// NULL cannot be used).
		if _, err := tx.Exec(ctx, `
			INSERT INTO post_quotes (post_id, quoted_post_native_id, board)
			VALUES ($1, $2, $3)
			ON CONFLICT DO NOTHING`,
			postID, q.PostID, q.Board); err != nil {
			return fmt.Errorf("store: insert quote %s for post %d: %w", q.PostID, postID, err)
		}
	}
	return nil
}

func upsertFile(ctx context.Context, tx pgx.Tx, postID int64, f *adapter.File) error {
	// Bind to an existing blob using any platform-provided hash first.
	var hash any
	if f.MD5 != "" {
		var h string
		err := tx.QueryRow(ctx,
			`SELECT file_hash FROM blobs WHERE platform_md5=$1 LIMIT 1`, f.MD5).Scan(&h)
		if err == nil {
			hash = h
		} else if err != pgx.ErrNoRows {
			return fmt.Errorf("store: lookup blob by md5: %w", err)
		}
	}
	if hash == nil && f.SHA1 != "" {
		var h string
		err := tx.QueryRow(ctx,
			`SELECT file_hash FROM blobs WHERE platform_sha1=$1 LIMIT 1`, f.SHA1).Scan(&h)
		if err == nil {
			hash = h
		} else if err != pgx.ErrNoRows {
			return fmt.Errorf("store: lookup blob by sha1: %w", err)
		}
	}

	var width, height any
	if f.Width > 0 {
		width = f.Width
	}
	if f.Height > 0 {
		height = f.Height
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO files (post_id, file_hash, original_filename, server_filename,
		                   width, height, thumbnail_url, full_url, spoiler)
		VALUES ($1, $2, NULLIF($3,''), NULLIF($4,''), $5, $6, $7, $8, $9)
		ON CONFLICT (post_id, full_url) DO UPDATE
			SET file_hash = COALESCE(EXCLUDED.file_hash, files.file_hash),
			    original_filename = EXCLUDED.original_filename,
			    server_filename = EXCLUDED.server_filename,
			    width = COALESCE(EXCLUDED.width, files.width),
			    height = COALESCE(EXCLUDED.height, files.height),
			    thumbnail_url = EXCLUDED.thumbnail_url,
			    spoiler = EXCLUDED.spoiler`,
		postID, hash, f.OriginalFilename, f.ServerFilename,
		width, height, f.ThumbURL, f.FullURL, f.Spoiler)
	if err != nil {
		return fmt.Errorf("store: upsert file %s for post %d: %w", f.FullURL, postID, err)
	}
	return nil
}
