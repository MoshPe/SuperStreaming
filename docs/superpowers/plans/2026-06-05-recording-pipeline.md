# Recording Pipeline (Sub-Project 3) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the `recording-uploader` Go sidecar that watches `/recordings`, uploads completed fMP4 segments to MinIO, indexes them in Postgres, then deletes the local copy.

**Architecture:** Single Go binary in `services/recording-uploader/`. Three packages: `db` (Postgres), `uploader` (MinIO upload + DB insert, testable via interfaces), `watcher` (fsnotify loop with debounce). Wired together in `main.go`. Runs alongside `mediamtx-origin` sharing the recordings volume.

**Tech Stack:** Go 1.23, `fsnotify` v1.7, `minio-go` v7, `lib/pq` v1.10, `zerolog` v1.33, Docker multi-stage build.

---

## File Structure

```
services/
└── recording-uploader/
    ├── go.mod
    ├── main.go                    ← config, env helpers, startup/shutdown wiring
    ├── db/
    │   └── db.go                  ← Postgres pool, Recording type, InsertRecording
    ├── uploader/
    │   ├── uploader.go            ← DB + MinioClient interfaces, Uploader, Upload, ScanOrphans, parseFilePath
    │   └── uploader_test.go       ← unit tests (mocks, no real services)
    └── watcher/
        ├── watcher.go             ← fsnotify loop, debounce, stability check
        └── watcher_test.go        ← integration test using temp dir

build/
└── recording-uploader/
    └── Dockerfile

docker-compose.yml                 ← add recording-uploader service (modify existing)
k8s/origin/statefulset.yaml        ← add sidecar container (modify existing)
scripts/test-recording-pipeline.ps1
```

---

### Task 1: Go module + skeleton

**Files:**
- Create: `services/recording-uploader/go.mod`
- Create: `services/recording-uploader/main.go`

- [ ] **Step 1: Create directories**

```powershell
mkdir services\recording-uploader\db, services\recording-uploader\uploader, services\recording-uploader\watcher
mkdir build\recording-uploader
```

- [ ] **Step 2: Initialize Go module**

```powershell
cd services\recording-uploader
go mod init superstreaming/recording-uploader
```

Expected: `go.mod` created with `module superstreaming/recording-uploader`

- [ ] **Step 3: Create main.go (skeleton — real wiring added in Task 6)**

Create `services/recording-uploader/main.go`:
```go
package main

import (
	"fmt"
	"os"
	"strconv"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

type config struct {
	recordingsDir    string
	minioEndpoint    string
	minioUser        string
	minioPassword    string
	minioBucket      string
	minioUseSSL      bool
	postgresHost     string
	postgresPort     string
	postgresUser     string
	postgresPassword string
	postgresDB       string
	orphanAgeSecs    int
	segmentDuration  int
}

func (c config) postgresDSN() string {
	return fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable",
		c.postgresUser, c.postgresPassword, c.postgresHost, c.postgresPort, c.postgresDB)
}

func loadConfig() config {
	return config{
		recordingsDir:    getEnv("RECORDINGS_DIR", "/recordings"),
		minioEndpoint:    mustEnv("MINIO_ENDPOINT"),
		minioUser:        mustEnv("MINIO_ROOT_USER"),
		minioPassword:    mustEnv("MINIO_ROOT_PASSWORD"),
		minioBucket:      mustEnv("MINIO_BUCKET"),
		minioUseSSL:      getEnv("MINIO_USE_SSL", "false") == "true",
		postgresHost:     getEnv("POSTGRES_HOST", "postgres"),
		postgresPort:     getEnv("POSTGRES_PORT", "5432"),
		postgresUser:     mustEnv("POSTGRES_USER"),
		postgresPassword: mustEnv("POSTGRES_PASSWORD"),
		postgresDB:       mustEnv("POSTGRES_DB"),
		orphanAgeSecs:    getEnvInt("ORPHAN_AGE_SECONDS", 90),
		segmentDuration:  getEnvInt("SEGMENT_DURATION_S", 60),
	}
}

func main() {
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stdout})
	log.Info().Msg("recording-uploader starting (skeleton)")
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
go get github.com/fsnotify/fsnotify@v1.7.0
go get github.com/lib/pq@v1.10.9
go get github.com/minio/minio-go/v7@v7.0.70
go mod tidy
```

- [ ] **Step 5: Verify compiles**

```powershell
go build ./...
```

Expected: no output (success)

- [ ] **Step 6: Commit**

```powershell
cd ..\..
git add services/recording-uploader/
git commit -m "feat: init recording-uploader Go module"
```

---

### Task 2: DB package

**Files:**
- Create: `services/recording-uploader/db/db.go`

- [ ] **Step 1: Create db.go**

Create `services/recording-uploader/db/db.go`:
```go
package db

import (
	"database/sql"
	"fmt"
	"time"

	_ "github.com/lib/pq"
)

// Recording is one fMP4 segment row.
type Recording struct {
	StreamID  string
	StartTime time.Time
	EndTime   time.Time
	MinioPath string
	DurationS int
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
	pool.SetMaxOpenConns(5)
	pool.SetMaxIdleConns(2)
	if err := pool.Ping(); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}
	return &DB{pool: pool}, nil
}

// Close releases all pool connections.
func (d *DB) Close() { d.pool.Close() }

// InsertRecording inserts one segment row into the recordings table.
func (d *DB) InsertRecording(r Recording) error {
	_, err := d.pool.Exec(`
		INSERT INTO recordings (stream_id, start_time, end_time, minio_path, duration_s)
		VALUES ($1, $2, $3, $4, $5)`,
		r.StreamID, r.StartTime, r.EndTime, r.MinioPath, r.DurationS,
	)
	return err
}
```

- [ ] **Step 2: Verify compiles**

```powershell
cd services/recording-uploader
go build ./...
```

Expected: no output

- [ ] **Step 3: Commit**

```powershell
cd ..\..
git add services/recording-uploader/db/
git commit -m "feat: add recording-uploader db package"
```

---

### Task 3: Uploader package — path parsing tests first

**Files:**
- Create: `services/recording-uploader/uploader/uploader.go`
- Create: `services/recording-uploader/uploader/uploader_test.go`

- [ ] **Step 1: Write failing test for parseFilePath**

Create `services/recording-uploader/uploader/uploader_test.go`:
```go
package uploader

import (
	"testing"
	"time"
)

func TestParseFilePath(t *testing.T) {
	tests := []struct {
		name      string
		path      string
		streamID  string
		startTime time.Time
		wantErr   bool
	}{
		{
			name:      "standard path",
			path:      "/recordings/cam-01/2026-06-05_10-30-00.mp4",
			streamID:  "cam-01",
			startTime: time.Date(2026, 6, 5, 10, 30, 0, 0, time.UTC),
		},
		{
			name:      "stream id with hyphens",
			path:      "/recordings/front-door/2026-01-01_00-00-00.mp4",
			streamID:  "front-door",
			startTime: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			name:    "filename not a timestamp",
			path:    "/recordings/cam-01/notadate.mp4",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			streamID, startTime, err := parseFilePath(tt.path)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if streamID != tt.streamID {
				t.Errorf("streamID = %q, want %q", streamID, tt.streamID)
			}
			if !startTime.Equal(tt.startTime) {
				t.Errorf("startTime = %v, want %v", startTime, tt.startTime)
			}
		})
	}
}
```

- [ ] **Step 2: Run test — confirm FAIL**

```powershell
cd services/recording-uploader
go test ./uploader/ -v -run TestParseFilePath
```

Expected: FAIL — `undefined: parseFilePath`

- [ ] **Step 3: Create uploader.go with parseFilePath + interfaces**

Create `services/recording-uploader/uploader/uploader.go`:
```go
package uploader

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	miniogo "github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/rs/zerolog/log"

	"superstreaming/recording-uploader/db"
)

// DBWriter is the subset of db.DB the uploader needs.
type DBWriter interface {
	InsertRecording(r db.Recording) error
}

// MinioClient is the subset of minio operations the uploader needs.
type MinioClient interface {
	FPutObject(ctx context.Context, bucket, object, filePath string) error
}

// Uploader uploads completed fMP4 segments to MinIO and records them in Postgres.
type Uploader struct {
	minio           MinioClient
	db              DBWriter
	bucket          string
	segmentDuration int
}

// Config holds resolved dependencies for New.
type Config struct {
	MinioEndpoint   string
	MinioUser       string
	MinioPassword   string
	MinioBucket     string
	MinioUseSSL     bool
	SegmentDuration int
	DB              DBWriter
}

// New creates an Uploader with a real MinIO client.
func New(cfg Config) (*Uploader, error) {
	mc, err := miniogo.New(cfg.MinioEndpoint, &miniogo.Options{
		Creds:  credentials.NewStaticV4(cfg.MinioUser, cfg.MinioPassword, ""),
		Secure: cfg.MinioUseSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("minio client: %w", err)
	}
	return &Uploader{
		minio:           &minioAdapter{mc},
		db:              cfg.DB,
		bucket:          cfg.MinioBucket,
		segmentDuration: cfg.SegmentDuration,
	}, nil
}

// minioAdapter adapts *minio.Client to MinioClient.
type minioAdapter struct{ c *miniogo.Client }

func (a *minioAdapter) FPutObject(ctx context.Context, bucket, object, filePath string) error {
	_, err := a.c.FPutObject(ctx, bucket, object, filePath, miniogo.PutObjectOptions{
		ContentType: "video/mp4",
	})
	return err
}

// Upload uploads filePath to MinIO, inserts a DB row, then deletes the local file.
// On MinIO or DB error the local file is NOT deleted (segment preserved for retry).
func (u *Uploader) Upload(ctx context.Context, filePath string) error {
	streamID, startTime, err := parseFilePath(filePath)
	if err != nil {
		return fmt.Errorf("parse path: %w", err)
	}

	objectName := fmt.Sprintf("recordings/%s/%s", streamID, filepath.Base(filePath))

	if err := u.minio.FPutObject(ctx, u.bucket, objectName, filePath); err != nil {
		return fmt.Errorf("minio upload: %w", err)
	}

	endTime := startTime.Add(time.Duration(u.segmentDuration) * time.Second)
	if err := u.db.InsertRecording(db.Recording{
		StreamID:  streamID,
		StartTime: startTime,
		EndTime:   endTime,
		MinioPath: objectName,
		DurationS: u.segmentDuration,
	}); err != nil {
		return fmt.Errorf("db insert: %w", err)
	}

	if err := os.Remove(filePath); err != nil {
		log.Warn().Str("file", filePath).Err(err).Msg("delete local file failed")
	}
	return nil
}

// ScanOrphans uploads any .mp4 in dir whose mtime is older than ageSeconds.
func (u *Uploader) ScanOrphans(ctx context.Context, dir string, ageSeconds int) error {
	cutoff := time.Now().Add(-time.Duration(ageSeconds) * time.Second)
	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || filepath.Ext(path) != ".mp4" {
			return nil
		}
		if info.ModTime().Before(cutoff) {
			log.Info().Str("file", path).Msg("recovering orphaned segment")
			if uploadErr := u.Upload(ctx, path); uploadErr != nil {
				log.Error().Str("file", path).Err(uploadErr).Msg("orphan upload failed")
			}
		}
		return nil
	})
}

// parseFilePath extracts stream_id and start_time from a MediaMTX recording path:
//
//	{dir}/{stream_id}/2026-06-05_10-30-00.mp4
func parseFilePath(filePath string) (streamID string, startTime time.Time, err error) {
	streamID = filepath.Base(filepath.Dir(filePath))
	name := strings.TrimSuffix(filepath.Base(filePath), ".mp4")
	startTime, err = time.Parse("2006-01-02_15-04-05", name)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("parse timestamp from %q: %w", name, err)
	}
	return streamID, startTime, nil
}
```

- [ ] **Step 4: Run parseFilePath test — confirm PASS**

```powershell
go test ./uploader/ -v -run TestParseFilePath
```

Expected: PASS (3 subtests)

- [ ] **Step 5: Commit**

```powershell
cd ..\..
git add services/recording-uploader/uploader/
git commit -m "feat: add uploader package with parseFilePath"
```

---

### Task 4: Uploader — Upload + ScanOrphans tests

**Files:**
- Modify: `services/recording-uploader/uploader/uploader_test.go`

- [ ] **Step 1: Add Upload tests to uploader_test.go**

Append to `services/recording-uploader/uploader/uploader_test.go`:
```go
import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"superstreaming/recording-uploader/db"
)

type mockMinio struct {
	uploaded []string
	err      error
}

func (m *mockMinio) FPutObject(_ context.Context, _, object, _ string) error {
	if m.err != nil {
		return m.err
	}
	m.uploaded = append(m.uploaded, object)
	return nil
}

type mockDB struct {
	recordings []db.Recording
	err        error
}

func (m *mockDB) InsertRecording(r db.Recording) error {
	if m.err != nil {
		return m.err
	}
	m.recordings = append(m.recordings, r)
	return nil
}

func TestUpload_HappyPath(t *testing.T) {
	dir := t.TempDir()
	streamDir := filepath.Join(dir, "cam-01")
	if err := os.MkdirAll(streamDir, 0755); err != nil {
		t.Fatal(err)
	}
	filePath := filepath.Join(streamDir, "2026-06-05_10-30-00.mp4")
	if err := os.WriteFile(filePath, []byte("fakefakefake"), 0644); err != nil {
		t.Fatal(err)
	}

	mc := &mockMinio{}
	mdb := &mockDB{}
	u := &Uploader{minio: mc, db: mdb, bucket: "recordings", segmentDuration: 60}

	if err := u.Upload(context.Background(), filePath); err != nil {
		t.Fatalf("Upload: %v", err)
	}

	if _, err := os.Stat(filePath); !errors.Is(err, os.ErrNotExist) {
		t.Error("expected file to be deleted after upload")
	}
	if len(mc.uploaded) != 1 || mc.uploaded[0] != "recordings/cam-01/2026-06-05_10-30-00.mp4" {
		t.Errorf("unexpected MinIO uploads: %v", mc.uploaded)
	}
	if len(mdb.recordings) != 1 {
		t.Fatalf("expected 1 db row, got %d", len(mdb.recordings))
	}
	r := mdb.recordings[0]
	if r.StreamID != "cam-01" {
		t.Errorf("StreamID = %q, want cam-01", r.StreamID)
	}
	if r.DurationS != 60 {
		t.Errorf("DurationS = %d, want 60", r.DurationS)
	}
	want := time.Date(2026, 6, 5, 10, 30, 0, 0, time.UTC)
	if !r.StartTime.Equal(want) {
		t.Errorf("StartTime = %v, want %v", r.StartTime, want)
	}
}

func TestUpload_MinioError_DoesNotDeleteFile(t *testing.T) {
	dir := t.TempDir()
	streamDir := filepath.Join(dir, "cam-01")
	os.MkdirAll(streamDir, 0755)
	filePath := filepath.Join(streamDir, "2026-06-05_10-30-00.mp4")
	os.WriteFile(filePath, []byte("fake"), 0644)

	mc := &mockMinio{err: errors.New("upload failed")}
	u := &Uploader{minio: mc, db: &mockDB{}, bucket: "recordings", segmentDuration: 60}

	if err := u.Upload(context.Background(), filePath); err == nil {
		t.Error("expected error, got nil")
	}
	if _, err := os.Stat(filePath); err != nil {
		t.Error("file must not be deleted on upload failure")
	}
}

func TestUpload_DBError_DoesNotDeleteFile(t *testing.T) {
	dir := t.TempDir()
	streamDir := filepath.Join(dir, "cam-01")
	os.MkdirAll(streamDir, 0755)
	filePath := filepath.Join(streamDir, "2026-06-05_10-30-00.mp4")
	os.WriteFile(filePath, []byte("fake"), 0644)

	mdb := &mockDB{err: errors.New("db down")}
	u := &Uploader{minio: &mockMinio{}, db: mdb, bucket: "recordings", segmentDuration: 60}

	if err := u.Upload(context.Background(), filePath); err == nil {
		t.Error("expected error, got nil")
	}
	if _, err := os.Stat(filePath); err != nil {
		t.Error("file must not be deleted on db failure")
	}
}
```

Note: the import block at top of file needs updating. Replace the existing import block in `uploader_test.go` with:
```go
import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"superstreaming/recording-uploader/db"
)
```

- [ ] **Step 2: Run all uploader tests — confirm PASS**

```powershell
cd services/recording-uploader
go test ./uploader/ -v
```

Expected: 5 tests PASS (TestParseFilePath/3 subtests + TestUpload_HappyPath + TestUpload_MinioError + TestUpload_DBError)

- [ ] **Step 3: Commit**

```powershell
cd ..\..
git add services/recording-uploader/uploader/uploader_test.go
git commit -m "test: add Upload unit tests with mocks"
```

---

### Task 5: Watcher package

**Files:**
- Create: `services/recording-uploader/watcher/watcher.go`
- Create: `services/recording-uploader/watcher/watcher_test.go`

- [ ] **Step 1: Write failing watcher test**

Create `services/recording-uploader/watcher/watcher_test.go`:
```go
package watcher_test

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"superstreaming/recording-uploader/watcher"
)

func TestWatcher_EmitsEventForStableFile(t *testing.T) {
	dir := t.TempDir()
	streamDir := filepath.Join(dir, "cam-01")
	if err := os.MkdirAll(streamDir, 0755); err != nil {
		t.Fatal(err)
	}

	var mu sync.Mutex
	var uploaded []string

	w, err := watcher.New(dir, func(_ context.Context, path string) error {
		mu.Lock()
		uploaded = append(uploaded, path)
		mu.Unlock()
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	go w.Run(ctx)

	time.Sleep(100 * time.Millisecond)

	filePath := filepath.Join(streamDir, "2026-06-05_10-30-00.mp4")
	if err := os.WriteFile(filePath, []byte("fakefakefake"), 0644); err != nil {
		t.Fatal(err)
	}

	deadline := time.After(5 * time.Second)
	for {
		mu.Lock()
		count := len(uploaded)
		mu.Unlock()
		if count > 0 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("timeout: upload fn not called within 5s")
		case <-time.After(100 * time.Millisecond):
		}
	}

	mu.Lock()
	defer mu.Unlock()
	if len(uploaded) != 1 || uploaded[0] != filePath {
		t.Errorf("uploads = %v, want [%s]", uploaded, filePath)
	}
}
```

- [ ] **Step 2: Run failing test**

```powershell
cd services/recording-uploader
go test ./watcher/ -v -run TestWatcher -timeout 15s
```

Expected: FAIL — `undefined: watcher.New`

- [ ] **Step 3: Create watcher.go**

Create `services/recording-uploader/watcher/watcher.go`:
```go
package watcher

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/rs/zerolog/log"
)

// UploadFn is called with the path of a complete, stable .mp4 segment.
type UploadFn func(ctx context.Context, path string) error

// Watcher watches a recordings directory tree for completed fMP4 segments.
type Watcher struct {
	dir      string
	uploadFn UploadFn
	fsw      *fsnotify.Watcher
}

// New creates a Watcher for dir, watching dir and all existing subdirectories.
func New(dir string, uploadFn UploadFn) (*Watcher, error) {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	w := &Watcher{dir: dir, uploadFn: uploadFn, fsw: fsw}
	if err := w.addTree(dir); err != nil {
		fsw.Close()
		return nil, err
	}
	return w, nil
}

func (w *Watcher) addTree(dir string) error {
	if err := w.fsw.Add(dir); err != nil {
		return err
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if e.IsDir() {
			sub := filepath.Join(dir, e.Name())
			if err := w.fsw.Add(sub); err != nil {
				log.Warn().Str("dir", sub).Err(err).Msg("watch add failed")
			}
		}
	}
	return nil
}

// Run processes fsnotify events until ctx is cancelled.
func (w *Watcher) Run(ctx context.Context) {
	defer w.fsw.Close()

	pending := make(map[string]*time.Timer)
	var mu sync.Mutex

	for {
		select {
		case <-ctx.Done():
			mu.Lock()
			for _, t := range pending {
				t.Stop()
			}
			mu.Unlock()
			return

		case event, ok := <-w.fsw.Events:
			if !ok {
				return
			}
			// Watch new stream subdirectories as MediaMTX creates them.
			if event.Has(fsnotify.Create) {
				if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
					if err := w.fsw.Add(event.Name); err != nil {
						log.Warn().Str("dir", event.Name).Err(err).Msg("watch add failed")
					}
				}
			}
			// Debounce .mp4 write/create events.
			if filepath.Ext(event.Name) == ".mp4" &&
				(event.Has(fsnotify.Write) || event.Has(fsnotify.Create)) {
				mu.Lock()
				p := event.Name
				if t, exists := pending[p]; exists {
					t.Reset(500 * time.Millisecond)
				} else {
					pending[p] = time.AfterFunc(500*time.Millisecond, func() {
						mu.Lock()
						delete(pending, p)
						mu.Unlock()
						w.handleStable(ctx, p)
					})
				}
				mu.Unlock()
			}

		case err, ok := <-w.fsw.Errors:
			if !ok {
				return
			}
			log.Error().Err(err).Msg("watcher error")
		}
	}
}

// handleStable calls uploadFn only if the file size is unchanged over 200ms.
func (w *Watcher) handleStable(ctx context.Context, path string) {
	info1, err := os.Stat(path)
	if err != nil {
		return
	}
	time.Sleep(200 * time.Millisecond)
	info2, err := os.Stat(path)
	if err != nil || info1.Size() != info2.Size() {
		return
	}
	log.Info().Str("file", path).Int64("bytes", info2.Size()).Msg("segment ready")
	if err := w.uploadFn(ctx, path); err != nil {
		log.Error().Str("file", path).Err(err).Msg("upload failed")
	}
}
```

- [ ] **Step 4: Run watcher test**

```powershell
go test ./watcher/ -v -run TestWatcher -timeout 15s
```

Expected: PASS

- [ ] **Step 5: Run all tests**

```powershell
go test ./... -timeout 30s
```

Expected: all PASS

- [ ] **Step 6: Commit**

```powershell
cd ..\..
git add services/recording-uploader/watcher/
git commit -m "feat: add recording-uploader watcher package"
```

---

### Task 6: Main entrypoint — wire everything

**Files:**
- Modify: `services/recording-uploader/main.go`

- [ ] **Step 1: Replace main.go with full wiring**

Replace `services/recording-uploader/main.go`:
```go
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"superstreaming/recording-uploader/db"
	"superstreaming/recording-uploader/uploader"
	"superstreaming/recording-uploader/watcher"
)

type config struct {
	recordingsDir    string
	minioEndpoint    string
	minioUser        string
	minioPassword    string
	minioBucket      string
	minioUseSSL      bool
	postgresHost     string
	postgresPort     string
	postgresUser     string
	postgresPassword string
	postgresDB       string
	orphanAgeSecs    int
	segmentDuration  int
}

func (c config) postgresDSN() string {
	return fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable",
		c.postgresUser, c.postgresPassword, c.postgresHost, c.postgresPort, c.postgresDB)
}

func main() {
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stdout})

	cfg := config{
		recordingsDir:    getEnv("RECORDINGS_DIR", "/recordings"),
		minioEndpoint:    mustEnv("MINIO_ENDPOINT"),
		minioUser:        mustEnv("MINIO_ROOT_USER"),
		minioPassword:    mustEnv("MINIO_ROOT_PASSWORD"),
		minioBucket:      mustEnv("MINIO_BUCKET"),
		minioUseSSL:      getEnv("MINIO_USE_SSL", "false") == "true",
		postgresHost:     getEnv("POSTGRES_HOST", "postgres"),
		postgresPort:     getEnv("POSTGRES_PORT", "5432"),
		postgresUser:     mustEnv("POSTGRES_USER"),
		postgresPassword: mustEnv("POSTGRES_PASSWORD"),
		postgresDB:       mustEnv("POSTGRES_DB"),
		orphanAgeSecs:    getEnvInt("ORPHAN_AGE_SECONDS", 90),
		segmentDuration:  getEnvInt("SEGMENT_DURATION_S", 60),
	}

	dbPool, err := db.Connect(cfg.postgresDSN())
	if err != nil {
		log.Fatal().Err(err).Msg("connect to postgres")
	}
	defer dbPool.Close()

	up, err := uploader.New(uploader.Config{
		MinioEndpoint:   cfg.minioEndpoint,
		MinioUser:       cfg.minioUser,
		MinioPassword:   cfg.minioPassword,
		MinioBucket:     cfg.minioBucket,
		MinioUseSSL:     cfg.minioUseSSL,
		SegmentDuration: cfg.segmentDuration,
		DB:              dbPool,
	})
	if err != nil {
		log.Fatal().Err(err).Msg("create uploader")
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	log.Info().Str("dir", cfg.recordingsDir).Msg("scanning orphaned segments")
	if err := up.ScanOrphans(ctx, cfg.recordingsDir, cfg.orphanAgeSecs); err != nil {
		log.Warn().Err(err).Msg("orphan scan partial error")
	}

	w, err := watcher.New(cfg.recordingsDir, up.Upload)
	if err != nil {
		log.Fatal().Err(err).Msg("create watcher")
	}

	log.Info().Str("dir", cfg.recordingsDir).Msg("watching for segments")
	w.Run(ctx)
	log.Info().Msg("shutdown complete")
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

- [ ] **Step 2: Verify full build**

```powershell
cd services/recording-uploader
go build ./...
go test ./... -timeout 30s
```

Expected: build succeeds, all tests PASS

- [ ] **Step 3: Commit**

```powershell
cd ..\..
git add services/recording-uploader/main.go
git commit -m "feat: wire recording-uploader main entrypoint"
```

---

### Task 7: Dockerfile

**Files:**
- Create: `build/recording-uploader/Dockerfile`

- [ ] **Step 1: Create Dockerfile**

Create `build/recording-uploader/Dockerfile`:
```dockerfile
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

- [ ] **Step 2: Build image**

```powershell
cd D:\Software_Projects\SuperStreaming
docker build -f build/recording-uploader/Dockerfile -t superstreaming/recording-uploader:latest .
```

Expected: image built successfully. Final line: `Successfully tagged superstreaming/recording-uploader:latest`

- [ ] **Step 3: Commit**

```powershell
git add build/recording-uploader/Dockerfile
git commit -m "feat: add recording-uploader Dockerfile"
```

---

### Task 8: Docker Compose wiring + smoke test

**Files:**
- Modify: `docker-compose.yml`
- Create: `scripts/test-recording-pipeline.ps1`

- [ ] **Step 1: Write smoke test first**

Create `scripts/test-recording-pipeline.ps1`:
```powershell
param(
    [string]$PublishSecret = $env:PUBLISH_SECRET,
    [string]$MinioUser = $env:MINIO_ROOT_USER,
    [string]$MinioPass = $env:MINIO_ROOT_PASSWORD,
    [string]$PgUser = $env:POSTGRES_USER,
    [string]$PgPass = $env:POSTGRES_PASSWORD,
    [string]$PgDb = $env:POSTGRES_DB
)

$pass = $true

# MinIO: check for at least one object under recordings/test-stream/
$cred = [Convert]::ToBase64String([Text.Encoding]::ASCII.GetBytes("${MinioUser}:${MinioPass}"))
try {
    $r = Invoke-RestMethod "http://localhost:9000/recordings?prefix=recordings/test-stream/" `
        -Headers @{ Authorization = "AWS4-HMAC-SHA256 Credential=${MinioUser}" } `
        -ErrorAction SilentlyContinue
    # Use mc alias instead — simpler
} catch {}

# Use docker exec mc to list objects
$objects = docker exec minio mc ls --json "local/recordings/recordings/test-stream/" 2>$null
if ($objects) {
    Write-Host "PASS: MinIO has objects for test-stream" -ForegroundColor Green
} else {
    Write-Host "FAIL: no MinIO objects for test-stream" -ForegroundColor Red
    $pass = $false
}

# Postgres: check recordings table has rows for test-stream
$rows = docker exec postgres psql -U $PgUser -d $PgDb -t -c "SELECT count(*) FROM recordings WHERE stream_id='test-stream';"
$count = [int]($rows.Trim())
if ($count -gt 0) {
    Write-Host "PASS: Postgres has $count recording(s) for test-stream" -ForegroundColor Green
} else {
    Write-Host "FAIL: no rows in recordings for test-stream" -ForegroundColor Red
    $pass = $false
}

# Local file deleted: recordings dir inside origin container should be empty
$files = docker exec mediamtx-origin-0 find /recordings/test-stream -name "*.mp4" 2>$null
if (-not $files) {
    Write-Host "PASS: no local .mp4 files remaining (all deleted after upload)" -ForegroundColor Green
} else {
    Write-Host "WARN: local files still present (uploader may still be processing): $files" -ForegroundColor Yellow
}

if ($pass) { Write-Host "`nRecording pipeline PASS" -ForegroundColor Green; exit 0 }
else { Write-Host "`nRecording pipeline FAIL" -ForegroundColor Red; exit 1 }
```

- [ ] **Step 2: Run smoke test — confirm FAIL (service not wired yet)**

```powershell
# Load .env
Get-Content .env | Where-Object { $_ -notmatch "^#" -and $_ -match "=" } | ForEach-Object {
    $k, $v = $_ -split "=", 2
    [System.Environment]::SetEnvironmentVariable($k.Trim(), $v.Trim())
}
.\scripts\test-recording-pipeline.ps1
```

Expected: FAIL (no MinIO objects yet)

- [ ] **Step 3: Add recording-uploader to docker-compose.yml**

Add this service to `docker-compose.yml` after the `minio-init` service:
```yaml
  # ── Recording Uploader ────────────────────────────────────────────
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

- [ ] **Step 4: Configure mc alias in minio-init**

The smoke test uses `docker exec minio mc ls`. First ensure mc alias is configured in MinIO container. Update `minio-init` entrypoint in `docker-compose.yml` to also configure the alias:
```yaml
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
```

Note: `mc` alias must be configured in the `minio` container itself for `docker exec minio mc ls` to work. Add a persistent mc config by updating the minio service or use the MinIO S3 API directly in the smoke test. Simplest fix: use the mc image for the list command:

Replace the mc check in the smoke test with:
```powershell
$objects = docker run --rm --network superstreaming_superstreaming `
    -e MC_HOST_local="http://${MinioUser}:${MinioPass}@minio:9000" `
    minio/mc:latest ls "local/recordings/recordings/test-stream/" 2>$null
```

Update `scripts/test-recording-pipeline.ps1` MinIO check block:
```powershell
$objects = docker run --rm --network superstreaming_superstreaming `
    -e "MC_HOST_local=http://${MinioUser}:${MinioPass}@minio:9000" `
    minio/mc:latest ls "local/recordings/recordings/test-stream/" 2>$null
if ($objects) {
    Write-Host "PASS: MinIO has objects for test-stream" -ForegroundColor Green
} else {
    Write-Host "FAIL: no MinIO objects for test-stream (run after publishing 65s)" -ForegroundColor Red
    $pass = $false
}
```

- [ ] **Step 5: Start full stack and publish a test stream**

```powershell
docker compose up -d
```

Wait 20s for all services healthy, then in a separate terminal publish for 70 seconds:
```powershell
Get-Content .env | Where-Object { $_ -notmatch "^#" -and $_ -match "=" } | ForEach-Object {
    $k, $v = $_ -split "=", 2; [System.Environment]::SetEnvironmentVariable($k.Trim(), $v.Trim())
}

docker run --rm --network superstreaming_superstreaming `
  linuxserver/ffmpeg `
  -re -f lavfi -i "testsrc=size=640x480:rate=25" `
  -f lavfi -i "sine=frequency=1000:sample_rate=44100" `
  -c:v libx264 -preset ultrafast -tune zerolatency -profile:v baseline `
  -b:v 500k -g 50 -keyint_min 50 `
  -c:a aac -b:a 64k `
  -t 70 `
  -f rtsp "rtsp://publisher:$($env:PUBLISH_SECRET)@mediamtx-origin-0:8554/test-stream"
```

Wait for the ffmpeg command to complete (70s). Then wait another 30s for the uploader to process.

- [ ] **Step 6: Run smoke test — confirm PASS**

```powershell
.\scripts\test-recording-pipeline.ps1
```

Expected:
```
PASS: MinIO has objects for test-stream
PASS: Postgres has 1 recording(s) for test-stream
PASS: no local .mp4 files remaining (all deleted after upload)

Recording pipeline PASS
```

- [ ] **Step 7: Stop and commit**

```powershell
docker compose down
git add docker-compose.yml scripts/test-recording-pipeline.ps1
git commit -m "feat: add recording-uploader to Docker Compose + smoke test"
```

---

### Task 9: k3s StatefulSet sidecar

**Files:**
- Modify: `k8s/origin/statefulset.yaml`

- [ ] **Step 1: Add sidecar container to StatefulSet**

In `k8s/origin/statefulset.yaml`, inside `spec.template.spec.containers`, add the following container after the `mediamtx` container:
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

The `recordings` volume is already declared in `volumeClaimTemplates` — the sidecar mounts the same PVC as the `mediamtx` container.

- [ ] **Step 2: Commit**

```powershell
git add k8s/origin/statefulset.yaml
git commit -m "feat: add recording-uploader sidecar to origin StatefulSet"
```

---

## Verification Summary

| Check | Command | Expected |
|-------|---------|----------|
| Unit tests pass | `cd services/recording-uploader && go test ./... -timeout 30s` | all PASS |
| Docker image builds | `docker build -f build/recording-uploader/Dockerfile -t superstreaming/recording-uploader:latest .` | success |
| Smoke test passes | publish 70s stream → `.\scripts\test-recording-pipeline.ps1` | all PASS |
| k3s StatefulSet | `kubectl apply -f k8s/origin/ && kubectl get pods -n superstreaming` | origin pod has 2 containers |
