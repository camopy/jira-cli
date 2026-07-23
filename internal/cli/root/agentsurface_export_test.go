package root

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/matcra587/jira-cli/internal/agentguides"
)

func TestAgentExportQualifiesEverySkillName(t *testing.T) {
	exportDir := t.TempDir()
	runAgentExport(t, "--format", "claude-skill", "--dir", exportDir)
	assertQualifiedSkillExports(t, exportDir)
}

func TestAgentExportAcceptsExplicitHarnessForUserScope(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	out := runAgentExport(t, "--scope", "user", "--harness", "claude-code")
	exportDir := filepath.Join(home, ".claude", "skills")

	if !strings.Contains(out, "# selected claude-code; exporting to "+exportDir) {
		t.Errorf("export report does not identify selected harness and destination:\n%s", out)
	}
	assertQualifiedSkillExports(t, exportDir)
}

func runAgentExport(t *testing.T, args ...string) string {
	t.Helper()

	cmd, _, err := NewRootCommandForTest()
	if err != nil {
		t.Fatalf("build root: %v", err)
	}

	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(append([]string{"agent", "export"}, args...))
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute agent export %v: %v\n%s", args, err, out.String())
	}

	return out.String()
}

func assertQualifiedSkillExports(t *testing.T, exportDir string) {
	t.Helper()

	guides, err := agentguides.Load()
	if err != nil {
		t.Fatalf("load agent guides: %v", err)
	}

	for _, guide := range guides.Guides() {
		qualifiedName := "jira-" + guide.Slug
		skillPath := filepath.Join(exportDir, qualifiedName, "SKILL.md")
		body, err := os.ReadFile(skillPath)
		if err != nil {
			t.Errorf("read qualified export %s: %v", skillPath, err)
			continue
		}
		if !strings.Contains(string(body), "\nname: "+qualifiedName+"\n") {
			t.Errorf("%s frontmatter does not contain name %q", skillPath, qualifiedName)
		}

		unqualifiedPath := filepath.Join(exportDir, guide.Slug, "SKILL.md")
		if _, err := os.Stat(unqualifiedPath); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("unqualified export %s exists or could not be checked: %v", unqualifiedPath, err)
		}
	}
}
