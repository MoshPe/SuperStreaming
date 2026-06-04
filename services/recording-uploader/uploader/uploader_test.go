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
