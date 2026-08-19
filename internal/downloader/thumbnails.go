package downloader

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"strings"

	_ "image/gif"
	_ "image/png"

	"golang.org/x/image/draw"
	"golang.org/x/image/webp"
)

// thumbMaxDim is the longest edge of generated thumbnails.
const thumbMaxDim = 250

// GenerateThumbnail writes a JPEG thumbnail (max thumbMaxDim on the longest
// edge) for raster image mime types. For anything not worth rendering (PDFs,
// videos, archives, ...) it writes a generic placeholder instead. PDF
// first-page rendering is intentionally a placeholder here: it needs a PDF
// rasterizer dependency that is out of scope for this build.
func GenerateThumbnail(src io.Reader, mimeType string, dst io.Writer) error {
	switch strings.ToLower(mimeType) {
	case "image/jpeg", "image/png", "image/gif", "image/webp":
		img, _, err := image.Decode(src)
		if err != nil {
			return fmt.Errorf("downloader: decode %s for thumbnail: %w", mimeType, err)
		}
		return writeScaledJPEG(img, dst)
	default:
		return writePlaceholderJPEG(dst)
	}
}

func writeScaledJPEG(src image.Image, dst io.Writer) error {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 {
		return fmt.Errorf("downloader: empty image bounds")
	}
	tw, th := w, h
	if tw > th {
		tw = thumbMaxDim
		th = h * thumbMaxDim / w
	} else {
		th = thumbMaxDim
		tw = w * thumbMaxDim / h
	}
	if tw < 1 {
		tw = 1
	}
	if th < 1 {
		th = 1
	}
	out := image.NewRGBA(image.Rect(0, 0, tw, th))
	draw.BiLinear.Scale(out, out.Bounds(), src, b, draw.Over, nil)
	return jpeg.Encode(dst, out, &jpeg.Options{Quality: 82})
}

// writePlaceholderJPEG writes a small neutral tile used for file types we do
// not render (PDF, video, archives, unknown).
func writePlaceholderJPEG(dst io.Writer) error {
	img := image.NewRGBA(image.Rect(0, 0, 128, 128))
	dim := color.RGBA{R: 0x3a, G: 0x3a, B: 0x3a, A: 0xff}
	for y := 0; y < 128; y++ {
		for x := 0; x < 128; x++ {
			img.Set(x, y, dim)
		}
	}
	return jpeg.Encode(dst, img, &jpeg.Options{Quality: 80})
}

// DecodeWebp is a tiny helper so webp thumbnails are testable in isolation.
func DecodeWebp(data []byte) (image.Image, error) {
	return webp.Decode(bytes.NewReader(data))
}
