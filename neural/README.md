# vdfusion neural backend

A containerised CLIP ViT-B/32 frame-embedding service that gives vdfusion
semantics-aware duplicate detection. Run it on any machine with spare GPU/CPU
(e.g. a spare MacBook M4 Pro) and point vdfusion at its URL.

## Quick start

### 1. Build the image

```bash
# from the repo root
docker build -t vdfusion-neural -f neural/Dockerfile .
# or
podman build -t vdfusion-neural -f neural/Dockerfile .
```

### 2. Run

```bash
docker run -d \
  --name vdfusion-neural \
  -p 8765:8765 \
  -v vdfusion-models:/models \
  --restart unless-stopped \
  vdfusion-neural
```

The container downloads the CLIP ViT-B/32 ONNX weights (~340 MB) into the
`vdfusion-models` volume on first start. Subsequent starts are instant.

### 3. Configure vdfusion

Open **Settings → Neural Backend** and set the URL to:

```
http://<ip-of-your-machine>:8765
```

Click **Test Connection** — the indicator should turn green.

---

## Running natively on macOS (M4 Pro recommended)

```bash
cd neural
python3 -m venv .venv
source .venv/bin/activate

# onnxruntime-silicon uses Apple's ANE/CoreML — much faster than CPU
pip install fastapi "uvicorn[standard]" onnxruntime-silicon Pillow numpy

python download_model.py --output-dir ./models --batch-sizes 1,2,4,8,16,32
MODEL_DIR=./models uvicorn server:app --host 0.0.0.0 --port 8765
```

---

## API

| Method | Path      | Description                                              |
|--------|-----------|----------------------------------------------------------|
| GET    | `/health` | Returns `{"status":"ok","model":"clip-vit-b32",...}`    |
| GET    | `/info`   | Model metadata (dim, providers, version)                |
| POST   | `/embed`  | Accepts `multipart/form-data` images → returns embeddings |

### POST /embed — example

```bash
curl -X POST http://localhost:8765/embed \
  -F "images=@frame1.jpg" \
  -F "images=@frame2.jpg"
```

Response:
```json
{
  "embeddings": [
    [0.023, -0.041, ...],   // 512 floats, L2-normalised
    [0.018, -0.039, ...]
  ]
}
```

---

## Environment variables

| Variable     | Default   | Description                              |
|--------------|-----------|------------------------------------------|
| `MODEL_DIR` | `/models` | Directory containing the ONNX/CoreML models |
| `MAX_BATCH` | `32` | Maximum images per `/embed` REST request |
| `COMPILED_BATCH_SIZE` | `2` on <12GB, `4` on <16GB, `16` on <32GB, otherwise `32` | Fixed CoreML/ANE micro-batch size; larger REST requests are split internally |
| `PREPROCESS_WORKERS` | `1` on <16GB, `3` on <32GB, otherwise `4` | Image decode/resize worker count |
| `FORCE_ONNX` | `0` on macOS with CoreML, otherwise `1` | Force universal ONNX path |
| `COREML_STARTUP_TIMEOUT` | `60` | Seconds to wait for CoreML load + warmup before fallback |
| `COREML_PREDICT_TIMEOUT` | `60` | Seconds to wait for each CoreML prediction before killing worker |
| `ORT_THREADS` | `4` | ONNX Runtime inter-op thread count |

---

## Accuracy note

CLIP ViT-B/32 understands semantic content, not just pixel similarity. It will
catch re-encoded copies, different crops, colour-graded versions, and
resolution-scaled duplicates that pHash misses.

vdfusion uses the **same similarity threshold** as for pHash — adjust it in
Settings → Similarity if you get too many or too few matches.

## OS detection

- Added OS detection (CoreML only on macOS)
- Auto-enables FORCE_ONNX when CoreML is not available
- On low-memory Macs, keeps CoreML enabled but defaults to a smaller `COMPILED_BATCH_SIZE`
- CoreML worker must load and complete a dummy prediction before startup is considered successful
- Larger REST batches are split into fixed-size CoreML micro-batches internally
Respects all env vars:
- MODEL_DIR
- FORCE_ONNX
- MAX_BATCH
- COMPILED_BATCH_SIZE
- PREPROCESS_WORKERS
- COREML_COMPUTE_UNITS
- COREML_STARTUP_TIMEOUT
- COREML_PREDICT_TIMEOUT

### CoreML batch sizing

`MAX_BATCH` controls how many images the REST endpoint accepts in one request. `COMPILED_BATCH_SIZE` controls the fixed tensor shape of the ANE model. They do not need to match.

For example, on an 8GB MacBook Air you can accept 23–32 images per request while running ANE in smaller chunks:

```bash
MODEL_DIR=./models \
COMPILED_BATCH_SIZE=2 \
MAX_BATCH=32 \
PREPROCESS_WORKERS=1 \
uvicorn server:app --host 0.0.0.0 --port 8765
```

A 23-image request will be executed as CoreML chunks of `2 + 2 + … + 1`, with the final chunk padded to 2 internally and trimmed back to 23 outputs.

Avoid automatic upward probing on low-memory Macs: if an oversized ANE batch hangs inside CoreML, the operating system may not unwind it cleanly even when the parent process has a timeout. Prefer selecting a conservative `COMPILED_BATCH_SIZE` directly (`2` for 8GB-class Macs, `1` if `2` is unstable).
