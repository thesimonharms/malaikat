# Restart serve with coding.yaml + optional overrides, then run bench-baseline.ps1.
$ErrorActionPreference = "Stop"
$root = Resolve-Path "$PSScriptRoot\.."
Set-Location $root
$port = 8080
$exe = Join-Path $root "malaikat.exe"
$config = Join-Path $root "coding.yaml"
$resultsPath = Join-Path $root "bench-experiments.json"

function Stop-Server {
  $pids = @()
  try { $pids += (Get-NetTCPConnection -LocalPort $port -State Listen -ErrorAction SilentlyContinue).OwningProcess } catch {}
  $pids += (Get-Process llama-server,malaikat -ErrorAction SilentlyContinue).Id
  $pids = $pids | Where-Object { $_ } | Select-Object -Unique
  foreach ($procId in $pids) {
    Stop-Process -Id $procId -Force -ErrorAction SilentlyContinue
  }
  Start-Sleep -Seconds 3
}

function Start-Server([string[]]$extra) {
  Stop-Server
  $argList = @("serve", "-config", $config, "-no-save") + $extra
  Write-Host ("Starting: {0} {1}" -f $exe, ($argList -join " ")) -ForegroundColor DarkGray
  $proc = Start-Process -FilePath $exe -ArgumentList $argList -PassThru -WindowStyle Hidden `
    -RedirectStandardOutput "$env:TEMP\malaikat-exp-out.txt" `
    -RedirectStandardError "$env:TEMP\malaikat-exp-err.txt"
  $deadline = (Get-Date).AddMinutes(25)
  while ((Get-Date) -lt $deadline) {
    try {
      $r = Invoke-WebRequest -Uri "http://127.0.0.1:$port/health" -UseBasicParsing -TimeoutSec 2
      if ($r.StatusCode -eq 200) { return $proc }
    } catch {}
    if ($proc.HasExited) {
      $err = Get-Content "$env:TEMP\malaikat-exp-err.txt" -Raw -ErrorAction SilentlyContinue
      $out = Get-Content "$env:TEMP\malaikat-exp-out.txt" -Raw -ErrorAction SilentlyContinue
      throw "server exited early`nOUT:`n$out`nERR:`n$err"
    }
    Start-Sleep -Seconds 2
  }
  throw "server not ready"
}

$variants = @(
  @{
    name = "baseline"
    args = @()
  },
  @{
    name = "n4"
    args = @("-spec-draft-n-max", "4")
  },
  @{
    name = "draft_kv_q8"
    args = @("--", "--cache-type-k-draft", "q8_0", "--cache-type-v-draft", "q8_0")
  },
  @{
    name = "mtp_ngram_mod"
    args = @(
      "-spec-type", "draft-mtp,ngram-mod",
      "--",
      "--spec-ngram-mod-n-match", "24",
      "--spec-ngram-mod-n-min", "48",
      "--spec-ngram-mod-n-max", "64"
    )
  },
  @{
    name = "b2048_ub512"
    args = @("-b", "2048", "-ub", "512")
  },
  @{
    name = "b2048_ub1024"
    args = @("-b", "2048", "-ub", "1024")
  },
  @{
    name = "n4_mtp_ngram"
    args = @(
      "-spec-draft-n-max", "4",
      "-spec-type", "draft-mtp,ngram-mod",
      "--",
      "--spec-ngram-mod-n-match", "24",
      "--spec-ngram-mod-n-min", "48",
      "--spec-ngram-mod-n-max", "64"
    )
  }
)

$all = @()
# Prefer existing baseline file if present and first variant is baseline — still re-run for fairness under same thermal state.
foreach ($v in $variants) {
  Write-Host "`n################ $($v.name) ################" -ForegroundColor Magenta
  try {
    Start-Server $v.args | Out-Null
    $out = Join-Path $root ("bench-{0}.json" -f $v.name)
    $env:MALAIKAT_BENCH_LABEL = $v.name
    $env:MALAIKAT_BENCH_RUNS = "5"
    $env:MALAIKAT_BENCH_N = "128"
    $env:MALAIKAT_BENCH_OUT = $out
    & powershell -NoProfile -ExecutionPolicy Bypass -File (Join-Path $PSScriptRoot "bench-baseline.ps1")
    $json = Get-Content $out -Raw | ConvertFrom-Json
    $short = $json.suites | Where-Object { $_.name -eq "short_codegen" } | Select-Object -First 1
    $long = $json.suites | Where-Object { $_.name -eq "long_code_review" } | Select-Object -First 1
    $row = [pscustomobject]@{
      name = $v.name
      args = ($v.args -join " ")
      short_tg = $short.tg.mean
      short_tg_stdev = $short.tg.stdev
      short_pp = $short.pp.mean
      long_tg = $long.tg.mean
      long_tg_stdev = $long.tg.stdev
      long_pp = $long.pp.mean
      ok = $true
    }
    $all += $row
    Write-Host ("SUMMARY {0}: short_tg={1} long_tg={2} long_pp={3}" -f $row.name, $row.short_tg, $row.long_tg, $row.long_pp) -ForegroundColor Green
  } catch {
    Write-Host ("FAIL {0}: {1}" -f $v.name, $_) -ForegroundColor Red
    $all += [pscustomobject]@{
      name = $v.name; args = ($v.args -join " "); short_tg = -1; short_tg_stdev = 0; short_pp = -1
      long_tg = -1; long_tg_stdev = 0; long_pp = -1; ok = $false; error = "$_"
    }
  }
}

Write-Host "`n======== COMPARISON (by long_tg then short_tg) ========" -ForegroundColor Yellow
$all | Sort-Object long_tg, short_tg -Descending | Format-Table name, short_tg, short_pp, long_tg, long_pp, ok -AutoSize
$all | ConvertTo-Json -Depth 5 | Set-Content $resultsPath -Encoding utf8
Write-Host "Wrote $resultsPath"
Stop-Server
Write-Host "done"
