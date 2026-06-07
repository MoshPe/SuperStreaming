# SRT Ingest Support — Design Spec

**Date:** 2026-06-07  
**Status:** Approved

## Problem

Far/WAN cameras cannot reliably use RTSP/TCP to reach the AG origin across an untrusted network. RTSP has no encryption, no loss recovery tuned to a latency budget, and RTSP-UDP requires open dynamic port ranges. SRT solves all three: single UDP port, ARQ retransmit within a configurable latency window, and AES-256 encryption.

Internal cluster traffic (origin → read replica) stays RTSP/TCP — clean LAN, no change.

## Architecture

```
[far camera]
     |
     | RTSP/TCP (LAN — near camera)
     v
[edge publisher relay]
     |
     | SRT/UDP (WAN — single port, AES encrypted, latency-budget ARQ)
     v
[AG firewall: UDP 8890]
     |
     v
[mediamtx-origin SRT listener :8890]
     |
     | RTSP/TCP (intra-cluster)
     v
[mediamtx-read] ──WHEP/WebRTC──> [browsers]
```

Local cameras (LAN) continue using existing RTSP/TCP path. Per-stream flag controls which leg uses SRT — no global mode switch, no separate binary.

## Stream Spec Format

Current format (unchanged for existing streams):
```
STREAMS=cam-01=test,cam-02=rtsp://cam-host/stream
```

New `@srt` suffix selects SRT push for a specific stream:
```
STREAMS=cam-01=test,cam-02=rtsp://cam-host/stream,cam-03=rtsp://far-cam/stream@srt
```

- No suffix → RTSP push to origin (existing behavior, backward compatible)
- `@srt` suffix → SRT push to origin; source is still pulled via RTSP/TCP locally

Parsing: `strings.HasSuffix(source, "@srt")` on the full value. Safe even when RTSP source URL contains `@` for auth credentials (e.g. `rtsp://user:pass@host/path@srt`) because the suffix match targets the end of the string unambiguously.

## New Environment Variables (publisher)

| Variable | Default | Description |
|----------|---------|-------------|
| `ORIGIN_SRT_PORT` | `8890` | SRT listener port on origin pods |
| `SRT_LATENCY_MS` | `200` | Latency budget in ms; set ≥ 3× RTT to far site |
| `SRT_PASSPHRASE` | `` | AES encryption key (libsrt default: AES-128); empty disables encryption |

Rule of thumb for `SRT_LATENCY_MS`: measure RTT to origin with `ping`, multiply by 4. Example: 40ms RTT → 160ms budget → use 200ms.

## SRT Target URL

```
srt://origin-host:8890?streamid=publish:STREAM_ID:publisher:PUBLISH_SECRET&latency=200000
```

- `streamid` encodes MediaMTX internal auth: `publish:<path>:<user>:<password>`
- `latency` is in **microseconds** (ffmpeg convention); `SRT_LATENCY_MS × 1000`
- `passphrase=<SRT_PASSPHRASE>` appended only when passphrase is non-empty

No new MediaMTX auth users needed — existing `publisher` user with `PUBLISH_SECRET` handles SRT authentication via streamid.

## ffmpeg Argument Changes

MPEG-TS is the container for SRT transport. MPEG-TS does not support Opus audio; audio must be AAC.

| Source | Target | Video | Audio | Container |
|--------|--------|-------|-------|-----------|
| `test` | `rtsp://` | libx264 | libopus | rtsp *(unchanged)* |
| `rtsp://` | `rtsp://` | copy | copy | rtsp *(unchanged)* |
| `test` | `srt://` | libx264 | **aac** | **mpegts** |
| `rtsp://` | `srt://` | copy | **aac -b:a 128k** | **mpegts** |

Detection: `strings.HasPrefix(target, "srt://")` in `BuildArgs`.

Audio transcoding for `rtsp://` → `srt://` is necessary to ensure MPEG-TS compatibility regardless of source codec. CPU cost is negligible.

## Files Changed

| File | Change |
|------|--------|
| `services/publisher/relay/relay.go` | Detect SRT target; switch to mpegts + aac |
| `services/publisher/relay/relay_test.go` | Add SRT target test cases |
| `services/publisher/main.go` | Parse `@srt` suffix; add SRT config fields; generate SRT target URL |
| `configs/mediamtx/origin.yml` | Enable `srt: true` on `:8890` |
| `docker-compose.yml` | Expose `8890/udp` on origin; add SRT env vars to publisher |
| `.env.example` | Add SRT vars with comments and latency guidance |
| `docs/deployment-guide.md` | SRT section: edge relay pattern, port table, latency tuning |

## Firewall Port Requirements

| Port | Protocol | Direction | Service |
|------|----------|-----------|---------|
| 8554 | TCP | LAN inbound | RTSP publish (existing) |
| **8890** | **UDP** | **WAN inbound** | **SRT ingest (new)** |
| 8889 | TCP | outbound viewer | WebRTC WHEP signaling (existing) |
| 8189 | UDP | outbound viewer | WebRTC media (existing) |
| 3478 | UDP/TCP | both | STUN/TURN (existing) |

Only one new rule: UDP 8890 inbound from edge relay IP(s).

## MediaMTX Origin Config Change

```yaml
srt: true
srtAddress: :8890
```

No auth section changes. SRT connections authenticate via streamid field using existing internal auth.

## Deployment Notes

### Edge relay deployment

The `publisher` service is deployed near the far camera with:
```
STREAMS=cam-id=rtsp://camera-local-ip/stream@srt
ORIGIN_HOST_TEMPLATE=<AG-origin-hostname-or-IP>
ORIGIN_SRT_PORT=8890
SRT_LATENCY_MS=<4x RTT in ms>
SRT_PASSPHRASE=<shared secret for AES-256>
PUBLISH_SECRET=<same as origin>
```

The edge publisher connects outbound to the AG origin's SRT port. AG firewall only needs to allow UDP 8890 inbound from the edge relay's IP — no dynamic port ranges.

### Local Compose (dev/test)

Port `8890/udp` is exposed but optional to use. Existing `PUBLISHER_STREAMS` env can mix local test patterns and SRT:
```
PUBLISHER_STREAMS=cam-01=test,cam-02=test@srt
```
`cam-02=test@srt` sends synthetic test pattern to origin over SRT (useful for smoke-testing the SRT path locally).

## Out of Scope

- SRT source pull (cameras that natively output SRT) — future work
- Per-stream SRT passphrase — all SRT streams share one passphrase per publisher instance
- RIST transport — separate feature
