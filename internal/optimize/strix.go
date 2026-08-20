package optimize

import (
	"runtime"

	"github.com/simon/malaikat/internal/config"
)

// Profile holds Strix Halo ROCm + MTP knobs for llama-server.
type Profile struct {
	Backend       string
	NGL           int
	Batch         int
	UBatch        int
	CtxSize       int
	NativeCtx     int
	RopeArch      string
	YarnScale     float64 // 0 = no YaRN; else target/native
	YarnOrigCtx   int
	Alias         string
	Threads       int
	FlashAttn     string // on|off|auto|""; empty omits flag
	CacheTypeK    string
	CacheTypeV    string
	Jinja         bool
	Parallel      int
	SpecType      string
	SpecDraftNMax int
	HighPriority  bool
	FitOff        bool
	ExtraArgs     []string
	ExtraEnv      map[string]string
}

// FromConfig builds the coding inference profile.
func FromConfig(cfg config.Config) Profile {
	p := Profile{
		Backend:       "rocm",
		NGL:           cfg.NGL,
		Batch:         cfg.Batch,
		UBatch:        cfg.UBatch,
		CtxSize:       cfg.CtxSize.Int(),
		NativeCtx:     cfg.NativeCtx,
		RopeArch:      cfg.RopeArch,
		Alias:         cfg.Alias,
		Threads:       cfg.Threads,
		FlashAttn:     cfg.FlashAttn,
		CacheTypeK:    cfg.CacheTypeK,
		CacheTypeV:    cfg.CacheTypeV,
		Jinja:         cfg.Jinja,
		Parallel:      cfg.Parallel,
		SpecType:      cfg.SpecType,
		SpecDraftNMax: cfg.SpecDraftNMax,
		HighPriority:  cfg.HighPriority,
		ExtraArgs:     append([]string{}, cfg.ExtraArgs...),
		ExtraEnv:      map[string]string{},
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
	if p.SpecType == "" {
		p.SpecType = config.DefaultSpecType
	}
	if p.SpecDraftNMax <= 0 {
		p.SpecDraftNMax = config.DefaultDraftMax
	}
	if p.Threads <= 0 {
		n := runtime.NumCPU()
		if n >= 16 {
			p.Threads = n - 4
		} else if n > 2 {
			p.Threads = n - 1
		} else {
			p.Threads = n
		}
	}
	if p.NativeCtx <= 0 {
		p.NativeCtx = config.DefaultNativeCtx
	}
	if p.RopeArch == "" {
		p.RopeArch = config.DefaultRopeArch
	}
	// Extend past the trained window with YaRN. llama.cpp also caps n_ctx to
	// the GGUF's declared context_length, so override that KV when scaling.
	if p.CtxSize > p.NativeCtx {
		p.YarnOrigCtx = p.NativeCtx
		p.YarnScale = float64(p.CtxSize) / float64(p.NativeCtx)
	}

	// Pin the Strix Halo iGPU. Linux also pins ROCr and routes GEMMs through
	// hipBLASLt; Windows lemonade already ships that stack and uses process
	// high-priority instead.
	p.ExtraEnv["HIP_VISIBLE_DEVICES"] = "0"
	if runtime.GOOS == "linux" {
		p.ExtraEnv["ROCR_VISIBLE_DEVICES"] = "0"
		p.ExtraEnv["ROCBLAS_USE_HIPBLASLT"] = "1"
		p.FitOff = true
	}
	for k, v := range cfg.Env {
		p.ExtraEnv[k] = v
	}
	return p
}
