package contract

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestArtifactsDocumentExplicitInteractiveDashboardLaunch(t *testing.T) {
	for _, path := range []string{
		"../../README.md",
		"../../docs/man/jira.1.md",
	} {
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile(%s): %v", path, err)
		}
		content := string(b)
		for _, want := range []string{"jira -i", "jira tui"} {
			if !strings.Contains(content, want) {
				t.Fatalf("%s does not document %s", path, want)
			}
		}
		for _, forbidden := range []string{"Run `jira` or `jira tui`", "`jira` launches the persistent TUI when attached to a TTY", "default `jira` entry point launches `jira tui`"} {
			if strings.Contains(content, forbidden) {
				t.Fatalf("%s still documents implicit root TUI launch phrase %q", path, forbidden)
			}
		}
	}
}

func TestArtifactsDoNotAdvertiseUnsupportedOAuth(t *testing.T) {
	for _, path := range []string{
		"../../README.md",
		"../../docs/man/jira.1.md",
	} {
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile(%s): %v", path, err)
		}
		if strings.Contains(strings.ToLower(string(b)), "oauth") {
			t.Fatalf("%s still advertises unsupported OAuth behavior", path)
		}
	}
}

func TestRuntimeSourceHonorsStackBoundary(t *testing.T) {
	for _, root := range []string{"../../cmd", "../../internal", "../../pkg"} {
		err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			b, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			content := string(b)
			for _, forbidden := range []string{
				"github.com/spf13/viper",
				"github.com/fatih/color",
				"github.com/sirupsen/logrus",
				"go.uber.org/zap",
			} {
				if strings.Contains(content, forbidden) {
					t.Fatalf("%s violates runtime stack boundary with %q", path, forbidden)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", root, err)
		}
	}
}
