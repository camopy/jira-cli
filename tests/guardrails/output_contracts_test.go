// MOTIVATION: output-contract regressions are a recurring upstream
// class — empty lists exiting non-zero, conflicting output modes,
// --debug leaking into stdout. Comments in this file are PROVENANCE
// ONLY and MUST NOT be a source of implementation, fixtures, wording,
// or test logic.
package guardrails

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/matcra587/jira-cli/internal/cli"
	"gopkg.in/yaml.v3"
)

// Empty lists MUST be a successful exit-0 result with an empty issues
// array, not a failure or null body. We exercise the envelope layer
// directly — the CLI integration test at tests/contract/issue_list_*.go
// covers the end-to-end path.
func TestEmptyListEnvelopeIsExitZeroWithEmptyArray(t *testing.T) {
	env := cli.Envelope{
		Meta:     cli.Meta{Command: "issue.list", Timestamp: "t"},
		Data:     map[string]any{"issues": []any{}, "count": 0},
		Errors:   []cli.Error{},
		Warnings: []cli.Warning{},
	}
	buf := &bytes.Buffer{}
	if err := cli.WriteEnvelope(buf, env); err != nil {
		t.Fatalf("WriteEnvelope: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("envelope not valid JSON: %v\n%s", err, buf.String())
	}
	data, ok := got["data"].(map[string]any)
	if !ok {
		t.Fatalf("data missing or wrong type: %v", got["data"])
	}
	issues, _ := data["issues"].([]any)
	if issues == nil {
		t.Fatalf("issues serialized as null; should be empty array")
	}
	if len(issues) != 0 {
		t.Fatalf("issues should be empty, got %d", len(issues))
	}
	// Errors and warnings MUST also be empty arrays, never null.
	if got["errors"] == nil {
		t.Fatalf("errors serialized as null; should be empty array")
	}
	if got["warnings"] == nil {
		t.Fatalf("warnings serialized as null; should be empty array")
	}
}

// The JSON envelope shape MUST stay consistent across all command
// names. The required keys are {meta, data, errors, warnings};
// nothing else is implied.
func TestEnvelopeShapeIsStable(t *testing.T) {
	env := cli.Envelope{
		Meta:     cli.Meta{Command: "x", Timestamp: "t"},
		Data:     map[string]any{"k": "v"},
		Errors:   []cli.Error{},
		Warnings: []cli.Warning{},
	}
	buf := &bytes.Buffer{}
	if err := cli.WriteEnvelope(buf, env); err != nil {
		t.Fatalf("WriteEnvelope: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("not valid JSON: %v", err)
	}
	for _, key := range []string{"meta", "data", "errors", "warnings"} {
		if _, has := got[key]; !has {
			t.Fatalf("envelope missing required key %q", key)
		}
	}
}

func TestLintChecksProductionBlankErrorsWithoutDisablingTestLint(t *testing.T) {
	type exclusionRule struct {
		Path    string   `yaml:"path"`
		Linters []string `yaml:"linters"`
		Source  string   `yaml:"source"`
		Text    string   `yaml:"text"`
	}
	var cfg struct {
		Run struct {
			Tests bool `yaml:"tests"`
		} `yaml:"run"`
		Linters struct {
			Settings struct {
				Errcheck struct {
					CheckBlank bool `yaml:"check-blank"`
				} `yaml:"errcheck"`
			} `yaml:"settings"`
			Exclusions struct {
				Rules []exclusionRule `yaml:"rules"`
			} `yaml:"exclusions"`
		} `yaml:"linters"`
	}

	raw, err := os.ReadFile(filepath.Join("..", "..", ".golangci.yml"))
	if err != nil {
		t.Fatalf("read .golangci.yml: %v", err)
	}
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		t.Fatalf("parse .golangci.yml: %v", err)
	}
	if !cfg.Run.Tests {
		t.Fatal("golangci-lint run.tests = false, want tests linted")
	}
	if !cfg.Linters.Settings.Errcheck.CheckBlank {
		t.Fatal("errcheck.check-blank = false, want production blank assignments checked")
	}

	const testPath = `_test\.go$`
	var testBlankRule *exclusionRule
	for i := range cfg.Linters.Exclusions.Rules {
		rule := &cfg.Linters.Exclusions.Rules[i]
		if len(rule.Linters) == 1 && rule.Linters[0] == "errcheck" {
			if rule.Path != testPath {
				t.Fatalf("errcheck exclusion path = %q, want only %q", rule.Path, testPath)
			}
			testBlankRule = rule
		}
	}
	if testBlankRule == nil {
		t.Fatal("missing narrow _test.go errcheck blank-assignment exclusion")
	}
	if testBlankRule.Source == "" || testBlankRule.Text == "" {
		t.Fatalf("test errcheck exclusion is not limited by source and finding text: %#v", testBlankRule)
	}
}
