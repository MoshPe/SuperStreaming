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
			// Remove watches for deleted directories to prevent fd leak.
			if event.Has(fsnotify.Remove) {
				w.fsw.Remove(event.Name) // no-op if not watched, safe to call
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
	// Don't upload if context was cancelled (graceful shutdown).
	if ctx.Err() != nil {
		return
	}
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
