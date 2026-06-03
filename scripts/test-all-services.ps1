$pass = $true

function Test-Http {
    param($name, $url, $expectedStatus = 200)
    try {
        $r = Invoke-WebRequest -Uri $url -UseBasicParsing -ErrorAction Stop
        if ($r.StatusCode -eq $expectedStatus) {
            Write-Host "PASS: $name ($url)" -ForegroundColor Green
        } else {
            Write-Host "FAIL: $name - got $($r.StatusCode), expected $expectedStatus" -ForegroundColor Red
            $script:pass = $false
        }
    } catch {
        Write-Host "FAIL: $name - $($_.Exception.Message)" -ForegroundColor Red
        $script:pass = $false
    }
}

Test-Http "MediaMTX origin API"    "http://localhost:9997/v3/config/global/get"
Test-Http "MediaMTX read API"      "http://localhost:9998/v3/config/global/get"
Test-Http "MinIO health"           "http://localhost:9000/minio/health/live"
Test-Http "API server stub"        "http://localhost:8080/"
Test-Http "Web frontend stub"      "http://localhost:80/"
Test-Http "Prometheus"             "http://localhost:9090/-/healthy"
Test-Http "Grafana"                "http://localhost:3000/api/health"

# Postgres
$pg = docker exec postgres pg_isready -U superstreaming 2>&1
if ($pg -match "accepting connections") {
    Write-Host "PASS: Postgres accepting connections" -ForegroundColor Green
} else {
    Write-Host "FAIL: Postgres not ready" -ForegroundColor Red
    $pass = $false
}

if ($pass) { Write-Host "`nAll services HEALTHY" -ForegroundColor Green; exit 0 }
else { Write-Host "`nSome services FAILED" -ForegroundColor Red; exit 1 }
