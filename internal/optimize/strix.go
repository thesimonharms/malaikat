package optimize

import (
	"fmt"
	"runtime"

	"github.com/simon/malaikat/internal/config"
	"github.com/simon/malaikat/internal/hw"
)

// Profile holds Strix Halo–tuned llama.cpp knobs.
type Profile struct {
	Backend      string
	NGL          int
	Batch        int
	UBatch       int
	CtxSize      int
	Threads      int
	FlashAttn    bool
	Jinja        bool
	Parallel     int
	HighPriority bool
	ExtraEnv     map[string]string
	Rationale    []string
}

// ForHost builds an optimized profile for the detected hardware.
func ForHost(info hw.Info, cfg config.Config) Profile {
	p := Profile{
		Backend:      "vulkan",
		NGL:          cfg.NGL,
		Batch:        cfg.Batch,
		UBatch:       cfg.UBatch,
		CtxSize:      cfg.CtxSize,
		Threads:      cfg.Threads,
		FlashAttn:    cfg.FlashAttn,
		Jinja:        cfg.Jinja,
		Parallel:     cfg.Parallel,
		HighPriority: cfg.HighPriority,
		ExtraEnv:     map[string]string{},
		Rationale:    []string{},
	}

	if p.NGL <= 0 {
		p.NGL = config.DefaultNGL
	}
	if p.Batch <= 0 {
		p.Batch = config.DefaultBatch
	}
	if p.UBatch <= 0 {
		p.UBatch = config.DefaultUBatch
	}
	if p.Parallel <= 0 {
		p.Parallel = 1
	}

	// Token generation on Strix Halo is memory-bandwidth bound. Keep enough
	// CPU threads for the residual host path, but leave headroom for the GPU
	// driver and OS compositor on Windows.
	if p.Threads <= 0 {
		n := runtime.NumCPU()
		if info.IsStrixHalo && n >= 16 {
			p.Threads = n - 4
		} else if n > 2 {
			p.Threads = n - 1
		} else {
			p.Threads = n
		}
	}

	p.Rationale = append(p.Rationale,
		"Backend=Vulkan: best generation throughput / stability on Strix Halo Windows (AMD Adrenalin ICD).",
		fmt.Sprintf("n-gpu-layers=%d: keep weights on the iGPU's unified memory pool.", p.NGL),
		fmt.Sprintf("batch=%d ubatch=%d: community sweeps show ~512 is optimal on gfx1151 Vulkan.", p.Batch, p.UBatch),
	)
	if p.FlashAttn {
		p.Rationale = append(p.Rationale, "flash-attn on: Wave32 FA path helps MoE models on recent llama.cpp Vulkan builds.")
	}
	if info.IsStrixHalo {
		p.Rationale = append(p.Rationale,
			"Prefer MoE GGUFs (active params << total) to escape the ~215 GB/s bandwidth ceiling.",
			"Always use the newest llama.cpp Vulkan build — AMD graphics-queue / FA fixes moved the needle more than batch tuning.",
		)
		// Avoid HIP competing with Vulkan if both stacks are installed.
		p.ExtraEnv["HIP_VISIBLE_DEVICES"] = "-1"
		p.ExtraEnv["GGML_VK_VISIBLE_DEVICES"] = "0"
	}

	return p
}

// SuggestModels returns practical GGUF targets for the machine's RAM budget.
func SuggestModels(info hw.Info) []string {
	ram := info.TotalRAMGB
	vgm := float64(info.RecommendedVGMGB())
	budget := vgm
	if info.GPUMemoryMB > 0 {
		budget = float64(info.GPUMemoryMB) / 1024
	}
	if budget < 8 {
		budget = ram * 0.5
	}

	out := []string{
		"Speed: Qwen/Qwen3-30B-A3B GGUF Q4_K_M or Q4_K_S (~17–20 GB) — often ~100 t/s on Strix Halo Vulkan",
	}
	if budget >= 48 || ram >= 96 {
		out = append(out, "Balanced MoE: Qwen3-35B-A3B / Qwen3-Coder-30B-A3B Q4–Q5 GGUF")
	}
	if budget >= 72 || ram >= 96 {
		out = append(out, "Quality/capacity: gpt-oss-120b MXFP4 GGUF (~60–65 GB) — needs ~96 GB VGM class, ~55 t/s")
	}
	if budget >= 44 || ram >= 64 {
		out = append(out, "Dense fallback: Llama-3.3-70B Q4_K_M (~40 GB) — works, but expect ~4–6 t/s (bandwidth-bound)")
	}
	out = append(out, "Tiny smoke test: any ~1–3B Q4/Q8 GGUF to validate the Vulkan path before large downloads")
	return out
}
