package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestWriteConfigGetPlainRendersNestedTOML(t *testing.T) {
	var buf bytes.Buffer
	data := map[string]any{"key": "theme.name", "value": "dark"}
	if err := WriteCommandPlain(&buf, "config.get", data); err != nil {
		t.Fatalf("WriteCommandPlain() error = %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "[theme]") || !strings.Contains(out, "name = 'dark'") {
		t.Fatalf("config get did not render nested TOML:\n%s", out)
	}
	if strings.Contains(out, "INF") {
		t.Fatalf("config get TOML output carries a log line:\n%s", out)
	}
}

func TestWriteConfigGetPlainScalarKeyStaysFlat(t *testing.T) {
	var buf bytes.Buffer
	data := map[string]any{"key": "default_profile", "value": "work"}
	if err := WriteCommandPlain(&buf, "config.get", data); err != nil {
		t.Fatalf("WriteCommandPlain() error = %v", err)
	}
	if !strings.Contains(buf.String(), "default_profile = 'work'") {
		t.Fatalf("config get did not render a flat TOML key:\n%s", buf.String())
	}
}

func TestWriteConfigGetPlainFallsBackOnUnexpectedShape(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteCommandPlain(&buf, "config.get", map[string]any{"unexpected": true}); err != nil {
		t.Fatalf("WriteCommandPlain() error = %v", err)
	}
	if strings.Contains(buf.String(), "= ") && !strings.Contains(buf.String(), "unexpected") {
		t.Fatalf("config get fallback did not render the payload:\n%s", buf.String())
	}
}

func TestWriteConfigProfilePlainRendersTOMLTables(t *testing.T) {
	var buf bytes.Buffer
	data := map[string]any{
		"active_profile": "work",
		"profiles": []map[string]any{
			{"name": "work", "active": true},
			{"name": "sandbox", "active": false},
		},
	}
	if err := WriteCommandPlain(&buf, "config.profile", data); err != nil {
		t.Fatalf("WriteCommandPlain() error = %v", err)
	}
	out := buf.String()
	for _, want := range []string{"active_profile = 'work'", "[[profiles]]", "name = 'sandbox'", "active = false"} {
		if !strings.Contains(out, want) {
			t.Fatalf("config profile TOML output missing %q:\n%s", want, out)
		}
	}
}
