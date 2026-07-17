package goldens

import (
	"os"
	"testing"
)

// TestMain pins the editor environment for every golden: the comment form's
// ctrl+e hint renders only when an editor is configured, so frames must not
// inherit the recording machine's $EDITOR — CI has none, and a golden recorded
// with the hint drifts the moment it runs elsewhere.
func TestMain(m *testing.M) {
	for _, key := range []string{"JIRA_EDITOR", "EDITOR"} {
		if err := os.Unsetenv(key); err != nil {
			panic(err)
		}
	}
	os.Exit(m.Run())
}
