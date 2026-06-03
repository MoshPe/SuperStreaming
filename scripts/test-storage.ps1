$pass = $true

# MinIO S3 health
$r = Invoke-WebRequest -Uri "http://localhost:9000/minio/health/live" -UseBasicParsing -ErrorAction SilentlyContinue
if ($r.StatusCode -eq 200) {
    Write-Host "PASS: MinIO healthy" -ForegroundColor Green
} else {
    Write-Host "FAIL: MinIO not healthy (expected after Task 7)" -ForegroundColor Yellow
    $pass = $false
}

# Postgres ping
$pg = docker exec postgres pg_isready -U superstreaming 2>&1
if ($pg -match "accepting connections") {
    Write-Host "PASS: Postgres accepting connections" -ForegroundColor Green
} else {
    Write-Host "FAIL: Postgres not ready (expected after Task 7)" -ForegroundColor Yellow
    $pass = $false
}

if ($pass) { exit 0 } else { exit 1 }
