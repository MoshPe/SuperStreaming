$pass = $true
$pfProcs = @()

function Start-PF($argList) {
    $pf = Start-Process -FilePath "kubectl" -ArgumentList $argList -NoNewWindow -PassThru
    $script:pfProcs += $pf
}

function Test-PF($name, $localPort, $path, $expectedCode) {
    if (-not $path) { $path = "/" }
    if (-not $expectedCode) { $expectedCode = 200 }
    try {
        $r = Invoke-WebRequest -Uri "http://localhost:$localPort$path" -UseBasicParsing -ErrorAction Stop -TimeoutSec 5
        if ($r.StatusCode -eq $expectedCode) {
            Write-Host "PASS: $name (HTTP $($r.StatusCode))" -ForegroundColor Green
        } else {
            Write-Host "FAIL: $name - got $($r.StatusCode), expected $expectedCode" -ForegroundColor Red
            $script:pass = $false
        }
    } catch {
        Write-Host "FAIL: $name - $($_.Exception.Message)" -ForegroundColor Red
        $script:pass = $false
    }
}

Write-Host "Starting port-forwards..."
Start-PF @("port-forward", "pod/mediamtx-origin-0",    "29997:9997", "-n", "superstreaming")
Start-PF @("port-forward", "deployment/mediamtx-read", "29998:9997", "-n", "superstreaming")
Start-PF @("port-forward", "deployment/prometheus",     "29090:9090", "-n", "superstreaming")
Start-PF @("port-forward", "deployment/grafana",        "23000:3000", "-n", "superstreaming")
Start-PF @("port-forward", "deployment/web-frontend",   "20080:80",   "-n", "superstreaming")
Start-Sleep 5

Test-PF "MediaMTX origin API" 29997 "/v3/config/global/get" 200
Test-PF "MediaMTX read API"   29998 "/v3/config/global/get" 200
Test-PF "Prometheus"          29090 "/-/healthy"            200
Test-PF "Grafana"             23000 "/api/health"           200
Test-PF "Web frontend stub"   20080 "/"                     200

$pg = kubectl exec -n superstreaming deployment/postgres -- pg_isready -U superstreaming 2>&1
if ($pg -match "accepting connections") {
    Write-Host "PASS: Postgres accepting connections" -ForegroundColor Green
} else {
    Write-Host "FAIL: Postgres not ready" -ForegroundColor Red
    $pass = $false
}

$pfProcs | Stop-Process -ErrorAction SilentlyContinue

if ($pass) { Write-Host "`nAll k8s services HEALTHY" -ForegroundColor Green; exit 0 }
else { Write-Host "`nSome k8s services FAILED" -ForegroundColor Red; exit 1 }
