package downloader

import (
	"bytes"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"log/slog"

	"github.com/corona10/goimagehash"
)

// ComputePHASH computes the perceptual hash (pHash) of an image.
// Returns the hash as a hex string (16 chars for 64-bit hash).
func ComputePHASH(data []byte, logger *slog.Logger) (string, error) {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	hash, err := goimagehash.PerceptionHash(img)
	if err != nil {
		return "", err
	}
	return hash.ToString(), nil
}

// ComputeDHash computes the difference hash (dHash) of an image.
// Returns the hash as a hex string (16 chars for 64-bit hash).
func ComputeDHash(data []byte, logger *slog.Logger) (string, error) {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	hash, err := goimagehash.DifferenceHash(img)
	if err != nil {
		return "", err
	}
	return hash.ToString(), nil
}

// ComputeAHash computes the average hash (aHash) of an image.
// Returns the hash as a hex string (16 chars for 64-bit hash).
func ComputeAHash(data []byte, logger *slog.Logger) (string, error) {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	hash, err := goimagehash.AverageHash(img)
	if err != nil {
		return "", err
	}
	return hash.ToString(), nil
}
