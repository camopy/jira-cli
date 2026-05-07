// MOTIVATION: stdin discipline regressions are a recurring class in
// CLI tools that mix prompts and piped input. Comments in this file
// are PROVENANCE ONLY and MUST NOT be a source of implementation,
// fixtures, wording, or test logic.
package guardrails

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The only places allowed to read os.Stdin are the canonical
// stdin-discipline package (internal/cli/stdin/) and tests. Every
// other source-tree reference to os.Stdin is a violation.
//
// The guard greps every .go file under cmd/ and internal/ (excluding the
// allowed package and _test.go files) and fails on any os.Stdin usage.
func TestNoRogueStdinReads(t *testing.T) {
	roots := []string{
		filepath.Join("..", "..", "cmd"),
		filepath.Join("..", "..", "internal"),
		filepath.Join("..", "..", "pkg"),
	}
	allowedPaths := map[string]bool{
		filepath.Join("..", "..", "internal", "cli", "stdin"): true,
	}

	for _, root := range roots {
		err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				if allowedPaths[path] {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(path, ".go") {
				return nil
			}
			if strings.HasSuffix(path, "_test.go") {
				return nil // tests may legitimately wrap stdin
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
				// Quick filter — be tolerant of comments mentioning os.Stdin.
				if !strings.Contains(line, "os.Stdin") {
					continue
				}
				stripped := strings.TrimSpace(line)
				if strings.HasPrefix(stripped, "//") {
					continue
				}
				// Lines explicitly marked "stdin-exempt" are allowed —
				// reserved for legitimate terminal-plumbing cases like
				// the external editor spawn. Each exemption MUST be
				// justified in a trailing comment.
				if strings.Contains(line, "stdin-exempt") {
					continue
				}
				t.Errorf("%s:%d uses os.Stdin outside internal/cli/stdin/: %s", path, lineNum, stripped)
			}
			return scan.Err()
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}
