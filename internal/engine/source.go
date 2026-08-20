package engine

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/simon/malaikat/internal/config"
)

const (
	llamaRepo   = "https://github.com/ggml-org/llama.cpp.git"
	llamaBranch = "master"
	gfxTarget   = "gfx1151"
)

// EnsureSource builds llama.cpp from source against the system ROCm
// (HIP, gfx1151) and records it as the active runtime.
// With force=false, ref empty, and a working source install, it is a no-op.
// ref optionally pins a tag/branch (e.g. "b1311"); empty tracks master.
func EnsureSource(force bool, ref string) (Install, error) {
	root, err := installRoot()
	if err != nil {
		return Install{}, err
	}

	current, _ := ReadManifest(root)
	if !force && ref == "" && current.Backend == "source" && exeExists(current) {
		return current, nil
	}

	if err := preflightBuild(); err != nil {
		return Install{}, err
	}

	data, err := config.DataDir()
	if err != nil {
		return Install{}, err
	}
	srcDir := filepath.Join(data, "llama.cpp-src")
	buildDir := filepath.Join(srcDir, "build")

	if err := checkoutLlama(srcDir, ref, force); err != nil {
		return Install{}, err
	}
	sha, _ := gitOut(srcDir, "rev-parse", "--short", "HEAD")
	if sha == "" {
		sha = "unknown"
	}

	if err := cmakeConfigure(srcDir, buildDir); err != nil {
		return Install{}, err
	}
	if err := cmakeBuild(buildDir); err != nil {
		return Install{}, err
	}

	inst := Install{
		Tag:     "src-" + sha,
		Dir:     filepath.Join(buildDir, "bin"),
		Backend: "source",
		Fetched: time.Now().UTC(),
	}
	if err := resolveBins(&inst); err != nil {
		return Install{}, err
	}
	if err := WriteManifest(root, inst); err != nil {
		return Install{}, err
	}
	fmt.Printf("Built llama.cpp %s → %s\n", inst.Tag, inst.Dir)
	return inst, nil
}

func preflightBuild() error {
	var missing []string
	for _, tool := range []string{"git", "cmake", "make"} {
		if _, err := exec.LookPath(tool); err != nil {
			missing = append(missing, tool)
		}
	}
	if _, err := os.Stat(filepath.Join(rocmRoot(), "bin", "hipcc")); err != nil {
		missing = append(missing, filepath.Join(rocmRoot(), "bin", "hipcc"))
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing build tools: %s\ninstall with: sudo pacman -S --needed base-devel cmake git rocm-hip-sdk rocblas hipblas hipblaslt",
			strings.Join(missing, ", "))
	}
	return nil
}

func rocmRoot() string {
	if v := os.Getenv("ROCM_PATH"); v != "" {
		return v
	}
	return "/opt/rocm"
}

func checkoutLlama(srcDir, ref string, force bool) error {
	if _, err := os.Stat(filepath.Join(srcDir, ".git")); err != nil {
		fmt.Printf("Cloning llama.cpp → %s\n", srcDir)
		return run("git", "clone", "--depth", "1", "--branch", llamaBranch, llamaRepo, srcDir)
	}
	switch {
	case ref != "":
		fmt.Printf("Fetching llama.cpp %s...\n", ref)
		if err := runIn(srcDir, "git", "fetch", "--depth", "1", "origin", ref); err != nil {
			return err
		}
		return runIn(srcDir, "git", "checkout", "--detach", "FETCH_HEAD")
	case force:
		fmt.Println("Updating llama.cpp master...")
		if err := runIn(srcDir, "git", "fetch", "--depth", "1", "origin", llamaBranch); err != nil {
			return err
		}
		return runIn(srcDir, "git", "reset", "--hard", "FETCH_HEAD")
	default:
		return nil // keep current checkout (works offline)
	}
}

func cmakeConfigure(srcDir, buildDir string) error {
	args := []string{
		"-S", srcDir,
		"-B", buildDir,
		"-DCMAKE_BUILD_TYPE=Release",
		"-DGGML_HIP=ON",
		"-DAMDGPU_TARGETS=" + gfxTarget,
		"-DLLAMA_CURL=OFF",
		"-DLLAMA_BUILD_TESTS=OFF",
		"-DLLAMA_BUILD_EXAMPLES=OFF",
	}
	// Current llama.cpp flash-attn on gfx1151 uses HIP WMMA builtins
	// (AMD_WMMA_AVAILABLE), not the rocWMMA library. GGML_HIP_ROCWMMA_FATTN
	// was removed upstream; enabling it used to regress long-context PP on
	// Strix Halo (ggml-org/llama.cpp#24437).
	fmt.Println("cmake", strings.Join(args, " "))
	return run("cmake", args...)
}

func cmakeBuild(buildDir string) error {
	args := []string{
		"--build", buildDir,
		"--config", "Release",
		"-j", fmt.Sprint(runtime.NumCPU()),
		"--target", "llama-server", "llama-cli", "llama-bench",
	}
	fmt.Println("cmake", strings.Join(args, " "))
	return run("cmake", args...)
}

// SmokeTest verifies the runtime can enumerate HIP devices.
func SmokeTest(inst Install) error {
	cmd := exec.Command(inst.ServerExe, "--list-devices")
	env := os.Environ()
	for _, d := range inst.LibDirs() {
		env = prependEnv(env, "LD_LIBRARY_PATH", d)
	}
	env = setEnv(env, "HIP_VISIBLE_DEVICES", "0")
	env = setEnv(env, "ROCR_VISIBLE_DEVICES", "0")
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	if trimmed := strings.TrimSpace(string(out)); trimmed != "" {
		fmt.Println(trimmed)
	}
	if err != nil {
		return fmt.Errorf("llama-server --list-devices: %w", err)
	}
	return nil
}

func run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func runIn(dir, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func gitOut(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
