package relay_test

import (
	"strings"
	"testing"

	"superstreaming/publisher/relay"
)

func TestBuildArgs(t *testing.T) {
	args := relay.BuildArgs("rtsp://cam/stream", "rtsp://publisher:secret@origin-0:8554/cam-01")

	if !contains(args, "-i") {
		t.Error("missing -i flag")
	}
	sourceIdx := indexOf(args, "-i")
	if sourceIdx < 0 || args[sourceIdx+1] != "rtsp://cam/stream" {
		t.Error("source URL not after -i")
	}
	if args[len(args)-1] != "rtsp://publisher:secret@origin-0:8554/cam-01" {
		t.Errorf("last arg should be target URL, got %q", args[len(args)-1])
	}
	if !contains(args, "copy") {
		t.Error("expected codec copy for passthrough relay")
	}
	if !contains(args, "rtsp") {
		t.Error("expected -f rtsp output format")
	}
	if !contains(args, "-an") {
		t.Error("expected -an to disable audio")
	}
}

func TestBuildArgsTestPattern(t *testing.T) {
	args := relay.BuildArgs("test", "rtsp://publisher:secret@origin-0:8554/test-stream")
	joined := strings.Join(args, " ")

	if !strings.Contains(joined, "lavfi") {
		t.Error("test pattern should use -f lavfi")
	}
	if !strings.Contains(joined, "libx264") {
		t.Error("test pattern should encode with libx264")
	}
	if strings.Contains(joined, "libopus") {
		t.Error("test pattern must not include libopus (audio disabled)")
	}
	if strings.Contains(joined, "sine") {
		t.Error("test pattern must not include sine audio source")
	}
	if !contains(args, "-an") {
		t.Error("test pattern should disable audio with -an")
	}
}

func TestBuildArgsTestPatternUsesShortKeyframeInterval(t *testing.T) {
	args := relay.BuildArgs("test", "rtsp://publisher:secret@origin-0:8554/test-stream")

	gopIdx := indexOf(args, "-g")
	if gopIdx < 0 || gopIdx+1 >= len(args) {
		t.Fatal("test pattern should set GOP size with -g")
	}
	if args[gopIdx+1] != "30" {
		t.Fatalf("test pattern GOP should be 30 frames, got %q", args[gopIdx+1])
	}
	keyintIdx := indexOf(args, "-keyint_min")
	if keyintIdx < 0 || keyintIdx+1 >= len(args) {
		t.Fatal("test pattern should set keyint_min with -keyint_min")
	}
	if args[keyintIdx+1] != "30" {
		t.Fatalf("test pattern keyint_min should be 30, got %q", args[keyintIdx+1])
	}
}

func TestBuildArgsNoTimestampOverwrite(t *testing.T) {
	args := relay.BuildArgs("rtsp://cam/stream", "rtsp://publisher:secret@origin-0:8554/cam-01")
	if contains(args, "-y") {
		t.Error("-y flag must not be present")
	}
}

func TestBuildArgsSRTTarget(t *testing.T) {
	target := "srt://mediamtx-origin-0:8890?streamid=publish:cam-01:publisher:secret&latency=200000"
	args := relay.BuildArgs("rtsp://cam/stream", target)

	if args[len(args)-1] != target {
		t.Errorf("last arg should be target URL, got %q", args[len(args)-1])
	}
	if !contains(args, "mpegts") {
		t.Error("SRT target requires -f mpegts")
	}
	if contains(args, "rtsp") {
		t.Error("SRT target must not use -f rtsp")
	}
	if !contains(args, "-an") {
		t.Error("expected -an to disable audio")
	}
	if !contains(args, "copy") {
		t.Error("expected video copy for passthrough relay")
	}
}

func TestBuildArgsTestPatternSRTTarget(t *testing.T) {
	target := "srt://mediamtx-origin-0:8890?streamid=publish:test-cam:publisher:secret&latency=200000"
	args := relay.BuildArgs("test", target)
	joined := strings.Join(args, " ")

	if !contains(args, "mpegts") {
		t.Error("SRT target requires -f mpegts")
	}
	if !strings.Contains(joined, "libx264") {
		t.Error("test pattern + SRT should encode with libx264")
	}
	if !contains(args, "-an") {
		t.Error("test pattern + SRT should disable audio with -an")
	}
	if args[len(args)-1] != target {
		t.Errorf("last arg should be target URL, got %q", args[len(args)-1])
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
