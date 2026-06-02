# SuperStreaming — System Design Spec
**Date:** 2026-06-02  
**Status:** Approved

---

## 1. Purpose

Large-scale live streaming platform for two use cases:
- **Surveillance** — always-on camera streams, persistent recording, low-latency live view
- **Live video** — operator-published streams, recorded and browsable

Air-gapped deployment on bare-metal Ubuntu VM running k3s. Full test environment on Windows (Docker Compose + Rancher Desktop k3s).

---

## 2. Scale Targets

| Dimension | Target |
|-----------|--------|
| Concurrent live streams | 20–200 |
| Concurrent viewers | 50–500 |
| Origin pods | 2–3 (StatefulSet) |
| Read replica pods | 3–6 (Deployment + HPA) |
| Recording retention (PVC) | 2h hot buffer |
| Recording retention (MinIO) | Configurable, long-term |

---

## 3. Architecture Overview

```
Publishers (Go app, RTSP)
        │
        │ hash(stream_id) % N → origin-N
        ▼ RTSP :8554
┌─────────────────────────────────┐
│  mediamtx-origin-{0,1,2}        │  StatefulSet
│  owns streams                   │  stable DNS identity
│  records fMP4 to PVC            │  2-3 replicas
│  exposes internal RTSP          │
│  └─ recording-uploader sidecar  │  Go, fsnotify
└────────────────┬────────────────┘
                 │ internal RTSP (sourceOnDemand: no)
                 ▼
┌─────────────────────────────────┐
│  mediamtx-read                  │  Deployment + HPA
│  serves WebRTC WHEP to browsers │  3-6 replicas
│  no recording                   │
└────────────────┬────────────────┘
                 │ WebRTC WHEP + UDP media
                 ▼
┌─────────────────────────────────┐
│  web-frontend (nginx + React)   │
│  live viewer: reader.js WHEP    │
│  recording browser              │
└─────────────────────────────────┘

┌──────────────────┐   ┌──────────────┐   ┌────────────────┐
│ recording-uploader│──►│    minio     │   │   api-server   │
│ watches /recordings│  │ S3 archive  │   │ Go, :8080      │
│ uploads segments  │  │ :9000/:9001  │   │ queries:       │
│ writes postgres   │  └──────────────┘   │ - MediaMTX API │
└──────────────────┘                      │ - Postgres     │
                                          │ - MinIO presign│
┌──────────────────┐                      │ SSE stream list│
│    postgres      │◄─────────────────────┘
│ recording index  │
│ :5432            │
└──────────────────┘

┌──────────────────┐   ┌──────────────┐
│   prometheus     │   │   grafana    │
│ scrapes :9998    │──►│  dashboards  │
│ all MediaMTX pods│   │  :3000       │
└──────────────────┘   └──────────────┘
```

---

## 4. Services

| Service | Kind | Ports | Purpose |
|---------|------|-------|---------|
| mediamtx-origin-{0,1,2} | StatefulSet | 8554 RTSP, 9997 API, 9998 metrics | Stream ownership, recording |
| recording-uploader | Sidecar (on origin) | — | Upload fMP4 → MinIO, index → Postgres |
| mediamtx-read | Deployment + HPA | 8889 WebRTC HTTP, 8189 UDP, 9998 metrics | Serve WebRTC WHEP to viewers |
| minio | Pod | 9000 S3, 9001 console | Durable recording archive |
| postgres | Pod | 5432 | Recording metadata index |
| api-server | Pod | 8080 | Go API: stream list (SSE), recordings query, MinIO presign |
| web-frontend | Pod | 80 | nginx serves React SPA |
| prometheus | Pod | 9090 | Metrics collection |
| grafana | Pod | 3000 | Dashboards |

---

## 5. Publisher Routing (Stream Registry)

No separate registry service. Consistent hash in Go publisher:

```go
func originIndex(streamID string, numOrigins int) int {
    h := fnv.New32a()
    h.Write([]byte(streamID))
    return int(h.Sum32()) % numOrigins
}

target := fmt.Sprintf("rtsp://mediamtx-origin-%d:8554/%s", originIndex(streamID, 3), streamID)
```

k3s: headless Service `mediamtx-origin-headless` exposes stable DNS per pod.  
Docker Compose: container names `mediamtx-origin-0`, `mediamtx-origin-1`, `mediamtx-origin-2`.

---

## 6. MediaMTX Configuration

### Origin Config
```yaml
api: yes
apiAddress: :9997
metrics: yes
metricsAddress: :9998
rtsp: yes
rtspAddress: :8554
webrtc: no

authMethod: internal

paths:
  "~^(.+)$":
    publishUser: publisher
    publishPass: ${PUBLISH_SECRET}
    readUser: internal
    readPass: ${INTERNAL_SECRET}
    record: yes
    recordPath: /recordings/%path/%Y-%m-%d_%H-%M-%S
    recordFormat: fmp4
    recordPartDuration: 1s
    recordSegmentDuration: 60s
    recordDeleteAfter: 2h
```

### Read Replica Config
```yaml
api: yes
apiAddress: :9997
metrics: yes
metricsAddress: :9998
webrtc: yes
webrtcAddress: :8889
webrtcLocalUDPAddress: :8189
rtsp: no

authMethod: internal

paths:
  "~^(.+)$":
    readUser: viewer
    readPass: ${VIEWER_SECRET}
    source: rtsp://internal:${INTERNAL_SECRET}@mediamtx-origin-headless:8554/$G1
    sourceOnDemand: no
```

**Codec constraint:** H264 baseline, no B-frames + Opus audio. No H265 (browser incompatible).

---

## 7. Recording Pipeline

```
mediamtx-origin writes:
  /recordings/{stream_id}/2026-06-02_10-30-00.mp4  (fMP4, 60s segments)

recording-uploader sidecar (Go, fsnotify):
  1. watches /recordings/** for file close events
  2. on segment complete:
     a. upload → MinIO bucket: recordings/{stream_id}/{filename}
     b. INSERT INTO recordings (stream_id, start_time, end_time, minio_path, duration_s)
     c. DELETE local file
  3. on startup: scan for orphaned files (crash recovery)
```

### Postgres Schema
```sql
CREATE TABLE recordings (
  id          BIGSERIAL PRIMARY KEY,
  stream_id   TEXT NOT NULL,
  start_time  TIMESTAMPTZ NOT NULL,
  end_time    TIMESTAMPTZ NOT NULL,
  minio_path  TEXT NOT NULL,
  duration_s  INTEGER NOT NULL,
  created_at  TIMESTAMPTZ DEFAULT now()
);
CREATE INDEX ON recordings (stream_id, start_time);
```

---

## 8. Authentication

MediaMTX static credentials in YAML config. No auth backend.

| Role | Credential | Scope |
|------|-----------|-------|
| publisher | PUBLISH_SECRET | Write streams to origin |
| internal | INTERNAL_SECRET | Read replicas pull from origin |
| viewer | VIEWER_SECRET | Browser viewers via read replicas |

Credentials injected as env vars into pods/containers. Admin API (:9997) never exposed externally.

**Upgrade path:** swap `authMethod: internal` → `authMethod: jwt` + add Keycloak pod when needed.

---

## 9. API Server (Go)

Endpoints:

| Method | Path | Description |
|--------|------|-------------|
| GET | /api/streams/live | SSE — stream list diffs, polls MediaMTX origin :9997/v3/paths/list every 3s |
| GET | /api/recordings | Query params: stream_id, from, to → returns segments with presigned MinIO URLs |
| GET | /health | Liveness check |

Polls all origin pods, merges results, pushes SSE events to connected browsers on changes.  
Origin count injected via env var `ORIGIN_COUNT` (default 3). api-server constructs URLs: `mediamtx-origin-{0..N-1}:9997`.

---

## 10. Web Frontend (React + Vite)

**Live Viewer:**
- Sidebar: SSE-fed stream list
- Main: `<video>` element + MediaMTX `reader.js` (`MediaMTXWebRTCReader`)
- WHEP URL: `http://{MEDIAMTX_READ_HOST}:8889/{stream_id}/whep` — host injected at build time via env var `VITE_MEDIAMTX_READ_HOST` (e.g. `localhost` on Docker Compose, node IP on k3s)
- Viewer credentials passed via token field
- Auto-reconnect on disconnect (reader.js built-in)

**Recording Browser:**
- Stream picker + date range filter
- Calls `GET /api/recordings` → list of segments
- Sequential fMP4 playback via presigned MinIO URLs

**Stack:** React, Vite, Tailwind CSS, reader.js, served by nginx.

---

## 11. WebRTC Networking

```
Happy path:   browser → HTTPS WHEP :8889 (signaling) + UDP :8189 (media)
Fallback:     browser → HTTPS WHEP :8889 (signaling) + TCP :8189 (media)
```

No STUN/TURN required for LAN/air-gapped deployment. Add if external access needed.

---

## 12. Observability

Prometheus scrapes:
- `mediamtx-origin-{0,1,2}:9998/metrics` — streams, bitrate, recording lag
- `mediamtx-read:9998/metrics` — viewer sessions, WebRTC connections, outbound bitrate

Grafana dashboards:
- Active streams per origin
- Viewer count (WebRTC sessions)
- Inbound/outbound bitrate
- Recording pipeline health (upload lag, segment size, MinIO usage)
- Per-pod CPU/memory

---

## 13. Infrastructure

### Dev Workflow (3 tiers)

| Tier | Tool | Purpose |
|------|------|---------|
| 1 | Docker Compose (Windows) | Rapid iteration, config testing |
| 2 | Rancher Desktop k3s (Windows) | k3s manifest validation |
| 3 | k3s bare metal (Ubuntu VM, air-gapped) | Production |

All tiers use identical container images and config files.

### Air-Gap Image Transfer
```bash
# On Windows: export all images
docker save image1 image2 ... | gzip > superstreaming-images.tar.gz

# Transfer to Ubuntu VM (USB/local network)

# On Ubuntu VM: import into k3s
k3s ctr images import superstreaming-images.tar.gz
```

### Docker Compose Port Exposure (Windows)
```
localhost:8554   RTSP publish
localhost:8889   WebRTC WHEP (browser)
localhost:8189   WebRTC UDP media
localhost:80     React frontend
localhost:8080   api-server
localhost:9000   MinIO S3
localhost:9001   MinIO console
localhost:9090   Prometheus
localhost:3000   Grafana
```

### k3s (Ubuntu VM)
- StatefulSet for origin pods (stable identity + PVC per pod)
- Deployment + HPA for read replicas (scale on active WebRTC connections)
- PersistentVolumeClaim per origin pod (recordings hot buffer, 500Gi)
- PersistentVolumeClaim for MinIO (durable archive, sized to retention policy)
- NodePort or Ingress for external port exposure
- PodDisruptionBudget on read tier

---

## 14. Sub-Projects (Implementation Order)

Each sub-project gets own implementation plan:

1. **Infrastructure** — Docker Compose + k3s manifests + Rancher Desktop setup
2. **MediaMTX cluster** — origin StatefulSet + read Deployment + configs + secrets
3. **Recording pipeline** — recording-uploader Go service + MinIO + Postgres schema
4. **API server** — Go service, SSE, recordings endpoint, MinIO presign
5. **Web frontend** — React + Vite + reader.js + Tailwind
6. **Observability** — Prometheus + Grafana + dashboards
7. **Air-gap bundle** — image export scripts, transfer procedure, import validation

---

## 15. Non-Goals (for now)

- External STUN/TURN (LAN only)
- Multi-user auth with web UI (upgrade to Keycloak later)
- CDN or external egress
- H265 support
- Multi-site replication
