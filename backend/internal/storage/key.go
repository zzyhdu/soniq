package storage

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"
)

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
