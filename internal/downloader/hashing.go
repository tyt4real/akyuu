package downloader

import (
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"hash"
	"io"
	"time"
)

const httpTimeout = 60 * time.Second

// hashes keeps running sha256 and md5 digests as a file streams through.
type hashes struct {
	sha256 hash.Hash
	md5    hash.Hash
}

func newHashes() *hashes {
	return &hashes{sha256: sha256.New(), md5: md5.New()}
}

// multi returns a writer that feeds both digests.
func (h *hashes) multi() io.Writer {
	return io.MultiWriter(h.sha256, h.md5)
}

func (h *hashes) SHA256Hex() string { return hex.EncodeToString(h.sha256.Sum(nil)) }
func (h *hashes) MD5Hex() string    { return hex.EncodeToString(h.md5.Sum(nil)) }
