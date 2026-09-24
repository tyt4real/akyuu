#!/usr/bin/env python3
"""
export_onnx.py - Exports a fine-tuned sentence-transformers model to ONNX format.

Compatible with the akyuu embedder (internal/embedder/onnx.go).
Exports model with tokenizer for use in the Go embedder.
"""

import argparse
import os
import sys
from pathlib import Path

import torch
from sentence_transformers import SentenceTransformer


def export_onnx(model_path, output_dir, model_name=None, opset_version=14):
    """
    Export a sentence-transformers model to ONNX format.
    
    Args:
        model_path: Path to fine-tuned model directory
        output_dir: Output directory for ONNX model
        model_name: Name to use for exported model (defaults to directory name)
        opset_version: ONNX opset version (default 14)
    """
    model_path = Path(model_path)
    output_dir = Path(output_dir)
    output_dir.mkdir(parents=True, exist_ok=True)

    if model_name is None:
        model_name = model_path.name

    print(f"Loading model from {model_path}...")
    model = SentenceTransformer(model_path)

    # Set model to eval mode
    model.eval()

    # Create dummy input for tracing
    dummy_input = ["This is a test sentence for ONNX export."]

    # Export tokenizer
    tokenizer_path = model_path / "tokenizer.json"
    if tokenizer_path.exists():
        import shutil
        shutil.copy2(tokenizer_path, output_dir / "tokenizer.json")
        print(f"Copied tokenizer.json to {output_dir}")

    # Copy config files
    for config_file in ["config.json", "config_sentence_transformers.json"]:
        src = model_path / config_file
        if src.exists():
            import shutil
            shutil.copy2(src, output_dir / config_file)
            print(f"Copied {config_file} to {output_dir}")

    # Export ONNX
    # We need to export the first module (the transformer) since
    # sentence-transformers wraps a transformer + pooling
    print("Exporting to ONNX...")

    # Get the first module (transformer)
    first_module = model._first_module()
    
    # Create dummy inputs for the transformer
    # We need input_ids, attention_mask, and optionally token_type_ids
    dummy_text = "This is a test sentence for ONNX export."
    
    # Tokenize
    tokens = model.tokenize([dummy_text])
    input_ids = tokens["input_ids"]
    attention_mask = tokens["attention_mask"]
    
    # Export the first module (transformer)
    # We need to handle the fact that the first module might be a Transformer
    first_module = model._first_module()
    
    # Export using torch.onnx.export
    # The first module expects input_ids and attention_mask
    class ExportWrapper(torch.nn.Module):
        def __init__(self, module):
            super().__init__()
            self.module = module
            
        def forward(self, input_ids, attention_mask):
            return self.module(input_ids, attention_mask)
    
    wrapper = ExportWrapper(first_module.auto_model)
    
    dummy_input_ids = torch.randint(0, 1000, (1, 128), dtype=torch.long)
    dummy_attention_mask = torch.ones((1, 128), dtype=torch.long)
    
    output_path = output_dir / "model.onnx"
    
    print(f"Exporting to {output_path}...")
    
    torch.onnx.export(
        first_module.auto_model,
        (torch.randint(0, 1000, (1, 128), dtype=torch.long),
         torch.ones((1, 128), dtype=torch.long)),
        output_path,
        export_params=True,
        opset_version=14,
        do_constant_folding=True,
        input_names=["input_ids", "attention_mask"],
        output_names=["last_hidden_state"],
        dynamic_axes={
            "input_ids": {0: "batch_size", 1: "sequence"},
            "attention_mask": {0: "batch_size", 1: "sequence"},
            "last_hidden_state": {0: "batch_size", 1: "sequence"},
        },
    )
    
    print(f"Model exported to {output_path}")
    
    # Verify the export
    import onnx
    onnx_model = onnx.load(output_path)
    onnx.checker.check_model(onnx_model)
    print("ONNX model verified successfully")
    
    print(f"\nExport complete! Files in {output_dir}:")
    for f in output_dir.iterdir():
        print(f"  {f.name}")


def main():
    parser = argparse.ArgumentParser(description="Export sentence-transformers model to ONNX")
    parser.add_argument("--model", required=True, help="Path to fine-tuned model directory")
    parser.add_argument("--output", required=True, help="Output directory for ONNX model")
    parser.add_argument("--opset", type=int, default=14, help="ONNX opset version")
    
    args = parser.parse_args()
    
    try:
        export_onnx(args.model, args.output)
    except Exception as e:
        print(f"Export failed: {e}", file=sys.stderr)
        sys.exit(1)


if __name__ == "__main__":
    main()