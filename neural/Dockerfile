# syntax=docker/dockerfile:1
# ──────────────────────────────────────────────────────────────────────────────
# vdfusion neural backend
# CLIP ViT-B/32 ONNX inference service
#
# Build:
#   docker build -t vdfusion-neural ./neural
#   # or from repo root:
#   docker build -t vdfusion-neural -f neural/Dockerfile .
#
# Run (pulls model on first start):
#   docker run -d \
#     -p 8765:8765 \
#     -v vdfusion-models:/models \
#     vdfusion-neural
#
# Point vdfusion settings to: http://<host>:8765
# ──────────────────────────────────────────────────────────────────────────────

FROM python:3.12-slim AS base

WORKDIR /app

# System deps (for Pillow & urllib)
RUN apt-get update && apt-get install -y --no-install-recommends \
        libgomp1 \
    && rm -rf /var/lib/apt/lists/*

# ── Python deps ───────────────────────────────────────────────────────────────
COPY neural/requirements.txt .

# Always install the generic onnxruntime inside Linux containers
# (onnxruntime-silicon is macOS-arm64 only; requirements.txt handles the guard,
#  but pip platform markers can't cross-compile so we force the right one here)
RUN pip install --no-cache-dir \
        fastapi \
        "uvicorn[standard]" \
        onnxruntime \
        Pillow \
        numpy

# ── Application ───────────────────────────────────────────────────────────────
COPY neural/server.py         ./server.py
COPY neural/download_model.py ./download_model.py

# Model volume – mount here or let entrypoint download on first run
VOLUME ["/models"]

ENV MODEL_DIR=/models \
    MAX_BATCH=32 \
    ORT_THREADS=4 \
    PYTHONUNBUFFERED=1

# Download model at container start if not already present
ENTRYPOINT ["sh", "-c", "python download_model.py --output-dir /models && exec uvicorn server:app --host 0.0.0.0 --port 8765"]

EXPOSE 8765

LABEL org.opencontainers.image.title="vdfusion-neural" \
      org.opencontainers.image.description="CLIP ViT-B/32 frame embedding backend for vdfusion" \
      org.opencontainers.image.source="https://github.com/vsevolod/vdfusion"
