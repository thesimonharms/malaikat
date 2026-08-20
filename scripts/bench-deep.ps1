# Deep-context fair bench: keep server ctx_size=0, but fill a large prefix so tg reflects coding agents.
$ErrorActionPreference = "Stop"
$port = if ($env:MALAIKAT_PORT) { [int]$env:MALAIKAT_PORT } else { 8080 }
$runs = if ($env:MALAIKAT_BENCH_RUNS) { [int]$env:MALAIKAT_BENCH_RUNS } else { 4 }
$n = if ($env:MALAIKAT_BENCH_N) { [int]$env:MALAIKAT_BENCH_N } else { 128 }
$prefixTarget = if ($env:MALAIKAT_PREFIX_TOKENS) { [int]$env:MALAIKAT_PREFIX_TOKENS } else { 8192 }
$label = if ($env:MALAIKAT_BENCH_LABEL) { $env:MALAIKAT_BENCH_LABEL } else { "deep" }
$outPath = if ($env:MALAIKAT_BENCH_OUT) { $env:MALAIKAT_BENCH_OUT } else { Join-Path (Split-Path $PSScriptRoot -Parent) "bench-deep.json" }

$unit = @"
// synthetic module chunk for context fill
package synth

import (
	"fmt"
	"sync"
	"time"
)

type Cache struct {
	mu sync.RWMutex
	m  map[string]entry
}

type entry struct {
	val string
	at  time.Time
}

func NewCache() *Cache {
	return &Cache{m: make(map[string]entry)}
}

func (c *Cache) Get(k string) (string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	e, ok := c.m[k]
	if !ok {
		return "", false
	}
	return e.val, true
}

func (c *Cache) Set(k, v string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.m[k] = entry{val: v, at: time.Now()}
}

func (c *Cache) String() string {
	return fmt.Sprintf("cache_len=%d", len(c.m))
}

"@

function Build-Prefix([int]$approxChars) {
  $sb = New-Object System.Text.StringBuilder
  [void]$sb.AppendLine("You are a coding assistant working in a large repository. Context files follow.")
  $i = 0
  while ($sb.Length -lt $approxChars) {
    [void]$sb.AppendLine("// file: synth/cache_$i.go")
    [void]$sb.AppendLine($unit)
    $i++
  }
  return $sb.ToString()
}

# ~4 chars/token rough heuristic for code
$prefix = Build-Prefix ($prefixTarget * 4)

$tasks = @(
  "Using the repo context above, write a Go helper that expires cache entries older than d. Only code. Tag: {0}",
  "Using the repo context above, add a thread-safe Delete(k) method. Only code. Tag: {0}",
  "Using the repo context above, write a unit test for Cache.Set/Get. Only code. Tag: {0}",
  "Using the repo context above, suggest one concurrency improvement. Short reply + code. Tag: {0}",
  "Using the repo context above, write Len() returning entry count. Only code. Tag: {0}"
)

function Wait-Healthy {
  $deadline = (Get-Date).AddMinutes(20)
  while ((Get-Date) -lt $deadline) {
    try {
      $r = Invoke-WebRequest -Uri "http://127.0.0.1:$port/health" -UseBasicParsing -TimeoutSec 2
      if ($r.StatusCode -eq 200) { return }
    } catch {}
    Start-Sleep -Seconds 2
  }
  throw "server not healthy"
}

function Invoke-Chat([string]$prompt, [int]$maxTokens) {
  $body = @{
    messages    = @(@{ role = "user"; content = $prompt })
    max_tokens  = $maxTokens
    temperature = 0.0
    stream      = $false
  } | ConvertTo-Json -Depth 5
  $sw = [System.Diagnostics.Stopwatch]::StartNew()
  $resp = Invoke-RestMethod -Uri "http://127.0.0.1:$port/v1/chat/completions" -Method Post -ContentType "application/json" -Body $body -TimeoutSec 600
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
  $mean = ($vals | Measure-Object -Average).Average
  $min = ($vals | Measure-Object -Minimum).Minimum
  $max = ($vals | Measure-Object -Maximum).Maximum
  $stdev = 0
  if ($vals.Count -gt 1) {
    $sumSq = 0.0
    foreach ($v in $vals) { $sumSq += ($v - $mean) * ($v - $mean) }
    $stdev = [math]::Sqrt($sumSq / ($vals.Count - 1))
  }
  @{ mean = [math]::Round($mean, 2); stdev = [math]::Round($stdev, 2); min = [math]::Round($min, 2); max = [math]::Round($max, 2); n = $vals.Count }
}

Write-Host "Waiting for health..."
Wait-Healthy
Write-Host "Healthy. Label=$label prefix_target≈$prefixTarget tokens (chars=$($prefix.Length))"

$warmTag = [guid]::NewGuid().ToString("N").Substring(0, 8)
Write-Host "warmup (deep)..."
[void](Invoke-Chat ($prefix + "`n`n" + ($tasks[0] -f $warmTag)) $n)

$rows = @()
for ($i = 0; $i -lt $runs; $i++) {
  $tag = [guid]::NewGuid().ToString("N").Substring(0, 8)
  $prompt = $prefix + "`n`n" + ($tasks[$i % $tasks.Count] -f $tag)
  $r = Invoke-Chat $prompt $n
  Write-Host ("  run {0}: tg={1:N1} pp={2:N1} prompt={3} completion={4} wall={5}s" -f ($i + 1), $r.tg, $r.pp, $r.prompt_n, $r.completion, $r.wall_s)
  $rows += $r
}

$tg = Stats @($rows | ForEach-Object { $_.tg })
$pp = Stats @($rows | ForEach-Object { $_.pp })
Write-Host ("RESULT {0}: tg mean={1} ±{2} | pp mean={3} ±{4} | prompt_n≈{5}" -f $label, $tg.mean, $tg.stdev, $pp.mean, $pp.stdev, ($rows | Select-Object -Last 1).prompt_n) -ForegroundColor Yellow

$result = [pscustomobject]@{
  label = $label
  method = "deep_unique"
  prefix_target = $prefixTarget
  timestamp = (Get-Date).ToString("o")
  tg = $tg
  pp = $pp
  prompt_n_last = ($rows | Select-Object -Last 1).prompt_n
  samples = $rows
}
$result | ConvertTo-Json -Depth 6 | Set-Content $outPath -Encoding utf8
Write-Host "Wrote $outPath"
