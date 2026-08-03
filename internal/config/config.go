package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	AppName         = "malaikat"
	DefaultHost     = "127.0.0.1"
	DefaultPort     = 8080
	DefaultCtxSize  = 32768
	DefaultBatch    = 512
	DefaultUBatch   = 512
	DefaultNGL      = 999
	DefaultSpecType  = "draft-mtp"
	DefaultDraftMax  = 3
	DefaultFlashAttn = "on"
	DefaultCacheType = "q8_0"
)

// Config is loaded from a file and/or CLI flags. CLI wins.
type Config struct {
	ModelPath     string            `json:"model" yaml:"model"`
	Host          string            `json:"host" yaml:"host"`
	Port          int               `json:"port" yaml:"port"`
	CtxSize       int               `json:"ctx_size" yaml:"ctx_size"`
	Batch         int               `json:"batch" yaml:"batch"`
	UBatch        int               `json:"ubatch" yaml:"ubatch"`
	NGL           int               `json:"n_gpu_layers" yaml:"n_gpu_layers"`
	Threads       int               `json:"threads" yaml:"threads"`
	FlashAttn     string            `json:"flash_attn" yaml:"flash_attn"` // on|off|auto; empty = omit
	CacheTypeK    string            `json:"cache_type_k" yaml:"cache_type_k"`
	CacheTypeV    string            `json:"cache_type_v" yaml:"cache_type_v"`
	Jinja         bool              `json:"jinja" yaml:"jinja"`
	Parallel      int               `json:"parallel" yaml:"parallel"`
	SpecType      string            `json:"spec_type" yaml:"spec_type"`
	SpecDraftNMax int               `json:"spec_draft_n_max" yaml:"spec_draft_n_max"`
	HighPriority  bool              `json:"high_priority" yaml:"high_priority"`
	ExtraArgs     []string          `json:"extra_args" yaml:"extra_args"`
	Env           map[string]string `json:"env" yaml:"env"`
	LlamaDir      string            `json:"llama_dir,omitempty" yaml:"llama_dir,omitempty"`
	LlamaTag      string            `json:"llama_tag,omitempty" yaml:"llama_tag,omitempty"`
}

func Default() Config {
	return Config{
		Host:          DefaultHost,
		Port:          DefaultPort,
		CtxSize:       DefaultCtxSize,
		Batch:         DefaultBatch,
		UBatch:        DefaultUBatch,
		NGL:           DefaultNGL,
		Threads:       0,
		FlashAttn:     DefaultFlashAttn,
		CacheTypeK:    DefaultCacheType,
		CacheTypeV:    DefaultCacheType,
		Jinja:         true,
		Parallel:      1,
		SpecType:      DefaultSpecType,
		SpecDraftNMax: DefaultDraftMax,
		HighPriority:  true,
		Env:           map[string]string{},
	}
}

func Dir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, AppName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

func DataDir() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, AppName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

func RuntimeDir() (string, error) {
	data, err := DataDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(data, "runtime")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

// LoadFile reads JSON or YAML. Empty path returns defaults.
func LoadFile(path string) (Config, error) {
	cfg := Default()
	if strings.TrimSpace(path) == "" {
		return cfg, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, err
	}
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".yaml", ".yml":
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			return cfg, fmt.Errorf("parse %s: %w", path, err)
		}
	case ".json", "":
		if err := json.Unmarshal(data, &cfg); err != nil {
			return cfg, fmt.Errorf("parse %s: %w", path, err)
		}
	default:
		// try YAML then JSON
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			if err2 := json.Unmarshal(data, &cfg); err2 != nil {
				return cfg, fmt.Errorf("parse %s: yaml: %v; json: %v", path, err, err2)
			}
		}
	}
	cfg.ApplyDefaults()
	return cfg, nil
}

func (c *Config) ApplyDefaults() {
	d := Default()
	if c.Host == "" {
		c.Host = d.Host
	}
	if c.Port == 0 {
		c.Port = d.Port
	}
	if c.CtxSize == 0 {
		c.CtxSize = d.CtxSize
	}
	if c.Batch == 0 {
		c.Batch = d.Batch
	}
	if c.UBatch == 0 {
		c.UBatch = d.UBatch
	}
	if c.NGL == 0 {
		c.NGL = d.NGL
	}
	if c.Parallel == 0 {
		c.Parallel = d.Parallel
	}
	if c.SpecType == "" {
		c.SpecType = d.SpecType
	}
	if c.SpecDraftNMax == 0 {
		c.SpecDraftNMax = d.SpecDraftNMax
	}
	if c.FlashAttn == "" {
		c.FlashAttn = d.FlashAttn
	}
	if c.CacheTypeK == "" {
		c.CacheTypeK = d.CacheTypeK
	}
	if c.CacheTypeV == "" {
		c.CacheTypeV = d.CacheTypeV
	}
	if c.Env == nil {
		c.Env = map[string]string{}
	}
}

// MergeFile is LoadFile with explicit missing-file handling for optional paths.
func MergeFile(path string) (Config, error) {
	if path == "" {
		return Default(), nil
	}
	cfg, err := LoadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Default(), fmt.Errorf("config not found: %s", path)
		}
		return cfg, err
	}
	return cfg, nil
}
