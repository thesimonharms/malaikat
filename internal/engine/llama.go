package engine

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
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
	args = append(args, p.ExtraArgs...)
	args = append(args, o.Extra...)
	return args
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

// Command prepares an exec.Cmd with ROCm DLL path, env, optional high priority.
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

	env := os.Environ()
	binDir := filepath.Dir(inst.ServerExe)
	pathPrepend := binDir
	if workDir != binDir {
		pathPrepend = workDir + string(os.PathListSeparator) + binDir
	}
	pathSet := false
	for i, e := range env {
		if strings.HasPrefix(strings.ToUpper(e), "PATH=") {
			env[i] = e[:5] + pathPrepend + string(os.PathListSeparator) + e[5:]
			// handle Path= on Windows
			if strings.HasPrefix(e, "Path=") {
				env[i] = "Path=" + pathPrepend + string(os.PathListSeparator) + e[5:]
			}
			pathSet = true
			break
		}
	}
	if !pathSet {
		env = append(env, "Path="+pathPrepend)
	}
	for k, v := range p.ExtraEnv {
		env = append(env, k+"="+v)
	}
	cmd.Env = env
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
	cmd.Dir = inst.Dir
	if cmd.Dir == "" {
		cmd.Dir = filepath.Dir(exe)
	}
	env := os.Environ()
	binDir := filepath.Dir(exe)
	for i, e := range env {
		if strings.HasPrefix(e, "Path=") || strings.HasPrefix(e, "PATH=") {
			env[i] = e[:5] + binDir + string(os.PathListSeparator) + e[5:]
			break
		}
	}
	for k, v := range p.ExtraEnv {
		env = append(env, k+"="+v)
	}
	cmd.Env = env
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
