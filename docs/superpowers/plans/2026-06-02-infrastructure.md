# SuperStreaming: Sub-Project 1 — Infrastructure Skeleton

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Stand up all service skeletons — MediaMTX origin + read tier + MinIO + Postgres + Prometheus + Grafana + stub services — runnable on Windows via Docker Compose and on Rancher Desktop via k3s manifests, with end-to-end publish→WebRTC path verified.

**Architecture:** Single origin pod for this plan (sharding deferred to later). Publisher connects via RTSP to `mediamtx-origin-0:8554`. Read replicas pull from `mediamtx-origin-0` and serve WebRTC WHEP. `api-server` and `web-frontend` are nginx stubs (replaced in Sub-Projects 4 and 5). All services share identical configs across Docker Compose and k3s.

**Tech Stack:** Docker Compose v2, k3s via Rancher Desktop, `bluenviron/mediamtx:latest`, `minio/minio:latest`, `postgres:16-alpine`, `prom/prometheus:latest`, `grafana/grafana:latest`, `nginx:alpine` (stubs), ffmpeg-in-Docker (test publisher).

---

## File Structure

```
SuperStreaming/
├── .gitignore
├── .env.example
├── .env                                        ← gitignored, dev secrets
├── CLAUDE.md
├── docker-compose.yml
├── configs/
│   ├── mediamtx/
│   │   ├── origin.yml                          ← origin MediaMTX config
│   │   └── read.yml                            ← read replica MediaMTX config
│   └── prometheus/
│       └── prometheus.yml                      ← scrape config
├── k8s/
│   ├── namespace.yaml
│   ├── configmaps/
│   │   ├── mediamtx-origin-config.yaml
│   │   ├── mediamtx-read-config.yaml
│   │   ├── prometheus-config.yaml
│   │   └── postgres-init.yaml
│   ├── secrets/
│   │   └── secrets.yaml.example                ← never commit real secrets
│   ├── origin/
│   │   ├── statefulset.yaml
│   │   ├── headless-service.yaml
│   │   ├── publisher-service.yaml              ← NodePort 30554 for RTSP publish
│   │   └── pvc.yaml
│   ├── read/
│   │   ├── deployment.yaml
│   │   ├── service.yaml
│   │   ├── hpa.yaml
│   │   └── pdb.yaml                            ← PodDisruptionBudget
│   ├── minio/
│   │   ├── deployment.yaml
│   │   ├── service.yaml
│   │   └── pvc.yaml
│   ├── postgres/
│   │   ├── deployment.yaml
│   │   ├── service.yaml
│   │   └── pvc.yaml
│   ├── prometheus/
│   │   ├── deployment.yaml
│   │   └── service.yaml
│   ├── grafana/
│   │   ├── deployment.yaml
│   │   └── service.yaml
│   └── stubs/
│       ├── api-server-stub.yaml
│       └── web-frontend-stub.yaml
├── sql/
│   └── init.sql                                ← recordings schema
└── docs/
    └── superpowers/
        ├── specs/
        │   └── 2026-06-02-superstreaming-design.md
        └── plans/
            └── 2026-06-02-infrastructure.md    ← this file
```

---

### Task 1: Project Init

**Files:**
- Create: `.gitignore`

- [ ] **Step 1: Init git repository**

```powershell
cd D:\Software_Projects\SuperStreaming
git init
```

Expected output: `Initialized empty Git repository in D:/Software_Projects/SuperStreaming/.git/`

- [ ] **Step 2: Create .gitignore**

Create `.gitignore`:
```gitignore
.env
*.tar.gz
*.tar
k8s/secrets/secrets.yaml
```

- [ ] **Step 3: Create directory structure**

```powershell
mkdir configs\mediamtx, configs\prometheus, k8s\configmaps, k8s\secrets, k8s\origin, k8s\read, k8s\minio, k8s\postgres, k8s\prometheus, k8s\grafana, k8s\stubs, sql, scripts
```

- [ ] **Step 4: Commit**

```powershell
git add .gitignore CLAUDE.md docs/
git commit -m "chore: init project structure"
```

---

### Task 2: Environment Configuration

**Files:**
- Create: `.env.example`
- Create: `.env`

- [ ] **Step 1: Create .env.example**

Create `.env.example`:
```env
# MediaMTX credentials
PUBLISH_SECRET=changeme_publish
INTERNAL_SECRET=changeme_internal
VIEWER_SECRET=changeme_viewer

# WebRTC external host (127.0.0.1 for Docker Compose local, node IP for k3s)
WEBRTC_EXTERNAL_HOST=127.0.0.1

# MinIO
MINIO_ROOT_USER=minioadmin
MINIO_ROOT_PASSWORD=changeme_minio
MINIO_BUCKET=recordings

# Postgres
POSTGRES_USER=superstreaming
POSTGRES_PASSWORD=changeme_postgres
POSTGRES_DB=superstreaming

# Origin count (for api-server, not used in this sub-project)
ORIGIN_COUNT=1
```

- [ ] **Step 2: Create .env for local dev**

Copy `.env.example` to `.env`, then set real dev values:
```powershell
Copy-Item .env.example .env
```

Edit `.env` — change all `changeme_*` to actual dev secrets. Use simple strings, no special shell characters:
```env
PUBLISH_SECRET=devpublish123
INTERNAL_SECRET=devinternal123
VIEWER_SECRET=devviewer123
WEBRTC_EXTERNAL_HOST=127.0.0.1
MINIO_ROOT_USER=minioadmin
MINIO_ROOT_PASSWORD=devminio123
MINIO_BUCKET=recordings
POSTGRES_USER=superstreaming
POSTGRES_PASSWORD=devpostgres123
POSTGRES_DB=superstreaming
ORIGIN_COUNT=1
```

- [ ] **Step 3: Verify .env is gitignored**

```powershell
git status
```

Expected: `.env` does NOT appear in untracked files. If it does, `.gitignore` is wrong — fix before continuing.

- [ ] **Step 4: Commit**

```powershell
git add .env.example
git commit -m "chore: add environment configuration template"
```

---

### Task 3: MediaMTX Origin Config

**Files:**
- Create: `configs/mediamtx/origin.yml`

- [ ] **Step 1: Write smoke test first**

Create `scripts/test-origin-api.ps1`:
```powershell
# Test: MediaMTX origin API returns 200
$response = Invoke-WebRequest -Uri "http://localhost:9997/v3/config/global/get" -UseBasicParsing -ErrorAction SilentlyContinue
if ($response.StatusCode -eq 200) {
    Write-Host "PASS: origin API healthy" -ForegroundColor Green
    exit 0
} else {
    Write-Host "FAIL: origin API not responding (expected after Task 5)" -ForegroundColor Yellow
    exit 1
}
```

- [ ] **Step 2: Run smoke test — confirm it fails (origin not running yet)**

```powershell
.\scripts\test-origin-api.ps1
```

Expected: `FAIL: origin API not responding (expected after Task 5)`

- [ ] **Step 3: Create origin config**

Create `configs/mediamtx/origin.yml`:
```yaml
logLevel: info
logDestinations:
  - stdout

api: yes
apiAddress: :9997

metrics: yes
metricsAddress: :9998

rtsp: yes
rtspAddress: :8554
rtspEncryption: no

webrtc: no

authMethod: internal
authInternalUsers:
  - user: publisher
    pass: ${PUBLISH_SECRET}
    permissions:
      - action: publish
        path: ""
  - user: internal
    pass: ${INTERNAL_SECRET}
    permissions:
      - action: read
        path: ""
      - action: playback
        path: ""

paths:
  all_others:
    record: yes
    recordPath: /recordings/%path/%Y-%m-%d_%H-%M-%S
    recordFormat: fmp4
    recordPartDuration: 1s
    recordSegmentDuration: 60s
    recordDeleteAfter: 2h
```

- [ ] **Step 4: Commit**

```powershell
git add configs/mediamtx/origin.yml scripts/test-origin-api.ps1
git commit -m "feat: add MediaMTX origin config"
```

---

### Task 4: MediaMTX Read Replica Config

**Files:**
- Create: `configs/mediamtx/read.yml`

- [ ] **Step 1: Write smoke test first**

Create `scripts/test-read-api.ps1`:
```powershell
# Test: MediaMTX read replica API returns 200
$response = Invoke-WebRequest -Uri "http://localhost:9998/v3/config/global/get" -UseBasicParsing -ErrorAction SilentlyContinue
if ($response.StatusCode -eq 200) {
    Write-Host "PASS: read replica API healthy" -ForegroundColor Green
    exit 0
} else {
    Write-Host "FAIL: read API not responding (expected after Task 5)" -ForegroundColor Yellow
    exit 1
}
```

Note: read replica API exposed on host port 9998 (origin uses 9997).

- [ ] **Step 2: Run smoke test — confirm it fails**

```powershell
.\scripts\test-read-api.ps1
```

Expected: `FAIL`

- [ ] **Step 3: Create read replica config**

Create `configs/mediamtx/read.yml`:
```yaml
logLevel: info
logDestinations:
  - stdout

api: yes
apiAddress: :9997

metrics: yes
metricsAddress: :9998

rtsp: no

webrtc: yes
webrtcAddress: :8889
webrtcLocalUDPAddress: :8189
webrtcLocalTCPAddress: :8189
webrtcAdditionalHosts:
  - ${WEBRTC_EXTERNAL_HOST}

authMethod: internal
authInternalUsers:
  - user: viewer
    pass: ${VIEWER_SECRET}
    permissions:
      - action: read
        path: ""

paths:
  "~^(.+)$":
    source: rtsp://internal:${INTERNAL_SECRET}@mediamtx-origin-0:8554/$G1
    sourceOnDemand: no
```

- [ ] **Step 4: Commit**

```powershell
git add configs/mediamtx/read.yml scripts/test-read-api.ps1
git commit -m "feat: add MediaMTX read replica config"
```

---

### Task 5: Docker Compose — Core Streaming Services

**Files:**
- Create: `docker-compose.yml`

- [ ] **Step 1: Create docker-compose.yml with origin + read**

Create `docker-compose.yml`:
```yaml
name: superstreaming

networks:
  superstreaming:
    driver: bridge

volumes:
  origin-recordings:
  minio-data:
  postgres-data:
  grafana-data:

services:

  # ── MediaMTX Origin ──────────────────────────────────────────────
  mediamtx-origin-0:
    image: bluenviron/mediamtx:latest
    container_name: mediamtx-origin-0
    restart: unless-stopped
    networks:
      - superstreaming
    ports:
      - "8554:8554"        # RTSP publish
      - "9997:9997"        # Control API (origin)
    volumes:
      - ./configs/mediamtx/origin.yml:/mediamtx.yml:ro
      - origin-recordings:/recordings
    environment:
      - PUBLISH_SECRET=${PUBLISH_SECRET}
      - INTERNAL_SECRET=${INTERNAL_SECRET}
    healthcheck:
      test: ["CMD", "wget", "-qO-", "http://localhost:9997/v3/config/global/get"]
      interval: 10s
      timeout: 5s
      retries: 5
      start_period: 5s

  # ── MediaMTX Read Replica ─────────────────────────────────────────
  mediamtx-read:
    image: bluenviron/mediamtx:latest
    container_name: mediamtx-read
    restart: unless-stopped
    networks:
      - superstreaming
    ports:
      - "8889:8889"        # WebRTC WHEP signaling (TCP)
      - "8189:8189/udp"    # WebRTC media (UDP — happy path)
      - "8189:8189/tcp"    # WebRTC media (TCP — fallback)
      - "9998:9997"        # Control API (read, mapped to host 9998)
    volumes:
      - ./configs/mediamtx/read.yml:/mediamtx.yml:ro
    environment:
      - INTERNAL_SECRET=${INTERNAL_SECRET}
      - VIEWER_SECRET=${VIEWER_SECRET}
      - WEBRTC_EXTERNAL_HOST=${WEBRTC_EXTERNAL_HOST}
    depends_on:
      mediamtx-origin-0:
        condition: service_healthy
    healthcheck:
      test: ["CMD", "wget", "-qO-", "http://localhost:9997/v3/config/global/get"]
      interval: 10s
      timeout: 5s
      retries: 5
      start_period: 10s
```

- [ ] **Step 2: Start core services**

```powershell
docker compose up -d mediamtx-origin-0 mediamtx-read
```

Expected output: containers starting. Wait ~15s for health checks.

- [ ] **Step 3: Verify both healthy**

```powershell
docker compose ps
```

Expected: both containers show `healthy` status.

- [ ] **Step 4: Run origin API smoke test**

```powershell
.\scripts\test-origin-api.ps1
```

Expected: `PASS: origin API healthy`

- [ ] **Step 5: Run read API smoke test**

```powershell
.\scripts\test-read-api.ps1
```

Expected: `PASS: read replica API healthy`

- [ ] **Step 6: Check read replica connected to origin**

```powershell
Invoke-RestMethod "http://localhost:9998/v3/paths/list" | ConvertTo-Json -Depth 5
```

Expected: JSON response with empty `items` array (no streams yet — correct, none published).

- [ ] **Step 7: Commit**

```powershell
docker compose down
git add docker-compose.yml
git commit -m "feat: add Docker Compose core streaming services"
```

---

### Task 6: Verify End-to-End Streaming Path

**Files:** none (smoke test only)

- [ ] **Step 1: Start core services**

```powershell
docker compose up -d mediamtx-origin-0 mediamtx-read
docker compose ps
```

Wait for both `healthy`.

- [ ] **Step 2: Load .env vars into PowerShell session**

```powershell
# Load .env file into current PowerShell session
Get-Content .env | Where-Object { $_ -notmatch "^#" -and $_ -match "=" } | ForEach-Object {
    $k, $v = $_ -split "=", 2
    [System.Environment]::SetEnvironmentVariable($k.Trim(), $v.Trim())
}
```

Run this once per terminal session before any `docker run` commands that use `${env:*}` vars.

- [ ] **Step 3: Publish a test RTSP stream using ffmpeg in Docker**

```powershell
docker run --rm --network superstreaming_superstreaming `
  linuxserver/ffmpeg `
  -re -f lavfi -i "testsrc=size=640x480:rate=25" `
  -f lavfi -i "sine=frequency=1000:sample_rate=44100" `
  -c:v libx264 -preset ultrafast -tune zerolatency -profile:v baseline `
  -b:v 500k -g 50 -keyint_min 50 `
  -c:a aac -b:a 64k `
  -f rtsp "rtsp://publisher:${env:PUBLISH_SECRET}@mediamtx-origin-0:8554/test-stream"
```

Leave this running in a separate terminal.

- [ ] **Step 4: Verify stream visible on origin**

```powershell
Invoke-RestMethod "http://localhost:9997/v3/paths/list" | ConvertTo-Json -Depth 5
```

Expected: `items` array contains one entry with `name: "test-stream"` and `ready: true`.

- [ ] **Step 5: Verify stream visible on read replica**

```powershell
Invoke-RestMethod "http://localhost:9998/v3/paths/list" | ConvertTo-Json -Depth 5
```

Expected: `items` array contains `test-stream` with source connected to origin.

- [ ] **Step 6: Test WHEP signaling endpoint**

```powershell
$headers = @{ "Content-Type" = "application/sdp" }
$cred = [Convert]::ToBase64String([Text.Encoding]::ASCII.GetBytes("viewer:${env:VIEWER_SECRET}"))
$headers["Authorization"] = "Basic $cred"

try {
    $response = Invoke-WebRequest -Uri "http://localhost:8889/test-stream/whep" `
        -Method POST -Headers $headers -Body "" -UseBasicParsing
    Write-Host "PASS: WHEP endpoint responded $($response.StatusCode)" -ForegroundColor Green
} catch {
    Write-Host "FAIL: $($_.Exception.Message)" -ForegroundColor Red
}
```

Expected: HTTP 201 Created with SDP answer in body (WebRTC offer/answer completed).

- [ ] **Step 7: Check recording created on origin**

```powershell
docker exec mediamtx-origin-0 find /recordings -name "*.mp4" 2>/dev/null
```

Wait ~65 seconds (first segment completes after 60s). Expected: one or more `.mp4` files in `/recordings/test-stream/`.

- [ ] **Step 8: Stop test publisher and clean up**

Stop the ffmpeg container (Ctrl+C in its terminal), then:
```powershell
docker compose down
```

- [ ] **Step 9: Commit smoke test script**

Create `scripts/test-streaming-path.ps1`:
```powershell
# Smoke test: publish RTSP stream, verify visible via origin and read APIs
param(
    [string]$PublishSecret = $env:PUBLISH_SECRET,
    [string]$ViewerSecret = $env:VIEWER_SECRET
)

$pass = $true

# Check origin
$r = Invoke-RestMethod "http://localhost:9997/v3/paths/list" -ErrorAction SilentlyContinue
if ($r.items.Count -gt 0) {
    Write-Host "PASS: origin has $($r.items.Count) active stream(s)" -ForegroundColor Green
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

# Check WHEP endpoint
$cred = [Convert]::ToBase64String([Text.Encoding]::ASCII.GetBytes("viewer:$ViewerSecret"))
$headers = @{ "Authorization" = "Basic $cred"; "Content-Type" = "application/sdp" }
try {
    $w = Invoke-WebRequest -Uri "http://localhost:8889/test-stream/whep" -Method POST -Headers $headers -Body "" -UseBasicParsing
    Write-Host "PASS: WHEP handshake succeeded (HTTP $($w.StatusCode))" -ForegroundColor Green
} catch {
    Write-Host "FAIL: WHEP handshake failed: $($_.Exception.Message)" -ForegroundColor Red
    $pass = $false
}

if ($pass) { Write-Host "`nAll streaming path tests PASSED" -ForegroundColor Green; exit 0 }
else { Write-Host "`nSome tests FAILED" -ForegroundColor Red; exit 1 }
```

```powershell
git add scripts/test-streaming-path.ps1
git commit -m "test: add streaming path smoke test"
```

---

### Task 7: Docker Compose — Storage (MinIO + Postgres)

**Files:**
- Modify: `docker-compose.yml` (add minio, postgres services)
- Create: `sql/init.sql`

- [ ] **Step 1: Write storage smoke test**

Create `scripts/test-storage.ps1`:
```powershell
$pass = $true

# MinIO S3 health
$r = Invoke-WebRequest -Uri "http://localhost:9000/minio/health/live" -UseBasicParsing -ErrorAction SilentlyContinue
if ($r.StatusCode -eq 200) {
    Write-Host "PASS: MinIO healthy" -ForegroundColor Green
} else {
    Write-Host "FAIL: MinIO not healthy (expected after Task 7)" -ForegroundColor Yellow
    $pass = $false
}

# Postgres ping
$pg = docker exec superstreaming-postgres-1 pg_isready -U superstreaming 2>&1
if ($pg -match "accepting connections") {
    Write-Host "PASS: Postgres accepting connections" -ForegroundColor Green
} else {
    Write-Host "FAIL: Postgres not ready (expected after Task 7)" -ForegroundColor Yellow
    $pass = $false
}

if ($pass) { exit 0 } else { exit 1 }
```

- [ ] **Step 2: Run test — confirm fails**

```powershell
.\scripts\test-storage.ps1
```

Expected: both FAIL lines.

- [ ] **Step 3: Create Postgres init SQL**

Create `sql/init.sql`:
```sql
CREATE TABLE IF NOT EXISTS recordings (
  id          BIGSERIAL PRIMARY KEY,
  stream_id   TEXT NOT NULL,
  start_time  TIMESTAMPTZ NOT NULL,
  end_time    TIMESTAMPTZ NOT NULL,
  minio_path  TEXT NOT NULL,
  duration_s  INTEGER NOT NULL,
  created_at  TIMESTAMPTZ DEFAULT now()
);

CREATE INDEX IF NOT EXISTS recordings_stream_time_idx ON recordings (stream_id, start_time);
```

- [ ] **Step 4: Add MinIO and Postgres to docker-compose.yml**

Add these services to `docker-compose.yml` under the existing services:
```yaml
  # ── MinIO ─────────────────────────────────────────────────────────
  minio:
    image: minio/minio:latest
    container_name: minio
    restart: unless-stopped
    networks:
      - superstreaming
    ports:
      - "9000:9000"        # S3 API
      - "9001:9001"        # MinIO console
    volumes:
      - minio-data:/data
    environment:
      - MINIO_ROOT_USER=${MINIO_ROOT_USER}
      - MINIO_ROOT_PASSWORD=${MINIO_ROOT_PASSWORD}
    command: server /data --console-address ":9001"
    healthcheck:
      test: ["CMD", "mc", "ready", "local"]
      interval: 10s
      timeout: 5s
      retries: 5
      start_period: 10s

  # MinIO bucket init (runs once, creates the recordings bucket)
  minio-init:
    image: minio/mc:latest
    container_name: minio-init
    networks:
      - superstreaming
    depends_on:
      minio:
        condition: service_healthy
    entrypoint: >
      /bin/sh -c "
        mc alias set local http://minio:9000 ${MINIO_ROOT_USER} ${MINIO_ROOT_PASSWORD};
        mc mb --ignore-existing local/${MINIO_BUCKET};
        echo 'MinIO bucket ready';
      "
    environment:
      - MINIO_ROOT_USER=${MINIO_ROOT_USER}
      - MINIO_ROOT_PASSWORD=${MINIO_ROOT_PASSWORD}
      - MINIO_BUCKET=${MINIO_BUCKET}

  # ── Postgres ───────────────────────────────────────────────────────
  postgres:
    image: postgres:16-alpine
    container_name: postgres
    restart: unless-stopped
    networks:
      - superstreaming
    ports:
      - "5432:5432"
    volumes:
      - postgres-data:/var/lib/postgresql/data
      - ./sql/init.sql:/docker-entrypoint-initdb.d/init.sql:ro
    environment:
      - POSTGRES_USER=${POSTGRES_USER}
      - POSTGRES_PASSWORD=${POSTGRES_PASSWORD}
      - POSTGRES_DB=${POSTGRES_DB}
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U ${POSTGRES_USER} -d ${POSTGRES_DB}"]
      interval: 10s
      timeout: 5s
      retries: 5
      start_period: 10s
```

- [ ] **Step 5: Start storage services**

```powershell
docker compose up -d minio minio-init postgres
```

Wait 20s, then:
```powershell
docker compose ps
```

Expected: `minio` is `healthy`, `postgres` is `healthy`, `minio-init` is `exited (0)`.

- [ ] **Step 6: Run storage smoke test**

```powershell
.\scripts\test-storage.ps1
```

Expected: `PASS: MinIO healthy` and `PASS: Postgres accepting connections`.

- [ ] **Step 7: Verify recordings table exists**

```powershell
docker exec postgres psql -U superstreaming -d superstreaming -c "\d recordings"
```

Expected: table description showing all columns (id, stream_id, start_time, end_time, minio_path, duration_s, created_at).

- [ ] **Step 8: Verify MinIO bucket exists**

Open browser: `http://localhost:9001`. Login with `MINIO_ROOT_USER` / `MINIO_ROOT_PASSWORD` from `.env`. Confirm `recordings` bucket exists.

- [ ] **Step 9: Stop and commit**

```powershell
docker compose down
git add docker-compose.yml sql/init.sql scripts/test-storage.ps1
git commit -m "feat: add MinIO and Postgres to Docker Compose"
```

---

### Task 8: Docker Compose — Stub Services + Observability

**Files:**
- Modify: `docker-compose.yml` (add api-server stub, web-frontend stub, prometheus, grafana)
- Create: `configs/prometheus/prometheus.yml`

- [ ] **Step 1: Create Prometheus scrape config**

Create `configs/prometheus/prometheus.yml`:
```yaml
global:
  scrape_interval: 15s
  evaluation_interval: 15s

scrape_configs:
  - job_name: mediamtx-origin
    static_configs:
      - targets:
          - mediamtx-origin-0:9998
    metrics_path: /metrics

  - job_name: mediamtx-read
    static_configs:
      - targets:
          - mediamtx-read:9998
    metrics_path: /metrics
```

- [ ] **Step 2: Add stub services and observability to docker-compose.yml**

Add these services to `docker-compose.yml`:
```yaml
  # ── API Server (stub — replaced in Sub-Project 4) ─────────────────
  api-server:
    image: nginx:alpine
    container_name: api-server
    restart: unless-stopped
    networks:
      - superstreaming
    ports:
      - "8080:80"
    healthcheck:
      test: ["CMD", "wget", "-qO-", "http://localhost/"]
      interval: 10s
      timeout: 5s
      retries: 3

  # ── Web Frontend (stub — replaced in Sub-Project 5) ───────────────
  web-frontend:
    image: nginx:alpine
    container_name: web-frontend
    restart: unless-stopped
    networks:
      - superstreaming
    ports:
      - "80:80"
    healthcheck:
      test: ["CMD", "wget", "-qO-", "http://localhost/"]
      interval: 10s
      timeout: 5s
      retries: 3

  # ── Prometheus ─────────────────────────────────────────────────────
  prometheus:
    image: prom/prometheus:latest
    container_name: prometheus
    restart: unless-stopped
    networks:
      - superstreaming
    ports:
      - "9090:9090"
    volumes:
      - ./configs/prometheus/prometheus.yml:/etc/prometheus/prometheus.yml:ro
    healthcheck:
      test: ["CMD", "wget", "-qO-", "http://localhost:9090/-/healthy"]
      interval: 10s
      timeout: 5s
      retries: 5
      start_period: 10s

  # ── Grafana ────────────────────────────────────────────────────────
  grafana:
    image: grafana/grafana:latest
    container_name: grafana
    restart: unless-stopped
    networks:
      - superstreaming
    ports:
      - "3000:3000"
    volumes:
      - grafana-data:/var/lib/grafana
    environment:
      - GF_SECURITY_ADMIN_PASSWORD=${VIEWER_SECRET}
    healthcheck:
      test: ["CMD", "wget", "-qO-", "http://localhost:3000/api/health"]
      interval: 10s
      timeout: 5s
      retries: 5
      start_period: 15s
```

- [ ] **Step 3: Commit**

```powershell
git add docker-compose.yml configs/prometheus/prometheus.yml
git commit -m "feat: add stub services and observability to Docker Compose"
```

---

### Task 9: Full Docker Compose Smoke Test

**Files:**
- Create: `scripts/test-all-services.ps1`

- [ ] **Step 1: Create full smoke test**

Create `scripts/test-all-services.ps1`:
```powershell
$pass = $true

function Test-Http {
    param($name, $url, $expectedStatus = 200)
    try {
        $r = Invoke-WebRequest -Uri $url -UseBasicParsing -ErrorAction Stop
        if ($r.StatusCode -eq $expectedStatus) {
            Write-Host "PASS: $name ($url)" -ForegroundColor Green
        } else {
            Write-Host "FAIL: $name - got $($r.StatusCode), expected $expectedStatus" -ForegroundColor Red
            $script:pass = $false
        }
    } catch {
        Write-Host "FAIL: $name - $($_.Exception.Message)" -ForegroundColor Red
        $script:pass = $false
    }
}

Test-Http "MediaMTX origin API"    "http://localhost:9997/v3/config/global/get"
Test-Http "MediaMTX read API"      "http://localhost:9998/v3/config/global/get"
Test-Http "MinIO health"           "http://localhost:9000/minio/health/live"
Test-Http "API server stub"        "http://localhost:8080/"
Test-Http "Web frontend stub"      "http://localhost:80/"
Test-Http "Prometheus"             "http://localhost:9090/-/healthy"
Test-Http "Grafana"                "http://localhost:3000/api/health"

# Postgres
$pg = docker exec postgres pg_isready -U superstreaming 2>&1
if ($pg -match "accepting connections") {
    Write-Host "PASS: Postgres accepting connections" -ForegroundColor Green
} else {
    Write-Host "FAIL: Postgres not ready" -ForegroundColor Red
    $pass = $false
}

if ($pass) { Write-Host "`nAll services HEALTHY" -ForegroundColor Green; exit 0 }
else { Write-Host "`nSome services FAILED" -ForegroundColor Red; exit 1 }
```

- [ ] **Step 2: Start all services**

```powershell
docker compose up -d
```

Wait 30s for all services to become healthy.

- [ ] **Step 3: Run full smoke test**

```powershell
.\scripts\test-all-services.ps1
```

Expected: all 8 PASS lines.

- [ ] **Step 4: Verify Prometheus scraping MediaMTX**

Open browser: `http://localhost:9090/targets`

Expected: two targets visible — `mediamtx-origin` and `mediamtx-read` — both showing `UP` state.

- [ ] **Step 5: Stop and commit**

```powershell
docker compose down
git add scripts/test-all-services.ps1
git commit -m "test: add full service smoke test"
```

---

### Task 10: k3s Manifests — Namespace + ConfigMaps + Secrets

**Files:**
- Create: `k8s/namespace.yaml`
- Create: `k8s/configmaps/mediamtx-origin-config.yaml`
- Create: `k8s/configmaps/mediamtx-read-config.yaml`
- Create: `k8s/configmaps/prometheus-config.yaml`
- Create: `k8s/secrets/secrets.yaml.example`

- [ ] **Step 1: Create namespace**

Create `k8s/namespace.yaml`:
```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: superstreaming
```

- [ ] **Step 2: Create origin ConfigMap**

Create `k8s/configmaps/mediamtx-origin-config.yaml`:
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: mediamtx-origin-config
  namespace: superstreaming
data:
  mediamtx.yml: |
    logLevel: info
    logDestinations:
      - stdout

    api: yes
    apiAddress: :9997

    metrics: yes
    metricsAddress: :9998

    rtsp: yes
    rtspAddress: :8554
    rtspEncryption: no

    webrtc: no

    authMethod: internal
    authInternalUsers:
      - user: publisher
        pass: $(PUBLISH_SECRET)
        permissions:
          - action: publish
            path: ""
      - user: internal
        pass: $(INTERNAL_SECRET)
        permissions:
          - action: read
            path: ""
          - action: playback
            path: ""

    paths:
      all_others:
        record: yes
        recordPath: /recordings/%path/%Y-%m-%d_%H-%M-%S
        recordFormat: fmp4
        recordPartDuration: 1s
        recordSegmentDuration: 60s
        recordDeleteAfter: 2h
```

Note: Use `$(VAR)` NOT `${VAR}` in ConfigMap data — k8s will not expand env vars in ConfigMap values. Instead, mediamtx reads env vars from the container environment using `$VAR_NAME` syntax. So keep `${PUBLISH_SECRET}` as-is — mediamtx expands it, not k8s.

Correction: keep `${PUBLISH_SECRET}` syntax. The ConfigMap stores the literal string, and mediamtx performs variable substitution at runtime from the container's env vars (injected via Secret).

Update the ConfigMap to use `${PUBLISH_SECRET}` (literal, not interpreted by k8s):
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: mediamtx-origin-config
  namespace: superstreaming
data:
  mediamtx.yml: |
    logLevel: info
    logDestinations:
      - stdout

    api: yes
    apiAddress: :9997

    metrics: yes
    metricsAddress: :9998

    rtsp: yes
    rtspAddress: :8554
    rtspEncryption: no

    webrtc: no

    authMethod: internal
    authInternalUsers:
      - user: publisher
        pass: ${PUBLISH_SECRET}
        permissions:
          - action: publish
            path: ""
      - user: internal
        pass: ${INTERNAL_SECRET}
        permissions:
          - action: read
            path: ""
          - action: playback
            path: ""

    paths:
      all_others:
        record: yes
        recordPath: /recordings/%path/%Y-%m-%d_%H-%M-%S
        recordFormat: fmp4
        recordPartDuration: 1s
        recordSegmentDuration: 60s
        recordDeleteAfter: 2h
```

- [ ] **Step 3: Create read replica ConfigMap**

Create `k8s/configmaps/mediamtx-read-config.yaml`:
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: mediamtx-read-config
  namespace: superstreaming
data:
  mediamtx.yml: |
    logLevel: info
    logDestinations:
      - stdout

    api: yes
    apiAddress: :9997

    metrics: yes
    metricsAddress: :9998

    rtsp: no

    webrtc: yes
    webrtcAddress: :8889
    webrtcLocalUDPAddress: :8189
    webrtcLocalTCPAddress: :8189
    webrtcAdditionalHosts:
      - ${WEBRTC_EXTERNAL_HOST}

    authMethod: internal
    authInternalUsers:
      - user: viewer
        pass: ${VIEWER_SECRET}
        permissions:
          - action: read
            path: ""

    paths:
      "~^(.+)$":
        source: rtsp://internal:${INTERNAL_SECRET}@mediamtx-origin-0.mediamtx-origin-headless.superstreaming.svc.cluster.local:8554/$G1
        sourceOnDemand: no
```

- [ ] **Step 4: Create Prometheus ConfigMap**

Create `k8s/configmaps/prometheus-config.yaml`:
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: prometheus-config
  namespace: superstreaming
data:
  prometheus.yml: |
    global:
      scrape_interval: 15s

    scrape_configs:
      - job_name: mediamtx-origin
        static_configs:
          - targets:
              - mediamtx-origin-0.mediamtx-origin-headless:9998
        metrics_path: /metrics

      - job_name: mediamtx-read
        kubernetes_sd_configs:
          - role: pod
            namespaces:
              names:
                - superstreaming
        relabel_configs:
          - source_labels: [__meta_kubernetes_pod_label_app]
            action: keep
            regex: mediamtx-read
          - source_labels: [__meta_kubernetes_pod_ip]
            target_label: __address__
            replacement: "${1}:9998"
```

- [ ] **Step 5: Create Secrets example**

Create `k8s/secrets/secrets.yaml.example`:
```yaml
apiVersion: v1
kind: Secret
metadata:
  name: superstreaming-secrets
  namespace: superstreaming
type: Opaque
stringData:
  PUBLISH_SECRET: "changeme_publish"
  INTERNAL_SECRET: "changeme_internal"
  VIEWER_SECRET: "changeme_viewer"
  WEBRTC_EXTERNAL_HOST: "192.168.1.100"     # replace with k3s node IP
  MINIO_ROOT_USER: "minioadmin"
  MINIO_ROOT_PASSWORD: "changeme_minio"
  MINIO_BUCKET: "recordings"
  POSTGRES_USER: "superstreaming"
  POSTGRES_PASSWORD: "changeme_postgres"
  POSTGRES_DB: "superstreaming"
```

- [ ] **Step 6: Apply to Rancher Desktop**

```powershell
kubectl apply -f k8s/namespace.yaml
kubectl apply -f k8s/configmaps/
```

Create `k8s/secrets/secrets.yaml` from example (never commit this):
```powershell
Copy-Item k8s\secrets\secrets.yaml.example k8s\secrets\secrets.yaml
```

Edit `k8s/secrets/secrets.yaml` with real dev values (same as `.env`). Then apply:
```powershell
kubectl apply -f k8s/secrets/secrets.yaml
```

Verify:
```powershell
kubectl get configmaps -n superstreaming
kubectl get secrets -n superstreaming
```

Expected: 4 ConfigMaps and 1 Secret listed.

- [ ] **Step 7: Commit (no secrets.yaml)**

```powershell
git add k8s/namespace.yaml k8s/configmaps/ k8s/secrets/secrets.yaml.example
git commit -m "feat: add k3s namespace, ConfigMaps, and secrets template"
```

---

### Task 11: k3s Manifests — Origin StatefulSet

**Files:**
- Create: `k8s/origin/headless-service.yaml`
- Create: `k8s/origin/pvc.yaml`
- Create: `k8s/origin/statefulset.yaml`

- [ ] **Step 1: Create headless service**

Create `k8s/origin/headless-service.yaml`:
```yaml
apiVersion: v1
kind: Service
metadata:
  name: mediamtx-origin-headless
  namespace: superstreaming
spec:
  clusterIP: None
  selector:
    app: mediamtx-origin
  ports:
    - name: rtsp
      port: 8554
      protocol: TCP
    - name: api
      port: 9997
      protocol: TCP
    - name: metrics
      port: 9998
      protocol: TCP
```

- [ ] **Step 2: Create StatefulSet (single replica for this plan)**

Create `k8s/origin/statefulset.yaml`:
```yaml
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: mediamtx-origin
  namespace: superstreaming
spec:
  serviceName: mediamtx-origin-headless
  replicas: 1
  selector:
    matchLabels:
      app: mediamtx-origin
  template:
    metadata:
      labels:
        app: mediamtx-origin
    spec:
      terminationGracePeriodSeconds: 60
      containers:
        - name: mediamtx
          image: bluenviron/mediamtx:latest
          args: ["/mediamtx.yml"]
          ports:
            - name: rtsp
              containerPort: 8554
              protocol: TCP
            - name: api
              containerPort: 9997
              protocol: TCP
            - name: metrics
              containerPort: 9998
              protocol: TCP
          envFrom:
            - secretRef:
                name: superstreaming-secrets
          volumeMounts:
            - name: config
              mountPath: /mediamtx.yml
              subPath: mediamtx.yml
            - name: recordings
              mountPath: /recordings
          readinessProbe:
            httpGet:
              path: /v3/config/global/get
              port: 9997
            initialDelaySeconds: 5
            periodSeconds: 10
          livenessProbe:
            tcpSocket:
              port: 8554
            initialDelaySeconds: 20
            periodSeconds: 15
          resources:
            requests:
              cpu: "500m"
              memory: "512Mi"
            limits:
              cpu: "2"
              memory: "2Gi"
      volumes:
        - name: config
          configMap:
            name: mediamtx-origin-config
  volumeClaimTemplates:
    - metadata:
        name: recordings
      spec:
        accessModes: ["ReadWriteOncePod"]
        storageClassName: local-path
        resources:
          requests:
            storage: 20Gi
```

Note: `storageClassName: local-path` uses k3s's built-in local-path provisioner. Sufficient for dev; use a real StorageClass in production.

- [ ] **Step 3: Apply and verify**

```powershell
kubectl apply -f k8s/origin/headless-service.yaml
kubectl apply -f k8s/origin/statefulset.yaml
```

Wait for pod:
```powershell
kubectl wait pod/mediamtx-origin-0 -n superstreaming --for=condition=Ready --timeout=60s
```

Expected: `pod/mediamtx-origin-0 condition met`

Verify API reachable via port-forward:
```powershell
kubectl port-forward pod/mediamtx-origin-0 19997:9997 -n superstreaming
# In separate terminal:
Invoke-RestMethod "http://localhost:19997/v3/config/global/get"
```

Expected: JSON config response.

- [ ] **Step 4: Commit**

```powershell
kubectl delete -f k8s/origin/   # clean up for now
git add k8s/origin/
git commit -m "feat: add k3s MediaMTX origin StatefulSet"
```

---

### Task 12: k3s Manifests — Read Deployment + Service + HPA

**Files:**
- Create: `k8s/read/deployment.yaml`
- Create: `k8s/read/service.yaml`
- Create: `k8s/read/hpa.yaml`

- [ ] **Step 1: Create read Deployment**

Create `k8s/read/deployment.yaml`:
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: mediamtx-read
  namespace: superstreaming
spec:
  replicas: 1
  selector:
    matchLabels:
      app: mediamtx-read
  template:
    metadata:
      labels:
        app: mediamtx-read
    spec:
      terminationGracePeriodSeconds: 30
      containers:
        - name: mediamtx
          image: bluenviron/mediamtx:latest
          args: ["/mediamtx.yml"]
          ports:
            - name: webrtc-http
              containerPort: 8889
              protocol: TCP
            - name: webrtc-udp
              containerPort: 8189
              protocol: UDP
            - name: webrtc-tcp
              containerPort: 8189
              protocol: TCP
            - name: api
              containerPort: 9997
              protocol: TCP
            - name: metrics
              containerPort: 9998
              protocol: TCP
          envFrom:
            - secretRef:
                name: superstreaming-secrets
          volumeMounts:
            - name: config
              mountPath: /mediamtx.yml
              subPath: mediamtx.yml
          readinessProbe:
            httpGet:
              path: /v3/config/global/get
              port: 9997
            initialDelaySeconds: 5
            periodSeconds: 10
          livenessProbe:
            httpGet:
              path: /v3/config/global/get
              port: 9997
            initialDelaySeconds: 20
            periodSeconds: 15
          resources:
            requests:
              cpu: "500m"
              memory: "512Mi"
            limits:
              cpu: "2"
              memory: "2Gi"
      volumes:
        - name: config
          configMap:
            name: mediamtx-read-config
```

- [ ] **Step 2: Create Service (NodePort for external access)**

Create `k8s/read/service.yaml`:
```yaml
apiVersion: v1
kind: Service
metadata:
  name: mediamtx-read
  namespace: superstreaming
spec:
  type: NodePort
  selector:
    app: mediamtx-read
  ports:
    - name: webrtc-http
      port: 8889
      targetPort: 8889
      nodePort: 30889
      protocol: TCP
    - name: webrtc-udp
      port: 8189
      targetPort: 8189
      nodePort: 30189
      protocol: UDP
    - name: webrtc-tcp
      port: 8189
      targetPort: 8189
      nodePort: 30189
      protocol: TCP
```

Note: NodePort 30889 (WebRTC signaling) and 30189 (media). On Rancher Desktop, access via `localhost:30889`.

- [ ] **Step 3: Create HPA (CPU-based for now)**

Create `k8s/read/hpa.yaml`:
```yaml
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: mediamtx-read
  namespace: superstreaming
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: mediamtx-read
  minReplicas: 1
  maxReplicas: 6
  metrics:
    - type: Resource
      resource:
        name: cpu
        target:
          type: Utilization
          averageUtilization: 70
```

Note: CPU-based HPA is a proxy for viewer load. Custom metric (WebRTC sessions) can replace this later when Prometheus adapter is configured.

- [ ] **Step 4: Create PodDisruptionBudget**

Create `k8s/read/pdb.yaml`:
```yaml
apiVersion: policy/v1
kind: PodDisruptionBudget
metadata:
  name: mediamtx-read
  namespace: superstreaming
spec:
  minAvailable: 1
  selector:
    matchLabels:
      app: mediamtx-read
```

Ensures at least 1 read replica stays up during voluntary disruptions (node drains, rolling updates).

- [ ] **Step 5: Apply and verify**

```powershell
kubectl apply -f k8s/origin/
kubectl wait pod/mediamtx-origin-0 -n superstreaming --for=condition=Ready --timeout=60s
kubectl apply -f k8s/read/
kubectl wait deployment/mediamtx-read -n superstreaming --for=condition=Available --timeout=60s
```

```powershell
kubectl get pods -n superstreaming
```

Expected:
```
NAME                             READY   STATUS    RESTARTS
mediamtx-origin-0                1/1     Running   0
mediamtx-read-xxxxxxxxx-xxxxx    1/1     Running   0
```

- [ ] **Step 5: Commit and clean up k3s**

```powershell
git add k8s/read/
git commit -m "feat: add k3s MediaMTX read Deployment, Service, and HPA"
kubectl delete -f k8s/read/ -f k8s/origin/
```

---

### Task 13: k3s Manifests — Storage (MinIO + Postgres)

**Files:**
- Create: `k8s/minio/deployment.yaml`
- Create: `k8s/minio/service.yaml`
- Create: `k8s/minio/pvc.yaml`
- Create: `k8s/postgres/deployment.yaml`
- Create: `k8s/postgres/service.yaml`
- Create: `k8s/postgres/pvc.yaml`

- [ ] **Step 1: Create MinIO PVC**

Create `k8s/minio/pvc.yaml`:
```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: minio-data
  namespace: superstreaming
spec:
  accessModes:
    - ReadWriteOnce
  storageClassName: local-path
  resources:
    requests:
      storage: 50Gi
```

- [ ] **Step 2: Create MinIO Deployment**

Create `k8s/minio/deployment.yaml`:
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: minio
  namespace: superstreaming
spec:
  replicas: 1
  selector:
    matchLabels:
      app: minio
  template:
    metadata:
      labels:
        app: minio
    spec:
      containers:
        - name: minio
          image: minio/minio:latest
          args: ["server", "/data", "--console-address", ":9001"]
          ports:
            - containerPort: 9000
              name: s3
            - containerPort: 9001
              name: console
          env:
            - name: MINIO_ROOT_USER
              valueFrom:
                secretKeyRef:
                  name: superstreaming-secrets
                  key: MINIO_ROOT_USER
            - name: MINIO_ROOT_PASSWORD
              valueFrom:
                secretKeyRef:
                  name: superstreaming-secrets
                  key: MINIO_ROOT_PASSWORD
          volumeMounts:
            - name: data
              mountPath: /data
          readinessProbe:
            httpGet:
              path: /minio/health/ready
              port: 9000
            initialDelaySeconds: 10
            periodSeconds: 10
          resources:
            requests:
              cpu: "250m"
              memory: "512Mi"
            limits:
              cpu: "1"
              memory: "2Gi"
      volumes:
        - name: data
          persistentVolumeClaim:
            claimName: minio-data
```

- [ ] **Step 3: Create MinIO Service**

Create `k8s/minio/service.yaml`:
```yaml
apiVersion: v1
kind: Service
metadata:
  name: minio
  namespace: superstreaming
spec:
  selector:
    app: minio
  ports:
    - name: s3
      port: 9000
      targetPort: 9000
    - name: console
      port: 9001
      targetPort: 9001
  type: ClusterIP
```

- [ ] **Step 4: Create Postgres PVC**

Create `k8s/postgres/pvc.yaml`:
```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: postgres-data
  namespace: superstreaming
spec:
  accessModes:
    - ReadWriteOnce
  storageClassName: local-path
  resources:
    requests:
      storage: 10Gi
```

- [ ] **Step 5: Create Postgres ConfigMap for init SQL**

Add to `k8s/configmaps/` — create `k8s/configmaps/postgres-init.yaml`:
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: postgres-init
  namespace: superstreaming
data:
  init.sql: |
    CREATE TABLE IF NOT EXISTS recordings (
      id          BIGSERIAL PRIMARY KEY,
      stream_id   TEXT NOT NULL,
      start_time  TIMESTAMPTZ NOT NULL,
      end_time    TIMESTAMPTZ NOT NULL,
      minio_path  TEXT NOT NULL,
      duration_s  INTEGER NOT NULL,
      created_at  TIMESTAMPTZ DEFAULT now()
    );
    CREATE INDEX IF NOT EXISTS recordings_stream_time_idx ON recordings (stream_id, start_time);
```

- [ ] **Step 6: Create Postgres Deployment**

Create `k8s/postgres/deployment.yaml`:
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: postgres
  namespace: superstreaming
spec:
  replicas: 1
  selector:
    matchLabels:
      app: postgres
  template:
    metadata:
      labels:
        app: postgres
    spec:
      containers:
        - name: postgres
          image: postgres:16-alpine
          ports:
            - containerPort: 5432
          env:
            - name: POSTGRES_USER
              valueFrom:
                secretKeyRef:
                  name: superstreaming-secrets
                  key: POSTGRES_USER
            - name: POSTGRES_PASSWORD
              valueFrom:
                secretKeyRef:
                  name: superstreaming-secrets
                  key: POSTGRES_PASSWORD
            - name: POSTGRES_DB
              valueFrom:
                secretKeyRef:
                  name: superstreaming-secrets
                  key: POSTGRES_DB
          volumeMounts:
            - name: data
              mountPath: /var/lib/postgresql/data
            - name: init
              mountPath: /docker-entrypoint-initdb.d
          readinessProbe:
            exec:
              command: ["pg_isready", "-U", "superstreaming"]
            initialDelaySeconds: 10
            periodSeconds: 10
          resources:
            requests:
              cpu: "250m"
              memory: "256Mi"
            limits:
              cpu: "1"
              memory: "1Gi"
      volumes:
        - name: data
          persistentVolumeClaim:
            claimName: postgres-data
        - name: init
          configMap:
            name: postgres-init
```

- [ ] **Step 7: Create Postgres Service**

Create `k8s/postgres/service.yaml`:
```yaml
apiVersion: v1
kind: Service
metadata:
  name: postgres
  namespace: superstreaming
spec:
  selector:
    app: postgres
  ports:
    - port: 5432
      targetPort: 5432
  type: ClusterIP
```

- [ ] **Step 8: Commit**

```powershell
git add k8s/minio/ k8s/postgres/ k8s/configmaps/postgres-init.yaml
git commit -m "feat: add k3s storage manifests (MinIO + Postgres)"
```

---

### Task 14: k3s Manifests — Monitoring + Stubs

**Files:**
- Create: `k8s/prometheus/deployment.yaml`, `k8s/prometheus/service.yaml`
- Create: `k8s/grafana/deployment.yaml`, `k8s/grafana/service.yaml`
- Create: `k8s/stubs/api-server-stub.yaml`, `k8s/stubs/web-frontend-stub.yaml`

- [ ] **Step 1: Create Prometheus Deployment**

Create `k8s/prometheus/deployment.yaml`:
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: prometheus
  namespace: superstreaming
spec:
  replicas: 1
  selector:
    matchLabels:
      app: prometheus
  template:
    metadata:
      labels:
        app: prometheus
    spec:
      containers:
        - name: prometheus
          image: prom/prometheus:latest
          args:
            - "--config.file=/etc/prometheus/prometheus.yml"
            - "--storage.tsdb.path=/prometheus"
          ports:
            - containerPort: 9090
          volumeMounts:
            - name: config
              mountPath: /etc/prometheus
          resources:
            requests:
              cpu: "250m"
              memory: "512Mi"
            limits:
              cpu: "1"
              memory: "1Gi"
      volumes:
        - name: config
          configMap:
            name: prometheus-config
```

- [ ] **Step 2: Create Prometheus Service**

Create `k8s/prometheus/service.yaml`:
```yaml
apiVersion: v1
kind: Service
metadata:
  name: prometheus
  namespace: superstreaming
spec:
  selector:
    app: prometheus
  ports:
    - port: 9090
      targetPort: 9090
      nodePort: 30090
  type: NodePort
```

- [ ] **Step 3: Create Grafana Deployment**

Create `k8s/grafana/deployment.yaml`:
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: grafana
  namespace: superstreaming
spec:
  replicas: 1
  selector:
    matchLabels:
      app: grafana
  template:
    metadata:
      labels:
        app: grafana
    spec:
      containers:
        - name: grafana
          image: grafana/grafana:latest
          ports:
            - containerPort: 3000
          env:
            - name: GF_SECURITY_ADMIN_PASSWORD
              valueFrom:
                secretKeyRef:
                  name: superstreaming-secrets
                  key: VIEWER_SECRET
          resources:
            requests:
              cpu: "250m"
              memory: "256Mi"
            limits:
              cpu: "1"
              memory: "512Mi"
```

- [ ] **Step 4: Create Grafana Service**

Create `k8s/grafana/service.yaml`:
```yaml
apiVersion: v1
kind: Service
metadata:
  name: grafana
  namespace: superstreaming
spec:
  selector:
    app: grafana
  ports:
    - port: 3000
      targetPort: 3000
      nodePort: 30300
  type: NodePort
```

- [ ] **Step 5: Create stub deployments**

Create `k8s/stubs/api-server-stub.yaml`:
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
          image: nginx:alpine
          ports:
            - containerPort: 80
---
apiVersion: v1
kind: Service
metadata:
  name: api-server
  namespace: superstreaming
spec:
  selector:
    app: api-server
  ports:
    - port: 8080
      targetPort: 80
  type: ClusterIP
```

Create `k8s/stubs/web-frontend-stub.yaml`:
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: web-frontend
  namespace: superstreaming
spec:
  replicas: 1
  selector:
    matchLabels:
      app: web-frontend
  template:
    metadata:
      labels:
        app: web-frontend
    spec:
      containers:
        - name: web-frontend
          image: nginx:alpine
          ports:
            - containerPort: 80
---
apiVersion: v1
kind: Service
metadata:
  name: web-frontend
  namespace: superstreaming
spec:
  selector:
    app: web-frontend
  ports:
    - port: 80
      targetPort: 80
      nodePort: 30080
  type: NodePort
```

- [ ] **Step 6: Commit**

```powershell
git add k8s/prometheus/ k8s/grafana/ k8s/stubs/
git commit -m "feat: add k3s monitoring and stub service manifests"
```

---

### Task 15: Rancher Desktop Full Validation

**Files:**
- Create: `scripts/test-k8s-services.ps1`

- [ ] **Step 1: Apply all k3s manifests**

```powershell
kubectl apply -f k8s/namespace.yaml
kubectl apply -f k8s/configmaps/
kubectl apply -f k8s/secrets/secrets.yaml
kubectl apply -f k8s/origin/
kubectl apply -f k8s/read/
kubectl apply -f k8s/minio/
kubectl apply -f k8s/postgres/
kubectl apply -f k8s/prometheus/
kubectl apply -f k8s/grafana/
kubectl apply -f k8s/stubs/
```

- [ ] **Step 2: Wait for all pods ready**

```powershell
kubectl wait --for=condition=Ready pods --all -n superstreaming --timeout=120s
```

- [ ] **Step 3: Verify all pods running**

```powershell
kubectl get pods -n superstreaming
```

Expected (all Running):
```
NAME                               READY   STATUS
mediamtx-origin-0                  1/1     Running
mediamtx-read-xxx                  1/1     Running
minio-xxx                          1/1     Running
postgres-xxx                       1/1     Running
prometheus-xxx                     1/1     Running
grafana-xxx                        1/1     Running
api-server-xxx                     1/1     Running
web-frontend-xxx                   1/1     Running
```

- [ ] **Step 4: Write k8s smoke test**

Create `scripts/test-k8s-services.ps1`:
```powershell
$pass = $true

function Test-NodePort {
    param($name, $port, $path = "/")
    try {
        $r = Invoke-WebRequest -Uri "http://localhost:$port$path" -UseBasicParsing -ErrorAction Stop
        Write-Host "PASS: $name (localhost:$port$path)" -ForegroundColor Green
    } catch {
        Write-Host "FAIL: $name (localhost:$port$path) - $($_.Exception.Message)" -ForegroundColor Red
        $script:pass = $false
    }
}

# Port-forward origin API for test
$pfOrigin = Start-Process kubectl -ArgumentList "port-forward pod/mediamtx-origin-0 19997:9997 -n superstreaming" -PassThru
Start-Sleep 3
try {
    $r = Invoke-RestMethod "http://localhost:19997/v3/config/global/get" -ErrorAction Stop
    Write-Host "PASS: MediaMTX origin API" -ForegroundColor Green
} catch {
    Write-Host "FAIL: MediaMTX origin API - $($_.Exception.Message)" -ForegroundColor Red
    $pass = $false
}
$pfOrigin | Stop-Process

Test-NodePort "MediaMTX read WHEP"  30889 "/"
Test-NodePort "Prometheus"          30090 "/-/healthy"
Test-NodePort "Grafana"             30300 "/api/health"
Test-NodePort "Web frontend stub"   30080 "/"

if ($pass) { Write-Host "`nAll k8s services HEALTHY" -ForegroundColor Green; exit 0 }
else { Write-Host "`nSome k8s services FAILED" -ForegroundColor Red; exit 1 }
```

- [ ] **Step 5: Run k8s smoke test**

```powershell
.\scripts\test-k8s-services.ps1
```

Expected: all PASS lines.

- [ ] **Step 6: Test streaming path on k3s**

Publish a test stream:
```powershell
# Get the k3s node IP used for NodePort
# On Rancher Desktop: usually 127.0.0.1
$nodeIP = "127.0.0.1"

docker run --rm `
  linuxserver/ffmpeg `
  -re -f lavfi -i "testsrc=size=640x480:rate=25" `
  -f lavfi -i "sine=frequency=1000:sample_rate=44100" `
  -c:v libx264 -preset ultrafast -tune zerolatency -profile:v baseline `
  -b:v 500k -g 50 -keyint_min 50 `
  -c:a aac -b:a 64k `
  -f rtsp "rtsp://publisher:${env:PUBLISH_SECRET}@${nodeIP}:30554/test-stream"
```

Note: origin RTSP port 8554 is NOT exposed via NodePort in current manifests. Add NodePort for RTSP to `k8s/origin/headless-service.yaml` or create a separate Service:

Add `k8s/origin/publisher-service.yaml`:
```yaml
apiVersion: v1
kind: Service
metadata:
  name: mediamtx-origin-publisher
  namespace: superstreaming
spec:
  selector:
    app: mediamtx-origin
  ports:
    - name: rtsp
      port: 8554
      targetPort: 8554
      nodePort: 30554
  type: NodePort
```

```powershell
kubectl apply -f k8s/origin/publisher-service.yaml
```

Now run the ffmpeg publisher to `localhost:30554`. Then verify on origin via port-forward:
```powershell
kubectl port-forward pod/mediamtx-origin-0 19997:9997 -n superstreaming &
Invoke-RestMethod "http://localhost:19997/v3/paths/list"
```

Expected: `test-stream` appears in items with `ready: true`.

Test WHEP on read replica:
```powershell
$cred = [Convert]::ToBase64String([Text.Encoding]::ASCII.GetBytes("viewer:${env:VIEWER_SECRET}"))
$headers = @{ "Authorization" = "Basic $cred"; "Content-Type" = "application/sdp" }
Invoke-WebRequest -Uri "http://localhost:30889/test-stream/whep" -Method POST -Headers $headers -Body "" -UseBasicParsing
```

Expected: HTTP 201 with SDP answer.

- [ ] **Step 7: Final commit**

```powershell
git add k8s/origin/publisher-service.yaml scripts/test-k8s-services.ps1
git commit -m "feat: complete k3s infrastructure skeleton + validation"
```

---

## Verification Summary

After completing all tasks, verify:

| Check | Command | Expected |
|-------|---------|----------|
| All Docker Compose services healthy | `docker compose up -d && .\scripts\test-all-services.ps1` | All PASS |
| Streaming path (Compose) | publish ffmpeg → `.\scripts\test-streaming-path.ps1` | All PASS |
| All k3s pods running | `kubectl get pods -n superstreaming` | All Running |
| Streaming path (k3s) | publish ffmpeg → WHEP curl test | HTTP 201 |
| MinIO bucket exists | Open `localhost:9001` | `recordings` bucket visible |
| Postgres schema | `kubectl exec postgres-xxx -- psql ... \d recordings` | table exists |
| Prometheus scraping | `localhost:9090/targets` or `localhost:30090/targets` | both targets UP |

---

## What's Next

Sub-Project 2 is not needed (it was merged into this plan). Continue with:

- **Sub-Project 3:** Recording pipeline — Go recording-uploader sidecar
- **Sub-Project 4:** API server — Go service, SSE stream list, recordings endpoint
- **Sub-Project 5:** Web frontend — React + Vite + reader.js + Tailwind
- **Sub-Project 6:** Observability — Grafana dashboards
- **Sub-Project 7:** Air-gap bundle — image export scripts
