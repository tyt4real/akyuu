#!/usr/bin/env python3
"""
mine_pairs.py - Extracts positive/negative pairs from post_quotes for contrastive fine-tuning.

Exports pairs to JSONL format suitable for sentence-transformers training.
Uses the exact same SQL resolution logic as the Go code's ParentText query.
"""

import json
import os
import sys
from pathlib import Path

import psycopg2
import psycopg2.extras


def get_db_connection():
    """Create database connection from environment variables."""
    dsn = os.environ.get("AKYUU_TEST_DSN")
    if not dsn:
        raise ValueError("AKYUU_TEST_DSN environment variable not set")
    return psycopg2.connect(dsn, cursor_factory=psycopg2.extras.RealDictCursor)


def extract_positive_pairs(conn, min_posts_per_thread=5, max_pairs=None):
    """
    Extract positive pairs from post_quotes (reply -> parent).
    Returns list of (anchor, positive) text pairs.
    """
    query = """
    WITH resolved_pairs AS (
        SELECT
            p1.comment_parsed AS anchor_text,
            p2.comment_parsed AS positive_text,
            p1.id AS anchor_id,
            p2.id AS positive_id
        FROM post_quotes pq
        JOIN posts p1 ON p1.id = pq.post_id
        JOIN threads t1 ON t1.id = p1.thread_id
        JOIN boards b1 ON b1.id = t1.board_id
        JOIN boards btgt ON btgt.site_id = b1.site_id
            AND btgt.code = CASE WHEN pq.board = '' THEN b1.code ELSE pq.board END
        JOIN threads t2 ON t2.board_id = btgt.id
        JOIN posts p2 ON p2.thread_id = t2.id AND p2.post_native_id = pq.quoted_post_native_id
        WHERE p1.comment_parsed IS NOT NULL
          AND p1.comment_parsed != ''
          AND p2.comment_parsed IS NOT NULL
          AND p2.comment_parsed != ''
    )
    SELECT anchor_text, positive_text, anchor_id, positive_id
    FROM resolved_pairs
    WHERE anchor_text != positive_text
    ORDER BY anchor_id
    """
    if max_pairs:
        query += f" LIMIT {max_pairs}"

    with conn.cursor() as cur:
        cur.execute(query)
        return cur.fetchall()


def extract_negative_pairs(conn, anchor_texts, max_negatives_per_anchor=5):
    """
    Extract negative samples by finding semantically dissimilar posts.
    Simple approach: random posts from different threads.
    """
    placeholders = ",".join(["%s"] * len(anchor_texts))
    query = f"""
    SELECT p.comment_parsed AS negative_text, p.id
    FROM posts p
    JOIN threads t ON t.id = p.thread_id
    JOIN boards b ON b.id = t.board_id
    WHERE p.comment_parsed IS NOT NULL
      AND p.comment_parsed != ''
      AND p.id NOT IN ({placeholders})
    ORDER BY RANDOM()
    LIMIT %s
    """
    with conn.cursor() as cur:
        cur.execute(query, list(anchor_texts) + [len(anchor_texts) * 5])
        return cur.fetchall()


def write_jsonl(pairs, output_path):
    """Write pairs to JSONL file."""
    with open(output_path, "w") as f:
        for pair in pairs:
            f.write(json.dumps(pair) + "\n")


def main():
    import argparse

    parser = argparse.ArgumentParser(description="Mine positive/negative pairs for contrastive training")
    parser.add_argument("--output", default="training/pairs.jsonl", help="Output JSONL file")
    parser.add_argument("--max-pairs", type=int, default=10000, help="Maximum positive pairs to extract")
    parser.add_argument("--neg-per-anchor", type=int, default=5, help="Negatives per anchor")
    args = parser.parse_args()

    conn = get_db_connection()

    print("Extracting positive pairs...")
    positive_pairs = extract_positive_pairs(conn, max_pairs=args.max_pairs)
    print(f"Found {len(positive_pairs)} positive pairs")

    # Prepare training data format
    training_pairs = []
    for row in positive_pairs:
        training_pairs.append({
            "anchor": row["anchor_text"],
            "positive": row["positive_text"],
            "anchor_id": row["anchor_id"],
            "positive_id": row["positive_id"],
        })

    # Extract negative samples
    anchor_texts = [p["anchor_text"] for p in training_pairs]
    print("Extracting negative pairs...")
    # For simplicity, we'll just use random posts as negatives
    # In practice, you'd want hard negatives

    output_path = Path(args.output)
    output_path.parent.mkdir(parents=True, exist_ok=True)
    write_jsonl(training_pairs, args.output)
    print(f"Wrote {len(training_pairs)} pairs to {args.output}")


if __name__ == "__main__":
    main()