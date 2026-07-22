package root

import (
	"testing"

	docentcobra "github.com/matcra587/docent/cobra"
	"github.com/matcra587/docent/docenttest"

	"github.com/matcra587/jira-cli/internal/agentguides"
)

// TestAgentGuidesSatisfyTheStandard self-hosts the Agent Guide Standard's
// contract tests against the embedded guide set and the live command
// tree: frontmatter validity, the six-section shape, and every command
// path referenced by a guide existing in the binary.
func TestAgentGuidesSatisfyTheStandard(t *testing.T) {
	t.Parallel()
	cmd, _, err := NewRootCommandForTest()
	if err != nil {
		t.Fatalf("build root: %v", err)
	}
	fsys, err := agentguides.FS()
	if err != nil {
		t.Fatalf("guides fs: %v", err)
	}
	docenttest.Validate(t, fsys, docentcobra.Tree(cmd))
}
