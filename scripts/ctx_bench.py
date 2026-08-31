#!/usr/bin/env python3
"""Single context-fill benchmark against a running malaikat/llama-server.

Calibrates chars-per-token, fills the context to ~target-tokens, sends one
non-streaming chat completion, and prints a JSON line with:
  prompt_tokens, completion_tokens, wall_s,
  ttft_s (prompt_ms), pp_tok_s, tg_tok_s,
  draft_n, draft_n_accepted, mtp_accept,
  plus raw timings.
"""
import json
import sys
import time
import urllib.request
import argparse

SENTENCE = (
    "The ornithologist recorded the migration pattern of the swift across the "
    "estuary while the tide receded and the wind shifted northward. "
)
# Calibration text must NOT be a prefix of SENTENCE, otherwise the server's
# prompt cache would reuse the calibration KV for the real (SENTENCE-based)
# request and undercount prefill time.
CAL_SENTENCE = (
    "Calibration probe: quantify tokenization density for this model now. "
)


def post(url, payload, timeout):
    data = json.dumps(payload).encode()
    req = urllib.request.Request(
        url, data=data, headers={"Content-Type": "application/json"}
    )
    return urllib.request.urlopen(req, timeout=timeout)


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--url", default="http://127.0.0.1:8080/v1/chat/completions")
    ap.add_argument("--target-tokens", type=int, required=True)
    ap.add_argument("--gen", type=int, default=256)
    ap.add_argument("--margin", type=float, default=0.97)
    ap.add_argument("--timeout", type=int, default=7200)
    args = ap.parse_args()

    # Calibrate chars-per-token with a short repetitive prompt.
    cal = CAL_SENTENCE * 20
    cal_payload = {
        "messages": [{"role": "user", "content": cal}],
        "max_tokens": 1,
        "temperature": 0.0,
        "stream": False,
    }
    with post(args.url, cal_payload, args.timeout) as r:
        j = json.loads(r.read())
    pt = j["usage"]["prompt_tokens"]
    cpt = len(cal) / max(pt, 1)

    target_chars = int(args.target_tokens * cpt * args.margin)
    filler = (SENTENCE * ((target_chars // len(SENTENCE)) + 1))[:target_chars]

    payload = {
        "messages": [{"role": "user", "content": filler}],
        "max_tokens": args.gen,
        "temperature": 0.0,
        "stream": False,
    }
    t0 = time.time()
    with post(args.url, payload, args.timeout) as r:
        j = json.loads(r.read())
    wall = time.time() - t0

    usage = j["usage"]
    t = j.get("timings") or {}
    dn = t.get("draft_n")
    da = t.get("draft_n_accepted")
    mtp = (da / dn) if (isinstance(dn, (int, float)) and dn > 0) else None

    out = {
        "prompt_tokens": usage["prompt_tokens"],
        "completion_tokens": usage["completion_tokens"],
        "wall_s": round(wall, 2),
        "ttft_s": round((t.get("prompt_ms") or 0) / 1000.0, 3),
        "pp_tok_s": t.get("prompt_per_second"),
        "tg_tok_s": t.get("predicted_per_second"),
        "prompt_ms": t.get("prompt_ms"),
        "predicted_ms": t.get("predicted_ms"),
        "draft_n": dn,
        "draft_n_accepted": da,
        "mtp_accept": round(mtp, 4) if mtp is not None else None,
        "timings_raw": t,
    }
    print(json.dumps(out))


if __name__ == "__main__":
    main()
