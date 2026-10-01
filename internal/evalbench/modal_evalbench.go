package evalbench

import (
	"context"
	"encoding/csv"
	"fmt"
	"os"
	"time"

	"akyuu/internal/config"

	modal "github.com/modal-labs/modal-client/go"
)

type ModalEvalbench struct {
	client    *modal.Client
	cls       *modal.Cls
	method    *modal.Function
	app       *modal.App
	dataVol   string
	logger    Logger
}

type Logger interface {
	Info(msg string, args ...any)
	Error(msg string, args ...any)
	Debug(msg string, args ...any)
	Warn(msg string, args ...any)
}

func NewModalEvalbench(cfg config.ModalConfig, logger Logger) (*ModalEvalbench, error) {
	mc, err := modal.NewClient()
	if err != nil {
		return nil, fmt.Errorf("create modal client: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Use new SDK API: Cls.FromName -> Instance -> Method
	cls, err := mc.Cls.FromName(ctx, cfg.AppName, "Embedder", nil)
	if err != nil {
		return nil, fmt.Errorf("get modal class %q: %w", "Embedder", err)
	}

	instance, err := cls.Instance(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("get modal class instance: %w", err)
	}

	method, err := instance.Method(cfg.EvalbenchFunction)
	if err != nil {
		return nil, fmt.Errorf("get modal method %q: %w", cfg.EvalbenchFunction, err)
	}

	app, err := mc.Apps.FromName(ctx, cfg.AppName, nil)
	if err != nil {
		return nil, fmt.Errorf("get modal app %q: %w", cfg.AppName, err)
	}

	return &ModalEvalbench{
		client:    mc,
		cls:       cls,
		method:    method,
		app:       app,
		dataVol:   cfg.DataVolume,
		logger:    logger,
	}, nil
}

type ModalEvalbenchResult struct {
	RawCSV     string `json:"raw_csv"`
	SummaryCSV string `json:"summary_csv"`
	Timestamp  int64  `json:"timestamp"`
}

func (m *ModalEvalbench) RunEvalbench(ctx context.Context, queryCases []QueryCase, k int, modelVersion string) (string, string, error) {
	input := map[string]any{
		"k":             k,
		"model_version": modelVersion,
	}

	m.logger.Info("starting modal evalbench", "queries", len(queryCases), "k", k)

	fc, err := m.method.Spawn(ctx, []any{input}, nil)
	if err != nil {
		return "", "", fmt.Errorf("spawn modal evalbench: %w", err)
	}

	m.logger.Info("waiting for modal evalbench to complete", "call_id", fc.FunctionCallID)

	result, err := fc.Get(ctx, nil)
	if err != nil {
		return "", "", fmt.Errorf("modal evalbench failed: %w", err)
	}

	var modalResult ModalEvalbenchResult
	if err := decodeModalResult(result, &modalResult); err != nil {
		return "", "", fmt.Errorf("decode modal result: %w", err)
	}

	m.logger.Info("modal evalbench completed", "raw_csv", modalResult.RawCSV, "summary_csv", modalResult.SummaryCSV)

	rawLocal := "/tmp/modal_raw.csv"
	summaryLocal := "/tmp/modal_summary.csv"

	if err := m.downloadFromVolume(ctx, modalResult.RawCSV, rawLocal); err != nil {
		return "", "", fmt.Errorf("download raw csv: %w", err)
	}
	if err := m.downloadFromVolume(ctx, modalResult.SummaryCSV, summaryLocal); err != nil {
		return "", "", fmt.Errorf("download summary csv: %w", err)
	}

	return rawLocal, summaryLocal, nil
}

func (m *ModalEvalbench) downloadFromVolume(ctx context.Context, remotePath, localPath string) error {
	vol, err := m.client.Volumes.FromName(ctx, m.dataVol, nil)
	if err != nil {
		return fmt.Errorf("get modal volume: %w", err)
	}

	image := m.client.Images.FromRegistry("alpine:3.20", nil)

	sb, err := m.client.Sandboxes.Create(ctx, m.app, image, &modal.SandboxCreateParams{
		Volumes: map[string]*modal.Volume{
			"/data": vol,
		},
		Timeout: 60 * time.Second,
	})
	if err != nil {
		return fmt.Errorf("create sandbox for volume download: %w", err)
	}
	defer sb.Terminate(ctx, nil)

	p, err := sb.Exec(ctx, []string{"cp", "/data/" + remotePath, localPath}, nil)
	if err != nil {
		return fmt.Errorf("exec cp in sandbox: %w", err)
	}

	_, err = p.Wait(ctx, nil)
	return err
}

func decodeModalResult(result any, target *ModalEvalbenchResult) error {
	m, ok := result.(map[string]any)
	if !ok {
		return fmt.Errorf("result is not a map: %T", result)
	}

	if raw, ok := m["raw_csv"].(string); ok {
		target.RawCSV = raw
	}
	if summary, ok := m["summary_csv"].(string); ok {
		target.SummaryCSV = summary
	}
	if ts, ok := m["timestamp"].(float64); ok {
		target.Timestamp = int64(ts)
	}

	return nil
}

func (m *ModalEvalbench) Close() error {
	if m.client != nil {
		m.client.Close()
	}
	return nil
}

func WriteRawCSVFromModal(rawCSVPath, outputPath string, queryCases []QueryCase, k int) error {
	src, err := os.Open(rawCSVPath)
	if err != nil {
		return err
	}
	defer src.Close()

	dst, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer dst.Close()

	reader := csv.NewReader(src)
	writer := csv.NewWriter(dst)
	defer writer.Flush()

	for {
		record, err := reader.Read()
		if err != nil {
			break
		}
		if err := writer.Write(record); err != nil {
			return err
		}
	}
	return nil
}

func WriteSummaryCSVFromModal(summaryCSVPath, outputPath string, queryCases []QueryCase, k int) error {
	src, err := os.Open(summaryCSVPath)
	if err != nil {
		return err
	}
	defer src.Close()

	dst, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer dst.Close()

	reader := csv.NewReader(src)
	writer := csv.NewWriter(dst)
	defer writer.Flush()

	for {
		record, err := reader.Read()
		if err != nil {
			break
		}
		if err := writer.Write(record); err != nil {
			return err
		}
	}
	return nil
}