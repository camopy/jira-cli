package editor

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/matcra587/jira-cli/internal/adf"
)

// A no-op editor round trip must preserve marks on text inside an
// opaque block — the original ADF JSON is reconstituted byte-for-byte,
// so strong/underline/textColor survive intact.
func TestRoundTripADF_OpaqueBlockPreservesMarks(t *testing.T) {
	original := `{"type":"doc","version":1,"content":[
		{"type":"paragraph","content":[
			{"type":"text","text":"hi "},
			{"type":"status","attrs":{"text":"DONE","color":"green"}}
		]}
	]}`
	doc, _, err := adf.Parse([]byte(original))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	out, _, err := RoundTripADF(context.Background(), RoundTripADFOptions{
		IssueKey:  "JCT-1",
		FieldName: "description",
		Document:  doc,
		EditFn:    func(_ context.Context, _ string) error { return nil },
	})
	if err != nil {
		t.Fatalf("RoundTripADF: %v", err)
	}
	got, err := adf.Marshal(out)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	for _, marker := range []string{`"status"`, "DONE", "green"} {
		if !strings.Contains(string(got), marker) {
			t.Fatalf("opaque inline node lost marker %q: %s", marker, got)
		}
	}
}

// An unknown top-level block (a node type the editor has no Markdown
// representation for) round-trips opaquely through a no-op edit.
func TestRoundTripADF_UnknownBlockPreserved(t *testing.T) {
	original := `{"type":"doc","version":1,"content":[
		{"type":"paragraph","content":[{"type":"text","text":"lead"}]},
		{"type":"some_future_node","attrs":{"k":"v"},"content":[
			{"type":"paragraph","content":[{"type":"text","text":"inner"}]}
		]}
	]}`
	doc, _, err := adf.Parse([]byte(original))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	out, _, err := RoundTripADF(context.Background(), RoundTripADFOptions{
		IssueKey:  "JCT-1",
		FieldName: "description",
		Document:  doc,
		EditFn:    func(_ context.Context, _ string) error { return nil },
	})
	if err != nil {
		t.Fatalf("RoundTripADF: %v", err)
	}
	got, err := adf.Marshal(out)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	for _, marker := range []string{`"some_future_node"`, `"k":"v"`, "inner"} {
		if !strings.Contains(string(got), marker) {
			t.Fatalf("unknown block lost marker %q: %s", marker, got)
		}
	}
}

// When the user corrupts the base64 payload inside an opaque fence, the
// round trip MUST fail clearly and preserve the temp file so the user
// can recover their edit — silently dropping the opaque block would
// lose Jira content.
func TestRoundTripADF_MalformedOpaqueFenceFailsAndPreservesFile(t *testing.T) {
	original := `{"type":"doc","version":1,"content":[
		{"type":"panel","attrs":{"panelType":"info"},"content":[
			{"type":"paragraph","content":[{"type":"text","text":"keep me"}]}
		]}
	]}`
	doc, _, err := adf.Parse([]byte(original))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	// The "editor" corrupts the base64 inside the opaque fence.
	corrupt := func(_ context.Context, path string) error {
		raw, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		// Replace the fenced payload line with non-base64 garbage.
		lines := strings.Split(string(raw), "\n")
		for i, ln := range lines {
			if strings.HasPrefix(ln, "```jira-adf-opaque:") {
				if i+1 < len(lines) {
					lines[i+1] = "!!! not base64 !!!"
				}
			}
		}
		return os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o600)
	}

	_, _, err = RoundTripADF(context.Background(), RoundTripADFOptions{
		IssueKey:  "JCT-1",
		FieldName: "description",
		Document:  doc,
		EditFn:    corrupt,
	})
	if err == nil {
		t.Fatal("RoundTripADF must fail when an opaque fence payload is corrupted")
	}
	preserved := extractPreservedPath(err.Error())
	if preserved == "" {
		t.Fatalf("error must name the preserved temp file path; got: %v", err)
	}
	defer func() { _ = os.Remove(preserved) }()
	if _, statErr := os.Stat(preserved); statErr != nil {
		t.Fatalf("temp file not preserved at %s: %v", preserved, statErr)
	}
}

// A no-op round trip of a document built entirely of opaque blocks
// re-marshals to the same logical ADF.
func TestRoundTripADF_NoOpPreservesPanelStructure(t *testing.T) {
	original := `{"type":"doc","version":1,"content":[
		{"type":"panel","attrs":{"panelType":"warning"},"content":[
			{"type":"paragraph","content":[{"type":"text","text":"careful"}]}
		]}
	]}`
	doc, _, err := adf.Parse([]byte(original))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	out, _, err := RoundTripADF(context.Background(), RoundTripADFOptions{
		IssueKey:  "JCT-1",
		FieldName: "description",
		Document:  doc,
		EditFn:    func(_ context.Context, _ string) error { return nil },
	})
	if err != nil {
		t.Fatalf("RoundTripADF: %v", err)
	}
	if len(out.Content) != 1 || out.Content[0].Type != "panel" {
		got, _ := json.Marshal(out)
		t.Fatalf("panel structure not preserved: %s", got)
	}
	pt, _ := out.Content[0].Attrs["panelType"].(string)
	if pt != "warning" {
		t.Fatalf("panelType lost: got %q", pt)
	}
}
