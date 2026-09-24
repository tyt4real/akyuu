#!/usr/bin/env python3
"""
train.py - Contrastive fine-tuning for sentence-transformers using mined pairs.

Uses MultipleNegativesRankingLoss for contrastive learning.
Supports multiple model architectures and training configurations.
"""

import argparse
import json
import os
import sys
from pathlib import Path

import torch
from sentence_transformers import SentenceTransformer, InputExample, losses, evaluation
from torch.utils.data import DataLoader


def load_pairs(jsonl_path):
    """Load training pairs from JSONL file."""
    pairs = []
    with open(jsonl_path, "r") as f:
        for line in f:
            data = json.loads(line.strip())
            # Expect format: {"anchor": "...", "positive": "..."}
            if "anchor" in data and "positive" in data:
                pairs.append(InputExample(texts=[data["anchor"], data["positive"]]))
    return pairs


def load_pairs_with_negatives(jsonl_path):
    """Load pairs with explicit negatives."""
    pairs = []
    with open(jsonl_path, "r") as f:
        for line in f:
            data = json.loads(line.strip())
            if "anchor" in data and "positive" in data:
                texts = [data["anchor"], data["positive"]]
                if "negative" in data:
                    texts.append(data["negative"])
                pairs.append(InputExample(texts=texts))
    return pairs


def evaluate_model(model, eval_pairs, batch_size=32):
    """Evaluate model on validation set."""
    if not eval_pairs:
        return {}

    evaluator = evaluation.EmbeddingSimilarityEvaluator.from_input_examples(
        eval_pairs, batch_size=batch_size, name="eval"
    )
    return evaluator(model)


def train_model(
    model_name,
    train_pairs,
    eval_pairs=None,
    output_dir="models/fine_tuned",
    epochs=3,
    batch_size=16,
    learning_rate=2e-5,
    warmup_steps=100,
    max_seq_length=128,
    loss_type="multiple_negatives_ranking",
    eval_steps=100,
    save_steps=100,
):
    """
    Fine-tune a sentence transformer model with contrastive learning.
    """
    print(f"Loading model: {model_name}")
    model = SentenceTransformer(model_name)
    model.max_seq_length = max_seq_length

    # Prepare training data
    if isinstance(train_pairs[0], list):
        # Already InputExample objects
        train_examples = train_pairs
    else:
        # Convert from dict format
        train_examples = []
        for item in train_pairs:
            texts = [item["anchor"], item["positive"]]
            if "negative" in item:
                texts.append(item["negative"])
            train_examples.append(InputExample(texts=texts))

    print(f"Training on {len(train_examples)} examples")

    # Create data loader
    train_dataloader = DataLoader(train_examples, shuffle=True, batch_size=batch_size)

    # Define loss function
    if loss_type == "multiple_negatives_ranking":
        train_loss = losses.MultipleNegativesRankingLoss(model)
    elif loss_type == "cosine_similarity":
        train_loss = losses.CosineSimilarityLoss(model)
    elif loss_type == "contrastive":
        train_loss = losses.ContrastiveLoss(model)
    else:
        raise ValueError(f"Unknown loss type: {loss_type}")

    # Create evaluator if eval pairs provided
    evaluator = None
    if eval_pairs:
        evaluator = evaluation.EmbeddingSimilarityEvaluator.from_input_examples(
            eval_pairs, batch_size=batch_size, name="eval"
        )

    # Train
    model.fit(
        train_objectives=[(train_dataloader, train_loss)],
        evaluator=evaluator,
        epochs=epochs,
        warmup_steps=warmup_steps,
        optimizer_params={"lr": learning_rate},
        evaluation_steps=eval_steps,
        save_best_model=True,
        output_path=output_dir,
        save_steps=save_steps,
    )

    return model


def main():
    parser = argparse.ArgumentParser(description="Contrastive fine-tuning for sentence transformers")
    parser.add_argument("--model", default="sentence-transformers/all-MiniLM-L6-v2", help="Base model name or path")
    parser.add_argument("--train-data", required=True, help="Path to training pairs JSONL")
    parser.add_argument("--eval-data", help="Path to evaluation pairs JSONL")
    parser.add_argument("--output-dir", default="models/fine_tuned", help="Output directory")
    parser.add_argument("--epochs", type=int, default=3, help="Number of epochs")
    parser.add_argument("--batch-size", type=int, default=16, help="Batch size")
    parser.add_argument("--learning-rate", type=float, default=2e-5, help="Learning rate")
    parser.add_argument("--warmup-steps", type=int, default=100, help="Warmup steps")
    parser.add_argument("--max-seq-length", type=int, default=128, help="Max sequence length")
    parser.add_argument("--loss", default="multiple_negatives_ranking",
                        choices=["multiple_negatives_ranking", "cosine_similarity", "contrastive"],
                        help="Loss function type")
    parser.add_argument("--eval-steps", type=int, default=100, help="Evaluation steps")
    parser.add_argument("--save-steps", type=int, default=100, help="Save steps")
    parser.add_argument("--device", default="cuda" if torch.cuda.is_available() else "cpu", help="Device")

    args = parser.parse_args()

    device = torch.device(args.device)
    print(f"Using device: {device}")

    # Load training data
    print(f"Loading training data from {args.train_data}...")
    train_pairs = []
    with open(args.train_data, "r") as f:
        for line in f:
            data = json.loads(line.strip())
            if "anchor" in data and "positive" in data:
                texts = [data["anchor"], data["positive"]]
                if "negative" in data:
                    train_pairs.append(InputExample(texts=[data["anchor"], data["positive"], data["negative"]]))
                else:
                    train_pairs.append(InputExample(texts=[data["anchor"], data["positive"]]))

    print(f"Loaded {len(train_pairs)} training examples")

    # Load eval data if provided
    eval_pairs = None
    if args.eval_data:
        eval_pairs = []
        with open(args.eval_data, "r") as f:
            for line in f:
                data = json.loads(line.strip())
                if "anchor" in data and "positive" in data:
                    texts = [data["anchor"], data["positive"]]
                    if "negative" in data:
                        texts.append(data["negative"])
                    eval_pairs.append(InputExample(texts=texts))
        print(f"Loaded {len(eval_pairs)} evaluation examples")

    # Train
    model = train_model(
        model_name=args.model,
        train_pairs=train_pairs,
        eval_pairs=eval_pairs,
        output_dir=args.output_dir,
        epochs=args.epochs,
        batch_size=args.batch_size,
        learning_rate=args.learning_rate,
        warmup_steps=args.warmup_steps,
        max_seq_length=args.max_seq_length,
        loss_type=args.loss,
        eval_steps=args.eval_steps,
        save_steps=args.save_steps,
    )

    print(f"Training complete. Model saved to {args.output_dir}")


if __name__ == "__main__":
    main()