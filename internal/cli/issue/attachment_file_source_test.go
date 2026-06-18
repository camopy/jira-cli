package issue

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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
