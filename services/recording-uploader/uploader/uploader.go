package uploader

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	miniogo "github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/rs/zerolog/log"

	"superstreaming/recording-uploader/db"
)

// DBWriter is the subset of db.DB the uploader needs.
type DBWriter interface {
	InsertRecording(ctx context.Context, r db.Recording) error
}

// MinioClient is the subset of minio operations the uploader needs.
type MinioClient interface {
	FPutObject(ctx context.Context, bucket, object, filePath string) error
}

// Uploader uploads completed fMP4 segments to MinIO and records them in Postgres.
type Uploader struct {
	minio           MinioClient
	db              DBWriter
	bucket          string
	segmentDuration int
}

// Config holds resolved dependencies for New.
type Config struct {
	MinioEndpoint   string
	MinioUser       string
	MinioPassword   string
	MinioBucket     string
	MinioUseSSL     bool
	SegmentDuration int
	DB              DBWriter
}

// New creates an Uploader with a real MinIO client.
func New(cfg Config) (*Uploader, error) {
	mc, err := miniogo.New(cfg.MinioEndpoint, &miniogo.Options{
		Creds:  credentials.NewStaticV4(cfg.MinioUser, cfg.MinioPassword, ""),
		Secure: cfg.MinioUseSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("minio client: %w", err)
	}
	return &Uploader{
		minio:           &minioAdapter{mc},
		db:              cfg.DB,
		bucket:          cfg.MinioBucket,
		segmentDuration: cfg.SegmentDuration,
	}, nil
}

// minioAdapter adapts *minio.Client to MinioClient.
type minioAdapter struct{ c *miniogo.Client }

func (a *minioAdapter) FPutObject(ctx context.Context, bucket, object, filePath string) error {
	_, err := a.c.FPutObject(ctx, bucket, object, filePath, miniogo.PutObjectOptions{
		ContentType: "video/mp4",
	})
	return err
}

// Upload uploads filePath to MinIO, inserts a DB row, then deletes the local file.
// On MinIO or DB error the local file is NOT deleted (segment preserved for retry).
func (u *Uploader) Upload(ctx context.Context, filePath string) error {
	streamID, startTime, err := parseFilePath(filePath)
	if err != nil {
		return fmt.Errorf("parse path: %w", err)
	}

	objectName := fmt.Sprintf("recordings/%s/%s", streamID, filepath.Base(filePath))

	if err := u.minio.FPutObject(ctx, u.bucket, objectName, filePath); err != nil {
		return fmt.Errorf("minio upload: %w", err)
	}

	endTime := startTime.Add(time.Duration(u.segmentDuration) * time.Second)
	if err := u.db.InsertRecording(ctx, db.Recording{
		StreamID:  streamID,
		StartTime: startTime,
		EndTime:   endTime,
		MinioPath: objectName,
		DurationS: u.segmentDuration,
	}); err != nil {
		return fmt.Errorf("db insert: %w", err)
	}

	if err := os.Remove(filePath); err != nil {
		log.Warn().Str("file", filePath).Err(err).Msg("delete local file failed")
	}
	return nil
}

// ScanOrphans uploads any .mp4 in dir whose mtime is older than ageSeconds.
func (u *Uploader) ScanOrphans(ctx context.Context, dir string, ageSeconds int) error {
	cutoff := time.Now().Add(-time.Duration(ageSeconds) * time.Second)
	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || filepath.Ext(path) != ".mp4" {
			return nil
		}
		if info.ModTime().Before(cutoff) {
			log.Info().Str("file", path).Msg("recovering orphaned segment")
			if uploadErr := u.Upload(ctx, path); uploadErr != nil {
				log.Error().Str("file", path).Err(uploadErr).Msg("orphan upload failed")
			}
		}
		return nil
	})
}

// parseFilePath extracts stream_id and start_time from a MediaMTX recording path:
//
//	{dir}/{stream_id}/2026-06-05_10-30-00.mp4
func parseFilePath(filePath string) (streamID string, startTime time.Time, err error) {
	streamID = filepath.Base(filepath.Dir(filePath))
	name := strings.TrimSuffix(filepath.Base(filePath), ".mp4")
	startTime, err = time.Parse("2006-01-02_15-04-05", name)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("parse timestamp from %q: %w", name, err)
	}
	return streamID, startTime, nil
}
