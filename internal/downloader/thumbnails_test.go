package downloader

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateThumbnailFromPNG(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 1000, 500))
	for y := 0; y < 500; y++ {
		for x := 0; x < 1000; x++ {
			src.Set(x, y, color.RGBA{R: 200, G: 30, B: 30, A: 255})
		}
	}
	var pngBytes bytes.Buffer
	if err := png.Encode(&pngBytes, src); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := GenerateThumbnail(bytes.NewReader(pngBytes.Bytes()), "image/png", &out); err != nil {
		t.Fatal(err)
	}
	img, _, err := image.Decode(bytes.NewReader(out.Bytes()))
	if err != nil {
		t.Fatalf("output not a decodable image: %v", err)
	}
	b := img.Bounds()
	if b.Dx() > thumbMaxDim || b.Dy() > thumbMaxDim {
		t.Errorf("thumb %dx%d exceeds %d", b.Dx(), b.Dy(), thumbMaxDim)
	}
	if b.Dx() != 250 || b.Dy() != 125 { // aspect ratio 2:1 preserved
		t.Errorf("thumb = %dx%d, want 250x125", b.Dx(), b.Dy())
	}
}

func TestGenerateThumbnailPortrait(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 200, 600))
	var pngBytes bytes.Buffer
	if err := png.Encode(&pngBytes, src); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := GenerateThumbnail(bytes.NewReader(pngBytes.Bytes()), "image/png", &out); err != nil {
		t.Fatal(err)
	}
	img, _, err := image.Decode(bytes.NewReader(out.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	b := img.Bounds()
	if b.Dx() != 83 || b.Dy() != 250 {
		t.Errorf("thumb = %dx%d, want 83x250", b.Dx(), b.Dy())
	}
}

func TestGenerateThumbnailJPEG(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 10, 10))
	var pngBytes bytes.Buffer
	if err := png.Encode(&pngBytes, src); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := GenerateThumbnail(bytes.NewReader(pngBytes.Bytes()), "image/jpeg", &out); err != nil {
		t.Fatal(err)
	}
	if out.Len() == 0 {
		t.Error("empty output")
	}
}

func TestGenerateThumbnailNonImageWritesPlaceholder(t *testing.T) {
	var out bytes.Buffer
	if err := GenerateThumbnail(strings.NewReader("not an image"), "application/pdf", &out); err != nil {
		t.Fatal(err)
	}
	if _, _, err := image.Decode(bytes.NewReader(out.Bytes())); err != nil {
		t.Fatalf("placeholder not a decodable jpeg: %v", err)
	}
}

func TestGenerateThumbnailCorruptImage(t *testing.T) {
	if err := GenerateThumbnail(strings.NewReader("garbage bytes"), "image/png", io.Discard); err == nil {
		t.Error("expected decode error for corrupt png")
	}
}

func TestGenerateThumbnailUnknownMimePlaceholder(t *testing.T) {
	var out bytes.Buffer
	if err := GenerateThumbnail(strings.NewReader("x"), "video/webm", &out); err != nil {
		t.Fatal(err)
	}
	if _, _, err := image.Decode(bytes.NewReader(out.Bytes())); err != nil {
		t.Fatal("video placeholder should be a jpeg")
	}
}

func TestDecodeWebp(t *testing.T) {
	// 1x1 webp, hand-built via a known-good encoder is out of scope; here we
	// only assert that DecodeWebp wires through x/image/webp without panicking.
	if _, err := DecodeWebp([]byte("not webp")); err == nil {
		t.Error("expected error for non-webp bytes")
	}
}

func TestSniffMime(t *testing.T) {
	dir := t.TempDir()
	png := filepath.Join(dir, "file")
	if err := os.WriteFile(png, []byte("\x89PNG\r\n\x1a\n..."), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := sniffMime(png, "https://x/img.png"); got != "image/png" {
		t.Errorf("sniffMime png = %q", got)
	}
	// Content sniff is authoritative even when the URL says otherwise.
	if got := sniffMime(png, "https://x/photo.jpg"); got != "image/png" {
		t.Errorf("sniffMime content vs url = %q", got)
	}
	// Undetectable content (no magic marker, non-text) falls back to URL ext.
	unknown := filepath.Join(dir, "blob")
	if err := os.WriteFile(unknown, []byte{0xff, 0x00, 0x01, 0x02, 0xfe}, 0o644); err != nil {
		t.Fatal(err)
	}
	if got := sniffMime(unknown, "https://x/photo.jpg"); got != "image/jpeg" {
		t.Errorf("sniffMime fallback = %q", got)
	}
	// Missing file and unknown extension -> octet-stream.
	if got := sniffMime(filepath.Join(dir, "nope"), "https://x/file.zzz"); got != "application/octet-stream" {
		t.Errorf("sniffMime default = %q", got)
	}
}

func TestHashes(t *testing.T) {
	h := newHashes()
	io.WriteString(h.multi(), "hello")
	if h.SHA256Hex() != "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824" {
		t.Errorf("sha256 = %q", h.SHA256Hex())
	}
	if h.MD5Hex() != "5d41402abc4b2a76b9719d911017c592" {
		t.Errorf("md5 = %q", h.MD5Hex())
	}
}
