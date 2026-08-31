#!/usr/bin/env bash
# Lightweight context-window benchmark for ornith-1.5-35b-a3b.
# Mirrors the standard short-prompt workflow (bench -n 128): a SHORT prompt is
# sent at each context window (256k/512k/1m) and we report decode tok/s (tg,
# with MTP n=3), prefill tok/s (pp) and MTP accept rate. This shows the
# overhead of *allocating* a larger window, not the cost of filling it.
#
# The server is restarted per rep so the prompt cache cannot reuse a prior
# run's KV and skew prefill/TTFT.
#
# Usage: scripts/bench-ctx-light.sh   (env: MODEL, EXE, REPS, TARGET, GEN)

set -uo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

EXE="${EXE:-$ROOT/malaikat}"
MODEL="${MODEL:-$HOME/.local/share/malaikat/models/Ornith-1.5-35B-A3B/Ornith-1.5-35B-Q4_K_M.gguf}"
PORT="${PORT:-8080}"
REPS="${REPS:-2}"
TARGET="${TARGET:-300}"
GEN="${GEN:-128}"
TMP="${TMPDIR:-/tmp}"
LOG="$TMP/bench-ctx-light.log"
SRVOUT="$TMP/bench-ctx-light-srv.txt"
ROWS="$TMP/bench-ctx-light-rows.jsonl"
: > "$ROWS"

if [[ ! -x "$EXE" ]]; then go build -o "$EXE" . || exit 1; fi
if [[ ! -f "$MODEL" ]]; then echo "model not found: $MODEL" >&2; exit 1; fi

CONTEXTS=("262144|256k" "524288|512k" "1048576|1m")

SRV_PID=""
stop_server() {
  pkill -x llama-server 2>/dev/null || true
  if [[ -n "${SRV_PID:-}" ]] && kill -0 "$SRV_PID" 2>/dev/null; then
    kill "$SRV_PID" 2>/dev/null || true; wait "$SRV_PID" 2>/dev/null || true
  fi
  SRV_PID=""; sleep 2
}
start_server() {
  local ctx="$1"
  stop_server
  "$EXE" serve -config "$ROOT/coding.yaml" -m "$MODEL" -c "$ctx" -port "$PORT" -no-save \
    >"$SRVOUT" 2>&1 &
  SRV_PID=$!
  local deadline=$((SECONDS + 1200))
  while (( SECONDS < deadline )); do
    curl -fsS -o /dev/null -m 2 "http://127.0.0.1:$PORT/health" 2>/dev/null && return 0
    kill -0 "$SRV_PID" 2>/dev/null || { echo "server exited; tail:" >&2; tail -n 30 "$SRVOUT" >&2; return 1; }
    sleep 1
  done
  echo "server not ready" >&2; return 1
}
bench_once() {
  python3 "$ROOT/scripts/ctx_bench.py" \
    --url "http://127.0.0.1:$PORT/v1/chat/completions" \
    --target-tokens "$TARGET" --gen "$GEN" --margin 0.97
}

echo "=== ornith-1.5-35b-a3b LIGHT context sweep (short prompt: target=$TARGET tok, gen=$GEN, REPS=$REPS) ===" | tee "$LOG"
echo "model: $MODEL" | tee -a "$LOG"

for entry in "${CONTEXTS[@]}"; do
  ctx="${entry%%|*}"; label="${entry#*|}"
  echo | tee -a "$LOG"
  echo "=== ctx=$label (n_ctx=$ctx) ===" | tee -a "$LOG"
  reps_json="[]"
  ok=1
  for (( r=1; r<=REPS; r++ )); do
    if ! start_server "$ctx"; then ok=0; echo "$label: FAIL (server)" | tee -a "$LOG"; break; fi
    echo -n "  rep $r/$REPS: " | tee -a "$LOG"
    row="$(bench_once)"
    echo "$row" | tee -a "$LOG"
    reps_json="$(python3 -c "import json,sys; rows=json.loads(sys.argv[1]); rows.append(json.loads(sys.argv[2])); print(json.dumps(rows))" "$reps_json" "$row")"
    stop_server
  done
  [[ $ok -eq 0 ]] && { printf '%s\n' "{\"label\":\"$label\",\"ctx\":$ctx,\"status\":\"fail\"}" >> "$ROWS"; continue; }
  agg="$(python3 - "$reps_json" "$label" "$ctx" <<'PY'
import json, sys
rows=json.loads(sys.argv[1]); label=sys.argv[2]; ctx=int(sys.argv[3])
def avg(k):
    v=[r[k] for r in rows if isinstance(r.get(k),(int,float))]; return sum(v)/len(v) if v else None
def f(x,n=2): return round(x,n) if x is not None else None
out={"label":label,"ctx":ctx,"reps":len(rows),
  "prompt_tokens_avg":f(avg("prompt_tokens"),0),
  "ttft_s_avg":f(avg("ttft_s"),3),
  "pp_tok_s_avg":f(avg("pp_tok_s"),1),
  "tg_tok_s_avg":f(avg("tg_tok_s"),1),
  "mtp_accept_avg":f(avg("mtp_accept"),4),
  "per_rep":rows}
print(json.dumps(out))
PY
)"
  echo "  >>> $label avg: tg=$(echo "$agg" | python3 -c 'import json,sys;print(json.load(sys.stdin)["tg_tok_s_avg"])') tok/s  pp=$(echo "$agg" | python3 -c 'import json,sys;print(json.load(sys.stdin)["pp_tok_s_avg"])') tok/s  mtp=$(echo "$agg" | python3 -c 'import json,sys;print(json.load(sys.stdin)["mtp_accept_avg"])')" | tee -a "$LOG"
  printf '%s\n' "$agg" >> "$ROWS"
done

echo | tee -a "$LOG"
python3 -c "
import json
rows=[json.loads(l) for l in open('$ROWS') if l.strip()]
print('SUMMARY (short prompt, MTP n=3, FA on, KV q8)')
print('%-6s %-8s %-9s %-9s %-8s' % ('ctx','pp_t/s','tg_t/s','TTFT_s','MTP'))
for r in rows:
    print('%-6s %-9s %-9s %-9s %-8s' % (r.get('label'), r.get('pp_tok_s_avg'), r.get('tg_tok_s_avg'), r.get('ttft_s_avg'), r.get('mtp_accept_avg')))
json.dump(rows, open('$TMP/bench-ctx-light-results.json','w'), indent=2)
print('wrote $TMP/bench-ctx-light-results.json')
"
echo done.
