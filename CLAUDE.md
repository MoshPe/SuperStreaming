# SuperStreaming Project Memory

Last refreshed: 2026-06-05.

## Purpose

SuperStreaming is a large-scale live streaming platform for always-on security/surveillance cameras and live event broadcasts. It is designed to run locally with Docker Compose on Windows and in production on air-gapped k3s/Linux using the same images and equivalent configs.

## Architecture

- Media plane is MediaMTX.
- Publisher pushes RTSP streams to origin pods.
- Read replicas pull from origins and serve browser viewers over WebRTC WHEP.
- Origins record fMP4 segments to a local `/recordings` volume.
- `recording-uploader` watches completed segments, uploads them to MinIO, inserts metadata into Postgres, then removes the local segment.
- `api-server` exposes live stream state and recording lookup APIs for the frontend.
- `web-frontend` is a Vite/React/Tailwind operations UI for live viewing and recording playback.
- Prometheus scrapes MediaMTX metrics; Grafana is provisioned with dashboards.

Target design from project docs:

- 20-200 concurrent streams.
- 50-500 concurrent viewers.
- 2-3 origin pods in production.
- 3-6 read replica pods, HPA based on connections.
- k3s-first production, Docker Compose full-system test on Windows.

Current Docker Compose shape:

- `mediamtx-origin-0`: RTSP origin and recorder.
- `mediamtx-read`: WHEP/WebRTC read replica.
- `publisher`: Go/ffmpeg stream publisher.
- `recording-uploader`: Go uploader/watcher.
- `minio` and `minio-init`.
- `postgres`.
- `api-server`.
- `web-frontend`.
- `prometheus`.
- `grafana`.

## Important Files

- `docker-compose.yml`: local full-stack deployment.
- `configs/mediamtx/origin.yml`: origin RTSP/API/metrics/recording config.
- `configs/mediamtx/read.yml`: read replica WHEP config.
- `configs/prometheus/prometheus.yml`: local Prometheus scrape config.
- `sql/init.sql`: `recordings` table and index.
- `docs/deployment-manual.md`: local Docker Compose, Docker Desktop Kubernetes, and air-gapped k3s deployment manual.
- `docs/superpowers/specs/*`: design specs.
- `docs/superpowers/plans/*`: implementation plans.
- `k8s/`: Kubernetes manifests for namespace, configmaps, secrets examples, origin/read, storage, API, frontend, monitoring.
- `build/*/Dockerfile`: service images.
- `scripts/*.ps1`: local/k8s smoke and pipeline tests.

Do not copy secrets from `.env`, `Credentials.txt`, or `k8s/secrets/secrets.yaml` into summaries or commits.

## MediaMTX Config

Origin config:

- API enabled on `:9997`.
- Metrics enabled on `:9998`.
- RTSP enabled on `:8554`.
- WebRTC disabled.
- Internal auth users:
  - `publisher` can publish using `PUBLISH_SECRET`.
  - `internal` can read/playback using `INTERNAL_SECRET`.
  - `any` can call API.
- All paths record to `/recordings/%path/%Y-%m-%d_%H-%M-%S`.
- Recording format is fMP4 with 1s parts, 60s segments, 2h local retention.

Read replica config:

- API enabled on `:9997`.
- Metrics enabled on `:9998`.
- WebRTC enabled on `:8889`.
- UDP and TCP WebRTC media both use `:8189`.
- `WEBRTC_EXTERNAL_HOST` is injected into `webrtcAdditionalHosts`.
- Viewer auth uses `viewer` plus `VIEWER_SECRET`.
- Regex path pulls from `rtsp://internal:$INTERNAL_SECRET@mediamtx-origin-0:8554/$G1`.
- Current read config is single-origin Compose-oriented; production design mentions headless-service/multi-origin routing.

## Go Services

### `services/api-server`

Module: `superstreaming/api-server`, Go 1.25.0.

Main dependencies:

- `github.com/lib/pq`
- `github.com/minio/minio-go/v7`
- `github.com/rs/zerolog`

Runtime config:

- `LISTEN_ADDR`, default `:8080`.
- `ORIGIN_COUNT`, default `3`.
- `ORIGIN_HOST_TEMPLATE`, default `mediamtx-origin-%d`.
- `ORIGIN_API_PORT`, default `9997`.
- `ORIGIN_POLL_INTERVAL`, default `3s`.
- MinIO endpoint/user/password/bucket/public endpoint/use SSL/presign expiry.
- Postgres host/port/user/password/db.
- `CORS_ORIGIN`, default `*`.

Routes:

- `GET /health`: health handler.
- `GET /api/streams/live`: SSE stream of active paths polled from all configured origin APIs.
- `GET /api/recordings`: JSON recording segment search with presigned MinIO URLs.

Implementation notes:

- `handlers/streams.go` polls `/v3/paths/list` through `mediamtx.ListPaths`, merges ready paths, emits `streams` events only on initial/diff changes, and emits heartbeat events every 15s.
- `handlers/recordings.go` accepts `stream_id`, RFC3339 `from`, RFC3339 `to`, `limit`, and `offset`.
- DB query defaults `limit` to 100 and caps it at 500.
- MinIO presigner uses explicit `us-east-1` region to avoid online bucket-location lookup.

Tests present:

- DB query tests.
- recordings handler tests.
- streams handler tests.
- MediaMTX client tests.

### `services/publisher`

Module: `superstreaming/publisher`, Go 1.24.4.

Purpose:

- Parses `STREAMS` as comma-separated `stream_id=source` pairs.
- Routes each stream with `fnv32a(stream_id) % ORIGIN_COUNT`.
- Builds target URLs as `rtsp://publisher:<PUBLISH_SECRET>@<origin-host>:<port>/<stream_id>`.
- Starts one ffmpeg relay goroutine per stream.

Relay behavior:

- `source == "test"` generates synthetic video/audio with ffmpeg lavfi, H264 baseline, yuv420p, Opus.
- Other sources are treated as RTSP URLs and copied with `-rtsp_transport tcp`.
- ffmpeg is restarted after exit every 3s until context cancellation.

Tests present:

- Hash routing tests.
- ffmpeg argument construction tests.

### `services/recording-uploader`

Module: `superstreaming/recording-uploader`, Go 1.24.4.

Main dependencies:

- `github.com/fsnotify/fsnotify`
- `github.com/lib/pq`
- `github.com/minio/minio-go/v7`
- `github.com/rs/zerolog`

Runtime config:

- `RECORDINGS_DIR`, default `/recordings`.
- MinIO endpoint/user/password/bucket/use SSL.
- Postgres host/port/user/password/db.
- `ORPHAN_AGE_SECONDS`, default `90`.

Behavior:

- Connects to Postgres and MinIO.
- Scans orphaned `.mp4` files older than configured age at startup.
- Watches the recordings tree recursively with fsnotify.
- Adds watches for new stream subdirectories.
- Debounces `.mp4` create/write events for 2s to infer segment completion.
- Uploads object as `recordings/{stream_id}/{filename}`.
- Inserts row into `recordings`.
- Deletes local file only after successful upload and DB insert.

Path convention:

- Expected MediaMTX file shape: `{recordings_dir}/{stream_id}/YYYY-MM-DD_HH-MM-SS.mp4`.
- Start time is parsed from filename.
- End time is file mtime.
- Duration has a minimum of 1 second.

Tests present:

- Uploader tests.
- Watcher tests.

## Database

`sql/init.sql` creates:

- `recordings`
  - `id BIGSERIAL PRIMARY KEY`
  - `stream_id TEXT NOT NULL`
  - `start_time TIMESTAMPTZ NOT NULL`
  - `end_time TIMESTAMPTZ NOT NULL`
  - `minio_path TEXT NOT NULL`
  - `duration_s INTEGER NOT NULL`
  - `created_at TIMESTAMPTZ DEFAULT now()`
- Index: `recordings_stream_time_idx` on `(stream_id, start_time)`.

## Frontend

Location: `services/web-frontend`.

Stack:

- Vite 6.
- React 19.
- Tailwind CSS 3.
- `lucide-react` icons.

Scripts:

- `npm run dev`
- `npm run build`
- `npm run preview`

Runtime config:

- `window.__ENV__` from `public/env-config.js` or Vite env fallback.
- `MEDIAMTX_WHEP_BASE`, default `http://localhost:8889/`.
- `API_SERVER_URL`, default `http://localhost:8080`.
- `VIEWER_PASSWORD`, optional.

UI:

- `App.jsx`: top nav with `Live` and `Recordings` tabs.
- `LiveViewer.jsx`: sidebar stream list plus full video pane.
- `StreamList.jsx`: active stream list and connection indicator.
- `VideoPlayer.jsx`: dynamically loads MediaMTX `webrtc/js/reader.js`, creates `MediaMTXWebRTCReader`, optionally uses viewer credentials, sets `video.srcObject`.
- `RecordingBrowser.jsx`: stream/time filters, results table, side playback panel.
- `useSSE.js`: connects to `/api/streams/live`, handles `streams` and `heartbeat` events, reconnects after errors.
- `useRecordings.js`: calls `/api/recordings` with filters and pagination args.

Visual style:

- Dark operational interface.
- Black video workspace, slate surfaces, rose accent.

## Kubernetes

Manifests exist for:

- Namespace.
- ConfigMaps for MediaMTX, Prometheus, Grafana, Postgres init.
- Origin StatefulSet and services.
- Read Deployment, HPA, PDB, service.
- MinIO Deployment/PVC/service.
- Postgres Deployment/PVC/service.
- API server Deployment/service.
- Web frontend Deployment/service.
- Publisher Deployment.
- Prometheus and Grafana.
- Secret example and local secret file.
- Older stub manifests still present under `k8s/stubs`.

Deployment manual notes:

- Docker Desktop Kubernetes validation uses port-forwarding because NodePorts may not map to Windows localhost.
- k3s production is air-gapped: export images on Windows, transfer tarballs, import via `k3s ctr images import`, then `kubectl apply`.

## Testing and Verification Scripts

PowerShell scripts:

- `scripts/test-all-services.ps1`: local Compose HTTP/health checks plus Postgres readiness.
- `scripts/test-api-server.ps1`: API server checks.
- `scripts/test-k8s-services.ps1`: Kubernetes health checks with port-forwarding.
- `scripts/test-origin-api.ps1`: origin API check.
- `scripts/test-read-api.ps1`: read API check.
- `scripts/test-recording-pipeline.ps1`: verifies MinIO object, Postgres row, and local file deletion after recording.
- `scripts/test-storage.ps1`: storage checks.
- `scripts/test-streaming-path.ps1`: stream path/WHEP check.
- `scripts/whep-render-test.mjs`: untracked browser-side WHEP render test script.

Go tests are organized per service; run them from each service directory with `go test ./...`.

Frontend build check is `npm run build` from `services/web-frontend`.

## Current Worktree Notes

As of this refresh, the worktree was already dirty before this memory update:

- Modified:
  - `configs/prometheus/prometheus.yml`
  - `docker-compose.yml`
  - `services/recording-uploader/main.go`
  - `services/recording-uploader/uploader/uploader.go`
  - `services/recording-uploader/uploader/uploader_test.go`
  - `services/recording-uploader/watcher/watcher.go`
  - `services/web-frontend/src/components/VideoPlayer.jsx`
- Untracked:
  - `Credentials.txt`
  - `scripts/whep-render-test.mjs`

This file (`CLAUDE.md`) was refreshed by Codex to preserve current project context. Treat unrelated dirty files as user work unless explicitly told otherwise.

## Known Cautions

- Secrets are present locally; do not expose them.
- Current Compose config uses one origin (`ORIGIN_COUNT=1`), while target production architecture supports multiple origins.
- `configs/mediamtx/read.yml` currently pulls from `mediamtx-origin-0`; multi-origin/headless-service behavior may need reconciliation before scaling.
- `scripts/test-all-services.ps1` still labels API and frontend checks as "stub" even though real service manifests/code exist.
- `docs/deployment-manual.md` has at least one troubleshooting command that warns origin API should be accessed by port-forward, not RTSP NodePort.
