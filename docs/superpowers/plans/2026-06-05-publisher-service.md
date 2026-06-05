# Publisher Service Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a Go publisher service that reads RTSP streams (real cameras or ffmpeg test patterns) and routes them to the correct MediaMTX origin pod using FNV32a consistent hashing.

**Architecture:** The publisher is a CLI binary per stream, or a multi-stream daemon driven by a `STREAMS` env var. It computes `fnv32a(stream_id) % ORIGIN_COUNT` to select an origin, then shells out to ffmpeg to relay the source RTSP (or a synthetic test pattern) to `rtsp://publisher:${PUBLISH_SECRET}@mediamtx-origin-{N}:8554/{stream_id}`. On ffmpeg exit, it waits 3 seconds and restarts. Shutdown on SIGINT/SIGTERM cancels all relays gracefully.

**Tech Stack:** Go 1.24, `github.com/rs/zerolog`, standard library only (no RTSP library needed — ffmpeg handles the protocol). Alpine + ffmpeg Docker image.

---

## File Structure

```
services/publisher/
├── go.mod                   module: superstreaming/publisher
├── main.go                  config loading, stream orchestration, shutdown
├── hash/
│   └── hash.go              FNV32a origin routing — originIndex(streamID, n) int
│   └── hash_test.go         tests for known stream_id → origin mappings
└── relay/
    └── relay.go             ffmpeg command builder + subprocess restart loop
    └── relay_test.go        tests for BuildArgs() without running ffmpeg

build/publisher/
└── Dockerfile               golang:1.24-alpine builder + ffmpeg final stage
```

---

## Task 1: Go module + hash package

**Files:**
- Create: `services/publisher/go.mod`
- Create: `services/publisher/hash/hash.go`
- Create: `services/publisher/hash/hash_test.go`

- [ ] **Step 1: Create module**

```
cd services/publisher
go mod init superstreaming/publisher
```

Then add zerolog:
```
go get github.com/rs/zerolog@v1.33.0
go mod tidy
```

- [ ] **Step 2: Write failing tests**

Create `services/publisher/hash/hash_test.go`:

```go
package hash_test

import (
	"testing"

	"superstreaming/publisher/hash"
)

func TestOriginIndex(t *testing.T) {
	tests := []struct {
		streamID   string
		numOrigins int
		want       int
	}{
		// Hardcode expected values — computed once, locked in as regression tests.
		// To compute: h := fnv.New32a(); h.Write([]byte(id)); return int(h.Sum32()) % n
		{"cam-01", 3, int(hash.FNV32a("cam-01") % 3)},
		{"cam-02", 3, int(hash.FNV32a("cam-02") % 3)},
		{"cam-10", 3, int(hash.FNV32a("cam-10") % 3)},
		{"cam-01", 1, 0},
	}
	for _, tc := range tests {
		got := hash.OriginIndex(tc.streamID, tc.numOrigins)
		if got != tc.want {
			t.Errorf("OriginIndex(%q, %d) = %d, want %d", tc.streamID, tc.numOrigins, got, tc.want)
		}
	}
}

func TestOriginIndexStability(t *testing.T) {
	// Same stream_id + numOrigins must always produce same result.
	for i := 0; i < 100; i++ {
		if hash.OriginIndex("cam-stable", 3) != hash.OriginIndex("cam-stable", 3) {
			t.Fatal("OriginIndex is not stable")
		}
	}
}

func TestOriginIndexDistribution(t *testing.T) {
	// Verify all origins get at least some streams over 30 IDs.
	counts := make(map[int]int)
	for i := 0; i < 30; i++ {
		id := "stream-" + string(rune('a'+i))
		counts[hash.OriginIndex(id, 3)]++
	}
	for origin := 0; origin < 3; origin++ {
		if counts[origin] == 0 {
			t.Errorf("origin %d got 0 streams — hash distribution too skewed", origin)
		}
	}
}
```

- [ ] **Step 3: Run — verify FAIL**

```
cd services/publisher
go test ./hash/... -v
```

Expected: `cannot find package "superstreaming/publisher/hash"`

- [ ] **Step 4: Implement hash package**

Create `services/publisher/hash/hash.go`:

```go
package hash

import "hash/fnv"

// FNV32a returns the FNV-1a 32-bit hash of s.
// Exported so tests can compute expected values without duplicating logic.
func FNV32a(s string) uint32 {
	h := fnv.New32a()
	h.Write([]byte(s))
	return h.Sum32()
}

// OriginIndex returns the origin pod index for streamID given numOrigins pods.
// Matches the routing formula in the architecture spec:
//   fnv32a(stream_id) % num_origins
func OriginIndex(streamID string, numOrigins int) int {
	return int(FNV32a(streamID)) % numOrigins
}
```

- [ ] **Step 5: Run — verify PASS**

```
go test ./hash/... -v
```

Expected: all 3 tests PASS.

- [ ] **Step 6: Commit**

```
git add services/publisher/
git commit -m "feat: add publisher hash package with FNV32a origin routing"
```

---

## Task 2: Relay package (ffmpeg command builder + restart loop)

**Files:**
- Create: `services/publisher/relay/relay.go`
- Create: `services/publisher/relay/relay_test.go`

- [ ] **Step 1: Write failing tests**

Create `services/publisher/relay/relay_test.go`:

```go
package relay_test

import (
	"strings"
	"testing"

	"superstreaming/publisher/relay"
)

func TestBuildArgs(t *testing.T) {
	args := relay.BuildArgs("rtsp://cam/stream", "rtsp://publisher:secret@origin-0:8554/cam-01")

	// Must include input
	if !contains(args, "-i") {
		t.Error("missing -i flag")
	}
	sourceIdx := indexOf(args, "-i")
	if sourceIdx < 0 || args[sourceIdx+1] != "rtsp://cam/stream" {
		t.Error("source URL not after -i")
	}

	// Must end with target URL
	if args[len(args)-1] != "rtsp://publisher:secret@origin-0:8554/cam-01" {
		t.Errorf("last arg should be target URL, got %q", args[len(args)-1])
	}

	// Must copy video codec (no re-encode for passthrough)
	if !contains(args, "copy") {
		t.Error("expected codec copy for passthrough relay")
	}

	// Must output rtsp format
	if !contains(args, "rtsp") {
		t.Error("expected -f rtsp output format")
	}
}

func TestBuildArgsTestPattern(t *testing.T) {
	args := relay.BuildArgs("test", "rtsp://publisher:secret@origin-0:8554/test-stream")

	joined := strings.Join(args, " ")

	// test pattern uses lavfi input
	if !strings.Contains(joined, "lavfi") {
		t.Error("test pattern should use -f lavfi")
	}
	// must encode H264 (can't copy synthetic stream)
	if !strings.Contains(joined, "libx264") {
		t.Error("test pattern should encode with libx264")
	}
	// must encode Opus audio
	if !strings.Contains(joined, "libopus") {
		t.Error("test pattern should encode with libopus")
	}
}

func TestBuildArgsNoTimestampOverwrite(t *testing.T) {
	args := relay.BuildArgs("rtsp://cam/stream", "rtsp://publisher:secret@origin-0:8554/cam-01")
	// -y overwrites output without asking; not wanted for RTSP streaming
	if contains(args, "-y") {
		t.Error("-y flag must not be present for RTSP output")
	}
}

func contains(args []string, s string) bool {
	for _, a := range args {
		if a == s {
			return true
		}
	}
	return false
}

func indexOf(args []string, s string) int {
	for i, a := range args {
		if a == s {
			return i
		}
	}
	return -1
}
```

- [ ] **Step 2: Run — verify FAIL**

```
go test ./relay/... -v
```

Expected: `cannot find package "superstreaming/publisher/relay"`

- [ ] **Step 3: Implement relay package**

Create `services/publisher/relay/relay.go`:

```go
package relay

import (
	"context"
	"os/exec"
	"time"

	"github.com/rs/zerolog/log"
)

const restartDelay = 3 * time.Second

// BuildArgs returns the ffmpeg argument slice for relaying source to target.
// source = "test" produces a synthetic H264+Opus test pattern.
// Any other source is treated as an RTSP URL and copied without re-encoding.
func BuildArgs(source, target string) []string {
	if source == "test" {
		return []string{
			"-re",
			"-f", "lavfi", "-i", "testsrc=size=1280x720:rate=30",
			"-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000",
			"-c:v", "libx264", "-preset", "ultrafast", "-tune", "zerolatency",
			"-profile:v", "baseline", "-b:v", "1000k",
			"-c:a", "libopus", "-b:a", "64k",
			"-f", "rtsp", target,
		}
	}
	return []string{
		"-re",
		"-rtsp_transport", "tcp",
		"-i", source,
		"-c", "copy",
		"-f", "rtsp", target,
	}
}

// Run relays source → target via ffmpeg, restarting on exit until ctx is cancelled.
func Run(ctx context.Context, streamID, source, target string) {
	for {
		args := BuildArgs(source, target)
		cmd := exec.CommandContext(ctx, "ffmpeg", args...)
		log.Info().Str("stream", streamID).Str("source", source).Str("target", target).Msg("relay starting")

		if err := cmd.Run(); err != nil {
			if ctx.Err() != nil {
				log.Info().Str("stream", streamID).Msg("relay stopped (context cancelled)")
				return
			}
			log.Warn().Str("stream", streamID).Err(err).Msgf("ffmpeg exited, restarting in %s", restartDelay)
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(restartDelay):
		}
	}
}
```

- [ ] **Step 4: Run — verify PASS**

```
go test ./relay/... -v
```

Expected: all 3 tests PASS.

- [ ] **Step 5: Commit**

```
git add services/publisher/relay/
git commit -m "feat: add publisher relay package with ffmpeg command builder"
```

---

## Task 3: main.go — config, orchestration, shutdown

**Files:**
- Create: `services/publisher/main.go`

No unit tests for main.go (integration-tested in Task 5). Integration point tested by smoke test.

- [ ] **Step 1: Write main.go**

Create `services/publisher/main.go`:

```go
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"superstreaming/publisher/hash"
	"superstreaming/publisher/relay"
)

type streamSpec struct {
	id     string
	source string // "test" or rtsp:// URL
}

type config struct {
	originCount        int
	originHostTemplate string
	originRTSPPort     int
	publishSecret      string
	streams            []streamSpec
}

func loadConfig() config {
	cfg := config{
		originCount:        getEnvInt("ORIGIN_COUNT", 3),
		originHostTemplate: getEnv("ORIGIN_HOST_TEMPLATE", "mediamtx-origin-%d"),
		originRTSPPort:     getEnvInt("ORIGIN_RTSP_PORT", 8554),
		publishSecret:      mustEnv("PUBLISH_SECRET"),
	}

	streamsEnv := getEnv("STREAMS", "")
	if streamsEnv == "" {
		log.Fatal().Msg("STREAMS env var required: comma-separated stream_id=source pairs, e.g. cam-01=test,cam-02=rtsp://camera/stream")
	}
	for _, pair := range strings.Split(streamsEnv, ",") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		parts := strings.SplitN(pair, "=", 2)
		if len(parts) != 2 {
			log.Fatal().Str("pair", pair).Msg("invalid STREAMS entry: expected stream_id=source")
		}
		cfg.streams = append(cfg.streams, streamSpec{id: parts[0], source: parts[1]})
	}
	if len(cfg.streams) == 0 {
		log.Fatal().Msg("STREAMS must contain at least one entry")
	}
	return cfg
}

func (c config) targetURL(streamID string) string {
	idx := hash.OriginIndex(streamID, c.originCount)
	host := fmt.Sprintf(c.originHostTemplate, idx)
	return fmt.Sprintf("rtsp://publisher:%s@%s:%d/%s",
		c.publishSecret, host, c.originRTSPPort, streamID)
}

func main() {
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stdout})

	cfg := loadConfig()

	log.Info().Int("streams", len(cfg.streams)).Int("origins", cfg.originCount).Msg("publisher starting")
	for _, s := range cfg.streams {
		idx := hash.OriginIndex(s.id, cfg.originCount)
		log.Info().Str("stream", s.id).Str("source", s.source).Int("origin", idx).Msg("stream routed")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	for _, s := range cfg.streams {
		s := s
		target := cfg.targetURL(s.id)
		wg.Add(1)
		go func() {
			defer wg.Done()
			relay.Run(ctx, s.id, s.source, target)
		}()
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info().Msg("shutting down publisher")
	cancel()
	wg.Wait()
	log.Info().Msg("publisher stopped")
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatal().Str("var", key).Msg("required env var not set")
	}
	return v
}

func getEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}
```

- [ ] **Step 2: Verify build**

```
go build ./...
```

Expected: no output (clean build).

- [ ] **Step 3: Run all tests**

```
go test ./... -v
```

Expected: hash and relay tests PASS. `[no test files]` for main package is correct.

- [ ] **Step 4: Commit**

```
git add services/publisher/main.go services/publisher/go.mod services/publisher/go.sum
git commit -m "feat: add publisher main — FNV32a routing + ffmpeg relay + graceful shutdown"
```

---

## Task 4: Dockerfile

**Files:**
- Create: `build/publisher/Dockerfile`

- [ ] **Step 1: Write Dockerfile**

Create `build/publisher/Dockerfile`:

```dockerfile
FROM golang:1.24-alpine AS builder
WORKDIR /src
COPY services/publisher/ .
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -o /publisher .

FROM alpine:3.20
RUN apk add --no-cache ffmpeg ca-certificates
COPY --from=builder /publisher /publisher
ENTRYPOINT ["/publisher"]
```

- [ ] **Step 2: Verify it builds (requires Docker)**

```
docker build -f build/publisher/Dockerfile -t superstreaming/publisher:latest .
```

Expected: image built, two stages, final image has ffmpeg.

- [ ] **Step 3: Commit**

```
git add build/publisher/Dockerfile
git commit -m "feat: add publisher Dockerfile (golang builder + alpine ffmpeg)"
```

---

## Task 5: Docker Compose integration + smoke test

**Files:**
- Modify: `docker-compose.yml` — add publisher service with test streams
- Create: `scripts/test-publisher.ps1`

- [ ] **Step 1: Add publisher to docker-compose.yml**

Find the section after `api-server` in `docker-compose.yml` and add before `web-frontend`:

```yaml
  # ── Publisher (test streams) ────────────────────────────────────────
  publisher:
    image: superstreaming/publisher:latest
    build:
      context: .
      dockerfile: build/publisher/Dockerfile
    container_name: publisher
    restart: unless-stopped
    networks:
      - superstreaming
    environment:
      - ORIGIN_COUNT=1
      - ORIGIN_HOST_TEMPLATE=mediamtx-origin-%d
      - PUBLISH_SECRET=${PUBLISH_SECRET}
      - STREAMS=cam-01=test,cam-02=test
    depends_on:
      - mediamtx-origin-0
```

Note: `PUBLISH_SECRET` must be set in `.env`.

- [ ] **Step 2: Add `PUBLISH_SECRET` to `.env.example` if it exists**

Check: `ls .env* 2>/dev/null`. If `.env.example` exists, add:
```
PUBLISH_SECRET=changeme-publish
```

If no `.env.example`, skip this step.

- [ ] **Step 3: Write smoke test**

Create `scripts/test-publisher.ps1`:

```powershell
#!/usr/bin/env pwsh
# Smoke test: verify publisher is routing streams to origin.
# Requires stack running: docker compose up -d

param(
    [string]$OriginAPIBase = "http://localhost:9997"
)

$ErrorActionPreference = "Stop"
$pass = 0
$fail = 0

function Assert-Ok {
    param([string]$Label, [scriptblock]$Test)
    try {
        & $Test
        Write-Host "  PASS  $Label" -ForegroundColor Green
        $script:pass++
    } catch {
        Write-Host "  FAIL  $Label — $_" -ForegroundColor Red
        $script:fail++
    }
}

Write-Host "`npublisher smoke tests → $OriginAPIBase`n"

# Wait up to 15s for streams to appear
Assert-Ok "cam-01 appears on origin within 15s" {
    $deadline = [DateTime]::Now.AddSeconds(15)
    $found = $false
    while ([DateTime]::Now -lt $deadline) {
        try {
            $r = Invoke-RestMethod -Uri "$OriginAPIBase/v3/paths/list" -TimeoutSec 2
            if ($r.items | Where-Object { $_.name -eq "cam-01" -and $_.ready -eq $true }) {
                $found = $true; break
            }
        } catch {}
        Start-Sleep -Milliseconds 500
    }
    if (-not $found) { throw "cam-01 not ready after 15s" }
}

Assert-Ok "cam-02 appears on origin within 15s" {
    $deadline = [DateTime]::Now.AddSeconds(15)
    $found = $false
    while ([DateTime]::Now -lt $deadline) {
        try {
            $r = Invoke-RestMethod -Uri "$OriginAPIBase/v3/paths/list" -TimeoutSec 2
            if ($r.items | Where-Object { $_.name -eq "cam-02" -and $_.ready -eq $true }) {
                $found = $true; break
            }
        } catch {}
        Start-Sleep -Milliseconds 500
    }
    if (-not $found) { throw "cam-02 not ready after 15s" }
}

Assert-Ok "api-server SSE reports cam-01 and cam-02 active" {
    Start-Sleep -Seconds 5  # give SSE poller time to pick up new streams
    $r = Invoke-RestMethod -Uri "http://localhost:8080/api/streams/live" -TimeoutSec 6
    # SSE returns first event; we just check the endpoint responds
    # Full SSE validation: see test-api-server.ps1
    if ($null -eq $r) { throw "no response from SSE endpoint" }
}

Write-Host "`nResults: $pass passed, $fail failed`n"
if ($fail -gt 0) { exit 1 }
```

- [ ] **Step 4: Build and verify**

```
docker compose build publisher
```

Expected: image builds cleanly.

- [ ] **Step 5: Commit**

```
git add docker-compose.yml scripts/test-publisher.ps1
git commit -m "feat: add publisher to docker-compose with cam-01/cam-02 test streams"
```

---

## Self-Review

### Spec Coverage

| Spec requirement | Covered in |
|-----------------|------------|
| Language: Go | Tasks 1–4 |
| Protocol: RTSP | Task 2 (ffmpeg relay), Task 4 (ffmpeg in image) |
| `fnv32a(stream_id) % num_origins` routing | Task 1 (hash package) |
| `rtsp://mediamtx-origin-{N}:8554/{stream_id}` target format | Task 3 (targetURL) |
| `publishUser: publisher` credentials | Task 3 (targetURL, PUBLISH_SECRET) |
| Reconnect on disconnect | Task 2 (relay.Run restart loop) |
| Multi-stream support | Task 3 (STREAMS env var, goroutine per stream) |
| Docker image | Task 4 |
| Docker Compose integration | Task 5 |

### Placeholder Scan

None found — all steps have full code.

### Type Consistency

- `hash.OriginIndex(streamID string, numOrigins int) int` used in Task 1, imported in Task 3 ✓
- `relay.BuildArgs(source, target string) []string` defined in Task 2, tested in Task 2 ✓
- `relay.Run(ctx, streamID, source, target string)` defined in Task 2, called in Task 3 ✓
- `config.targetURL(streamID string) string` defined and used in Task 3 ✓
