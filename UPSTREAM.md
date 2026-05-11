# Upstream Reference

This repository contains custom services and configuration built on top of [microsoft/BitNet](https://github.com/microsoft/BitNet).

## Original Repository

| Field | Value |
|---|---|
| **Repo** | [https://github.com/microsoft/BitNet](https://github.com/microsoft/BitNet) |
| **License** | MIT |
| **Purpose** | 1-bit quantized LLM inference engine (BitNet b1.58) |

## Relationship

The `bitnet/` submodule (when initialized) pins the exact upstream commit this stack was built against. To add it:

```bash
git submodule add https://github.com/microsoft/BitNet.git bitnet
git submodule update --init --recursive
```

## What this repo adds

| Component | Path | Description |
|---|---|---|
| Anthropic proxy | `proxy.go`, `proxy.Dockerfile` | Anthropic API → OpenAI translation layer |
| GPU server | `gpu/server.go`, `gpu/worker.py` | W2A8 GPU inference via PyTorch |
| Memory service | `memory/main.go` | PostgreSQL-backed facts/preferences store |
| Tool server | `tools/` | 27-endpoint OpenAPI tool server |
| nginx config | `nginx.conf` | HTTPS reverse proxy |
| Compose stack | `docker-compose.yaml` | Full service orchestration |
| SearXNG config | `searxng/` | Self-hosted metasearch settings |
| Postgres init | `postgres/` | DB init scripts (pgvector, langfuse) |
| Key generation | `keygen.py` | API key management |
| Olla config | `olla.yaml` | Load balancer routing (GPU → CPU fallback) |
