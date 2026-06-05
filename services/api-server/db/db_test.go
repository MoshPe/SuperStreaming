package db

import (
	"testing"
	"time"
)

func TestQueryParams_Defaults(t *testing.T) {
	p := QueryParams{Limit: 0, Offset: 0}
	p.applyDefaults()
	if p.Limit != 100 {
		t.Errorf("Limit = %d, want 100", p.Limit)
	}
}

func TestQueryParams_LimitCap(t *testing.T) {
	p := QueryParams{Limit: 1000}
	p.applyDefaults()
	if p.Limit != 500 {
		t.Errorf("Limit = %d, want 500 (capped)", p.Limit)
	}
}

func TestQueryParams_TimeRange(t *testing.T) {
	from := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 6, 2, 0, 0, 0, 0, time.UTC)
	p := QueryParams{From: &from, To: &to}
	p.applyDefaults()
	if *p.From != from {
		t.Error("From changed unexpectedly")
	}
	if *p.To != to {
		t.Error("To changed unexpectedly")
	}
}
