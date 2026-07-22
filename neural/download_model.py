#!/usr/bin/env python3
"""
Download the CLIP ViT-B/32 visual ONNX model and convert to CoreML (MLProgram format).

Usage (inside container or local venv):
    python download_model.py [--output-dir /models] [--batch-size 32]

The visual encoder is exported from openai/clip-vit-base-patch32 via
the clip-as-service / optimum-onnx pipeline and published to HuggingFace Hub.
We fetch the pre-exported file directly from the community model:
  Xenova/clip-vit-base-patch32 (ONNX export from @xenova/transformers)

The ONNX model is then converted to CoreML MLProgram format for optimal
Apple Neural Engine (ANE) support and performance on Apple Silicon.
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

ONNX_FILENAME = "clip-vit-b32-visual_fp16.onnx"


def _sha256(path: Path) -> str:
    h = hashlib.sha256()
    with open(path, "rb") as f:
        for chunk in iter(lambda: f.read(1 << 20), b""):
            h.update(chunk)
    return h.hexdigest()


def _convert_to_coreml(onnx_path: Path, output_dir: Path, batch_size: int) -> Path:
    """Convert ONNX model to CoreML MLProgram format for ANE optimization."""
    try:
        from onnx2coreml import convert
        import onnx
    except ImportError as e:
        print(f"[convert] onnx2coreml not installed: {e}")
        print("[convert] Install with: pip install onnx2coreml")
        return onnx_path

    coreml_filename = f"clip-vit-b32-visual_fp16_bs{batch_size}.mlpackage"
    coreml_path = output_dir / coreml_filename
    mlmodelc_filename = f"clip-vit-b32-visual_fp16_bs{batch_size}.mlmodelc"
    mlmodelc_path = output_dir / mlmodelc_filename

    if coreml_path.exists():
        if not mlmodelc_path.exists():
            print(f"[convert] Pre-compiled CoreML model already present at {mlmodelc_path}, skipping.")
            return coreml_path
        else:
            print(f"[convert] Pre-compiling existing CoreML model to {mlmodelc_path.name}…")
            try:
                from coremltools.models.utils import compile_model
                compile_model(str(coreml_path), destination_path=str(mlmodelc_path))
                print(f"[convert] Pre-compiled model ready at {mlmodelc_path}")
            except Exception as e:
                    print(f"[convert] Pre-compilation failed: {e}")
        return coreml_path

    print(f"[convert] Converting ONNX to CoreML MLProgram format with batch size {batch_size}…")
    print(f"          Source : {onnx_path}")
    print(f"          Dest   : {coreml_path}")

    try:
        # Load ONNX model (keep original dynamic batch dimension for ONNX Runtime)
        onnx_model = onnx.load(str(onnx_path))
        
        # Create a copy of the ONNX model for CoreML conversion (only fix batch size here)
        coreml_onnx_model = onnx.load(str(onnx_path))

        for input_node in coreml_onnx_model.graph.input:
            # Replace dynamic dimensions with static ones for CoreML only
            for i, dim in enumerate(input_node.type.tensor_type.shape.dim):
                if dim.dim_value == 0 or dim.dim_param:  # dynamic dimension
                    dim.ClearField('dim_param')
                    if i == 0:
                        dim.dim_value = batch_size  # batch size (for CoreML)
                    elif i == 1:
                        dim.dim_value = 3  # channels
                    elif i == 2:
                        dim.dim_value = 224  # height
                    elif i == 3:
                        dim.dim_value = 224  # width

        # Convert modified ONNX model (with fixed batch size) to CoreML
        mlmodel = convert(coreml_onnx_model)
        mlmodel.save(str(coreml_path))
        print(f"[convert] Done — {coreml_path.stat().st_size / 1e6:.1f} MB")

        # Pre-compile .mlpackage -> .mlmodelc for instant startup & zero JIT overhead
        print(f"[convert] Pre-compiling CoreML model to {mlmodelc_path.name}…")
        try:
            from coremltools.models.utils import compile_model
            compile_model(str(coreml_path), destination_path=str(mlmodelc_path))
            print(f"[convert] Pre-compiled model ready at {mlmodelc_path}")
        except Exception as e:
            print(f"[convert] Pre-compilation failed: {e}")

        return coreml_path

    except Exception as e:
        print(f"[convert] Conversion failed: {e}")
        print("[convert] Falling back to ONNX model.")
        return onnx_path


def download(output_dir: Path, batch_size: int) -> Path:
    output_dir.mkdir(parents=True, exist_ok=True)
    onnx_dest = output_dir / ONNX_FILENAME

    if not onnx_dest.exists():
        print(f"[download] Downloading CLIP ViT-B/32 visual encoder…")
        print(f"           Source : {MODEL_URL}")
        print(f"           Dest   : {onnx_dest}")

        def _progress(count, block_size, total_size):
            if total_size > 0:
                pct = count * block_size / total_size * 100
                sys.stdout.write(f"\r  {min(pct, 100):.1f}%")
                sys.stdout.flush()

        urlretrieve(MODEL_URL, onnx_dest, reporthook=_progress)
        print()  # newline after progress

        if EXPECTED_SHA256:
            actual = _sha256(onnx_dest)
            if actual != EXPECTED_SHA256:
                onnx_dest.unlink(missing_ok=True)
                raise RuntimeError(
                    f"SHA-256 mismatch!\n  expected: {EXPECTED_SHA256}\n  actual  : {actual}"
                )
            print("[download] Integrity check passed.")

        print(f"[download] Done — {onnx_dest.stat().st_size / 1e6:.1f} MB")
    else:
        print(f"[download] ONNX model already present at {onnx_dest}, skipping.")

    # Convert to CoreML for ANE optimization
    model_path = _convert_to_coreml(onnx_dest, output_dir, batch_size)
    return model_path


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("--output-dir", default="/models", type=Path)
    parser.add_argument("--batch-size", type=int, default=32, help="Compiled batch size for ANE (default: 32)")
    args = parser.parse_args()
    download(args.output_dir, args.batch_size)
