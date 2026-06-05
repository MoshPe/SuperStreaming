package main

import (
	"context"
	"fmt"
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
	id     string
	source string // "test" or rtsp:// URL
}

type config struct {
	originCount        int
	originHostTemplate string
	originRTSPPort     int
	publishSecret      string
	streams            []streamSpec
}

func loadConfig() config {
	cfg := config{
		originCount:        getEnvInt("ORIGIN_COUNT", 3),
		originHostTemplate: getEnv("ORIGIN_HOST_TEMPLATE", "mediamtx-origin-%d"),
		originRTSPPort:     getEnvInt("ORIGIN_RTSP_PORT", 8554),
		publishSecret:      mustEnv("PUBLISH_SECRET"),
	}

	streamsEnv := getEnv("STREAMS", "")
	if streamsEnv == "" {
		log.Fatal().Msg("STREAMS env var required: comma-separated stream_id=source pairs, e.g. cam-01=test,cam-02=rtsp://camera/stream")
	}
	for _, pair := range strings.Split(streamsEnv, ",") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		parts := strings.SplitN(pair, "=", 2)
		if len(parts) != 2 {
			log.Fatal().Str("pair", pair).Msg("invalid STREAMS entry: expected stream_id=source")
		}
		cfg.streams = append(cfg.streams, streamSpec{id: parts[0], source: parts[1]})
	}
	if len(cfg.streams) == 0 {
		log.Fatal().Msg("STREAMS must contain at least one entry")
	}
	return cfg
}

func (c config) targetURL(streamID string) string {
	idx := hash.OriginIndex(streamID, c.originCount)
	host := fmt.Sprintf(c.originHostTemplate, idx)
	return fmt.Sprintf("rtsp://publisher:%s@%s:%d/%s",
		c.publishSecret, host, c.originRTSPPort, streamID)
}

func main() {
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stdout})

	cfg := loadConfig()

	log.Info().Int("streams", len(cfg.streams)).Int("origins", cfg.originCount).Msg("publisher starting")
	for _, s := range cfg.streams {
		idx := hash.OriginIndex(s.id, cfg.originCount)
		log.Info().Str("stream", s.id).Str("source", s.source).Int("origin", idx).Msg("stream routed")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	for _, s := range cfg.streams {
		s := s
		target := cfg.targetURL(s.id)
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
