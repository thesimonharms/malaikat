# Restart malaikat with flag variants and record tok/s.
$ErrorActionPreference = "Stop"
$root = Split-Path (Split-Path $PSScriptRoot -Parent) -Parent
if (-not (Test-Path "$PSScriptRoot\..\malaikat.exe")) { $root = Resolve-Path "$PSScriptRoot\.." } else { $root = Resolve-Path "$PSScriptRoot\.." }
Set-Location $root

$model = "$env:LOCALAPPDATA\malaikat\models\Qwen3.6-35B-A3B-MTP\Qwen3.6-35B-A3B-UD-Q4_K_XL.gguf"
$exe = Join-Path $root "malaikat.exe"
$prompt = "Write a Python function that merges two sorted lists. Only code."
$port = 8080

function Stop-Server {
  $pids = @()
  try {
    $pids += (Get-NetTCPConnection -LocalPort $port -State Listen -ErrorAction SilentlyContinue).OwningProcess
  } catch {}
  $pids += (Get-Process llama-server -ErrorAction SilentlyContinue).Id
  $pids = $pids | Where-Object { $_ } | Select-Object -Unique
  foreach ($procId in $pids) {
    Stop-Process -Id $procId -Force -ErrorAction SilentlyContinue
  }
  Start-Sleep -Seconds 2
}

function Start-Server([string[]]$extra) {
  Stop-Server
  $argList = @("serve", "-m", $model, "-c", "8192", "-port", "$port") + $extra
  $proc = Start-Process -FilePath $exe -ArgumentList $argList -PassThru -WindowStyle Hidden -RedirectStandardOutput "$env:TEMP\malaikat-sweep-out.txt" -RedirectStandardError "$env:TEMP\malaikat-sweep-err.txt"
  $deadline = (Get-Date).AddMinutes(8)
  while ((Get-Date) -lt $deadline) {
    try {
      $r = Invoke-WebRequest -Uri "http://127.0.0.1:$port/health" -UseBasicParsing -TimeoutSec 2
      if ($r.StatusCode -eq 200) { return $proc }
    } catch {}
    if ($proc.HasExited) { throw "server exited early; see $env:TEMP\malaikat-sweep-err.txt" }
    Start-Sleep -Milliseconds 500
  }
  throw "server not ready"
}

function Bench-Once {
  $body = @{
    messages     = @(@{ role = "user"; content = $prompt })
    max_tokens   = 128
    temperature  = 0.0
    stream       = $false
  } | ConvertTo-Json -Depth 5
  # warmup
  Invoke-RestMethod -Uri "http://127.0.0.1:$port/v1/chat/completions" -Method Post -ContentType "application/json" -Body $body | Out-Null
  $tg = @()
  for ($i = 0; $i -lt 2; $i++) {
    $resp = Invoke-RestMethod -Uri "http://127.0.0.1:$port/v1/chat/completions" -Method Post -ContentType "application/json" -Body $body
    if ($resp.timings.predicted_per_second) {
      $tg += [double]$resp.timings.predicted_per_second
    }
  }
  if ($tg.Count -eq 0) { return 0 }
  return ($tg | Measure-Object -Average).Average
}

$variants = @(
  @{ name = "baseline_n2"; args = @("-spec-draft-n-max", "2") },
  @{ name = "n3"; args = @("-spec-draft-n-max", "3") },
  @{ name = "n4"; args = @("-spec-draft-n-max", "4") },
  @{ name = "n3_fa_on"; args = @("-spec-draft-n-max", "3", "-fa", "on") },
  @{ name = "n3_fa_off"; args = @("-spec-draft-n-max", "3", "-fa", "off") },
  @{ name = "n3_kv_q8"; args = @("-spec-draft-n-max", "3", "--", "--cache-type-k", "q8_0", "--cache-type-v", "q8_0") },
  @{ name = "n3_fa_kv_q8"; args = @("-spec-draft-n-max", "3", "-fa", "on", "--", "--cache-type-k", "q8_0", "--cache-type-v", "q8_0") },
  @{ name = "n3_fa_kv_q4"; args = @("-spec-draft-n-max", "3", "-fa", "on", "--", "--cache-type-k", "q4_0", "--cache-type-v", "q4_0") },
  @{ name = "n3_fa_kv_q8_b2048"; args = @("-spec-draft-n-max", "3", "-fa", "on", "-b", "2048", "-ub", "512", "--", "--cache-type-k", "q8_0", "--cache-type-v", "q8_0") },
  @{ name = "no_mtp"; args = @("-no-mtp") }
)

$results = @()
foreach ($v in $variants) {
  Write-Host "=== $($v.name) ===" -ForegroundColor Cyan
  try {
    Start-Server $v.args | Out-Null
    $avg = Bench-Once
    Write-Host ("{0}: {1:N1} tok/s" -f $v.name, $avg)
    $results += [pscustomobject]@{ name = $v.name; tps = [math]::Round($avg, 1); args = ($v.args -join " ") }
  } catch {
    Write-Host ("{0}: FAIL {1}" -f $v.name, $_) -ForegroundColor Red
    $results += [pscustomobject]@{ name = $v.name; tps = -1; args = ($v.args -join " ") }
  }
}

Write-Host "`nRESULTS" -ForegroundColor Yellow
$results | Sort-Object tps -Descending | Format-Table -AutoSize
$results | ConvertTo-Json | Set-Content "$root\sweep-results.json"
Stop-Server
Write-Host "done"
