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
		CtxSize:       cfg.CtxSize,
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

	// Pin the Strix Halo iGPU; do not hide HIP.
	p.ExtraEnv["HIP_VISIBLE_DEVICES"] = "0"
	for k, v := range cfg.Env {
		p.ExtraEnv[k] = v
	}
	return p
}
