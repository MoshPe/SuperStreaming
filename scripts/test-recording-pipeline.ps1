param(
    [string]$MinioUser = $env:MINIO_ROOT_USER,
    [string]$MinioPass = $env:MINIO_ROOT_PASSWORD,
    [string]$PgUser = $env:POSTGRES_USER,
    [string]$PgPass = $env:POSTGRES_PASSWORD,
    [string]$PgDb = $env:POSTGRES_DB
)

$pass = $true

# MinIO: check for at least one object under recordings/test-stream/
$objects = docker run --rm --network superstreaming_superstreaming `
    -e "MC_HOST_local=http://${MinioUser}:${MinioPass}@minio:9000" `
    minio/mc:latest ls "local/recordings/recordings/test-stream/" 2>$null
if ($objects) {
    Write-Host "PASS: MinIO has objects for test-stream" -ForegroundColor Green
} else {
    Write-Host "FAIL: no MinIO objects for test-stream (run after publishing 65s)" -ForegroundColor Red
    $pass = $false
}

# Postgres: check recordings table has rows for test-stream
$rows = docker exec postgres psql -U $PgUser -d $PgDb -t -c "SELECT count(*) FROM recordings WHERE stream_id='test-stream';"
$count = [int]($rows.Trim())
if ($count -gt 0) {
    Write-Host "PASS: Postgres has $count recording(s) for test-stream" -ForegroundColor Green
} else {
    Write-Host "FAIL: no rows in recordings for test-stream" -ForegroundColor Red
    $pass = $false
}

# Local file deleted: recordings dir inside origin container should be empty
$files = docker exec mediamtx-origin-0 find /recordings/test-stream -name "*.mp4" 2>$null
if (-not $files) {
    Write-Host "PASS: no local .mp4 files remaining (all deleted after upload)" -ForegroundColor Green
} else {
    Write-Host "WARN: local files still present (uploader may still be processing): $files" -ForegroundColor Yellow
}

if ($pass) { Write-Host "`nRecording pipeline PASS" -ForegroundColor Green; exit 0 }
else { Write-Host "`nRecording pipeline FAIL" -ForegroundColor Red; exit 1 }
