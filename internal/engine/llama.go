package engine

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
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

// BuildServerArgs returns Strix Halo–tuned llama-server arguments.
func BuildServerArgs(o ServerOpts) []string {
	p := o.Profile
	args := []string{
		"-m", o.Model,
		"--host", o.Host,
		"--port", strconv.Itoa(o.Port),
		"-c", strconv.Itoa(p.CtxSize),
		"-b", strconv.Itoa(p.Batch),
		"-ub", strconv.Itoa(p.UBatch),
		"-ngl", strconv.Itoa(p.NGL),
		"-t", strconv.Itoa(p.Threads),
		"-np", strconv.Itoa(p.Parallel),
	}
	if p.FlashAttn {
		args = append(args, "-fa", "on")
	}
	if p.Jinja {
		args = append(args, "--jinja")
	}
	args = append(args, o.Extra...)
	return args
}

// BuildCLIArgs returns args for interactive llama-cli.
func BuildCLIArgs(model string, p optimize.Profile, extra []string) []string {
	args := []string{
		"-m", model,
		"-c", strconv.Itoa(p.CtxSize),
		"-b", strconv.Itoa(p.Batch),
		"-ub", strconv.Itoa(p.UBatch),
		"-ngl", strconv.Itoa(p.NGL),
		"-t", strconv.Itoa(p.Threads),
		"-cnv",
	}
	if p.FlashAttn {
		args = append(args, "-fa", "on")
	}
	if p.Jinja {
		args = append(args, "--jinja")
	}
	args = append(args, extra...)
	return args
}

// BuildBenchArgs returns args for llama-bench.
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
	if p.FlashAttn {
		args = append(args, "-fa", "1")
	}
	return args
}

// Command prepares an exec.Cmd with env and optional high priority.
func Command(exe string, args []string, p optimize.Profile) *exec.Cmd {
	cmd := exec.Command(exe, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	env := os.Environ()
	for k, v := range p.ExtraEnv {
		env = append(env, k+"="+v)
	}
	cmd.Env = env
	if p.HighPriority {
		setHighPriority(cmd)
	}
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
