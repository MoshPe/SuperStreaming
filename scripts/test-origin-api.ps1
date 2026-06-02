# Test: MediaMTX origin API returns 200
$response = Invoke-WebRequest -Uri "http://localhost:9997/v3/config/global/get" -UseBasicParsing -ErrorAction SilentlyContinue
if ($response.StatusCode -eq 200) {
    Write-Host "PASS: origin API healthy" -ForegroundColor Green
    exit 0
} else {
    Write-Host "FAIL: origin API not responding (expected after Task 5)" -ForegroundColor Yellow
    exit 1
}
