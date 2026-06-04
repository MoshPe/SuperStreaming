package uploader

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"superstreaming/recording-uploader/db"
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

// mockMinio records FPutObject calls without touching real MinIO.
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

// mockDB records InsertRecording calls.
type mockDB struct {
	recordings []db.Recording
	err        error
}

func (m *mockDB) InsertRecording(_ context.Context, r db.Recording) error {
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
