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
Start-PF @("port-forward", "statefulset/mediamtx-origin", "29997:9997", "-n", "superstreaming")
Start-PF @("port-forward", "deployment/mediamtx-read-0",  "29998:9997", "-n", "superstreaming")
Start-PF @("port-forward", "deployment/mediamtx-read-1",  "29999:9997", "-n", "superstreaming")
Start-PF @("port-forward", "deployment/mediamtx-read-2",  "30000:9997", "-n", "superstreaming")
Start-PF @("port-forward", "deployment/prometheus",        "29090:9090", "-n", "superstreaming")
Start-PF @("port-forward", "deployment/grafana",           "23000:3000", "-n", "superstreaming")
Start-PF @("port-forward", "deployment/web-frontend",      "20080:80",   "-n", "superstreaming")
Start-Sleep 5

Test-PF "MediaMTX origin-0 API" 29997 "/v3/config/global/get" 200
Test-PF "MediaMTX read-0 API"   29998 "/v3/config/global/get" 200
Test-PF "MediaMTX read-1 API"   29999 "/v3/config/global/get" 200
Test-PF "MediaMTX read-2 API"   30000 "/v3/config/global/get" 200
Test-PF "Prometheus"            29090 "/-/healthy"            200
Test-PF "Grafana"               23000 "/api/health"           200
Test-PF "Web frontend"          20080 "/"                     200

$pg = kubectl exec -n superstreaming deployment/postgres -- pg_isready -U superstreaming 2>&1
if ($pg -match "accepting connections") {
    Write-Host "PASS: Postgres accepting connections" -ForegroundColor Green
} else {
    Write-Host "FAIL: Postgres not ready" -ForegroundColor Red
    $pass = $false
}

# Verify all 3 origins running
for ($i = 0; $i -lt 3; $i++) {
    $pod = kubectl get pod "mediamtx-origin-$i" -n superstreaming --no-headers 2>&1
    if ($pod -match "2/2.*Running") {
        Write-Host "PASS: mediamtx-origin-$i Running 2/2" -ForegroundColor Green
    } else {
        Write-Host "FAIL: mediamtx-origin-$i not ready: $pod" -ForegroundColor Red
        $pass = $false
    }
}

# Verify all 3 read deployments have 2 ready replicas
for ($i = 0; $i -lt 3; $i++) {
    $dep = kubectl get deployment "mediamtx-read-$i" -n superstreaming --no-headers 2>&1
    if ($dep -match "2/2") {
        Write-Host "PASS: mediamtx-read-$i 2/2 ready" -ForegroundColor Green
    } else {
        Write-Host "FAIL: mediamtx-read-$i not ready: $dep" -ForegroundColor Red
        $pass = $false
    }
}

$pfProcs | Stop-Process -ErrorAction SilentlyContinue

if ($pass) { Write-Host "`nAll k8s services HEALTHY" -ForegroundColor Green; exit 0 }
else { Write-Host "`nSome k8s services FAILED" -ForegroundColor Red; exit 1 }
