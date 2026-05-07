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
	taskfile, err := os.ReadFile("../../Taskfile.yml")
	if err != nil {
		t.Fatalf("ReadFile(Taskfile.yml) error = %v", err)
	}
	combined := string(ci) + "\n" + string(actions) + "\n" + string(mise) + "\n" + string(hk) + "\n" + string(taskfile)
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
			t.Fatalf("quality gates missing %q\nCI:\n%s\nmise:\n%s\nhk:\n%s\nTaskfile:\n%s", want, ci, mise, hk, taskfile)
		}
	}
	for _, want := range []string{
		"matcra587/github-actions/.github/workflows/go-test.yml@8b104684e72bef79fca78b294accb5f789d3f202",
		"matcra587/github-actions/.github/workflows/go-lint.yml@8b104684e72bef79fca78b294accb5f789d3f202",
		"matcra587/github-actions/.github/workflows/md-lint.yml@8b104684e72bef79fca78b294accb5f789d3f202",
		"matcra587/github-actions/.github/workflows/workflow-lint.yml@8b104684e72bef79fca78b294accb5f789d3f202",
		"zizmor-persona: pedantic",
		"actionlint = \"1.7.12\"",
		"cosign = \"3.0.6\"",
		"hk = \"1.44.3\"",
		"rumdl = \"0.1.86\"",
		"zizmor = \"1.24.1\"",
		"Builtins.actionlint",
		"Builtins.zizmor",
	} {
		if !strings.Contains(combined, want) {
			t.Fatalf("shared workflow gate missing %q\nCI:\n%s\nActions:\n%s", want, ci, actions)
		}
	}
	for _, unwanted := range []string{"biome =", "bun =", "node =", "pkl ="} {
		if strings.Contains(string(mise), unwanted) {
			t.Fatalf("mise config should not include node-related tool %q:\n%s", unwanted, mise)
		}
	}
}
