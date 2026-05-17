package editor

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/matcra587/jira-cli/internal/adf"
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
	md, opaques, warnings, err := renderToMarkdownWithOpaques(opts.Document, opts.FieldName)
	if err != nil {
		return adf.Document{}, nil, err
	}

	// Write to a temp file with frontmatter the user is told not to touch.
	path, err := WriteTemp(opts.IssueKey, opts.FieldName, md)
	if err != nil {
		return adf.Document{}, nil, err
	}
	// The temp file is removed only on success. Any failure past this
	// point preserves the file so the user can recover the edit — a
	// silently deleted buffer would lose Jira content.
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(path)
		}
	}()

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
	doc, mdWarnings, err := buildDocFromMarkdownWithOpaques(editedMD, opaques)
	if err != nil {
		cleanup = false
		return adf.Document{}, nil, fmt.Errorf("%w; your edit is preserved at %s", err, path)
	}
	warnings = append(warnings, mdWarnings...)
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
//
// A block that fails to marshal is a hard error: silently dropping it
// would erase Jira content with no warning, the exact failure this
// helper exists to prevent.
func renderToMarkdownWithOpaques(doc adf.Document, field string) (string, [][]byte, []adf.Warning, error) {
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
			return "", nil, nil, fmt.Errorf("ADF block %q at index %d could not be preserved for editing: %w", block.Type, i, err)
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
			// Not lossy: the block is reconstituted byte-for-byte. The
			// warning is informational — it tells the operator a subtree
			// could not be shown as editable Markdown, not that content
			// was dropped. Marking it lossy would abort strict-mode
			// submission of a document that is in fact fully preserved.
			Lossy: false,
		})
	}
	return strings.TrimSpace(buf.String()) + "\n", opaques, warnings, nil
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
// The non-opaque portions are converted via FromMarkdownLossy so the
// user's text edits land and any lossy conversion is reported.
//
// An opaque fence that no longer reconstitutes — a corrupted base64
// payload, an out-of-range id, or JSON that no longer parses — is a
// hard error: the caller preserves the temp file and fails clearly
// rather than silently dropping the opaque Jira content.
func buildDocFromMarkdownWithOpaques(md string, opaques [][]byte) (adf.Document, []adf.Warning, error) {
	out := adf.Document{Type: "doc", Version: 1}
	var warnings []adf.Warning
	pos := 0
	matches := opaqueRegex.FindAllStringSubmatchIndex(md, -1)
	for _, m := range matches {
		start, end := m[0], m[1]
		idStart, idEnd := m[2], m[3]
		payStart, payEnd := m[4], m[5]
		// Anything before the opaque is regular Markdown.
		if start > pos {
			frag, fragWarnings, err := adfFromMarkdown(md[pos:start])
			if err != nil {
				return adf.Document{}, nil, err
			}
			if frag != nil {
				out.Content = append(out.Content, frag.Content...)
			}
			warnings = append(warnings, fragWarnings...)
		}
		// Opaque payload — reconstitute from the id-keyed table, falling
		// back to the inline base64 the fence carries. Either must yield
		// the original node; a failure is fatal.
		node, err := reconstituteOpaque(md[idStart:idEnd], md[payStart:payEnd], opaques)
		if err != nil {
			return adf.Document{}, nil, err
		}
		out.Content = append(out.Content, node)
		pos = end
	}
	if pos < len(md) {
		frag, fragWarnings, err := adfFromMarkdown(md[pos:])
		if err != nil {
			return adf.Document{}, nil, err
		}
		if frag != nil {
			out.Content = append(out.Content, frag.Content...)
		}
		warnings = append(warnings, fragWarnings...)
	}
	return out, warnings, nil
}

// reconstituteOpaque turns one opaque fence (its id and inline base64
// payload) back into the original ADF node. The id-keyed table is the
// primary source; the inline base64 is the fallback. A corrupt id,
// corrupt base64, or unparseable JSON is a hard error.
func reconstituteOpaque(idText, payload string, opaques [][]byte) (adf.Node, error) {
	idx, err := strconv.Atoi(strings.TrimSpace(idText))
	if err != nil {
		return adf.Node{}, fmt.Errorf("opaque ADF block has an unreadable id %q", idText)
	}
	var raw []byte
	switch {
	case idx >= 0 && idx < len(opaques):
		raw = opaques[idx]
	default:
		decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(payload))
		if err != nil {
			return adf.Node{}, fmt.Errorf("opaque ADF block %d has an unreadable payload: %w", idx, err)
		}
		raw = decoded
	}
	// Cross-check the inline payload against the id-keyed table when the
	// editor left the fence intact — a tampered base64 line must not be
	// silently ignored.
	if idx >= 0 && idx < len(opaques) {
		if decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(payload)); err != nil {
			return adf.Node{}, fmt.Errorf("opaque ADF block %d payload was edited and no longer decodes: %w", idx, err)
		} else if string(decoded) != string(opaques[idx]) {
			return adf.Node{}, fmt.Errorf("opaque ADF block %d payload was modified inside the protected fences", idx)
		}
	}
	node := adf.Node{}
	if err := json.Unmarshal(raw, &node); err != nil {
		return adf.Node{}, fmt.Errorf("opaque ADF block %d no longer parses as ADF: %w", idx, err)
	}
	return node, nil
}

// adfFromMarkdown converts a Markdown fragment, returning nil for an
// empty fragment. Conversion warnings are propagated so editor round
// trips surface lossy text edits the same way direct Markdown input
// does.
func adfFromMarkdown(md string) (*adf.Document, []adf.Warning, error) {
	md = strings.TrimSpace(md)
	if md == "" {
		return nil, nil, nil
	}
	doc, warnings, err := adf.FromMarkdownLossy(md)
	if err != nil {
		return nil, nil, err
	}
	return &doc, warnings, nil
}
