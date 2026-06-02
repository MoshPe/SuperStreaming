# Test: MediaMTX read replica API returns 200
$response = Invoke-WebRequest -Uri "http://localhost:9998/v3/config/global/get" -UseBasicParsing -ErrorAction SilentlyContinue
if ($response.StatusCode -eq 200) {
    Write-Host "PASS: read replica API healthy" -ForegroundColor Green
    exit 0
} else {
    Write-Host "FAIL: read API not responding (expected after Task 5)" -ForegroundColor Yellow
    exit 1
}
