# malaikat

> **Status: untested.** Scaffolded against public Strix Halo / llama.cpp Vulkan guidance. `doctor` and `setup` have been smoke-checked on one Windows Ryzen AI Max+ 395 machine; end-to-end `serve` / `chat` / `bench` with real GGUF workloads has **not** been validated. Expect sharp edges.

Local LLM runner for **AMD Strix Halo** (Ryzen AI Max / Radeon 8060S) on **Windows**, tuned for maximum practical speed.

It does not reimplement inference in Go. It orchestrates the fastest proven stack on this hardware:

**latest [llama.cpp](https://github.com/ggml-org/llama.cpp) + Vulkan (AMD Adrenalin)** with Strix Halo–specific defaults.

## Why this stack

| Choice | Reason |
|--------|--------|
| Vulkan (not HIP/ROCm first) | Best generation throughput and stability on Strix Halo Windows; zero ROCm wrangling |
| Latest llama.cpp build | AMD graphics-queue / FlashAttention Vulkan fixes moved MoE speed more than batch tuning |
| `-ngl 999` | Keep the full model in the iGPU unified-memory pool after VGM is raised |
| `-b 512 -ub 512` | Community sweeps on gfx1151 show ~512 is optimal |
| Flash attention on | Helps MoE models on recent Vulkan builds |
| High process priority | Reduces Windows preemption during token generation |
| Prefer MoE GGUFs | Dense 70B is bandwidth-bound (~4–6 t/s); 30B-A3B MoE can hit ~100 t/s |

## Prerequisites

1. **AMD Software: Adrenalin Edition** (Vulkan ICD)
2. **Variable Graphics Memory (VGM)**  
   Adrenalin → Performance → Tuning → Variable Graphics Memory → **Custom**  
   On 128 GB systems, AMD’s common guidance is **~96 GB** (leave ~32 GB for Windows). **Reboot** after changing.
3. A **GGUF** model on disk
4. Go 1.22+ to build this CLI (or use a prebuilt `malaikat.exe`)

## Install

```powershell
cd malaikat
go build -o malaikat.exe .
.\malaikat.exe doctor
.\malaikat.exe setup
```

`setup` downloads the latest `llama-*-bin-win-vulkan-x64.zip` from llama.cpp GitHub releases into your user cache.

## Usage

```powershell
# Hardware + VGM + Vulkan readiness
.\malaikat.exe doctor

# Model suggestions for your RAM / VGM budget
.\malaikat.exe models

# OpenAI-compatible server (http://127.0.0.1:8080/v1)
.\malaikat.exe serve -m C:\models\qwen3-30b-a3b-q4_k_m.gguf

# Interactive chat
.\malaikat.exe chat -m C:\models\qwen3-30b-a3b-q4_k_m.gguf

# Throughput check
.\malaikat.exe bench -m C:\models\qwen3-30b-a3b-q4_k_m.gguf
```

Persist the model path:

```powershell
.\malaikat.exe serve -m C:\models\model.gguf --save
```

Config lives at `%AppData%\malaikat\config.json`. Runtime binaries live under `%LocalAppData%\malaikat\runtime\`.

## Suggested models (128 GB / ~96 GB VGM)

| Goal | Model | Expect |
|------|--------|--------|
| Speed | Qwen3-30B-A3B Q4_K_M/S | ~100 t/s |
| Capacity | gpt-oss-120b MXFP4 | ~55 t/s |
| Dense quality | Llama-3.3-70B Q4_K_M | ~4–6 t/s |

## API example

```powershell
curl http://127.0.0.1:8080/v1/chat/completions `
  -H "Content-Type: application/json" `
  -d '{"messages":[{"role":"user","content":"Hello"}],"temperature":0.7}'
```

## Design notes

- Windows WMI/PowerShell detection for Ryzen AI Max / Radeon 8060S / gfx1151-class parts
- Env `HIP_VISIBLE_DEVICES=-1` so HIP does not fight Vulkan when both are installed
- `GGML_VK_VISIBLE_DEVICES=0` pins the first Vulkan device
- Always prefer `malaikat setup` again after major llama.cpp releases — updates beat micro-tuning on this APU
