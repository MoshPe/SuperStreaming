# SuperStreaming — Complete Deployment Guide

> Authoritative, fully detailed deployment reference for **Docker Compose** (local/single-host),
> **Helm on Kubernetes** (Docker Desktop validation + air-gapped k3s production), every **port**,
> **URL**, **environment variable**, **firewall rule**, and **Coturn STUN/TURN** setup.
>
> This guide supersedes the older `deployment-manual.md` (which predates the Helm chart,
> Coturn, the 3-origin layout, and per-origin read routing).

---

## Table of Contents

1. [Architecture & traffic flow](#1-architecture--traffic-flow)
2. [The golden rule: WEBRTC_EXTERNAL_HOST and COTURN_EXTERNAL_IP](#2-the-golden-rule)
3. [Secrets & environment variables (full reference)](#3-secrets--environment-variables)
4. [Complete port matrix](#4-complete-port-matrix)
5. [Firewall: exactly which ports to open](#5-firewall-exactly-which-ports-to-open)
6. [Coturn STUN/TURN — when and how](#6-coturn-stunturn)
7. [Deployment A — Docker Compose (single host / LAN)](#7-deployment-a--docker-compose)
8. [Deployment B — Helm on Docker Desktop (validation)](#8-deployment-b--helm-on-docker-desktop)
9. [Deployment C — Helm on air-gapped k3s (production LAN)](#9-deployment-c--helm-on-air-gapped-k3s)
10. [URL reference per environment](#10-url-reference-per-environment)
11. [Verification & smoke tests](#11-verification--smoke-tests)
12. [Troubleshooting](#12-troubleshooting)

---

## 1. Architecture & traffic flow

```
                  PUBLISH (RTSP/TCP)                  VIEW (WebRTC/WHEP)
 ┌───────────┐    8554 / 30554       ┌─────────────┐                    ┌─────────┐
 │ publisher │ ───────────────────▶  │   ORIGIN    │                    │ browser │
 │ (ffmpeg)  │                       │  (3 pods)   │                    │ viewer  │
 └───────────┘                       │  records    │                    └────┬────┘
                                      │  fMP4       │                         │
                                      └──────┬──────┘            WHEP signaling│ (TCP 8889/3088x)
                  internal RTSP pull         │                                 │
                  (INTERNAL_SECRET)          ▼                                 ▼
                                      ┌─────────────┐   WebRTC media     ┌──────────┐
                                      │  READ tier  │ ◀────UDP 8189 ────▶ │  viewer  │
                                      │ (per-origin │   (TCP fallback)   │  + ICE   │
                                      │  replicas)  │                    └────┬─────┘
                                      └─────────────┘                         │
                                                                 STUN/TURN    │ (UDP/TCP 3478
                                                              ┌──────────┐    │  + relay range)
                                                              │  COTURN  │ ◀──┘
                                                              └──────────┘

 recordings ─▶ recording-uploader ─▶ MinIO (S3) + Postgres (metadata)
 api-server  ─▶ polls origin /v3/paths/list, serves /api/streams/live (SSE) + /api/recordings
 web-frontend ─▶ React UI; routes each stream's WHEP to the correct per-origin read tier
 prometheus  ─▶ scrapes origin+read :9998/metrics ;  grafana ─▶ dashboards
```

**Key routing fact (per-origin read tier):**
The publisher hashes each `stream_id` with FNV32a and assigns it to origin `hash % ORIGIN_COUNT`.
Read replicas are **paired to a single origin** — read tier `N` only pulls from origin `N`. The
API server reports each live stream's `origin_index`; the frontend uses that index to send the
WHEP request to the matching per-origin read Service. A viewer never lands on a read pod that
can't source their stream.

---

## 2. The golden rule

Two variables decide whether WebRTC actually plays. Get these wrong and you get a black video
pane with no error.

| Variable | What it is | Local Compose | LAN k3s |
|----------|-----------|---------------|---------|
| `WEBRTC_EXTERNAL_HOST` | IP the **browser** uses to reach the read tier for ICE candidates | `127.0.0.1` | k3s node LAN IP, e.g. `192.168.1.100` |
| `COTURN_EXTERNAL_IP` | IP the **browser** uses to reach Coturn | `127.0.0.1` | Coturn node LAN IP, e.g. `192.168.1.100` |

These must be reachable **from the browser machine**, not from inside the cluster. On a LAN this
is the server's LAN IP. The values are baked into the MediaMTX config via `envsubst` at container
start (`webrtcAdditionalHosts` and `webrtcICEServers2`).

> If browsers and server are the same machine → `127.0.0.1` works.
> If browsers are other machines on the LAN → use the server's LAN IP everywhere.

---

## 3. Secrets & environment variables

### 3.1 Shared secrets (identical meaning across all deployments)

| Variable | Used by | Purpose |
|----------|---------|---------|
| `PUBLISH_SECRET` | origin, publisher | RTSP publishers authenticate as user `publisher` |
| `INTERNAL_SECRET` | origin, read | Read tier pulls from origin as user `internal` |
| `VIEWER_SECRET` | read, web-frontend, grafana | WHEP viewers authenticate as user `viewer`; also Grafana admin password in Compose |
| `TURN_SECRET` | coturn, read | Coturn `--static-auth-secret`; MediaMTX TURN credential |
| `WEBRTC_EXTERNAL_HOST` | read | See [golden rule](#2-the-golden-rule) |
| `COTURN_EXTERNAL_IP` | coturn, read | See [golden rule](#2-the-golden-rule) |
| `MINIO_ROOT_USER` | minio, uploader, api-server | MinIO/S3 access key |
| `MINIO_ROOT_PASSWORD` | minio, uploader, api-server | MinIO/S3 secret key |
| `MINIO_BUCKET` | minio-init, uploader, api-server | Recordings bucket name (default `recordings`) |
| `POSTGRES_USER` / `POSTGRES_PASSWORD` / `POSTGRES_DB` | postgres, uploader, api-server | Metadata DB |

### 3.2 Docker Compose `.env`

Copy `.env.example` → `.env` (gitignored). Full contents:

```env
# MediaMTX credentials
PUBLISH_SECRET=changeme_publish
INTERNAL_SECRET=changeme_internal
VIEWER_SECRET=changeme_viewer

# WebRTC external host (127.0.0.1 local; LAN IP if viewers are remote)
WEBRTC_EXTERNAL_HOST=127.0.0.1

# Coturn STUN/TURN (127.0.0.1 local; server LAN/public IP in production)
COTURN_EXTERNAL_IP=127.0.0.1
TURN_SECRET=changeme_turn

# MinIO
MINIO_ROOT_USER=minioadmin
MINIO_ROOT_PASSWORD=changeme_minio
MINIO_BUCKET=recordings

# Postgres
POSTGRES_USER=superstreaming
POSTGRES_PASSWORD=changeme_postgres
POSTGRES_DB=superstreaming

# Origin count (Compose runs a single origin)
ORIGIN_COUNT=1
```

Optional Compose override:
- `PUBLISHER_STREAMS` — comma-separated `stream_id=source` pairs. Default is 10 synthetic test cams (`cam-01=test,…,cam-10=test`).

### 3.3 Helm `values.yaml` (`secrets:` block)

The Helm chart renders all of the above into a single Kubernetes `Secret` and injects it via
`envFrom: secretRef`. Override at install time or in a values file:

```yaml
secrets:
  publishSecret: "..."
  internalSecret: "..."
  viewerSecret: "..."
  turnSecret: "..."
  webrtcExternalHost: "192.168.1.100"   # k3s node LAN IP seen by browsers
  minioRootUser: "minioadmin"
  minioRootPassword: "..."
  minioBucket: "recordings"
  postgresUser: "superstreaming"
  postgresPassword: "..."
  postgresDB: "superstreaming"
```

> `coturn.externalIP` is a **separate** top-level value (not under `secrets:`) — set it to the
> Coturn node's LAN IP. `READ_BASE_URLS` for the frontend is **auto-derived** from
> `secrets.webrtcExternalHost` + `read.originNodePorts` — you do not set it manually.

### 3.4 Per-service runtime env (reference)

**api-server** (Compose values shown; Helm derives cluster FQDNs):
`LISTEN_ADDR=:8080`, `ORIGIN_COUNT`, `ORIGIN_HOST_TEMPLATE` (`mediamtx-origin-%d` in Compose / pod FQDN in k8s), `ORIGIN_API_PORT=9997`, `MINIO_ENDPOINT`, `MINIO_PUBLIC_ENDPOINT` (Compose only, `localhost:9000`), `POSTGRES_HOST`, `CORS_ORIGIN=*`, `PRESIGN_EXPIRY=1h`.

**web-frontend**: `MEDIAMTX_WHEP_BASE`, `READ_BASE_URLS` (k8s only, auto-derived), `API_SERVER_URL`, `VIEWER_PASSWORD`.

**publisher**: `ORIGIN_COUNT`, `ORIGIN_HOST_TEMPLATE`, `ORIGIN_RTSP_PORT=8554`, `ORIGIN_SRT_PORT=8890`, `PUBLISH_SECRET`, `STREAMS`, `SRT_LATENCY_MS=200`, `SRT_PASSPHRASE` (optional).

`STREAMS` entries may carry an `@srt` suffix to select SRT push for that stream: `cam-03=rtsp://far-cam/stream@srt`. No suffix = RTSP push (default).

---

## 4. Complete port matrix

### 4.1 Container / internal ports (constant in every environment)

| Service | Port | Proto | Purpose |
|---------|------|-------|---------|
| origin | 8554 | TCP | RTSP publish + internal pull |
| origin | 8890 | UDP | SRT ingest (far/WAN camera relay) |
| origin | 9997 | TCP | Control API (`/v3/paths/list`) |
| origin | 9998 | TCP | Prometheus metrics |
| read | 8889 | TCP | WebRTC WHEP signaling (HTTP) |
| read | 8189 | UDP | WebRTC media (primary path) |
| read | 8189 | TCP | WebRTC media (fallback) |
| read | 9997 | TCP | Control API |
| read | 9998 | TCP | Prometheus metrics |
| coturn | 3478 | UDP+TCP | STUN/TURN listener |
| coturn | 49152–49201 (Compose) / 49152–49251 (k8s) | UDP | TURN relay range |
| minio | 9000 | TCP | S3 API |
| minio | 9001 | TCP | Web console |
| postgres | 5432 | TCP | SQL |
| api-server | 8080 | TCP | REST + SSE |
| web-frontend | 80 | TCP | UI (nginx) |
| prometheus | 9090 | TCP | UI / query |
| grafana | 3000 | TCP | UI |

### 4.2 Docker Compose host port mappings

| Service | Host port | → Container | Proto |
|---------|-----------|-------------|-------|
| origin | **8554** | 8554 | TCP |
| origin | **8890** | 8890 | UDP |
| origin | **9997** | 9997 | TCP |
| coturn | **3478** | 3478 | UDP **and** TCP |
| coturn | **49152–49201** | 49152–49201 | UDP |
| read | **8889** | 8889 | TCP |
| read | **8189** | 8189 | UDP **and** TCP |
| read | **9998** | 9997 | TCP (host 9998 → container 9997) |
| minio | **9000**, **9001** | 9000, 9001 | TCP |
| postgres | **5432** | 5432 | TCP |
| api-server | **8080** | 8080 | TCP |
| web-frontend | **80** | 80 | TCP |
| prometheus | **9090** | 9090 | TCP |
| grafana | **3000** | 3000 | TCP |

### 4.3 Kubernetes Service types & NodePorts (from `values.yaml`)

| Service | k8s Service type | NodePort | Proto | Notes |
|---------|------------------|----------|-------|-------|
| origin RTSP publish | NodePort | **30554** | TCP | `origin-publisher` Service |
| origin API/metrics | Headless (ClusterIP None) | — | TCP | reach via `port-forward` |
| read origin-0 WHEP | NodePort | **30889** | TCP | per-origin Service |
| read origin-0 media UDP | NodePort | **30189** | UDP | |
| read origin-0 media TCP | NodePort | **30190** | TCP | fallback |
| read origin-1 WHEP | NodePort | **30890** | TCP | |
| read origin-1 media UDP | NodePort | **30191** | UDP | |
| read origin-1 media TCP | NodePort | **30192** | TCP | |
| read origin-2 WHEP | NodePort | **30891** | TCP | |
| read origin-2 media UDP | NodePort | **30193** | UDP | |
| read origin-2 media TCP | NodePort | **30194** | TCP | |
| coturn STUN/TURN | NodePort (+hostNetwork) | 30478 | UDP+TCP | **see note below** |
| coturn relay range | hostNetwork | 49152–49251 | UDP | bound directly to node NIC |
| api-server | NodePort | **30080** | TCP | |
| web-frontend | NodePort | **30000** | TCP | |
| minio S3 + console | ClusterIP | — | TCP | `port-forward` for console |
| postgres | ClusterIP | — | TCP | internal only |
| prometheus | **ClusterIP** | — | TCP | `port-forward` to view |
| grafana | **ClusterIP** | — | TCP | `port-forward` to view |

> ⚠️ **Prometheus & Grafana are ClusterIP-only in the Helm chart** — there is no NodePort.
> Access them with `kubectl port-forward`. (The legacy manual mentioned 30090/30300; those no
> longer exist.)
>
> ⚠️ **Coturn runs with `hostNetwork: true`.** The pod binds `3478` and the relay range
> **directly on the node's NIC**. Browsers reach Coturn at `COTURN_EXTERNAL_IP:3478` — the
> `30478` NodePort is effectively redundant under hostNetwork and is **not** the path clients
> use. Open **3478**, not 30478, on the firewall.

---

## 5. Firewall: exactly which ports to open

### 5.1 Docker Compose host (LAN, viewers on other machines)

Open inbound **to the host** on the LAN:

| Port | Proto | Why | Who connects |
|------|-------|-----|--------------|
| 80 | TCP | Web UI | viewers |
| 8080 | TCP | api-server (SSE + recordings) | viewers' browser |
| 8889 | TCP | WHEP signaling | viewers' browser |
| 8189 | UDP | WebRTC media (primary) | viewers' browser |
| 8189 | TCP | WebRTC media fallback | viewers' browser |
| 3478 | UDP+TCP | Coturn STUN/TURN | viewers' browser |
| 49152–49201 | UDP | Coturn TURN relay | viewers' browser |
| 8554 | TCP | RTSP publish | publishers (only if external) |
| 8890 | UDP | SRT ingest (WAN edge relay) | edge publisher relays (only if external far cameras) |

Internal-only (do **not** expose to untrusted networks): 9000/9001 (MinIO), 5432 (Postgres),
9090 (Prometheus), 3000 (Grafana), 9997/9998 (MediaMTX API/metrics). Keep these on localhost or a
management subnet.

### 5.2 k3s node (LAN production)

Open inbound **to the k3s node** LAN IP:

| Port(s) | Proto | Why |
|---------|-------|-----|
| 30000 | TCP | web-frontend |
| 30080 | TCP | api-server |
| 30889, 30890, 30891 | TCP | WHEP signaling (one per origin) |
| 30189, 30191, 30193 | UDP | WebRTC media (one per origin) |
| 30190, 30192, 30194 | TCP | WebRTC media fallback (one per origin) |
| 3478 | UDP+TCP | Coturn STUN/TURN (hostNetwork) |
| 49152–49251 | UDP | Coturn TURN relay range (hostNetwork) |
| 30554 | TCP | RTSP publish (only if publishers are external) |

Plus standard k3s ports between cluster nodes (6443 API server, 10250 kubelet, 8472/UDP flannel
VXLAN, 51820/UDP if WireGuard) — only relevant for multi-node clusters.

### 5.3 UFW example (single-node k3s, LAN viewers)

```bash
sudo ufw allow 30000/tcp                 # web UI
sudo ufw allow 30080/tcp                 # api
sudo ufw allow 30889:30891/tcp           # WHEP signaling
sudo ufw allow 30189:30194/udp           # WebRTC media UDP (covers 30189/91/93)
sudo ufw allow 30189:30194/tcp           # WebRTC media TCP fallback (30190/92/94)
sudo ufw allow 3478                      # STUN/TURN (udp+tcp)
sudo ufw allow 49152:49251/udp           # TURN relay range
sudo ufw allow 30554/tcp                 # RTSP publish (if external publishers)
```

> The `30189:30194` ranges over-allow a couple of ports — harmless, nothing listens on the gaps.
> Tighten to exact ports if your policy requires it.

---

## 6. Coturn STUN/TURN

### 6.1 When you need it

- **STUN only** (no Coturn / public STUN): enough when browser and server can exchange UDP
  directly — same LAN, no symmetric NAT, no restrictive firewall.
- **TURN (Coturn) required** when: strict corporate firewall blocks the WebRTC media UDP ports,
  symmetric NAT, or clients on a different network that can't reach the read tier's `8189/3018x`
  directly. TURN relays the media through Coturn over the open `3478` + relay range.

This platform ships Coturn enabled by default because the target environment has a **strict
firewall**.

### 6.2 How it's wired

MediaMTX read config (`webrtcICEServers2`) advertises both STUN and TURN to the browser:

```yaml
webrtcICEServers2:
  - url: stun:$COTURN_EXTERNAL_IP:3478
  - url: turn:$COTURN_EXTERNAL_IP:3478
    username: AUTH_SECRET          # literal marker — MediaMTX generates HMAC time-limited creds
    password: $TURN_SECRET         # must equal Coturn --static-auth-secret
```

Coturn runs with `--use-auth-secret --static-auth-secret=$TURN_SECRET`. The shared `TURN_SECRET`
is the only thing tying the two together — **they must match**.

### 6.3 Docker Compose

Already in `docker-compose.yml`. Set in `.env`:
```env
COTURN_EXTERNAL_IP=192.168.1.100   # server LAN IP (or 127.0.0.1 if same machine)
TURN_SECRET=<random-long-secret>
```

### 6.4 k3s (hostNetwork)

Coturn uses `hostNetwork: true` so it binds the relay range directly to the node NIC (avoids
mapping 100 NodePorts). It is pinned to a labeled node:

```bash
kubectl label node <node-name> superstreaming.io/coturn=true
```

In `values.yaml`:
```yaml
coturn:
  enabled: true
  externalIP: "192.168.1.100"      # LAN IP of the labeled node
  relayPortMin: 49152
  relayPortMax: 49251
  hostNetwork: true
  nodeSelectorKey: "superstreaming.io/coturn"
  nodeSelectorValue: "true"
```

> If you do **not** label a node, the Coturn pod stays `Pending`. Either label a node or set
> `coturn.enabled: false` (STUN-only via the still-advertised STUN URL will then point at a dead
> host — so only disable Coturn if you also remove the TURN entry, or accept direct-UDP-only).

---

## 7. Deployment A — Docker Compose

**Use for:** local dev, single-host LAN demo, full-system integration test on Windows.

### 7.1 Prerequisites
- Docker Desktop (Windows/Mac/Linux), PowerShell 5.1+ (Windows) or bash.

### 7.2 Configure
```powershell
Copy-Item .env.example .env
# Edit .env: replace every changeme_* ; set WEBRTC_EXTERNAL_HOST + COTURN_EXTERNAL_IP
#   - same machine viewers  → 127.0.0.1
#   - LAN viewers           → host LAN IP (e.g. 192.168.1.50)
```

### 7.3 Build & start
```powershell
docker compose build
docker compose up -d
docker compose ps        # wait for (healthy)
```

Expected healthy containers: `mediamtx-origin-0`, `coturn`, `mediamtx-read`, `minio`,
`minio-init` (exits 0), `recording-uploader`, `postgres`, `api-server`, `web-frontend`,
`publisher`, `prometheus`, `grafana`.

### 7.4 Access (same machine)

| What | URL |
|------|-----|
| Web UI | http://localhost/ |
| api-server health | http://localhost:8080/health |
| Live streams (SSE) | http://localhost:8080/api/streams/live |
| MinIO console | http://localhost:9001/ |
| Prometheus | http://localhost:9090/ |
| Grafana | http://localhost:3000/ (admin / `VIEWER_SECRET`) |
| RTSP publish target | `rtsp://publisher:<PUBLISH_SECRET>@localhost:8554/<stream-id>` |

LAN viewers replace `localhost` with the host LAN IP, and you **must** have set
`WEBRTC_EXTERNAL_HOST` / `COTURN_EXTERNAL_IP` to that same LAN IP before `up`.

### 7.5 Teardown
```powershell
docker compose down            # keep volumes
docker compose down -v         # also wipe recordings/minio/postgres data
```

---

## 8. Deployment B — Helm on Docker Desktop

**Use for:** validating the Helm chart and k8s manifests before shipping to k3s.

### 8.1 One-time cluster setup
1. Docker Desktop → Settings → Kubernetes → Enable → Apply & Restart.
2. Create the `local-path` StorageClass alias:
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

### 8.2 Build & make images available
Docker Desktop's k8s shares the local Docker image store, so a local `docker compose build`
(or `docker build`) is enough — set `pullPolicy: IfNotPresent` (already the default).

### 8.3 Local-validation values

Create `values.local.yaml` to scale down and disable Coturn's node pinning:

```yaml
origin:
  replicaCount: 1                  # Docker Desktop is single-node; 1 origin is enough to validate
read:
  minReplicas: 1
  maxReplicas: 2
secrets:
  webrtcExternalHost: "127.0.0.1"
coturn:
  enabled: false                   # skip hostNetwork/node-label requirement for local validation
```

> Validating the full 3-origin layout on a single Docker Desktop node works but spins up 3×(min
> replicas) read pods + 3 origin pods — heavy. Use `replicaCount: 1` unless you specifically want
> to see the per-origin routing render.

### 8.4 Install
```powershell
helm install superstreaming "D:\Software_Projects\SuperStreaming\helm\superstreaming" `
  --namespace superstreaming --create-namespace `
  -f "D:\Software_Projects\SuperStreaming\helm\superstreaming\values.local.yaml"

kubectl wait --for=condition=Ready pods --all -n superstreaming --timeout=180s
kubectl get pods -n superstreaming
```

### 8.5 Access (NodePorts don't reach Windows localhost reliably — use port-forward)
```powershell
kubectl port-forward svc/superstreaming-web-frontend 8088:80   -n superstreaming
kubectl port-forward svc/superstreaming-api-server   8080:8080 -n superstreaming
kubectl port-forward svc/prometheus                  9090:9090 -n superstreaming
kubectl port-forward svc/superstreaming-grafana      3000:3000 -n superstreaming
```

### 8.6 Teardown
```powershell
helm uninstall superstreaming -n superstreaming
kubectl delete namespace superstreaming
```

---

## 9. Deployment C — Helm on air-gapped k3s

**Use for:** production LAN, single or multi-node k3s, no internet.

### 9.1 Build & export images (on a connected build machine)
```powershell
docker compose build      # builds superstreaming/* images
docker save `
  superstreaming/mediamtx:local `
  superstreaming/api-server:latest `
  superstreaming/web-frontend:latest `
  superstreaming/publisher:latest `
  superstreaming/recording-uploader:latest `
  minio/minio:latest minio/mc:latest `
  postgres:16-alpine `
  coturn/coturn:latest `
  prom/prometheus:latest grafana/grafana:latest `
  | gzip > superstreaming-images.tar.gz
```

### 9.2 Transfer
```bash
scp superstreaming-images.tar.gz user@<node-ip>:~/
scp -r helm/ user@<node-ip>:~/superstreaming-helm/
```

### 9.3 Import into k3s containerd (on each node that schedules pods)
```bash
sudo k3s ctr images import ~/superstreaming-images.tar.gz
sudo k3s ctr images ls | grep superstreaming
```

> Air-gap note: set every image `pullPolicy: IfNotPresent` (chart default) so k3s never tries to
> pull from a registry. Verify imported image **names+tags exactly match** `values.yaml`.

### 9.4 Label the Coturn node
```bash
kubectl label node <node-name> superstreaming.io/coturn=true
```

### 9.5 Production values

Create `values.prod.yaml` (LAN node IP = `192.168.1.100` in this example):

```yaml
origin:
  replicaCount: 3
read:
  minReplicas: 2
  maxReplicas: 6
secrets:
  publishSecret: "<strong>"
  internalSecret: "<strong>"
  viewerSecret: "<strong>"
  turnSecret: "<strong>"
  webrtcExternalHost: "192.168.1.100"
  minioRootPassword: "<strong>"
  postgresPassword: "<strong>"
coturn:
  enabled: true
  externalIP: "192.168.1.100"
webFrontend:
  mediamtxWhepBase: "http://192.168.1.100:30889/"
  apiServerUrl: "http://192.168.1.100:30080"
```

> `READ_BASE_URLS` (the per-origin WHEP map) is generated automatically from
> `webrtcExternalHost` + `read.originNodePorts`:
> `http://192.168.1.100:30889/,http://192.168.1.100:30890/,http://192.168.1.100:30891/`.
> `mediamtxWhepBase` is the single-URL fallback (origin-0); both should point at the node IP.

### 9.6 Install
```bash
helm install superstreaming ~/superstreaming-helm/superstreaming \
  --namespace superstreaming --create-namespace \
  -f values.prod.yaml

kubectl wait --for=condition=Ready pods --all -n superstreaming --timeout=300s
kubectl get pods -n superstreaming -o wide
```

Expected pods: `*-origin-0/1/2`, `*-read-0/1/2-*` (≥2 each from HPA min), `*-coturn` (on labeled
node), `minio`, `postgres`, `*-api-server` (×2), `*-web-frontend` (×2), `*-publisher`,
`prometheus`, `grafana`.

### 9.7 Open firewall
See [§5.2 / §5.3](#5-firewall-exactly-which-ports-to-open).

### 9.8 Upgrade / rollback
```bash
helm upgrade superstreaming ~/superstreaming-helm/superstreaming -n superstreaming -f values.prod.yaml
helm rollback superstreaming -n superstreaming         # to previous revision
helm history superstreaming -n superstreaming
```

### 9.9 Teardown
```bash
helm uninstall superstreaming -n superstreaming
kubectl delete namespace superstreaming                # also removes PVCs/data
```

---

## 10. URL reference per environment

Replace `<NODE>` with the k3s node LAN IP (e.g. `192.168.1.100`).

| Purpose | Docker Compose | k3s (NodePort) | k3s (port-forward) |
|---------|----------------|----------------|--------------------|
| Web UI | http://localhost/ | http://\<NODE\>:30000/ | `pf svc/...-web-frontend 8088:80` |
| api-server | http://localhost:8080 | http://\<NODE\>:30080 | `pf svc/...-api-server 8080:8080` |
| WHEP origin-0 | http://localhost:8889/\<id\>/whep | http://\<NODE\>:30889/\<id\>/whep | — |
| WHEP origin-1 | (n/a, 1 origin) | http://\<NODE\>:30890/\<id\>/whep | — |
| WHEP origin-2 | (n/a, 1 origin) | http://\<NODE\>:30891/\<id\>/whep | — |
| RTSP publish | rtsp://...@localhost:8554/\<id\> | rtsp://...@\<NODE\>:30554/\<id\> | `pf svc/...-origin-headless 8554:8554` |
| STUN/TURN | \<COTURN_IP\>:3478 | \<NODE\>:3478 | — |
| MinIO console | http://localhost:9001 | — | `pf svc/minio 9001:9001` |
| Prometheus | http://localhost:9090 | — (ClusterIP) | `pf svc/prometheus 9090:9090` |
| Grafana | http://localhost:3000 | — (ClusterIP) | `pf svc/...-grafana 3000:3000` |
| Origin API | http://localhost:9997/v3/paths/list | — (headless) | `pf pod/...-origin-0 9997:9997` |
| Read API | http://localhost:9998/v3/paths/list | — | `pf deploy/...-read-0 9997:9997` |

(`pf` = `kubectl port-forward ... -n superstreaming`)

---

## 11. Verification & smoke tests

### 11.1 Docker Compose
```powershell
.\scripts\test-all-services.ps1          # health of all containers
.\scripts\test-streaming-path.ps1        # publish → origin → read → WHEP 201
.\scripts\test-recording-pipeline.ps1    # MinIO object + Postgres row + local file deleted
```

Manual end-to-end:
1. Open http://localhost/ → Live tab. The default publisher pushes `cam-01..cam-10`; they appear in the sidebar within ~15s.
2. Click a stream → video plays (WebRTC). If black with no error → check `WEBRTC_EXTERNAL_HOST`.
3. Wait ~65s, open Recordings tab → segments appear with playable presigned URLs.

### 11.2 k3s
```bash
kubectl get pods -n superstreaming
# WHEP reachability (origin-0 read tier) from a LAN machine:
curl -i http://<NODE>:30889/<stream-id>/whep -X OPTIONS
# api-server:
curl http://<NODE>:30080/health
```

Verify per-origin routing: the api-server SSE event now includes `origin_index` per stream:
```bash
curl -N http://<NODE>:30080/api/streams/live | head
# {"active":[{"id":"cam-01","origin_index":0}, ...], ...}
```
The frontend uses that index to pick `30889/30890/30891`. Confirm a stream assigned to origin-2
opens against `:30891` in the browser network tab.

### 11.3 Coturn relay check
```bash
# From a viewer machine, confirm STUN reachability:
nc -u -z -v <COTURN_IP> 3478
# In Chrome: chrome://webrtc-internals → check selected candidate pair is "relay" when UDP blocked.
```

---

## 12. Troubleshooting

| Symptom | Cause | Fix |
|---------|-------|-----|
| Black video, no error | `WEBRTC_EXTERNAL_HOST` wrong / unreachable from browser | Set to the IP the browser can reach; rebuild/restart read |
| WHEP 401 | Wrong `VIEWER_SECRET` | Match frontend `VIEWER_PASSWORD` to read `VIEWER_SECRET` |
| WHEP 400 `codecs not supported` | Source has AAC audio | Publish Opus (`-c:a libopus`); no transcoding in MediaMTX |
| WHEP 400 `failed to unmarshal SDP: EOF` | LF line endings in SDP | Use CRLF (`\r\n`) |
| Stream never plays, only on strict firewall | Media UDP blocked, TURN not working | Verify Coturn up, `TURN_SECRET` matches, port 3478 + relay range open |
| Viewer hits wrong read pod (no video for some streams) | `READ_BASE_URLS` / `origin_index` mismatch | Confirm frontend env has the 3-URL map; check api-server `ORIGIN_COUNT`=3 |
| Coturn pod `Pending` | No node labeled `superstreaming.io/coturn=true` | Label a node, or set `coturn.enabled:false` |
| Read pod CrashLoop `sourceOnDemand` | Regex path needs on-demand | `sourceOnDemand: true` (already in chart) |
| Origin pod `Pending` (PVC) | Provisioner lacks `ReadWriteOncePod` | StatefulSet uses `ReadWriteOnce` (already set); check StorageClass exists |
| MediaMTX `conf.alias` error | Wrong YAML types | `rtspEncryption: "no"` (quoted), booleans unquoted |
| NodePort unreachable on Docker Desktop | WSL2 doesn't forward NodePorts to Windows localhost | Use `kubectl port-forward` |
| Port bind forbidden (Windows) | Port in reserved range | `netsh int ipv4 show excludedportrange tcp`; pick another |
| Prometheus/Grafana not on NodePort | Chart exposes them as ClusterIP only | `kubectl port-forward` (see §10) |

---

---

## 13. SRT edge relay — far/WAN cameras

Use when a camera is on a remote site and cannot push RTSP/TCP into the AG network reliably. SRT provides loss-tolerant, encrypted transport over a single UDP port.

### 13.1 Architecture

```
[far camera]
     | RTSP/TCP (LAN — near camera)
     v
[edge publisher relay]  ← same binary, different config
     | SRT/UDP (WAN — single port, AES encrypted, latency-budget ARQ)
     v
[AG firewall: UDP 8890 inbound from edge relay IP]
     v
[mediamtx-origin SRT listener :8890]
     | RTSP/TCP (intra-cluster, unchanged)
     v
[mediamtx-read] ──WHEP/WebRTC──> [browsers]
```

The edge relay is the same `publisher` binary deployed near the camera. Internal cluster traffic is unaffected.

### 13.2 Edge relay configuration

```env
STREAMS=cam-id=rtsp://camera-local-ip/stream@srt
ORIGIN_COUNT=<same as AG origin count>
ORIGIN_HOST_TEMPLATE=<AG origin hostname or IP>
ORIGIN_SRT_PORT=8890
PUBLISH_SECRET=<same PUBLISH_SECRET as on the AG origin>
SRT_LATENCY_MS=<4 × RTT in ms — see below>
SRT_PASSPHRASE=<shared AES key — recommended>
```

The publisher connects **outbound** to UDP 8890. The AG firewall needs one rule: `UDP 8890 inbound from <edge-relay-IP>`. No dynamic port ranges.

### 13.3 Tuning SRT_LATENCY_MS

Rule of thumb: `SRT_LATENCY_MS = RTT × 4`. Measure RTT with `ping <AG-host-IP>` from the edge relay.

| RTT to AG host | Recommended SRT_LATENCY_MS |
|----------------|---------------------------|
| < 20 ms | 100 |
| 20–50 ms | 200 (default) |
| 50–100 ms | 400 |
| 100–200 ms | 800 |
| > 200 ms | 1000+ |

### 13.4 Mixing local and far cameras

```env
STREAMS=cam-01=test,cam-02=rtsp://local-cam/stream,cam-03=rtsp://far-cam/stream@srt
```

`cam-01` and `cam-02` push via RTSP/TCP. `cam-03` pushes via SRT. Only `cam-03`'s traffic crosses the WAN.

### 13.5 Firewall rules

```bash
# Compose host or k3s node — restrict to known edge relay IP
sudo ufw allow from <edge-relay-IP> to any port 8890 proto udp
```

### 13.6 Verify SRT connection

```bash
# Origin API should show the stream active
curl http://localhost:9997/v3/paths/list | python3 -m json.tool | grep cam-id

# Edge relay publisher logs should show:
# relay starting  stream=cam-id  source=rtsp://...  target=srt://...
```

---

## Appendix — quick command cheat sheet

```bash
# Compose
docker compose up -d ; docker compose ps ; docker compose logs -f mediamtx-read
docker compose down [-v]

# Helm
helm lint helm/superstreaming
helm template superstreaming helm/superstreaming -f values.prod.yaml | less
helm install   superstreaming helm/superstreaming -n superstreaming --create-namespace -f values.prod.yaml
helm upgrade   superstreaming helm/superstreaming -n superstreaming -f values.prod.yaml
helm uninstall superstreaming -n superstreaming

# k8s inspection
kubectl get pods,svc,hpa -n superstreaming
kubectl logs -f deploy/superstreaming-read-0 -n superstreaming
kubectl describe pod <pod> -n superstreaming
kubectl label node <node> superstreaming.io/coturn=true
```
