package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

const (
	AppName        = "malaikat"
	DefaultHost    = "127.0.0.1"
	DefaultPort    = 8080
	DefaultCtxSize = 8192
	DefaultBatch   = 512
	DefaultUBatch  = 512
	DefaultNGL     = 999
)

// Config is persisted under the user config directory.
type Config struct {
	ModelPath   string `json:"model_path,omitempty"`
	Host        string `json:"host"`
	Port        int    `json:"port"`
	CtxSize     int    `json:"ctx_size"`
	Batch       int    `json:"batch"`
	UBatch      int    `json:"ubatch"`
	NGL         int    `json:"n_gpu_layers"`
	Threads     int    `json:"threads"`
	FlashAttn   bool   `json:"flash_attn"`
	Jinja       bool   `json:"jinja"`
	Parallel    int    `json:"parallel"`
	LlamaDir    string `json:"llama_dir,omitempty"`
	LlamaTag    string `json:"llama_tag,omitempty"`
	HighPriority bool  `json:"high_priority"`
}

func Default() Config {
	return Config{
		Host:         DefaultHost,
		Port:         DefaultPort,
		CtxSize:      DefaultCtxSize,
		Batch:        DefaultBatch,
		UBatch:       DefaultUBatch,
		NGL:          DefaultNGL,
		Threads:      0, // 0 = let llama.cpp decide / we fill from CPU
		FlashAttn:    true,
		Jinja:        true,
		Parallel:     1,
		HighPriority: true,
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

func Path() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.json"), nil
}

func Load() (Config, error) {
	cfg := Default()
	path, err := Path()
	if err != nil {
		return cfg, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cfg, nil
		}
		return cfg, err
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func Save(cfg Config) error {
	path, err := Path()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func ModelsDir() (string, error) {
	data, err := DataDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(data, "models")
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
