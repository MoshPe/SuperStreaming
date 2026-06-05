#!/usr/bin/env pwsh
# Smoke tests for api-server. Requires the stack to be running.

param(
    [string]$BaseUrl = "http://localhost:8080"
)

$ErrorActionPreference = "Stop"
$pass = 0
$fail = 0

function Assert-Ok {
    param([string]$Label, [scriptblock]$Test)
    try {
        & $Test
        Write-Host "  PASS  $Label" -ForegroundColor Green
        $script:pass++
    } catch {
        Write-Host "  FAIL  $Label — $_" -ForegroundColor Red
        $script:fail++
    }
}

Write-Host "`napi-server smoke tests → $BaseUrl`n"

# 1. Health check
Assert-Ok "GET /health returns 200 {status:ok}" {
    $r = Invoke-RestMethod -Uri "$BaseUrl/health" -Method GET
    if ($r.status -ne "ok") { throw "status = $($r.status)" }
}

# 2. Recordings endpoint returns 200 with segments array
Assert-Ok "GET /api/recordings returns segments array" {
    $r = Invoke-RestMethod -Uri "$BaseUrl/api/recordings" -Method GET
    if ($null -eq $r.segments) { throw "no segments field" }
}

# 3. Recordings with nonexistent stream_id returns empty segments
Assert-Ok "GET /api/recordings?stream_id=nonexistent returns empty segments" {
    $r = Invoke-RestMethod -Uri "$BaseUrl/api/recordings?stream_id=nonexistent" -Method GET
    if ($r.segments.Count -ne 0) { throw "expected 0 segments, got $($r.segments.Count)" }
}

# 4. SSE endpoint upgrades to text/event-stream and sends first event within 5s
Assert-Ok "GET /api/streams/live returns text/event-stream with event within 5s" {
    $job = Start-Job {
        param($url)
        try {
            $req = [System.Net.HttpWebRequest]::Create($url)
            $req.Timeout = 5000
            $req.ReadWriteTimeout = 5000
            $resp = $req.GetResponse()
            $ct = $resp.ContentType
            $stream = $resp.GetResponseStream()
            $reader = New-Object System.IO.StreamReader($stream)
            $line = $reader.ReadLine()
            $reader.Close()
            $resp.Close()
            return @{ ContentType = $ct; FirstLine = $line }
        } catch {
            return @{ Error = $_.ToString() }
        }
    } -ArgumentList "$BaseUrl/api/streams/live"

    $result = $job | Wait-Job -Timeout 6 | Receive-Job
    Remove-Job $job -Force -ErrorAction SilentlyContinue

    if ($result.Error) { throw $result.Error }
    if ($result.ContentType -notmatch "text/event-stream") {
        throw "Content-Type = $($result.ContentType)"
    }
    if ([string]::IsNullOrEmpty($result.FirstLine)) {
        throw "no data received within 5s"
    }
}

# 5. CORS header present
Assert-Ok "GET /health includes Access-Control-Allow-Origin header" {
    $r = Invoke-WebRequest -Uri "$BaseUrl/health" -Method GET -UseBasicParsing
    if (-not $r.Headers["Access-Control-Allow-Origin"]) {
        throw "missing CORS header"
    }
}

Write-Host "`nResults: $pass passed, $fail failed`n"
if ($fail -gt 0) { exit 1 }
