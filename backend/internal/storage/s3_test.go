package storage

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestS3CompatibleStorePutGetDeleteObject(t *testing.T) {
	var storedBody string
	var sawPut bool
	var sawGet bool
	var sawDelete bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/soniq/recordings/rec_123/original.wav" {
			t.Fatalf("path = %q, want path-style bucket/object path", r.URL.Path)
		}
		switch r.Method {
		case http.MethodPut:
			sawPut = true
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read put body: %v", err)
			}
			storedBody = string(body)
			if got := r.Header.Get("Content-Type"); got != "audio/wav" {
				t.Fatalf("put content type = %q, want audio/wav", got)
			}
			w.WriteHeader(http.StatusOK)
		case http.MethodGet:
			sawGet = true
			w.Header().Set("Content-Type", "audio/wav")
			_, _ = io.WriteString(w, storedBody)
		case http.MethodDelete:
			sawDelete = true
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("method = %s, want PUT/GET/DELETE", r.Method)
		}
	}))
	defer server.Close()

	store, err := NewS3CompatibleStore(context.Background(), S3CompatibleConfig{
		Endpoint:       server.URL,
		Region:         "us-east-1",
		Bucket:         "soniq",
		AccessKey:      "test-access",
		SecretKey:      "test-secret",
		ForcePathStyle: true,
	})
	if err != nil {
		t.Fatalf("NewS3CompatibleStore returned error: %v", err)
	}

	putResult, err := store.PutObject(context.Background(), PutObjectInput{
		Key:         "recordings/rec_123/original.wav",
		Body:        strings.NewReader("audio-bytes"),
		ContentType: "audio/wav",
	})
	if err != nil {
		t.Fatalf("PutObject returned error: %v", err)
	}
	if putResult.Key != "recordings/rec_123/original.wav" || putResult.SizeBytes != int64(len("audio-bytes")) {
		t.Fatalf("PutObject result = %+v, want key and byte count", putResult)
	}

	getResult, err := store.GetObject(context.Background(), "recordings/rec_123/original.wav")
	if err != nil {
		t.Fatalf("GetObject returned error: %v", err)
	}
	defer getResult.Body.Close()
	body, err := io.ReadAll(getResult.Body)
	if err != nil {
		t.Fatalf("read object body: %v", err)
	}
	if string(body) != "audio-bytes" {
		t.Fatalf("GetObject body = %q, want audio-bytes", string(body))
	}
	if getResult.ContentType != "audio/wav" {
		t.Fatalf("GetObject content type = %q, want audio/wav", getResult.ContentType)
	}

	if err := store.DeleteObject(context.Background(), "recordings/rec_123/original.wav"); err != nil {
		t.Fatalf("DeleteObject returned error: %v", err)
	}
	if !sawPut || !sawGet || !sawDelete {
		t.Fatalf("saw put/get/delete = %v/%v/%v, want all true", sawPut, sawGet, sawDelete)
	}
}

func TestS3CompatibleStoreRejectsInvalidObjectKey(t *testing.T) {
	store, err := NewS3CompatibleStore(context.Background(), S3CompatibleConfig{
		Endpoint:       "http://127.0.0.1:1",
		Region:         "us-east-1",
		Bucket:         "soniq",
		AccessKey:      "test-access",
		SecretKey:      "test-secret",
		ForcePathStyle: true,
	})
	if err != nil {
		t.Fatalf("NewS3CompatibleStore returned error: %v", err)
	}

	if _, err := store.PutObject(context.Background(), PutObjectInput{Key: "../secret.wav", Body: strings.NewReader("x")}); err == nil {
		t.Fatal("PutObject returned nil error, want invalid key error")
	}
	if _, err := store.GetObject(context.Background(), "../secret.wav"); err == nil {
		t.Fatal("GetObject returned nil error, want invalid key error")
	}
	if err := store.DeleteObject(context.Background(), "../secret.wav"); err == nil {
		t.Fatal("DeleteObject returned nil error, want invalid key error")
	}
}

func TestS3CompatibleStorePresignsGetObject(t *testing.T) {
	store, err := NewS3CompatibleStore(context.Background(), S3CompatibleConfig{
		Endpoint:       "https://objects.example.test",
		Region:         "us-east-1",
		Bucket:         "soniq",
		AccessKey:      "test-access",
		SecretKey:      "test-secret",
		ForcePathStyle: true,
	})
	if err != nil {
		t.Fatalf("NewS3CompatibleStore returned error: %v", err)
	}

	rawURL, err := store.PresignGetObject(context.Background(), "recordings/rec_123/normalized.wav", time.Hour)
	if err != nil {
		t.Fatalf("PresignGetObject returned error: %v", err)
	}
	if !strings.HasPrefix(rawURL, "https://objects.example.test/soniq/recordings/rec_123/normalized.wav?") {
		t.Fatalf("presigned URL = %q, want configured endpoint and path-style object URL", rawURL)
	}
	if !strings.Contains(rawURL, "X-Amz-Signature=") {
		t.Fatalf("presigned URL = %q, want signature query", rawURL)
	}
}
