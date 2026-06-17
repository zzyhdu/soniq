package storage

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// S3CompatibleConfig contains connection settings for S3-compatible object storage.
type S3CompatibleConfig struct {
	Endpoint       string
	Region         string
	Bucket         string
	AccessKey      string
	SecretKey      string
	ForcePathStyle bool
}

// S3CompatibleStore stores objects in an S3-compatible bucket.
type S3CompatibleStore struct {
	bucket        string
	client        *s3.Client
	presignClient *s3.PresignClient
}

// NewS3CompatibleStore creates an object store for S3-compatible providers such as MinIO.
func NewS3CompatibleStore(ctx context.Context, cfg S3CompatibleConfig) (*S3CompatibleStore, error) {
	endpoint := strings.TrimSpace(cfg.Endpoint)
	region := strings.TrimSpace(cfg.Region)
	bucket := strings.TrimSpace(cfg.Bucket)
	accessKey := strings.TrimSpace(cfg.AccessKey)
	secretKey := strings.TrimSpace(cfg.SecretKey)
	if endpoint == "" {
		return nil, fmt.Errorf("s3 endpoint is required")
	}
	if region == "" {
		return nil, fmt.Errorf("s3 region is required")
	}
	if bucket == "" {
		return nil, fmt.Errorf("s3 bucket is required")
	}
	if accessKey == "" {
		return nil, fmt.Errorf("s3 access key is required")
	}
	if secretKey == "" {
		return nil, fmt.Errorf("s3 secret key is required")
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion(region),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKey, secretKey, "")),
		awsconfig.WithRequestChecksumCalculation(aws.RequestChecksumCalculationWhenRequired),
	)
	if err != nil {
		return nil, fmt.Errorf("load s3 config: %w", err)
	}
	client := s3.NewFromConfig(awsCfg, func(options *s3.Options) {
		options.BaseEndpoint = aws.String(endpoint)
		options.UsePathStyle = cfg.ForcePathStyle
	})
	return &S3CompatibleStore{bucket: bucket, client: client, presignClient: s3.NewPresignClient(client)}, nil
}

// PutObject stores an object and returns the number of bytes sent to S3.
func (s *S3CompatibleStore) PutObject(ctx context.Context, input PutObjectInput) (PutObjectResult, error) {
	if input.Body == nil {
		return PutObjectResult{}, fmt.Errorf("object body is required")
	}
	key, err := cleanObjectKey(input.Key)
	if err != nil {
		return PutObjectResult{}, err
	}
	body, sizeBytes, cleanup, err := stageReaderToTempFile(ctx, key, input.Body)
	if err != nil {
		return PutObjectResult{}, err
	}
	defer cleanup()
	defer body.Close()
	putInput := &s3.PutObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
		Body:   body,
	}
	if strings.TrimSpace(input.ContentType) != "" {
		putInput.ContentType = aws.String(input.ContentType)
	}
	if _, err := s.client.PutObject(ctx, putInput); err != nil {
		return PutObjectResult{}, fmt.Errorf("put s3 object: %w", err)
	}
	return PutObjectResult{Key: key, SizeBytes: sizeBytes}, nil
}

// GetObject opens an object body from S3-compatible storage.
func (s *S3CompatibleStore) GetObject(ctx context.Context, key string) (GetObjectResult, error) {
	key, err := cleanObjectKey(key)
	if err != nil {
		return GetObjectResult{}, err
	}
	output, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return GetObjectResult{}, fmt.Errorf("get s3 object: %w", err)
	}
	var sizeBytes int64
	if output.ContentLength != nil {
		sizeBytes = *output.ContentLength
	}
	result := GetObjectResult{Key: key, Body: output.Body, SizeBytes: sizeBytes}
	if output.ContentType != nil {
		result.ContentType = *output.ContentType
	}
	return result, nil
}

// Check verifies that the configured bucket is reachable.
func (s *S3CompatibleStore) Check(ctx context.Context) error {
	if _, err := s.client.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(s.bucket)}); err != nil {
		return fmt.Errorf("head s3 bucket: %w", err)
	}
	return nil
}

// PresignGetObject creates a temporary URL for reading an object.
func (s *S3CompatibleStore) PresignGetObject(ctx context.Context, key string, ttl time.Duration) (string, error) {
	key, err := cleanObjectKey(key)
	if err != nil {
		return "", err
	}
	if ttl <= 0 {
		ttl = time.Hour
	}
	output, err := s.presignClient.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	}, func(options *s3.PresignOptions) {
		options.Expires = ttl
	})
	if err != nil {
		return "", fmt.Errorf("presign s3 object: %w", err)
	}
	return output.URL, nil
}

// DeleteObject removes an object from S3-compatible storage.
func (s *S3CompatibleStore) DeleteObject(ctx context.Context, key string) error {
	key, err := cleanObjectKey(key)
	if err != nil {
		return err
	}
	if _, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	}); err != nil {
		return fmt.Errorf("delete s3 object: %w", err)
	}
	return nil
}

func stageReaderToTempFile(ctx context.Context, key string, reader io.Reader) (*os.File, int64, func(), error) {
	dir, err := os.MkdirTemp("", "soniq-s3-upload-*")
	if err != nil {
		return nil, 0, nil, fmt.Errorf("create temporary s3 upload directory: %w", err)
	}
	cleanup := func() {
		_ = os.RemoveAll(dir)
	}
	base := filepath.Base(filepath.FromSlash(key))
	if base == "." || base == "" {
		base = "object"
	}
	file, err := os.Create(filepath.Join(dir, base))
	if err != nil {
		cleanup()
		return nil, 0, nil, fmt.Errorf("create temporary s3 upload file: %w", err)
	}
	sizeBytes, err := io.Copy(file, readerWithContext{ctx: ctx, reader: reader})
	if err != nil {
		_ = file.Close()
		cleanup()
		return nil, 0, nil, fmt.Errorf("write temporary s3 upload file: %w", err)
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		_ = file.Close()
		cleanup()
		return nil, 0, nil, fmt.Errorf("seek temporary s3 upload file: %w", err)
	}
	return file, sizeBytes, cleanup, nil
}

var _ ObjectStore = (*S3CompatibleStore)(nil)
