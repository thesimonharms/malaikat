#!/usr/bin/env bash
# Benchmark ornith-1.5-35b-a3b across 256k / 512k / 1m context windows.
# Fully fills each window (~95%) and measures, per window:
#   - TTFT (prompt_ms), prefill tok/s (pp), decode tok/s (tg, with MTP n=3)
#   - MTP draft accept rate, KV cache footprint (from server startup log)
# Results averaged over REPS runs per window. 1m is filled to ~500k (capped)
# to keep the prefill runnable; that still exercises a 1m window allocation.
#
# Usage: scripts/sweep-ctx.sh
# Env overrides: MODEL, EXE, PORT, REPS, TMP, FILL_MULT(0..1, default 0.95)

set -uo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

EXE="${EXE:-$ROOT/malaikat}"
MODEL="${MODEL:-$HOME/.local/share/malaikat/models/Ornith-1.5-35B-A3B/Ornith-1.5-35B-Q4_K_M.gguf}"
PORT="${PORT:-8080}"
REPS="${REPS:-2}"
FILL_MULT="${FILL_MULT:-0.95}"
GEN="${GEN:-256}"
TMP="${TMPDIR:-/tmp}"
LOG="$TMP/sweep-ctx.log"
SRVOUT="$TMP/sweep-ctx-srv.txt"
RESULTS="$TMP/sweep-ctx-results.json"
ROWS="$TMP/sweep-ctx-rows.jsonl"
: > "$ROWS"

if [[ ! -x "$EXE" ]]; then
  echo "building $EXE ..." >&2
  go build -o "$EXE" . || exit 1
fi
if [[ ! -f "$MODEL" ]]; then
  echo "model not found: $MODEL (set MODEL=...)" >&2
  exit 1
fi

# ctx|label|fill_tokens
CONTEXTS=(
  "262144|256k|249000"
  "524288|512k|498000"
  "1048576|1m|500000"
)

# KV cache footprint is not printed by this llama.cpp build, so compute it
# analytically from the GGUF metadata (q8_0 K+V => 1 byte/element).
KVJSON="$(python3 "$ROOT/scripts/gguf_kv.py" "$MODEL" 2>/dev/null)"
kv_for() {
  local ctx="$1"
  python3 - "$KVJSON" "$ctx" <<'PY'
import json, sys
d = json.loads(sys.argv[1]); ctx = int(sys.argv[2])
gib = {262144:"kv_gib_256k",524288:"kv_gib_512k",1048576:"kv_gib_1m"}.get(ctx)
val = d.get(gib) if gib else None
if val is None:
    byp = d["kv_bytes_per_token"]; val = round(byp*ctx/(1<<30),2)
print("%s GiB (layers=%s, kv_heads=%s, head_dim=%s)" % (val, d["n_layers"], d["n_kv_heads"], d["head_dim"]))
PY
}

SRV_PID=""

stop_server() {
  pkill -x llama-server 2>/dev/null || true
  if [[ -n "${SRV_PID:-}" ]] && kill -0 "$SRV_PID" 2>/dev/null; then
    kill "$SRV_PID" 2>/dev/null || true
    wait "$SRV_PID" 2>/dev/null || true
  fi
  SRV_PID=""
  sleep 2
}
trap stop_server EXIT

start_server() {
  local ctx="$1"
  stop_server
  "$EXE" serve -config "$ROOT/coding.yaml" -m "$MODEL" -c "$ctx" -port "$PORT" -no-save \
    >"$SRVOUT" 2>&1 &
  SRV_PID=$!
  local deadline=$((SECONDS + 1200))
  while (( SECONDS < deadline )); do
    if curl -fsS -o /dev/null -m 2 "http://127.0.0.1:$PORT/health" 2>/dev/null; then
      return 0
    fi
    if ! kill -0 "$SRV_PID" 2>/dev/null; then
      echo "server exited early; tail of $SRVOUT:" >&2
      tail -n 40 "$SRVOUT" >&2 || true
      return 1
    fi
    sleep 1
  done
  echo "server not ready" >&2
  return 1
}

kv_size() {
  kv_for "$1"
}

bench_once() {
  local target="$1"
  python3 "$ROOT/scripts/ctx_bench.py" \
    --url "http://127.0.0.1:$PORT/v1/chat/completions" \
    --target-tokens "$target" --gen "$GEN" --margin "$FILL_MULT"
}

echo "=== ornith-1.5-35b-a3b context sweep (REPS=$REPS, fill=$(python3 -c "print('%.0f%%'%( $FILL_MULT*100 ))")) ===" | tee "$LOG"
echo "model: $MODEL" | tee -a "$LOG"

all_rows=()

for entry in "${CONTEXTS[@]}"; do
  ctx="${entry%%|*}"
  rest="${entry#*|}"
  label="${rest%%|*}"
  fill="${rest#*|}"

  echo | tee -a "$LOG"
  echo "=== ctx=$label (n_ctx=$ctx, fill≈$fill) ===" | tee -a "$LOG"

  if ! start_server "$ctx"; then
    echo "$label: FAIL (server start)" | tee -a "$LOG"
    printf '%s\n' "{\"label\":\"$label\",\"ctx\":$ctx,\"status\":\"fail\"}" >> "$ROWS"
    continue
  fi

  kv="$(kv_size "$ctx")"
  echo "  KV cache footprint: $kv" | tee -a "$LOG"

  reps_json="[]"
  for (( r=1; r<=REPS; r++ )); do
    echo -n "  rep $r/$REPS: " | tee -a "$LOG"
    row="$(bench_once "$fill")"
    echo "$row" | tee -a "$LOG"
    reps_json="$(python3 -c "import json,sys; rows=json.loads(sys.argv[1]); rows.append(json.loads(sys.argv[2])); print(json.dumps(rows))" "$reps_json" "$row")"
  done

  # Aggregate this window.
  agg="$(python3 - "$reps_json" "$kv" "$label" "$ctx" "$fill" <<'PY'
import json, sys
rows = json.loads(sys.argv[1])
kv = sys.argv[2]; label = sys.argv[3]; ctx = int(sys.argv[4]); fill = int(sys.argv[5])
def avg(k): 
    v=[r[k] for r in rows if isinstance(r.get(k),(int,float))]; return sum(v)/len(v) if v else None
def f(x,n=2): return round(x,n) if x is not None else None
out = {
  "label": label, "ctx": ctx, "fill_target": fill,
  "kv_footprint": kv,
  "reps": len(rows),
  "prompt_tokens_avg": f(avg("prompt_tokens"),0),
  "ttft_s_avg": f(avg("ttft_s"),3),
  "pp_tok_s_avg": f(avg("pp_tok_s"),1),
  "tg_tok_s_avg": f(avg("tg_tok_s"),1),
  "mtp_accept_avg": f(avg("mtp_accept"),4),
  "per_rep": rows,
}
print(json.dumps(out))
PY
)"
  echo "  >>> $label avg: ttft=$(echo "$agg" | python3 -c 'import json,sys;d=json.load(sys.stdin);print(d["ttft_s_avg"])')s  pp=$(echo "$agg" | python3 -c 'import json,sys;d=json.load(sys.stdin);print(d["pp_tok_s_avg"])') tok/s  tg=$(echo "$agg" | python3 -c 'import json,sys;d=json.load(sys.stdin);print(d["tg_tok_s_avg"])') tok/s  mtp=$(echo "$agg" | python3 -c 'import json,sys;d=json.load(sys.stdin);print(d["mtp_accept_avg"])')" | tee -a "$LOG"
  printf '%s\n' "$agg" >> "$ROWS"
  stop_server
done

echo | tee -a "$LOG"
python3 -c "
import json
rows=[json.loads(l) for l in open('$ROWS') if l.strip()]
print('SUMMARY')
print('%-6s %-10s %-12s %-8s %-9s %-9s %-8s' % ('ctx','fill','KV','TTFT_s','pp_t/s','tg_t/s','MTP'))
for r in rows:
    kv=r.get('kv_footprint','n/a')
    print('%-6s %-10s %-12s %-8s %-9s %-9s %-8s' % (r.get('label'), r.get('fill_target'), kv, r.get('ttft_s_avg'), r.get('pp_tok_s_avg'), r.get('tg_tok_s_avg'), r.get('mtp_accept_avg')))
json.dump(rows, open('$RESULTS','w'), indent=2)
print('wrote $RESULTS')
"

echo "done."
