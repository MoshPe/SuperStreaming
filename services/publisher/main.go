package main

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"superstreaming/publisher/hash"
	"superstreaming/publisher/relay"
)

type streamSpec struct {
	id        string
	source    string // "test" or rtsp:// URL
	transport string // "rtsp" (default) or "srt"
}

type config struct {
	originCount        int
	originHostTemplate string
	originRTSPPort     int
	originSRTPort      int
	publishSecret      string
	srtLatencyMS       int
	srtPassphrase      string
	streams            []streamSpec
}

func loadConfig() config {
	cfg := config{
		originCount:        getEnvInt("ORIGIN_COUNT", 3),
		originHostTemplate: getEnv("ORIGIN_HOST_TEMPLATE", "mediamtx-origin-%d"),
		originRTSPPort:     getEnvInt("ORIGIN_RTSP_PORT", 8554),
		originSRTPort:      getEnvInt("ORIGIN_SRT_PORT", 8890),
		publishSecret:      mustEnv("PUBLISH_SECRET"),
		srtLatencyMS:       getEnvInt("SRT_LATENCY_MS", 200),
		srtPassphrase:      getEnv("SRT_PASSPHRASE", ""),
	}

	streamsEnv := getEnv("STREAMS", "")
	if streamsEnv == "" {
		log.Fatal().Msg("STREAMS env var required: comma-separated stream_id=source[@srt] pairs, e.g. cam-01=test,cam-02=rtsp://camera/stream,cam-03=rtsp://far-cam/stream@srt")
	}
	for _, pair := range strings.Split(streamsEnv, ",") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		spec, err := parseStreamEntry(pair)
		if err != nil {
			log.Fatal().Str("pair", pair).Msg(err.Error())
		}
		cfg.streams = append(cfg.streams, spec)
	}
	if len(cfg.streams) == 0 {
		log.Fatal().Msg("STREAMS must contain at least one entry")
	}
	return cfg
}

// parseStreamEntry parses one "stream_id=source[@srt]" pair.
// The @srt suffix selects SRT transport for the push leg; absent means RTSP.
func parseStreamEntry(pair string) (streamSpec, error) {
	parts := strings.SplitN(pair, "=", 2)
	if len(parts) != 2 {
		return streamSpec{}, fmt.Errorf("invalid STREAMS entry: expected stream_id=source")
	}
	source := parts[1]
	transport := "rtsp"
	if strings.HasSuffix(source, "@srt") {
		source = strings.TrimSuffix(source, "@srt")
		transport = "srt"
	}
	return streamSpec{id: parts[0], source: source, transport: transport}, nil
}

func (c config) targetURL(s streamSpec) string {
	idx := hash.OriginIndex(s.id, c.originCount)
	host := fmt.Sprintf(c.originHostTemplate, idx)
	if s.transport == "srt" {
		streamid := fmt.Sprintf("publish:%s:publisher:%s", s.id, c.publishSecret)
		u := fmt.Sprintf("srt://%s:%d?streamid=%s&latency=%d",
			host, c.originSRTPort, streamid, c.srtLatencyMS*1000)
		if c.srtPassphrase != "" {
			u += "&passphrase=" + url.QueryEscape(c.srtPassphrase)
		}
		return u
	}
	return fmt.Sprintf("rtsp://publisher:%s@%s:%d/%s",
		c.publishSecret, host, c.originRTSPPort, s.id)
}

func main() {
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stdout})

	cfg := loadConfig()

	log.Info().Int("streams", len(cfg.streams)).Int("origins", cfg.originCount).Msg("publisher starting")
	for _, s := range cfg.streams {
		idx := hash.OriginIndex(s.id, cfg.originCount)
		log.Info().Str("stream", s.id).Str("source", s.source).Str("transport", s.transport).Int("origin", idx).Msg("stream routed")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	for _, s := range cfg.streams {
		s := s
		target := cfg.targetURL(s)
		wg.Add(1)
		go func() {
			defer wg.Done()
			relay.Run(ctx, s.id, s.source, target)
		}()
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info().Msg("shutting down publisher")
	cancel()
	wg.Wait()
	log.Info().Msg("publisher stopped")
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatal().Str("var", key).Msg("required env var not set")
	}
	return v
}

func getEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}
