# SuperStreaming — Sub-Project 4: API Server
**Date:** 2026-06-05  
**Status:** Approved  
**Parent spec:** `docs/superpowers/specs/2026-06-02-superstreaming-design.md` (section 9)

---

## 1. Scope

Build the `api-server` Go service that exposes the HTTP API consumed by the web frontend. It aggregates live stream state from all MediaMTX origin pods and serves recording metadata with presigned MinIO playback URLs.

The nginx stub deployed in Sub-Project 1 is replaced by this real service. No new infrastructure is required.

---

## 2. Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/streams/live` | SSE — stream list diffs |
| `GET` | `/api/recordings` | Recording segments query |
| `GET` | `/health` | Liveness check |

---

## 3. File Layout

```
services/
└── api-server/
    ├── go.mod                  (module: superstreaming/api-server)
    ├── main.go                 (entry point, HTTP server, graceful shutdown)
    ├── handlers/
    │   ├── streams.go          (SSE handler, poller)
    │   ├── recordings.go       (recordings query handler)
    │   └── health.go           (health check)
    ├── mediamtx/
    │   └── client.go           (HTTP client for MediaMTX /v3/paths/list)
    ├── minio/
    │   └── presign.go          (presigned URL generation)
    └── db/
        └── db.go               (Postgres connection pool, recordings query)

build/
└── api-server/
    └── Dockerfile
```

---

## 4. Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `LISTEN_ADDR` | no | `:8080` | HTTP listen address |
| `ORIGIN_COUNT` | no | `3` | Number of origin pods to poll |
| `ORIGIN_HOST_TEMPLATE` | no | `mediamtx-origin-%d` | Printf template for origin hostnames |
| `ORIGIN_API_PORT` | no | `9997` | MediaMTX API port on origin pods |
| `ORIGIN_POLL_INTERVAL` | no | `3s` | How often to poll all origins |
| `MINIO_ENDPOINT` | yes | — | e.g. `minio:9000` |
| `MINIO_ROOT_USER` | yes | — | MinIO root user (matches secrets template key) |
| `MINIO_ROOT_PASSWORD` | yes | — | MinIO root password (matches secrets template key) |
| `MINIO_BUCKET` | yes | — | e.g. `recordings` |
| `MINIO_USE_SSL` | no | `false` | TLS for MinIO |
| `MINIO_PRESIGN_EXPIRY` | no | `24h` | Presigned URL validity |
| `POSTGRES_HOST` | no | `postgres` | Postgres hostname |
| `POSTGRES_PORT` | no | `5432` | Postgres port |
| `POSTGRES_USER` | yes | — | Postgres user (matches secrets template key) |
| `POSTGRES_PASSWORD` | yes | — | Postgres password (matches secrets template key) |
| `POSTGRES_DB` | yes | — | Postgres database (matches secrets template key) |
| `CORS_ORIGIN` | no | `*` | Allowed CORS origin (`*` for dev, set to frontend URL in prod) |
| `LOG_LEVEL` | no | `info` | `debug`, `info`, `warn`, `error` |

---

## 5. GET /api/streams/live (SSE)

### Behaviour

- Upgrades to SSE (`Content-Type: text/event-stream`)
- Polls all origin pods every `ORIGIN_POLL_INTERVAL`
- Sends `data:` events only on **changes** (diff against previous state)
- Sends a `heartbeat` event every 15s to keep connection alive

### Polling

```
for i := 0; i < ORIGIN_COUNT; i++:
  GET http://mediamtx-origin-{i}:{ORIGIN_API_PORT}/v3/paths/list
  if unreachable: log warning, skip (partial results are valid)
merge all items, deduplicate by path name
diff against previous snapshot
```

### Event Format

```
event: streams
data: {"active":["cam-01","cam-02"],"added":["cam-02"],"removed":[]}

event: heartbeat
data: {}
```

`active`: full current list. `added`/`removed`: delta since last event. Frontend can use either.

### Client Reconnect

SSE spec handles reconnect automatically. No `Last-Event-ID` tracking needed (client requests full state on reconnect via next `streams` event within 3s).

---

## 6. GET /api/recordings

### Query Parameters

| Param | Required | Format | Description |
|-------|----------|--------|-------------|
| `stream_id` | no | string | Filter by stream. Omit = all streams |
| `from` | no | RFC3339 | Start of time range |
| `to` | no | RFC3339 | End of time range |
| `limit` | no | int | Max rows returned, default 100, max 500 |
| `offset` | no | int | Pagination offset, default 0 |

### Response

```json
{
  "segments": [
    {
      "id": 42,
      "stream_id": "cam-01",
      "start_time": "2026-06-05T10:30:00Z",
      "end_time": "2026-06-05T10:31:00Z",
      "duration_s": 60,
      "url": "https://minio:9000/recordings/cam-01/...?X-Amz-Signature=..."
    }
  ],
  "total": 142,
  "limit": 100,
  "offset": 0
}
```

`url` is a presigned MinIO GET URL, valid for `MINIO_PRESIGN_EXPIRY`. Generated fresh on each request (no caching — avoids serving expired URLs).

### SQL Query

```sql
SELECT id, stream_id, start_time, end_time, duration_s, minio_path
FROM recordings
WHERE ($1::text IS NULL OR stream_id = $1)
  AND ($2::timestamptz IS NULL OR start_time >= $2)
  AND ($3::timestamptz IS NULL OR end_time <= $3)
ORDER BY start_time DESC
LIMIT $4 OFFSET $5
```

`COUNT(*)` with same filters for `total` field (separate query).

---

## 7. GET /health

```json
{"status": "ok"}
```

Returns 200 always (liveness only, not readiness). Used by Docker Compose healthcheck and k3s liveness probe.

---

## 8. CORS

All responses include:
```
Access-Control-Allow-Origin: {CORS_ORIGIN}
Access-Control-Allow-Methods: GET, OPTIONS
Access-Control-Allow-Headers: Content-Type
```

OPTIONS preflight returns 204.

---

## 9. Docker Image

```dockerfile
# build/api-server/Dockerfile
FROM golang:1.23-alpine AS builder
WORKDIR /src
COPY services/api-server/ .
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -o /api-server .

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
COPY --from=builder /api-server /api-server
ENTRYPOINT ["/api-server"]
```

Image tag: `superstreaming/api-server:latest`

---

## 10. Docker Compose Wiring

Replace the nginx stub in `docker-compose.yml`:

```yaml
api-server:
  image: superstreaming/api-server:latest
  build:
    context: .
    dockerfile: build/api-server/Dockerfile
  container_name: api-server
  restart: unless-stopped
  networks:
    - superstreaming
  ports:
    - "8080:8080"
  environment:
    - LISTEN_ADDR=:8080
    - ORIGIN_COUNT=1
    - ORIGIN_HOST_TEMPLATE=mediamtx-origin-%d
    - ORIGIN_API_PORT=9997
    - MINIO_ENDPOINT=minio:9000
    - MINIO_ROOT_USER=${MINIO_ROOT_USER}
    - MINIO_ROOT_PASSWORD=${MINIO_ROOT_PASSWORD}
    - MINIO_BUCKET=${MINIO_BUCKET}
    - POSTGRES_HOST=postgres
    - POSTGRES_USER=${POSTGRES_USER}
    - POSTGRES_PASSWORD=${POSTGRES_PASSWORD}
    - POSTGRES_DB=${POSTGRES_DB}
    - CORS_ORIGIN=*
  depends_on:
    postgres:
      condition: service_healthy
    minio:
      condition: service_healthy
  healthcheck:
    test: ["CMD", "wget", "-qO-", "http://localhost:8080/health"]
    interval: 10s
    timeout: 5s
    retries: 5
```

---

## 11. k3s Wiring

Replace stub Deployment in `k8s/stubs/api-server-stub.yaml` with a real Deployment at `k8s/api-server/deployment.yaml`:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: api-server
  namespace: superstreaming
spec:
  replicas: 1
  selector:
    matchLabels:
      app: api-server
  template:
    metadata:
      labels:
        app: api-server
    spec:
      containers:
        - name: api-server
          image: superstreaming/api-server:latest
          ports:
            - containerPort: 8080
          env:
            - name: LISTEN_ADDR
              value: ":8080"
            - name: ORIGIN_COUNT
              value: "3"
            - name: ORIGIN_HOST_TEMPLATE
              value: "mediamtx-origin-%d.mediamtx-origin-headless.superstreaming.svc.cluster.local"
            - name: ORIGIN_API_PORT
              value: "9997"
            - name: MINIO_ENDPOINT
              value: "minio.superstreaming.svc.cluster.local:9000"
            - name: POSTGRES_HOST
              value: "postgres.superstreaming.svc.cluster.local"
            - name: CORS_ORIGIN
              value: "*"
          envFrom:
            - secretRef:
                name: superstreaming-secrets
          readinessProbe:
            httpGet:
              path: /health
              port: 8080
            initialDelaySeconds: 5
            periodSeconds: 10
          resources:
            requests:
              cpu: "100m"
              memory: "64Mi"
            limits:
              cpu: "500m"
              memory: "256Mi"
```

Service at `k8s/api-server/service.yaml` (ClusterIP, port 8080). The web-frontend pod calls `api-server:8080` within the cluster — no NodePort needed for internal traffic.

---

## 12. Go Dependencies

```
github.com/minio/minio-go/v7       v7.0+   presigned URL generation
github.com/lib/pq                   v1.10+  Postgres driver
github.com/rs/zerolog               v1.33+  structured logging
```

No web framework — standard `net/http` is sufficient for three endpoints.

---

## 13. Smoke Tests

`scripts/test-api-server.ps1`:
1. Assert `GET /health` returns 200 `{"status":"ok"}`
2. Assert `GET /api/streams/live` returns `Content-Type: text/event-stream` and first event within 5s
3. Assert `GET /api/recordings` returns 200 with `segments` array (may be empty)
4. Assert `GET /api/recordings?stream_id=nonexistent` returns 200 with empty `segments`

---

## 14. Non-Goals

- Authentication on the API (viewers access via frontend, which passes VIEWER_SECRET to WHEP — API itself is internal-only)
- Recording upload or delete via API
- WebSocket (SSE is sufficient for one-way stream list updates)
- Pagination cursors (offset-based is fine at this scale)
