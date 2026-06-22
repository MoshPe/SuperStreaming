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

// StreamInfo carries a stream name and which origin index hosts it.
type StreamInfo struct {
	Name        string `json:"name"`
	OriginIndex int    `json:"origin_index"`
}

type streamsEvent struct {
	Active  []StreamInfo `json:"active"`
	Added   []StreamInfo `json:"added"`
	Removed []StreamInfo `json:"removed"`
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

	var prev []StreamInfo

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

// pollAll queries all origins and returns the merged set of ready streams with their origin index.
// Streams are hash-routed so each name should appear on exactly one origin; first-seen wins on conflict.
func (h *StreamsHandler) pollAll(ctx context.Context) []StreamInfo {
	seen := make(map[string]StreamInfo)
	for i, addr := range h.origins {
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
			if _, exists := seen[n]; !exists {
				seen[n] = StreamInfo{Name: n, OriginIndex: i}
			}
		}
	}
	result := make([]StreamInfo, 0, len(seen))
	for _, s := range seen {
		result = append(result, s)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result
}

// diff returns streams added to and removed from prev to reach current, keyed by name.
func diff(prev, current []StreamInfo) (added, removed []StreamInfo) {
	prevMap := make(map[string]struct{}, len(prev))
	for _, s := range prev {
		prevMap[s.Name] = struct{}{}
	}
	currMap := make(map[string]struct{}, len(current))
	for _, s := range current {
		currMap[s.Name] = struct{}{}
		if _, ok := prevMap[s.Name]; !ok {
			added = append(added, s)
		}
	}
	for _, s := range prev {
		if _, ok := currMap[s.Name]; !ok {
			removed = append(removed, s)
		}
	}
	return added, removed
}
