# ── Stage 1: Builder ──────────────────────────────────────────────────────────
# Ubuntu 24.04 ships clang-18 and cmake 3.28 natively — no external installers.
FROM ubuntu:24.04 AS builder

ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential \
    cmake \
    clang \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app/BitNet
# Copy the upstream BitNet source from the submodule, not the Lumen repo root.
# The submodule provides CMakeLists.txt, include/, src/, 3rdparty/, etc.
COPY bitnet/ .

# bitnet-lut-kernels.h is included unconditionally but all content is guarded by
# GGML_BITNET_X86_TL2, which we disable. A stub satisfies the include on fresh clones.
RUN [ -f include/bitnet-lut-kernels.h ] || touch include/bitnet-lut-kernels.h

# i2_s quantization uses no lookup-table kernels, so TL2 is disabled.
# Python and pip are not needed here — cmake builds pure C++.
RUN cmake -B build \
        -DBITNET_X86_TL2=OFF \
        -DBUILD_SHARED_LIBS=OFF \
        -DCMAKE_C_COMPILER=clang \
        -DCMAKE_CXX_COMPILER=clang++

# Only build llama-server — avoids pulling in optional deps for other targets.
RUN cmake --build build --config Release --target llama-server -j$(nproc)


# ── Stage 2: Runtime ──────────────────────────────────────────────────────────
# Must match the builder OS so glibc/GLIBCXX versions align
FROM ubuntu:24.04 AS runtime

ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update && apt-get install -y --no-install-recommends \
    python3 \
    libstdc++6 \
    libgomp1 \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app/BitNet

COPY --from=builder /app/BitNet/build/bin ./build/bin

# Models are mounted at runtime via the compose volume
VOLUME ["/app/BitNet/models"]

EXPOSE 8081

# Override these in docker-compose or at `docker run -e` time
ENV MODEL_PATH=models/BitNet-b1.58-2B-4T/ggml-model-i2_s.gguf
ENV MODEL_ALIAS=bitnet-b1.58-2b-4t
# THREADS defaults to all available cores; override in docker-compose if needed
ENV THREADS=""

# -t  = generation threads (token-by-token decode)
# -tb = batch threads (prompt prefill — dominates latency as conversation grows)
# Both default to nproc so the container uses all cores it can see.
# --alias sets the model ID reported by /v1/models so clients (Claude Code, Olla) see it by name
CMD build/bin/llama-server \
    -m ${MODEL_PATH} \
    --alias ${MODEL_ALIAS} \
    -c 2048 \
    -t ${THREADS:-$(nproc)} \
    -tb ${THREADS:-$(nproc)} \
    -ngl 0 \
    --host 0.0.0.0 \
    --port 8081 \
    -cb \
    --cache-reuse 256
