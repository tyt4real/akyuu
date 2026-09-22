package store

import (
	"context"
	"encoding/json"
	"fmt"
)

func (s *Store) UpsertOCR(ctx context.Context, postID int64, text, textCleaned, language string, confidence float32, engine, engineVersion, model string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO post_ocr (post_id, text, text_cleaned, language, confidence, engine, engine_version, model, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, now())
		ON CONFLICT (post_id) DO UPDATE
			SET text = EXCLUDED.text,
			    text_cleaned = EXCLUDED.text_cleaned,
			    language = EXCLUDED.language,
			    confidence = EXCLUDED.confidence,
			    engine = EXCLUDED.engine,
			    engine_version = EXCLUDED.engine_version,
			    model = EXCLUDED.model,
			    updated_at = now()`,
		postID, text, textCleaned, language, confidence, engine, engineVersion, model)
	if err != nil {
		return fmt.Errorf("store: upsert ocr %d: %w", postID, err)
	}
	return nil
}

func (s *Store) UpsertCLIPEmbedding(ctx context.Context, postID, fileID int64, embedding []float32, modelVersion string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("store: begin clip tx: %w", err)
	}
	defer tx.Rollback(ctx)

	vecStr := formatVector(embedding)
	if _, err := tx.Exec(ctx, `
		INSERT INTO post_clip_embedding (post_id, file_id, embedding, model_version)
		VALUES ($1, $2, $3::vector, $4)
		ON CONFLICT (post_id) DO UPDATE
			SET embedding = EXCLUDED.embedding,
			    model_version = EXCLUDED.model_version,
			    created_at = now()`,
		postID, fileID, vecStr, modelVersion); err != nil {
		return fmt.Errorf("store: upsert clip embedding %d: %w", postID, err)
	}
	return tx.Commit(ctx)
}

func (s *Store) UpsertTranscript(ctx context.Context, postID, fileID int64, text, language string, durationSec float32, segments any, engine, model string) error {
	segmentsJSON, err := toJSONB(segments)
	if err != nil {
		return fmt.Errorf("store: marshal segments: %w", err)
	}

	_, err = s.pool.Exec(ctx, `
		INSERT INTO post_transcript (post_id, file_id, text, language, duration_sec, segments, engine, model)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (post_id) DO UPDATE
			SET text = EXCLUDED.text,
			    language = EXCLUDED.language,
			    duration_sec = EXCLUDED.duration_sec,
			    segments = EXCLUDED.segments,
			    engine = EXCLUDED.engine,
			    model = EXCLUDED.model,
			    created_at = now()`,
		postID, fileID, text, language, durationSec, segmentsJSON, engine, model)
	if err != nil {
		return fmt.Errorf("store: upsert transcript %d: %w", postID, err)
	}
	return nil
}

func toJSONB(v any) ([]byte, error) {
	if v == nil {
		return []byte("null"), nil
	}
	return json.Marshal(v)
}
