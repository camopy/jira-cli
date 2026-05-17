package jira

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
)

func TestAttachmentAddRejectsOversizedSourceBeforeHTTP(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		atomic.AddInt32(&hits, 1)
	}))
	defer srv.Close()

	service := NewAttachmentService(NewClient(WithBaseURL(srv.URL + "/")))
	_, _, err := service.Add(context.Background(), "PROJ-1", []FileSource{{
		Name:   "large.bin",
		Size:   maxAttachmentUploadBytes + 1,
		Reader: strings.NewReader(""),
	}})
	if err == nil {
		t.Fatal("Add() error = nil, want upload size refusal")
	}
	if !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("Add() error = %v, want size context", err)
	}
	if got := atomic.LoadInt32(&hits); got != 0 {
		t.Fatalf("server hits = %d, want 0", got)
	}
}

func TestAttachmentDownloadErrorCarriesRetryMetadata(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "7")
		w.Header().Set("X-RateLimit-Remaining", "2")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"errorMessages":["maintenance"]}`))
	}))
	defer srv.Close()

	service := NewAttachmentService(NewClient(WithBaseURL(srv.URL + "/")))
	_, _, err := service.Download(context.Background(), "10042")
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("Download() error = %T %[1]v, want *APIError", err)
	}
	if apiErr.RetryAfterSeconds != 7 {
		t.Fatalf("RetryAfterSeconds = %d, want 7", apiErr.RetryAfterSeconds)
	}
	if apiErr.RateLimitRemaining != 2 {
		t.Fatalf("RateLimitRemaining = %d, want 2", apiErr.RateLimitRemaining)
	}
}

func TestAttachmentDownloadWrapsTransportErrorWithContext(t *testing.T) {
	sentinel := errors.New("dial failed")
	client := NewClient(
		WithBaseURL("https://jira.example.com/"),
		WithHTTPClient(&http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return nil, sentinel
		})}),
	)
	service := NewAttachmentService(client)

	_, _, err := service.Download(context.Background(), "10042")
	if !errors.Is(err, sentinel) {
		t.Fatalf("Download() error = %v, want wrapping sentinel", err)
	}
	if !strings.Contains(err.Error(), "jira request GET /rest/api/3/attachment/content/10042") {
		t.Fatalf("Download() error lacks operation context: %v", err)
	}
}

func TestAttachmentAddDebugDoesNotLogMultipartFileBytes(t *testing.T) {
	const secret = "SECRET_ATTACHMENT_BYTES"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("Pipe() error = %v", err)
	}
	orig := os.Stderr
	os.Stderr = writer
	t.Cleanup(func() { os.Stderr = orig })

	service := NewAttachmentService(NewClient(WithBaseURL(srv.URL+"/"), WithDebug(true)))
	if _, _, err := service.Add(context.Background(), "PROJ-1", []FileSource{{
		Name:   "secret.txt",
		Size:   int64(len(secret)),
		Reader: strings.NewReader(secret),
	}}); err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	_ = writer.Close()
	logBytes, _ := io.ReadAll(reader)
	logText := string(logBytes)
	if strings.Contains(logText, secret) {
		t.Fatalf("debug log leaked multipart body:\n%s", logText)
	}
	if !strings.Contains(logText, "redacted non-json body") {
		t.Fatalf("debug log missing multipart redaction marker:\n%s", logText)
	}
}
