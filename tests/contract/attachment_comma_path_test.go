package contract

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A --file value containing a comma is a single path. A comma-splitting
// flag would shatter "report,final.pdf" into two paths, and the dry-run
// preview would list two bogus files instead of the one real file.
func TestAttachmentFileFlagDoesNotSplitOnCommas(t *testing.T) {
	dir := t.TempDir()
	commaName := filepath.Join(dir, "report,final.pdf")
	if err := os.WriteFile(commaName, []byte("pdf-bytes"), 0o600); err != nil {
		t.Fatalf("write attachment: %v", err)
	}

	stdout, stderr, code := runJira(t, "issue", "attachment", "add", "PROJ-1",
		"--file", commaName, "--dry-run", "--output=json")
	if code != 0 {
		t.Fatalf("dry-run attachment add exit = %d; want 0\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}
	out := string(stdout)
	// The comma path must appear verbatim as one path; the split-half
	// "report" must not appear as a standalone path entry.
	if !strings.Contains(out, "report,final.pdf") {
		t.Fatalf("comma-containing path not preserved as a single attachment:\n%s", out)
	}
	if strings.Contains(out, `"path":"`+filepath.Join(dir, "report")+`"`) {
		t.Fatalf("--file split the path on the comma:\n%s", out)
	}
}
