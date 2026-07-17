// MOTIVATION: the activity registry is an OBSERVATIONAL record — the footer
// status slot and the operation-log overlay read it; nothing else may. Keeping
// the read side to those two surfaces stops the registry from becoming a
// backdoor re-entrancy signal (canMutate/writing/bulkPending stay the sole
// guards). Comments here are PROVENANCE ONLY and MUST NOT be a source of
// implementation, fixtures, wording, or test logic.
package guardrails

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Only the footer (core/app_chrome.go) and the operation-log overlay
// (core/log_dialog.go) may read the activity registry. The read methods are
// InFlight/Recent/Log; a call site is a "." followed by the method name and a
// paren, which never matches the method definitions in the activity package
// itself. Writers (Start/Finish/Fail) are unrestricted — this guards reads.
func TestActivityRegistryReadsStayInFooterAndLog(t *testing.T) {
	readCalls := []string{".InFlight(", ".Recent(", ".Log("}
	allowed := map[string]bool{
		"app_chrome.go": true, // the footer status slot
		"log_dialog.go": true, // the operation-log overlay
	}
	root := "../../internal/tui"
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		if allowed[filepath.Base(path)] {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, call := range readCalls {
			if strings.Contains(string(body), call) {
				t.Errorf("%s reads the activity registry (%s) — only the footer and operation log may; use Start/Finish/Fail to write", path, strings.Trim(call, ".("))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
}
