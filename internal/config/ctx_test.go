package config

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestParseCtxSize(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"256k", Ctx256K},
		{"256K", Ctx256K},
		{"512k", Ctx512K},
		{"1m", Ctx1M},
		{"1M", Ctx1M},
		{"0", 0},
		{"max", 0},
		{"262144", 262144},
		{" 512K ", Ctx512K},
		{"1024k", Ctx1M},
	}
	for _, tc := range cases {
		got, err := ParseCtxSize(tc.in)
		if err != nil {
			t.Fatalf("ParseCtxSize(%q): %v", tc.in, err)
		}
		if got != tc.want {
			t.Fatalf("ParseCtxSize(%q)=%d, want %d", tc.in, got, tc.want)
		}
	}
}

func TestParseCtxSizeRejectsGarbage(t *testing.T) {
	if _, err := ParseCtxSize("lots"); err == nil {
		t.Fatal("expected error")
	}
}

func TestCtxSizeYAML(t *testing.T) {
	var cfg Config
	if err := yaml.Unmarshal([]byte("ctx_size: 512k\n"), &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.CtxSize.Int() != Ctx512K {
		t.Fatalf("got %d", cfg.CtxSize)
	}
	if err := yaml.Unmarshal([]byte("ctx_size: 0\n"), &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.CtxSize.Int() != 0 {
		t.Fatalf("got %d", cfg.CtxSize)
	}
}

func TestCodingYAMLPresets(t *testing.T) {
	var path string
	for _, r := range []string{".", "..", "../.."} {
		p := filepath.Join(r, "coding.yaml")
		if _, err := os.Stat(p); err == nil {
			path = p
			break
		}
	}
	if path == "" {
		t.Skip("coding.yaml not found from test cwd")
	}
	cfg, err := LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.CtxSize.Int() != Ctx256K {
		t.Fatalf("coding.yaml ctx_size=%d, want %d", cfg.CtxSize, Ctx256K)
	}
	if cfg.NativeCtx != NativeCtxQwen35 {
		t.Fatalf("native_ctx=%d", cfg.NativeCtx)
	}
	if cfg.RopeArch != "qwen35moe" {
		t.Fatalf("rope_arch=%s", cfg.RopeArch)
	}
	if cfg.Alias != "ornith-1.5-35b-a3b" {
		t.Fatalf("alias=%s", cfg.Alias)
	}
}
