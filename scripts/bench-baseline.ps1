# Proper API baseline against a running malaikat/llama-server.
# Warmup + N timed runs; reports mean/stdev of server tg & pp (not wall-clock).
$ErrorActionPreference = "Stop"
$port = if ($env:MALAIKAT_PORT) { [int]$env:MALAIKAT_PORT } else { 8080 }
$runs = if ($env:MALAIKAT_BENCH_RUNS) { [int]$env:MALAIKAT_BENCH_RUNS } else { 5 }
$n = if ($env:MALAIKAT_BENCH_N) { [int]$env:MALAIKAT_BENCH_N } else { 128 }
$label = if ($env:MALAIKAT_BENCH_LABEL) { $env:MALAIKAT_BENCH_LABEL } else { "baseline" }
$outPath = if ($env:MALAIKAT_BENCH_OUT) { $env:MALAIKAT_BENCH_OUT } else { Join-Path (Split-Path $PSScriptRoot -Parent) "bench-baseline.json" }

$shortPrompt = "Write a Python function that merges two sorted lists. Only code."
# ~coding-shaped: larger prompt, still short completion — stresses prefill without capping ctx.
$longPrompt = @"
You are a coding assistant. Review this Go module and suggest a focused performance improvement.
Do not rewrite the whole file. Reply with a short patch description and one code block.

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
)

// ServerOpts configures llama-server.
type ServerOpts struct {
	Model   string
	Host    string
	Port    int
	Extra   []string
}

func (o ServerOpts) Addr() string {
	return net.JoinHostPort(o.Host, strconv.Itoa(o.Port))
}

func (o ServerOpts) BaseURL() string {
	return "http://" + o.Addr()
}

// BuildServerArgs returns ROCm + MTP llama-server arguments.
func BuildServerArgs(model, host string, port, ctx, batch, ubatch, ngl, threads, np int, fa, ctk, ctv, spec string, draftMax int, jinja bool, extra []string) []string {
	args := []string{
		"-m", model,
		"--host", host,
		"--port", strconv.Itoa(port),
		"-c", strconv.Itoa(ctx),
		"-b", strconv.Itoa(batch),
		"-ub", strconv.Itoa(ubatch),
		"-ngl", strconv.Itoa(ngl),
		"-t", strconv.Itoa(threads),
		"-np", strconv.Itoa(np),
	}
	if fa != "" {
		args = append(args, "-fa", fa)
	}
	if ctk != "" {
		args = append(args, "--cache-type-k", ctk)
	}
	if ctv != "" {
		args = append(args, "--cache-type-v", ctv)
	}
	if jinja {
		args = append(args, "--jinja")
	}
	if spec != "" && spec != "none" {
		args = append(args, "--spec-type", spec)
		if draftMax > 0 {
			args = append(args, "--spec-draft-n-max", strconv.Itoa(draftMax))
		}
	}
	return append(args, extra...)
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

func FormatArgs(exe string, args []string) string {
	parts := make([]string, 0, 1+len(args))
	parts = append(parts, strconv.Quote(exe))
	for _, a := range args {
		parts = append(parts, strconv.Quote(a))
	}
	return strings.Join(parts, " ")
}

func Command(serverExe string, args []string, env map[string]string) *exec.Cmd {
	cmd := exec.Command(serverExe, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Dir = filepath.Dir(serverExe)
	e := os.Environ()
	for k, v := range env {
		e = append(e, k+"="+v)
	}
	cmd.Env = e
	return cmd
}
"@

function Wait-Healthy {
  $deadline = (Get-Date).AddMinutes(20)
  while ((Get-Date) -lt $deadline) {
    try {
      $r = Invoke-WebRequest -Uri "http://127.0.0.1:$port/health" -UseBasicParsing -TimeoutSec 2
      if ($r.StatusCode -eq 200) { return }
    } catch {}
    Start-Sleep -Seconds 2
  }
  throw "server not healthy on port $port"
}

function Invoke-Chat([string]$prompt, [int]$maxTokens) {
  $body = @{
    messages    = @(@{ role = "user"; content = $prompt })
    max_tokens  = $maxTokens
    temperature = 0.0
    stream      = $false
  } | ConvertTo-Json -Depth 5
  $sw = [System.Diagnostics.Stopwatch]::StartNew()
  $resp = Invoke-RestMethod -Uri "http://127.0.0.1:$port/v1/chat/completions" -Method Post -ContentType "application/json" -Body $body
  $sw.Stop()
  $comp = 0
  if ($resp.usage.completion_tokens) { $comp = [int]$resp.usage.completion_tokens }
  elseif ($resp.timings.predicted_n) { $comp = [int]$resp.timings.predicted_n }
  $promptTok = 0
  if ($resp.usage.prompt_tokens) { $promptTok = [int]$resp.usage.prompt_tokens }
  [pscustomobject]@{
    wall_s     = [math]::Round($sw.Elapsed.TotalSeconds, 3)
    prompt_n   = $promptTok
    completion = $comp
    tg         = if ($resp.timings.predicted_per_second) { [double]$resp.timings.predicted_per_second } else { 0 }
    pp         = if ($resp.timings.prompt_per_second) { [double]$resp.timings.prompt_per_second } else { 0 }
  }
}

function Stats([double[]]$vals) {
  if (-not $vals -or $vals.Count -eq 0) {
    return @{ mean = 0; stdev = 0; min = 0; max = 0; n = 0 }
  }
  $mean = ($vals | Measure-Object -Average).Average
  $min = ($vals | Measure-Object -Minimum).Minimum
  $max = ($vals | Measure-Object -Maximum).Maximum
  $stdev = 0
  if ($vals.Count -gt 1) {
    $sumSq = 0.0
    foreach ($v in $vals) { $sumSq += ($v - $mean) * ($v - $mean) }
    $stdev = [math]::Sqrt($sumSq / ($vals.Count - 1))
  }
  @{
    mean  = [math]::Round($mean, 2)
    stdev = [math]::Round($stdev, 2)
    min   = [math]::Round($min, 2)
    max   = [math]::Round($max, 2)
    n     = $vals.Count
  }
}

function Bench-Suite([string]$name, [string]$prompt) {
  Write-Host "`n=== $name (warmup + $runs runs, n=$n) ===" -ForegroundColor Cyan
  Write-Host "warmup..."
  [void](Invoke-Chat $prompt $n)
  $rows = @()
  for ($i = 1; $i -le $runs; $i++) {
    $r = Invoke-Chat $prompt $n
    Write-Host ("  run {0}: tg={1:N1} pp={2:N1} prompt={3} completion={4} wall={5}s" -f $i, $r.tg, $r.pp, $r.prompt_n, $r.completion, $r.wall_s)
    $rows += $r
  }
  $tg = Stats @($rows | ForEach-Object { $_.tg })
  $pp = Stats @($rows | ForEach-Object { $_.pp })
  Write-Host ("  RESULT {0}: tg mean={1} ±{2} (min={3} max={4}) | pp mean={5} ±{6}" -f $name, $tg.mean, $tg.stdev, $tg.min, $tg.max, $pp.mean, $pp.stdev) -ForegroundColor Yellow
  [pscustomobject]@{
    name         = $name
    runs         = $runs
    max_tokens   = $n
    prompt_tokens_last = ($rows | Select-Object -Last 1).prompt_n
    tg           = $tg
    pp           = $pp
    samples      = $rows
  }
}

Write-Host "Waiting for http://127.0.0.1:$port/health ..."
Wait-Healthy
Write-Host "Healthy. Label=$label"

$suites = @(
  (Bench-Suite "short_codegen" $shortPrompt),
  (Bench-Suite "long_code_review" $longPrompt)
)

$result = [pscustomobject]@{
  label     = $label
  port      = $port
  timestamp = (Get-Date).ToString("o")
  suites    = $suites
}
$result | ConvertTo-Json -Depth 8 | Set-Content -Path $outPath -Encoding utf8
Write-Host "`nWrote $outPath" -ForegroundColor Green
