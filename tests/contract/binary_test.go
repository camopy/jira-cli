package contract

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
)

// buildJiraBinary returns the path to a pre-built `jira` binary suitable
// for driving end-to-end tests. The binary is built once per `go test`
// invocation and reused across every test that asks for it; subtests
// that build per-iteration would otherwise spend most of their wall time
// recompiling the CLI.
func buildJiraBinary(t *testing.T) string {
	t.Helper()
	binPath, err := jiraBinary()
	if err != nil {
		t.Fatalf("build jira: %v", err)
	}
	return binPath
}

var (
	jiraBinaryOnce sync.Once
	jiraBinaryPath string
	jiraBinaryErr  error
)

func jiraBinary() (string, error) {
	jiraBinaryOnce.Do(func() {
		dir, err := os.MkdirTemp("", "jira-cli-contract-bin-*")
		if err != nil {
			jiraBinaryErr = err
			return
		}
		// go build appends .exe on Windows when -o names an extensionless
		// path, so the exec path must carry it too or lookups fail there.
		bin := filepath.Join(dir, "jira")
		if runtime.GOOS == "windows" {
			bin += ".exe"
		}
		build := exec.Command("go", "build", "-o", bin, "../../cmd/jira")
		if out, buildErr := build.CombinedOutput(); buildErr != nil {
			jiraBinaryErr = &buildError{output: string(out), err: buildErr}
			return
		}
		jiraBinaryPath = bin
	})
	return jiraBinaryPath, jiraBinaryErr
}

type buildError struct {
	output string
	err    error
}

func (e *buildError) Error() string { return e.err.Error() + "\n" + e.output }
func (e *buildError) Unwrap() error { return e.err }
