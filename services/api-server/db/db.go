package db

import (
	"database/sql"
	"fmt"
	"time"

	_ "github.com/lib/pq"
)

// Recording is one row from the recordings table.
type Recording struct {
	ID        int64
	StreamID  string
	StartTime time.Time
	EndTime   time.Time
	MinioPath string
	DurationS int
}

// QueryParams filters for QueryRecordings.
type QueryParams struct {
	StreamID *string
	From     *time.Time
	To       *time.Time
	Limit    int
	Offset   int
}

func (p *QueryParams) applyDefaults() {
	if p.Limit <= 0 {
		p.Limit = 100
	}
	if p.Limit > 500 {
		p.Limit = 500
	}
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
	pool.SetMaxOpenConns(10)
	pool.SetMaxIdleConns(3)
	if err := pool.Ping(); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}
	return &DB{pool: pool}, nil
}

// Close releases all pool connections.
func (d *DB) Close() { d.pool.Close() }

// QueryRecordings returns segments matching params plus the total count.
func (d *DB) QueryRecordings(p QueryParams) ([]Recording, int, error) {
	p.applyDefaults()

	const q = `
		SELECT id, stream_id, start_time, end_time, minio_path, duration_s
		FROM recordings
		WHERE ($1::text   IS NULL OR stream_id  = $1)
		  AND ($2::timestamptz IS NULL OR start_time >= $2)
		  AND ($3::timestamptz IS NULL OR end_time   <= $3)
		ORDER BY start_time DESC
		LIMIT $4 OFFSET $5`

	rows, err := d.pool.Query(q, p.StreamID, p.From, p.To, p.Limit, p.Offset)
	if err != nil {
		return nil, 0, fmt.Errorf("query: %w", err)
	}
	defer rows.Close()

	var recs []Recording
	for rows.Next() {
		var r Recording
		if err := rows.Scan(&r.ID, &r.StreamID, &r.StartTime, &r.EndTime, &r.MinioPath, &r.DurationS); err != nil {
			return nil, 0, fmt.Errorf("scan: %w", err)
		}
		recs = append(recs, r)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("rows: %w", err)
	}

	const cq = `
		SELECT count(*)
		FROM recordings
		WHERE ($1::text   IS NULL OR stream_id  = $1)
		  AND ($2::timestamptz IS NULL OR start_time >= $2)
		  AND ($3::timestamptz IS NULL OR end_time   <= $3)`

	var total int
	if err := d.pool.QueryRow(cq, p.StreamID, p.From, p.To).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count: %w", err)
	}
	return recs, total, nil
}
