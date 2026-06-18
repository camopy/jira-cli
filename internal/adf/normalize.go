package adf

import "fmt"

// Normalize repairs structurally-invalid but losslessly-fixable constructs that
// Jira's ADF validator rejects outright — an opaque INVALID_INPUT on the whole
// document — even though they carry no renderable content. It returns the
// cleaned document and one non-lossy Warning per class of repair, and is meant
// to run just before ValidateDoc so the submitted document is both valid and
// minimal. Every repair is provably lossless: it changes nothing the user can
// see, only "rejected by Jira" into "accepted", so it runs in every mode.
//
// Repairs:
//
//   - Empty text nodes. A text node's `text` must be a non-empty string; an
//     empty (or absent) one is rejected by Jira yet renders as nothing. These
//     are a common artifact of document generators that emit blank table
//     cells. Removing them is lossless — the node carried no content — and a
//     parent left with no inline children (e.g. an empty paragraph in a blank
//     cell) is itself valid ADF.
func Normalize(d Document) (Document, []Warning) {
	cleaned, removedEmptyText := pruneEmptyText(d.Content)
	d.Content = cleaned

	var warnings []Warning
	if removedEmptyText > 0 {
		warnings = append(warnings, Warning{
			Type:    "adf_normalized",
			Message: fmt.Sprintf("removed %d empty text node(s): a text node must carry non-empty text or Jira rejects the document", removedEmptyText),
			Lossy:   false,
		})
	}
	return d, warnings
}

// pruneEmptyText returns a copy of nodes with empty text nodes removed,
// recursing into every node's content. A text node is "empty" when its text is
// the empty string — which also covers a node that omitted the `text` field
// entirely, since the typed model decodes the absence to "". The input slice
// and its nodes are not mutated.
func pruneEmptyText(nodes []Node) ([]Node, int) {
	if len(nodes) == 0 {
		return nodes, 0
	}
	out := make([]Node, 0, len(nodes))
	removed := 0
	for _, n := range nodes {
		if n.Type == "text" && n.Text == "" {
			removed++
			continue
		}
		if len(n.Content) > 0 {
			child, c := pruneEmptyText(n.Content)
			n.Content = child
			removed += c
		}
		out = append(out, n)
	}
	return out, removed
}
