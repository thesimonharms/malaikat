#!/usr/bin/env bash
# Restart malaikat with flag/env variants and record tok/s.
# Always uses ctx_size 0 (model max) and -no-save so last.yaml is untouched.
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
  pkill -x llama-server 2>/dev/null || true
  if [[ -n "$SRV_PID" ]] && kill -0 "$SRV_PID" 2>/dev/null; then
    kill "$SRV_PID" 2>/dev/null || true
  fi
  SRV_PID=""
  sleep 3
}
trap stop_server EXIT

start_server() {
  local extra_env="$1"
  shift
  stop_server
  # shellcheck disable=SC2086
  env $extra_env "$EXE" serve -config "$ROOT/coding.yaml" -m "$MODEL" -c 0 -port "$PORT" -no-save "$@" \
    >"$TMP/malaikat-sweep-out.txt" 2>"$TMP/malaikat-sweep-err.txt" &
  SRV_PID=$!
  local deadline=$((SECONDS + 900))
  while (( SECONDS < deadline )); do
    if curl -fsS -o /dev/null -m 2 "http://127.0.0.1:$PORT/health" 2>/dev/null; then
      return 0
    fi
    if ! kill -0 "$SRV_PID" 2>/dev/null; then
      echo "server exited early; see $TMP/malaikat-sweep-err.txt" >&2
      tail -n 40 "$TMP/malaikat-sweep-err.txt" >&2 || true
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
  curl -fsS -o /dev/null -m 600 -H 'Content-Type: application/json' \
    -d "$body" "http://127.0.0.1:$PORT/v1/chat/completions" || return 1
  local tg pp sum_tg=0 sum_pp=0 n=0
  for _ in 1 2; do
    read -r tg pp <<<"$(curl -fsS -m 600 -H 'Content-Type: application/json' \
      -d "$body" "http://127.0.0.1:$PORT/v1/chat/completions" \
      | python3 -c 'import json,sys; t=json.load(sys.stdin).get("timings") or {}; print((t.get("predicted_per_second") or 0), (t.get("prompt_per_second") or 0))')"
    sum_tg="$(python3 -c "print($sum_tg + ($tg))")"
    sum_pp="$(python3 -c "print($sum_pp + ($pp))")"
    n=$((n + 1))
  done
  python3 -c "print(f'{$sum_tg / $n:.1f} {$sum_pp / $n:.1f}')"
}

# name|env|args   (env may be empty; args may be empty)
variants=(
  "baseline_n3|| -spec-draft-n-max 3 -fa on -b 2048 -ub 1024"
  "n2|| -spec-draft-n-max 2 -fa on -b 2048 -ub 1024"
  "n4|| -spec-draft-n-max 4 -fa on -b 2048 -ub 1024"
  "n5|| -spec-draft-n-max 5 -fa on -b 2048 -ub 1024"
  "fa_off|| -spec-draft-n-max 3 -fa off -b 2048 -ub 1024"
  "kv_q4|| -spec-draft-n-max 3 -fa on -b 2048 -ub 1024 -- --cache-type-k q4_0 --cache-type-v q4_0"
  "ub512|| -spec-draft-n-max 3 -fa on -b 2048 -ub 512"
  "b4096_ub1024|| -spec-draft-n-max 3 -fa on -b 4096 -ub 1024"
  "sdma0|HSA_ENABLE_SDMA=0| -spec-draft-n-max 3 -fa on -b 2048 -ub 1024"
  "xnack|HSA_XNACK=1| -spec-draft-n-max 3 -fa on -b 2048 -ub 1024"
  "sdma0_xnack|HSA_ENABLE_SDMA=0 HSA_XNACK=1| -spec-draft-n-max 3 -fa on -b 2048 -ub 1024"
  "pmin08|| -spec-draft-n-max 3 -fa on -b 2048 -ub 1024 -- --spec-draft-p-min 0.8"
  "no_mtp|| -no-mtp -fa on -b 2048 -ub 1024"
)

results=()
for v in "${variants[@]}"; do
  name="${v%%|*}"
  rest="${v#*|}"
  extra_env="${rest%%|*}"
  argstr="${rest#*|}"
  echo "=== $name ==="
  echo "  env: ${extra_env:-<none>}"
  echo "  args:$argstr"
  read -ra vargs <<<"$argstr"
  if start_server "$extra_env" "${vargs[@]}"; then
    if rates="$(bench_once)"; then
      tg="${rates%% *}"
      pp="${rates#* }"
      echo "$name: tg=$tg tok/s  pp=$pp tok/s"
      results+=("$name|$tg|$pp|$extra_env|$argstr")
    else
      echo "$name: FAIL (bench)" >&2
      results+=("$name|-1|-1|$extra_env|$argstr")
    fi
  else
    echo "$name: FAIL (start)" >&2
    results+=("$name|-1|-1|$extra_env|$argstr")
  fi
done

echo
echo "RESULTS ranked by tg"
printf '%s\n' "${results[@]}" | sort -t'|' -k2 -rn | while IFS='|' read -r name tps pp env args; do
  printf '%-18s tg=%8s  pp=%8s   env=%s  %s\n' "$name" "$tps" "$pp" "$env" "$args"
done

printf '%s\n' "${results[@]}" | python3 -c '
import json, sys
rows = []
for line in sys.stdin:
    name, tps, pp, env, args = line.rstrip("\n").split("|", 4)
    rows.append({"name": name, "tg": float(tps), "pp": float(pp), "env": env, "args": args})
print(json.dumps(rows, indent=2))
' > sweep-results.json
echo "wrote sweep-results.json"
