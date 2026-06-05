package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
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
		// Parse "host:port" using url.Host so IPv4/IPv6 and hostnames all work.
		u, err := url.Parse("dummy://" + addr)
		if err != nil || u.Hostname() == "" || u.Port() == "" {
			log.Warn().Str("addr", addr).Msg("bad origin addr format")
			continue
		}
		host := u.Hostname()
		port, err := strconv.Atoi(u.Port())
		if err != nil {
			log.Warn().Str("addr", addr).Msg("bad origin port")
			continue
		}
		names, err := mediamtx.ListPaths(ctx, host, port)
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
