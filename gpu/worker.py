"""
BitNet GPU inference worker.
Reads JSON requests from stdin, writes JSON chunks to stdout.

Startup messages (no "id"):
  {"status": "loading_prefill"}
  {"status": "loading_decode"}
  {"status": "warming_up"}
  {"status": "ready"}

Per-request output:
  {"id": "...", "text": "..."}  (one per decoded chunk)
  {"id": "...", "done": true, "finish_reason": "stop"|"length",
   "prompt_tokens": N, "completion_tokens": N}
"""
import json
import os
import sys
import torch

sys.path.insert(0, os.path.dirname(__file__))

import model as fast
from tokenizer import ChatFormat, Tokenizer
from xformers.ops.fmha.attn_bias import (
    BlockDiagonalCausalWithOffsetPaddedKeysMask as AttnBias,
)

CKPT_DIR = os.environ.get("CKPT_DIR", "checkpoints")
MAX_SEQ = int(os.environ.get("MAX_SEQ", "4096"))
VALID_ROLES = frozenset({"system", "user", "assistant"})


def emit(obj: dict) -> None:
    print(json.dumps(obj), flush=True)


def _bias(q_len: int, kv_len: int, max_seq: int) -> AttnBias:
    b = AttnBias.from_seqlens(q_seqlen=[q_len], kv_seqlen=[kv_len], kv_padding=max_seq)
    b.q_seqinfo.to("cuda")
    b.k_seqinfo.to("cuda")
    return b


def _sample(logits: torch.Tensor, temperature: float, top_p: float) -> int:
    if temperature <= 0:
        return int(torch.argmax(logits).item())
    probs = torch.softmax(logits / temperature, dim=-1)
    sorted_probs, sorted_idx = torch.sort(probs, descending=True)
    cum = torch.cumsum(sorted_probs, dim=-1)
    sorted_probs[cum - sorted_probs > top_p] = 0.0
    sorted_probs /= sorted_probs.sum()
    return int(sorted_idx[torch.multinomial(sorted_probs, 1)].item())


@torch.inference_mode()
def _warmup(prefill, decode, cache) -> None:
    """Precompile xformers CUDA kernels so first real request doesn't incur JIT overhead."""
    try:
        # Use a short sequence (8 tokens max) to exercise both prefill and decode paths.
        max_seq = 8
        c = fast.cache_prefix(cache, max_seq)
        toks = torch.tensor([1, 2, 3, 4], dtype=torch.int, device="cuda")
        logits = prefill.forward_with_attn_bias(toks, _bias(4, 4, max_seq), c)
        nxt = int(torch.argmax(logits[-1]).item())
        tok1 = torch.tensor([nxt], dtype=torch.int, device="cuda")
        decode.forward_with_attn_bias(tok1, _bias(1, 5, max_seq), c)
        torch.cuda.synchronize()
    except Exception as e:
        emit({"status": f"warmup_warning: {e}"})


def load():
    device = "cuda:0"
    torch.set_default_device(device)
    torch.set_default_dtype(torch.bfloat16)

    tokenizer = Tokenizer(os.path.join(os.path.dirname(__file__), "tokenizer.model"))
    chat_fmt = ChatFormat(tokenizer)
    stop_tokens = tokenizer.stop_tokens

    model_args = fast.ModelArgs()

    emit({"status": "loading_prefill"})
    prefill = fast.Transformer(fast.ModelArgs(use_kernel=False))
    prefill.load_state_dict(
        torch.load(f"{CKPT_DIR}/model_state_fp16.pt", map_location="cpu", weights_only=True)
    )
    prefill.eval()

    emit({"status": "loading_decode"})
    decode = fast.Transformer(fast.ModelArgs(use_kernel=True))
    decode.load_state_dict(
        torch.load(f"{CKPT_DIR}/model_state_int2.pt", map_location="cpu", weights_only=True)
    )
    decode.eval()

    cache = fast.make_cache(model_args, MAX_SEQ)
    torch.cuda.synchronize()

    emit({"status": "warming_up"})
    _warmup(prefill, decode, cache)

    emit({"status": "ready"})
    return chat_fmt, stop_tokens, prefill, decode, cache


@torch.inference_mode()
def infer(req: dict, chat_fmt, stop_tokens, prefill, decode, cache) -> None:
    req_id = req["id"]
    messages = [
        {"role": m["role"], "content": m.get("content", "")}
        for m in req["messages"]
        if m.get("role") in VALID_ROLES
    ]
    max_tokens = int(req.get("max_tokens", 512))
    temperature = float(req.get("temperature", 0.7))
    top_p = float(req.get("top_p", 0.95))

    tokens = chat_fmt.encode_dialog_prompt(dialog=messages, completion=True)
    prompt_len = min(len(tokens), MAX_SEQ - 1)
    tokens = tokens[-prompt_len:]
    max_new = min(max_tokens, MAX_SEQ - prompt_len)
    max_seq = prompt_len + max_new

    c = fast.cache_prefix(cache, max_seq)
    tok_t = torch.tensor(tokens, dtype=torch.int, device="cuda")

    logits = prefill.forward_with_attn_bias(tok_t, _bias(prompt_len, prompt_len, max_seq), c)
    next_tok = _sample(logits[prompt_len - 1], temperature, top_p)

    if next_tok in stop_tokens:
        emit({"id": req_id, "done": True, "finish_reason": "stop",
              "prompt_tokens": prompt_len, "completion_tokens": 0})
        return

    completion_tokens = 1
    kv_len = prompt_len + 1
    finish_reason = "length"
    buf = [next_tok]

    # Pre-allocate a single-element tensor and update it in-place each step
    # to avoid per-token host→device allocations.
    next_tok_t = torch.zeros(1, dtype=torch.int, device="cuda")

    for _ in range(max_new - 1):
        if kv_len >= max_seq:
            break
        next_tok_t[0] = next_tok
        logits = decode.forward_with_attn_bias(next_tok_t, _bias(1, kv_len, max_seq), c)
        next_tok = _sample(logits[0], temperature, top_p)
        if next_tok in stop_tokens:
            finish_reason = "stop"
            break
        buf.append(next_tok)
        completion_tokens += 1
        kv_len += 1
        # Flush when the tokenizer can fully decode the buffer.
        try:
            text = chat_fmt.tokenizer.decode(buf)
            if text:
                emit({"id": req_id, "text": text})
                buf = []
        except Exception:
            pass

    if buf:
        text = chat_fmt.tokenizer.decode(buf)
        if text:
            emit({"id": req_id, "text": text})

    emit({"id": req_id, "done": True, "finish_reason": finish_reason,
          "prompt_tokens": prompt_len, "completion_tokens": completion_tokens})


def main():
    chat_fmt, stop_tokens, prefill, decode, cache = load()
    for line in sys.stdin:
        line = line.strip()
        if not line:
            continue
        try:
            req = json.loads(line)
            infer(req, chat_fmt, stop_tokens, prefill, decode, cache)
        except Exception as e:
            emit({"error": str(e)})


if __name__ == "__main__":
    main()
