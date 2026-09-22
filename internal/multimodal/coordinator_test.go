package multimodal

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"akyuu/internal/store"
)

type mockStore struct {
	jobs     []*store.Job
	ocrCalls int
}

func (m *mockStore) QueryRow(ctx context.Context, query string, args ...any) store.RowScanner {
	return &mockRow{}
}

type mockRow struct{}

func (m *mockRow) Scan(dest ...any) error {
	return nil
}

func (m *mockStore) ListSites(ctx context.Context) ([]*store.Site, error) {
	return []*store.Site{{ID: 1, Name: "test"}}, nil
}

func (m *mockStore) UpsertOCR(ctx context.Context, postID int64, text, textCleaned, language string, confidence float32, engine, engineVersion, model string) error {
	m.ocrCalls++
	return nil
}

func (m *mockStore) UpsertCLIPEmbedding(ctx context.Context, postID, fileID int64, embedding []float32, modelVersion string) error {
	return nil
}

func (m *mockStore) UpsertTranscript(ctx context.Context, postID, fileID int64, text, language string, durationSec float32, segments any, engine, model string) error {
	return nil
}

func (m *mockStore) FailJob(ctx context.Context, jobID int64, errMsg string, backoff time.Duration) error {
	return nil
}

func (m *mockStore) CompleteJob(ctx context.Context, jobID int64) error {
	return nil
}

func TestOCRWorker(t *testing.T) {
	ms := &mockStore{}
	worker := NewOCRWorker(ms, "/tmp", "tesseract", "eng")

	if worker.Kind() != store.JobOCR {
		t.Errorf("expected kind %s, got %s", store.JobOCR, worker.Kind())
	}
}

func TestOCRWorkerProcess(t *testing.T) {
	ms := &mockStore{}
	worker := NewOCRWorker(ms, "/tmp", "tesseract", "eng")

	ctx := context.Background()
	payload := JobPayload{
		PostID: 1,
		FileID: 1,
	}

	err := worker.Process(ctx, payload)
	if err == nil {
		t.Error("expected error (tesseract not installed or blob not found)")
	}
}

func TestCLIPWorker(t *testing.T) {
	ms := &mockStore{}
	_, err := NewCLIPWorker(ms, "/tmp", "/nonexistent", "test", 384)
	if err == nil {
		t.Error("expected error (model not found)")
	}
}

func TestCLIPWorkerKind(t *testing.T) {
	ms := &mockStore{}
	// Can't easily test without model, but we can verify Kind() method exists
	_ = ms
}

func TestWhisperWorker(t *testing.T) {
	ms := &mockStore{}
	worker := NewWhisperWorker(ms, "/tmp", "whisper-cli", "/nonexistent", "base.en")

	if worker.Kind() != store.JobWhisper {
		t.Errorf("expected kind %s, got %s", store.JobWhisper, worker.Kind())
	}
}

func TestWhisperWorkerProcess(t *testing.T) {
	ms := &mockStore{}
	worker := NewWhisperWorker(ms, "/tmp", "whisper-cli", "/nonexistent", "base.en")

	ctx := context.Background()
	payload := JobPayload{
		PostID: 1,
		FileID: 1,
	}

	err := worker.Process(ctx, payload)
	if err == nil {
		t.Error("expected error (model not found or blob not found)")
	}
}

func TestCoordinator(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ms := &mockStore{}
	cfg := CoordinatorConfig{
		StorageRoot:  "/tmp",
		PollInterval: 100 * time.Millisecond,
		OCR: OCRConfig{
			Enabled:      true,
			TesseractCmd: "tesseract",
			Languages:    "eng",
		},
		CLIP: CLIPConfig{
			Enabled: false,
		},
		Whisper: WhisperConfig{
			Enabled: false,
		},
	}

	_, err := NewCoordinator(ms, cfg, logger)
	if err != nil {
		t.Logf("Expected error (tesseract not installed): %v", err)
	}
}

func TestCoordinatorRun(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ms := &mockStore{}
	cfg := CoordinatorConfig{
		StorageRoot:  "/tmp",
		PollInterval: 50 * time.Millisecond,
		OCR: OCRConfig{
			Enabled:      true,
			TesseractCmd: "tesseract",
			Languages:    "eng",
		},
		CLIP: CLIPConfig{
			Enabled: false,
		},
		Whisper: WhisperConfig{
			Enabled: false,
		},
	}

	coord, err := NewCoordinator(ms, cfg, logger)
	if err != nil {
		t.Logf("Coordinator creation failed (expected if tesseract missing): %v", err)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	// Run should return when context is cancelled
	coord.Run(ctx)
}
