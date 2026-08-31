#!/usr/bin/env bash
# Search for decode-throughput optimizations for ornith-1.5-35b-a3b at 256k.
# Same short-prompt harness as bench-ctx-light.sh; varies ONE knob per variant
# and ranks by decode tok/s (tg, MTP n=3 baseline). Reports pp/tg/mtp.
#
# Usage: scripts/sweep-opt.sh   (env: MODEL, EXE, REPS, TARGET, GEN, CTX)

set -uo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"
EXE="${EXE:-$ROOT/malaikat}"
MODEL="${MODEL:-$HOME/.local/share/malaikat/models/Ornith-1.5-35B-A3B/Ornith-1.5-35B-Q4_K_M.gguf}"
PORT="${PORT:-8080}"
REPS="${REPS:-2}"
TARGET="${TARGET:-300}"
GEN="${GEN:-128}"
CTX="${CTX:-262144}"
TMP="${TMPDIR:-/tmp}"
LOG="$TMP/sweep-opt.log"
SRVOUT="$TMP/sweep-opt-srv.txt"

if [[ ! -x "$EXE" ]]; then go build -o "$EXE" . || exit 1; fi
if [[ ! -f "$MODEL" ]]; then echo "model not found: $MODEL" >&2; exit 1; fi

# name|env|args   (env empty -> none; args appended to serve; use -- for passthrough)
VARIANTS=(
  "baseline||"
  "combo|GPU_MAX_HW_QUEUES=8 HSA_ENABLE_SDMA=0|"
  "hwq16|GPU_MAX_HW_QUEUES=16|"
  "nommap||--no-mmap"
  "combo_mmap|GPU_MAX_HW_QUEUES=8 HSA_ENABLE_SDMA=0|--no-mmap"
)

SRV_PID=""
stop_server() {
  pkill -x llama-server 2>/dev/null || true
  if [[ -n "${SRV_PID:-}" ]] && kill -0 "$SRV_PID" 2>/dev/null; then kill "$SRV_PID" 2>/dev/null; wait "$SRV_PID" 2>/dev/null; fi
  SRV_PID=""; sleep 2
}
start_server() {
  local env="$1"; shift
  stop_server
  # shellcheck disable=SC2086
  env $env "$EXE" serve -config "$ROOT/coding.yaml" -m "$MODEL" -c "$CTX" -port "$PORT" -no-save "$@" \
    >"$SRVOUT" 2>&1 &
  SRV_PID=$!
  local deadline=$((SECONDS+1200))
  while (( SECONDS < deadline )); do
    curl -fsS -o /dev/null -m 2 "http://127.0.0.1:$PORT/health" 2>/dev/null && return 0
    kill -0 "$SRV_PID" 2>/dev/null || { echo "server exited; tail:" >&2; tail -n 30 "$SRVOUT" >&2; return 1; }
    sleep 1
  done
  echo "server not ready" >&2; return 1
}
bench_once() {
  python3 "$ROOT/scripts/ctx_bench.py" --url "http://127.0.0.1:$PORT/v1/chat/completions" \
    --target-tokens "$TARGET" --gen "$GEN" --margin 0.97
}

echo "=== ornith optimization sweep (ctx=$CTX, target=$TARGET, gen=$GEN, REPS=$REPS) ===" | tee "$LOG"
results=()
for v in "${VARIANTS[@]}"; do
  name="${v%%|*}"; rest="${v#*|}"; env="${rest%%|*}"; args="${rest#*|}"
  echo | tee -a "$LOG"; echo "=== $name (env='${env:-none}' args='${args:-none}') ===" | tee -a "$LOG"
  read -ra vargs <<<"$args"
  if ! start_server "$env" "${vargs[@]}"; then
    echo "$name: FAIL" | tee -a "$LOG"; results+=("$name|-1|-1|-1"); stop_server; continue
  fi
  reps_json="[]"
  for (( r=1; r<=REPS; r++ )); do
    echo -n "  rep $r/$REPS: " | tee -a "$LOG"
    row="$(bench_once)"
    echo "$row" | tee -a "$LOG"
    reps_json="$(python3 -c "import json,sys; rows=json.loads(sys.argv[1]); rows.append(json.loads(sys.argv[2])); print(json.dumps(rows))" "$reps_json" "$row")"
  done
  stop_server
  agg="$(python3 - "$reps_json" "$name" <<'PY'
import json,sys
rows=json.loads(sys.argv[1]); name=sys.argv[2]
def avg(k):
    v=[r[k] for r in rows if isinstance(r.get(k),(int,float))]; return sum(v)/len(v) if v else 0
print("%s|%.1f|%.1f|%.4f" % (name, avg("pp_tok_s"), avg("tg_tok_s"), avg("mtp_accept")))
PY
)"
  results+=("$agg")
done

echo | tee -a "$LOG"
echo "RESULTS ranked by decode tok/s (tg):" | tee -a "$LOG"
printf '%s\n' "${results[@]}" | sort -t'|' -k3 -rn | while IFS='|' read -r name pp tg mtp; do
  printf '  %-12s tg=%.1f tok/s  pp=%.1f  mtp=%.4f\n' "$name" "$tg" "$pp" "$mtp"
done | tee -a "$LOG"
