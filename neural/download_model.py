#!/usr/bin/env python3
"""
Download the CLIP ViT-B/32 visual ONNX model into the /models directory.

Usage (inside container or local venv):
    python download_model.py [--output-dir /models]

The visual encoder is exported from openai/clip-vit-base-patch32 via
the clip-as-service / optimum-onnx pipeline and published to HuggingFace Hub.
We fetch the pre-exported file directly from the community model:
  Xenova/clip-vit-base-patch32 (ONNX export from @xenova/transformers)
"""

import argparse
import hashlib
import sys
from pathlib import Path
from urllib.request import urlretrieve

# Pre-exported ONNX visual encoder from Xenova (MIT licence)
MODEL_URL = (
    "https://huggingface.co/Xenova/clip-vit-base-patch32/resolve/main/onnx/vision_model_fp16.onnx"
)
# sha256 of the canonical Xenova export — update if the upstream file changes
EXPECTED_SHA256 = None  # set to None to skip integrity check (safe for dev)

OUTPUT_FILENAME = "clip-vit-b32-visual_fp16.onnx"


def _sha256(path: Path) -> str:
    h = hashlib.sha256()
    with open(path, "rb") as f:
        for chunk in iter(lambda: f.read(1 << 20), b""):
            h.update(chunk)
    return h.hexdigest()


def download(output_dir: Path) -> Path:
    output_dir.mkdir(parents=True, exist_ok=True)
    dest = output_dir / OUTPUT_FILENAME

    if dest.exists():
        print(f"[download] Model already present at {dest}, skipping.")
        return dest

    print(f"[download] Downloading CLIP ViT-B/32 visual encoder…")
    print(f"           Source : {MODEL_URL}")
    print(f"           Dest   : {dest}")

    def _progress(count, block_size, total_size):
        if total_size > 0:
            pct = count * block_size / total_size * 100
            sys.stdout.write(f"\r  {min(pct, 100):.1f}%")
            sys.stdout.flush()

    urlretrieve(MODEL_URL, dest, reporthook=_progress)
    print()  # newline after progress

    if EXPECTED_SHA256:
        actual = _sha256(dest)
        if actual != EXPECTED_SHA256:
            dest.unlink(missing_ok=True)
            raise RuntimeError(
                f"SHA-256 mismatch!\n  expected: {EXPECTED_SHA256}\n  actual  : {actual}"
            )
        print("[download] Integrity check passed.")

    print(f"[download] Done — {dest.stat().st_size / 1e6:.1f} MB")
    return dest


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("--output-dir", default="/models", type=Path)
    args = parser.parse_args()
    download(args.output_dir)
