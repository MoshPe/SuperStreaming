package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"superstreaming/api-server/db"
	"superstreaming/api-server/handlers"
	"superstreaming/api-server/miniopresign"
)

type config struct {
	listenAddr         string
	originCount        int
	originHostTemplate string
	originAPIPort      int
	originPollInterval time.Duration
	minioEndpoint      string
	minioUser          string
	minioPassword      string
	minioBucket        string
	minioUseSSL        bool
	minioPresignExpiry time.Duration
	postgresHost       string
	postgresPort       string
	postgresUser       string
	postgresPassword   string
	postgresDB         string
	corsOrigin         string
}

func (c config) postgresDSN() string {
	return fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable",
		c.postgresUser, c.postgresPassword, c.postgresHost, c.postgresPort, c.postgresDB)
}

func (c config) originAddrs() []string {
	addrs := make([]string, c.originCount)
	for i := range addrs {
		addrs[i] = fmt.Sprintf("%s:%d", fmt.Sprintf(c.originHostTemplate, i), c.originAPIPort)
	}
	return addrs
}

func loadConfig() config {
	return config{
		listenAddr:         getEnv("LISTEN_ADDR", ":8080"),
		originCount:        getEnvInt("ORIGIN_COUNT", 3),
		originHostTemplate: getEnv("ORIGIN_HOST_TEMPLATE", "mediamtx-origin-%d"),
		originAPIPort:      getEnvInt("ORIGIN_API_PORT", 9997),
		originPollInterval: getEnvDuration("ORIGIN_POLL_INTERVAL", 3*time.Second),
		minioEndpoint:      mustEnv("MINIO_ENDPOINT"),
		minioUser:          mustEnv("MINIO_ROOT_USER"),
		minioPassword:      mustEnv("MINIO_ROOT_PASSWORD"),
		minioBucket:        mustEnv("MINIO_BUCKET"),
		minioUseSSL:        getEnvBool("MINIO_USE_SSL", false),
		minioPresignExpiry: getEnvDuration("MINIO_PRESIGN_EXPIRY", 24*time.Hour),
		postgresHost:       getEnv("POSTGRES_HOST", "postgres"),
		postgresPort:       getEnv("POSTGRES_PORT", "5432"),
		postgresUser:       mustEnv("POSTGRES_USER"),
		postgresPassword:   mustEnv("POSTGRES_PASSWORD"),
		postgresDB:         mustEnv("POSTGRES_DB"),
		corsOrigin:         getEnv("CORS_ORIGIN", "*"),
	}
}

func main() {
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stdout})

	cfg := loadConfig()

	database, err := db.Connect(cfg.postgresDSN())
	if err != nil {
		log.Fatal().Err(err).Msg("postgres connect failed")
	}
	defer database.Close()
	log.Info().Str("host", cfg.postgresHost).Msg("postgres connected")

	presigner, err := miniopresign.New(
		cfg.minioEndpoint,
		cfg.minioUser,
		cfg.minioPassword,
		cfg.minioBucket,
		cfg.minioUseSSL,
		cfg.minioPresignExpiry,
	)
	if err != nil {
		log.Fatal().Err(err).Msg("minio client init failed")
	}
	log.Info().Str("endpoint", cfg.minioEndpoint).Msg("minio client ready")

	mux := http.NewServeMux()
	mux.HandleFunc("/health", handlers.Health)
	mux.Handle("/api/recordings", handlers.NewRecordingsHandler(database, presigner))
	mux.Handle("/api/streams/live", handlers.NewStreamsHandler(cfg.originAddrs(), cfg.originPollInterval))

	srv := &http.Server{
		Addr:         cfg.listenAddr,
		Handler:      corsMiddleware(cfg.corsOrigin, mux),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 0, // SSE streams must not timeout
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Info().Str("addr", cfg.listenAddr).Msg("api-server listening")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("server error")
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Error().Err(err).Msg("graceful shutdown failed")
	}
	log.Info().Msg("api-server stopped")
}

func corsMiddleware(origin string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
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

func getEnvBool(key string, fallback bool) bool {
	if v := os.Getenv(key); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return fallback
}

func getEnvDuration(key string, fallback time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return fallback
}
