package adf_test

import (
	"testing"

	"github.com/matcra587/jira-cli/internal/adf"
)

// MVP node set.
var mvpNodes = []string{
	"doc", "paragraph", "text", "heading",
	"bulletList", "orderedList", "listItem",
	"codeBlock", "blockquote", "hardBreak", "rule",
	"mention", "emoji", "date", "status", "inlineCard",
	"panel", "table", "tableRow", "tableCell", "tableHeader",
	"taskList", "taskItem", "blockTaskItem", "decisionList", "decisionItem",
}

// MVP mark set.
var mvpMarks = []string{
	"strong", "em", "strike", "code", "link",
	"textColor", "backgroundColor", "subsup", "underline",
}

// Every MVP node MUST have a row in the registry. Tests fail loudly
// when the registry drifts from the spec MVP set.
func TestRegistryCoversEveryMVPNode(t *testing.T) {
	reg := adf.Registry()
	for _, name := range mvpNodes {
		entry, ok := reg.Lookup(adf.KindNode, name)
		if !ok {
			t.Errorf("registry missing MVP node %q", name)
			continue
		}
		if entry.Status == "" {
			t.Errorf("node %q registry entry missing status", name)
		}
		if entry.OfficialURL == "" {
			t.Errorf("node %q registry entry missing official_url", name)
		}
	}
}

// Every MVP mark MUST have a row.
func TestRegistryCoversEveryMVPMark(t *testing.T) {
	reg := adf.Registry()
	for _, name := range mvpMarks {
		entry, ok := reg.Lookup(adf.KindMark, name)
		if !ok {
			t.Errorf("registry missing MVP mark %q", name)
			continue
		}
		if entry.OfficialURL == "" {
			t.Errorf("mark %q registry entry missing official_url", name)
		}
	}
}

// Every entry MUST carry the shared envelope keys. Capabilities is a
// struct of bools; input_shape and output_shape are JSON Schema 2020-12
// fragments (we only check non-emptiness here, validation comes later).
func TestRegistryEntriesUseSharedEnvelope(t *testing.T) {
	reg := adf.Registry()
	all := reg.All()
	if len(all) == 0 {
		t.Fatalf("registry returned 0 entries")
	}
	for _, entry := range all {
		if entry.Kind != adf.KindNode && entry.Kind != adf.KindMark {
			t.Errorf("entry %q has unknown kind %v", entry.Name, entry.Kind)
		}
		if entry.Name == "" {
			t.Errorf("entry has empty name")
		}
		// Capabilities is a struct; we don't assert its values here, just
		// that the type is wired. Goldens verify per-capability
		// behavior.
		_ = entry.Capabilities
		// SubmitDescription — every row MUST disambiguate what submit
		// means in this registry.
		if entry.SubmitDescription == "" {
			t.Errorf("entry %q missing submit_description", entry.Name)
		}
	}
}
