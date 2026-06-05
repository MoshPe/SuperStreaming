package miniopresign

import (
	"context"
	"fmt"
	"net/url"
	"time"

	miniogo "github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// Client wraps a MinIO client for presigned URL generation.
type Client struct {
	mc     *miniogo.Client
	bucket string
	expiry time.Duration
}

// New creates a Client connected to endpoint.
func New(endpoint, user, password, bucket string, useSSL bool, expiry time.Duration) (*Client, error) {
	mc, err := miniogo.New(endpoint, &miniogo.Options{
		Creds:  credentials.NewStaticV4(user, password, ""),
		Secure: useSSL,
		Region: "us-east-1", // explicit region keeps presign fully offline (no GetBucketLocation lookup)
	})
	if err != nil {
		return nil, fmt.Errorf("minio client: %w", err)
	}
	return &Client{mc: mc, bucket: bucket, expiry: expiry}, nil
}

// PresignGetURL returns a presigned GET URL for objectName valid for c.expiry.
func (c *Client) PresignGetURL(ctx context.Context, objectName string) (string, error) {
	u, err := c.mc.PresignedGetObject(ctx, c.bucket, objectName, c.expiry, url.Values{})
	if err != nil {
		return "", fmt.Errorf("presign %q: %w", objectName, err)
	}
	return u.String(), nil
}
