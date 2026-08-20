package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	AppName          = "malaikat"
	DefaultHost      = "127.0.0.1"
	DefaultPort      = 8080
	DefaultCtxSize   = 0 // 0 = model's trained context (largest available)
	DefaultBatch     = 512
	DefaultUBatch    = 512
	DefaultNGL       = 999
	DefaultSpecType  = "draft-mtp"
	DefaultDraftMax  = 3
	DefaultFlashAttn = "on"
	DefaultCacheType = "q8_0"
)

// Config is loaded from a file and/or CLI flags. CLI wins.
type Config struct {
	ModelPath     string            `json:"model" yaml:"model"`
	Alias         string            `json:"alias" yaml:"alias"`
	Host          string            `json:"host" yaml:"host"`
	Port          int               `json:"port" yaml:"port"`
	CtxSize       int               `json:"ctx_size" yaml:"ctx_size"` // 0 = model max
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
	ExtraArgs     []string          `json:"extra_args" yaml:"extra_args"`
	Env           map[string]string `json:"env" yaml:"env"`
	HighPriority  bool              `json:"high_priority" yaml:"high_priority"`
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
		HighPriority:  runtime.GOOS == "windows",
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

// ModelsDir is the default model storage directory.
// Windows: %LOCALAPPDATA%\malaikat\models
// Linux: $XDG_DATA_HOME/malaikat/models or ~/.local/share/malaikat/models
func ModelsDir() (string, error) {
	var base string
	if runtime.GOOS == "windows" {
		var err error
		base, err = os.UserCacheDir() // %LOCALAPPDATA%
		if err != nil {
			return "", err
		}
	} else {
		base = os.Getenv("XDG_DATA_HOME")
		if base == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return "", err
			}
			base = filepath.Join(home, ".local", "share")
		}
	}
	dir := filepath.Join(base, AppName, "models")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

// ExpandPath expands environment variables and a leading "~/" in p.
func ExpandPath(p string) string {
	p = os.ExpandEnv(p)
	if p == "~" {
		if home, err := os.UserHomeDir(); err == nil {
			return home
		}
		return p
	}
	if strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, p[2:])
		}
	}
	return p
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
	// CtxSize 0 means "use model trained context" — do not replace.
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

// LastPath is where the most recent successful serve settings are stored.
func LastPath() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "last.yaml"), nil
}

// LoadLast reads last.yaml. ok is false if it does not exist.
func LoadLast() (Config, bool, error) {
	path, err := LastPath()
	if err != nil {
		return Default(), false, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Default(), false, nil
		}
		return Default(), false, err
	}
	cfg := Default()
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Default(), false, fmt.Errorf("parse %s: %w", path, err)
	}
	cfg.ApplyDefaults()
	return cfg, true, nil
}

// SaveLast writes the effective serve settings for the next bare `malaikat serve`.
func SaveLast(cfg Config) error {
	path, err := LastPath()
	if err != nil {
		return err
	}
	cfg.ApplyDefaults()
	data, err := yaml.Marshal(&cfg)
	if err != nil {
		return err
	}
	header := "# Last successful malaikat serve settings.\n# Used by bare: malaikat serve\n"
	return os.WriteFile(path, append([]byte(header), data...), 0o644)
}
