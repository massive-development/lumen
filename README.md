# Lumen

A self-contained, fully local LLM stack built on [Microsoft BitNet.cpp](https://github.com/microsoft/BitNet), with drop-in Claude Code compatibility via an Anthropic→OpenAI proxy.

Run inference entirely on your own hardware. Point Claude Code (or any Anthropic SDK client) at your local machine and it works transparently — no API keys, no usage costs, no data leaving your network.

---

## What you get

| Service | Purpose |
|---|---|
| **bitnet-cpu** | CPU inference via llama-server (BitNet b1.58 GGUF) |
| **bitnet-gpu** | GPU inference via W2A8 PyTorch kernel (Ampere+) |
| **olla** | Load balancer — routes to GPU when healthy, CPU fallback |
| **anthropic-proxy** | Anthropic API → OpenAI translation layer for Claude Code |
| **memory** | Persistent facts/preferences store (PostgreSQL-backed) |
| **bitnet-tools** | 27-endpoint OpenAPI tool server (time, math, web, finance, system) |
| **openwebui** | Chat UI with RAG, web search, STT, TTS |
| **postgres** | pgvector store for memory and RAG embeddings |
| **ollama** | Embedding model host (nomic-embed-text) |
| **searxng** | Self-hosted metasearch (Google, Bing, DDG) |
| **pipelines** | OpenWebUI middleware for filters and custom providers |
| **whisper** | Local speech-to-text (OpenAI-compatible) |
| **kokoro** | Local text-to-speech (OpenAI-compatible) |
| **tika** | Document extraction for RAG (PDFs, Word, PowerPoint, 1000+ formats) |
| **langfuse** | LLM request tracing and observability dashboard |
| **nginx** | HTTPS reverse proxy (port 80/443) |

---

## Prerequisites

- Docker + Docker Compose
- 16 GB RAM minimum (32 GB recommended)
- For GPU inference: NVIDIA GPU with CUDA 12+, drivers installed, [nvidia-container-toolkit](https://docs.nvidia.com/datacenter/cloud-native/container-toolkit/install-guide.html)
- A domain or self-signed cert if you want HTTPS (see [Certificates](#certificates))

---

## Quick start

### 1. Clone

```bash
git clone --recurse-submodules https://github.com/massive-development/lumen.git
cd lumen
```

### 2. Download the model

```bash
# CPU model (GGUF)
mkdir -p models/BitNet-b1.58-2B-4T
huggingface-cli download microsoft/BitNet-b1.58-2B-4T-GGUF \
  ggml-model-i2_s.gguf --local-dir models/BitNet-b1.58-2B-4T

# GPU checkpoints (optional)
mkdir -p gpu/checkpoints
huggingface-cli download microsoft/BitNet-b1.58-2B-4T \
  --local-dir gpu/checkpoints/bitnet-b1.58-2B-4T-bf16
```

### 3. Configure

```bash
cp .env.example .env

# Generate all secrets in one shot
python3 keygen.py --write

# Optionally set a personalization context for the proxy
cp personalization.json.example personalization.json
$EDITOR personalization.json
```

Edit `.env` for any non-secret settings:
- `MODEL_PATH` — path inside the container to the GGUF file
- `CUDA_ARCH` — your GPU compute capability (86 = RTX 3000, 89 = RTX 4000, 75 = Turing)
- `WEBUI_URL` — public URL for OpenWebUI CORS
- `LANGFUSE_ADMIN_EMAIL` / `LANGFUSE_ADMIN_NAME` — Langfuse dashboard credentials

### 4. Certificates

Place your TLS cert and key in `certs/`:

```bash
mkdir -p certs

# Self-signed (development)
openssl req -x509 -nodes -days 365 -newkey rsa:2048 \
  -keyout certs/bitnet.key -out certs/bitnet.crt \
  -subj "/CN=localhost"
```

Or drop in a real cert/key pair from Let's Encrypt, Cloudflare, etc.

### 5. Start

```bash
docker compose up -d
```

First boot takes a few minutes — the CPU image builds from source, and Ollama pulls the embedding model.

---

## Claude Code integration

Point Claude Code at your local proxy instead of `api.anthropic.com`:

```bash
# In your shell profile or .env
export ANTHROPIC_BASE_URL=https://your-domain/proxy
export ANTHROPIC_API_KEY=sk-local   # any non-empty value
```

Or in Claude Code settings:

```json
{
  "apiBaseUrl": "https://your-domain/proxy"
}
```

The proxy translates Anthropic API calls to OpenAI-format requests, injects persistent memory context, and routes inference through Olla to whichever backend is healthy.

> If running locally without a domain, use `http://localhost:5000` directly (the proxy container exposes port 5000 internally; map it in docker-compose if needed).

---

## Post-boot configuration

Four one-time steps in the OpenWebUI admin panel (`https://your-domain/`). These are persisted in OpenWebUI's database and survive restarts.

### 1. Connect Pipelines middleware

**Admin → Settings → Connections → Add OpenAI connection**

| Field | Value |
|---|---|
| URL | `http://pipelines:9099` |
| API Key | `PIPELINES_API_KEY` from `.env` |

### 2. Install the Langfuse tracing pipeline

After step 1, go to **Admin → Settings → Pipelines**, search for **"Langfuse Filter Pipeline"**, install it, then configure:

| Field | Value |
|---|---|
| Host | `http://langfuse:3000` |
| Public Key | `LANGFUSE_PUBLIC_KEY` from `.env` |
| Secret Key | `LANGFUSE_SECRET_KEY` from `.env` |

Dashboard at `http://localhost:3000` — credentials are `LANGFUSE_ADMIN_EMAIL` / `LANGFUSE_ADMIN_PASSWORD`.

### 3. Connect the tool server

**Admin → Settings → Tools → Add tool server**

| Field | Value |
|---|---|
| URL | `http://bitnet-tools:8083` |

No auth required (internal only).

### 4. Connect Open Terminal

**Admin → Settings → Open Terminal**

| Field | Value |
|---|---|
| URL | `http://open-terminal:8000` |
| API Key | `OPEN_TERMINAL_API_KEY` from `.env` |

---

## Changing the model

Any GGUF-format model compatible with llama-server works. Update `.env`:

```bash
MODEL_PATH=models/your-model/model.gguf
MODEL_ALIAS=your-model-name
```

Then rebuild the CPU container:

```bash
docker compose up -d --build bitnet-cpu
```

For GPU, convert your checkpoint to the W2A8 format — see [`gpu/README.md`](gpu/README.md).

---

## Rotating secrets

To regenerate all secrets:

```bash
# Reset all managed secrets to change-me
sed -i 's/^POSTGRES_PASSWORD=.*/POSTGRES_PASSWORD=change-me/' .env
# ... (or edit manually)

python3 keygen.py --write
```

If you change `POSTGRES_PASSWORD`, also update the live database — postgres ignores the env var after the data directory is initialized:

```bash
PW=$(grep POSTGRES_PASSWORD .env | cut -d= -f2)
docker exec bitnet-postgres psql -U memory -d bitnet_memory \
  -c "ALTER USER memory PASSWORD '$PW';"
```

---

## Architecture

```
Claude Code / any Anthropic client
        │
        ▼
   nginx (443)
        │
   /proxy/  ──────────► anthropic-proxy :5000
                               │  (memory injection, model routing)
                               ▼
                          olla :40114
                        ┌────┴────┐
                        ▼         ▼
                  bitnet-cpu   bitnet-gpu
                   :8081         :8082

   /        ──────────► openwebui :8080
                          │  RAG, chat UI
               ┌──────────┼──────────────┐
               ▼          ▼              ▼
          postgres      ollama        searxng
         (pgvector)  (embeddings)   (web search)
```

---

## Services reference

| Container | Internal port | External |
|---|---|---|
| bitnet-cpu | 8081 | — |
| bitnet-gpu | 8082 | — |
| olla | 40114 | — |
| anthropic-proxy | 5000 | via nginx `/proxy/` |
| memory | 6000 | via nginx `/memory/` |
| bitnet-tools | 8083 | — |
| openwebui | 8080 | via nginx `/` |
| postgres | 5432 | — |
| ollama | 11434 | — |
| searxng | 8080 | — |
| pipelines | 9099 | — |
| whisper | 8000 | — |
| kokoro | 8880 | — |
| tika | 9998 | — |
| langfuse | 3000 | host port 3000 |
| nginx | 80, 443 | host ports 80, 443 |

---

## License

The BitNet.cpp core ([`src/`](src/), [`3rdparty/`](3rdparty/), [`utils/`](utils/), [`preset_kernels/`](preset_kernels/)) is © Microsoft Corporation, licensed under [MIT](LICENSE).

The stack layer (`docker-compose.yaml`, `proxy.go`, `memory/`, `tools/`, `nginx.conf`, `keygen.py`, etc.) is original work released under the same MIT license.
