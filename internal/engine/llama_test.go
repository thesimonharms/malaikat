package engine

import (
	"slices"
	"strings"
	"testing"

	"github.com/simon/malaikat/internal/config"
	"github.com/simon/malaikat/internal/optimize"
)

func TestBuildServerArgsYarn512k(t *testing.T) {
	cfg := config.Default()
	cfg.CtxSize = config.CtxSize(config.Ctx512K)
	cfg.NativeCtx = config.NativeCtxQwen35
	cfg.RopeArch = "qwen35moe"
	cfg.FlashAttn = ""
	cfg.CacheTypeK = ""
	cfg.CacheTypeV = ""
	cfg.Jinja = false
	cfg.SpecType = "none"
	p := optimize.FromConfig(cfg)
	p.FitOff = false
	p.Threads = 8
	args := BuildServerArgs(ServerOpts{
		Model:   "m.gguf",
		Host:    "127.0.0.1",
		Port:    8080,
		Profile: p,
	})
	joined := strings.Join(args, " ")
	for _, want := range []string{
		"-c 524288",
		"--rope-scaling yarn",
		"--rope-scale 2",
		"--yarn-orig-ctx 262144",
		"--override-kv qwen35moe.context_length=int:524288",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %q in %s", want, joined)
		}
	}
}

func TestBuildServerArgsNative256kNoYarn(t *testing.T) {
	cfg := config.Default()
	cfg.CtxSize = config.CtxSize(config.Ctx256K)
	cfg.FlashAttn = ""
	cfg.CacheTypeK = ""
	cfg.CacheTypeV = ""
	cfg.Jinja = false
	cfg.SpecType = "none"
	p := optimize.FromConfig(cfg)
	p.FitOff = false
	p.Threads = 8
	args := BuildServerArgs(ServerOpts{
		Model:   "m.gguf",
		Host:    "127.0.0.1",
		Port:    8080,
		Profile: p,
	})
	if slices.Contains(args, "--rope-scaling") {
		t.Fatalf("native 256k should not enable YaRN: %v", args)
	}
	if !slices.Contains(args, "262144") {
		t.Fatalf("expected -c 262144, got %v", args)
	}
}

func TestBuildServerArgsYarn1M(t *testing.T) {
	cfg := config.Default()
	cfg.CtxSize = config.CtxSize(config.Ctx1M)
	cfg.FlashAttn = ""
	cfg.CacheTypeK = ""
	cfg.CacheTypeV = ""
	cfg.Jinja = false
	cfg.SpecType = "none"
	p := optimize.FromConfig(cfg)
	p.FitOff = false
	p.Threads = 8
	args := BuildServerArgs(ServerOpts{
		Model:   "m.gguf",
		Host:    "127.0.0.1",
		Port:    8080,
		Profile: p,
	})
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "-c 1048576") {
		t.Fatalf("got %s", joined)
	}
	if !strings.Contains(joined, "--rope-scale 4") {
		t.Fatalf("1m should be 4x YaRN, got %s", joined)
	}
}
