package engine

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/simon/malaikat/internal/optimize"
)

// ServerOpts configures llama-server.
type ServerOpts struct {
	Model   string
	Host    string
	Port    int
	Profile optimize.Profile
	Extra   []string
}

func (o ServerOpts) Addr() string {
	return net.JoinHostPort(o.Host, strconv.Itoa(o.Port))
}

func (o ServerOpts) BaseURL() string {
	return "http://" + o.Addr()
}

// BuildServerArgs returns ROCm + MTP llama-server arguments.
func BuildServerArgs(o ServerOpts) []string {
	p := o.Profile
	args := []string{
		"-m", o.Model,
		"--host", o.Host,
		"--port", strconv.Itoa(o.Port),
		"-c", strconv.Itoa(p.CtxSize), // 0 = model trained context
		"-b", strconv.Itoa(p.Batch),
		"-ub", strconv.Itoa(p.UBatch),
		"-ngl", strconv.Itoa(p.NGL),
		"-t", strconv.Itoa(p.Threads),
		"-np", strconv.Itoa(p.Parallel),
	}
	if p.Alias != "" {
		args = append(args, "--alias", p.Alias)
	}
	if p.FlashAttn != "" {
		args = append(args, "-fa", p.FlashAttn)
	}
	if p.CacheTypeK != "" {
		args = append(args, "--cache-type-k", p.CacheTypeK)
	}
	if p.CacheTypeV != "" {
		args = append(args, "--cache-type-v", p.CacheTypeV)
	}
	if p.Jinja {
		args = append(args, "--jinja")
	}
	if p.SpecType != "" && p.SpecType != "none" {
		args = append(args, "--spec-type", p.SpecType)
		if p.SpecDraftNMax > 0 {
			args = append(args, "--spec-draft-n-max", strconv.Itoa(p.SpecDraftNMax))
		}
	}
	// Linux source llama.cpp --fit on treats -c 0 as unset and shrinks ctx.
	// Windows lemonade builds do not take this flag.
	if p.FitOff && !hasArg(p.ExtraArgs, "--fit") && !hasArg(o.Extra, "--fit") {
		args = append(args, "--fit", "off")
	}
	args = append(args, p.ExtraArgs...)
	args = append(args, o.Extra...)
	return args
}

func hasArg(args []string, flag string) bool {
	for _, a := range args {
		if a == flag {
			return true
		}
	}
	return false
}

// BuildBenchArgs returns args for llama-bench (no MTP; measures base kernels).
func BuildBenchArgs(model string, p optimize.Profile) []string {
	args := []string{
		"-m", model,
		"-ngl", strconv.Itoa(p.NGL),
		"-b", strconv.Itoa(p.Batch),
		"-ub", strconv.Itoa(p.UBatch),
		"-t", strconv.Itoa(p.Threads),
		"-p", "512",
		"-n", "128",
	}
	if p.FlashAttn != "" {
		args = append(args, "-fa", p.FlashAttn)
	}
	return args
}

// prependEnv prepends val to a list-valued env entry, adding it if missing.
func prependEnv(env []string, key, val string) []string {
	if val == "" {
		return env
	}
	prefix := key + "="
	for i, e := range env {
		if strings.HasPrefix(e, prefix) {
			env[i] = prefix + val + string(os.PathListSeparator) + e[len(prefix):]
			return env
		}
	}
	return append(env, prefix+val)
}

// setEnv sets key=val, replacing any existing entry so the profile wins.
func setEnv(env []string, key, val string) []string {
	prefix := key + "="
	for i, e := range env {
		if strings.HasPrefix(e, prefix) {
			env[i] = prefix + val
			return env
		}
	}
	return append(env, prefix+val)
}

// prependPath prepends val to PATH (or Path on Windows).
func prependPath(env []string, val string) []string {
	if val == "" {
		return env
	}
	for i, e := range env {
		eq := strings.IndexByte(e, '=')
		if eq <= 0 {
			continue
		}
		if strings.EqualFold(e[:eq], "PATH") {
			env[i] = e[:eq+1] + val + string(os.PathListSeparator) + e[eq+1:]
			return env
		}
	}
	return append(env, "PATH="+val)
}

// serverEnv builds the child environment for the ROCm runtime.
// Windows: PATH/DLL search next to llama-server.
// Linux: LD_LIBRARY_PATH plus PATH.
func serverEnv(inst Install, p optimize.Profile) []string {
	env := os.Environ()
	binDir := filepath.Dir(inst.ServerExe)
	if runtime.GOOS == "windows" {
		workDir := inst.Dir
		if workDir == "" {
			workDir = binDir
		}
		pathPrepend := binDir
		if workDir != binDir {
			pathPrepend = workDir + string(os.PathListSeparator) + binDir
		}
		env = prependPath(env, pathPrepend)
	} else {
		for _, d := range inst.LibDirs() {
			env = prependEnv(env, "LD_LIBRARY_PATH", d)
		}
		env = prependPath(env, binDir)
	}
	for k, v := range p.ExtraEnv {
		env = setEnv(env, k, v)
	}
	return env
}

// Command prepares an exec.Cmd with the ROCm runtime on the library path.
func Command(inst Install, args []string, p optimize.Profile) *exec.Cmd {
	cmd := exec.Command(inst.ServerExe, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	workDir := inst.Dir
	if workDir == "" {
		workDir = filepath.Dir(inst.ServerExe)
	}
	cmd.Dir = workDir
	cmd.Env = serverEnv(inst, p)
	if p.HighPriority {
		setHighPriority(cmd)
	}
	return cmd
}

// BenchCommand runs llama-bench from the same install tree.
func BenchCommand(inst Install, args []string, p optimize.Profile) *exec.Cmd {
	exe := inst.BenchExe
	if exe == "" {
		exe = inst.ServerExe
	}
	cmd := exec.Command(exe, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Dir = filepath.Dir(exe)
	cmd.Env = serverEnv(inst, p)
	return cmd
}

// WaitReady polls the llama-server health endpoint.
func WaitReady(ctx context.Context, baseURL string) error {
	client := &http.Client{Timeout: 2 * time.Second}
	url := baseURL + "/health"
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		resp, err := client.Do(req)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("server not ready: %w", ctx.Err())
		case <-ticker.C:
		}
	}
}

// FormatArgs joins args for logging.
func FormatArgs(exe string, args []string) string {
	parts := make([]string, 0, 1+len(args))
	parts = append(parts, strconv.Quote(exe))
	for _, a := range args {
		if strings.ContainsAny(a, " \t\"") {
			parts = append(parts, strconv.Quote(a))
		} else {
			parts = append(parts, a)
		}
	}
	return strings.Join(parts, " ")
}
