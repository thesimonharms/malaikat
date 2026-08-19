package engine

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/simon/malaikat/internal/config"
)

// Install describes a local lemonade ROCm llama.cpp tree.
type Install struct {
	Tag       string    `json:"tag"`
	Dir       string    `json:"dir"`
	Backend   string    `json:"backend"`
	ServerExe string    `json:"server_exe"`
	CLIExe    string    `json:"cli_exe"`
	BenchExe  string    `json:"bench_exe"`
	Fetched   time.Time `json:"fetched"`
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
		if HasEmbeddedROCm() {
			return EnsureROCm(false)
		}
		return Install{}, fmt.Errorf("runtime not installed; run: malaikat setup")
	}
	if inst.Backend != "" && inst.Backend != "rocm" {
		return Install{}, fmt.Errorf("runtime backend is %q; run: malaikat setup --force (need ROCm gfx1151)", inst.Backend)
	}
	if !exeExists(inst) {
		if HasEmbeddedROCm() {
			return EnsureROCm(true)
		}
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

	inst.ServerExe = find("llama-server")
	inst.CLIExe = find("llama-cli")
	inst.BenchExe = find("llama-bench")
	if inst.ServerExe == "" {
		return fmt.Errorf("llama-server not found under %s", inst.Dir)
	}
	// Prefer the directory that contains the server (ROCm libs live beside it).
	inst.Dir = filepath.Dir(inst.ServerExe)
	// Zip archives do not reliably preserve the executable bit.
	for _, exe := range []string{inst.ServerExe, inst.CLIExe, inst.BenchExe} {
		if exe == "" {
			continue
		}
		if st, err := os.Stat(exe); err == nil && st.Mode()&0o111 == 0 {
			_ = os.Chmod(exe, st.Mode()|0o755)
		}
	}
	return nil
}

// LibDirs returns the directories holding the bundled ROCm shared libraries,
// for LD_LIBRARY_PATH: the server binary dir plus lib/ and ../lib when present.
func (inst Install) LibDirs() []string {
	binDir := filepath.Dir(inst.ServerExe)
	seen := map[string]bool{binDir: true}
	dirs := []string{binDir}
	for _, cand := range []string{
		filepath.Join(binDir, "lib"),
		filepath.Join(binDir, "..", "lib"),
		filepath.Join(binDir, "..", "lib64"),
	} {
		cand = filepath.Clean(cand)
		if seen[cand] {
			continue
		}
		if st, err := os.Stat(cand); err == nil && st.IsDir() {
			seen[cand] = true
			dirs = append(dirs, cand)
		}
	}
	return dirs
}
