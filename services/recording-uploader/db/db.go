package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/lib/pq"
)

// Recording is one fMP4 segment row.
type Recording struct {
	StreamID  string
	StartTime time.Time
	EndTime   time.Time
	MinioPath string
	DurationS int
}

// DB wraps a Postgres connection pool.
type DB struct {
	pool *sql.DB
}

// Connect opens a Postgres pool and verifies connectivity.
func Connect(dsn string) (*DB, error) {
	pool, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("sql.Open: %w", err)
	}
	pool.SetMaxOpenConns(5)
	pool.SetMaxIdleConns(2)
	if err := pool.Ping(); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}
	return &DB{pool: pool}, nil
}

// Close releases all pool connections.
func (d *DB) Close() { d.pool.Close() }

// InsertRecording inserts one segment row into the recordings table.
func (d *DB) InsertRecording(ctx context.Context, r Recording) error {
	_, err := d.pool.ExecContext(ctx, `
		INSERT INTO recordings (stream_id, start_time, end_time, minio_path, duration_s)
		VALUES ($1, $2, $3, $4, $5)`,
		r.StreamID, r.StartTime, r.EndTime, r.MinioPath, r.DurationS,
	)
	if err != nil {
		return fmt.Errorf("insert recording (stream_id=%s): %w", r.StreamID, err)
	}
	return nil
}
