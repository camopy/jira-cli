package editor

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/matcra587/jira-cli/pkg/adf"
)

// RoundTripADFOptions configures one external --edit cycle.
type RoundTripADFOptions struct {
	IssueKey  string
	FieldName string
	Document  adf.Document
	// EditCmd is the user's editor invocation (env-driven). Mutually
	// exclusive with EditFn — EditFn wins for tests.
	EditCmd string
	EditFn  func(ctx context.Context, path string) error
}

// RoundTripADF takes an ADF document, renders it to GFM Markdown for the
// editor buffer, preserves any non-Markdown-representable subtrees as
// fenced opaque blocks, launches the editor, parses the edited Markdown
// back, reconstitutes the opaques, and returns the resulting Document
// plus any structured warnings emitted along the way.
func RoundTripADF(ctx context.Context, opts RoundTripADFOptions) (adf.Document, []adf.Warning, error) {
	md, opaques, warnings := renderToMarkdownWithOpaques(opts.Document, opts.FieldName)

	// Write to a temp file with frontmatter the user is told not to touch.
	path, err := WriteTemp(opts.IssueKey, opts.FieldName, md)
	if err != nil {
		return adf.Document{}, nil, err
	}
	defer func() { _ = os.Remove(path) }()

	// Run the editor. Tests pass an EditFn (no-op or content rewriter);
	// production passes EditCmd.
	if opts.EditFn != nil {
		if err := opts.EditFn(ctx, path); err != nil {
			return adf.Document{}, nil, err
		}
	} else if opts.EditCmd != "" {
		if err := Run(ctx, opts.EditCmd, path); err != nil {
			return adf.Document{}, nil, err
		}
	}

	editedMD, err := ReadMarkdown(path)
	if err != nil {
		return adf.Document{}, nil, err
	}
	doc := buildDocFromMarkdownWithOpaques(editedMD, opaques)
	return doc, warnings, nil
}

// opaqueMarker is the fenced placeholder we use to embed opaque ADF JSON
// in the Markdown buffer. The user is instructed not to modify the
// content between the opening and closing fences. The id keeps multiple
// opaques in the same buffer distinct.
const opaqueMarker = "jira-adf-opaque"

var opaqueRegex = regexp.MustCompile("(?s)```" + opaqueMarker + ":(\\d+)\\s*\\n(.*?)\\n```")

// renderToMarkdownWithOpaques walks the doc, emitting GFM for blocks the
// editor can usefully present, and a fenced opaque block for everything
// else. Returns the rendered Markdown, the opaque payloads (so we can
// reinsert them after the user edits), and any lossy-step warnings.
func renderToMarkdownWithOpaques(doc adf.Document, field string) (string, [][]byte, []adf.Warning) {
	var (
		buf      strings.Builder
		opaques  [][]byte
		warnings []adf.Warning
	)
	for i, block := range doc.Content {
		if isMarkdownRepresentable(block.Type) && !containsInlineOpaque(block) {
			buf.WriteString(renderBlockMarkdown(block))
			buf.WriteString("\n\n")
			continue
		}
		// Opaque path — emit fenced placeholder and remember the payload.
		raw, err := json.Marshal(block)
		if err != nil {
			// Should not happen given Marshal contract — degrade silently
			// to an empty paragraph rather than crash.
			continue
		}
		id := len(opaques)
		opaques = append(opaques, raw)
		fmt.Fprintf(&buf, "```%s:%d\n%s\n```\n\n", opaqueMarker, id, base64.StdEncoding.EncodeToString(raw))
		warnings = append(warnings, adf.Warning{
			Type:     "external_edit_opaque",
			Message:  fmt.Sprintf("%s block at index %d preserved as opaque placeholder; do not edit between the fences", block.Type, i),
			Field:    field,
			Path:     fmt.Sprintf("/content/%d", i),
			NodeType: block.Type,
			Lossy:    true,
		})
	}
	return strings.TrimSpace(buf.String()) + "\n", opaques, warnings
}

// isMarkdownRepresentable reports which ADF block types this round-trip
// renders to GFM. Anything outside this set is opaque-preserved.
func isMarkdownRepresentable(t string) bool {
	switch t {
	case "paragraph", "heading", "bulletList", "orderedList", "codeBlock", "blockquote", "rule", "hardBreak":
		return true
	}
	return false
}

// containsInlineOpaque returns true if any child node (recursively) is an
// inline node we cannot represent in Markdown — mention, emoji, date,
// status, inlineCard. Paragraphs containing those route through the
// opaque-preservation path so the inline node survives the edit.
func containsInlineOpaque(block adf.Node) bool {
	for _, c := range block.Content {
		switch c.Type {
		case "mention", "emoji", "date", "status", "inlineCard":
			return true
		}
		if containsInlineOpaque(c) {
			return true
		}
	}
	return false
}

// renderBlockMarkdown turns one ADF block into a GFM string. Currently
// supports paragraph + heading; the full set lands incrementally as the
// editor matures. Anything else routes to the opaque path upstream.
func renderBlockMarkdown(block adf.Node) string {
	switch block.Type {
	case "paragraph":
		return renderInlineMarkdown(block.Content)
	case "heading":
		level := 1
		switch v := block.Attrs["level"].(type) {
		case int:
			level = v
		case float64:
			level = int(v)
		}
		return strings.Repeat("#", level) + " " + renderInlineMarkdown(block.Content)
	case "blockquote":
		return "> " + renderInlineMarkdown(block.Content)
	case "rule":
		return "---"
	case "hardBreak":
		return "  "
	case "codeBlock":
		lang, _ := block.Attrs["language"].(string)
		return "```" + lang + "\n" + renderInlineMarkdown(block.Content) + "\n```"
	}
	return ""
}

func renderInlineMarkdown(nodes []adf.Node) string {
	var b strings.Builder
	for _, n := range nodes {
		if n.Type == "text" {
			b.WriteString(n.Text)
		}
	}
	return b.String()
}

// buildDocFromMarkdownWithOpaques is the reverse of
// renderToMarkdownWithOpaques: parses the edited Markdown looking for
// opaque fence markers and reconstitutes the original ADF JSON for each.
// The non-opaque portions are converted via the existing FromMarkdown
// path so the user's text edits land.
func buildDocFromMarkdownWithOpaques(md string, opaques [][]byte) adf.Document {
	out := adf.Document{Type: "doc", Version: 1}
	pos := 0
	matches := opaqueRegex.FindAllStringSubmatchIndex(md, -1)
	for _, m := range matches {
		start, end := m[0], m[1]
		idStart, idEnd := m[2], m[3]
		// Anything before the opaque is regular Markdown.
		if start > pos {
			plain := md[pos:start]
			if frag := adfFromMarkdown(plain); frag != nil {
				out.Content = append(out.Content, frag.Content...)
			}
		}
		// Opaque payload — look up by id, fallback to the inline base64.
		var idx int
		if _, err := fmt.Sscanf(md[idStart:idEnd], "%d", &idx); err != nil {
			continue
		}
		if idx >= 0 && idx < len(opaques) {
			node := adf.Node{}
			if err := json.Unmarshal(opaques[idx], &node); err == nil {
				out.Content = append(out.Content, node)
			}
		}
		pos = end
	}
	if pos < len(md) {
		plain := md[pos:]
		if frag := adfFromMarkdown(plain); frag != nil {
			out.Content = append(out.Content, frag.Content...)
		}
	}
	return out
}

// adfFromMarkdown wraps adf.FromMarkdown but tolerates errors by
// returning nil — the caller treats nil as "skip this fragment".
func adfFromMarkdown(md string) *adf.Document {
	md = strings.TrimSpace(md)
	if md == "" {
		return nil
	}
	doc, err := adf.FromMarkdown(md)
	if err != nil {
		return nil
	}
	return &doc
}
