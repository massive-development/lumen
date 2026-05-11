# ── Stage 1: Compile libbitnet.so ────────────────────────────────────────────
FROM nvidia/cuda:13.2.0-devel-ubuntu24.04 AS cuda-builder

RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /build
COPY bitnet/gpu/bitnet_kernels/ .

# 86 = Ampere (RTX 3000). Override with --build-arg CUDA_ARCH=89 for RTX 4000.
ARG CUDA_ARCH=86
RUN nvcc -std=c++17 \
    -Xcudafe --diag_suppress=177 \
    --compiler-options -fPIC \
    -lineinfo --shared \
    bitnet_kernels.cu -lcuda \
    -gencode=arch=compute_${CUDA_ARCH},code=compute_${CUDA_ARCH} \
    -o libbitnet.so


# ── Stage 2: Compile Go HTTP server ──────────────────────────────────────────
FROM golang:1.26.3-alpine AS go-builder

WORKDIR /build
COPY gpu/server.go gpu/go.mod ./
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o server .


# ── Stage 3: Runtime ──────────────────────────────────────────────────────────
# Official PyTorch image: known-good torch+Python+CUDA triple pre-installed.
# cu124 runs on CUDA 13.x drivers (forward-compatible). conda pip is in PATH.
FROM pytorch/pytorch:2.11.0-cuda13.0-cudnn9-runtime AS runtime

RUN apt-get update && apt-get install -y --no-install-recommends \ 
    python3-venv

RUN python3 -m venv /opt/venv                                                                                 
ENV PATH="/opt/venv/bin:$PATH"

WORKDIR /app/BitNet/gpu

# xformers from the matching wheel index — same cu124 build as the base torch.
RUN pip install --no-cache-dir xformers --index-url https://download.pytorch.org/whl/cu124

# Install remaining Python deps (torch and xformers excluded — already above).
COPY bitnet/gpu/requirements.txt .
RUN grep -vE '^(torch|xformers)' requirements.txt | pip install --no-cache-dir -r /dev/stdin

# Copy upstream GPU pipeline source, then overlay Lumen custom files on top.
COPY bitnet/gpu/ .
COPY gpu/ .
COPY --from=cuda-builder /build/libbitnet.so ./bitnet_kernels/libbitnet.so
COPY --from=go-builder   /build/server       ./server

VOLUME ["/app/BitNet/gpu/checkpoints"]

EXPOSE 8082

ENV CKPT_DIR=checkpoints
ENV MODEL_ALIAS=bitnet-b1.58-2b-4t
ENV PORT=8082
ENV MAX_SEQ=4096

CMD ["./server"]
