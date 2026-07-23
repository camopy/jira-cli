package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"
)

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("broken pipe") }

// WriteEnvelopeDocument must surface a write failure rather than swallow it —
// the whole reason the raw-warning path was moved off a bare json.Encoder onto
// the clog write-tracker path.
func TestWriteEnvelopeDocumentSurfacesWriteError(t *testing.T) {
	err := WriteEnvelopeDocument(failingWriter{}, map[string]any{"ok": true})
	if err == nil {
		t.Fatal("WriteEnvelopeDocument must return the write error, not swallow it")
	}
}

// A pre-built envelope document round-trips as valid JSON with its shape intact.
func TestWriteEnvelopeDocumentEmitsValidJSON(t *testing.T) {
	var buf bytes.Buffer
	doc := map[string]any{
		"ok":       true,
		"meta":     map[string]any{"command": "issue.list"},
		"data":     map[string]any{"issues": []any{}},
		"errors":   []any{},
		"warnings": []any{},
	}
	if err := WriteEnvelopeDocument(&buf, doc); err != nil {
		t.Fatalf("WriteEnvelopeDocument error = %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, buf.String())
	}
	if got["ok"] != true {
		t.Fatalf("ok = %v, want true", got["ok"])
	}
	meta, _ := got["meta"].(map[string]any)
	if meta["command"] != "issue.list" {
		t.Fatalf("meta.command = %v, want issue.list", meta["command"])
	}
}
