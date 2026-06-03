# Smoke test: publish RTSP stream, verify visible via origin and read APIs, test WHEP
param(
    [string]$PublishSecret = $env:PUBLISH_SECRET,
    [string]$ViewerSecret = $env:VIEWER_SECRET
)

$pass = $true

# Check origin
$r = Invoke-RestMethod "http://localhost:9997/v3/paths/list" -ErrorAction SilentlyContinue
if ($r.items.Count -gt 0) {
    Write-Host "PASS: origin has $($r.items.Count) active stream(s): $($r.items[0].tracks -join ', ')" -ForegroundColor Green
} else {
    Write-Host "FAIL: no streams on origin" -ForegroundColor Red
    $pass = $false
}

# Check read replica
$r2 = Invoke-RestMethod "http://localhost:9998/v3/paths/list" -ErrorAction SilentlyContinue
if ($r2.items.Count -gt 0) {
    Write-Host "PASS: read replica has $($r2.items.Count) active stream(s)" -ForegroundColor Green
} else {
    Write-Host "FAIL: no streams on read replica" -ForegroundColor Red
    $pass = $false
}

# Check WHEP endpoint — send valid H264+Opus SDP offer (video-only offer also accepted)
$sdpLines = @(
    "v=0",
    "o=- 4611731400430051336 2 IN IP4 127.0.0.1",
    "s=-",
    "t=0 0",
    "a=group:BUNDLE 0 1",
    "m=video 9 UDP/TLS/RTP/SAVPF 96",
    "c=IN IP4 0.0.0.0",
    "a=ice-ufrag:ABcd",
    "a=ice-pwd:ABcdEFghIJklMNopQRstUVwx",
    "a=fingerprint:sha-256 AA:BB:CC:DD:EE:FF:00:11:22:33:44:55:66:77:88:99:AA:BB:CC:DD:EE:FF:00:11:22:33:44:55:66:77:88:99",
    "a=setup:actpass",
    "a=mid:0",
    "a=recvonly",
    "a=rtcp-mux",
    "a=rtpmap:96 H264/90000",
    "a=fmtp:96 level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=42e01f",
    "m=audio 9 UDP/TLS/RTP/SAVPF 111",
    "c=IN IP4 0.0.0.0",
    "a=ice-ufrag:ABcd",
    "a=ice-pwd:ABcdEFghIJklMNopQRstUVwx",
    "a=fingerprint:sha-256 AA:BB:CC:DD:EE:FF:00:11:22:33:44:55:66:77:88:99:AA:BB:CC:DD:EE:FF:00:11:22:33:44:55:66:77:88:99",
    "a=setup:actpass",
    "a=mid:1",
    "a=recvonly",
    "a=rtcp-mux",
    "a=rtpmap:111 opus/48000/2",
    "a=fmtp:111 minptime=10;useinbandfec=1"
)
$sdpOffer = ($sdpLines -join "`r`n") + "`r`n"
$cred = [Convert]::ToBase64String([Text.Encoding]::ASCII.GetBytes("viewer:$ViewerSecret"))
$headers = @{ "Authorization" = "Basic $cred"; "Content-Type" = "application/sdp" }
try {
    $w = Invoke-WebRequest -Uri "http://localhost:8889/test-stream/whep" -Method POST -Headers $headers -Body $sdpOffer -UseBasicParsing
    Write-Host "PASS: WHEP handshake succeeded (HTTP $($w.StatusCode))" -ForegroundColor Green
} catch {
    Write-Host "FAIL: WHEP handshake failed: $($_.Exception.Message)" -ForegroundColor Red
    $pass = $false
}

if ($pass) { Write-Host "`nAll streaming path tests PASSED" -ForegroundColor Green; exit 0 }
else { Write-Host "`nSome tests FAILED" -ForegroundColor Red; exit 1 }
