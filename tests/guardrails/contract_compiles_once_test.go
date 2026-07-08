// MOTIVATION: the contract suite drives the real jira binary end-to-end. When
// each test shells out to `go run ./cmd/jira`, the Go toolchain recompiles and
// links the whole CLI per invocation, which dominated contract-suite wall time
// (~4 minutes) and was the single largest verified CI sink. The suite must
// compile the binary once (buildJiraBinary, backed by a sync.Once `go build`)
// and exec that path; no individual contract test may `go run`/`go build` the
// CLI itself. This guardrail keeps a converted suite from silently regressing.
package guardrails

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// contractDir is the contract suite relative to this guardrail package.
const contractDir = "../contract"

// buildHarnessFile is the one file allowed to invoke `go build` for the CLI:
// it is the compile-once harness every other contract test routes through.
const buildHarnessFile = "binary_test.go"

func TestContractSuiteNeverGoRunsTheCLI(t *testing.T) {
	entries, err := os.ReadDir(contractDir)
	if err != nil {
		t.Fatalf("ReadDir(%s) error = %v", contractDir, err)
	}
	// A single test shelling out to `go run ./cmd/jira` reintroduces the
	// per-invocation recompile the compile-once harness exists to remove, so
	// the forbidden fragments are the two spellings that name the CLI package.
	forbidden := []string{
		`"run", "../../cmd/jira"`,
		`"go", "run"`,
	}
	var offenders []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, "_test.go") || name == buildHarnessFile {
			continue
		}
		body, err := os.ReadFile(filepath.Join(contractDir, name))
		if err != nil {
			t.Fatalf("ReadFile(%s) error = %v", name, err)
		}
		src := string(body)
		for _, frag := range forbidden {
			if strings.Contains(src, frag) {
				offenders = append(offenders, name+": "+frag)
			}
		}
		// `go build` of the CLI belongs only in the compile-once harness.
		if strings.Contains(src, `exec.Command("go", "build"`) {
			offenders = append(offenders, name+`: exec.Command("go", "build"`)
		}
	}
	if len(offenders) > 0 {
		t.Fatalf("contract tests must exec the compile-once binary (buildJiraBinary), not go run/go build the CLI:\n%s",
			strings.Join(offenders, "\n"))
	}
}
