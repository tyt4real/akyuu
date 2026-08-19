package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// Blob is one row of the content-addressable blob index.
type Blob struct {
	FileHash         string
	MimeType         string
	SizeBytes        int64
	StoragePath      string
	ThumbStoragePath string
	PlatformMD5      string
	PlatformSHA1     string
	FirstSeenPostID  int64
	RefCount         int
}

// ErrBlobNotFound is returned when a hash has no blob row.
var ErrBlobNotFound = errors.New("store: blob not found")

// GetBlobByHash returns a blob by its content sha256.
func (s *Store) GetBlobByHash(ctx context.Context, hash string) (*Blob, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT file_hash, coalesce(mime_type,''), size_bytes, coalesce(storage_path,''),
		       coalesce(thumb_storage_path,''), coalesce(platform_md5,''), coalesce(platform_sha1,''),
		       first_seen_post_id, ref_count
		FROM blobs WHERE file_hash=$1`, hash)
	return scanBlob(row)
}

// GetBlobByPlatformHash returns a blob keyed by a platform-reported md5 or
// sha1, whichever is present and matches.
func (s *Store) GetBlobByPlatformHash(ctx context.Context, md5, sha1 string) (*Blob, error) {
	if md5 == "" && sha1 == "" {
		return nil, ErrBlobNotFound
	}
	row := s.pool.QueryRow(ctx, `
		SELECT file_hash, coalesce(mime_type,''), size_bytes, coalesce(storage_path,''),
		       coalesce(thumb_storage_path,''), coalesce(platform_md5,''), coalesce(platform_sha1,''),
		       first_seen_post_id, ref_count
		FROM blobs
		WHERE ($1 <> '' AND platform_md5 = $1)
		   OR ($2 <> '' AND platform_sha1 = $2)
		LIMIT 1`, md5, sha1)
	b, err := scanBlob(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrBlobNotFound
	}
	return b, err
}

func scanBlob(r rowScanner) (*Blob, error) {
	var b Blob
	if err := r.Scan(&b.FileHash, &b.MimeType, &b.SizeBytes, &b.StoragePath,
		&b.ThumbStoragePath, &b.PlatformMD5, &b.PlatformSHA1, &b.FirstSeenPostID, &b.RefCount); err != nil {
		return nil, err
	}
	return &b, nil
}

// RecordDownloadedFile binds a file row to a blob and records that the full
// content is stored, all in one transaction. On a blob hit it increments
// ref_count exactly once per file row (guarded by the row's prior download
// state); on a miss it creates the blob with ref_count 1. This is the single
// write path for the content-addressable index, so cross-site dedup and the
// ref_count bookkeeping stay consistent.
func (s *Store) RecordDownloadedFile(ctx context.Context, fileID int64, b *Blob, downloadedFull, downloadedThumb bool) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("store: begin download tx: %w", err)
	}
	defer tx.Rollback(ctx)

	var alreadyFull, alreadyThumb bool
	if err := tx.QueryRow(ctx,
		`SELECT downloaded_full, downloaded_thumb FROM files WHERE id=$1 FOR UPDATE`, fileID).
		Scan(&alreadyFull, &alreadyThumb); err != nil {
		return fmt.Errorf("store: lock file %d: %w", fileID, err)
	}

	inc := downloadedFull && !alreadyFull
	if _, err := tx.Exec(ctx, `
		INSERT INTO blobs (file_hash, mime_type, size_bytes, storage_path,
		                   platform_md5, platform_sha1, first_seen_post_id, ref_count)
		VALUES ($1, $2, $3, NULLIF($4,''), NULLIF($5,''), NULLIF($6,''), NULLIF($7,0), 1)
		ON CONFLICT (file_hash) DO UPDATE
			SET mime_type   = COALESCE(EXCLUDED.mime_type, blobs.mime_type),
			    size_bytes  = GREATEST(COALESCE(EXCLUDED.size_bytes,0), COALESCE(blobs.size_bytes,0)),
			    storage_path = COALESCE(EXCLUDED.storage_path, blobs.storage_path),
			    platform_md5  = COALESCE(EXCLUDED.platform_md5, blobs.platform_md5),
			    platform_sha1 = COALESCE(EXCLUDED.platform_sha1, blobs.platform_sha1),
			    ref_count = blobs.ref_count + CASE WHEN $8 THEN 1 ELSE 0 END`,
		b.FileHash, b.MimeType, b.SizeBytes, b.StoragePath,
		b.PlatformMD5, b.PlatformSHA1, b.FirstSeenPostID, inc); err != nil {
		return fmt.Errorf("store: upsert blob %s: %w", b.FileHash, err)
	}

	if _, err := tx.Exec(ctx, `
		UPDATE files
		SET file_hash = COALESCE($2, file_hash),
		    downloaded_full  = downloaded_full OR $3,
		    downloaded_thumb = downloaded_thumb OR $4
		WHERE id=$1`, fileID, b.FileHash, downloadedFull, downloadedThumb); err != nil {
		return fmt.Errorf("store: mark file %d downloaded: %w", fileID, err)
	}
	return tx.Commit(ctx)
}

// RecordThumbDownloaded marks a file row's thumbnail as stored and, when the
// file references a blob, records the thumbnail path on the blob. It never
// changes ref_count: a thumbnail is an enhancement of an existing reference,
// not a new one.
func (s *Store) RecordThumbDownloaded(ctx context.Context, fileID int64, thumbHash, thumbRelPath string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("store: begin thumb tx: %w", err)
	}
	defer tx.Rollback(ctx)

	var fileHash string
	if err := tx.QueryRow(ctx,
		`SELECT coalesce(file_hash,'') FROM files WHERE id=$1 FOR UPDATE`, fileID).
		Scan(&fileHash); err != nil {
		return fmt.Errorf("store: lock file %d: %w", fileID, err)
	}
	if _, err := tx.Exec(ctx,
		`UPDATE files SET downloaded_thumb = TRUE WHERE id=$1`, fileID); err != nil {
		return fmt.Errorf("store: mark file %d thumb: %w", fileID, err)
	}
	if fileHash != "" {
		if _, err := tx.Exec(ctx,
			`UPDATE blobs SET thumb_storage_path = COALESCE(thumb_storage_path, $2)
			 WHERE file_hash=$1`, fileHash, thumbRelPath); err != nil {
			return fmt.Errorf("store: set blob thumb %s: %w", fileHash, err)
		}
	}
	return tx.Commit(ctx)
}

// ListPendingDownloads returns file rows of a thread that still need their
// content fetched, with the site's download policy already resolved into the
// per-file booleans.
func (s *Store) ListPendingDownloads(ctx context.Context, threadID int64) ([]*PendingDownload, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT f.id, f.post_id, f.full_url, coalesce(f.thumbnail_url,''),
		       coalesce(b.platform_md5,''), coalesce(b.platform_sha1,''), coalesce(f.file_hash,'')
		FROM files f
		LEFT JOIN blobs b ON b.file_hash = f.file_hash
		WHERE f.post_id IN (SELECT id FROM posts WHERE thread_id=$1)
		  AND (NOT f.downloaded_full OR NOT f.downloaded_thumb)
		ORDER BY f.id`, threadID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*PendingDownload
	for rows.Next() {
		var pd PendingDownload
		if err := rows.Scan(&pd.FileID, &pd.PostID, &pd.FullURL, &pd.ThumbURL,
			&pd.PlatformMD5, &pd.PlatformSHA1, &pd.FileHash); err != nil {
			return nil, err
		}
		out = append(out, &pd)
	}
	return out, rows.Err()
}

// ListBoardPendingDownloads returns file rows across a board that still need
// content fetched, up to limit. Used by backfill jobs.
func (s *Store) ListBoardPendingDownloads(ctx context.Context, boardID int64, limit int) ([]*PendingDownload, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT f.id, f.post_id, f.full_url, coalesce(f.thumbnail_url,''),
		       coalesce(b.platform_md5,''), coalesce(b.platform_sha1,''), coalesce(f.file_hash,'')
		FROM files f
		LEFT JOIN blobs b ON b.file_hash = f.file_hash
		WHERE f.post_id IN (
			SELECT p.id FROM posts p JOIN threads t ON t.id = p.thread_id WHERE t.board_id=$1
		)
		  AND (NOT f.downloaded_full OR NOT f.downloaded_thumb)
		ORDER BY f.id
		LIMIT $2`, boardID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*PendingDownload
	for rows.Next() {
		var pd PendingDownload
		if err := rows.Scan(&pd.FileID, &pd.PostID, &pd.FullURL, &pd.ThumbURL,
			&pd.PlatformMD5, &pd.PlatformSHA1, &pd.FileHash); err != nil {
			return nil, err
		}
		out = append(out, &pd)
	}
	return out, rows.Err()
}

// BlobCount returns the number of stored blobs (used by tests).
func (s *Store) BlobCount(ctx context.Context) (int, error) {
	var n int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM blobs`).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}
