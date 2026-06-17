package storage

import (
	"context"
	"io"
	"time"
)

// PutObjectInput contains the bytes and metadata for storing an object.
type PutObjectInput struct {
	Key         string
	Body        io.Reader
	ContentType string
}

// PutObjectResult describes the stored object.
type PutObjectResult struct {
	Key       string
	SizeBytes int64
}

// GetObjectResult contains an object body and metadata.
type GetObjectResult struct {
	Key         string
	Body        io.ReadCloser
	ContentType string
	SizeBytes   int64
}

// ObjectStore persists binary objects such as original recording audio.
type ObjectStore interface {
	PutObject(ctx context.Context, input PutObjectInput) (PutObjectResult, error)
	GetObject(ctx context.Context, key string) (GetObjectResult, error)
	PresignGetObject(ctx context.Context, key string, ttl time.Duration) (string, error)
	DeleteObject(ctx context.Context, key string) error
}
