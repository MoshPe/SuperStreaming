# SuperStreaming — Sub-Project 3: Recording Pipeline
**Date:** 2026-06-05  
**Status:** Approved  
**Parent spec:** `docs/superpowers/specs/2026-06-02-superstreaming-design.md` (sections 7, 13)

---

## 1. Scope

Build the `recording-uploader` Go service that runs as a sidecar next to every `mediamtx-origin` pod. It watches `/recordings`, uploads completed fMP4 segments to MinIO, indexes them in Postgres, then deletes the local copy.

Infrastructure (MinIO pod, Postgres pod, PVCs, Docker Compose skeleton, k3s manifests) already exists from Sub-Project 1. This sub-project adds only the Go service, its Docker image, and the wiring into Docker Compose and the origin StatefulSet.

---

## 2. Service Responsibilities

1. **Watch** `/recordings/**/*.mp4` for file-close events via `fsnotify`
2. **Upload** completed segment to MinIO: `recordings/{stream_id}/{filename}`
3. **Insert** row into Postgres `recordings` table
4. **Delete** local file after confirmed upload + insert
5. **Recover** orphaned files on startup (files whose `mtime` is older than 90s that have not been uploaded)

Not responsible for: bucket creation (handled by `minio-init`), schema migration (handled by `sql/init.sql`), any MediaMTX configuration.

---

## 3. File Layout

```
services/
└── recording-uploader/
    ├── go.mod                  (module: superstreaming/recording-uploader)
    ├── main.go                 (entry point: env parsing, watcher start, shutdown)
    ├── watcher/
    │   └── watcher.go          (fsnotify loop, debounce, emit segment events)
    ├── uploader/
    │   └── uploader.go         (MinIO upload, Postgres insert, local delete)
    └── db/
        └── db.go               (Postgres connection pool, INSERT helper)

build/
└── recording-uploader/
    └── Dockerfile
```

---

## 4. Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `RECORDINGS_DIR` | no | `/recordings` | Root directory to watch |
| `MINIO_ENDPOINT` | yes | — | e.g. `minio:9000` |
| `MINIO_ROOT_USER` | yes | — | MinIO root user (matches secrets template key) |
| `MINIO_ROOT_PASSWORD` | yes | — | MinIO root password (matches secrets template key) |
| `MINIO_BUCKET` | yes | — | e.g. `recordings` |
| `MINIO_USE_SSL` | no | `false` | TLS for MinIO |
| `POSTGRES_HOST` | no | `postgres` | Postgres hostname |
| `POSTGRES_PORT` | no | `5432` | Postgres port |
| `POSTGRES_USER` | yes | — | Postgres user (matches secrets template key) |
| `POSTGRES_PASSWORD` | yes | — | Postgres password (matches secrets template key) |
| `POSTGRES_DB` | yes | — | Postgres database (matches secrets template key) |
| `ORPHAN_AGE_SECONDS` | no | `90` | Files older than this on startup → treated as orphaned |
| `LOG_LEVEL` | no | `info` | `debug`, `info`, `warn`, `error` |

---

## 5. Segment Event Flow

```
fsnotify WRITE/RENAME/CREATE on *.mp4 file
  → debounce 500ms (MediaMTX may write in bursts)
  → file size stable? (stat twice, 200ms apart)
  → segment complete → emit to upload pipeline

upload pipeline (goroutine per segment):
  1. Upload to MinIO: recordings/{stream_id}/{filename}
  2. Parse start_time from filename: YYYY-MM-DD_HH-MM-SS.mp4
  3. Derive end_time: start_time + duration (from MP4 header via go-mp4 or fixed 60s)
  4. INSERT INTO recordings (stream_id, start_time, end_time, minio_path, duration_s)
  5. DELETE local file
  6. On any step failure: exponential backoff, max 5 retries, then log + skip
```

**Filename → stream_id mapping:** MediaMTX recordPath is `/recordings/%path/%Y-%m-%d_%H-%M-%S`, so file path is `/recordings/{stream_id}/2026-06-05_10-30-00.mp4`. stream_id = parent directory name.

**Duration:** Read from MP4 `mvhd` box via lightweight parse (no full decode). If parse fails, fall back to `recordSegmentDuration` (60s) from env var `SEGMENT_DURATION_S`, default `60`.

---

## 6. Startup Orphan Recovery

On startup, before starting the watcher:

```
scan RECORDINGS_DIR/**/*.mp4
for each file:
  if mtime < now - ORPHAN_AGE_SECONDS:
    emit to upload pipeline (same path as live segments)
```

This recovers segments that completed while the uploader was down (pod crash, restart).

---

## 7. Graceful Shutdown

On `SIGTERM`/`SIGINT`:
1. Stop fsnotify watcher (no new events)
2. Drain upload pipeline (finish in-flight uploads, max 60s)
3. Exit 0

MediaMTX pod `terminationGracePeriodSeconds: 60` — uploader gets the same window.

---

## 8. Docker Image

```dockerfile
# build/recording-uploader/Dockerfile
FROM golang:1.23-alpine AS builder
WORKDIR /src
COPY services/recording-uploader/ .
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -o /recording-uploader .

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
COPY --from=builder /recording-uploader /recording-uploader
ENTRYPOINT ["/recording-uploader"]
```

Image tag: `superstreaming/recording-uploader:latest`

---

## 9. Docker Compose Wiring

Add to `docker-compose.yml` as a separate service sharing the `origin-recordings` volume:

```yaml
recording-uploader:
  image: superstreaming/recording-uploader:latest
  build:
    context: .
    dockerfile: build/recording-uploader/Dockerfile
  container_name: recording-uploader
  restart: unless-stopped
  networks:
    - superstreaming
  volumes:
    - origin-recordings:/recordings
  environment:
    - RECORDINGS_DIR=/recordings
    - MINIO_ENDPOINT=minio:9000
    - MINIO_ROOT_USER=${MINIO_ROOT_USER}
    - MINIO_ROOT_PASSWORD=${MINIO_ROOT_PASSWORD}
    - MINIO_BUCKET=${MINIO_BUCKET}
    - POSTGRES_HOST=postgres
    - POSTGRES_USER=${POSTGRES_USER}
    - POSTGRES_PASSWORD=${POSTGRES_PASSWORD}
    - POSTGRES_DB=${POSTGRES_DB}
  depends_on:
    minio:
      condition: service_healthy
    postgres:
      condition: service_healthy
```

Volume has no `:ro` flag — the uploader needs write access to delete files after upload. MediaMTX and the uploader share the volume cooperatively (uploader only deletes files MediaMTX has finished writing).

---

## 10. k3s Wiring

Add sidecar container to `k8s/origin/statefulset.yaml` containers list:

```yaml
- name: recording-uploader
  image: superstreaming/recording-uploader:latest
  env:
    - name: RECORDINGS_DIR
      value: /recordings
    - name: MINIO_ENDPOINT
      value: minio.superstreaming.svc.cluster.local:9000
    - name: POSTGRES_HOST
      value: postgres.superstreaming.svc.cluster.local
  envFrom:
    - secretRef:
        name: superstreaming-secrets
  volumeMounts:
    - name: recordings
      mountPath: /recordings
  resources:
    requests:
      cpu: "100m"
      memory: "64Mi"
    limits:
      cpu: "500m"
      memory: "256Mi"
```

The `recordings` PVC is already declared in the StatefulSet's `volumeClaimTemplates` — the sidecar mounts the same volume.

---

## 11. Go Dependencies

```
github.com/fsnotify/fsnotify       v1.7+   file watching
github.com/minio/minio-go/v7       v7.0+   MinIO S3 client
github.com/lib/pq                   v1.10+  Postgres driver
github.com/rs/zerolog               v1.33+  structured logging
```

No external MP4 library needed initially — parse `duration_s` from filename timestamp + `SEGMENT_DURATION_S` fallback. Add MP4 header parsing only if accurate duration becomes a requirement.

---

## 12. Smoke Test

`scripts/test-recording-pipeline.ps1`:
1. Start Docker Compose stack
2. Publish ffmpeg RTSP test stream for 65s (one full segment)
3. Wait 90s
4. Assert: MinIO bucket contains at least one object under `recordings/test-stream/`
5. Assert: Postgres `recordings` table has at least one row for `stream_id = 'test-stream'`
6. Assert: `/recordings/test-stream/` directory inside origin container is empty (file deleted)

---

## 13. Non-Goals

- Multi-origin coordination (each origin pod has its own uploader sidecar — no cross-pod logic)
- Segment re-upload on demand
- MinIO bucket lifecycle policy (set separately in MinIO console)
- Recording playback (handled by api-server + web-frontend)
