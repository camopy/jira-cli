package schema

import (
	"maps"
	"strings"

	"github.com/matcra587/docent"
)

// adfDocumentRef is the legacy $ref target the hand-written input schemas
// use to share the ADF document shape. Docent embeds schemas per command
// with no shared definitions block, so the bridge inlines the referenced
// shape at each site.
const adfDocumentRef = "#/data/input_schemas/adf_document"

// DocentRegistry bridges the host's schema knowledge into docent's
// registry form: the typed-Output-derived output schemas and the
// hand-written --json-input schemas, keyed by full command path
// ("jira issue create"). Registry keys with no matching command are
// ignored by Apply, so alias ops cost nothing.
func DocentRegistry() docent.SchemaRegistry {
	outputs := outputSchemas()
	inputs := inputSchemas()
	adfDocument, _ := inputs["adf_document"].(map[string]any)

	reg := docent.SchemaRegistry{}
	entry := func(op string) docent.CommandSchemas {
		return reg[commandPathForOp(op)]
	}
	for op, s := range outputs {
		m, ok := s.(map[string]any)
		if !ok {
			continue
		}
		e := entry(op)
		e.Output = m
		reg[commandPathForOp(op)] = e
	}
	for op, s := range inputs {
		if op == "adf_document" {
			continue // shared shape, inlined below rather than emitted
		}
		m, ok := s.(map[string]any)
		if !ok {
			continue
		}
		e := entry(op)
		e.Input = inlineADFRefs(m, adfDocument)
		reg[commandPathForOp(op)] = e
	}
	return reg
}

// OutputContract returns the tool-wide envelope and structured-error
// schemas — shapes that describe every response rather than one command,
// so they ride the docent schema root's extensions instead of a command
// node.
func OutputContract() map[string]any {
	schemas := outputSchemas()
	return map[string]any{
		"envelope": schemas["envelope"],
		"error":    schemas["error"],
	}
}

// commandPathForOp turns an envelope op key ("issue.comment.add") into the
// docent command path ("jira issue comment add").
func commandPathForOp(op string) string {
	return "jira " + strings.ReplaceAll(op, ".", " ")
}

// inlineADFRefs deep-copies a schema, replacing every legacy
// adf_document $ref with the inline ADF document shape. Keys already on
// the referring node (description, type) win over the inlined ones.
func inlineADFRefs(m, adfDocument map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	inline := m["$ref"] == adfDocumentRef && adfDocument != nil
	if inline {
		maps.Copy(out, adfDocument)
	}
	for k, v := range m {
		if k == "$ref" && inline {
			continue
		}
		if child, ok := v.(map[string]any); ok {
			out[k] = inlineADFRefs(child, adfDocument)
			continue
		}
		out[k] = v
	}
	return out
}
