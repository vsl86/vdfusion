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

python download_model.py --output-dir ./models
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
| `MODEL_DIR`  | `/models` | Directory containing the ONNX model     |
| `MAX_BATCH`  | `32`      | Maximum images per `/embed` request      |
| `ORT_THREADS`| `4`       | ONNX Runtime inter-op thread count       |

---

## Accuracy note

CLIP ViT-B/32 understands semantic content, not just pixel similarity. It will
catch re-encoded copies, different crops, colour-graded versions, and
resolution-scaled duplicates that pHash misses.

vdfusion uses the **same similarity threshold** as for pHash — adjust it in
Settings → Similarity if you get too many or too few matches.
