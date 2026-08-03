# malaikat

Personal **AMD Strix Halo** coding inference launcher: **ROCm llama.cpp + MoE + MTP**.

No chat UI. No model store. Config file or CLI → OpenAI-compatible API.

> Prefer **MoE MTP** GGUFs (e.g. Qwen3.6-35B-A3B-MTP). Dense models are bandwidth-bound on this APU.

## Setup

```powershell
go build -o malaikat.exe .
.\malaikat.exe setup
```

`setup` pulls the latest [lemonade-sdk/llamacpp-rocm](https://github.com/lemonade-sdk/llamacpp-rocm) **Windows gfx1151** zip (ROCm runtime bundled) into `%LocalAppData%\malaikat\runtime`.

## Serve

```powershell
.\malaikat.exe serve -config .\coding.yaml -m D:\models\Qwen3.6-35B-A3B-MTP-Q4_K_M.gguf
```

Or put `model:` in the YAML. Passthrough extra llama-server flags after `--`:

```powershell
.\malaikat.exe serve -m .\model.gguf -- --cache-type-k q8_0
```

API: `http://127.0.0.1:8080/v1`

Defaults (swept on Strix Halo ROCm): `-ngl 999`, `-b/-ub 512`, `-fa on`, `--cache-type-k/v q8_0`, `--jinja`, `--spec-type draft-mtp --spec-draft-n-max 3`, `HIP_VISIBLE_DEVICES=0`.

Disable MTP: `-no-mtp`. Draft depth: `-spec-draft-n-max N`.

## Bench vs Ollama

```powershell
# terminal A
.\malaikat.exe serve -config .\coding.yaml -m path\to\moe-mtp.gguf

# terminal B
.\malaikat.exe bench -url http://127.0.0.1:8080 -ollama qwen3.6:35b-a3b -n 128
```

Optional kernel microbench: `bench -kernel -m path\to\model.gguf`.

Measured on this machine (Qwen3.6-35B-A3B MTP UD-Q4_K_XL, 128 completion tokens):

| Path | tok/s |
|------|------:|
| malaikat tuned (MTP n=3 + FA + KV q8) | ~73 |
| malaikat MTP n=2 baseline | ~68 |
| malaikat no MTP | ~51 |
| Ollama `qwen3.6:35b-a3b` (warm) | ~51 |

Use `scripts/sweep-speed.ps1` to re-sweep after runtime upgrades.

## Config

JSON or YAML. See [`coding.yaml`](coding.yaml). CLI overrides the file.
