package storage

import (
	"context"
	"io"
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

// ObjectStore persists binary objects such as original recording audio.
type ObjectStore interface {
	PutObject(ctx context.Context, input PutObjectInput) (PutObjectResult, error)
	DeleteObject(ctx context.Context, key string) error
}
