# Deep-context variants (ctx_size stays 0; prompts fill ~8k tokens).
$ErrorActionPreference = "Stop"
$root = Resolve-Path "$PSScriptRoot\.."
Set-Location $root
$port = 8080
$exe = Join-Path $root "malaikat.exe"
$config = Join-Path $root "coding.yaml"
$resultsPath = Join-Path $root "bench-deep-experiments.json"

function Stop-Server {
  $pids = @()
  try { $pids += (Get-NetTCPConnection -LocalPort $port -State Listen -ErrorAction SilentlyContinue).OwningProcess } catch {}
  $pids += (Get-Process llama-server,malaikat -ErrorAction SilentlyContinue).Id
  $pids = $pids | Where-Object { $_ } | Select-Object -Unique
  foreach ($procId in $pids) { Stop-Process -Id $procId -Force -ErrorAction SilentlyContinue }
  Start-Sleep -Seconds 3
}

function Start-Server([string[]]$extra) {
  Stop-Server
  $argList = @("serve", "-config", $config, "-no-save") + $extra
  Write-Host ("Starting: {0} {1}" -f $exe, ($argList -join " ")) -ForegroundColor DarkGray
  $proc = Start-Process -FilePath $exe -ArgumentList $argList -PassThru -WindowStyle Hidden `
    -RedirectStandardOutput "$env:TEMP\malaikat-deep-out.txt" `
    -RedirectStandardError "$env:TEMP\malaikat-deep-err.txt"
  $deadline = (Get-Date).AddMinutes(25)
  while ((Get-Date) -lt $deadline) {
    try {
      $r = Invoke-WebRequest -Uri "http://127.0.0.1:$port/health" -UseBasicParsing -TimeoutSec 2
      if ($r.StatusCode -eq 200) { return $proc }
    } catch {}
    if ($proc.HasExited) {
      throw ("server exited early`n" + (Get-Content "$env:TEMP\malaikat-deep-err.txt","$env:TEMP\malaikat-deep-out.txt" -Raw -ErrorAction SilentlyContinue))
    }
    Start-Sleep -Seconds 2
  }
  throw "server not ready"
}

$variants = @(
  @{ name = "baseline"; args = @() },
  @{ name = "b2048_ub1024"; args = @("-b", "2048", "-ub", "1024") },
  @{
    name = "mtp_ngram_simple"
    args = @(
      "-spec-type", "draft-mtp,ngram-simple",
      "--",
      "--spec-ngram-simple-size-n", "12",
      "--spec-ngram-simple-size-m", "48"
    )
  },
  @{
    name = "draft_kv_q8"
    args = @("--", "--cache-type-k-draft", "q8_0", "--cache-type-v-draft", "q8_0")
  }
)

$all = @()
foreach ($v in $variants) {
  Write-Host "`n################ $($v.name) ################" -ForegroundColor Magenta
  try {
    Start-Server $v.args | Out-Null
    $out = Join-Path $root ("bench-deep-{0}.json" -f $v.name)
    $env:MALAIKAT_BENCH_LABEL = $v.name
    $env:MALAIKAT_BENCH_RUNS = "4"
    $env:MALAIKAT_BENCH_N = "128"
    $env:MALAIKAT_PREFIX_TOKENS = "8192"
    $env:MALAIKAT_BENCH_OUT = $out
    & powershell -NoProfile -ExecutionPolicy Bypass -File (Join-Path $PSScriptRoot "bench-deep.ps1")
    $json = Get-Content $out -Raw | ConvertFrom-Json
    $row = [pscustomobject]@{
      name = $v.name
      args = ($v.args -join " ")
      tg = $json.tg.mean
      tg_stdev = $json.tg.stdev
      pp = $json.pp.mean
      pp_stdev = $json.pp.stdev
      prompt_n = $json.prompt_n_last
      ok = $true
    }
    $all += $row
    Write-Host ("SUMMARY {0}: tg={1}±{2} pp={3} prompt_n={4}" -f $row.name, $row.tg, $row.tg_stdev, $row.pp, $row.prompt_n) -ForegroundColor Green
  } catch {
    Write-Host ("FAIL {0}: {1}" -f $v.name, $_) -ForegroundColor Red
    $all += [pscustomobject]@{ name = $v.name; args = ($v.args -join " "); tg = -1; tg_stdev = 0; pp = -1; pp_stdev = 0; prompt_n = 0; ok = $false }
  }
}

Write-Host "`n======== DEEP COMPARISON (~8k prefix, ctx uncapped) ========" -ForegroundColor Yellow
$all | Sort-Object tg -Descending | Format-Table name, tg, tg_stdev, pp, prompt_n, ok -AutoSize
$all | ConvertTo-Json -Depth 5 | Set-Content $resultsPath -Encoding utf8
Write-Host "Wrote $resultsPath"
# Restore original config server for continued use
Start-Server @() | Out-Null
Write-Host "Restored baseline server (coding.yaml). done"
