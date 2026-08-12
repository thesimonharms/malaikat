# malaikat

Personal **AMD Strix Halo** coding inference launcher: **ROCm llama.cpp + MoE + MTP**.

No chat UI. No model store. Config file or CLI → OpenAI-compatible API.

> Prefer **MoE MTP** GGUFs (e.g. Qwen3.6-35B-A3B-MTP). Dense models are bandwidth-bound on this APU.

## Download (all-in-one)

Grab `malaikat-*-windows-amd64.exe` from [Releases](https://github.com/thesimonharms/malaikat/releases). It embeds the lemonade ROCm gfx1151 runtime (not the model):

```powershell
.\malaikat-0.1.0-windows-amd64.exe serve -m path\to\moe-mtp.gguf
```

First run extracts the runtime to `%LocalAppData%\malaikat\runtime`.

## Setup (from source)

```powershell
go build -o malaikat.exe .
.\malaikat.exe setup
```

`setup` pulls the latest [lemonade-sdk/llamacpp-rocm](https://github.com/lemonade-sdk/llamacpp-rocm) **Windows gfx1151** zip (ROCm runtime bundled) into `%LocalAppData%\malaikat\runtime`. All-in-one builds skip the download and unpack the embedded zip instead.

## Serve

```powershell
# First time (or when changing models): pass a config / flags once
.\malaikat.exe serve -config .\coding.yaml

# After that, bare serve reloads the last successful settings:
.\malaikat.exe serve
```

Last settings are stored at `%AppData%\malaikat\last.yaml`. Use `-no-save` to skip updating them. Passthrough extra llama-server flags after `--` (also remembered):

```powershell
.\malaikat.exe serve -m .\model.gguf -- --cache-type-k q8_0
```

API: `http://127.0.0.1:8080/v1`

Defaults (swept on Strix Halo ROCm): `-ngl 999`, `-b/-ub 2048/1024` (coding.yaml; better long-prompt pp), `-fa on`, `--cache-type-k/v q8_0`, `--jinja`, `--spec-type draft-mtp --spec-draft-n-max 3`, `HIP_VISIBLE_DEVICES=0`.

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

JSON or YAML. See [`coding.example.yaml`](coding.example.yaml). CLI overrides the file.

## Release build

```powershell
.\scripts\release.ps1              # → dist\malaikat-0.1.0-windows-amd64.exe
.\scripts\release.ps1 -Publish     # tag v0.1.0 + GitHub release
```
