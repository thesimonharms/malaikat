# malaikat

Personal **AMD Strix Halo** coding inference launcher for **Linux**: **ROCm llama.cpp + MoE + MTP**.

No chat UI. No model store. Config file or CLI → OpenAI-compatible API.

> Prefer **MoE MTP** GGUFs (e.g. Qwen3.6-35B-A3B-MTP). Dense models are bandwidth-bound on this APU.

## Setup (from source)

Requirements: Go 1.22+, and the ROCm HIP SDK (Arch: `sudo pacman -S --needed base-devel cmake git rocm-hip-sdk rocblas hipblas hipblaslt`).

```bash
go build -o malaikat .
./malaikat setup
```

`setup` clones [llama.cpp](https://github.com/ggml-org/llama.cpp) into `~/.cache/malaikat/llama.cpp-src` and builds it natively against the system ROCm (`-DGGML_HIP=ON -DAMDGPU_TARGETS=gfx1151`). Flash-attn uses HIP WMMA builtins on gfx1151. Serve sets `ROCBLAS_USE_HIPBLASLT=1` so rocBLAS uses hipBLASLt's tuned gfx1151 GEMMs. No bundled runtime, no container, links straight against `/opt/rocm`.

Useful variants:

```bash
./malaikat setup --force        # git pull master + rebuild
./malaikat setup --ref b1311    # pin a known-good llama.cpp tag
./malaikat setup --bundle       # fallback: prebuilt lemonade-sdk Ubuntu ROCm zip instead
```

Large GGUFs are allocated from unified (GTT) memory; if llama-server fails to allocate for a big model, check `dmesg | grep -i amdgpu` for GTT limits.

## Download (all-in-one, no ROCm toolchain)

For machines *without* a ROCm install, `malaikat-*-linux-amd64` from [Releases](https://github.com/thesimonharms/malaikat/releases) embeds the lemonade ROCm gfx1151 bundle (not the model) and extracts it to `~/.cache/malaikat/runtime` on first run:

```bash
chmod +x malaikat-0.2.0-linux-amd64
./malaikat-0.2.0-linux-amd64 serve -m path/to/moe-mtp.gguf
```

## Serve

```bash
# First time (or when changing models): pass a config / flags once
./malaikat serve -config ./coding.yaml

# After that, bare serve reloads the last successful settings:
./malaikat serve
```

Last settings are stored at `~/.config/malaikat/last.yaml`. Use `-no-save` to skip updating them. Passthrough extra llama-server flags after `--` (also remembered):

```bash
./malaikat serve -m ./model.gguf -- --cache-type-k q8_0
```

Model paths may use `~` and `$ENV` vars. Default model dir: `~/.local/share/malaikat/models` (see `scripts/download_qwen36.py`).

`serve` always passes `--fit off` (unless you override it after `--`) so llama.cpp cannot silently shrink `ctx_size: 0` (model max, 262144 for Qwen3.6) to leave a device-memory margin.

API: `http://127.0.0.1:8080/v1`

Defaults (Strix Halo ROCm): `-ngl 999`, `-b/-ub 2048/1024` (coding.yaml; better long-prompt pp), `-fa on`, `--cache-type-k/v q8_0`, `--jinja`, `--spec-type draft-mtp --spec-draft-n-max 3`, `HIP_VISIBLE_DEVICES=0` + `ROCR_VISIBLE_DEVICES=0`.

Disable MTP: `-no-mtp`. Draft depth: `-spec-draft-n-max N`.

## Bench vs Ollama

```bash
# terminal A
./malaikat serve -config ./coding.yaml

# terminal B
./malaikat bench -url http://127.0.0.1:8080 -ollama qwen3.6:35b-a3b -n 128
```

Optional kernel microbench: `bench -kernel -m path/to/model.gguf`.

Measured on this machine (Linux, llama.cpp built against system ROCm HIP gfx1151, Qwen3.6-35B-A3B MTP UD-Q4_K_XL, **ctx 262144**):

| Path | tok/s |
|------|------:|
| malaikat MTP n=3 + FA on + KV q8 + 2048/1024 | ~79 tg (short), ~830 pp @ 6.4k |
| malaikat MTP n=2 | ~71 tg |
| malaikat no MTP | ~40 tg |

Use `scripts/sweep-speed.sh` to re-sweep after runtime upgrades.

## Config

JSON or YAML. See [`coding.example.yaml`](coding.example.yaml). CLI overrides the file.

## Release build

```bash
scripts/release.sh              # → dist/malaikat-0.2.0-linux-amd64
scripts/release.sh --publish    # tag v0.2.0 + GitHub release (needs gh auth)
```
