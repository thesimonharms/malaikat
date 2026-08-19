#!/usr/bin/env bash
# Restart malaikat with flag variants and record tok/s.
# Usage: scripts/sweep-speed.sh
# Env overrides: MODEL, EXE, PORT.
set -uo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

MODEL="${MODEL:-$HOME/.local/share/malaikat/models/Qwen3.6-35B-A3B-MTP/Qwen3.6-35B-A3B-UD-Q4_K_XL.gguf}"
EXE="${EXE:-$ROOT/malaikat}"
PORT="${PORT:-8080}"
PROMPT="Write a Python function that merges two sorted lists. Only code."
TMP="${TMPDIR:-/tmp}"

if [[ ! -x "$EXE" ]]; then
  echo "building $EXE ..."
  go build -o "$EXE" .
fi
if [[ ! -f "$MODEL" ]]; then
  echo "model not found: $MODEL (set MODEL=...)" >&2
  exit 1
fi

SRV_PID=""

stop_server() {
  pkill -f 'llama-server' 2>/dev/null || true
  if [[ -n "$SRV_PID" ]] && kill -0 "$SRV_PID" 2>/dev/null; then
    kill "$SRV_PID" 2>/dev/null || true
  fi
  SRV_PID=""
  sleep 2
}
trap stop_server EXIT

start_server() {
  stop_server
  "$EXE" serve -m "$MODEL" -c 8192 -port "$PORT" "$@" \
    >"$TMP/malaikat-sweep-out.txt" 2>"$TMP/malaikat-sweep-err.txt" &
  SRV_PID=$!
  local deadline=$((SECONDS + 480))
  while (( SECONDS < deadline )); do
    if curl -fsS -o /dev/null -m 2 "http://127.0.0.1:$PORT/health" 2>/dev/null; then
      return 0
    fi
    if ! kill -0 "$SRV_PID" 2>/dev/null; then
      echo "server exited early; see $TMP/malaikat-sweep-err.txt" >&2
      return 1
    fi
    sleep 0.5
  done
  echo "server not ready" >&2
  return 1
}

bench_once() {
  local body
  body="$(PROMPT="$PROMPT" python3 - <<'PY'
import json, os
print(json.dumps({
    "messages": [{"role": "user", "content": os.environ["PROMPT"]}],
    "max_tokens": 128, "temperature": 0.0, "stream": False,
}))
PY
)"
  # warmup
  curl -fsS -o /dev/null -m 600 -H 'Content-Type: application/json' \
    -d "$body" "http://127.0.0.1:$PORT/v1/chat/completions" || return 1
  local tg sum=0 n=0
  for _ in 1 2; do
    tg="$(curl -fsS -m 600 -H 'Content-Type: application/json' \
      -d "$body" "http://127.0.0.1:$PORT/v1/chat/completions" \
      | python3 -c 'import json,sys; print(json.load(sys.stdin).get("timings", {}).get("predicted_per_second") or 0)')"
    sum="$(python3 -c "print($sum + ($tg))")"
    n=$((n + 1))
  done
  python3 -c "print(f'{$sum / $n:.1f}')"
}

variants=(
  "baseline_n2|-spec-draft-n-max 2"
  "n3|-spec-draft-n-max 3"
  "n4|-spec-draft-n-max 4"
  "n3_fa_on|-spec-draft-n-max 3 -fa on"
  "n3_fa_off|-spec-draft-n-max 3 -fa off"
  "n3_kv_q8|-spec-draft-n-max 3 -- --cache-type-k q8_0 --cache-type-v q8_0"
  "n3_fa_kv_q8|-spec-draft-n-max 3 -fa on -- --cache-type-k q8_0 --cache-type-v q8_0"
  "n3_fa_kv_q4|-spec-draft-n-max 3 -fa on -- --cache-type-k q4_0 --cache-type-v q4_0"
  "n3_fa_kv_q8_b2048|-spec-draft-n-max 3 -fa on -b 2048 -ub 512 -- --cache-type-k q8_0 --cache-type-v q8_0"
  "no_mtp|-no-mtp"
)

results=()
for v in "${variants[@]}"; do
  name="${v%%|*}"
  argstr="${v#*|}"
  echo "=== $name ==="
  read -ra vargs <<<"$argstr"
  if start_server "${vargs[@]}"; then
    if tps="$(bench_once)"; then
      echo "$name: $tps tok/s"
      results+=("$name|$tps|$argstr")
    else
      echo "$name: FAIL (bench)" >&2
      results+=("$name|-1|$argstr")
    fi
  else
    echo "$name: FAIL (start)" >&2
    results+=("$name|-1|$argstr")
  fi
done

echo
echo "RESULTS"
printf '%s\n' "${results[@]}" | sort -t'|' -k2 -rn | while IFS='|' read -r name tps args; do
  printf '%-22s %8s tok/s   %s\n' "$name" "$tps" "$args"
done

printf '%s\n' "${results[@]}" | python3 -c '
import json, sys
rows = []
for line in sys.stdin:
    name, tps, args = line.rstrip("\n").split("|", 2)
    rows.append({"name": name, "tps": float(tps), "args": args})
print(json.dumps(rows, indent=2))
' > sweep-results.json
echo "wrote sweep-results.json"
