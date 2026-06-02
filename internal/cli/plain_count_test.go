package cli

import (
	"bytes"
	"strings"
	"testing"

	termansi "github.com/gechr/x/ansi"
)

func countData() map[string]any {
	return map[string]any{
		"count":  4242,
		"jql":    "project = DEVOPS AND statusCategory != Done",
		"source": "inline",
	}
}

// `--count` output is a single number meant to pipe cleanly: its human output
// must be the bare estimate and nothing else — no INF prefix, no jql/source
// fields leaking from the envelope.
func TestCountPlainPrintsOnlyTheNumber(t *testing.T) {
	t.Parallel()
	for _, command := range []string{"issue.list.count", "search.count"} {
		var buf bytes.Buffer
		if err := WriteCommandPlain(&buf, command, countData(), WithPlainTTY(false)); err != nil {
			t.Fatalf("WriteCommandPlain(%s) error = %v", command, err)
		}
		got := termansi.Strip(buf.String())
		if strings.TrimSpace(got) != "4242" {
			t.Fatalf("%s: want only the bare count, got:\n%q", command, got)
		}
		for _, leak := range []string{"INF", "jql", "source", "project ="} {
			if strings.Contains(got, leak) {
				t.Fatalf("%s: count output leaked %q:\n%q", command, leak, got)
			}
		}
	}
}

// --debug restores the query that was counted, for troubleshooting, while the
// count itself stays present.
func TestCountPlainDebugRestoresQuery(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	if err := WriteCommandPlain(&buf, "issue.list.count", countData(), WithPlainTTY(false), WithPlainDebug(true)); err != nil {
		t.Fatalf("WriteCommandPlain error = %v", err)
	}
	got := termansi.Strip(buf.String())
	if !strings.Contains(got, "4242") {
		t.Fatalf("debug count dropped the number:\n%q", got)
	}
	if !strings.Contains(got, "project = DEVOPS AND statusCategory != Done") {
		t.Fatalf("debug count missing the counted query:\n%q", got)
	}
}
