package engine

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/simon/malaikat/internal/config"
)

// Install describes a local llama.cpp Vulkan tree.
type Install struct {
	Tag        string    `json:"tag"`
	Dir        string    `json:"dir"`
	Backend    string    `json:"backend"`
	ServerExe  string    `json:"server_exe"`
	CLIExe     string    `json:"cli_exe"`
	BenchExe   string    `json:"bench_exe"`
	Fetched    time.Time `json:"fetched"`
}

func installRoot() (string, error) {
	return config.RuntimeDir()
}

func manifestPath(root string) string {
	return filepath.Join(root, "manifest.json")
}

func ReadManifest(root string) (Install, error) {
	data, err := os.ReadFile(manifestPath(root))
	if err != nil {
		return Install{}, err
	}
	var inst Install
	if err := json.Unmarshal(data, &inst); err != nil {
		return Install{}, err
	}
	_ = resolveBins(&inst)
	return inst, nil
}

func WriteManifest(root string, inst Install) error {
	data, err := json.MarshalIndent(inst, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(manifestPath(root), data, 0o644)
}

func Current() (Install, error) {
	root, err := installRoot()
	if err != nil {
		return Install{}, err
	}
	inst, err := ReadManifest(root)
	if err != nil {
		return Install{}, fmt.Errorf("runtime not installed; run: malaikat setup")
	}
	if !exeExists(inst) {
		return Install{}, fmt.Errorf("runtime binaries missing; run: malaikat setup --force")
	}
	return inst, nil
}

func exeExists(inst Install) bool {
	if inst.ServerExe == "" {
		return false
	}
	_, err := os.Stat(inst.ServerExe)
	return err == nil
}

func resolveBins(inst *Install) error {
	if inst.Dir == "" {
		return fmt.Errorf("empty install dir")
	}
	candidates := []string{inst.Dir, filepath.Join(inst.Dir, "bin")}
	find := func(names ...string) string {
		for _, dir := range candidates {
			for _, name := range names {
				p := filepath.Join(dir, name)
				if st, err := os.Stat(p); err == nil && !st.IsDir() {
					return p
				}
			}
		}
		// recursive shallow search (zip layout varies by release)
		var found string
		_ = filepath.WalkDir(inst.Dir, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() || found != "" {
				return nil
			}
			base := d.Name()
			for _, name := range names {
				if base == name {
					found = path
					return filepath.SkipAll
				}
			}
			return nil
		})
		return found
	}

	inst.ServerExe = find("llama-server.exe", "llama-server")
	inst.CLIExe = find("llama-cli.exe", "llama-cli", "main.exe", "llama.exe")
	inst.BenchExe = find("llama-bench.exe", "llama-bench")
	if inst.ServerExe == "" {
		return fmt.Errorf("llama-server not found under %s", inst.Dir)
	}
	return nil
}
