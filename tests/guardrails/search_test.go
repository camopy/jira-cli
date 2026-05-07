// MOTIVATION: manual startAt/nextPageToken management in command code
// is a recurring source of pagination bugs (off-by-one, missed last
// page, double-fetch). Comments in this file are PROVENANCE ONLY and
// MUST NOT be a source of implementation, fixtures, wording, or test
// logic.
package guardrails

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Callers MUST go through the SearchService iterator / DrainSearch
// helpers; no direct manipulation of nextPageToken or startAt is
// allowed in command/TUI code. Pagination mechanics live in pkg/jira
// only.
//
// The guard greps cmd/, internal/cli/, internal/tui/ for the literal
// strings "nextPageToken" and "startAt". The pkg/jira package is
// allowed (mechanics live there); the SearchRequest struct in pkg/jira
// is allowed to expose them.
func TestNoRawPaginationCursorsInConsumerCode(t *testing.T) {
	roots := []string{
		filepath.Join("..", "..", "cmd"),
		filepath.Join("..", "..", "internal", "cli"),
		filepath.Join("..", "..", "internal", "tui"),
	}
	for _, root := range roots {
		err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() || !strings.HasSuffix(path, ".go") {
				return nil
			}
			f, err := os.Open(path)
			if err != nil {
				return err
			}
			defer func() { _ = f.Close() }()
			scan := bufio.NewScanner(f)
			lineNum := 0
			for scan.Scan() {
				lineNum++
				line := scan.Text()
				stripped := strings.TrimSpace(line)
				if strings.HasPrefix(stripped, "//") {
					continue
				}
				for _, token := range []string{"nextPageToken", "NextPageToken", "startAt", "StartAt"} {
					if !strings.Contains(line, token) {
						continue
					}
					// Allow exempted lines that explicitly tag the use.
					if strings.Contains(line, "pagination-exempt") {
						continue
					}
					t.Errorf("%s:%d uses raw pagination cursor %q outside pkg/jira: %s", path, lineNum, token, stripped)
				}
			}
			return scan.Err()
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}
