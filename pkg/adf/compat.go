package adf

import (
	"fmt"

	"github.com/matcra587/jira-cli/internal/cli/adfmode"
)

// FieldCompatibility describes what a single Jira target field will accept.
// The zero value (no flags set) means "unknown" and unknown is treated as
// unsupported.
type FieldCompatibility struct {
	Field               string
	InlineCardSupported bool
	// Future: per-node/per-mark capability matrix.
}

// CompatibilityError is the typed error returned by ApplyCompatibility in
// strict mode when a node would have to be degraded or dropped to fit the
// target field schema.
type CompatibilityError struct {
	Field    string
	NodeType string
	MarkType string
	Path     string
	Reason   string
}

func (e *CompatibilityError) Error() string {
	if e == nil {
		return "<nil compatibility error>"
	}
	return fmt.Sprintf("adf compatibility: field=%s node=%s mark=%s path=%s: %s", e.Field, e.NodeType, e.MarkType, e.Path, e.Reason)
}

// ApplyCompatibility walks doc and reconciles Jira field compatibility.
// In best-effort mode, unsupported nodes are degraded and each
// degradation produces one Warning. In strict mode, the first
// incompatibility aborts with a CompatibilityError; no partial result.
func ApplyCompatibility(doc Document, schema FieldCompatibility, mode adfmode.Mode) (Document, []Warning, error) {
	var warnings []Warning
	out := Document{Type: doc.Type, Version: doc.Version}
	out.Content = make([]Node, len(doc.Content))
	for i, n := range doc.Content {
		path := fmt.Sprintf("/content/%d", i)
		converted, ws, err := walkCompat(n, schema, mode, path)
		if err != nil {
			return Document{}, nil, err
		}
		out.Content[i] = converted
		warnings = append(warnings, ws...)
	}
	return out, warnings, nil
}

func walkCompat(n Node, schema FieldCompatibility, mode adfmode.Mode, path string) (Node, []Warning, error) {
	if n.Type == "inlineCard" && !schema.InlineCardSupported {
		if mode == adfmode.ModeStrict {
			return Node{}, nil, &CompatibilityError{
				Field:    schema.Field,
				NodeType: "inlineCard",
				Path:     path,
				Reason:   "inlineCard not supported on this field",
			}
		}
		// Best-effort degrade.
		url, _ := n.Attrs["url"].(string)
		degraded := Node{
			Type: "text",
			Text: url,
			Marks: []Mark{{
				Type:  "link",
				Attrs: map[string]any{"href": url},
			}},
		}
		w := Warning{
			Type:     "adf_compatibility",
			Message:  fmt.Sprintf("inlineCard not supported on %s; degraded to text+link mark", schema.Field),
			Field:    schema.Field,
			Path:     path,
			NodeType: "inlineCard",
			Lossy:    true,
		}
		return degraded, []Warning{w}, nil
	}
	// Recurse into children.
	if len(n.Content) > 0 {
		var warnings []Warning
		converted := make([]Node, len(n.Content))
		for i, c := range n.Content {
			child, ws, err := walkCompat(c, schema, mode, fmt.Sprintf("%s/content/%d", path, i))
			if err != nil {
				return Node{}, nil, err
			}
			converted[i] = child
			warnings = append(warnings, ws...)
		}
		n.Content = converted
		return n, warnings, nil
	}
	return n, nil, nil
}
