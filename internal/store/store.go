// Package store is the Postgres access layer for the archiver: schema
// migration, site/board/thread/post persistence, the content-addressable blob
// index, and the Postgres-backed job queue.
package store

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"log/slog"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Store wraps the pgx connection pool.
type Store struct {
	pool   *pgxpool.Pool
	logger *slog.Logger
}

// New opens a pool and returns a Store. Connect + Ping are performed here so
// misconfiguration fails fast.
func New(ctx context.Context, dsn string, logger *slog.Logger) (*Store, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("store: open pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("store: ping: %w", err)
	}
	return &Store{pool: pool, logger: logger}, nil
}

// Close releases the pool.
func (s *Store) Close() { s.pool.Close() }

// Ping verifies the database connection; used by health checks.
func (s *Store) Ping(ctx context.Context) error {
	if err := s.pool.Ping(ctx); err != nil {
		return fmt.Errorf("store: ping: %w", err)
	}
	return nil
}

// ResetAll truncates every table. It exists for integration tests that run
// against a shared database; it is not used by the running archiver.
func (s *Store) ResetAll(ctx context.Context) error {
	if _, err := s.pool.Exec(ctx,
		`TRUNCATE post_embeddings, files, post_quotes, posts, threads, boards, sites, blobs, jobs RESTART IDENTITY`); err != nil {
		return fmt.Errorf("store: reset all: %w", err)
	}
	return nil
}

// Migrate applies embedded migrations in filename order, tracking applied
// versions in schema_migrations.
func (s *Store) Migrate(ctx context.Context) error {
	if _, err := s.pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version    TEXT PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)`); err != nil {
		return fmt.Errorf("store: create schema_migrations: %w", err)
	}

	names, err := fs.Glob(migrationsFS, "migrations/*.sql")
	if err != nil {
		return err
	}
	sort.Strings(names)
	for _, name := range names {
		version := name[len("migrations/"):]
		var exists bool
		if err := s.pool.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version=$1)`, version).
			Scan(&exists); err != nil {
			return fmt.Errorf("store: check migration %s: %w", version, err)
		}
		if exists {
			continue
		}
		raw, err := migrationsFS.ReadFile(name)
		if err != nil {
			return fmt.Errorf("store: read migration %s: %w", version, err)
		}
		tx, err := s.pool.Begin(ctx)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, string(raw)); err != nil {
			tx.Rollback(ctx)
			return fmt.Errorf("store: apply migration %s: %w", version, err)
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO schema_migrations (version) VALUES ($1)`, version); err != nil {
			tx.Rollback(ctx)
			return fmt.Errorf("store: record migration %s: %w", version, err)
		}
		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("store: commit migration %s: %w", version, err)
		}
		s.logger.Info("applied migration", "version", version)
	}
	return nil
}

// UpsertSite creates or updates a site by name and returns its id.
func (s *Store) UpsertSite(ctx context.Context, name, baseURL, platform string, apiAvailable bool) (int64, error) {
	var id int64
	err := s.pool.QueryRow(ctx, `
		INSERT INTO sites (name, base_url, platform_type, api_available)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (name) DO UPDATE
			SET base_url = EXCLUDED.base_url,
			    platform_type = EXCLUDED.platform_type,
			    api_available = EXCLUDED.api_available
		RETURNING id`, name, baseURL, platform, apiAvailable).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("store: upsert site %s: %w", name, err)
	}
	return id, nil
}

// UpsertBoard creates or updates a board under a site.
func (s *Store) UpsertBoard(ctx context.Context, siteID int64, code, title string, nsfw, worksafe bool) (int64, error) {
	var id int64
	err := s.pool.QueryRow(ctx, `
		INSERT INTO boards (site_id, code, title, nsfw, worksafe)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (site_id, code) DO UPDATE
			SET title = EXCLUDED.title, nsfw = EXCLUDED.nsfw, worksafe = EXCLUDED.worksafe
		RETURNING id`, siteID, code, title, nsfw, worksafe).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("store: upsert board %s: %w", code, err)
	}
	return id, nil
}

// ListSites returns all sites.
func (s *Store) ListSites(ctx context.Context) ([]*Site, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, name, base_url, platform_type, api_available,
		       last_successful_poll, consecutive_failures, coalesce(last_error,''),
		       circuit_open, circuit_until
		FROM sites ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("store: list sites: %w", err)
	}
	defer rows.Close()
	var out []*Site
	for rows.Next() {
		var st Site
		if err := rows.Scan(&st.ID, &st.Name, &st.BaseURL, &st.Platform, &st.APIAvailable,
			&st.LastSuccessfulPoll, &st.ConsecutiveFailures, &st.LastError,
			&st.CircuitOpen, &st.CircuitUntil); err != nil {
			return nil, err
		}
		out = append(out, &st)
	}
	return out, rows.Err()
}

// GetSite returns a single site by name.
func (s *Store) GetSite(ctx context.Context, name string) (*Site, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, name, base_url, platform_type, api_available,
		       last_successful_poll, consecutive_failures, coalesce(last_error,''),
		       circuit_open, circuit_until
		FROM sites WHERE name = $1`, name)
	var st Site
	if err := row.Scan(&st.ID, &st.Name, &st.BaseURL, &st.Platform, &st.APIAvailable,
		&st.LastSuccessfulPoll, &st.ConsecutiveFailures, &st.LastError,
		&st.CircuitOpen, &st.CircuitUntil); err != nil {
		return nil, fmt.Errorf("store: get site %s: %w", name, err)
	}
	return &st, nil
}

// ListBoards returns the boards of a site.
func (s *Store) ListBoards(ctx context.Context, siteID int64) ([]*Board, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, site_id, code, coalesce(title,''), nsfw, worksafe, archive_page
		FROM boards WHERE site_id=$1 ORDER BY code`,
		siteID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Board
	for rows.Next() {
		var b Board
		if err := rows.Scan(&b.ID, &b.SiteID, &b.Code, &b.Title, &b.NSFW, &b.Worksafe, &b.ArchivePage); err != nil {
			return nil, err
		}
		out = append(out, &b)
	}
	return out, rows.Err()
}

// GetBoard resolves a board by site name and board code.
func (s *Store) GetBoard(ctx context.Context, siteName, code string) (*Board, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT b.id, b.site_id, b.code, coalesce(b.title,''), b.nsfw, b.worksafe, b.archive_page
		FROM boards b JOIN sites st ON st.id = b.site_id
		WHERE st.name = $1 AND b.code = $2`, siteName, code)
	var b Board
	if err := row.Scan(&b.ID, &b.SiteID, &b.Code, &b.Title, &b.NSFW, &b.Worksafe, &b.ArchivePage); err != nil {
		return nil, fmt.Errorf("store: get board %s/%s: %w", siteName, code, err)
	}
	return &b, nil
}

// GetBoardByID resolves a board by its primary key.
func (s *Store) GetBoardByID(ctx context.Context, id int64) (*Board, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, site_id, code, coalesce(title,''), nsfw, worksafe, archive_page FROM boards WHERE id=$1`, id)
	var b Board
	if err := row.Scan(&b.ID, &b.SiteID, &b.Code, &b.Title, &b.NSFW, &b.Worksafe, &b.ArchivePage); err != nil {
		return nil, fmt.Errorf("store: get board %d: %w", id, err)
	}
	return &b, nil
}

// SetBoardArchivePage persists the paginated-catalog cursor for a board.
func (s *Store) SetBoardArchivePage(ctx context.Context, boardID int64, page int) error {
	if _, err := s.pool.Exec(ctx,
		`UPDATE boards SET archive_page = $1 WHERE id = $2`, page, boardID); err != nil {
		return fmt.Errorf("store: set archive page %d = %d: %w", boardID, page, err)
	}
	return nil
}

// FindThreadMirror returns the first thread whose native id (4chan post
// number) matches a thread already stored under a different board on any
// fourchan-family site (fourchan and desuarchive, where post numbers are
// globally unique). Returns (nil, nil) when no mirror exists.
func (s *Store) FindThreadMirror(ctx context.Context, boardID int64, nativeID string) (*Thread, error) {
	t, err := s.scanThread(s.pool.QueryRow(ctx, `
		SELECT t.id, t.board_id, t.thread_native_id, coalesce(t.subject,''),
		       t.sticky, t.locked, t.archived, t.status,
	       t.last_bump_time, t.reply_count, t.file_count, t.missing_count, t.last_seen_at
		FROM threads t
		JOIN boards b ON b.id = t.board_id
		JOIN sites st ON st.id = b.site_id
		WHERE t.thread_native_id = $1
		  AND t.board_id <> $2
		  AND st.platform_type IN ('fourchan', 'desuarchive')
		ORDER BY t.id
		LIMIT 1`, nativeID, boardID))
	if err == ErrThreadNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("store: find mirror %s: %w", nativeID, err)
	}
	return t, nil
}

// ListPosts returns posts with optional filtering by site, board, thread native ID, and attachment status.
func (s *Store) ListPosts(ctx context.Context, site, board, nativeID string, hasAttachment *bool) ([]*Post, error) {
	query := `
		SELECT id, thread_id, post_native_id, "timestamp", author_name, tripcode,
		       capcode, poster_id, comment_parsed, sage, country, flag,
	               original_board, website, original_thread_number, original_attachment_link
		FROM posts WHERE 1=1`
	args := []any{}
	conds := []string{}

	if site != "" {
		conds = append(conds, fmt.Sprintf("thread_id IN (SELECT id FROM threads WHERE board_id IN (SELECT id FROM boards WHERE site_id = $%d))", len(args)+1))
		args = append(args, site)
	}
	if board != "" {
		conds = append(conds, fmt.Sprintf("thread_id IN (SELECT id FROM threads WHERE board_id = $%d)", len(args)+1))
		args = append(args, board)
	}
	if nativeID != "" {
		conds = append(conds, fmt.Sprintf("post_native_id = $%d", len(args)+1))
		args = append(args, nativeID)
	}
	if hasAttachment != nil {
		if *hasAttachment {
			conds = append(conds, "comment_parsed IS NOT NULL AND comment_parsed != ''")
		} else {
			conds = append(conds, "comment_parsed IS NULL OR comment_parsed = ''")
		}
	}

	if len(conds) > 0 {
		query += " AND " + strings.Join(conds, " AND ")
	}

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("store: list posts: %w", err)
	}
	defer rows.Close()

	var out []*Post
	for rows.Next() {
		var p Post
		var commentParsed sql.NullString
		if err := rows.Scan(&p.ID, &p.ThreadID, &p.NativeID, &p.Timestamp, &p.AuthorName, &p.Tripcode,
			&p.Capcode, &p.PosterID, &commentParsed, &p.PendingEmbedding,
			&p.Country, &p.Flag, &p.OriginalBoard, &p.Website, &p.OriginalThread, &p.OriginalLink); err != nil {
			return nil, fmt.Errorf("store: scan post: %w", err)
		}
		if commentParsed.Valid {
			p.CommentParsed = commentParsed.String
		}
		out = append(out, &p)
	}
	return out, rows.Err()
}

// Tag returns a tag by name.
func (s *Store) Tag(ctx context.Context, name string) (*Tag, error) {
	row := s.pool.QueryRow(ctx, `SELECT id, name, created_at FROM tags WHERE name = $1`, name)
	t := &Tag{}
	if err := row.Scan(&t.ID, &t.Name, &t.CreatedAt); err != nil {
		return nil, fmt.Errorf("store: tag %q: %w", name, err)
	}
	return t, nil
}

// TagCreate creates a tag, returning an error if it already exists.
func (s *Store) TagCreate(ctx context.Context, name string) (*Tag, error) {
	_, err := s.Tag(ctx, name)
	if err == nil {
		return nil, fmt.Errorf("store: tag %q already exists", name)
	}
	if _, err := s.pool.Exec(ctx, `INSERT INTO tags (name) VALUES ($1)`, name); err != nil {
		return nil, fmt.Errorf("store: create tag %q: %w", name, err)
	}
	return s.Tag(ctx, name)
}

// TagAttachLinks adds tag-to-post associations. Existing links are silently ignored.
func (s *Store) TagAttachLinks(ctx context.Context, tagID int64, postIDs []int64) error {
	if len(postIDs) == 0 {
		return nil
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("store: begin tag tx: %w", err)
	}
	defer tx.Rollback(ctx)
	for _, postID := range postIDs {
		_, err := tx.Exec(ctx, `INSERT INTO taggings (tag_id, post_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`, tagID, postID)
		if err != nil {
			return fmt.Errorf("store: tag attach post %d: %w", postID, err)
		}
	}
	return tx.Commit(ctx)
}

// TagThreadLinks adds tag-to-thread associations. Existing links are silently ignored.
func (s *Store) TagThreadLinks(ctx context.Context, tagID int64, threadIDs []int64) error {
	if len(threadIDs) == 0 {
		return nil
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("store: begin tag tx: %w", err)
	}
	defer tx.Rollback(ctx)
	for _, threadID := range threadIDs {
		_, err := tx.Exec(ctx, `INSERT INTO taggings_threads (tag_id, thread_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`, tagID, threadID)
		if err != nil {
			return fmt.Errorf("store: tag attach thread %d: %w", threadID, err)
		}
	}
	return tx.Commit(ctx)
}
