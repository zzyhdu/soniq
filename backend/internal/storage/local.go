package storage

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// LocalStore stores objects under a local filesystem root.
type LocalStore struct {
	root string
}

// NewLocalStore creates a local filesystem object store rooted at root.
func NewLocalStore(root string) *LocalStore {
	return &LocalStore{root: root}
}

// LocalPathForObject resolves an object key to a safe local filesystem path.
func (s *LocalStore) LocalPathForObject(key string) (string, error) {
	key, err := cleanObjectKey(key)
	if err != nil {
		return "", err
	}
	root := s.root
	if strings.TrimSpace(root) == "" {
		root = "."
	}
	return filepath.Join(root, filepath.FromSlash(key)), nil
}

// PutObject writes an object to local storage and returns the number of bytes written.
func (s *LocalStore) PutObject(ctx context.Context, input PutObjectInput) (PutObjectResult, error) {
	if input.Body == nil {
		return PutObjectResult{}, fmt.Errorf("object body is required")
	}
	key, err := cleanObjectKey(input.Key)
	if err != nil {
		return PutObjectResult{}, err
	}
	root := s.root
	if strings.TrimSpace(root) == "" {
		root = "."
	}

	path := filepath.Join(root, filepath.FromSlash(key))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return PutObjectResult{}, fmt.Errorf("create object parent directories: %w", err)
	}

	file, err := os.Create(path)
	if err != nil {
		return PutObjectResult{}, fmt.Errorf("create object file: %w", err)
	}
	defer file.Close()

	written, err := io.Copy(file, readerWithContext{ctx: ctx, reader: input.Body})
	if err != nil {
		return PutObjectResult{}, fmt.Errorf("write object: %w", err)
	}

	return PutObjectResult{Key: key, SizeBytes: written}, nil
}

func cleanObjectKey(key string) (string, error) {
	key = strings.TrimSpace(strings.ReplaceAll(key, "\\", "/"))
	if key == "" {
		return "", fmt.Errorf("object key is required")
	}
	if strings.HasPrefix(key, "/") {
		return "", fmt.Errorf("object key must be relative")
	}
	cleaned := filepath.ToSlash(filepath.Clean(key))
	if cleaned == "." || strings.HasPrefix(cleaned, "../") || cleaned == ".." {
		return "", fmt.Errorf("object key must not traverse directories")
	}
	if strings.HasSuffix(key, "/") {
		return "", fmt.Errorf("object key must include a file name")
	}
	return cleaned, nil
}

type readerWithContext struct {
	ctx    context.Context
	reader io.Reader
}

func (r readerWithContext) Read(p []byte) (int, error) {
	select {
	case <-r.ctx.Done():
		return 0, r.ctx.Err()
	default:
		return r.reader.Read(p)
	}
}

var _ ObjectStore = (*LocalStore)(nil)
