package adf

import (
	"fmt"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	extast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/text"
)

// markdownParser parses GFM with the table and strikethrough
// extensions enabled. Enabling the table extension means a pipe table
// is parsed as a real Table node — FromMarkdownLossy can then warn that
// table authoring is unsupported rather than letting the table render
// as literal pipe-laden paragraph text.
var markdownParser = goldmark.New(
	goldmark.WithExtensions(extension.Table, extension.Strikethrough),
).Parser()

// FromMarkdownLossy converts GFM Markdown to an ADF document and reports
// every Markdown construct it could not represent faithfully in the
// supported ADF node set. Each unsupported construct yields one
// Warning (lossy=true) naming the construct and its source position so
// callers — and strict-mode abort gates — can act on the content loss
// instead of letting it slip through silently.
func FromMarkdownLossy(markdown string) (Document, []Warning, error) {
	source := []byte(markdown)
	reader := text.NewReader(source)
	root := markdownParser.Parse(reader)

	conv := &mdConverter{source: source}
	doc := Document{Type: "doc", Version: 1}
	for child := root.FirstChild(); child != nil; child = child.NextSibling() {
		if node, ok := conv.block(child); ok {
			doc.Content = append(doc.Content, node)
		}
	}
	if len(doc.Content) == 0 && strings.TrimSpace(markdown) != "" && len(conv.warnings) == 0 {
		doc.Content = append(doc.Content, Node{Type: "paragraph", Content: []Node{{Type: "text", Text: strings.TrimSpace(markdown)}}})
	}
	return doc, conv.warnings, nil
}

// mdConverter carries the Markdown source plus the running warning list
// through the recursive goldmark walk.
type mdConverter struct {
	source   []byte
	warnings []Warning
}

// warn records one lossy-conversion warning naming an unsupported
// Markdown construct.
func (c *mdConverter) warn(construct, detail string) {
	msg := fmt.Sprintf("Markdown %s is not supported and was dropped during ADF conversion", construct)
	if detail != "" {
		msg += ": " + detail
	}
	c.warnings = append(c.warnings, Warning{
		Type:     "markdown_lossy_conversion",
		Message:  msg,
		NodeType: construct,
		Lossy:    true,
	})
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
		// Blockquote has no authoring path in the supported node set:
		// the quoted content cannot be preserved without dropping the
		// blockquote semantics. Warn and skip.
		c.warn("blockquote", "quoted block dropped")
		return Node{}, false
	case ast.KindThematicBreak:
		return Node{Type: "rule"}, true
	case ast.KindHTMLBlock:
		c.warn("raw HTML block", "")
		return Node{}, false
	case extast.KindTable:
		c.warn("table", "GFM table dropped")
		return Node{}, false
	default:
		c.warn("block "+n.Kind().String(), "")
		return Node{}, false
	}
}

// list converts a bullet or ordered list, recursing into list items.
func (c *mdConverter) list(list *ast.List) (Node, bool) {
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
		default:
			if node, ok := c.block(child); ok {
				out = append(out, node)
			}
		}
	}
	return out
}

// codeBlock converts an indented or fenced code block, preserving the
// fence language hint.
func (c *mdConverter) codeBlock(n ast.Node) Node {
	node := Node{Type: "codeBlock", Content: []Node{{Type: "text", Text: codeBlockText(n, c.source)}}}
	if fenced, ok := n.(*ast.FencedCodeBlock); ok {
		if lang := string(fenced.Language(c.source)); lang != "" {
			node.Attrs = map[string]any{"language": lang}
		}
	}
	return node
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
			nodes = append(nodes, Node{Type: "text", Text: seg, Marks: cloneMarks(marks)})
		}
		// A trailing hard break inside a paragraph is a real ADF node.
		if t.HardLineBreak() {
			nodes = append(nodes, Node{Type: "hardBreak"})
		} else if t.SoftLineBreak() {
			// Soft breaks render as a space in ADF inline flow.
			if len(nodes) > 0 {
				nodes[len(nodes)-1].Text += " "
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
		text := string(codeSpanText(n, c.source))
		return []Node{{Type: "text", Text: text, Marks: append(cloneMarks(marks), Mark{Type: "code"})}}
	case ast.KindAutoLink:
		al := n.(*ast.AutoLink)
		url := string(al.URL(c.source))
		mark := Mark{Type: "link", Attrs: map[string]any{"href": url}}
		return []Node{{Type: "text", Text: url, Marks: append(cloneMarks(marks), mark)}}
	case extast.KindStrikethrough:
		return c.inlineRun(n, append(cloneMarks(marks), Mark{Type: "strike"}))
	case ast.KindImage:
		c.warn("image", "inline image dropped")
		return nil
	case ast.KindRawHTML:
		c.warn("inline raw HTML", "")
		return nil
	default:
		c.warn("inline "+n.Kind().String(), "")
		return nil
	}
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
