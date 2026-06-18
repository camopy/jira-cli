//go:build unix

package issue

import (
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

// A FIFO is the simplest non-regular file to create portably, so it stands in
// for the broader "refuse anything that is not a regular file" guard.
// syscall.Mkfifo exists only on Unix, so this lives behind a `unix` build
// constraint rather than a runtime runtime.GOOS skip: a runtime skip cannot
// rescue a file that fails to COMPILE on Windows (where syscall.Mkfifo is
// undefined), which previously broke `go test ./...` for the whole package on
// Windows. On Windows there is no FIFO to attach, so dropping the test there
// loses no meaningful coverage.
func TestAttachmentFileSourcesRejectNonRegularFiles(t *testing.T) {
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
