package adf

import (
	"bytes"
	"fmt"
	"strings"

	xstrings "github.com/gechr/x/strings"
	"github.com/google/uuid"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	extast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/text"
)

// markdownParser parses GFM with the table, strikethrough, and task-list
// extensions enabled. Enabling the table extension means a pipe table
// is parsed as a real Table node that converts to an ADF table, rather
// than rendering as literal pipe-laden paragraph text; the task-list
// extension parses `- [ ]` / `- [x]` items as real checkboxes that
// convert to ADF taskList/taskItem nodes.
var markdownParser = goldmark.New(
	goldmark.WithExtensions(extension.Table, extension.Strikethrough, extension.TaskList),
).Parser()

// newLocalID generates the localId attr ADF requires on every
// taskList/taskItem/decisionList node. A package variable so tests can
// pin deterministic ids.
var newLocalID = func() string { return uuid.NewString() }

// FromMarkdownLossy converts GFM Markdown to an ADF document and reports
// every Markdown construct it could not represent faithfully in the
// supported ADF node set. Each unsupported construct yields one
// Warning (lossy=true) naming the construct and its source position so
// callers — and strict-mode abort gates — can act on the content loss
// instead of letting it slip through silently.
func FromMarkdownLossy(markdown string) (Document, []Warning, error) {
	var dialectWarnings []Warning
	wikiEmoji := false
	if detectDialect(markdown) == dialectWiki {
		normalized := normalizeWikiMarkup(markdown)
		markdown = normalized.text
		wikiEmoji = true
		dialectWarnings = append(dialectWarnings, wikiNormalizationWarning(normalized.constructs))
	}

	source := []byte(markdown)
	reader := text.NewReader(source)
	root := markdownParser.Parse(reader)

	conv := &mdConverter{source: source, wikiEmoji: wikiEmoji, warnings: dialectWarnings}
	doc := Document{Type: "doc", Version: 1}
	for child := root.FirstChild(); child != nil; child = child.NextSibling() {
		if node, ok := conv.block(child); ok {
			doc.Content = append(doc.Content, node)
		}
		// Blocks hoisted out of forbidden nesting (tables in list items
		// or blockquotes) land right after the block that contained them.
		if len(conv.hoisted) > 0 {
			doc.Content = append(doc.Content, conv.hoisted...)
			conv.hoisted = nil
		}
	}
	if len(doc.Content) == 0 && !xstrings.IsBlank(markdown) && len(conv.warnings) == 0 {
		doc.Content = append(doc.Content, Node{Type: "paragraph", Content: []Node{{Type: "text", Text: strings.TrimSpace(markdown)}}})
	}
	return doc, conv.warnings, nil
}

// mdConverter carries the Markdown source plus the running warning list
// through the recursive goldmark walk. wikiEmoji is set when the input was
// normalized from Jira wiki markup, enabling emoticon-shortcut expansion in
// plain text runs — never inside code spans or code blocks, which take
// different paths through the walk.
type mdConverter struct {
	source    []byte
	warnings  []Warning
	wikiEmoji bool
	// hoisted collects block nodes that converted cleanly but sat in a
	// position ADF forbids (a table inside a list item or blockquote).
	// FromMarkdownLossy drains it after each top-level block, so the
	// node re-enters the document at the nearest valid position.
	hoisted []Node
	// inTaskItem is set while converting a task item's inline content,
	// where the leading GFM checkbox is consumed by the taskItem's state
	// attr. Outside a task item (a checkbox in a mixed list) the checkbox
	// degrades to literal text instead.
	inTaskItem bool
}

// warn records one lossy-conversion warning naming an unsupported
// Markdown construct, source-mapped to the offending input when n carries
// position information.
func (c *mdConverter) warn(n ast.Node, construct, detail string) {
	msg := fmt.Sprintf("Markdown %s is not supported and was dropped during ADF conversion", construct)
	if detail != "" {
		msg += ": " + detail
	}
	c.warnings = append(c.warnings, c.sourceMapped(n, Warning{
		Type:     "markdown_lossy_conversion",
		Message:  msg,
		NodeType: construct,
		Lossy:    true,
	}))
}

// warnDowngrade records a non-lossy downgrade notice: the content survives
// in a different shape, so strict mode does not abort on it.
func (c *mdConverter) warnDowngrade(n ast.Node, construct, resolution string) {
	c.warnings = append(c.warnings, c.sourceMapped(n, Warning{
		Type:     "markdown_lossy_conversion",
		Message:  fmt.Sprintf("Markdown %s was downgraded during ADF conversion: %s", construct, resolution),
		NodeType: construct,
		Lossy:    false,
	}))
}

// sourceMapped appends the 1-based source position and the offending line
// to w so a diagnostic points at the Markdown the author wrote, not a JSON
// path into a document they never saw. A node with no resolvable position
// (synthesized, or an empty document) leaves w unchanged.
func (c *mdConverter) sourceMapped(n ast.Node, w Warning) Warning {
	off := nodeOffset(n)
	if off < 0 || off > len(c.source) {
		return w
	}
	line := 1 + bytes.Count(c.source[:off], []byte("\n"))
	lineStart := bytes.LastIndexByte(c.source[:off], '\n') + 1
	col := off - lineStart + 1
	w.Path = fmt.Sprintf("line %d, col %d", line, col)
	if snippet := sourceLine(c.source, lineStart); snippet != "" {
		w.Message += fmt.Sprintf(" (line %d, col %d: %q)", line, col, snippet)
	} else {
		w.Message += fmt.Sprintf(" (line %d, col %d)", line, col)
	}
	return w
}

// nodeOffset resolves the byte offset of n's first piece of source text:
// block nodes carry line segments, text nodes carry their own segment, and
// containers defer to their first positioned child. -1 means no position.
func nodeOffset(n ast.Node) int {
	if n == nil {
		return -1
	}
	// Lines() is only defined for block nodes; goldmark's inline base
	// panics on it.
	if n.Type() == ast.TypeBlock {
		if lines := n.Lines(); lines != nil && lines.Len() > 0 {
			return lines.At(0).Start
		}
	}
	if t, ok := n.(*ast.Text); ok {
		return t.Segment.Start
	}
	for child := n.FirstChild(); child != nil; child = child.NextSibling() {
		if off := nodeOffset(child); off >= 0 {
			return off
		}
	}
	return -1
}

// sourceLine returns the trimmed source line starting at lineStart, capped
// so a pathological input cannot balloon a warning message.
func sourceLine(source []byte, lineStart int) string {
	end := bytes.IndexByte(source[lineStart:], '\n')
	if end < 0 {
		end = len(source) - lineStart
	}
	const maxSnippet = 80
	line := strings.TrimSpace(string(source[lineStart : lineStart+end]))
	if len(line) > maxSnippet {
		line = line[:maxSnippet] + "…"
	}
	return line
}

// block converts one block-level goldmark node to an ADF block node.
// Returns ok=false when the node produced nothing (e.g. an unsupported
// construct, which is recorded as a warning instead).
func (c *mdConverter) block(n ast.Node) (Node, bool) {
	switch n.Kind() {
	case ast.KindParagraph, ast.KindTextBlock:
		return Node{Type: "paragraph", Content: c.inlineChildren(n)}, true
	case ast.KindHeading:
		h := n.(*ast.Heading)
		return Node{Type: "heading", Attrs: map[string]any{"level": h.Level}, Content: c.inlineChildren(n)}, true
	case ast.KindList:
		return c.list(n.(*ast.List))
	case ast.KindCodeBlock, ast.KindFencedCodeBlock:
		return c.codeBlock(n), true
	case ast.KindBlockquote:
		return c.blockquote(n), true
	case ast.KindThematicBreak:
		return Node{Type: "rule"}, true
	case ast.KindHTMLBlock:
		c.warn(n, "raw HTML block", "")
		return Node{}, false
	case ast.KindLinkReferenceDefinition:
		// Goldmark v1.8 surfaces reference definitions as block nodes.
		// They are parser metadata; resolved links already carry the href.
		return Node{}, false
	case extast.KindTable:
		return c.table(n), true
	default:
		c.warn(n, "block "+n.Kind().String(), "")
		return Node{}, false
	}
}

// blockquote converts a Markdown blockquote to the ADF blockquote node. The
// schema restricts blockquote children to paragraphs, lists, and code
// blocks, so other quoted constructs degrade: a nested quote is flattened
// into its parent, and a quoted heading becomes a paragraph. Both keep the
// content and report a non-lossy downgrade.
func (c *mdConverter) blockquote(n ast.Node) Node {
	return Node{Type: "blockquote", Content: c.blockquoteChildren(n)}
}

func (c *mdConverter) blockquoteChildren(n ast.Node) []Node {
	var out []Node
	for child := n.FirstChild(); child != nil; child = child.NextSibling() {
		switch child.Kind() {
		case ast.KindBlockquote:
			c.warnDowngrade(child, "nested blockquote", "flattened into its parent blockquote")
			out = append(out, c.blockquoteChildren(child)...)
		case extast.KindTable:
			// ADF forbids a table inside a blockquote; the table itself
			// converts cleanly, so it moves after the enclosing block.
			c.warnDowngrade(child, "table inside a blockquote", "moved after the enclosing block")
			if node, ok := c.block(child); ok {
				c.hoisted = append(c.hoisted, node)
			}
		case ast.KindHeading:
			c.warnDowngrade(child, "heading inside a blockquote", "converted to a paragraph")
			out = append(out, Node{Type: "paragraph", Content: c.inlineChildren(child)})
		default:
			if node, ok := c.block(child); ok {
				out = append(out, node)
			}
		}
	}
	return out
}

// list converts a bullet or ordered list, recursing into list items. An
// unordered list whose every item leads with a GFM task checkbox becomes an
// ADF taskList; any other list with checkboxes keeps its shape and the
// checkboxes degrade to literal text (ADF forbids taskItem outside a pure
// taskList), reported as a non-lossy downgrade.
func (c *mdConverter) list(list *ast.List) (Node, bool) {
	pure, hasCheckboxes := taskListShape(list)
	if pure {
		return c.taskList(list), true
	}
	if hasCheckboxes {
		c.warnDowngrade(list, "task items in a mixed or ordered list", "checkboxes rendered as literal text")
	}
	// The items of a non-task list convert outside any surrounding task
	// context: their checkboxes (if any) must degrade to text, not vanish.
	prev := c.inTaskItem
	c.inTaskItem = false
	defer func() { c.inTaskItem = prev }()

	nodeType := "bulletList"
	var attrs map[string]any
	if list.IsOrdered() {
		nodeType = "orderedList"
		if list.Start > 1 {
			attrs = map[string]any{"order": list.Start}
		}
	}
	var items []Node
	for child := list.FirstChild(); child != nil; child = child.NextSibling() {
		if child.Kind() != ast.KindListItem {
			continue
		}
		items = append(items, Node{Type: "listItem", Content: c.listItemChildren(child)})
	}
	return Node{Type: nodeType, Attrs: attrs, Content: items}, true
}

// taskListShape reports whether a list is a pure task list (every item
// leads with a GFM checkbox — convertible to ADF taskList) and whether any
// item carries a checkbox at all. Ordered lists are never task lists: ADF
// task lists are inherently unordered.
func taskListShape(list *ast.List) (pure, hasCheckboxes bool) {
	if list.FirstChild() == nil {
		return false, false
	}
	pure = !list.IsOrdered()
	for child := list.FirstChild(); child != nil; child = child.NextSibling() {
		if child.Kind() != ast.KindListItem {
			continue
		}
		if taskCheckbox(child) != nil {
			hasCheckboxes = true
		} else {
			pure = false
		}
	}
	return pure && hasCheckboxes, hasCheckboxes
}

// taskCheckbox returns the GFM checkbox leading a list item's first
// paragraph, or nil when the item is not a task item.
func taskCheckbox(item ast.Node) *extast.TaskCheckBox {
	first := item.FirstChild()
	if first == nil || (first.Kind() != ast.KindTextBlock && first.Kind() != ast.KindParagraph) {
		return nil
	}
	if cb, ok := first.FirstChild().(*extast.TaskCheckBox); ok {
		return cb
	}
	return nil
}

// taskList converts a pure GFM task list to the ADF taskList shape:
// taskItem children carrying the checkbox state, with nested task lists
// as taskList siblings (ADF nests them inside the parent taskList, not
// inside the inline-only taskItem).
func (c *mdConverter) taskList(list *ast.List) Node {
	out := Node{Type: "taskList", Attrs: map[string]any{"localId": newLocalID()}}
	for child := list.FirstChild(); child != nil; child = child.NextSibling() {
		if child.Kind() != ast.KindListItem {
			continue
		}
		out.Content = append(out.Content, c.taskItemNodes(child)...)
	}
	return out
}

// taskItemNodes converts one GFM task item into an ADF taskItem plus any
// sibling nodes that must live beside it in the parent taskList. The
// taskItem is inline-only: extra paragraphs join its inline run after a
// hardBreak, a nested pure task list becomes a taskList sibling, and any
// other block content is hoisted after the enclosing list with a
// downgrade warning — the content survives, only its nesting moves.
func (c *mdConverter) taskItemNodes(item ast.Node) []Node {
	state := "TODO"
	if cb := taskCheckbox(item); cb != nil && cb.IsChecked {
		state = "DONE"
	}
	task := Node{Type: "taskItem", Attrs: map[string]any{"localId": newLocalID(), "state": state}}
	var siblings []Node
	for child := item.FirstChild(); child != nil; child = child.NextSibling() {
		switch child.Kind() {
		case ast.KindTextBlock, ast.KindParagraph:
			run := c.taskItemInline(child)
			if len(task.Content) > 0 && len(run) > 0 {
				task.Content = append(task.Content, Node{Type: "hardBreak"})
			}
			task.Content = append(task.Content, run...)
		case ast.KindList:
			nested := child.(*ast.List)
			if pure, _ := taskListShape(nested); pure {
				siblings = append(siblings, c.taskList(nested))
				continue
			}
			// ADF taskList children are task nodes only; a non-task nested
			// list converts cleanly, so it moves after the enclosing list.
			c.warnDowngrade(child, "list nested under a task item", "moved after the enclosing task list")
			if node, ok := c.list(nested); ok {
				c.hoisted = append(c.hoisted, node)
			}
		default:
			c.warnDowngrade(child, "block content inside a task item", "moved after the enclosing task list")
			if node, ok := c.block(child); ok {
				c.hoisted = append(c.hoisted, node)
			}
		}
	}
	return append([]Node{task}, siblings...)
}

// taskItemInline converts a task item paragraph's inline children with the
// leading checkbox consumed (it became the taskItem's state attr).
func (c *mdConverter) taskItemInline(n ast.Node) []Node {
	prev := c.inTaskItem
	c.inTaskItem = true
	defer func() { c.inTaskItem = prev }()
	return c.inlineChildren(n)
}

// listItemChildren converts a list item's children. A list item's
// content must be block nodes (paragraph, nested list, codeBlock); a
// bare text run is wrapped in a paragraph.
func (c *mdConverter) listItemChildren(item ast.Node) []Node {
	var out []Node
	for child := item.FirstChild(); child != nil; child = child.NextSibling() {
		switch child.Kind() {
		case ast.KindTextBlock, ast.KindParagraph:
			out = append(out, Node{Type: "paragraph", Content: c.inlineChildren(child)})
		case ast.KindList:
			if node, ok := c.list(child.(*ast.List)); ok {
				out = append(out, node)
			}
		case ast.KindCodeBlock, ast.KindFencedCodeBlock:
			out = append(out, c.codeBlock(child))
		case ast.KindBlockquote:
			// ADF list items cannot contain blockquotes. Hoist the quoted
			// blocks into the list item so the content survives — this is
			// exactly the "- >50 keys" shape, where Markdown parses the
			// remainder of a list item as a nested quote.
			c.warnDowngrade(child, "blockquote inside a list item", "quoted content hoisted into the list item")
			out = append(out, c.blockquoteChildren(child)...)
		case extast.KindTable:
			// ADF forbids a table inside a list item — a common Markdown
			// shape (a status bullet followed by its indented table). The
			// table converts cleanly, so it moves after the enclosing list.
			c.warnDowngrade(child, "table inside a list item", "moved after the enclosing list")
			if node, ok := c.block(child); ok {
				c.hoisted = append(c.hoisted, node)
			}
		default:
			if node, ok := c.block(child); ok {
				out = append(out, node)
			}
		}
	}
	return out
}

// codeBlock converts an indented or fenced code block. The language attr is
// always present — empty string when the fence has no hint — because Jira
// renders an attr-less codeBlock with its default language (java) instead of
// plain text. An empty block carries no content array at all: the ADF schema
// requires text nodes to be non-empty, so an empty text child would be
// rejected. Trailing newlines are trimmed — the fence's closing newline is
// Markdown syntax, not code, and Jira renders it as a spurious blank line.
func (c *mdConverter) codeBlock(n ast.Node) Node {
	lang := ""
	if fenced, ok := n.(*ast.FencedCodeBlock); ok {
		lang = string(fenced.Language(c.source))
	}
	node := Node{Type: "codeBlock", Attrs: map[string]any{"language": lang}}
	if text := strings.TrimRight(codeBlockText(n, c.source), "\n"); text != "" {
		node.Content = []Node{{Type: "text", Text: text}}
	}
	return node
}

// table converts a GFM table to the ADF table shape: a tableRow of
// tableHeader cells, then tableCell rows, with the standard table attrs.
// Every cell wraps its inline run in a paragraph — tableCell/tableHeader
// require at least one block child, and an empty GFM cell becomes an
// empty paragraph, which the schema permits.
func (c *mdConverter) table(n ast.Node) Node {
	table := Node{
		Type: "table",
		Attrs: map[string]any{
			"isNumberColumnEnabled": false,
			"layout":                "default",
		},
	}
	// In goldmark, TableHeader directly contains TableCell children (no
	// wrapping TableRow), while TableRow does contain TableCells.
	for child := n.FirstChild(); child != nil; child = child.NextSibling() {
		switch child.Kind() {
		case extast.KindTableHeader:
			table.Content = append(table.Content, c.tableRow(child, "tableHeader"))
		case extast.KindTableRow:
			table.Content = append(table.Content, c.tableRow(child, "tableCell"))
		}
	}
	return table
}

// tableRow converts one goldmark header or body row into an ADF tableRow
// whose cells carry cellType ("tableHeader" or "tableCell").
func (c *mdConverter) tableRow(row ast.Node, cellType string) Node {
	out := Node{Type: "tableRow"}
	for cell := row.FirstChild(); cell != nil; cell = cell.NextSibling() {
		if cell.Kind() != extast.KindTableCell {
			continue
		}
		out.Content = append(out.Content, Node{
			Type:    cellType,
			Content: []Node{{Type: "paragraph", Content: c.inlineChildren(cell)}},
		})
	}
	return out
}

// inlineChildren converts the inline children of a block node to ADF
// inline nodes, threading marks down onto leaf text nodes.
func (c *mdConverter) inlineChildren(n ast.Node) []Node {
	var out []Node
	for child := n.FirstChild(); child != nil; child = child.NextSibling() {
		out = append(out, c.inline(child, nil)...)
	}
	return out
}

// inline converts one inline goldmark node, applying the accumulated
// marks to every leaf text node it produces.
func (c *mdConverter) inline(n ast.Node, marks []Mark) []Node {
	switch n.Kind() {
	case ast.KindText:
		t := n.(*ast.Text)
		nodes := []Node{}
		if seg := string(t.Segment.Value(c.source)); seg != "" {
			if c.wikiEmoji {
				if split := splitWikiEmoji(seg, marks); split != nil {
					nodes = append(nodes, split...)
				} else {
					nodes = append(nodes, Node{Type: "text", Text: seg, Marks: cloneMarks(marks)})
				}
			} else {
				nodes = append(nodes, Node{Type: "text", Text: seg, Marks: cloneMarks(marks)})
			}
		}
		// A trailing hard break inside a paragraph is a real ADF node.
		if t.HardLineBreak() {
			nodes = append(nodes, Node{Type: "hardBreak"})
		} else if t.SoftLineBreak() {
			// Soft breaks render as a space in ADF inline flow. When the
			// run ends on an emoji node the space needs its own text node —
			// an emoji carries no top-level text.
			if last := len(nodes) - 1; last >= 0 {
				if nodes[last].Type == "text" {
					nodes[last].Text += " "
				} else {
					nodes = append(nodes, Node{Type: "text", Text: " "})
				}
			}
		}
		return nodes
	case ast.KindString:
		s := n.(*ast.String)
		return []Node{{Type: "text", Text: string(s.Value), Marks: cloneMarks(marks)}}
	case ast.KindEmphasis:
		mark := Mark{Type: "em"}
		if n.(*ast.Emphasis).Level == 2 {
			mark.Type = "strong"
		}
		return c.inlineRun(n, append(cloneMarks(marks), mark))
	case ast.KindLink:
		link := n.(*ast.Link)
		mark := Mark{Type: "link", Attrs: map[string]any{"href": string(link.Destination)}}
		return c.inlineRun(n, append(cloneMarks(marks), mark))
	case ast.KindCodeSpan:
		// The ADF spec allows the code mark to combine only with link.
		// Resolve the conflict here, where the Markdown source is still at
		// hand: keep code (and any link) and drop the decorative marks.
		// The text and its code mark survive verbatim — like an image
		// degrading to its alt-text link, this loses decoration, not
		// content — so the warning is a non-lossy downgrade and the
		// default strict mutation mode proceeds with it.
		kept, dropped := sanitizeCodeMarks(cloneMarks(marks))
		if len(dropped) > 0 {
			w := c.sourceMapped(n, Warning{
				Type:     "markdown_lossy_conversion",
				Message:  fmt.Sprintf("Markdown formatting cannot combine with inline code in ADF (code combines only with link); kept the code mark and dropped %s", strings.Join(dropped, ", ")),
				NodeType: "codeSpan",
				MarkType: dropped[0],
				Lossy:    false,
			})
			c.warnings = append(c.warnings, w)
		}
		text := string(codeSpanText(n, c.source))
		return []Node{{Type: "text", Text: text, Marks: append(kept, Mark{Type: "code"})}}
	case ast.KindAutoLink:
		al := n.(*ast.AutoLink)
		url := string(al.URL(c.source))
		mark := Mark{Type: "link", Attrs: map[string]any{"href": url}}
		return []Node{{Type: "text", Text: url, Marks: append(cloneMarks(marks), mark)}}
	case extast.KindStrikethrough:
		return c.inlineRun(n, append(cloneMarks(marks), Mark{Type: "strike"}))
	case extast.KindTaskCheckBox:
		// Inside a task item the checkbox became the taskItem state attr;
		// in a mixed list it degrades to the literal GFM text (the list
		// carries one downgrade warning).
		if c.inTaskItem {
			return nil
		}
		// The literal carries the separator space: goldmark strips the
		// space between "]" and the item text from the following segment.
		box := "[ ] "
		if n.(*extast.TaskCheckBox).IsChecked {
			box = "[x] "
		}
		return []Node{{Type: "text", Text: box, Marks: cloneMarks(marks)}}
	case ast.KindImage:
		return c.image(n.(*ast.Image), marks)
	case ast.KindRawHTML:
		c.warn(n, "inline raw HTML", "")
		return nil
	default:
		c.warn(n, "inline "+n.Kind().String(), "")
		return nil
	}
}

// sanitizeCodeMarks applies the ADF mark-combination rule at conversion
// time: the code mark combines only with link, so every other accumulated
// mark is dropped. Returns the surviving marks and the dropped mark types.
func sanitizeCodeMarks(marks []Mark) ([]Mark, []string) {
	var kept []Mark
	var dropped []string
	for _, m := range marks {
		if m.Type == "link" {
			kept = append(kept, m)
			continue
		}
		dropped = append(dropped, m.Type)
	}
	return kept, dropped
}

// image degrades a Markdown image to its alt text (the URL when the alt is
// empty) carrying a link mark to the image URL. ADF cannot embed external
// images by URL — media nodes need attachment IDs — so the clickable
// reference is the faithful fallback. The downgrade is reported with
// Lossy=false: the reference survives in full, so a strict mutation is not
// aborted (only silently-dropped content trips the strict gate).
func (c *mdConverter) image(img *ast.Image, marks []Mark) []Node {
	url := string(img.Destination)
	if url == "" {
		c.warn(img, "image", "image with no URL dropped")
		return nil
	}
	alt := imageAltText(img, c.source)
	if alt == "" {
		alt = url
	}
	c.warnDowngrade(img, "image", "rendered as a link to "+url)
	mark := Mark{Type: "link", Attrs: map[string]any{"href": url}}
	return []Node{{Type: "text", Text: alt, Marks: append(cloneMarks(marks), mark)}}
}

// imageAltText collects the image's child text runs (the [alt] part).
func imageAltText(img *ast.Image, source []byte) string {
	var b strings.Builder
	for child := img.FirstChild(); child != nil; child = child.NextSibling() {
		if t, ok := child.(*ast.Text); ok {
			b.Write(t.Segment.Value(source))
		}
	}
	return strings.TrimSpace(b.String())
}

// inlineRun converts the children of an inline container (emphasis,
// link) and threads marks onto every produced leaf.
func (c *mdConverter) inlineRun(n ast.Node, marks []Mark) []Node {
	var out []Node
	for child := n.FirstChild(); child != nil; child = child.NextSibling() {
		out = append(out, c.inline(child, marks)...)
	}
	return out
}

func cloneMarks(marks []Mark) []Mark {
	if len(marks) == 0 {
		return nil
	}
	out := make([]Mark, len(marks))
	copy(out, marks)
	return out
}

func codeBlockText(n ast.Node, source []byte) string {
	var b strings.Builder
	lines := n.Lines()
	for i := 0; i < lines.Len(); i++ {
		segment := lines.At(i)
		b.Write(segment.Value(source))
	}
	return b.String()
}

func codeSpanText(n ast.Node, source []byte) []byte {
	var b strings.Builder
	for child := n.FirstChild(); child != nil; child = child.NextSibling() {
		if t, ok := child.(*ast.Text); ok {
			b.Write(t.Segment.Value(source))
		}
	}
	return []byte(b.String())
}
