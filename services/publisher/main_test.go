package main

import (
	"strings"
	"testing"
)

func TestParseStreamEntry(t *testing.T) {
	tests := []struct {
		pair      string
		wantID    string
		wantSrc   string
		wantTrans string
	}{
		{"cam-01=test", "cam-01", "test", "rtsp"},
		{"cam-02=rtsp://cam/stream", "cam-02", "rtsp://cam/stream", "rtsp"},
		{"cam-03=rtsp://far-cam/stream@srt", "cam-03", "rtsp://far-cam/stream", "srt"},
		{"cam-04=test@srt", "cam-04", "test", "srt"},
		{"cam-05=rtsp://user:pass@cam/stream@srt", "cam-05", "rtsp://user:pass@cam/stream", "srt"},
		{"cam-06=rtsp://user:pass@cam/stream", "cam-06", "rtsp://user:pass@cam/stream", "rtsp"},
	}
	for _, tc := range tests {
		spec, err := parseStreamEntry(tc.pair)
		if err != nil {
			t.Fatalf("parseStreamEntry(%q) error: %v", tc.pair, err)
		}
		if spec.id != tc.wantID {
			t.Errorf("pair %q: id got %q, want %q", tc.pair, spec.id, tc.wantID)
		}
		if spec.source != tc.wantSrc {
			t.Errorf("pair %q: source got %q, want %q", tc.pair, spec.source, tc.wantSrc)
		}
		if spec.transport != tc.wantTrans {
			t.Errorf("pair %q: transport got %q, want %q", tc.pair, spec.transport, tc.wantTrans)
		}
	}
}

func TestParseStreamEntryInvalid(t *testing.T) {
	_, err := parseStreamEntry("no-equals-sign")
	if err == nil {
		t.Error("expected error for entry without '='")
	}
}

func TestTargetURLRTSP(t *testing.T) {
	cfg := config{
		originCount:        1,
		originHostTemplate: "mediamtx-origin-%d",
		originRTSPPort:     8554,
		publishSecret:      "secret",
	}
	spec := streamSpec{id: "cam-01", source: "test", transport: "rtsp"}
	got := cfg.targetURL(spec)
	want := "rtsp://publisher:secret@mediamtx-origin-0:8554/cam-01"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestTargetURLSRT(t *testing.T) {
	cfg := config{
		originCount:        1,
		originHostTemplate: "mediamtx-origin-%d",
		originSRTPort:      8890,
		publishSecret:      "secret",
		srtLatencyMS:       200,
	}
	spec := streamSpec{id: "cam-03", source: "rtsp://cam/stream", transport: "srt"}
	got := cfg.targetURL(spec)
	want := "srt://mediamtx-origin-0:8890?streamid=publish:cam-03:publisher:secret&latency=200000"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestTargetURLSRTWithPassphrase(t *testing.T) {
	cfg := config{
		originCount:        1,
		originHostTemplate: "mediamtx-origin-%d",
		originSRTPort:      8890,
		publishSecret:      "secret",
		srtLatencyMS:       200,
		srtPassphrase:      "mykey",
	}
	spec := streamSpec{id: "cam-03", source: "rtsp://cam/stream", transport: "srt"}
	got := cfg.targetURL(spec)
	want := "srt://mediamtx-origin-0:8890?streamid=publish:cam-03:publisher:secret&latency=200000&passphrase=mykey"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestTargetURLSRTLatencyScaling(t *testing.T) {
	cfg := config{
		originCount:        1,
		originHostTemplate: "mediamtx-origin-%d",
		originSRTPort:      8890,
		publishSecret:      "s",
		srtLatencyMS:       500,
	}
	spec := streamSpec{id: "x", transport: "srt"}
	got := cfg.targetURL(spec)
	if !strings.Contains(got, "latency=500000") {
		t.Errorf("expected latency=500000 (500ms * 1000), got %q", got)
	}
}
