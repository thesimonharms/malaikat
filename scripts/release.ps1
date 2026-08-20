# Build an all-in-one malaikat.exe (launcher + lemonade ROCm gfx1151 runtime).
# Model GGUFs are NOT bundled.
#
# Usage:
#   .\scripts\release.ps1              # build only → dist\malaikat-0.1.0-windows-amd64.exe
#   .\scripts\release.ps1 -Publish     # build, tag, and upload GitHub release

param(
    [string]$Version = "0.1.0",
    [string]$Tag = "",
    [string]$LlamaTag = "",
    [switch]$Publish,
    [switch]$SkipDownload
)

$ErrorActionPreference = "Stop"
$Root = Split-Path -Parent (Split-Path -Parent $MyInvocation.MyCommand.Path)
Set-Location $Root

if (-not $Tag) { $Tag = "v$Version" }

$EmbedDir = Join-Path $Root "internal\engine\embedded"
$ZipPath = Join-Path $EmbedDir "runtime.zip"
$TagPath = Join-Path $EmbedDir "runtime.tag"
$DistDir = Join-Path $Root "dist"
$OutName = "malaikat-$Version-windows-amd64.exe"
$OutPath = Join-Path $DistDir $OutName

New-Item -ItemType Directory -Force -Path $EmbedDir | Out-Null
New-Item -ItemType Directory -Force -Path $DistDir | Out-Null

if (-not $SkipDownload -or -not (Test-Path $ZipPath)) {
    Write-Host "Resolving lemonade-sdk/llamacpp-rocm Windows gfx1151 asset..."
    if ($LlamaTag) {
        $relJson = gh api "repos/lemonade-sdk/llamacpp-rocm/releases/tags/$LlamaTag"
    } else {
        $relJson = gh api "repos/lemonade-sdk/llamacpp-rocm/releases/latest"
    }
    $rel = $relJson | ConvertFrom-Json
    $asset = $rel.assets | Where-Object { $_.name -match "windows-rocm-gfx1151-x64\.zip" } | Select-Object -First 1
    if (-not $asset) {
        throw "No windows-rocm-gfx1151-x64.zip in release $($rel.tag_name)"
    }
    $LlamaTag = $rel.tag_name
    Write-Host "Downloading $($asset.name) ($([math]::Round($asset.size/1MB)) MB) from $($rel.tag_name)..."
    gh release download $LlamaTag -R lemonade-sdk/llamacpp-rocm -p $asset.name -D $EmbedDir --clobber
    Move-Item -Force (Join-Path $EmbedDir $asset.name) $ZipPath
    Set-Content -Path $TagPath -Value $LlamaTag -NoNewline
} else {
    if (-not (Test-Path $TagPath)) {
        throw "Missing $TagPath (re-run without -SkipDownload)"
    }
    $LlamaTag = (Get-Content $TagPath -Raw).Trim()
    Write-Host "Using cached runtime.zip (tag $LlamaTag)"
}

Write-Host "Building all-in-one $OutName (embedruntime, llama $LlamaTag)..."
$env:CGO_ENABLED = "0"
go build -tags embedruntime -trimpath -ldflags "-s -w" -o $OutPath .
if ($LASTEXITCODE -ne 0) { throw "go build failed" }

$sizeMB = [math]::Round((Get-Item $OutPath).Length / 1MB, 1)
Write-Host "Built $OutPath ($sizeMB MB)"

& $OutPath version
if ($LASTEXITCODE -ne 0) { throw "version check failed" }

Copy-Item -Force (Join-Path $Root "coding.example.yaml") (Join-Path $DistDir "coding.example.yaml")

if (-not $Publish) {
    Write-Host ""
    Write-Host "Build only. To publish: .\scripts\release.ps1 -Publish"
    exit 0
}

Write-Host "Publishing GitHub release $Tag..."
$existing = git tag -l $Tag
if (-not $existing) {
    git tag -a $Tag -m "malaikat $Version"
    git push origin $Tag
} else {
    Write-Host "Tag $Tag already exists locally"
    git push origin $Tag 2>$null
}

$notes = @"
## malaikat $Version

All-in-one Windows exe for AMD Strix Halo (gfx1151) ROCm inference.

**Bundled:** malaikat launcher + lemonade-sdk ``llamacpp-rocm`` ``$LlamaTag`` (Windows ROCm gfx1151).
**Not bundled:** GGUF model weights — pass ``-m`` or a config.

### Quick start

``````powershell
.\malaikat-$Version-windows-amd64.exe serve -m path\to\moe-mtp.gguf
``````

First run extracts the ROCm runtime to ``%LocalAppData%\malaikat\runtime``. Optional config: see ``coding.example.yaml`` in this release.

API: ``http://127.0.0.1:8080/v1``
"@

$releaseExists = gh release view $Tag 2>$null
if ($LASTEXITCODE -eq 0) {
    Write-Host "Release $Tag exists; uploading asset..."
    gh release upload $Tag $OutPath (Join-Path $DistDir "coding.example.yaml") --clobber
} else {
    gh release create $Tag $OutPath (Join-Path $DistDir "coding.example.yaml") --title "malaikat $Version" --notes $notes
}

Write-Host "Done: https://github.com/thesimonharms/malaikat/releases/tag/$Tag"
