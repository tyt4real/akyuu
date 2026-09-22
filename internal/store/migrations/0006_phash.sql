-- 0006_phash.sql
-- Perceptual hashing infrastructure for meme lineage tracking.

-- platform_phash stores perceptual hashes (pHash/dHash) for blobs.
-- These enable "find me similar images" across different boards/sites,
-- even when images are recompressed, resized, or have watermarks added.
CREATE TABLE IF NOT EXISTS platform_phash (
    blob_id TEXT NOT NULL REFERENCES blobs(file_hash) ON DELETE CASCADE,
    hash    TEXT NOT NULL,             -- perceptual hash hex string (e.g. 16 hex chars for 64-bit pHash)
    algo    TEXT NOT NULL,           -- "phash" or "dhash"
    width   INTEGER,                 -- image width at time of hash computation
    height  INTEGER,                 -- image height at time of hash computation
    UNIQUE (blob_id, algo)
);

-- Index for fast lookups
CREATE INDEX IF NOT EXISTS idx_phash_blob ON platform_phash (blob_id);
CREATE INDEX IF NOT EXISTS idx_phash_algo ON platform_phash (algo);

-- Add platform_phash column to blobs table (nullable, default NULL)
ALTER TABLE blobs ADD COLUMN IF NOT EXISTS platform_phash TEXT;
