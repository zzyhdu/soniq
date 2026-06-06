package storage

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLocalStorePutObjectWritesBytesAndReturnsSize(t *testing.T) {
	root := t.TempDir()
	store := NewLocalStore(root)

	result, err := store.PutObject(context.Background(), PutObjectInput{
		Key:         "recordings/rec_123/original.wav",
		Body:        strings.NewReader("audio-bytes"),
		ContentType: "audio/wav",
	})
	if err != nil {
		t.Fatalf("PutObject returned error: %v", err)
	}

	if result.Key != "recordings/rec_123/original.wav" {
		t.Fatalf("result.Key = %q, want object key", result.Key)
	}
	if result.SizeBytes != int64(len("audio-bytes")) {
		t.Fatalf("result.SizeBytes = %d, want %d", result.SizeBytes, len("audio-bytes"))
	}

	contents, err := os.ReadFile(filepath.Join(root, "recordings", "rec_123", "original.wav"))
	if err != nil {
		t.Fatalf("ReadFile stored object: %v", err)
	}
	if string(contents) != "audio-bytes" {
		t.Fatalf("stored contents = %q, want audio-bytes", contents)
	}
}

func TestLocalStorePutObjectCreatesParentDirectories(t *testing.T) {
	root := t.TempDir()
	store := NewLocalStore(root)

	_, err := store.PutObject(context.Background(), PutObjectInput{
		Key:  "a/deep/object/key.bin",
		Body: strings.NewReader("x"),
	})
	if err != nil {
		t.Fatalf("PutObject returned error: %v", err)
	}

	if _, err := os.Stat(filepath.Join(root, "a", "deep", "object", "key.bin")); err != nil {
		t.Fatalf("stored object missing: %v", err)
	}
}

func TestLocalStorePutObjectRejectsInvalidKeys(t *testing.T) {
	store := NewLocalStore(t.TempDir())

	invalidKeys := []string{
		"",
		"../secret.wav",
		"recordings/../../secret.wav",
		"/absolute/path.wav",
		"recordings/rec_123/",
	}

	for _, key := range invalidKeys {
		t.Run(key, func(t *testing.T) {
			_, err := store.PutObject(context.Background(), PutObjectInput{
				Key:  key,
				Body: strings.NewReader("audio"),
			})
			if err == nil {
				t.Fatal("PutObject returned nil error, want invalid key error")
			}
		})
	}
}

func TestLocalStorePutObjectRequiresBody(t *testing.T) {
	store := NewLocalStore(t.TempDir())

	_, err := store.PutObject(context.Background(), PutObjectInput{
		Key: "recordings/rec_123/original.wav",
	})
	if err == nil {
		t.Fatal("PutObject returned nil error, want body required error")
	}
}

func TestLocalStorePutObjectPropagatesReadErrors(t *testing.T) {
	store := NewLocalStore(t.TempDir())
	readErr := errors.New("boom")

	_, err := store.PutObject(context.Background(), PutObjectInput{
		Key:  "recordings/rec_123/original.wav",
		Body: errReader{err: readErr},
	})
	if !errors.Is(err, readErr) {
		t.Fatalf("PutObject error = %v, want wrapped read error", err)
	}
}

type errReader struct {
	err error
}

func (r errReader) Read([]byte) (int, error) {
	return 0, r.err
}

var _ io.Reader = errReader{}
