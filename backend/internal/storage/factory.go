package storage

import (
	"context"
	"fmt"
	"strings"
)

// ProviderConfig contains object storage provider settings.
type ProviderConfig struct {
	Provider         string
	S3Endpoint       string
	S3Region         string
	S3Bucket         string
	S3AccessKey      string
	S3SecretKey      string
	S3ForcePathStyle bool
}

// NewObjectStore creates the configured object store.
func NewObjectStore(ctx context.Context, cfg ProviderConfig) (ObjectStore, error) {
	switch strings.ToLower(strings.TrimSpace(cfg.Provider)) {
	case "s3_compatible":
		return NewS3CompatibleStore(ctx, S3CompatibleConfig{
			Endpoint:       cfg.S3Endpoint,
			Region:         cfg.S3Region,
			Bucket:         cfg.S3Bucket,
			AccessKey:      cfg.S3AccessKey,
			SecretKey:      cfg.S3SecretKey,
			ForcePathStyle: cfg.S3ForcePathStyle,
		})
	default:
		return nil, fmt.Errorf("unsupported storage provider %q", cfg.Provider)
	}
}
