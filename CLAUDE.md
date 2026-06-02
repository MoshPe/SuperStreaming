# SuperStreaming — Project Memory

## What This Is
Large-scale live streaming platform. Two use cases: security/surveillance (always-on cameras) + live events (ephemeral broadcasts).

## Architecture Decision: Approved
k3s-first design. Docker Compose = full test on Windows. k3s manifests = prod on Ubuntu VM (air-gapped, bare metal).
Same images + configs for both targets. Service names identical across both.

## Scale Targets
- Streams: 20–200 concurrent live streams
- Viewers: 50–500 concurrent (internal + limited external)
- Origins: 2–3 StatefulSet pods
- Read replicas: 3–6 Deployment pods, HPA on connections

## Services (7 total)
| Service | Type | Purpose |
|---------|------|---------|
| mediamtx-origin-{0,1,2} | StatefulSet | Owns streams, records fMP4 to PVC |
| mediamtx-read | Deployment + HPA | Serves WebRTC WHEP to browsers |
| recording-uploader | Sidecar on origin | Watches /recordings, uploads to MinIO, indexes Postgres |
| minio | Single-node pod | S3-compatible durable recording archive |
| postgres | Pod | Recording metadata (stream_id, times, path, duration) |
| prometheus + grafana | Pods | Metrics from MediaMTX :9998/metrics |
| web-frontend | nginx + React | Live viewer (reader.js WHEP) + recording browser |

## Publisher
- Language: Go
- Protocol: RTSP (TCP/UDP)
- Routing: `hash(stream_id) % num_origins` → `mediamtx-origin-{N}:8554/{stream_id}`

## Stream Path Groups
Single group — all streams are always-on live video.
```
{stream_id}  → sourceOnDemand: no  (always connected, read replicas stay pulled)
```

## Auth
MediaMTX static credentials in YAML config. No auth backend.
- Publishers: `publishUser/publishPass` (env: PUBLISH_SECRET)
- Read replicas pulling from origin: `readUser: internal` (env: INTERNAL_SECRET)
- Viewers: `readUser: viewer` (env: VIEWER_SECRET)
- Admin API: internal only, never exposed

## Recording Pipeline
```
mediamtx writes → /recordings/{stream_id}/{timestamp}.mp4 (fMP4, 60s segments)
recording-uploader sidecar:
  watches /recordings via inotify
  on segment complete → upload to MinIO
  write row to Postgres (stream_id, start_time, end_time, minio_path, duration)
  delete local file after confirmed upload
  local PVC retention: 2h (fallback if uploader down)
```

## WebRTC Networking
- TCP 8889: WHEP/WHIP signaling
- UDP 8189: media (happy path, lowest latency)
- TCP 8189: fallback
- Browser → WHEP → read replica → RTSP pull → origin
- H264 baseline + Opus (no H265, no B-frames — browser compat)

## Key Config Decisions
- Read replicas pull from `mediamtx-origin-headless:8554` (headless Service DNS)
- recordDeleteAfter: 2h (PVC hot buffer only, MinIO = durable store)
- recordPartDuration: 1s, recordSegmentDuration: 60s

## Infrastructure
- **Windows dev:** Docker Compose, bridge network `superstreaming`, full system test
- **Ubuntu VM prod:** k3s bare metal, air-gapped
  - Images pre-pulled on Windows → tar export → transfer → `k3s ctr images import`
  - `kubectl apply` deploys all manifests

## Status
- [x] Architecture design approved
- [x] Stream registry design approved
- [x] MediaMTX config shape approved (path groups pending final approval)
- [x] Spec doc written → docs/superpowers/specs/2026-06-02-superstreaming-design.md
- [ ] Implementation plan written
- [ ] Docker Compose implementation
- [ ] k3s manifests implementation
