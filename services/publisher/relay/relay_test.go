package relay_test

import (
	"strings"
	"testing"

	"superstreaming/publisher/relay"
)

func TestBuildArgs(t *testing.T) {
	args := relay.BuildArgs("rtsp://cam/stream", "rtsp://publisher:secret@origin-0:8554/cam-01")

	// Must include input
	if !contains(args, "-i") {
		t.Error("missing -i flag")
	}
	sourceIdx := indexOf(args, "-i")
	if sourceIdx < 0 || args[sourceIdx+1] != "rtsp://cam/stream" {
		t.Error("source URL not after -i")
	}

	// Must end with target URL
	if args[len(args)-1] != "rtsp://publisher:secret@origin-0:8554/cam-01" {
		t.Errorf("last arg should be target URL, got %q", args[len(args)-1])
	}

	// Must copy video codec (no re-encode for passthrough)
	if !contains(args, "copy") {
		t.Error("expected codec copy for passthrough relay")
	}

	// Must output rtsp format
	if !contains(args, "rtsp") {
		t.Error("expected -f rtsp output format")
	}
}

func TestBuildArgsTestPattern(t *testing.T) {
	args := relay.BuildArgs("test", "rtsp://publisher:secret@origin-0:8554/test-stream")

	joined := strings.Join(args, " ")

	// test pattern uses lavfi input
	if !strings.Contains(joined, "lavfi") {
		t.Error("test pattern should use -f lavfi")
	}
	// must encode H264 (can't copy synthetic stream)
	if !strings.Contains(joined, "libx264") {
		t.Error("test pattern should encode with libx264")
	}
	// must encode Opus audio
	if !strings.Contains(joined, "libopus") {
		t.Error("test pattern should encode with libopus")
	}
}

func TestBuildArgsNoTimestampOverwrite(t *testing.T) {
	args := relay.BuildArgs("rtsp://cam/stream", "rtsp://publisher:secret@origin-0:8554/cam-01")
	// -y overwrites output without asking; not wanted for RTSP streaming
	if contains(args, "-y") {
		t.Error("-y flag must not be present for RTSP output")
	}
}

func contains(args []string, s string) bool {
	for _, a := range args {
		if a == s {
			return true
		}
	}
	return false
}

func indexOf(args []string, s string) int {
	for i, a := range args {
		if a == s {
			return i
		}
	}
	return -1
}
