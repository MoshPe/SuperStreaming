#!/usr/bin/env pwsh
# Air-gap image export: builds custom images then saves all images to a tar.gz.
# Run on the Windows dev machine before transferring to the Ubuntu VM.

param(
    [string]$OutputFile = "superstreaming-images.tar.gz",
    [switch]$SkipBuild
)

$ErrorActionPreference = "Stop"
$ProjectRoot = Split-Path -Parent $PSScriptRoot

Write-Host "`nSuperStreaming — Air-Gap Image Export`n" -ForegroundColor Cyan

# ── 1. Build custom images ────────────────────────────────────────────────────
if (-not $SkipBuild) {
    Write-Host "[1/3] Building custom images..." -ForegroundColor Yellow
    Push-Location $ProjectRoot
    try {
        docker compose build --no-cache
        if ($LASTEXITCODE -ne 0) { throw "docker compose build failed (exit $LASTEXITCODE)" }
    } finally {
        Pop-Location
    }
    Write-Host "      Build complete." -ForegroundColor Green
} else {
    Write-Host "[1/3] Skipping build (-SkipBuild)." -ForegroundColor Gray
}

# ── 2. Collect image list ─────────────────────────────────────────────────────
Write-Host "[2/3] Collecting images..." -ForegroundColor Yellow

$images = @(
    "superstreaming/mediamtx:local",
    "superstreaming/recording-uploader:latest",
    "superstreaming/api-server:latest",
    "superstreaming/web-frontend:latest",
    "postgres:16-alpine",
    "minio/minio:latest",
    "prom/prometheus:latest",
    "grafana/grafana:latest"
)

# Verify all images exist locally
foreach ($img in $images) {
    $exists = docker image inspect $img 2>$null
    if ($LASTEXITCODE -ne 0) {
        Write-Warning "Image not found locally: $img — pull it first or run without -SkipBuild"
    }
}

Write-Host "      Images to export:"
$images | ForEach-Object { Write-Host "        $_" -ForegroundColor Gray }

# ── 3. Export ─────────────────────────────────────────────────────────────────
Write-Host "[3/3] Saving to $OutputFile ..." -ForegroundColor Yellow

$OutPath = Join-Path $ProjectRoot $OutputFile
$imageArgs = $images -join " "

# docker save pipes to gzip
$cmd = "docker save $imageArgs | gzip > `"$OutPath`""
Invoke-Expression $cmd
if ($LASTEXITCODE -ne 0) { throw "docker save failed (exit $LASTEXITCODE)" }

$sizeMB = [math]::Round((Get-Item $OutPath).Length / 1MB, 1)
Write-Host "`nExport complete: $OutputFile ($sizeMB MB)" -ForegroundColor Green
Write-Host "Transfer to Ubuntu VM then run: bash scripts/airgap-import.sh $OutputFile`n"
