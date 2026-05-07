package adf

import (
	"fmt"

	"github.com/matcra587/jira-cli/internal/cli/adfmode"
)

// ValidateDoc validates a parsed ADF Document according to the given mode.
//
// Root shape (type="doc", version=1) is always enforced regardless of mode —
// a structurally invalid root cannot be sent to Jira in any mode.
//
// In ModeStrict (the mutation-submit default):
//   - Unknown node types → error naming the type
//   - Marks on block nodes → error naming the field path
//   - Unknown mark types → error naming the mark
//
// In ModeBestEffort:
//   - Unknown node types → Warning (lossy=false, opaque passthrough preserved)
//   - Marks on block nodes → Warning (lossy=false)
//   - Unknown mark types → Warning (lossy=false, mark forwarded opaquely)
//   - No error; the document is forwarded as-is
//
// Returns (warnings, nil) on success; (nil, err) on fatal validation failure.
func ValidateDoc(doc Document, mode adfmode.Mode) ([]Warning, error) {
	if doc.Type != "doc" {
		return nil, fmt.Errorf("ADF root type must be \"doc\", got %q", doc.Type)
	}
	if doc.Version != 1 {
		return nil, fmt.Errorf("ADF version must be 1, got %d", doc.Version)
	}

	reg := Registry()
	var warnings []Warning
	for _, node := range doc.Content {
		ws, err := validateNodeMode(node, reg, mode, "content")
		warnings = append(warnings, ws...)
		if err != nil {
			return nil, err
		}
	}
	return warnings, nil
}

// blockNodeTypes is the set of ADF node types that are block-level and must
// not carry marks. Marks are only legal on inline nodes (primarily "text").
var blockNodeTypes = map[string]bool{
	"paragraph":   true,
	"heading":     true,
	"bulletList":  true,
	"orderedList": true,
	"listItem":    true,
	"codeBlock":   true,
	"blockquote":  true,
	"rule":        true,
	"hardBreak":   true,
	"panel":       true,
	"table":       true,
	"tableRow":    true,
	"tableCell":   true,
	"tableHeader": true,
	"doc":         true,
}

func validateNodeMode(n Node, reg RegistryView, mode adfmode.Mode, path string) ([]Warning, error) {
	var warnings []Warning
	nodePath := path + "/" + n.Type

	// Check whether node type is known.
	_, known := reg.Lookup(KindNode, n.Type)
	if !known {
		if mode == adfmode.ModeStrict {
			return nil, fmt.Errorf("ADF validation: unsupported node type %q at %s", n.Type, path)
		}
		// Best-effort: warn but preserve opaquely.
		warnings = append(warnings, Warning{
			Type:     "unknown_adf_node",
			Message:  fmt.Sprintf("unsupported ADF node type %q will be forwarded opaquely", n.Type),
			Path:     path,
			NodeType: n.Type,
			Lossy:    false,
		})
		// Do not recurse into unknown nodes — they are opaque.
		return warnings, nil
	}

	// Block nodes must not carry marks (marks are inline-only).
	if blockNodeTypes[n.Type] && len(n.Marks) > 0 {
		msg := fmt.Sprintf("ADF validation: block node %q at %s must not have marks", n.Type, path)
		if mode == adfmode.ModeStrict {
			return nil, fmt.Errorf("%s", msg)
		}
		warnings = append(warnings, Warning{
			Type:     "illegal_marks_on_block",
			Message:  msg,
			Path:     nodePath,
			NodeType: n.Type,
			Lossy:    false,
		})
	}

	// Check each mark against the mark registry ( ).
	for _, m := range n.Marks {
		if _, ok := reg.Lookup(KindMark, m.Type); !ok {
			if mode == adfmode.ModeStrict {
				return nil, fmt.Errorf("ADF validation: unsupported mark type %q at %s", m.Type, nodePath)
			}
			warnings = append(warnings, Warning{
				Type:     "unknown_adf_mark",
				Message:  fmt.Sprintf("unsupported ADF mark type %q will be forwarded opaquely", m.Type),
				Path:     nodePath,
				NodeType: n.Type,
				MarkType: m.Type,
				Lossy:    false,
			})
		}
	}

	// Recurse into children.
	for _, child := range n.Content {
		ws, err := validateNodeMode(child, reg, mode, nodePath)
		warnings = append(warnings, ws...)
		if err != nil {
			return nil, err
		}
	}

	return warnings, nil
}

// Validate is the legacy mode-unaware validator (kept for backwards
// compatibility with existing callers). It uses strict mode.
//
// Deprecated: prefer ValidateDoc(doc, adfmode.ModeStrict).
func (d Document) Validate() error {
	_, err := ValidateDoc(d, adfmode.ModeStrict)
	return err
}
