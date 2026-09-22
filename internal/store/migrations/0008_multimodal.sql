-- 0008_multimodal.sql
-- Multimodal search: OCR, CLIP image embeddings, Whisper transcripts.

-- OCR extracted text from images
CREATE TABLE IF NOT EXISTS post_ocr (
    post_id         BIGINT PRIMARY KEY REFERENCES posts(id) ON DELETE CASCADE,
    text            TEXT NOT NULL,                    -- raw OCR output
    text_cleaned    TEXT,                             -- cleaned for indexing
    language        TEXT,                             -- detected language (tesseract)
    confidence      REAL,                             -- mean confidence 0-100
    engine          TEXT NOT NULL DEFAULT 'tesseract',
    engine_version  TEXT,                             -- e.g. '5.3.0'
    model           TEXT,                             -- e.g. 'eng+jpn+chi_sim'
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Full-text search on OCR text (separate from post body for weighting)
ALTER TABLE post_ocr ADD COLUMN IF NOT EXISTS text_tsv tsvector
    GENERATED ALWAYS AS (to_tsvector('simple', coalesce(text_cleaned, text))) STORED;
CREATE INDEX IF NOT EXISTS idx_post_ocr_text_tsv ON post_ocr USING GIN (text_tsv);

-- CLIP image embeddings for semantic visual search
CREATE TABLE IF NOT EXISTS post_clip_embedding (
    post_id         BIGINT PRIMARY KEY REFERENCES posts(id) ON DELETE CASCADE,
    file_id         BIGINT NOT NULL REFERENCES files(id) ON DELETE CASCADE,  -- which file was embedded
    embedding       VECTOR(384) NOT NULL,           -- CLIP embedding (MobileCLIP-S1/B16 = 384-d)
    model_version   TEXT NOT NULL,                  -- e.g. 'mobileclip-s1@1.0'
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- HNSW index for fast ANN search (requires pgvector 0.7+)
CREATE INDEX IF NOT EXISTS idx_post_clip_embedding_hnsw
    ON post_clip_embedding USING hnsw (embedding vector_cosine_ops)
    WITH (m = 16, ef_construction = 64);

-- Whisper audio/video transcripts
CREATE TABLE IF NOT EXISTS post_transcript (
    post_id         BIGINT PRIMARY KEY REFERENCES posts(id) ON DELETE CASCADE,
    file_id         BIGINT NOT NULL REFERENCES files(id) ON DELETE CASCADE,
    text            TEXT NOT NULL,                    -- full transcript
    language        TEXT,                             -- detected language
    duration_sec    REAL,                             -- audio/video duration
    segments        JSONB,                            -- [{"start":0.0,"end":2.5,"text":"..."}, ...]
    engine          TEXT NOT NULL DEFAULT 'whisper.cpp',
    model           TEXT,                             -- e.g. 'base.en', 'tiny.en'
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- FTS on transcript text
ALTER TABLE post_transcript ADD COLUMN IF NOT EXISTS text_tsv tsvector
    GENERATED ALWAYS AS (to_tsvector('simple', text)) STORED;
CREATE INDEX IF NOT EXISTS idx_post_transcript_text_tsv ON post_transcript USING GIN (text_tsv);

-- Multimodal job tracking (for scheduler)
-- Job kinds: 'ocr', 'clip', 'whisper' - added to jobs table via existing kind enum
-- Payload examples:
--   OCR:     {"file_id": 123, "post_id": 456, "mime": "image/jpeg"}
--   CLIP:    {"file_id": 123, "post_id": 456, "mime": "image/jpeg"}
--   Whisper: {"file_id": 123, "post_id": 456, "mime": "video/webm", "duration_sec": 30.5}