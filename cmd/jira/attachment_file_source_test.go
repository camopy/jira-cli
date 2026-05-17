package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
)

func TestAttachmentFileSourcesRejectNonRegularFiles(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fifo mode checks are Unix-specific")
	}
	path := filepath.Join(t.TempDir(), "pipe")
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		t.Fatalf("Mkfifo() error = %v", err)
	}

	_, err := attachmentFileSources([]string{path})
	if err == nil {
		t.Fatal("attachmentFileSources() error = nil, want non-regular file refusal")
	}
	if !strings.Contains(err.Error(), "regular file") {
		t.Fatalf("attachmentFileSources() error = %v, want regular-file context", err)
	}
}

func TestAttachmentFileSourcesRejectOversizedFiles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "large.bin")
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if err := f.Truncate(jiraMaxAttachmentUploadBytes() + 1); err != nil {
		t.Fatalf("Truncate() error = %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	_, err = attachmentFileSources([]string{path})
	if err == nil {
		t.Fatal("attachmentFileSources() error = nil, want size refusal")
	}
	if !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("attachmentFileSources() error = %v, want size context", err)
	}
}
