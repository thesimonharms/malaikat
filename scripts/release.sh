#!/usr/bin/env bash
# Build an all-in-one malaikat binary (launcher + lemonade ROCm gfx1151 runtime).
# Model GGUFs are NOT bundled.
#
# Usage:
#   scripts/release.sh                 # build only -> dist/malaikat-<ver>-linux-amd64
#   scripts/release.sh --publish       # build, tag, and upload GitHub release (needs gh auth)
#
# Env overrides: VERSION, TAG, LLAMA_TAG.
set -euo pipefail

VERSION="${VERSION:-0.2.0}"
TAG="${TAG:-}"
LLAMA_TAG="${LLAMA_TAG:-}"
PUBLISH=0
SKIP_DOWNLOAD=0

while [[ $# -gt 0 ]]; do
  case "$1" in
    --publish)       PUBLISH=1 ;;
    --skip-download) SKIP_DOWNLOAD=1 ;;
    --version)       VERSION="$2"; shift ;;
    --llama-tag)     LLAMA_TAG="$2"; shift ;;
    *) echo "unknown arg: $1" >&2; exit 2 ;;
  esac
  shift
done
[[ -n "$TAG" ]] || TAG="v$VERSION"

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

EMBED_DIR="internal/engine/embedded"
ZIP_PATH="$EMBED_DIR/runtime.zip"
TAG_PATH="$EMBED_DIR/runtime.tag"
DIST_DIR="dist"
OUT_NAME="malaikat-$VERSION-linux-amd64"
OUT_PATH="$DIST_DIR/$OUT_NAME"

mkdir -p "$EMBED_DIR" "$DIST_DIR"

if [[ "$SKIP_DOWNLOAD" == "0" || ! -f "$ZIP_PATH" ]]; then
  echo "Resolving lemonade-sdk/llamacpp-rocm Ubuntu gfx1151 asset..."
  if [[ -n "$LLAMA_TAG" ]]; then
    api="repos/lemonade-sdk/llamacpp-rocm/releases/tags/$LLAMA_TAG"
  else
    api="repos/lemonade-sdk/llamacpp-rocm/releases/latest"
  fi
  rel_json="$(curl -fsSL -H 'Accept: application/vnd.github+json' "https://api.github.com/$api")"
  resolved="$(printf '%s' "$rel_json" | python3 -c '
import json, sys
rel = json.load(sys.stdin)
for a in rel["assets"]:
    if "ubuntu-rocm-gfx1151-x64.zip" in a["name"]:
        print(rel["tag_name"], a["name"], a["browser_download_url"])
        break
else:
    sys.exit("no ubuntu-rocm-gfx1151-x64.zip asset in release " + rel.get("tag_name", "?"))
')"
  read -r LLAMA_TAG asset url <<<"$resolved"
  [[ -n "${asset:-}" && -n "${url:-}" ]] || { echo "asset resolution failed" >&2; exit 1; }
  echo "Downloading $asset from $LLAMA_TAG..."
  curl -fSL -o "$ZIP_PATH" "$url"
  printf '%s' "$LLAMA_TAG" > "$TAG_PATH"
else
  [[ -f "$TAG_PATH" ]] || { echo "Missing $TAG_PATH (re-run without --skip-download)" >&2; exit 1; }
  LLAMA_TAG="$(tr -d '[:space:]' < "$TAG_PATH")"
  echo "Using cached runtime.zip (tag $LLAMA_TAG)"
fi

echo "Building all-in-one $OUT_NAME (embedruntime, llama $LLAMA_TAG)..."
CGO_ENABLED=0 go build -tags embedruntime -trimpath -ldflags "-s -w" -o "$OUT_PATH" .

size_mb="$(du -m "$OUT_PATH" | cut -f1)"
echo "Built $OUT_PATH (${size_mb} MB)"

"$OUT_PATH" version

cp -f coding.example.yaml "$DIST_DIR/coding.example.yaml"

if [[ "$PUBLISH" == "0" ]]; then
  echo
  echo "Build only. To publish: scripts/release.sh --publish"
  exit 0
fi

echo "Publishing GitHub release $TAG..."
if ! git tag -l "$TAG" | grep -q .; then
  git tag -a "$TAG" -m "malaikat $VERSION"
fi
git push origin "$TAG" || true

notes="$(cat <<EOF
## malaikat $VERSION

All-in-one Linux binary for AMD Strix Halo (gfx1151) ROCm inference.

**Bundled:** malaikat launcher + lemonade-sdk \`llamacpp-rocm\` \`$LLAMA_TAG\` (Ubuntu ROCm gfx1151).
**Not bundled:** GGUF model weights — pass \`-m\` or a config.

### Quick start

\`\`\`bash
chmod +x $OUT_NAME
./$OUT_NAME serve -m path/to/moe-mtp.gguf
\`\`\`

First run extracts the ROCm runtime to \`~/.cache/malaikat/runtime\`. Optional config: see \`coding.example.yaml\` in this release.

API: \`http://127.0.0.1:8080/v1\`
EOF
)"

if gh release view "$TAG" >/dev/null 2>&1; then
  echo "Release $TAG exists; uploading asset..."
  gh release upload "$TAG" "$OUT_PATH" "$DIST_DIR/coding.example.yaml" --clobber
else
  gh release create "$TAG" "$OUT_PATH" "$DIST_DIR/coding.example.yaml" --title "malaikat $VERSION" --notes "$notes"
fi

echo "Done: https://github.com/thesimonharms/malaikat/releases/tag/$TAG"
