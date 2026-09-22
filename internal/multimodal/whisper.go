package multimodal

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"akyuu/internal/store"
)

type WhisperSegment struct {
	Start float64 `json:"start"`
	End   float64 `json:"end"`
	Text  string  `json:"text"`
}

type WhisperOutput struct {
	Text     string           `json:"text"`
	Language string           `json:"language"`
	Duration float64          `json:"duration"`
	Segments []WhisperSegment `json:"segments"`
}

type WhisperWorker struct {
	store       StoreAPI
	storageRoot string
	whisperCmd  string
	modelPath   string
	modelName   string
}

func NewWhisperWorker(st StoreAPI, storageRoot, whisperCmd, modelPath, modelName string) *WhisperWorker {
	if whisperCmd == "" {
		whisperCmd = "whisper-cli"
	}
	if modelName == "" {
		modelName = "base.en"
	}
	return &WhisperWorker{
		store:       st,
		storageRoot: storageRoot,
		whisperCmd:  whisperCmd,
		modelPath:   modelPath,
		modelName:   modelName,
	}
}

func (w *WhisperWorker) Kind() string { return store.JobWhisper }

func (w *WhisperWorker) Process(ctx context.Context, payload JobPayload) error {
	blobData, err := w.getBlobData(ctx, payload.FileID)
	if err != nil {
		return fmt.Errorf("whisper: get blob: %w", err)
	}

	// Write to temp file
	tmpFile, err := os.CreateTemp("", "whisper-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.Write(blobData); err != nil {
		return err
	}
	tmpFile.Close()

	result, err := w.runWhisper(ctx, tmpFile.Name())
	if err != nil {
		return fmt.Errorf("whisper: run failed: %w", err)
	}

	segmentsJSON, _ := json.Marshal(result.Segments)

	if err := w.store.UpsertTranscript(ctx, payload.PostID, payload.FileID,
		result.Text, result.Language, float32(result.Duration), segmentsJSON,
		"whisper.cpp", w.modelName); err != nil {
		return fmt.Errorf("whisper: store: %w", err)
	}

	return nil
}

func (w *WhisperWorker) getBlobData(ctx context.Context, fileID int64) ([]byte, error) {
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

func (w *WhisperWorker) runWhisper(ctx context.Context, audioPath string) (*WhisperOutput, error) {
	// whisper-cli -m model.bin -f audio.wav -oj
	modelFile := filepath.Join(w.modelPath, "ggml-"+w.modelName+".bin")
	if _, err := os.Stat(modelFile); err != nil {
		return nil, fmt.Errorf("whisper model not found: %s", modelFile)
	}

	cmd := exec.CommandContext(ctx, w.whisperCmd,
		"-m", modelFile,
		"-f", audioPath,
		"-oj",        // JSON output
		"-l", "auto", // auto-detect language
	)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("whisper exited: %w, stderr: %s", err, stderr.String())
	}

	var result WhisperOutput
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		return nil, fmt.Errorf("whisper: parse json: %w", err)
	}

	return &result, nil
}
