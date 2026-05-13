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
		"go tool golangci-lint run",
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
		"matcra587/github-actions/.github/workflows/go-test.yml@8b104684e72bef79fca78b294accb5f789d3f202",
		"matcra587/github-actions/.github/workflows/go-lint.yml@8b104684e72bef79fca78b294accb5f789d3f202",
		"matcra587/github-actions/.github/workflows/md-lint.yml@8b104684e72bef79fca78b294accb5f789d3f202",
		"matcra587/github-actions/.github/workflows/workflow-lint.yml@8b104684e72bef79fca78b294accb5f789d3f202",
		"zizmor-persona: pedantic",
		"lockfile = true",
		"actionlint = \"latest\"",
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
}
