package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"superstreaming/recording-uploader/db"
	"superstreaming/recording-uploader/uploader"
	"superstreaming/recording-uploader/watcher"
)

type config struct {
	recordingsDir    string
	minioEndpoint    string
	minioUser        string
	minioPassword    string
	minioBucket      string
	minioUseSSL      bool
	postgresHost     string
	postgresPort     string
	postgresUser     string
	postgresPassword string
	postgresDB       string
	orphanAgeSecs int
}

func (c config) postgresDSN() string {
	return fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable",
		c.postgresUser, c.postgresPassword, c.postgresHost, c.postgresPort, c.postgresDB)
}

func main() {
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stdout})

	cfg := config{
		recordingsDir:    getEnv("RECORDINGS_DIR", "/recordings"),
		minioEndpoint:    mustEnv("MINIO_ENDPOINT"),
		minioUser:        mustEnv("MINIO_ROOT_USER"),
		minioPassword:    mustEnv("MINIO_ROOT_PASSWORD"),
		minioBucket:      mustEnv("MINIO_BUCKET"),
		minioUseSSL:      getEnv("MINIO_USE_SSL", "false") == "true",
		postgresHost:     getEnv("POSTGRES_HOST", "postgres"),
		postgresPort:     getEnv("POSTGRES_PORT", "5432"),
		postgresUser:     mustEnv("POSTGRES_USER"),
		postgresPassword: mustEnv("POSTGRES_PASSWORD"),
		postgresDB:       mustEnv("POSTGRES_DB"),
		orphanAgeSecs: getEnvInt("ORPHAN_AGE_SECONDS", 90),
	}

	dbPool, err := db.Connect(cfg.postgresDSN())
	if err != nil {
		log.Fatal().Err(err).Msg("connect to postgres")
	}
	defer dbPool.Close()

	up, err := uploader.New(uploader.Config{
		MinioEndpoint: cfg.minioEndpoint,
		MinioUser:     cfg.minioUser,
		MinioPassword: cfg.minioPassword,
		MinioBucket:   cfg.minioBucket,
		MinioUseSSL:   cfg.minioUseSSL,
		DB:            dbPool,
	})
	if err != nil {
		log.Fatal().Err(err).Msg("create uploader")
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	log.Info().Str("dir", cfg.recordingsDir).Msg("scanning orphaned segments")
	if err := up.ScanOrphans(ctx, cfg.recordingsDir, cfg.orphanAgeSecs); err != nil {
		log.Warn().Err(err).Msg("orphan scan partial error")
	}

	w, err := watcher.New(cfg.recordingsDir, up.Upload)
	if err != nil {
		log.Fatal().Err(err).Msg("create watcher")
	}

	log.Info().Str("dir", cfg.recordingsDir).Msg("watching for segments")
	w.Run(ctx)
	log.Info().Msg("shutdown complete")
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
