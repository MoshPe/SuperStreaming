# Start local development tunnel.
# Runs a single kubectl port-forward through Traefik on localhost:8080.
# All traffic routes through Traefik: UI, API, and WHEP per-origin.
#
# Prerequisites:
#   1. Docker Desktop Kubernetes running
#   2. Traefik installed: .\install-traefik.ps1
#   3. SuperStreaming pods running: kubectl apply -f ..\k8s\
#
# Usage: .\dev-start.ps1

$ErrorActionPreference = "Stop"

Write-Host "Checking cluster..." -ForegroundColor Cyan

$notReady = kubectl get pods -n superstreaming --no-headers 2>&1 |
    Where-Object { $_ -notmatch "Running|Completed" }
if ($notReady) {
    Write-Host "Some pods not ready:" -ForegroundColor Yellow
    $notReady | ForEach-Object { Write-Host "  $_" }
    Write-Host "Run: kubectl apply -f k8s/" -ForegroundColor Yellow
}

$traefik = kubectl get pods -n traefik --no-headers 2>&1
if ($traefik -notmatch "Running") {
    Write-Host "Traefik not running. Install it first: .\install-traefik.ps1" -ForegroundColor Red
    exit 1
}

Write-Host ""
Write-Host "Starting tunnel: localhost:8080 -> Traefik -> all services" -ForegroundColor Green
Write-Host ""
Write-Host "  UI:        http://localhost:8080/" -ForegroundColor Cyan
Write-Host "  API:       http://localhost:8080/api/" -ForegroundColor Cyan
Write-Host "  WHEP 0:    http://localhost:8080/whep/0/" -ForegroundColor Cyan
Write-Host "  WHEP 1:    http://localhost:8080/whep/1/" -ForegroundColor Cyan
Write-Host "  WHEP 2:    http://localhost:8080/whep/2/" -ForegroundColor Cyan
Write-Host ""
Write-Host "Press Ctrl+C to stop." -ForegroundColor DarkGray

kubectl port-forward -n traefik deployment/traefik 8080:8000
