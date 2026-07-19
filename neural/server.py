"""
vdfusion neural backend — CLIP ViT-B/32 frame embedding service.

POST /embed   — accepts multipart/form-data with image files, returns embeddings
GET  /health  — liveness probe
GET  /info    — model metadata

Runs on ONNX Runtime with CoreML execution provider on Apple Silicon,
falling back to CPU on any other platform.
"""

from __future__ import annotations

import io
import os
import time
from pathlib import Path
from typing import Annotated

import numpy as np
from fastapi import FastAPI, File, UploadFile, HTTPException
from fastapi.responses import JSONResponse
import onnxruntime as ort
from PIL import Image

# ---------------------------------------------------------------------------
# Constants
# ---------------------------------------------------------------------------

MODEL_DIR = Path(os.environ.get("MODEL_DIR", "/models"))
VISUAL_MODEL = MODEL_DIR / "clip-vit-b32-visual_fp16.onnx"
# TEXT_MODEL = MODEL_DIR / "clip-vit-b32-textual.onnx"  # not used at runtime, kept for completeness

# CLIP ViT-B/32 normalisation params
CLIP_MEAN = np.array([0.48145466, 0.4578275, 0.40821073], dtype=np.float32)
CLIP_STD = np.array([0.26862954, 0.26130258, 0.27577711], dtype=np.float32)
CLIP_SIZE = 224  # input resolution expected by ViT-B/32

EMBEDDING_DIM = 512
MODEL_NAME = "clip-vit-b32"
VERSION = "1.0.0"

MAX_BATCH = int(os.environ.get("MAX_BATCH", "32"))

# ---------------------------------------------------------------------------
# ONNX session bootstrap
# ---------------------------------------------------------------------------

def _build_session(model_path: Path) -> ort.InferenceSession:
    """Create an ORT session, preferring CoreML on Apple Silicon."""
    providers: list = []

    available = ort.get_available_providers()
    print(f"[neural] Available providers: {available}")
    if "CoreMLExecutionProvider" in available:
        providers.append("CoreMLExecutionProvider")
    providers.append("CPUExecutionProvider")

    opts = ort.SessionOptions()
    # Disable advanced graph optimizations that caused issues before
    opts.graph_optimization_level = ort.GraphOptimizationLevel.ORT_ENABLE_BASIC
    opts.inter_op_num_threads = int(os.environ.get("ORT_THREADS", "4"))

    try:
        session = ort.InferenceSession(str(model_path), sess_options=opts, providers=providers)
    except Exception as e:
        print(f"[neural] Failed initializing with preferred providers: {e}")
        print("[neural] Falling back to CPU only")
        session = ort.InferenceSession(str(model_path), sess_options=opts, providers=["CPUExecutionProvider"])
    
    active = session.get_providers()
    print(f"[neural] Loaded {model_path.name} | active providers: {active}")
    return session


_session: ort.InferenceSession | None = None
_start_time = time.time()


def get_session() -> ort.InferenceSession:
    global _session
    if _session is None:
        if not VISUAL_MODEL.exists():
            raise RuntimeError(
                f"Model not found at {VISUAL_MODEL}. "
                "Run `python download_model.py` inside the container or mount a model volume."
            )
        _session = _build_session(VISUAL_MODEL)
    return _session


# ---------------------------------------------------------------------------
# Image preprocessing
# ---------------------------------------------------------------------------

def preprocess(image_bytes: bytes) -> np.ndarray:
    """Load and preprocess a single image into a (3, 224, 224) float32 array."""
    img = Image.open(io.BytesIO(image_bytes)).convert("RGB")
    img = img.resize((CLIP_SIZE, CLIP_SIZE), Image.BICUBIC)
    arr = np.array(img, dtype=np.float32) / 255.0          # (H, W, 3)
    arr = (arr - CLIP_MEAN) / CLIP_STD                     # normalise
    arr = arr.transpose(2, 0, 1)                            # (3, H, W)
    return arr


# ---------------------------------------------------------------------------
# Application
# ---------------------------------------------------------------------------

app = FastAPI(
    title="vdfusion neural backend",
    description="CLIP ViT-B/32 video-frame embedding service",
    version=VERSION,
)


@app.on_event("startup")
async def _warmup() -> None:
    """Pre-load the model so the first real request isn't slow."""
    try:
        sess = get_session()
        # Warmup with a blank image — model expects float32 input
        dummy = np.zeros((1, 3, CLIP_SIZE, CLIP_SIZE), dtype=np.float32)
        input_name = sess.get_inputs()[0].name
        sess.run(None, {input_name: dummy})
        print("[neural] Warmup complete.")
    except Exception as exc:
        print(f"[neural] Warmup failed (model may not be present yet): {exc}")


@app.get("/health")
async def health() -> JSONResponse:
    ready = VISUAL_MODEL.exists()
    return JSONResponse(
        content={
            "status": "ok" if ready else "model_missing",
            "model": MODEL_NAME,
            "uptime_seconds": round(time.time() - _start_time, 1),
        },
        status_code=200 if ready else 503,
    )


@app.get("/info")
async def info() -> dict:
    return {
        "model": MODEL_NAME,
        "embedding_dim": EMBEDDING_DIM,
        "input_size": CLIP_SIZE,
        "version": VERSION,
        "providers": ort.get_available_providers(),
    }


@app.post("/embed")
async def embed(
    images: Annotated[list[UploadFile], File(description="JPEG/PNG frames to embed")],
) -> dict:
    """
    Accept up to MAX_BATCH images and return their L2-normalised CLIP embeddings.

    Response:
        {"embeddings": [[f32, ...], ...]}   — one list per input image, same order
    """
    if not images:
        raise HTTPException(status_code=422, detail="No images provided")
    if len(images) > MAX_BATCH:
        raise HTTPException(status_code=422, detail=f"Batch too large (max {MAX_BATCH})")

    sess = get_session()
    input_name = sess.get_inputs()[0].name
    output_name = sess.get_outputs()[0].name

    batch: list[np.ndarray] = []
    for upload in images:
        raw = await upload.read()
        try:
            arr = preprocess(raw)
        except Exception as exc:
            raise HTTPException(status_code=422, detail=f"Bad image ({upload.filename}): {exc}")
        batch.append(arr)

    batch_arr = np.stack(batch, axis=0)  # (N, 3, 224, 224) float32

    try:
        output = sess.run([output_name], {input_name: batch_arr})[0]  # (N, 512)
    except Exception as exc:
        raise HTTPException(status_code=500, detail=f"Inference error: {exc}")

    # L2-normalise each embedding so cosine similarity = dot product
    norms = np.linalg.norm(output, axis=1, keepdims=True)
    norms = np.where(norms == 0, 1.0, norms)
    output = output / norms

    return {"embeddings": output.tolist()}
