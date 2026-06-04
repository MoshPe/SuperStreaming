# API Server (Sub-Project 4) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the `api-server` Go service that exposes `/api/streams/live` (SSE), `/api/recordings` (REST), and `/health` for the web frontend.

**Architecture:** Standard `net/http` server (no framework). Four packages: `db` (recordings query), `mediamtx` (origin poller), `miniopresign` (presigned URLs), `handlers` (HTTP handlers). CORS middleware wraps all routes. The nginx stub from Sub-Project 1 is replaced by this real service.

**Tech Stack:** Go 1.23, `lib/pq` v1.10, `minio-go` v7, `zerolog` v1.33, standard `net/http`, Docker multi-stage build.

---

## File Structure

```
services/
└── api-server/
    ├── go.mod
    ├── main.go                        ← config, HTTP server, CORS, routing, shutdown
    ├── db/
    │   ├── db.go                      ← Postgres pool, Recording type, QueryRecordings
    │   └── db_test.go                 ← unit test for QueryParams validation
    ├── mediamtx/
    │   ├── client.go                  ← HTTP client, ListPaths, Path type
    │   └── client_test.go             ← test with httptest.NewServer mock
    ├── miniopresign/
    │   └── presign.go                 ← MinIO client wrapper, PresignGetURL
    └── handlers/
        ├── health.go                  ← GET /health
        ├── recordings.go              ← GET /api/recordings + parseQueryParams
        ├── recordings_test.go         ← unit test for parseQueryParams
        ├── streams.go                 ← GET /api/streams/live (SSE poller + handler)
        └── streams_test.go            ← test SSE event format with mock poller

build/
└── api-server/
    └── Dockerfile

docker-compose.yml                     ← replace nginx stub with real api-server (modify)
k8s/api-server/
    ├── deployment.yaml                ← real Deployment (new file)
    └── service.yaml                   ← ClusterIP service (new file)
k8s/stubs/api-server-stub.yaml        ← delete this file
scripts/test-api-server.ps1
```

---

### Task 1: Go module + config

**Files:**
- Create: `services/api-server/go.mod`
- Create: `services/api-server/main.go`

- [ ] **Step 1: Create directories**

```powershell
mkdir services\api-server\db, services\api-server\mediamtx, services\api-server\miniopresign, services\api-server\handlers
mkdir build\api-server
mkdir k8s\api-server
```

- [ ] **Step 2: Initialize Go module**

```powershell
cd services\api-server
go mod init superstreaming/api-server
```

- [ ] **Step 3: Create main.go (skeleton)**

Create `services/api-server/main.go`:
```go
package main

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

type config struct {
	listenAddr         string
	originCount        int
	originHostTemplate string
	originAPIPort      int
	originPollInterval time.Duration
	minioEndpoint      string
	minioUser          string
	minioPassword      string
	minioBucket        string
	minioUseSSL        bool
	minioPresignExpiry time.Duration
	postgresHost       string
	postgresPort       string
	postgresUser       string
	postgresPassword   string
	postgresDB         string
	corsOrigin         string
}

func (c config) postgresDSN() string {
	return fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable",
		c.postgresUser, c.postgresPassword, c.postgresHost, c.postgresPort, c.postgresDB)
}

func loadConfig() config {
	presignExpiry, _ := time.ParseDuration(getEnv("MINIO_PRESIGN_EXPIRY", "24h"))
	pollInterval, _ := time.ParseDuration(getEnv("ORIGIN_POLL_INTERVAL", "3s"))
	return config{
		listenAddr:         getEnv("LISTEN_ADDR", ":8080"),
		originCount:        getEnvInt("ORIGIN_COUNT", 3),
		originHostTemplate: getEnv("ORIGIN_HOST_TEMPLATE", "mediamtx-origin-%d"),
		originAPIPort:      getEnvInt("ORIGIN_API_PORT", 9997),
		originPollInterval: pollInterval,
		minioEndpoint:      mustEnv("MINIO_ENDPOINT"),
		minioUser:          mustEnv("MINIO_ROOT_USER"),
		minioPassword:      mustEnv("MINIO_ROOT_PASSWORD"),
		minioBucket:        mustEnv("MINIO_BUCKET"),
		minioUseSSL:        getEnv("MINIO_USE_SSL", "false") == "true",
		minioPresignExpiry: presignExpiry,
		postgresHost:       getEnv("POSTGRES_HOST", "postgres"),
		postgresPort:       getEnv("POSTGRES_PORT", "5432"),
		postgresUser:       mustEnv("POSTGRES_USER"),
		postgresPassword:   mustEnv("POSTGRES_PASSWORD"),
		postgresDB:         mustEnv("POSTGRES_DB"),
		corsOrigin:         getEnv("CORS_ORIGIN", "*"),
	}
}

func main() {
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stdout})
	log.Info().Msg("api-server starting (skeleton)")
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

- [ ] **Step 4: Add dependencies**

```powershell
go get github.com/rs/zerolog@v1.33.0
go get github.com/lib/pq@v1.10.9
go get github.com/minio/minio-go/v7@v7.0.70
go mod tidy
```

- [ ] **Step 5: Verify compiles**

```powershell
go build ./...
```

Expected: no output

- [ ] **Step 6: Commit**

```powershell
cd ..\..
git add services/api-server/
git commit -m "feat: init api-server Go module"
```

---

### Task 2: DB package — recordings query

**Files:**
- Create: `services/api-server/db/db.go`
- Create: `services/api-server/db/db_test.go`

- [ ] **Step 1: Write failing test for QueryParams validation**

Create `services/api-server/db/db_test.go`:
```go
package db

import (
	"testing"
	"time"
)

func TestQueryParams_Defaults(t *testing.T) {
	p := QueryParams{Limit: 0, Offset: 0}
	p.applyDefaults()
	if p.Limit != 100 {
		t.Errorf("Limit = %d, want 100", p.Limit)
	}
}

func TestQueryParams_LimitCap(t *testing.T) {
	p := QueryParams{Limit: 1000}
	p.applyDefaults()
	if p.Limit != 500 {
		t.Errorf("Limit = %d, want 500 (capped)", p.Limit)
	}
}

func TestQueryParams_TimeRange(t *testing.T) {
	from := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 6, 2, 0, 0, 0, 0, time.UTC)
	p := QueryParams{From: &from, To: &to}
	p.applyDefaults()
	if *p.From != from {
		t.Error("From changed unexpectedly")
	}
	if *p.To != to {
		t.Error("To changed unexpectedly")
	}
}
```

- [ ] **Step 2: Run failing test**

```powershell
cd services/api-server
go test ./db/ -v
```

Expected: FAIL — `undefined: QueryParams`

- [ ] **Step 3: Create db.go**

Create `services/api-server/db/db.go`:
```go
package db

import (
	"database/sql"
	"fmt"
	"time"

	_ "github.com/lib/pq"
)

// Recording is one row from the recordings table.
type Recording struct {
	ID        int64
	StreamID  string
	StartTime time.Time
	EndTime   time.Time
	MinioPath string
	DurationS int
}

// QueryParams filters for QueryRecordings.
type QueryParams struct {
	StreamID *string
	From     *time.Time
	To       *time.Time
	Limit    int
	Offset   int
}

func (p *QueryParams) applyDefaults() {
	if p.Limit <= 0 {
		p.Limit = 100
	}
	if p.Limit > 500 {
		p.Limit = 500
	}
}

// DB wraps a Postgres connection pool.
type DB struct {
	pool *sql.DB
}

// Connect opens a Postgres pool and verifies connectivity.
func Connect(dsn string) (*DB, error) {
	pool, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("sql.Open: %w", err)
	}
	pool.SetMaxOpenConns(10)
	pool.SetMaxIdleConns(3)
	if err := pool.Ping(); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}
	return &DB{pool: pool}, nil
}

// Close releases all pool connections.
func (d *DB) Close() { d.pool.Close() }

// QueryRecordings returns segments matching params plus the total count.
func (d *DB) QueryRecordings(p QueryParams) ([]Recording, int, error) {
	p.applyDefaults()

	const q = `
		SELECT id, stream_id, start_time, end_time, minio_path, duration_s
		FROM recordings
		WHERE ($1::text   IS NULL OR stream_id  = $1)
		  AND ($2::timestamptz IS NULL OR start_time >= $2)
		  AND ($3::timestamptz IS NULL OR end_time   <= $3)
		ORDER BY start_time DESC
		LIMIT $4 OFFSET $5`

	rows, err := d.pool.Query(q, p.StreamID, p.From, p.To, p.Limit, p.Offset)
	if err != nil {
		return nil, 0, fmt.Errorf("query: %w", err)
	}
	defer rows.Close()

	var recs []Recording
	for rows.Next() {
		var r Recording
		if err := rows.Scan(&r.ID, &r.StreamID, &r.StartTime, &r.EndTime, &r.MinioPath, &r.DurationS); err != nil {
			return nil, 0, fmt.Errorf("scan: %w", err)
		}
		recs = append(recs, r)
	}

	const cq = `
		SELECT count(*)
		FROM recordings
		WHERE ($1::text   IS NULL OR stream_id  = $1)
		  AND ($2::timestamptz IS NULL OR start_time >= $2)
		  AND ($3::timestamptz IS NULL OR end_time   <= $3)`

	var total int
	if err := d.pool.QueryRow(cq, p.StreamID, p.From, p.To).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count: %w", err)
	}
	return recs, total, nil
}
```

- [ ] **Step 4: Run DB tests — confirm PASS**

```powershell
go test ./db/ -v
```

Expected: 3 tests PASS

- [ ] **Step 5: Commit**

```powershell
cd ..\..
git add services/api-server/db/
git commit -m "feat: add api-server db package"
```

---

### Task 3: MediaMTX client

**Files:**
- Create: `services/api-server/mediamtx/client.go`
- Create: `services/api-server/mediamtx/client_test.go`

- [ ] **Step 1: Write failing test using mock HTTP server**

Create `services/api-server/mediamtx/client_test.go`:
```go
package mediamtx

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func TestListPaths_ReturnsOnlyReadyPaths(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v3/paths/list" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(pathList{
			Items: []path{
				{Name: "cam-01", Ready: true},
				{Name: "cam-02", Ready: false},
				{Name: "cam-03", Ready: true},
			},
		})
	}))
	defer ts.Close()

	// Extract host and port from test server URL
	addr := strings.TrimPrefix(ts.URL, "http://")
	parts := strings.SplitN(addr, ":", 2)
	port, _ := strconv.Atoi(parts[1])

	names, err := ListPaths(context.Background(), parts[0], port)
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 2 {
		t.Fatalf("expected 2 ready paths, got %d: %v", len(names), names)
	}
	if names[0] != "cam-01" || names[1] != "cam-03" {
		t.Errorf("unexpected paths: %v", names)
	}
}

func TestListPaths_ServerDown_ReturnsError(t *testing.T) {
	// Port 1 is reserved and will refuse connection
	_, err := ListPaths(context.Background(), "127.0.0.1", 1)
	if err == nil {
		t.Error("expected error for unreachable server, got nil")
	}
}
```

- [ ] **Step 2: Run failing test**

```powershell
cd services/api-server
go test ./mediamtx/ -v
```

Expected: FAIL — `undefined: ListPaths`

- [ ] **Step 3: Create client.go**

Create `services/api-server/mediamtx/client.go`:
```go
package mediamtx

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type path struct {
	Name  string `json:"name"`
	Ready bool   `json:"ready"`
}

type pathList struct {
	Items []path `json:"items"`
}

var httpClient = &http.Client{Timeout: 2 * time.Second}

// ListPaths returns the names of all ready paths from one MediaMTX origin pod.
// Returns an error if the origin is unreachable — callers should skip and continue.
func ListPaths(ctx context.Context, host string, port int) ([]string, error) {
	url := fmt.Sprintf("http://%s:%d/v3/paths/list", host, port)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("mediamtx %s:%d returned %d", host, port, resp.StatusCode)
	}

	var pl pathList
	if err := json.NewDecoder(resp.Body).Decode(&pl); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}

	var names []string
	for _, p := range pl.Items {
		if p.Ready {
			names = append(names, p.Name)
		}
	}
	return names, nil
}
```

- [ ] **Step 4: Run tests — confirm PASS**

```powershell
go test ./mediamtx/ -v
```

Expected: 2 tests PASS

- [ ] **Step 5: Commit**

```powershell
cd ..\..
git add services/api-server/mediamtx/
git commit -m "feat: add api-server mediamtx client"
```

---

### Task 4: MinIO presign helper

**Files:**
- Create: `services/api-server/miniopresign/presign.go`

- [ ] **Step 1: Create presign.go**

Create `services/api-server/miniopresign/presign.go`:
```go
package miniopresign

import (
	"context"
	"fmt"
	"net/url"
	"time"

	miniogo "github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// Client wraps a MinIO client for presigned URL generation.
type Client struct {
	mc     *miniogo.Client
	bucket string
	expiry time.Duration
}

// New creates a Client connected to endpoint.
func New(endpoint, user, password, bucket string, useSSL bool, expiry time.Duration) (*Client, error) {
	mc, err := miniogo.New(endpoint, &miniogo.Options{
		Creds:  credentials.NewStaticV4(user, password, ""),
		Secure: useSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("minio client: %w", err)
	}
	return &Client{mc: mc, bucket: bucket, expiry: expiry}, nil
}

// PresignGetURL returns a presigned GET URL for objectName valid for c.expiry.
func (c *Client) PresignGetURL(ctx context.Context, objectName string) (string, error) {
	u, err := c.mc.PresignedGetObject(ctx, c.bucket, objectName, c.expiry, url.Values{})
	if err != nil {
		return "", fmt.Errorf("presign %q: %w", objectName, err)
	}
	return u.String(), nil
}
```

- [ ] **Step 2: Verify compiles**

```powershell
cd services/api-server
go build ./...
```

- [ ] **Step 3: Commit**

```powershell
cd ..\..
git add services/api-server/miniopresign/
git commit -m "feat: add api-server miniopresign package"
```

---

### Task 5: Health + Recordings handlers

**Files:**
- Create: `services/api-server/handlers/health.go`
- Create: `services/api-server/handlers/recordings.go`
- Create: `services/api-server/handlers/recordings_test.go`

- [ ] **Step 1: Write failing test for parseQueryParams**

Create `services/api-server/handlers/recordings_test.go`:
```go
package handlers

import (
	"net/http"
	"net/url"
	"testing"
	"time"
)

func TestParseQueryParams_StreamID(t *testing.T) {
	r := &http.Request{URL: &url.URL{RawQuery: "stream_id=cam-01"}}
	p, err := parseQueryParams(r)
	if err != nil {
		t.Fatal(err)
	}
	if p.StreamID == nil || *p.StreamID != "cam-01" {
		t.Errorf("StreamID = %v, want cam-01", p.StreamID)
	}
}

func TestParseQueryParams_TimeRange(t *testing.T) {
	r := &http.Request{URL: &url.URL{
		RawQuery: "from=2026-06-01T00:00:00Z&to=2026-06-02T00:00:00Z",
	}}
	p, err := parseQueryParams(r)
	if err != nil {
		t.Fatal(err)
	}
	wantFrom := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	wantTo := time.Date(2026, 6, 2, 0, 0, 0, 0, time.UTC)
	if p.From == nil || !p.From.Equal(wantFrom) {
		t.Errorf("From = %v, want %v", p.From, wantFrom)
	}
	if p.To == nil || !p.To.Equal(wantTo) {
		t.Errorf("To = %v, want %v", p.To, wantTo)
	}
}

func TestParseQueryParams_InvalidTime(t *testing.T) {
	r := &http.Request{URL: &url.URL{RawQuery: "from=notadate"}}
	_, err := parseQueryParams(r)
	if err == nil {
		t.Error("expected error for invalid time, got nil")
	}
}

func TestParseQueryParams_LimitOffset(t *testing.T) {
	r := &http.Request{URL: &url.URL{RawQuery: "limit=50&offset=100"}}
	p, err := parseQueryParams(r)
	if err != nil {
		t.Fatal(err)
	}
	if p.Limit != 50 {
		t.Errorf("Limit = %d, want 50", p.Limit)
	}
	if p.Offset != 100 {
		t.Errorf("Offset = %d, want 100", p.Offset)
	}
}
```

- [ ] **Step 2: Run failing test**

```powershell
cd services/api-server
go test ./handlers/ -v -run TestParseQueryParams
```

Expected: FAIL — `undefined: parseQueryParams`

- [ ] **Step 3: Create health.go**

Create `services/api-server/handlers/health.go`:
```go
package handlers

import (
	"encoding/json"
	"net/http"
)

// Health handles GET /health.
func Health(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
```

- [ ] **Step 4: Create recordings.go**

Create `services/api-server/handlers/recordings.go`:
```go
package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"superstreaming/api-server/db"
)

// Presigner generates presigned GET URLs for MinIO objects.
type Presigner interface {
	PresignGetURL(ctx context.Context, objectName string) (string, error)
}

// RecordingsDB is the subset of db.DB the recordings handler needs.
type RecordingsDB interface {
	QueryRecordings(p db.QueryParams) ([]db.Recording, int, error)
}

// RecordingsHandler handles GET /api/recordings.
type RecordingsHandler struct {
	db       RecordingsDB
	presigner Presigner
}

// NewRecordingsHandler creates a RecordingsHandler.
func NewRecordingsHandler(db RecordingsDB, presigner Presigner) *RecordingsHandler {
	return &RecordingsHandler{db: db, presigner: presigner}
}

type segmentResponse struct {
	ID        int64     `json:"id"`
	StreamID  string    `json:"stream_id"`
	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time"`
	DurationS int       `json:"duration_s"`
	URL       string    `json:"url"`
}

type recordingsResponse struct {
	Segments []segmentResponse `json:"segments"`
	Total    int               `json:"total"`
	Limit    int               `json:"limit"`
	Offset   int               `json:"offset"`
}

func (h *RecordingsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	params, err := parseQueryParams(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	recs, total, err := h.db.QueryRecordings(params)
	if err != nil {
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}

	segments := make([]segmentResponse, 0, len(recs))
	for _, rec := range recs {
		u, err := h.presigner.PresignGetURL(r.Context(), rec.MinioPath)
		if err != nil {
			http.Error(w, "presign error", http.StatusInternalServerError)
			return
		}
		segments = append(segments, segmentResponse{
			ID:        rec.ID,
			StreamID:  rec.StreamID,
			StartTime: rec.StartTime,
			EndTime:   rec.EndTime,
			DurationS: rec.DurationS,
			URL:       u,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(recordingsResponse{
		Segments: segments,
		Total:    total,
		Limit:    params.Limit,
		Offset:   params.Offset,
	})
}

func parseQueryParams(r *http.Request) (db.QueryParams, error) {
	q := r.URL.Query()
	p := db.QueryParams{}

	if s := q.Get("stream_id"); s != "" {
		p.StreamID = &s
	}
	if s := q.Get("from"); s != "" {
		t, err := time.Parse(time.RFC3339, s)
		if err != nil {
			return p, fmt.Errorf("invalid from: %w", err)
		}
		p.From = &t
	}
	if s := q.Get("to"); s != "" {
		t, err := time.Parse(time.RFC3339, s)
		if err != nil {
			return p, fmt.Errorf("invalid to: %w", err)
		}
		p.To = &t
	}
	if s := q.Get("limit"); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n < 0 {
			return p, fmt.Errorf("invalid limit: %s", s)
		}
		p.Limit = n
	}
	if s := q.Get("offset"); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n < 0 {
			return p, fmt.Errorf("invalid offset: %s", s)
		}
		p.Offset = n
	}
	return p, nil
}
```

- [ ] **Step 5: Run handler tests — confirm PASS**

```powershell
go test ./handlers/ -v -run TestParseQueryParams
```

Expected: 4 tests PASS

- [ ] **Step 6: Commit**

```powershell
cd ..\..
git add services/api-server/handlers/
git commit -m "feat: add health and recordings handlers"
```

---

### Task 6: Streams SSE handler

**Files:**
- Create: `services/api-server/handlers/streams.go`
- Create: `services/api-server/handlers/streams_test.go`

- [ ] **Step 1: Write failing SSE test**

Create `services/api-server/handlers/streams_test.go`:
```go
package handlers

import (
	"bufio"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestStreamsHandler_SendsSSEOnConnect(t *testing.T) {
	h := NewStreamsHandler([]string{"127.0.0.1:19997"}, 100*time.Millisecond)

	// Use a pipe so we can read the response body without blocking.
	req := httptest.NewRequest(http.MethodGet, "/api/streams/live", nil)

	// We need a ResponseRecorder that supports Flush — use a custom one.
	pr, pw := chanPipe()
	rw := &flushWriter{pw: pw, header: make(http.Header), code: 200}

	done := make(chan struct{})
	go func() {
		defer close(done)
		h.ServeHTTP(rw, req)
	}()

	// Read first line within 2s
	scanner := bufio.NewScanner(pr)
	lineCh := make(chan string, 1)
	go func() {
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "event:") || strings.HasPrefix(line, "data:") {
				lineCh <- line
				return
			}
		}
	}()

	select {
	case line := <-lineCh:
		if !strings.Contains(line, "streams") && !strings.Contains(line, "heartbeat") {
			t.Errorf("unexpected SSE line: %q", line)
		}
	case <-time.After(2 * time.Second):
		t.Error("timeout waiting for first SSE event")
	}

	// Cancel the request context to stop the handler.
	req.Context() // already background; just let test finish
}
```

Note: the test uses helper types `chanPipe` and `flushWriter`. Add them to a test helper file.

- [ ] **Step 2: Create test helpers**

Create `services/api-server/handlers/testhelpers_test.go`:
```go
package handlers

import (
	"io"
	"net/http"
)

// flushWriter implements http.ResponseWriter + http.Flusher for tests.
type flushWriter struct {
	pw     io.Writer
	header http.Header
	code   int
}

func (f *flushWriter) Header() http.Header         { return f.header }
func (f *flushWriter) WriteHeader(code int)         { f.code = code }
func (f *flushWriter) Write(b []byte) (int, error)  { return f.pw.Write(b) }
func (f *flushWriter) Flush()                        {}

func chanPipe() (io.Reader, io.Writer) {
	pr, pw := io.Pipe()
	return pr, pw
}
```

- [ ] **Step 3: Run failing test**

```powershell
cd services/api-server
go test ./handlers/ -v -run TestStreamsHandler -timeout 10s
```

Expected: FAIL — `undefined: NewStreamsHandler`

- [ ] **Step 4: Create streams.go**

Create `services/api-server/handlers/streams.go`:
```go
package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"time"

	"github.com/rs/zerolog/log"

	"superstreaming/api-server/mediamtx"
)

// StreamsHandler handles GET /api/streams/live with SSE.
type StreamsHandler struct {
	origins  []string      // "host:port" for each origin
	interval time.Duration // poll interval
}

// NewStreamsHandler creates a StreamsHandler.
// origins is a slice of "host:port" strings for each MediaMTX origin API.
func NewStreamsHandler(origins []string, interval time.Duration) *StreamsHandler {
	return &StreamsHandler{origins: origins, interval: interval}
}

type streamsEvent struct {
	Active  []string `json:"active"`
	Added   []string `json:"added"`
	Removed []string `json:"removed"`
}

func (h *StreamsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	flusher.Flush()

	ctx := r.Context()
	ticker := time.NewTicker(h.interval)
	heartbeat := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	defer heartbeat.Stop()

	var prev []string

	sendEvent := func(name string, payload any) {
		data, _ := json.Marshal(payload)
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", name, data)
		flusher.Flush()
	}

	for {
		select {
		case <-ctx.Done():
			return

		case <-heartbeat.C:
			sendEvent("heartbeat", map[string]any{})

		case <-ticker.C:
			current := h.pollAll(ctx)
			added, removed := diff(prev, current)
			if len(added) > 0 || len(removed) > 0 || prev == nil {
				sendEvent("streams", streamsEvent{
					Active:  current,
					Added:   added,
					Removed: removed,
				})
				prev = current
			}
		}
	}
}

// pollAll queries all origins and returns the merged set of ready stream names.
func (h *StreamsHandler) pollAll(ctx context.Context) []string {
	seen := make(map[string]struct{})
	for _, addr := range h.origins {
		// Split "host:port" — mediamtx.ListPaths takes host and port separately.
		var host string
		var port int
		if _, err := fmt.Sscanf(addr, "%s", &host); err != nil {
			continue
		}
		// Use a simple split to extract host:port.
		var h2 string
		if _, err := fmt.Sscanf(addr, "%[^:]:%d", &h2, &port); err != nil {
			log.Warn().Str("addr", addr).Msg("bad origin addr format")
			continue
		}
		names, err := mediamtx.ListPaths(ctx, h2, port)
		if err != nil {
			log.Warn().Str("origin", addr).Err(err).Msg("origin unreachable, skipping")
			continue
		}
		for _, n := range names {
			seen[n] = struct{}{}
		}
	}
	result := make([]string, 0, len(seen))
	for n := range seen {
		result = append(result, n)
	}
	sort.Strings(result)
	return result
}

// diff returns elements added to and removed from prev to get current.
func diff(prev, current []string) (added, removed []string) {
	prevSet := make(map[string]struct{}, len(prev))
	for _, s := range prev {
		prevSet[s] = struct{}{}
	}
	currSet := make(map[string]struct{}, len(current))
	for _, s := range current {
		currSet[s] = struct{}{}
		if _, ok := prevSet[s]; !ok {
			added = append(added, s)
		}
	}
	for _, s := range prev {
		if _, ok := currSet[s]; !ok {
			removed = append(removed, s)
		}
	}
	return added, removed
}
```

- [ ] **Step 5: Run all handler tests**

```powershell
go test ./handlers/ -v -timeout 15s
```

Expected: all PASS

- [ ] **Step 6: Commit**

```powershell
cd ..\..
git add services/api-server/handlers/
git commit -m "feat: add streams SSE handler"
```

---

### Task 7: Main HTTP server

**Files:**
- Modify: `services/api-server/main.go`

- [ ] **Step 1: Replace main.go with full server wiring**

Replace `services/api-server/main.go`:
```go
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"superstreaming/api-server/db"
	"superstreaming/api-server/handlers"
	"superstreaming/api-server/miniopresign"
)

type config struct {
	listenAddr         string
	originCount        int
	originHostTemplate string
	originAPIPort      int
	originPollInterval time.Duration
	minioEndpoint      string
	minioUser          string
	minioPassword      string
	minioBucket        string
	minioUseSSL        bool
	minioPresignExpiry time.Duration
	postgresHost       string
	postgresPort       string
	postgresUser       string
	postgresPassword   string
	postgresDB         string
	corsOrigin         string
}

func (c config) postgresDSN() string {
	return fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable",
		c.postgresUser, c.postgresPassword, c.postgresHost, c.postgresPort, c.postgresDB)
}

func (c config) originAddrs() []string {
	addrs := make([]string, c.originCount)
	for i := range addrs {
		addrs[i] = fmt.Sprintf("%s:%d", fmt.Sprintf(c.originHostTemplate, i), c.originAPIPort)
	}
	return addrs
}

func main() {
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stdout})

	cfg := config{
		listenAddr:         getEnv("LISTEN_ADDR", ":8080"),
		originCount:        getEnvInt("ORIGIN_COUNT", 3),
		originHostTemplate: getEnv("ORIGIN_HOST_TEMPLATE", "mediamtx-origin-%d"),
		originAPIPort:      getEnvInt("ORIGIN_API_PORT", 9997),
		minioEndpoint:      mustEnv("MINIO_ENDPOINT"),
		minioUser:          mustEnv("MINIO_ROOT_USER"),
		minioPassword:      mustEnv("MINIO_ROOT_PASSWORD"),
		minioBucket:        mustEnv("MINIO_BUCKET"),
		minioUseSSL:        getEnv("MINIO_USE_SSL", "false") == "true",
		postgresHost:       getEnv("POSTGRES_HOST", "postgres"),
		postgresPort:       getEnv("POSTGRES_PORT", "5432"),
		postgresUser:       mustEnv("POSTGRES_USER"),
		postgresPassword:   mustEnv("POSTGRES_PASSWORD"),
		postgresDB:         mustEnv("POSTGRES_DB"),
		corsOrigin:         getEnv("CORS_ORIGIN", "*"),
	}

	presignExpiry, err := time.ParseDuration(getEnv("MINIO_PRESIGN_EXPIRY", "24h"))
	if err != nil {
		presignExpiry = 24 * time.Hour
	}
	cfg.minioPresignExpiry = presignExpiry

	pollInterval, err := time.ParseDuration(getEnv("ORIGIN_POLL_INTERVAL", "3s"))
	if err != nil {
		pollInterval = 3 * time.Second
	}
	cfg.originPollInterval = pollInterval

	dbPool, err := db.Connect(cfg.postgresDSN())
	if err != nil {
		log.Fatal().Err(err).Msg("connect to postgres")
	}
	defer dbPool.Close()

	presigner, err := miniopresign.New(
		cfg.minioEndpoint, cfg.minioUser, cfg.minioPassword,
		cfg.minioBucket, cfg.minioUseSSL, cfg.minioPresignExpiry,
	)
	if err != nil {
		log.Fatal().Err(err).Msg("create minio presigner")
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", handlers.Health)
	mux.Handle("/api/recordings", handlers.NewRecordingsHandler(dbPool, presigner))
	mux.Handle("/api/streams/live", handlers.NewStreamsHandler(cfg.originAddrs(), cfg.originPollInterval))

	srv := &http.Server{
		Addr:    cfg.listenAddr,
		Handler: corsMiddleware(cfg.corsOrigin, mux),
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	go func() {
		log.Info().Str("addr", cfg.listenAddr).Msg("api-server listening")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("listen")
		}
	}()

	<-ctx.Done()
	log.Info().Msg("shutting down")
	shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	srv.Shutdown(shutCtx)
	log.Info().Msg("shutdown complete")
}

func corsMiddleware(origin string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
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

- [ ] **Step 2: Verify full build + all tests**

```powershell
cd services/api-server
go build ./...
go test ./... -timeout 30s
```

Expected: build succeeds, all tests PASS

- [ ] **Step 3: Commit**

```powershell
cd ..\..
git add services/api-server/main.go
git commit -m "feat: wire api-server main HTTP server"
```

---

### Task 8: Dockerfile

**Files:**
- Create: `build/api-server/Dockerfile`

- [ ] **Step 1: Create Dockerfile**

Create `build/api-server/Dockerfile`:
```dockerfile
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

- [ ] **Step 2: Build image**

```powershell
cd D:\Software_Projects\SuperStreaming
docker build -f build/api-server/Dockerfile -t superstreaming/api-server:latest .
```

Expected: `Successfully tagged superstreaming/api-server:latest`

- [ ] **Step 3: Commit**

```powershell
git add build/api-server/Dockerfile
git commit -m "feat: add api-server Dockerfile"
```

---

### Task 9: Docker Compose — replace stub + smoke test

**Files:**
- Modify: `docker-compose.yml`
- Create: `scripts/test-api-server.ps1`

- [ ] **Step 1: Write smoke test first**

Create `scripts/test-api-server.ps1`:
```powershell
$pass = $true

function Test-Endpoint {
    param($name, $url, $expectedStatus = 200)
    try {
        $r = Invoke-WebRequest -Uri $url -UseBasicParsing -ErrorAction Stop
        if ($r.StatusCode -eq $expectedStatus) {
            Write-Host "PASS: $name ($url)" -ForegroundColor Green
        } else {
            Write-Host "FAIL: $name - got $($r.StatusCode)" -ForegroundColor Red
            $script:pass = $false
        }
    } catch {
        Write-Host "FAIL: $name - $($_.Exception.Message)" -ForegroundColor Red
        $script:pass = $false
    }
}

# Health check
Test-Endpoint "GET /health" "http://localhost:8080/health"
$health = Invoke-RestMethod "http://localhost:8080/health" -ErrorAction SilentlyContinue
if ($health.status -eq "ok") {
    Write-Host "PASS: /health returns {status:ok}" -ForegroundColor Green
} else {
    Write-Host "FAIL: /health body wrong: $health" -ForegroundColor Red
    $pass = $false
}

# Recordings endpoint (may be empty, must return 200 + segments array)
$recs = Invoke-RestMethod "http://localhost:8080/api/recordings" -ErrorAction SilentlyContinue
if ($null -ne $recs.segments) {
    Write-Host "PASS: /api/recordings returns segments array (count: $($recs.segments.Count))" -ForegroundColor Green
} else {
    Write-Host "FAIL: /api/recordings missing segments field" -ForegroundColor Red
    $pass = $false
}

# Recordings with nonexistent stream_id
$empty = Invoke-RestMethod "http://localhost:8080/api/recordings?stream_id=does-not-exist" -ErrorAction SilentlyContinue
if ($empty.segments.Count -eq 0) {
    Write-Host "PASS: /api/recordings?stream_id=nonexistent returns empty segments" -ForegroundColor Green
} else {
    Write-Host "FAIL: expected empty segments, got $($empty.segments.Count)" -ForegroundColor Red
    $pass = $false
}

# SSE endpoint: connect and check Content-Type
try {
    $req = [System.Net.HttpWebRequest]::Create("http://localhost:8080/api/streams/live")
    $req.Timeout = 3000
    $resp = $req.GetResponse()
    $ct = $resp.ContentType
    $resp.Close()
    if ($ct -like "*text/event-stream*") {
        Write-Host "PASS: /api/streams/live Content-Type is text/event-stream" -ForegroundColor Green
    } else {
        Write-Host "FAIL: /api/streams/live Content-Type = $ct" -ForegroundColor Red
        $pass = $false
    }
} catch [System.Net.WebException] {
    # Timeout is expected (SSE keeps connection open) — check response
    $resp = $_.Exception.Response
    if ($resp -and $resp.ContentType -like "*text/event-stream*") {
        Write-Host "PASS: /api/streams/live Content-Type is text/event-stream" -ForegroundColor Green
    } else {
        Write-Host "FAIL: /api/streams/live error: $($_.Exception.Message)" -ForegroundColor Red
        $pass = $false
    }
}

# CORS header
$r = Invoke-WebRequest -Uri "http://localhost:8080/health" -UseBasicParsing -ErrorAction SilentlyContinue
if ($r.Headers["Access-Control-Allow-Origin"]) {
    Write-Host "PASS: CORS header present" -ForegroundColor Green
} else {
    Write-Host "FAIL: CORS header missing" -ForegroundColor Red
    $pass = $false
}

if ($pass) { Write-Host "`nAPI server PASS" -ForegroundColor Green; exit 0 }
else { Write-Host "`nAPI server FAIL" -ForegroundColor Red; exit 1 }
```

- [ ] **Step 2: Run smoke test — confirm FAIL (stub still running)**

```powershell
docker compose up -d api-server
.\scripts\test-api-server.ps1
```

Expected: FAIL on `/health` returning JSON (nginx stub returns HTML)

- [ ] **Step 3: Replace nginx stub in docker-compose.yml**

In `docker-compose.yml`, find the `api-server` service and replace it entirely with:
```yaml
  # ── API Server ────────────────────────────────────────────────────
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

- [ ] **Step 4: Start and run smoke test**

```powershell
docker compose up -d
```

Wait 20s, then:
```powershell
.\scripts\test-api-server.ps1
```

Expected: all PASS

- [ ] **Step 5: Verify full service stack still healthy**

```powershell
.\scripts\test-all-services.ps1
```

Expected: all services PASS (api-server now returns real JSON on `/health`)

- [ ] **Step 6: Stop and commit**

```powershell
docker compose down
git add docker-compose.yml scripts/test-api-server.ps1
git commit -m "feat: replace api-server stub with real Go service in Docker Compose"
```

---

### Task 10: k3s manifests

**Files:**
- Create: `k8s/api-server/deployment.yaml`
- Create: `k8s/api-server/service.yaml`
- Delete: `k8s/stubs/api-server-stub.yaml`

- [ ] **Step 1: Create api-server Deployment**

Create `k8s/api-server/deployment.yaml`:
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
          livenessProbe:
            httpGet:
              path: /health
              port: 8080
            initialDelaySeconds: 15
            periodSeconds: 15
          resources:
            requests:
              cpu: "100m"
              memory: "64Mi"
            limits:
              cpu: "500m"
              memory: "256Mi"
```

- [ ] **Step 2: Create api-server Service**

Create `k8s/api-server/service.yaml`:
```yaml
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
      targetPort: 8080
  type: ClusterIP
```

The web-frontend calls `api-server:8080` within the cluster — no NodePort needed.

- [ ] **Step 3: Delete nginx stub**

```powershell
Remove-Item k8s\stubs\api-server-stub.yaml
```

- [ ] **Step 4: Commit**

```powershell
git add k8s/api-server/
git rm k8s/stubs/api-server-stub.yaml
git commit -m "feat: add api-server k3s Deployment + Service, remove stub"
```

---

## Verification Summary

| Check | Command | Expected |
|-------|---------|----------|
| Unit tests pass | `cd services/api-server && go test ./... -timeout 30s` | all PASS |
| Docker image builds | `docker build -f build/api-server/Dockerfile -t superstreaming/api-server:latest .` | success |
| Smoke test passes | `.\scripts\test-api-server.ps1` | all PASS |
| Full stack healthy | `.\scripts\test-all-services.ps1` | all PASS |
| k3s Deployment | `kubectl apply -f k8s/api-server/ && kubectl get pods -n superstreaming` | api-server Running |
