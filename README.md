# malaikat

Personal **AMD Strix Halo** coding inference launcher: **ROCm llama.cpp + MoE + MTP**.

No chat UI. No model store. Config file or CLI → OpenAI-compatible API.

> Prefer **MoE MTP** GGUFs. Default is **Ornith-1.5-35B-A3B** (~3B active). Dense models are bandwidth-bound on this APU.

`runtime.GOOS` selects the platform stack:

| | Windows | Linux |
|--|--|--|
| Runtime | lemonade-sdk `windows-rocm-gfx1151` zip (PATH/DLLs) | llama.cpp built against system ROCm (`gfx1151`) |
| Extra env | `HIP_VISIBLE_DEVICES=0`, high-priority process | `HIP_VISIBLE_DEVICES=0`, `ROCR_VISIBLE_DEVICES=0`, `ROCBLAS_USE_HIPBLASLT=1` |
| Context | `-c 256k\|512k\|1m` (YaRN auto for 512k/1m) | same, **and** `--fit off` so the window is not shrunk |
| Models | `%LOCALAPPDATA%\malaikat\models` | `~/.local/share/malaikat/models` |
| last.yaml | `%AppData%\malaikat\last.yaml` | `~/.config/malaikat/last.yaml` |

## Setup

### Linux (from source)

Requirements: Go 1.22+, ROCm HIP SDK (Arch: `sudo pacman -S --needed base-devel cmake git rocm-hip-sdk rocblas hipblas hipblaslt`).

```bash
go build -o malaikat .
./malaikat setup
```

`setup` clones [llama.cpp](https://github.com/ggml-org/llama.cpp) and compiles natively (`-DGGML_HIP=ON -DAMDGPU_TARGETS=gfx1151`). Flash-attn uses HIP WMMA builtins. `--bundle` falls back to the lemonade Ubuntu zip.

```bash
./malaikat setup --force        # git pull master + rebuild
./malaikat setup --ref b1311    # pin a llama.cpp tag
./malaikat setup --bundle       # lemonade Ubuntu ROCm zip instead
```

Large GGUFs come from unified (GTT) memory; if allocation fails, check `dmesg | grep -i amdgpu`.

### Windows

```powershell
go build -o malaikat.exe .
.\malaikat.exe setup
```

`setup` pulls the [lemonade-sdk/llamacpp-rocm](https://github.com/lemonade-sdk/llamacpp-rocm) **Windows gfx1151** zip into `%LocalAppData%\malaikat\runtime`. All-in-one release builds skip the download and unpack the embedded zip. `--source` is available if you have a HIP SDK, but is not the default.

## Download (all-in-one, no toolchain)

Grab `malaikat-*` from [Releases](https://github.com/thesimonharms/malaikat/releases). It embeds the lemonade ROCm gfx1151 runtime for that OS (not the model):

```powershell
.\malaikat-0.2.0-windows-amd64.exe serve -m path\to\moe-mtp.gguf
```

```bash
chmod +x malaikat-0.2.0-linux-amd64
./malaikat-0.2.0-linux-amd64 serve -m path/to/moe-mtp.gguf
```

Ornith-1.5-35B-A3B GGUF (~22 GB Q4_K_M):

```bash
python3 scripts/download_ornith15.py
```

Previous default, Unsloth Qwen3.6-35B-A3B MTP: `scripts/download_qwen36.py`.

## Serve

```bash
# First time: download, then pick a context at launch
./malaikat serve -config ./coding.yaml -c 256k
./malaikat serve -config ./coding.yaml -c 512k
./malaikat serve -config ./coding.yaml -c 1m

# After that, bare serve reloads the last successful settings:
./malaikat serve
```

| `-c` | Tokens | YaRN |
|------|-------:|------|
| `256k` (default in `coding.yaml`) | 262144 | off — native window |
| `512k` | 524288 | 2× from 262144 |
| `1m` | 1048576 | 4× from 262144 |

512k and 1m pass `--rope-scaling yarn --rope-scale N --yarn-orig-ctx 262144` and raise `qwen35moe.context_length` so llama.cpp does not cap the slot at 256k.

Model paths in YAML may be relative to the platform models dir, or use `~` / `$ENV`. Fallback Qwen3.6 config: `coding-qwen36.yaml`.

On Linux, `serve` passes `--fit off` unless you override it after `--`. Windows lemonade is left as originally shipped.

API: `http://127.0.0.1:8080/v1`

Shared defaults: `-ngl 999`, `-b/-ub 2048/1024`, `-fa on`, `--cache-type-k/v q8_0`, `--jinja`, `--spec-type draft-mtp --spec-draft-n-max 3`, `HIP_VISIBLE_DEVICES=0`.

Disable MTP: `-no-mtp`. Draft depth: `-spec-draft-n-max N`.

## Bench vs Ollama

```bash
# terminal A
./malaikat serve -config ./coding.yaml -c 256k

# terminal B
./malaikat bench -url http://127.0.0.1:8080 -ollama qwen3.6:35b-a3b -n 128
```

Optional kernel microbench: `bench -kernel -m path/to/model.gguf`.

Measured on Linux (system ROCm HIP gfx1151, Qwen3.6-35B-A3B MTP UD-Q4_K_XL, **ctx 262144**):

| Path | tok/s |
|------|------:|
| malaikat MTP n=3 + FA on + KV q8 + 2048/1024 | ~79 tg (short), ~830 pp @ 6.4k |
| malaikat MTP n=2 | ~71 tg |
| malaikat no MTP | ~40 tg |

Windows (same model, lemonade ROCm, 128 completion tokens): tuned MTP n=3 + FA + KV q8 ~73 tok/s vs Ollama ~51.

Re-sweep: `scripts/sweep-speed.sh` (Linux) or `scripts/sweep-speed.ps1` (Windows).

## Config

JSON or YAML. See [`coding.example.yaml`](coding.example.yaml). CLI overrides the file. `ctx_size` accepts `256k`, `512k`, `1m`, or a raw token count.

## Release build

```bash
scripts/release.sh              # → dist/malaikat-0.2.0-linux-amd64
scripts/release.sh --publish    # tag v0.2.0 + GitHub release (needs gh auth)
```

```powershell
.\scripts\release.ps1           # → dist\malaikat-0.2.0-windows-amd64.exe
```
