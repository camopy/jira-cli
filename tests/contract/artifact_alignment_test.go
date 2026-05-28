package contract

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// agentGuideDir is the repo-relative directory of the embedded agent
// guide. The guide is split into one `<slug>.md` file per workflow plus
// `_preamble.md`; readAgentGuide concatenates them for content
// assertions so a future restructure within the directory does not
// require updating each test.
const agentGuideDir = "../../internal/cli/agent/guide"

// readAgentGuide reads and concatenates every markdown file under
// agentGuideDir. Order is not significant — these checks only use
// strings.Contains / does-not-contain.
func readAgentGuide(t *testing.T) string {
	t.Helper()
	entries, err := os.ReadDir(agentGuideDir)
	if err != nil {
		t.Fatalf("ReadDir(%s): %v", agentGuideDir, err)
	}
	var b strings.Builder
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(agentGuideDir, e.Name()))
		if err != nil {
			t.Fatalf("ReadFile(%s): %v", e.Name(), err)
		}
		b.Write(data)
		b.WriteString("\n")
	}
	return b.String()
}

func TestArtifactsDocumentExplicitInteractiveDashboardLaunch(t *testing.T) {
	for _, path := range []string{
		"../../README.md",
		// man page removed in the docs overhaul; see docs/reference (generated)
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
		// man page removed in the docs overhaul; see docs/reference (generated)
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

func TestReadmeDocumentsReleaseVersionMetadata(t *testing.T) {
	readme, err := os.ReadFile("../../README.md")
	if err != nil {
		t.Fatalf("ReadFile(README.md): %v", err)
	}
	got := string(readme)
	for _, want := range []string{
		"Homebrew and GoReleaser release archives include release version metadata",
		"`go install github.com/matcra587/jira-cli/cmd/jira@latest`",
		"`dev`",
		"Release archives currently target macOS and Linux",
		"CGO-enabled source build for 1Password-backed profiles",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("README missing install/version contract %q\n%s", want, readme)
		}
	}
}

func TestHumanDocsUseCurrentOutputFlag(t *testing.T) {
	removedFlags := map[string]*regexp.Regexp{
		"--json":    regexp.MustCompile(`(^|[^[:alnum:]_-])--json([^[:alnum:]_-]|$)`),
		"--compact": regexp.MustCompile(`(^|[^[:alnum:]_-])--compact([^[:alnum:]_-]|$)`),
		"--plain":   regexp.MustCompile(`(^|[^[:alnum:]_-])--plain([^[:alnum:]_-]|$)`),
		"--raw":     regexp.MustCompile(`(^|[^[:alnum:]_-])--raw([^[:alnum:]_-]|$)`),
	}
	for _, path := range []string{
		"../../README.md",
		// man page removed in the docs overhaul; see docs/reference (generated)
	} {
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile(%s): %v", path, err)
		}
		got := string(b)
		if !strings.Contains(got, "--output=json") {
			t.Fatalf("%s does not document the current --output=json flag", path)
		}
		for forbidden, re := range removedFlags {
			if re.MatchString(got) {
				t.Fatalf("%s still advertises removed output flag %q", path, forbidden)
			}
		}
	}
}

func TestReadmeScopeDocsMatchCurrentCommands(t *testing.T) {
	readme, err := os.ReadFile("../../README.md")
	if err != nil {
		t.Fatalf("ReadFile(README.md): %v", err)
	}
	if strings.Contains(string(readme), "worklog delete") {
		t.Fatalf("README documents worklog delete, but the command surface only has worklog add/list")
	}
}

func TestAgentGuideRecipesMatchLiveCommandSurface(t *testing.T) {
	got := readAgentGuide(t)
	for _, want := range []string{
		"jira issue link delete KEY 9001 --force --output=json",
		"Configured backend lookup",
		"JIRA_TOKEN_<PROFILE>",
		"`AGENT=amp`",
		"`CLAUDECODE`",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("agent guide missing current recipe text %q", want)
		}
	}
	for _, forbidden := range []string{
		"jira issue link delete 9001 --force --output=json",
		"1Password (`op` CLI fallback)",
		"Backends are tried in this priority order on every API call",
		"jira issue list --all",
		"jira issue list --all --max-pages",
		"jira issue list --all --max-results-total",
		"jira issue list --all --unbounded",
	} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("agent guide still documents stale recipe %q", forbidden)
		}
	}
}

func TestJQLDocsMatchLiveBuilderDefaults(t *testing.T) {
	jql, err := os.ReadFile("../../docs/jql.md")
	if err != nil {
		t.Fatalf("ReadFile(jql) error = %v", err)
	}
	got := string(jql)
	for _, want := range []string{
		"Sort defaults to descending",
		"`--desc=false`",
		"ORDER BY updated DESC",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("JQL docs missing live builder behavior %q\n%s", want, jql)
		}
	}
	if strings.Contains(got, "default ascending") {
		t.Fatalf("JQL docs still claim ascending is the default\n%s", jql)
	}
}

func TestDocsSiteArtifactsExist(t *testing.T) {
	for _, path := range []string{
		"../../zensical.toml",
		"../../.github/workflows/docs.yml",
		"../../docs/index.md",
		"../../docs/installation.md",
		"../../docs/auth.md",
		"../../docs/output.md",
		"../../docs/issues.md",
		"../../docs/adf.md",
		"../../docs/custom-fields.md",
		"../../docs/search.md",
		"../../docs/cache.md",
		"../../docs/agent.md",
		"../../docs/stylesheets/catppuccin.css",
	} {
		if _, err := os.ReadFile(path); err != nil {
			t.Fatalf("docs site artifact %s missing: %v", path, err)
		}
	}

	tasks, err := os.ReadFile("../../tasks.toml")
	if err != nil {
		t.Fatalf("ReadFile(tasks.toml): %v", err)
	}
	for _, want := range []string{
		`["docs:build"]`,
		`uvx --from zensical zensical build --clean --strict`,
		`["docs:serve"]`,
		`file = ".mise/tasks/docs-serve"`,
	} {
		if !strings.Contains(string(tasks), want) {
			t.Fatalf("docs tooling missing %q\n%s", want, tasks)
		}
	}

	serveTask, err := os.ReadFile("../../.mise/tasks/docs-serve")
	if err != nil {
		t.Fatalf("ReadFile(.mise/tasks/docs-serve): %v", err)
	}
	for _, want := range []string{
		`local_site_url="http://localhost:8000/"`,
		`uvx --from zensical zensical build --clean --strict -f "$tmp_config"`,
		`uvx --from zensical zensical serve -f "$tmp_config" "$@"`,
	} {
		if !strings.Contains(string(serveTask), want) {
			t.Fatalf("docs serve task missing %q\n%s", want, serveTask)
		}
	}
}

func TestOnePasswordDocsExplainDesktopIntegrationPrerequisite(t *testing.T) {
	sources := map[string]func() string{
		"../../README.md": func() string {
			b, err := os.ReadFile("../../README.md")
			if err != nil {
				t.Fatalf("ReadFile(README.md): %v", err)
			}
			return string(b)
		},
		"../../docs/auth.md": func() string {
			b, err := os.ReadFile("../../docs/auth.md")
			if err != nil {
				t.Fatalf("ReadFile(docs/auth.md): %v", err)
			}
			return string(b)
		},
		"agent guide": func() string { return readAgentGuide(t) },
	}
	for label, read := range sources {
		got := read()
		for _, want := range []string{
			"Further reading",
			"https://www.1password.dev/sdks#1password-desktop-app",
			"1Password SDK desktop app integration",
			"Integrate with other apps",
			"per account and per process",
		} {
			if !strings.Contains(got, want) {
				t.Fatalf("%s missing 1Password desktop app prerequisite %q", label, want)
			}
		}
	}
}

func TestRuntimeSourceHonorsStackBoundary(t *testing.T) {
	for _, root := range []string{"../../cmd", "../../internal"} {
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
