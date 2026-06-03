# SuperStreaming — Deployment Manual

> Covers local Docker Compose dev, Kubernetes (Docker Desktop) validation, and air-gapped k3s production deploy on Ubuntu VM.

---

## Prerequisites

| Tool | Purpose | Notes |
|------|---------|-------|
| Docker Desktop | Local dev, Docker Compose, image builds | Enable k8s in Settings → Kubernetes |
| kubectl | k8s CLI | Bundled with Docker Desktop |
| git | Source control | — |
| PowerShell 5.1+ | Scripts | Windows built-in |

---

## Environment Configuration

All secrets live in `.env` (gitignored). Never commit it.

```powershell
# One-time setup
Copy-Item .env.example .env
# Edit .env — replace all changeme_* values with real secrets
```

`.env` variables:

```env
PUBLISH_SECRET=<secret>          # RTSP publishers authenticate with this
INTERNAL_SECRET=<secret>         # read replicas pull from origin with this
VIEWER_SECRET=<secret>           # WebRTC viewers (WHEP) authenticate with this
WEBRTC_EXTERNAL_HOST=127.0.0.1   # host IP browsers use for ICE candidates
MINIO_ROOT_USER=minioadmin
MINIO_ROOT_PASSWORD=<secret>
MINIO_BUCKET=recordings
POSTGRES_USER=superstreaming
POSTGRES_PASSWORD=<secret>
POSTGRES_DB=superstreaming
ORIGIN_COUNT=1
```

Load vars into current PowerShell session before running docker commands:

```powershell
Get-Content .env | Where-Object { $_ -notmatch "^#" -and $_ -match "=" } | ForEach-Object {
    $k, $v = $_ -split "=", 2
    [System.Environment]::SetEnvironmentVariable($k.Trim(), $v.Trim())
}
```

---

## Local Dev — Docker Compose

### Build

Required once, and again after any config file or Dockerfile change:

```powershell
docker compose build
```

This builds `superstreaming/mediamtx:local` — a thin wrapper around the official MediaMTX binary that runs `envsubst` on the config before startup.

### Start All Services

```powershell
docker compose up -d
docker compose ps     # wait until all containers show (healthy)
```

Expected: 8 containers healthy — `mediamtx-origin-0`, `mediamtx-read`, `minio`, `minio-init` (exits 0), `postgres`, `api-server`, `web-frontend`, `prometheus`, `grafana`.

### Smoke Tests

```powershell
.\scripts\test-all-services.ps1
```

Expected output: 8 × `PASS` lines, then `All services HEALTHY`.

### Publish a Test Stream

```powershell
# Load env vars first (see above)
docker run -d --name ffmpeg-test --network superstreaming_superstreaming `
  linuxserver/ffmpeg `
  -re -f lavfi -i "testsrc=size=640x480:rate=25" `
  -f lavfi -i "aevalsrc=sin(440*2*PI*t):s=48000:c=mono" `
  -vf "format=yuv420p" `
  -c:v libx264 -preset ultrafast -tune zerolatency -profile:v baseline `
  -b:v 500k -g 50 -keyint_min 50 `
  -c:a libopus -ar 48000 -ac 1 `
  -f rtsp -rtsp_transport tcp `
  "rtsp://publisher:${env:PUBLISH_SECRET}@mediamtx-origin-0:8554/test-stream"
```

### Verify End-to-End

```powershell
.\scripts\test-streaming-path.ps1
```

Expected: origin stream active, read replica active, WHEP handshake HTTP 201.

Check recordings after ~65 seconds:

```powershell
docker exec mediamtx-origin-0 find /recordings -name "*.mp4"
```

### Teardown

```powershell
docker stop ffmpeg-test; docker rm ffmpeg-test
docker compose down
```

---

## Kubernetes — Docker Desktop (Validation)

### One-Time Cluster Setup

1. Docker Desktop → Settings → Kubernetes → Enable Kubernetes → Apply & Restart
2. Wait ~2 minutes for cluster to come up
3. Verify:

```powershell
kubectl config use-context docker-desktop
kubectl cluster-info
```

4. Create `local-path` StorageClass alias (Docker Desktop uses `rancher.io/local-path` provisioner but doesn't create this alias by default):

```powershell
@"
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: local-path
provisioner: rancher.io/local-path
volumeBindingMode: WaitForFirstConsumer
reclaimPolicy: Delete
"@ | kubectl apply -f -
```

### Create Secrets

```powershell
Copy-Item k8s\secrets\secrets.yaml.example k8s\secrets\secrets.yaml
# Edit k8s\secrets\secrets.yaml — fill in all values
# WEBRTC_EXTERNAL_HOST: "127.0.0.1" for Docker Desktop local testing
```

> **Never commit `k8s/secrets/secrets.yaml`** — it is gitignored.

### Deploy

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

### Wait for Pods

```powershell
kubectl wait --for=condition=Ready pods --all -n superstreaming --timeout=180s
kubectl get pods -n superstreaming
```

Expected — all Running:

```
NAME                            READY   STATUS
mediamtx-origin-0               1/1     Running
mediamtx-read-xxx               1/1     Running
minio-xxx                       1/1     Running
postgres-xxx                    1/1     Running
prometheus-xxx                  1/1     Running
grafana-xxx                     1/1     Running
api-server-xxx                  1/1     Running
web-frontend-xxx                1/1     Running
```

### Smoke Test

```powershell
.\scripts\test-k8s-services.ps1
```

Uses `kubectl port-forward` internally (Docker Desktop WSL2 networking does not expose NodePorts to Windows localhost).

### Streaming Path Test

Port-forward RTSP and publish via `host.docker.internal`:

```powershell
$pfRTSP = Start-Process -FilePath kubectl -ArgumentList @("port-forward","pod/mediamtx-origin-0","28554:8554","-n","superstreaming") -NoNewWindow -PassThru
Start-Sleep 3

docker run -d --name ffmpeg-k8s `
  linuxserver/ffmpeg `
  -re -f lavfi -i "testsrc=size=640x480:rate=25" `
  -f lavfi -i "aevalsrc=sin(440*2*PI*t):s=48000:c=mono" `
  -vf "format=yuv420p" `
  -c:v libx264 -preset ultrafast -tune zerolatency -profile:v baseline `
  -b:v 500k -g 50 -keyint_min 50 `
  -c:a libopus -ar 48000 -ac 1 `
  -f rtsp -rtsp_transport tcp `
  "rtsp://publisher:<PUBLISH_SECRET>@host.docker.internal:28554/test-stream"
```

Verify stream on origin:

```powershell
$pf = Start-Process -FilePath kubectl -ArgumentList @("port-forward","pod/mediamtx-origin-0","29997:9997","-n","superstreaming") -NoNewWindow -PassThru
Start-Sleep 3
Invoke-RestMethod "http://localhost:29997/v3/paths/list" | ConvertTo-Json -Depth 3
$pf | Stop-Process
```

WHEP test:

```powershell
$pfRead = Start-Process -FilePath kubectl -ArgumentList @("port-forward","deployment/mediamtx-read","28889:8889","-n","superstreaming") -NoNewWindow -PassThru
Start-Sleep 3
$sdp = "v=0`r`no=- 0 0 IN IP4 0.0.0.0`r`ns=-`r`nt=0 0`r`nm=video 9 UDP/TLS/RTP/SAVPF 96`r`nc=IN IP4 0.0.0.0`r`na=ice-ufrag:ABcd`r`na=ice-pwd:ABcdEFghIJklMNopQRstUVwx`r`na=fingerprint:sha-256 AA:BB:CC:DD:EE:FF:00:11:22:33:44:55:66:77:88:99:AA:BB:CC:DD:EE:FF:00:11:22:33:44:55:66:77:88:99`r`na=setup:actpass`r`na=mid:0`r`na=recvonly`r`na=rtcp-mux`r`na=rtpmap:96 H264/90000`r`na=fmtp:96 level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=42e01f`r`n"
$cred = [Convert]::ToBase64String([Text.Encoding]::ASCII.GetBytes("viewer:<VIEWER_SECRET>"))
Invoke-WebRequest -Uri "http://localhost:28889/test-stream/whep" -Method POST `
  -Headers @{ Authorization="Basic $cred"; "Content-Type"="application/sdp" } `
  -Body $sdp -UseBasicParsing | Select-Object StatusCode
# Expected: 201
$pfRead | Stop-Process
```

### Teardown

```powershell
kubectl delete namespace superstreaming
```

---

## Production — Air-Gapped k3s on Ubuntu VM

### Architecture

- k3s bare metal, single node (extend to multi-node by adding agents)
- Images transferred via tar — no internet access required
- `local-path` StorageClass built into k3s — no alias needed

### Step 1 — Export Images on Windows

```powershell
docker save superstreaming/mediamtx:local | gzip > mediamtx-local.tar.gz
docker save minio/minio:latest            | gzip > minio.tar.gz
docker save minio/mc:latest               | gzip > minio-mc.tar.gz
docker save postgres:16-alpine            | gzip > postgres.tar.gz
docker save prom/prometheus:latest        | gzip > prometheus.tar.gz
docker save grafana/grafana:latest        | gzip > grafana.tar.gz
docker save nginx:alpine                  | gzip > nginx.tar.gz
docker save linuxserver/ffmpeg:latest     | gzip > ffmpeg.tar.gz   # test publisher only
```

### Step 2 — Transfer to Ubuntu VM

```bash
scp *.tar.gz user@<vm-ip>:~/images/
scp -r k8s/ user@<vm-ip>:~/superstreaming/
```

### Step 3 — Import Images into k3s

```bash
# On Ubuntu VM, for each image:
sudo k3s ctr images import ~/images/mediamtx-local.tar.gz
sudo k3s ctr images import ~/images/minio.tar.gz
sudo k3s ctr images import ~/images/minio-mc.tar.gz
sudo k3s ctr images import ~/images/postgres.tar.gz
sudo k3s ctr images import ~/images/prometheus.tar.gz
sudo k3s ctr images import ~/images/grafana.tar.gz
sudo k3s ctr images import ~/images/nginx.tar.gz
```

### Step 4 — Create Secrets

```bash
cd ~/superstreaming
cp k8s/secrets/secrets.yaml.example k8s/secrets/secrets.yaml
nano k8s/secrets/secrets.yaml
# Set WEBRTC_EXTERNAL_HOST to the k3s node's LAN IP (e.g. 192.168.1.100)
```

### Step 5 — Deploy

```bash
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

kubectl wait --for=condition=Ready pods --all -n superstreaming --timeout=180s
kubectl get pods -n superstreaming
```

### Step 6 — Access Services

On k3s, NodePorts are accessible via the node's LAN IP:

| Service | URL |
|---------|-----|
| RTSP publish | `rtsp://<node-ip>:30554/<stream-id>` |
| WHEP viewer | `http://<node-ip>:30889/<stream-id>/whep` |
| MinIO console | access via port-forward: `kubectl port-forward svc/minio 9001:9001 -n superstreaming` |
| Prometheus | `http://<node-ip>:30090` |
| Grafana | `http://<node-ip>:30300` |

### Step 7 — Verify Streaming Path

```bash
# Publish from any machine on the LAN
ffmpeg \
  -re -f lavfi -i "testsrc=size=640x480:rate=25" \
  -f lavfi -i "aevalsrc=sin(440*2*PI*t):s=48000:c=mono" \
  -vf "format=yuv420p" \
  -c:v libx264 -preset ultrafast -tune zerolatency -profile:v baseline \
  -b:v 500k -g 50 -keyint_min 50 \
  -c:a libopus -ar 48000 -ac 1 \
  -f rtsp -rtsp_transport tcp \
  "rtsp://publisher:<PUBLISH_SECRET>@<node-ip>:30554/test-stream"

# Verify origin API
curl http://<node-ip>:30554/v3/paths/list    # wrong port, use port-forward
kubectl port-forward pod/mediamtx-origin-0 19997:9997 -n superstreaming &
curl http://localhost:19997/v3/paths/list | jq .
```

---

## Port Reference

| Service | Docker Compose (host) | k8s NodePort | Protocol |
|---------|----------------------|-------------|----------|
| RTSP publish | 8554 | 30554 | TCP |
| WHEP signaling | 8889 | 30889 | TCP |
| WebRTC media UDP | 8189 | 30189 | UDP |
| WebRTC media TCP | 8189 | 30189 | TCP |
| MediaMTX origin API | 9997 | port-forward | HTTP |
| MediaMTX read API | 9998 | port-forward | HTTP |
| MinIO S3 | 9000 | — (ClusterIP) | HTTP |
| MinIO console | 9001 | — (ClusterIP) | HTTP |
| Postgres | 5432 | — (ClusterIP) | TCP |
| Prometheus | 9090 | 30090 | HTTP |
| Grafana | 3000 | 30300 | HTTP |
| API server stub | 8080 | — (ClusterIP) | HTTP |
| Web frontend stub | 80 | 30080 | HTTP |

---

## Smoke Test Scripts

| Script | When to run |
|--------|------------|
| `scripts/test-origin-api.ps1` | After starting origin only |
| `scripts/test-read-api.ps1` | After starting read replica |
| `scripts/test-storage.ps1` | After starting MinIO + Postgres |
| `scripts/test-streaming-path.ps1` | After publishing a test stream (Docker Compose) |
| `scripts/test-all-services.ps1` | Full Docker Compose health check |
| `scripts/test-k8s-services.ps1` | Full k8s health check |

---

## Troubleshooting

### MediaMTX fails to start in k8s — `conf.alias` error

Config field uses wrong YAML type. Check:
- `rtspEncryption` must be `"no"` (quoted string), not `false`
- All boolean fields (`api`, `metrics`, `rtsp`, `webrtc`) must be `true`/`false` (unquoted)

### `sourceOnDemand` error on read replica

MediaMTX requires `sourceOnDemand: true` for regex path patterns. Cannot use `false` with `~^(.+)$` source pattern.

### WHEP returns 400 — `codecs not supported by client`

Stream has AAC audio. MediaMTX without FFmpeg cannot transcode to Opus. Publisher must output Opus audio (`-c:a libopus`), not AAC.

### WHEP returns 400 — `failed to unmarshal SDP: EOF`

SDP offer has LF line endings. Must use CRLF (`\r\n`).

### NodePort not reachable on Docker Desktop

Docker Desktop WSL2 networking does not forward NodePorts to Windows localhost. Use `kubectl port-forward` instead.

### Port binding forbidden on Windows

Port is in Windows reserved/excluded range (check with `netsh int ipv4 show excludedportrange tcp`). Use a different local port number.

### Origin pod stuck Pending — PVC not binding

`ReadWriteOncePod` not supported by provisioner. StatefulSet PVC must use `ReadWriteOnce`.

---

## What's Next

| Sub-Project | Description |
|------------|-------------|
| Sub-Project 3 | Recording pipeline — Go `recording-uploader` sidecar |
| Sub-Project 4 | API server — Go service, SSE stream list, recordings endpoint |
| Sub-Project 5 | Web frontend — React + Vite + reader.js + Tailwind |
| Sub-Project 6 | Observability — Grafana dashboards |
| Sub-Project 7 | Air-gap bundle — image export + transfer scripts |
