package relay

import (
	"context"
	"os/exec"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
)

const restartDelay = 3 * time.Second

// BuildArgs returns the ffmpeg argument slice for relaying source to target.
// source = "test" produces a synthetic H264-only test pattern.
// Any other source is treated as an RTSP URL and copied without re-encoding.
// Audio is always disabled (-an). If target starts with "srt://", the output
// container is mpegts; otherwise rtsp.
func BuildArgs(source, target string) []string {
	isSRT := strings.HasPrefix(target, "srt://")
	outFormat := "rtsp"
	if isSRT {
		outFormat = "mpegts"
	}

	if source == "test" {
		return []string{
			"-re",
			"-f", "lavfi", "-i", "testsrc=size=1280x720:rate=30",
			"-c:v", "libx264", "-preset", "ultrafast", "-tune", "zerolatency",
			"-profile:v", "baseline", "-pix_fmt", "yuv420p", "-b:v", "1000k",
			"-g", "30", "-keyint_min", "30", "-sc_threshold", "0",
			"-an",
			"-f", outFormat, target,
		}
	}
	return []string{
		"-re",
		"-rtsp_transport", "tcp",
		"-i", source,
		"-c:v", "copy",
		"-an",
		"-f", outFormat, target,
	}
}

// Run relays source → target via ffmpeg, restarting on exit until ctx is cancelled.
func Run(ctx context.Context, streamID, source, target string) {
	for {
		args := BuildArgs(source, target)
		cmd := exec.CommandContext(ctx, "ffmpeg", args...)
		log.Info().Str("stream", streamID).Str("source", source).Str("target", target).Msg("relay starting")

		if err := cmd.Run(); err != nil {
			if ctx.Err() != nil {
				log.Info().Str("stream", streamID).Msg("relay stopped (context cancelled)")
				return
			}
			log.Warn().Str("stream", streamID).Err(err).Msgf("ffmpeg exited, restarting in %s", restartDelay)
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(restartDelay):
		}
	}
}
