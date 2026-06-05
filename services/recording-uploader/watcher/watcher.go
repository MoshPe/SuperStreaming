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
			// Debounce .mp4 Write events. MediaMTX writes a 1s fMP4 part every
			// ~1000ms for the duration of the segment (60s). The timer resets on
			// every part write; it fires only after writes stop for 2s, meaning
			// the segment is closed and complete.
			if filepath.Ext(event.Name) == ".mp4" &&
				(event.Has(fsnotify.Write) || event.Has(fsnotify.Create)) {
				mu.Lock()
				p := event.Name
				if t, exists := pending[p]; exists {
					t.Reset(2 * time.Second)
				} else {
					pending[p] = time.AfterFunc(2*time.Second, func() {
						mu.Lock()
						delete(pending, p)
						mu.Unlock()
						if ctx.Err() != nil {
							return
						}
						info, err := os.Stat(p)
						if err != nil {
							return
						}
						log.Info().Str("file", p).Int64("bytes", info.Size()).Msg("segment ready")
						if err := w.uploadFn(ctx, p); err != nil {
							log.Error().Str("file", p).Err(err).Msg("upload failed")
						}
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
