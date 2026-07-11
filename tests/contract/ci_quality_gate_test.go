package contract

import (
	"os"
	"strings"
	"testing"
)

func TestCIQualityGateRunsRequiredGoChecks(t *testing.T) {
	ci, err := os.ReadFile("../../.github/workflows/ci.yml")
	if err != nil {
		t.Fatalf("ReadFile(ci) error = %v", err)
	}
	actions, err := os.ReadFile("../../.github/workflows/actions.yml")
	if err != nil {
		t.Fatalf("ReadFile(actions) error = %v", err)
	}
	mise, err := os.ReadFile("../../.mise.toml")
	if err != nil {
		t.Fatalf("ReadFile(.mise.toml) error = %v", err)
	}
	hk, err := os.ReadFile("../../hk.pkl")
	if err != nil {
		t.Fatalf("ReadFile(hk.pkl) error = %v", err)
	}
	tasks, err := os.ReadFile("../../tasks.toml")
	if err != nil {
		t.Fatalf("ReadFile(tasks.toml) error = %v", err)
	}
	buildTask, err := os.ReadFile("../../.mise/tasks/build")
	if err != nil {
		t.Fatalf("ReadFile(.mise/tasks/build) error = %v", err)
	}
	buildEnv, err := os.ReadFile("../../.mise/tasks/lib/build-env.sh")
	if err != nil {
		t.Fatalf("ReadFile(.mise/tasks/lib/build-env.sh) error = %v", err)
	}
	combined := strings.Join([]string{
		string(ci),
		string(actions),
		string(mise),
		string(hk),
		string(tasks),
		string(buildTask),
		string(buildEnv),
	}, "\n")
	for _, want := range []string{
		"-race",
		"go vet ./...",
		"run = \"golangci-lint run ./...\"",
		"&& golangci-lint run",
		"go tool govulncheck ./...",
		"go mod tidy",
		"git diff --exit-code",
		"goreleaser check",
	} {
		if !strings.Contains(combined, want) {
			t.Fatalf("quality gates missing %q\nCI:\n%s\nmise:\n%s\nhk:\n%s\ntasks:\n%s", want, ci, mise, hk, tasks)
		}
	}
	for _, want := range []string{
		"matcra587/github-actions/.github/workflows/go-test.yml@67f0de0d0ceebe69895e868207c04e5c66b3bde8",
		"matcra587/github-actions/.github/workflows/go-lint.yml@67f0de0d0ceebe69895e868207c04e5c66b3bde8",
		"matcra587/github-actions/.github/workflows/md-lint.yml@67f0de0d0ceebe69895e868207c04e5c66b3bde8",
		"matcra587/github-actions/.github/workflows/workflow-lint.yml@67f0de0d0ceebe69895e868207c04e5c66b3bde8",
		"mise-version: \"2026.5.0\"",
		"zizmor-persona: pedantic",
		"zizmor-advanced-security: false",
		"zizmor-annotations: true",
		"lockfile = true",
		"actionlint = \"latest\"",
		// The binary linter must stay version-pinned: .golangci.yml is
		// written against a specific release, and the go.mod tool
		// directive (kept for the shared go-lint workflow) must carry the
		// same version — asserted against go.mod below.
		"golangci-lint = \"2.12.2\"",
		"cosign = \"latest\"",
		"hk = \"latest\"",
		"pkl = \"latest\"",
		"rumdl = \"latest\"",
		"zizmor = \"latest\"",
		"shellcheck = \"latest\"",
		"[task_config]",
		"includes = [\"tasks.toml\"]",
		"Builtins.actionlint",
		"Builtins.zizmor",
	} {
		if !strings.Contains(combined, want) {
			t.Fatalf("shared workflow gate missing %q\nCI:\n%s\nActions:\n%s", want, ci, actions)
		}
	}
	for _, unwanted := range []string{"biome =", "bun =", "node ="} {
		if strings.Contains(string(mise), unwanted) {
			t.Fatalf("mise config should not include node-related tool %q:\n%s", unwanted, mise)
		}
	}
	// The mise binary pin and the go.mod tool pin must move in lockstep:
	// the shared go-lint workflow still lints PRs via `go tool`, so a bump
	// of one without the other lints PRs and main with different linter
	// versions.
	gomod, err := os.ReadFile("../../go.mod")
	if err != nil {
		t.Fatalf("ReadFile(go.mod) error = %v", err)
	}
	if !strings.Contains(string(gomod), "github.com/golangci/golangci-lint/v2 v2.12.2") {
		t.Fatalf("go.mod golangci-lint pin drifted from .mise.toml's 2.12.2 — bump both together")
	}
	if strings.Contains(combined, "8b104684e72bef79fca78b294accb5f789d3f202") {
		t.Fatalf("shared workflow refs should use the Slack-aligned pinned SHA, not old 8b104684 refs")
	}
}

func TestCIWorkflowGatesMainPushesForRelease(t *testing.T) {
	ci, err := os.ReadFile("../../.github/workflows/ci.yml")
	if err != nil {
		t.Fatalf("ReadFile(ci) error = %v", err)
	}
	got := string(ci)
	for _, want := range []string{
		// The release workflow awaits this gate instead of re-running the
		// suite at tag time, so main pushes must run the full local gate
		// plus the cross-platform snapshot build.
		"name: Release gate",
		"if: github.event_name == 'push'",
		"run: mise run ci",
		"run: mise run release:build",
		// Push runs group per commit so a rapid push series cannot cancel
		// a gate run that a release tag is waiting on.
		"group: ${{ github.workflow }}-${{ github.event_name == 'push' && github.sha || github.ref }}",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("ci workflow missing release gate wiring %q\n%s", want, ci)
		}
	}
	if !strings.Contains(got, "push:") || !strings.Contains(got, "- main") {
		t.Fatalf("ci workflow must trigger on pushes to main\n%s", ci)
	}
}

func TestLocalCITaskIncludesReleasePreflightInputs(t *testing.T) {
	tasks, err := os.ReadFile("../../tasks.toml")
	if err != nil {
		t.Fatalf("ReadFile(tasks.toml) error = %v", err)
	}
	got := string(tasks)
	for _, want := range []string{
		`["workflow:lint"]`,
		`run = ["actionlint .github/workflows/*.yml", "zizmor --persona pedantic .github/"]`,
		`depends = ["check", "test:integration", "tidy", "workflow:lint", "security"]`,
		`run = ["mise run ci", "mise run release:check", "mise run release:build"]`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("tasks.toml missing local CI gate %q\n%s", want, tasks)
		}
	}
}
