package contract

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// Jira wiki markup pasted as Markdown input is normalized rather than
// mangled: `jira adf convert` accepts it in the default strict mode (the
// normalization is informational, not lossy), announces the dialect
// rewrite in the warnings array, and produces the intended ADF nodes.
func TestADFConvertNormalizesWikiMarkup(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wiki.md")
	input := "h2. Rollout\n\nrun {{helm upgrade}} per [runbook|https://example.com/rb]\n\n||Step||Owner||\n|prep|ops|\n"
	if err := os.WriteFile(path, []byte(input), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cmd := exec.Command(buildJiraBinary(t),
		"adf", "convert", "--input", path, "--output=json")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("adf convert must accept wiki markup in strict mode, got error = %v\n%s", err, out)
	}

	var env struct {
		OK   bool `json:"ok"`
		Data struct {
			Type    string `json:"type"`
			Content []struct {
				Type string `json:"type"`
			} `json:"content"`
		} `json:"data"`
		Warnings []struct {
			Type  string `json:"type"`
			Lossy bool   `json:"lossy"`
		} `json:"warnings"`
	}
	if err := json.Unmarshal(out, &env); err != nil {
		t.Fatalf("output not JSON: %v\n%s", err, out)
	}
	if !env.OK {
		t.Fatalf("expected ok envelope: %s", out)
	}

	var normalized bool
	for _, w := range env.Warnings {
		if w.Type == "markdown_dialect_normalized" {
			normalized = true
			if w.Lossy {
				t.Fatalf("dialect normalization must not be lossy: %s", out)
			}
		}
	}
	if !normalized {
		t.Fatalf("expected a markdown_dialect_normalized warning: %s", out)
	}

	kinds := map[string]bool{}
	for _, n := range env.Data.Content {
		kinds[n.Type] = true
	}
	if !kinds["heading"] || !kinds["table"] {
		t.Fatalf("wiki heading and table must convert to real ADF nodes, got %s", out)
	}
}

// Pure CommonMark input must never trigger dialect normalization — the
// byte-identity guarantee at the CLI boundary.
func TestADFConvertLeavesCommonMarkAlone(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cm.md")
	input := "## Title\n\nplain **bold** with `code` and a [link](https://example.com)\n"
	if err := os.WriteFile(path, []byte(input), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cmd := exec.Command(buildJiraBinary(t),
		"adf", "convert", "--input", path, "--output=json")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("adf convert error = %v\n%s", err, out)
	}
	var env struct {
		Warnings []struct {
			Type string `json:"type"`
		} `json:"warnings"`
	}
	if err := json.Unmarshal(out, &env); err != nil {
		t.Fatalf("output not JSON: %v\n%s", err, out)
	}
	for _, w := range env.Warnings {
		if w.Type == "markdown_dialect_normalized" {
			t.Fatalf("CommonMark input must not be dialect-normalized: %s", out)
		}
	}
}
