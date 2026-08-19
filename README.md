# malaikat

Personal **AMD Strix Halo** coding inference launcher for **Linux**: **ROCm llama.cpp + MoE + MTP**.

No chat UI. No model store. Config file or CLI → OpenAI-compatible API.

> Prefer **MoE MTP** GGUFs (e.g. Qwen3.6-35B-A3B-MTP). Dense models are bandwidth-bound on this APU.

## Download (all-in-one)

Grab `malaikat-*-linux-amd64` from [Releases](https://github.com/thesimonharms/malaikat/releases). It embeds the lemonade ROCm gfx1151 runtime (not the model):

```bash
chmod +x malaikat-0.2.0-linux-amd64
./malaikat-0.2.0-linux-amd64 serve -m path/to/moe-mtp.gguf
```

First run extracts the runtime to `~/.cache/malaikat/runtime`.

## Setup (from source)

```bash
go build -o malaikat .
./malaikat setup
```

`setup` pulls the latest [lemonade-sdk/llamacpp-rocm](https://github.com/lemonade-sdk/llamacpp-rocm) **Ubuntu gfx1151** zip (ROCm runtime bundled) into `~/.cache/malaikat/runtime`. All-in-one builds skip the download and unpack the embedded zip instead. Serving puts the bundled ROCm libs on `LD_LIBRARY_PATH` automatically — a system ROCm install is not required, but the kernel driver (`/dev/kfd`) is.

Large GGUFs are allocated from unified (GTT) memory; if llama-server fails to allocate for a big model, check `dmesg | grep -i amdgpu` for GTT limits.

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

Model paths may use `~` and `$ENV` vars. Default model dir: `~/.local/share/malaikat/models` (see `scripts/download_qwen38_27b.py`).

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

Throughput numbers on Linux differ from the old Windows runs — re-sweep after runtime upgrades with `scripts/sweep-speed.sh`.

## Config

JSON or YAML. See [`coding.example.yaml`](coding.example.yaml). CLI overrides the file.

## Release build

```bash
scripts/release.sh              # → dist/malaikat-0.2.0-linux-amd64
scripts/release.sh --publish    # tag v0.2.0 + GitHub release (needs gh auth)
```
