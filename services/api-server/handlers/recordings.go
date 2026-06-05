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
	db        RecordingsDB
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
