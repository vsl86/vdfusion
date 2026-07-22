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
from concurrent.futures import ThreadPoolExecutor
from pathlib import Path
from typing import Annotated

import numpy as np
from fastapi import FastAPI, File, UploadFile, HTTPException
from fastapi.responses import JSONResponse
import onnxruntime as ort
from PIL import Image

try:
    import coremltools as ct
    COREML_AVAILABLE = True
except ImportError:
    COREML_AVAILABLE = False

# ---------------------------------------------------------------------------
# Constants
# ---------------------------------------------------------------------------

MODEL_DIR = Path(os.environ.get("MODEL_DIR", "/models"))
VISUAL_MODEL_ONNX = MODEL_DIR / "clip-vit-b32-visual_fp16.onnx"
VISUAL_MODEL_COREML = MODEL_DIR / "clip-vit-b32-visual_fp16.mlpackage"

# Prefer MLProgram for ANE when available, fall back to ONNX
if VISUAL_MODEL_COREML.exists() and COREML_AVAILABLE:
    VISUAL_MODEL = VISUAL_MODEL_COREML
    USE_COREML = True
else:
    VISUAL_MODEL = VISUAL_MODEL_ONNX
    USE_COREML = False

# CLIP ViT-B/32 normalisation params
CLIP_MEAN = np.array([0.48145466, 0.4578275, 0.40821073], dtype=np.float32)
CLIP_STD = np.array([0.26862954, 0.26130258, 0.27577711], dtype=np.float32)
CLIP_SIZE = 224  # input resolution expected by ViT-B/32

EMBEDDING_DIM = 512
MODEL_NAME = "clip-vit-b32"
VERSION = "1.0.0"

MAX_BATCH = int(os.environ.get("MAX_BATCH", "32"))

# Parallel preprocessing with thread pool
_preprocess_executor = ThreadPoolExecutor(max_workers=8, thread_name_prefix="preprocess")

# ---------------------------------------------------------------------------
# ONNX session bootstrap
# ---------------------------------------------------------------------------

def _build_session(model_path: Path) -> object:
    """Create an inference session, preferring CoreML MLProgram on Apple Silicon."""
    
    if USE_COREML:
        print(f"[neural] Loading CoreML MLProgram model for ANE optimization…")
        try:
            mlmodel = ct.models.MLModel(str(model_path), compute_units=ct.ComputeUnit.CPU_AND_NE)
            print(f"[neural] Loaded {model_path.name} (CoreML MLProgram) | Compute units: CPU_AND_NE")
            
            # Get input/output names from model spec
            try:
                spec = mlmodel.get_spec()
                # MLProgram models have descriptions with input/output info
                if hasattr(spec, 'description') and spec.description:
                    mlmodel._input_names = [inp.name for inp in spec.description.input]
                    mlmodel._output_names = [out.name for out in spec.description.output]
                    print(f"[neural] Inputs: {mlmodel._input_names}, Outputs: {mlmodel._output_names}")
                else:
                    print(f"[neural] Warning: Could not extract input/output names from spec")
                    mlmodel._input_names = []
                    mlmodel._output_names = []
            except Exception as e:
                print(f"[neural] Could not extract I/O names: {e}")
                mlmodel._input_names = []
                mlmodel._output_names = []
            
            return mlmodel
        except Exception as e:
            print(f"[neural] Failed to load CoreML model: {e}")
            print("[neural] Falling back to ONNX Runtime")
            model_path = VISUAL_MODEL_ONNX  # Use ONNX file for fallback
    
    # ONNX Runtime with CoreML provider
    print(f"[neural] Loading ONNX model with CoreML provider…")
    
    available = ort.get_available_providers()
    print(f"[neural] Available providers: {available}")
    
    opts = ort.SessionOptions()
    opts.graph_optimization_level = ort.GraphOptimizationLevel.ORT_ENABLE_BASIC
    opts.inter_op_num_threads = int(os.environ.get("ORT_THREADS", "4"))

    # Prepare CoreML provider options for ANE optimization
    cache_dir = MODEL_DIR / ".ort_cache"
    cache_dir.mkdir(parents=True, exist_ok=True)

    providers: list = []
    if "CoreMLExecutionProvider" in available:
        coreml_options = {
            "MLComputeUnits": "CPUAndNeuralEngine",  # Exclude GPU, use ANE only
            "ModelFormat": "MLProgram",  # Modern format, better for transformers
            "RequireStaticInputShapes": "1",  # Enable ANE compilation with static shapes
            "ModelCacheDirectory": str(cache_dir),  # Cache compiled models
        }
        providers.append(("CoreMLExecutionProvider", coreml_options))
    providers.append(("CPUExecutionProvider", {}))

    try:
        session = ort.InferenceSession(str(model_path), sess_options=opts, providers=providers)
    except Exception as e:
        print(f"[neural] Failed initializing with preferred providers: {e}")
        print("[neural] Falling back to CPU only")
        session = ort.InferenceSession(str(model_path), sess_options=opts, providers=[("CPUExecutionProvider", {})])
    
    active = session.get_providers()
    print(f"[neural] Loaded {model_path.name} (ONNX) | active providers: {active}")
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
        dummy = np.zeros((32, 3, CLIP_SIZE, CLIP_SIZE), dtype=np.float32)
        
        if USE_COREML:
            # CoreML API with dynamic names
            if hasattr(sess, '_input_names') and sess._input_names:
                input_name = sess._input_names[0]
                print(f"[neural] Warmup using CoreML input: {input_name}")
                sess.predict({input_name: dummy})
            else:
                print("[neural] Warning: Could not determine CoreML input name, skipping warmup")
        else:
            # ONNX Runtime API
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
    
    # Read all images first (this is I/O bound, not worth parallelizing)
    image_bytes_list = []
    filenames = []
    for upload in images:
        raw = await upload.read()
        image_bytes_list.append(raw)
        filenames.append(upload.filename)
    
    # Preprocess in parallel (decode + resize + normalize)
    t_preprocess_start = time.time()
    futures = [_preprocess_executor.submit(preprocess, raw) for raw in image_bytes_list]
    batch = []
    for i, f in enumerate(futures):
        try:
            arr = f.result()
            batch.append(arr)
        except Exception as exc:
            raise HTTPException(status_code=422, detail=f"Bad image ({filenames[i]}): {exc}")
    t_preprocess_ms = (time.time() - t_preprocess_start) * 1000

    num_real_images = len(batch)  # store original count before padding
    batch_arr = np.stack(batch, axis=0)  # (N, 3, 224, 224) float32
    t_stack_ms = (time.time() - t_preprocess_start - t_preprocess_ms) * 1000

    try:
        if USE_COREML:
            # CoreML inference with dynamically discovered input/output names
            # Model compiled with batch=32, so pad incoming batches to that size
            input_name = sess._input_names[0] if hasattr(sess, '_input_names') else "pixel_values"
            output_name = sess._output_names[0] if hasattr(sess, '_output_names') else "image_embeds"
            
            compiled_batch_size = 32
            if batch_arr.shape[0] < compiled_batch_size:
                # Pad with zero images to match compiled batch size
                padding_size = compiled_batch_size - batch_arr.shape[0]
                padding = np.zeros((padding_size, 3, CLIP_SIZE, CLIP_SIZE), dtype=np.float32)
                batch_arr = np.vstack([batch_arr, padding])
            
            print(f"[embed] Using CoreML input={input_name}, output={output_name}, batch_size={batch_arr.shape[0]}")
            
            # Run inference on full padded batch
            t0 = time.time()
            pred = sess.predict({input_name: batch_arr})
            full_output = pred[output_name]  # (32, 512)
            output = full_output[:num_real_images]  # Extract only real results (N, 512)
            elapsed_ms = (time.time() - t0) * 1000
            print(f"[embed] Preprocessing: {t_preprocess_ms:.1f}ms | CoreML inference: {elapsed_ms:.1f}ms | Total for {num_real_images} images")
        else:
            # ONNX Runtime inference
            input_name = sess.get_inputs()[0].name
            output_name = sess.get_outputs()[0].name
            
            print(f"[embed] Using ONNX Runtime input={input_name}, output={output_name}, batch_size={batch_arr.shape[0]}")
            
            t0 = time.time()
            output = sess.run([output_name], {input_name: batch_arr})[0]  # (N, 512)
            elapsed_ms = (time.time() - t0) * 1000
            print(f"[embed] Preprocessing: {t_preprocess_ms:.1f}ms | ONNX inference: {elapsed_ms:.1f}ms | Total for {num_real_images} images")
    except Exception as exc:
        import traceback
        traceback.print_exc()
        raise HTTPException(status_code=500, detail=f"Inference error: {exc}")

    # L2-normalise each embedding so cosine similarity = dot product
    t0 = time.time()
    norms = np.linalg.norm(output, axis=1, keepdims=True)
    norms = np.where(norms == 0, 1.0, norms)
    output = output / norms
    postproc_ms = (time.time() - t0) * 1000

    return {"embeddings": output.tolist()}
