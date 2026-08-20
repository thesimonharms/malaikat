# Fair bench: unique prompt each run so ngram speculation cannot memorize the prior reply.
$ErrorActionPreference = "Stop"
$port = if ($env:MALAIKAT_PORT) { [int]$env:MALAIKAT_PORT } else { 8080 }
$runs = if ($env:MALAIKAT_BENCH_RUNS) { [int]$env:MALAIKAT_BENCH_RUNS } else { 5 }
$n = if ($env:MALAIKAT_BENCH_N) { [int]$env:MALAIKAT_BENCH_N } else { 128 }
$label = if ($env:MALAIKAT_BENCH_LABEL) { $env:MALAIKAT_BENCH_LABEL } else { "fair" }
$outPath = if ($env:MALAIKAT_BENCH_OUT) { $env:MALAIKAT_BENCH_OUT } else { Join-Path (Split-Path $PSScriptRoot -Parent) "bench-fair.json" }

$shortTasks = @(
  "Write a Python function that merges two sorted lists. Only code. Tag: {0}",
  "Write a TypeScript function that deep-clones a plain object. Only code. Tag: {0}",
  "Write a Rust function that returns the nth Fibonacci number iteratively. Only code. Tag: {0}",
  "Write a Go function that deduplicates a []string preserving order. Only code. Tag: {0}",
  "Write a SQL query that finds duplicate emails in a users table. Only SQL. Tag: {0}",
  "Write a Python function that flattens a nested list. Only code. Tag: {0}"
)

$longTasks = @(
  @"
You are a coding assistant. Suggest one focused performance improvement for this snippet. Short reply + one code block. Tag: {0}

func FormatArgs(exe string, args []string) string {{
	parts := make([]string, 0, 1+len(args))
	parts = append(parts, strconv.Quote(exe))
	for _, a := range args {{
		parts = append(parts, strconv.Quote(a))
	}}
	return strings.Join(parts, " ")
}}
"@,
  @"
You are a coding assistant. Suggest one focused readability improvement. Short reply + one code block. Tag: {0}

def merge(a, b):
    i = j = 0
    out = []
    while i < len(a) and j < len(b):
        if a[i] <= b[j]:
            out.append(a[i]); i += 1
        else:
            out.append(b[j]); j += 1
    out.extend(a[i:]); out.extend(b[j:])
    return out
"@,
  @"
You are a coding assistant. Suggest one focused bugfix. Short reply + one code block. Tag: {0}

function deepClone(obj) {{
  if (obj === null || typeof obj !== 'object') return obj
  const out = Array.isArray(obj) ? [] : {{}}
  for (const k of Object.keys(obj)) out[k] = deepClone(obj[k])
  return out
}}
"@,
  @"
You are a coding assistant. Suggest one focused API improvement. Short reply + one code block. Tag: {0}

pub fn fib(n: u32) -> u64 {{
    let mut a = 0u64;
    let mut b = 1u64;
    for _ in 0..n {{
        let c = a.wrapping_add(b);
        a = b;
        b = c;
    }}
    a
}}
"@,
  @"
You are a coding assistant. Suggest one focused test idea and a tiny example. Tag: {0}

func Dedup(in []string) []string {{
	seen := map[string]struct{{}}{{}}
	out := make([]string, 0, len(in))
	for _, s := range in {{
		if _, ok := seen[s]; ok {{ continue }}
		seen[s] = struct{{}}{{}}
		out = append(out, s)
	}}
	return out
}}
"@,
  @"
You are a coding assistant. Suggest one focused null-safety improvement. Short reply + one code block. Tag: {0}

def flatten(xs):
    out = []
    for x in xs:
        if isinstance(x, list):
            out.extend(flatten(x))
        else:
            out.append(x)
    return out
"@
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

function Bench-Suite([string]$name, [string[]]$templates) {
  Write-Host "`n=== $name (warmup + $runs UNIQUE prompts, n=$n) ===" -ForegroundColor Cyan
  $warmTag = [guid]::NewGuid().ToString("N").Substring(0, 8)
  Write-Host "warmup..."
  [void](Invoke-Chat ($templates[0] -f $warmTag) $n)
  $rows = @()
  for ($i = 0; $i -lt $runs; $i++) {
    $tag = [guid]::NewGuid().ToString("N").Substring(0, 8)
    $tmpl = $templates[($i + 1) % $templates.Count]
    $prompt = $tmpl -f $tag
    $r = Invoke-Chat $prompt $n
    Write-Host ("  run {0}: tg={1:N1} pp={2:N1} prompt={3} completion={4} wall={5}s" -f ($i + 1), $r.tg, $r.pp, $r.prompt_n, $r.completion, $r.wall_s)
    $rows += $r
  }
  $tg = Stats @($rows | ForEach-Object { $_.tg })
  $pp = Stats @($rows | ForEach-Object { $_.pp })
  Write-Host ("  RESULT {0}: tg mean={1} ±{2} (min={3} max={4}) | pp mean={5} ±{6}" -f $name, $tg.mean, $tg.stdev, $tg.min, $tg.max, $pp.mean, $pp.stdev) -ForegroundColor Yellow
  [pscustomobject]@{
    name               = $name
    runs               = $runs
    max_tokens         = $n
    prompt_tokens_last = ($rows | Select-Object -Last 1).prompt_n
    tg                 = $tg
    pp                 = $pp
    samples            = $rows
  }
}

Write-Host "Waiting for http://127.0.0.1:$port/health ..."
Wait-Healthy
Write-Host "Healthy. Label=$label (unique prompts)"

$suites = @(
  (Bench-Suite "short_codegen" $shortTasks),
  (Bench-Suite "long_code_review" $longTasks)
)

$result = [pscustomobject]@{
  label     = $label
  method    = "unique_prompts"
  port      = $port
  timestamp = (Get-Date).ToString("o")
  suites    = $suites
}
$result | ConvertTo-Json -Depth 8 | Set-Content -Path $outPath -Encoding utf8
Write-Host "`nWrote $outPath" -ForegroundColor Green
