package unit

import (
	"bytes"
	"strings"
	"testing"

	"github.com/matcra587/jira-cli/internal/cli"
	"github.com/matcra587/jira-cli/pkg/adf"
)

func TestPlainOutputExtractsADFText(t *testing.T) {
	doc, _, err := adf.FromMarkdownLossy("hello **world**")
	if err != nil {
		t.Fatalf("FromMarkdownLossy() error = %v", err)
	}
	var buf bytes.Buffer
	if err := cli.WritePlain(&buf, map[string]any{"description": doc}); err != nil {
		t.Fatalf("WritePlain() error = %v", err)
	}
	got := buf.String()
	if !strings.Contains(got, "INF") || !strings.Contains(got, `description="hello world"`) {
		t.Fatalf("plain output = %q", buf.String())
	}
}
