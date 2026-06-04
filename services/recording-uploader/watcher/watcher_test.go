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
