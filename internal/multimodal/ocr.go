package multimodal

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"akyuu/internal/store"
)

type OCRWorker struct {
	store        StoreAPI
	storageRoot  string
	tesseractCmd string
	languages    string
}

func NewOCRWorker(st StoreAPI, storageRoot, tesseractCmd, languages string) *OCRWorker {
	if tesseractCmd == "" {
		tesseractCmd = "tesseract"
	}
	if languages == "" {
		languages = "eng"
	}
	return &OCRWorker{
		store:        st,
		storageRoot:  storageRoot,
		tesseractCmd: tesseractCmd,
		languages:    languages,
	}
}

func (w *OCRWorker) Kind() string { return store.JobOCR }

func (w *OCRWorker) Process(ctx context.Context, payload JobPayload) error {
	blobData, err := w.getBlobData(ctx, payload.FileID)
	if err != nil {
		return fmt.Errorf("ocr: get blob: %w", err)
	}

	text, confidence, err := w.runTesseract(ctx, blobData)
	if err != nil {
		return fmt.Errorf("ocr: tesseract failed: %w", err)
	}

	textCleaned := cleanOCRText(text)

	if err := w.store.UpsertOCR(ctx, payload.PostID, text, textCleaned, "eng", confidence, "tesseract", "5.3.0", w.languages); err != nil {
		return fmt.Errorf("ocr: store: %w", err)
	}

	return nil
}

func (w *OCRWorker) getBlobData(ctx context.Context, fileID int64) ([]byte, error) {
	var storagePath string
	err := w.store.QueryRow(ctx, `
		SELECT b.storage_path
		FROM files f
		JOIN blobs b ON b.file_hash = f.file_hash
		WHERE f.id = $1 AND f.downloaded_full AND b.storage_path IS NOT NULL`,
		fileID).Scan(&storagePath)
	if err != nil {
		return nil, err
	}
	if storagePath == "" {
		return nil, fmt.Errorf("blob not downloaded")
	}

	fullPath := filepath.Join(w.storageRoot, storagePath)
	data, err := os.ReadFile(fullPath)
	if err != nil {
		return nil, fmt.Errorf("read blob file: %w", err)
	}
	return data, nil
}

func (w *OCRWorker) runTesseract(ctx context.Context, imgData []byte) (string, float32, error) {
	tmpFile, err := os.CreateTemp("", "ocr-*.png")
	if err != nil {
		return "", 0, err
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.Write(imgData); err != nil {
		return "", 0, err
	}
	tmpFile.Close()

	cmd := exec.CommandContext(ctx, w.tesseractCmd, tmpFile.Name(), "stdout", "-l", w.languages, "--psm", "6")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", 0, fmt.Errorf("tesseract exited: %w, stderr: %s", err, stderr.String())
	}

	cmd2 := exec.CommandContext(ctx, w.tesseractCmd, tmpFile.Name(), "stdout", "-l", w.languages, "--psm", "6", "tsv")
	var stdout2 bytes.Buffer
	cmd2.Stdout = &stdout2
	cmd2.Stderr = &stderr
	if err := cmd2.Run(); err != nil {
		return stdout.String(), 0, nil
	}

	confidence := parseTSVConfidence(stdout2.String())
	return stdout.String(), confidence, nil
}

func parseTSVConfidence(tsv string) float32 {
	lines := strings.Split(tsv, "\n")
	if len(lines) < 2 {
		return 0
	}

	var totalConf float64
	var count int

	for i, line := range lines {
		if i == 0 {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) >= 11 {
			if conf, err := strconv.ParseFloat(fields[10], 32); err == nil && conf >= 0 {
				totalConf += conf
				count++
			}
		}
	}

	if count == 0 {
		return 0
	}
	return float32(totalConf / float64(count))
}

func cleanOCRText(text string) string {
	text = regexp.MustCompile(`\s+`).ReplaceAllString(text, " ")
	text = strings.TrimSpace(text)
	return text
}
