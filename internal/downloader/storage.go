package downloader

import (
	"mime"
	"os"
	"path/filepath"
	"strings"
)

// Storage maps content hashes to hash-prefixed paths under a root directory:
//
//	storage/full/<hash[0:2]>/<hash[2:4]>/<hash><ext>
//	storage/thumb/<hash[0:2]>/<hash[2:4]>/<hash>.jpg
//
// The layout dedupes by construction (same content -> same path) and avoids a
// single flat directory.
type Storage struct {
	root string
}

// NewStorage builds a Storage rooted at root.
func NewStorage(root string) *Storage {
	return &Storage{root: root}
}

// FullPath returns the on-disk path for full content with the given hash and
// mime type.
func (s *Storage) FullPath(hash, mimeType string) string {
	return filepath.Join(s.root, "full", s.dir(hash), hash+extForMime(mimeType))
}

// ThumbPath returns the on-disk path for a thumbnail with the given hash.
func (s *Storage) ThumbPath(hash string) string {
	return filepath.Join(s.root, "thumb", s.dir(hash), hash+".jpg")
}

func (s *Storage) dir(hash string) string {
	if len(hash) < 4 {
		return "xx"
	}
	return filepath.Join(hash[0:2], hash[2:4])
}

// EnsureDirs creates the parent directory for a path.
func (s *Storage) EnsureDirs(path string) error {
	return os.MkdirAll(filepath.Dir(path), 0o755)
}

// Rel makes a storage path relative to the root (stored in blobs.storage_path).
func (s *Storage) Rel(abs string) string {
	rel, err := filepath.Rel(s.root, abs)
	if err != nil {
		return abs
	}
	return filepath.ToSlash(rel)
}

// extForMime returns the canonical file extension for a mime type. Unknown
// types fall back to .bin.
func extForMime(mimeType string) string {
	switch strings.ToLower(mimeType) {
	case "image/jpeg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	case "image/avif":
		return ".avif"
	case "video/webm":
		return ".webm"
	case "video/mp4":
		return ".mp4"
	case "video/quicktime":
		return ".mov"
	case "audio/mpeg":
		return ".mp3"
	case "application/pdf":
		return ".pdf"
	case "text/plain":
		return ".txt"
	case "application/zip":
		return ".zip"
	case "application/x-7z-compressed":
		return ".7z"
	default:
		// Last resort: use the extension the platform gave us, if any.
		return ".bin"
	}
}

// MimeFromName guesses a mime type from a file extension (used before bytes
// are sniffed).
func MimeFromName(name string) string {
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".webm":
		return "video/webm"
	case ".mp4":
		return "video/mp4"
	case ".pdf":
		return "application/pdf"
	case ".mp3":
		return "audio/mpeg"
	}
	return mime.TypeByExtension(ext)
}
