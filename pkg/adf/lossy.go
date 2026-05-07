// Package adf — lossy detection for ADF→Markdown rendering.
//
// When the ADF→Markdown renderer can't fully represent a node or mark
// (e.g. inlineCard, custom panel variants, extension nodes), the affected
// constructs MUST be visible to callers so they can warn the user or
// fall back to --raw. Today the detection logic was inline in
// cmd/jira/commands.go's `issue view` description path; this file
// extracts it as a shared helper used by `comment list` and reused from
// `issue view`.
//
// The set of "supported" nodes/marks is the set the renderer in
// render.go switches on explicitly — anything else is "lossy" because
// the renderer falls through to children-only output, dropping
// node-specific semantics (e.g. inlineCard becomes empty text since it
// has no .Content). Marks not in the explicit switch are ignored
// silently by the renderer; same logic applies.

package adf

import "sort"

// LossyResult is the typed return for ToMarkdownLossy. Markdown is the
// best-effort GFM rendering; LossyConstructs is the sorted unique list
// of node/mark type names the renderer didn't fully round-trip.
type LossyResult struct {
	Markdown        string
	LossyConstructs []string
}

// renderableMarkdownNodes is the set of node types the renderer in
// render.go's markdownBlock has an explicit case for. Anything outside
// this set falls through to markdownChildren and is therefore lossy
// (the node type itself isn't represented in the Markdown output).
var renderableMarkdownNodes = map[string]bool{
	"doc":         true, // root; never visited as a content node
	"paragraph":   true,
	"text":        true,
	"heading":     true,
	"bulletList":  true,
	"orderedList": true,
	"listItem":    true,
	"codeBlock":   true,
}

// renderableMarkdownMarks is the set of mark types the renderer in
// render.go's markdownText has an explicit case for. Anything outside
// the set is silently dropped from the rendered Markdown.
var renderableMarkdownMarks = map[string]bool{
	"strong": true,
	"em":     true,
	"code":   true,
	"link":   true,
}

// ToMarkdownLossy renders doc to GFM Markdown and reports every node or
// mark type the renderer dropped or simplified. The returned
// LossyConstructs slice is sorted unique. A nil-or-empty doc yields an
// empty list.
func ToMarkdownLossy(doc Document) LossyResult {
	seen := make(map[string]struct{})
	for _, node := range doc.Content {
		collectLossy(node, seen)
	}
	return LossyResult{
		Markdown:        ToMarkdown(doc),
		LossyConstructs: sortedKeys(seen),
	}
}

func collectLossy(n Node, seen map[string]struct{}) {
	if n.Type != "" && !renderableMarkdownNodes[n.Type] {
		seen[n.Type] = struct{}{}
	}
	for _, m := range n.Marks {
		if m.Type == "" {
			continue
		}
		if !renderableMarkdownMarks[m.Type] {
			seen[m.Type] = struct{}{}
		}
	}
	for _, child := range n.Content {
		collectLossy(child, seen)
	}
}

func sortedKeys(set map[string]struct{}) []string {
	if len(set) == 0 {
		return nil
	}
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
