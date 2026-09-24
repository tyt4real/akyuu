// Package store provides additional embedding-related methods for canonical texts.
package store

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// parseVector parses a pgvector text representation "[a,b,c]" into []float32
func parseVector(s string) ([]float32, error) {
	s = strings.Trim(s, "[]")
	if s == "" {
		return []float32{}, nil
	}
	parts := strings.Split(s, ",")
	out := make([]float32, len(parts))
	for i, p := range parts {
		f, err := strconv.ParseFloat(strings.TrimSpace(p), 32)
		if err != nil {
			return nil, err
		}
		out[i] = float32(f)
	}
	return out, nil
}

// CanonicalEmbedding looks up a canonical embedding for the given normalized text.
// Returns (embedding, true, nil) if found, (nil, false, nil) if not found, (nil, false, err) on error.
func (s *Store) CanonicalEmbedding(ctx context.Context, normalizedText string) ([]float32, bool, error) {
	var vecStr string
	err := s.pool.QueryRow(ctx, `
		SELECT embedding::text FROM canonical_texts WHERE normalized_text = $1
	`, normalizedText).Scan(&vecStr)
	if err != nil {
		return nil, false, nil
	}
	vec, err := parseVector(vecStr)
	if err != nil {
		return nil, false, fmt.Errorf("store: parse canonical vector: %w", err)
	}
	return vec, true, nil
}

// UpsertCanonicalEmbedding inserts or updates a canonical embedding.
func (s *Store) UpsertCanonicalEmbedding(ctx context.Context, normalizedText string, vec []float32) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO canonical_texts (normalized_text, embedding, ref_count, first_seen_at)
		VALUES ($1, $2::vector, 1, now())
		ON CONFLICT (normalized_text) DO UPDATE
			SET embedding = EXCLUDED.embedding,
			    ref_count = canonical_texts.ref_count + 1,
			    first_seen_at = LEAST(canonical_texts.first_seen_at, EXCLUDED.first_seen_at)
	`, normalizedText, formatVector(vec))
	if err != nil {
		return fmt.Errorf("store: upsert canonical embedding: %w", err)
	}
	return nil
}

// IncrementCanonicalRefCount increments the reference count for a canonical text.
func (s *Store) IncrementCanonicalRefCount(ctx context.Context, normalizedText string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE canonical_texts SET ref_count = ref_count + 1 WHERE normalized_text = $1
	`, normalizedText)
	if err != nil {
		return fmt.Errorf("store: increment canonical ref count: %w", err)
	}
	return nil
}

// RecordCanonicalCandidate increments the seen count for a canonical candidate.
// Returns true if the candidate was promoted to canonical_texts.
func (s *Store) RecordCanonicalCandidate(ctx context.Context, normalizedText string, promoteThreshold int) (bool, error) {
	var seenCount int
	err := s.pool.QueryRow(ctx, `
		INSERT INTO canonical_candidates (normalized_text, seen_count, first_seen_at, last_seen_at)
		VALUES ($1, 1, now(), now())
		ON CONFLICT (normalized_text) DO UPDATE
			SET seen_count = canonical_candidates.seen_count + 1,
			    last_seen_at = now()
		RETURNING seen_count
	`, normalizedText).Scan(&seenCount)
	if err != nil {
		return false, fmt.Errorf("store: record canonical candidate: %w", err)
	}

	if seenCount >= promoteThreshold {
		// Promote to canonical_texts
		_, err := s.pool.Exec(ctx, `
			INSERT INTO canonical_texts (normalized_text, embedding, ref_count, first_seen_at)
			SELECT normalized_text, NULL, seen_count, first_seen_at
			FROM canonical_candidates WHERE normalized_text = $1
			ON CONFLICT (normalized_text) DO UPDATE
				SET ref_count = EXCLUDED.ref_count
		`, normalizedText)
		if err != nil {
			return false, fmt.Errorf("store: promote canonical candidate: %w", err)
		}
		// Clean up candidate
		_, _ = s.pool.Exec(ctx, `DELETE FROM canonical_candidates WHERE normalized_text = $1`, normalizedText)
		return true, nil
	}
	return false, nil
}
