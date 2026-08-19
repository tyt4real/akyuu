package downloader

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func hashOf(t *testing.T, data []byte) string {
	t.Helper()
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

func TestBlocklistChecker(t *testing.T) {
	h := hashOf(t, []byte("evil content"))
	path := filepath.Join(t.TempDir(), "hashes.txt")
	if err := os.WriteFile(path, []byte("# comment\n"+h+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := NewBlocklistChecker(path, "", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	matched, err := c.Check(context.Background(), h)
	if err != nil || !matched {
		t.Errorf("listed hash should match (matched=%v err=%v)", matched, err)
	}
	matched, err = c.Check(context.Background(), hashOf(t, []byte("clean")))
	if err != nil || matched {
		t.Errorf("unlisted hash should not match (matched=%v err=%v)", matched, err)
	}
}

func TestBlocklistCheckerMissingFile(t *testing.T) {
	if _, err := NewBlocklistChecker(filepath.Join(t.TempDir(), "nope.txt"), "", "", nil); err == nil {
		t.Error("expected error for missing hash list")
	}
}

func TestStoragePaths(t *testing.T) {
	st := NewStorage("/data")
	got := st.FullPath("abcdef0123456789", "image/jpeg")
	if got != "/data/full/ab/cd/abcdef0123456789.jpg" {
		t.Errorf("full path = %q", got)
	}
	got = st.ThumbPath("abcdef0123456789")
	if got != "/data/thumb/ab/cd/abcdef0123456789.jpg" {
		t.Errorf("thumb path = %q", got)
	}
	if rel := st.Rel("/data/full/ab/cd/abcdef0123456789.jpg"); rel != "full/ab/cd/abcdef0123456789.jpg" {
		t.Errorf("rel = %q", rel)
	}
}

func TestExtForMime(t *testing.T) {
	cases := map[string]string{
		"image/jpeg":               ".jpg",
		"image/png":                ".png",
		"image/webp":               ".webp",
		"video/mp4":                ".mp4",
		"application/pdf":          ".pdf",
		"application/octet-stream": ".bin",
	}
	for mime, want := range cases {
		if got := extForMime(mime); got != want {
			t.Errorf("extForMime(%q) = %q, want %q", mime, got, want)
		}
	}
}

func TestMimeFromName(t *testing.T) {
	if got := MimeFromName("photo.JPG"); got != "image/jpeg" {
		t.Errorf("MimeFromName = %q", got)
	}
	if got := MimeFromName("file.png"); got != "image/png" {
		t.Errorf("MimeFromName = %q", got)
	}
}
