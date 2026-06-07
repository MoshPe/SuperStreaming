# SRT Ingest Support Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add per-stream SRT push transport to the publisher so far/WAN cameras relay video to the AG MediaMTX origin over a single encrypted UDP port instead of RTSP/TCP.

**Architecture:** Each stream spec may carry an `@srt` suffix (`cam-03=rtsp://far-cam/stream@srt`) selecting SRT for the push leg only; the pull leg (publisher→camera) stays RTSP/TCP. SRT target URLs embed MediaMTX internal auth via the `streamid` query field so no new auth users are needed. All relay paths (both RTSP and SRT) disable audio (`-an`); SRT paths use MPEG-TS container.

**Tech Stack:** Go 1.25, ffmpeg (Alpine apk), MediaMTX, Docker Compose YAML.

**Spec:** `docs/superpowers/specs/2026-06-07-srt-ingest-design.md`

---

## File Map

| File | Change |
|------|--------|
| `services/publisher/relay/relay_test.go` | Add `-an` assertions; add SRT target tests; remove `libopus` assertion |
| `services/publisher/relay/relay.go` | Strip audio (`-an`); detect `srt://` target → `mpegts` format |
| `services/publisher/main_test.go` | New — test `parseStreamEntry` and `config.targetURL` |
| `services/publisher/main.go` | Add `transport` to `streamSpec`; extract `parseStreamEntry`; add SRT config fields; update `targetURL` signature |
| `configs/mediamtx/origin.yml` | Enable `srt: true` on `:8890` |
| `docker-compose.yml` | Expose `8890:8890/udp` on origin; add SRT env vars to publisher |
| `.env.example` | Add SRT vars with comments |
| `docs/deployment-guide.md` | Add SRT to port matrix, firewall tables, publisher env section, and new SRT edge relay section |

---

## Task 1: Update relay tests and implementation

**Files:**
- Modify: `services/publisher/relay/relay_test.go`
- Modify: `services/publisher/relay/relay.go`

- [ ] **Step 1: Write failing tests**

Replace the full contents of `services/publisher/relay/relay_test.go`:

```go
package relay_test

import (
	"strings"
	"testing"

	"superstreaming/publisher/relay"
)

func TestBuildArgs(t *testing.T) {
	args := relay.BuildArgs("rtsp://cam/stream", "rtsp://publisher:secret@origin-0:8554/cam-01")

	if !contains(args, "-i") {
		t.Error("missing -i flag")
	}
	sourceIdx := indexOf(args, "-i")
	if sourceIdx < 0 || args[sourceIdx+1] != "rtsp://cam/stream" {
		t.Error("source URL not after -i")
	}
	if args[len(args)-1] != "rtsp://publisher:secret@origin-0:8554/cam-01" {
		t.Errorf("last arg should be target URL, got %q", args[len(args)-1])
	}
	if !contains(args, "copy") {
		t.Error("expected codec copy for passthrough relay")
	}
	if !contains(args, "rtsp") {
		t.Error("expected -f rtsp output format")
	}
	if !contains(args, "-an") {
		t.Error("expected -an to disable audio")
	}
}

func TestBuildArgsTestPattern(t *testing.T) {
	args := relay.BuildArgs("test", "rtsp://publisher:secret@origin-0:8554/test-stream")
	joined := strings.Join(args, " ")

	if !strings.Contains(joined, "lavfi") {
		t.Error("test pattern should use -f lavfi")
	}
	if !strings.Contains(joined, "libx264") {
		t.Error("test pattern should encode with libx264")
	}
	if strings.Contains(joined, "libopus") {
		t.Error("test pattern must not include libopus (audio disabled)")
	}
	if strings.Contains(joined, "sine") {
		t.Error("test pattern must not include sine audio source")
	}
	if !contains(args, "-an") {
		t.Error("test pattern should disable audio with -an")
	}
}

func TestBuildArgsTestPatternUsesShortKeyframeInterval(t *testing.T) {
	args := relay.BuildArgs("test", "rtsp://publisher:secret@origin-0:8554/test-stream")

	gopIdx := indexOf(args, "-g")
	if gopIdx < 0 || gopIdx+1 >= len(args) {
		t.Fatal("test pattern should set GOP size with -g")
	}
	if args[gopIdx+1] != "30" {
		t.Fatalf("test pattern GOP should be 30 frames, got %q", args[gopIdx+1])
	}
	keyintIdx := indexOf(args, "-keyint_min")
	if keyintIdx < 0 || keyintIdx+1 >= len(args) {
		t.Fatal("test pattern should set keyint_min with -keyint_min")
	}
	if args[keyintIdx+1] != "30" {
		t.Fatalf("test pattern keyint_min should be 30, got %q", args[keyintIdx+1])
	}
}

func TestBuildArgsNoTimestampOverwrite(t *testing.T) {
	args := relay.BuildArgs("rtsp://cam/stream", "rtsp://publisher:secret@origin-0:8554/cam-01")
	if contains(args, "-y") {
		t.Error("-y flag must not be present")
	}
}

func TestBuildArgsSRTTarget(t *testing.T) {
	target := "srt://mediamtx-origin-0:8890?streamid=publish:cam-01:publisher:secret&latency=200000"
	args := relay.BuildArgs("rtsp://cam/stream", target)

	if args[len(args)-1] != target {
		t.Errorf("last arg should be target URL, got %q", args[len(args)-1])
	}
	if !contains(args, "mpegts") {
		t.Error("SRT target requires -f mpegts")
	}
	if contains(args, "rtsp") {
		t.Error("SRT target must not use -f rtsp")
	}
	if !contains(args, "-an") {
		t.Error("expected -an to disable audio")
	}
	if !contains(args, "copy") {
		t.Error("expected video copy for passthrough relay")
	}
}

func TestBuildArgsTestPatternSRTTarget(t *testing.T) {
	target := "srt://mediamtx-origin-0:8890?streamid=publish:test-cam:publisher:secret&latency=200000"
	args := relay.BuildArgs("test", target)
	joined := strings.Join(args, " ")

	if !contains(args, "mpegts") {
		t.Error("SRT target requires -f mpegts")
	}
	if !strings.Contains(joined, "libx264") {
		t.Error("test pattern + SRT should encode with libx264")
	}
	if !contains(args, "-an") {
		t.Error("test pattern + SRT should disable audio with -an")
	}
	if args[len(args)-1] != target {
		t.Errorf("last arg should be target URL, got %q", args[len(args)-1])
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

- [ ] **Step 2: Run tests — verify they fail**

```
cd services/publisher && go test ./relay/... -v
```

Expected failures:
- `TestBuildArgs` — FAIL: `-an` not present
- `TestBuildArgsTestPattern` — FAIL: `libopus` present, `-an` absent
- `TestBuildArgsSRTTarget` — FAIL: `mpegts` not present
- `TestBuildArgsTestPatternSRTTarget` — FAIL: `mpegts` not present

- [ ] **Step 3: Implement updated relay.go**

Replace the full contents of `services/publisher/relay/relay.go`:

```go
package relay

import (
	"context"
	"os/exec"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
)

const restartDelay = 3 * time.Second

// BuildArgs returns the ffmpeg argument slice for relaying source to target.
// source = "test" produces a synthetic H264-only test pattern.
// Any other source is treated as an RTSP URL and copied without re-encoding.
// Audio is always disabled (-an). If target starts with "srt://", the output
// container is mpegts; otherwise rtsp.
func BuildArgs(source, target string) []string {
	isSRT := strings.HasPrefix(target, "srt://")
	outFormat := "rtsp"
	if isSRT {
		outFormat = "mpegts"
	}

	if source == "test" {
		return []string{
			"-re",
			"-f", "lavfi", "-i", "testsrc=size=1280x720:rate=30",
			"-c:v", "libx264", "-preset", "ultrafast", "-tune", "zerolatency",
			"-profile:v", "baseline", "-pix_fmt", "yuv420p", "-b:v", "1000k",
			"-g", "30", "-keyint_min", "30", "-sc_threshold", "0",
			"-an",
			"-f", outFormat, target,
		}
	}
	return []string{
		"-re",
		"-rtsp_transport", "tcp",
		"-i", source,
		"-c:v", "copy",
		"-an",
		"-f", outFormat, target,
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

- [ ] **Step 4: Run tests — verify all pass**

```
cd services/publisher && go test ./relay/... -v
```

Expected: all 6 tests PASS.

- [ ] **Step 5: Commit**

```
cd services/publisher && git add relay/relay.go relay/relay_test.go
git commit -m "feat(publisher): strip audio and add SRT target support in relay"
```

---

## Task 2: Update main.go — @srt parsing, SRT config, SRT URL builder

**Files:**
- Create: `services/publisher/main_test.go`
- Modify: `services/publisher/main.go`

- [ ] **Step 1: Write failing tests**

Create `services/publisher/main_test.go`:

```go
package main

import (
	"strings"
	"testing"
)

func TestParseStreamEntry(t *testing.T) {
	tests := []struct {
		pair      string
		wantID    string
		wantSrc   string
		wantTrans string
	}{
		{"cam-01=test", "cam-01", "test", "rtsp"},
		{"cam-02=rtsp://cam/stream", "cam-02", "rtsp://cam/stream", "rtsp"},
		{"cam-03=rtsp://far-cam/stream@srt", "cam-03", "rtsp://far-cam/stream", "srt"},
		{"cam-04=test@srt", "cam-04", "test", "srt"},
		// RTSP URL with auth @ — suffix check must only strip @srt at end
		{"cam-05=rtsp://user:pass@cam/stream@srt", "cam-05", "rtsp://user:pass@cam/stream", "srt"},
		{"cam-06=rtsp://user:pass@cam/stream", "cam-06", "rtsp://user:pass@cam/stream", "rtsp"},
	}
	for _, tc := range tests {
		spec, err := parseStreamEntry(tc.pair)
		if err != nil {
			t.Fatalf("parseStreamEntry(%q) error: %v", tc.pair, err)
		}
		if spec.id != tc.wantID {
			t.Errorf("pair %q: id got %q, want %q", tc.pair, spec.id, tc.wantID)
		}
		if spec.source != tc.wantSrc {
			t.Errorf("pair %q: source got %q, want %q", tc.pair, spec.source, tc.wantSrc)
		}
		if spec.transport != tc.wantTrans {
			t.Errorf("pair %q: transport got %q, want %q", tc.pair, spec.transport, tc.wantTrans)
		}
	}
}

func TestParseStreamEntryInvalid(t *testing.T) {
	_, err := parseStreamEntry("no-equals-sign")
	if err == nil {
		t.Error("expected error for entry without '='")
	}
}

func TestTargetURLRTSP(t *testing.T) {
	cfg := config{
		originCount:        1,
		originHostTemplate: "mediamtx-origin-%d",
		originRTSPPort:     8554,
		publishSecret:      "secret",
	}
	spec := streamSpec{id: "cam-01", source: "test", transport: "rtsp"}
	got := cfg.targetURL(spec)
	want := "rtsp://publisher:secret@mediamtx-origin-0:8554/cam-01"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestTargetURLSRT(t *testing.T) {
	cfg := config{
		originCount:        1,
		originHostTemplate: "mediamtx-origin-%d",
		originSRTPort:      8890,
		publishSecret:      "secret",
		srtLatencyMS:       200,
	}
	spec := streamSpec{id: "cam-03", source: "rtsp://cam/stream", transport: "srt"}
	got := cfg.targetURL(spec)
	want := "srt://mediamtx-origin-0:8890?streamid=publish:cam-03:publisher:secret&latency=200000"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestTargetURLSRTWithPassphrase(t *testing.T) {
	cfg := config{
		originCount:        1,
		originHostTemplate: "mediamtx-origin-%d",
		originSRTPort:      8890,
		publishSecret:      "secret",
		srtLatencyMS:       200,
		srtPassphrase:      "mykey",
	}
	spec := streamSpec{id: "cam-03", source: "rtsp://cam/stream", transport: "srt"}
	got := cfg.targetURL(spec)
	want := "srt://mediamtx-origin-0:8890?streamid=publish:cam-03:publisher:secret&latency=200000&passphrase=mykey"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestTargetURLSRTLatencyScaling(t *testing.T) {
	cfg := config{
		originCount:        1,
		originHostTemplate: "mediamtx-origin-%d",
		originSRTPort:      8890,
		publishSecret:      "s",
		srtLatencyMS:       500,
	}
	spec := streamSpec{id: "x", transport: "srt"}
	got := cfg.targetURL(spec)
	if !strings.Contains(got, "latency=500000") {
		t.Errorf("expected latency=500000 (500ms * 1000), got %q", got)
	}
}
```

- [ ] **Step 2: Run tests — verify they fail**

```
cd services/publisher && go test . -v
```

Expected: compilation failure or test failure — `parseStreamEntry` and updated `streamSpec`/`config` don't exist yet.

- [ ] **Step 3: Implement updated main.go**

Replace the full contents of `services/publisher/main.go`:

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
	id        string
	source    string // "test" or rtsp:// URL
	transport string // "rtsp" (default) or "srt"
}

type config struct {
	originCount        int
	originHostTemplate string
	originRTSPPort     int
	originSRTPort      int
	publishSecret      string
	srtLatencyMS       int
	srtPassphrase      string
	streams            []streamSpec
}

func loadConfig() config {
	cfg := config{
		originCount:        getEnvInt("ORIGIN_COUNT", 3),
		originHostTemplate: getEnv("ORIGIN_HOST_TEMPLATE", "mediamtx-origin-%d"),
		originRTSPPort:     getEnvInt("ORIGIN_RTSP_PORT", 8554),
		originSRTPort:      getEnvInt("ORIGIN_SRT_PORT", 8890),
		publishSecret:      mustEnv("PUBLISH_SECRET"),
		srtLatencyMS:       getEnvInt("SRT_LATENCY_MS", 200),
		srtPassphrase:      getEnv("SRT_PASSPHRASE", ""),
	}

	streamsEnv := getEnv("STREAMS", "")
	if streamsEnv == "" {
		log.Fatal().Msg("STREAMS env var required: comma-separated stream_id=source[@srt] pairs, e.g. cam-01=test,cam-02=rtsp://camera/stream,cam-03=rtsp://far-cam/stream@srt")
	}
	for _, pair := range strings.Split(streamsEnv, ",") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		spec, err := parseStreamEntry(pair)
		if err != nil {
			log.Fatal().Str("pair", pair).Msg(err.Error())
		}
		cfg.streams = append(cfg.streams, spec)
	}
	if len(cfg.streams) == 0 {
		log.Fatal().Msg("STREAMS must contain at least one entry")
	}
	return cfg
}

// parseStreamEntry parses one "stream_id=source[@srt]" pair.
// The @srt suffix selects SRT transport for the push leg; absent means RTSP.
func parseStreamEntry(pair string) (streamSpec, error) {
	parts := strings.SplitN(pair, "=", 2)
	if len(parts) != 2 {
		return streamSpec{}, fmt.Errorf("invalid STREAMS entry: expected stream_id=source")
	}
	source := parts[1]
	transport := "rtsp"
	if strings.HasSuffix(source, "@srt") {
		source = strings.TrimSuffix(source, "@srt")
		transport = "srt"
	}
	return streamSpec{id: parts[0], source: source, transport: transport}, nil
}

func (c config) targetURL(s streamSpec) string {
	idx := hash.OriginIndex(s.id, c.originCount)
	host := fmt.Sprintf(c.originHostTemplate, idx)
	if s.transport == "srt" {
		streamid := fmt.Sprintf("publish:%s:publisher:%s", s.id, c.publishSecret)
		u := fmt.Sprintf("srt://%s:%d?streamid=%s&latency=%d",
			host, c.originSRTPort, streamid, c.srtLatencyMS*1000)
		if c.srtPassphrase != "" {
			u += "&passphrase=" + c.srtPassphrase
		}
		return u
	}
	return fmt.Sprintf("rtsp://publisher:%s@%s:%d/%s",
		c.publishSecret, host, c.originRTSPPort, s.id)
}

func main() {
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stdout})

	cfg := loadConfig()

	log.Info().Int("streams", len(cfg.streams)).Int("origins", cfg.originCount).Msg("publisher starting")
	for _, s := range cfg.streams {
		idx := hash.OriginIndex(s.id, cfg.originCount)
		log.Info().Str("stream", s.id).Str("source", s.source).Str("transport", s.transport).Int("origin", idx).Msg("stream routed")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	for _, s := range cfg.streams {
		s := s
		target := cfg.targetURL(s)
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

- [ ] **Step 4: Run all publisher tests — verify all pass**

```
cd services/publisher && go test ./... -v
```

Expected: all tests in `relay/`, `hash/`, and root package PASS.

- [ ] **Step 5: Verify build**

```
cd services/publisher && go build .
```

Expected: exits 0, no output.

- [ ] **Step 6: Commit**

```
cd services/publisher
git add main.go main_test.go
git commit -m "feat(publisher): add per-stream @srt transport with SRT URL builder"
```

---

## Task 3: Enable SRT on MediaMTX origin

**Files:**
- Modify: `configs/mediamtx/origin.yml`

- [ ] **Step 1: Add SRT stanza**

In `configs/mediamtx/origin.yml`, add after the `rtsp` block (after line `rtspEncryption: "no"`):

```yaml
srt: true
srtAddress: :8890
```

The full file becomes:

```yaml
logLevel: info
logDestinations:
  - stdout

api: true
apiAddress: :9997

metrics: true
metricsAddress: :9998

rtsp: true
rtspAddress: :8554
rtspEncryption: "no"

srt: true
srtAddress: :8890

webrtc: false

authMethod: internal
authInternalUsers:
  - user: any
    pass: ""
    permissions:
      - action: api
      - action: metrics
  - user: publisher
    pass: $PUBLISH_SECRET
    permissions:
      - action: publish
        path: ""
  - user: internal
    pass: $INTERNAL_SECRET
    permissions:
      - action: read
        path: ""
      - action: playback
        path: ""

paths:
  all_others:
    record: true
    recordPath: /recordings/%path/%Y-%m-%d_%H-%M-%S
    recordFormat: fmp4
    recordPartDuration: 1s
    recordSegmentDuration: 60s
    recordDeleteAfter: 2h
```

- [ ] **Step 2: Commit**

```
git add configs/mediamtx/origin.yml
git commit -m "feat(mediamtx): enable SRT listener on port 8890"
```

---

## Task 4: Expose SRT port in docker-compose and update .env.example

**Files:**
- Modify: `docker-compose.yml`
- Modify: `.env.example`

- [ ] **Step 1: Add SRT port to mediamtx-origin-0 in docker-compose.yml**

In `docker-compose.yml`, find the `mediamtx-origin-0` ports block:

```yaml
    ports:
      - "8554:8554"        # RTSP publish
      - "9997:9997"        # Control API (origin)
```

Replace with:

```yaml
    ports:
      - "8554:8554"        # RTSP publish
      - "8890:8890/udp"    # SRT ingest (WAN camera relay)
      - "9997:9997"        # Control API (origin)
```

- [ ] **Step 2: Add SRT env vars to publisher service in docker-compose.yml**

Find the `publisher` service environment block:

```yaml
    environment:
      - ORIGIN_COUNT=1
      - ORIGIN_HOST_TEMPLATE=mediamtx-origin-%d
      - ORIGIN_RTSP_PORT=8554
      - PUBLISH_SECRET=${PUBLISH_SECRET}
      - STREAMS=${PUBLISHER_STREAMS:-cam-01=test,cam-02=test,cam-03=test,cam-04=test,cam-05=test,cam-06=test,cam-07=test,cam-08=test,cam-09=test,cam-10=test}
```

Replace with:

```yaml
    environment:
      - ORIGIN_COUNT=1
      - ORIGIN_HOST_TEMPLATE=mediamtx-origin-%d
      - ORIGIN_RTSP_PORT=8554
      - ORIGIN_SRT_PORT=8890
      - PUBLISH_SECRET=${PUBLISH_SECRET}
      - SRT_LATENCY_MS=${SRT_LATENCY_MS:-200}
      - SRT_PASSPHRASE=${SRT_PASSPHRASE:-}
      - STREAMS=${PUBLISHER_STREAMS:-cam-01=test,cam-02=test,cam-03=test,cam-04=test,cam-05=test,cam-06=test,cam-07=test,cam-08=test,cam-09=test,cam-10=test}
```

- [ ] **Step 3: Add SRT vars to .env.example**

Append to `.env.example`:

```env

# SRT ingest for far/WAN cameras (ORIGIN_TRANSPORT is per-stream via @srt suffix in STREAMS)
# Set SRT_LATENCY_MS to approx 4x the RTT (ms) from the edge relay to this host.
# Example: 40ms RTT -> use 200. Leave blank to use default (200ms).
# SRT_PASSPHRASE enables AES encryption on the SRT stream; leave blank to disable.
SRT_LATENCY_MS=200
SRT_PASSPHRASE=
```

- [ ] **Step 4: Commit**

```
git add docker-compose.yml .env.example
git commit -m "feat(compose): expose SRT port 8890/udp and add SRT env vars to publisher"
```

---

## Task 5: Update deployment-guide.md

**Files:**
- Modify: `docs/deployment-guide.md`

- [ ] **Step 1: Add SRT to container port matrix (§4.1)**

In the **§4.1 Container / internal ports** table, add after the `origin | 9998` row:

```
| origin | 8890 | UDP | SRT ingest (far/WAN camera relay) |
```

The table block becomes:

```markdown
| Service | Port | Proto | Purpose |
|---------|------|-------|---------|
| origin | 8554 | TCP | RTSP publish + internal pull |
| origin | 8890 | UDP | SRT ingest (far/WAN camera relay) |
| origin | 9997 | TCP | Control API (`/v3/paths/list`) |
| origin | 9998 | TCP | Prometheus metrics |
```

- [ ] **Step 2: Add SRT to Compose host port mappings (§4.2)**

In the **§4.2 Docker Compose host port mappings** table, add after the `origin | 8554` row:

```
| origin | **8890** | 8890 | UDP |
```

- [ ] **Step 3: Add SRT to Compose firewall table (§5.1)**

In the **§5.1 Docker Compose host** firewall table, add after the `8554` row:

```
| 8890 | UDP | SRT ingest (WAN edge relay) | edge publisher relays (external) |
```

With the note: `8890/UDP` only needs opening if edge publisher relays connect from outside the host (far cameras). Local-only Compose setups can skip it.

- [ ] **Step 4: Update publisher runtime env reference (§3.4)**

In **§3.4 Per-service runtime env**, find the **publisher** line:

```
**publisher**: `ORIGIN_COUNT`, `ORIGIN_HOST_TEMPLATE`, `ORIGIN_RTSP_PORT=8554`, `PUBLISH_SECRET`, `STREAMS`.
```

Replace with:

```
**publisher**: `ORIGIN_COUNT`, `ORIGIN_HOST_TEMPLATE`, `ORIGIN_RTSP_PORT=8554`, `ORIGIN_SRT_PORT=8890`, `PUBLISH_SECRET`, `STREAMS`, `SRT_LATENCY_MS=200`, `SRT_PASSPHRASE` (optional).

`STREAMS` entries may carry an `@srt` suffix to select SRT push for that stream: `cam-03=rtsp://far-cam/stream@srt`. No suffix = RTSP push (default).
```

- [ ] **Step 5: Add SRT edge relay section**

Append a new section **§13** at the end of the document (before the Appendix quick-command cheat sheet, or after it):

```markdown
---

## 13. SRT edge relay — far/WAN cameras

Use when a camera is on a remote site and cannot push RTSP/TCP into the AG network reliably.

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

The edge relay is the same `publisher` binary, deployed near the camera (Docker, bare metal, or edge device). Internal cluster traffic is unaffected.

### 13.2 Edge relay configuration

Run the publisher near the far camera with these env vars:

```env
STREAMS=cam-id=rtsp://camera-local-ip/stream@srt
ORIGIN_COUNT=<same as AG origin count>
ORIGIN_HOST_TEMPLATE=<AG origin hostname or IP>
ORIGIN_SRT_PORT=8890
PUBLISH_SECRET=<same PUBLISH_SECRET as on the AG origin>
SRT_LATENCY_MS=<4 × RTT in ms — see below>
SRT_PASSPHRASE=<shared AES key — recommended>
```

The publisher connects **outbound** from the edge site to the AG host's UDP port 8890. The AG firewall needs one inbound rule: `UDP 8890 from <edge-relay-IP>`. No dynamic port ranges.

### 13.3 Tuning SRT_LATENCY_MS

`SRT_LATENCY_MS` is the ARQ recovery window. If a packet is lost, SRT has this many milliseconds to retransmit it before the decoder needs it. Insufficient budget → glitches; excess budget → added glass-to-glass delay.

Rule of thumb: `SRT_LATENCY_MS = RTT × 4`

| RTT to AG host | Recommended SRT_LATENCY_MS |
|----------------|---------------------------|
| < 20 ms (LAN-like) | 100 |
| 20–50 ms | 200 (default) |
| 50–100 ms | 400 |
| 100–200 ms | 800 |
| > 200 ms | 1000+ |

Measure RTT: `ping <AG-host-IP>` from the edge relay machine.

### 13.4 Mixing local and far cameras in one publisher

A single publisher instance handles both transports:

```env
STREAMS=cam-01=test,cam-02=rtsp://local-cam/stream,cam-03=rtsp://far-cam/stream@srt
```

`cam-01` and `cam-02` push to origin via RTSP/TCP. `cam-03` pushes via SRT. Only `cam-03`'s traffic crosses the WAN.

### 13.5 Firewall rules

Add to the AG host (Compose or k3s node):

```bash
# Compose host
sudo ufw allow from <edge-relay-IP> to any port 8890 proto udp

# k3s node (if adding SRT NodePort — not in default Helm chart)
sudo ufw allow from <edge-relay-IP> to any port <SRT-NodePort> proto udp
```

Restrict source IP to the known edge relay IP(s) for defense in depth.

### 13.6 Verify SRT connection

From the AG host, after starting the edge relay:

```bash
# MediaMTX origin API should show the stream as active
curl http://localhost:9997/v3/paths/list | python3 -m json.tool | grep cam-id

# Publisher logs on the edge relay should show:
# relay starting  stream=cam-id  source=rtsp://...  target=srt://...
```
```

- [ ] **Step 6: Commit**

```
git add docs/deployment-guide.md
git commit -m "docs: add SRT to port matrix, firewall tables, and add SRT edge relay section"
```

---

## Self-Review Checklist

- [x] **Spec coverage:** relay audio stripping ✅ | SRT target detection ✅ | @srt suffix parsing ✅ | SRT config fields ✅ | SRT URL builder ✅ | MediaMTX origin config ✅ | docker-compose port + env ✅ | .env.example ✅ | docs port matrix ✅ | docs firewall ✅ | docs publisher env ✅ | docs SRT section ✅
- [x] **No placeholders:** all code blocks complete; no TBD/TODO
- [x] **Type consistency:** `streamSpec.transport` defined in Task 2 Step 3; used in same task's `targetURL`; `parseStreamEntry` defined and tested in same task
- [x] **TDD:** relay tests written before impl (Task 1); main tests written before impl (Task 2)
- [x] **Audio removal global:** `-an` added to both test-pattern and RTSP-source paths in all combinations
