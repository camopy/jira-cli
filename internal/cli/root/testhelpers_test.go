package root

import (
	"github.com/matcra587/jira-cli/internal/cli/runtime"
	"github.com/spf13/cobra"
)

// NewRootCommandForTest builds a runtime and a fully assembled root
// command for tests. It mirrors what Execute does in production minus the
// process-exit and argv wiring, so a test can drive the real command tree
// against caller-supplied IO without touching process-global state.
//
// It returns the root command, the runtime backing it, and any runtime
// construction error so callers can assert per-instance isolation.
func NewRootCommandForTest(options ...runtime.Option) (*cobra.Command, *runtime.Runtime, error) {
	rt, err := runtime.New(options...)
	if err != nil {
		return nil, nil, err
	}
	return New(rt), rt, nil
}
