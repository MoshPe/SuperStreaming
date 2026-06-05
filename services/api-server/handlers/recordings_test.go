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
