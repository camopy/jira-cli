package adf

import (
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

func FromMarkdown(markdown string) (Document, error) {
	source := []byte(markdown)
	reader := text.NewReader(source)
	root := goldmark.DefaultParser().Parse(reader)
	doc := Document{Type: "doc", Version: 1}
	for child := root.FirstChild(); child != nil; child = child.NextSibling() {
		if node, ok := fromGoldmarkNode(child, source); ok {
			doc.Content = append(doc.Content, node)
		}
	}
	if len(doc.Content) == 0 && strings.TrimSpace(markdown) != "" {
		doc.Content = append(doc.Content, Node{Type: "paragraph", Content: []Node{{Type: "text", Text: strings.TrimSpace(markdown)}}})
	}
	return doc, nil
}

func fromGoldmarkNode(n ast.Node, source []byte) (Node, bool) {
	switch n.Kind() {
	case ast.KindParagraph, ast.KindTextBlock:
		return Node{Type: "paragraph", Content: childNodes(n, source)}, true
	case ast.KindHeading:
		return Node{Type: "heading", Attrs: map[string]any{"level": n.(*ast.Heading).Level}, Content: childNodes(n, source)}, true
	case ast.KindText:
		t := n.(*ast.Text)
		return Node{Type: "text", Text: string(t.Segment.Value(source))}, true
	case ast.KindString:
		s := n.(*ast.String)
		return Node{Type: "text", Text: string(s.Value)}, true
	case ast.KindEmphasis:
		children := childNodes(n, source)
		mark := Mark{Type: "em"}
		if n.(*ast.Emphasis).Level == 2 {
			mark.Type = "strong"
		}
		for i := range children {
			children[i].Marks = append(children[i].Marks, mark)
		}
		return Node{Type: "paragraph", Content: children}, true
	case ast.KindLink:
		children := childNodes(n, source)
		link := n.(*ast.Link)
		mark := Mark{Type: "link", Attrs: map[string]any{"href": string(link.Destination)}}
		for i := range children {
			children[i].Marks = append(children[i].Marks, mark)
		}
		return Node{Type: "paragraph", Content: children}, true
	case ast.KindList:
		list := n.(*ast.List)
		nodeType := "bulletList"
		attrs := map[string]any(nil)
		if list.IsOrdered() {
			nodeType = "orderedList"
			if list.Start > 0 {
				attrs = map[string]any{"order": list.Start}
			}
		}
		return Node{Type: nodeType, Attrs: attrs, Content: childNodes(n, source)}, true
	case ast.KindListItem:
		return Node{Type: "listItem", Content: childNodes(n, source)}, true
	case ast.KindCodeBlock, ast.KindFencedCodeBlock:
		node := Node{Type: "codeBlock", Content: []Node{{Type: "text", Text: codeBlockText(n, source)}}}
		// Preserve the language hint from a fenced ```lang block. ADF's
		// codeBlock.attrs.language is what downstream renderers (Jira,
		// our TUI, etc.) use to apply syntax styling.
		if fenced, ok := n.(*ast.FencedCodeBlock); ok {
			if lang := string(fenced.Language(source)); lang != "" {
				node.Attrs = map[string]any{"language": lang}
			}
		}
		return node, true
	default:
		return Node{}, false
	}
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

func childNodes(n ast.Node, source []byte) []Node {
	var nodes []Node
	for child := n.FirstChild(); child != nil; child = child.NextSibling() {
		node, ok := fromGoldmarkNode(child, source)
		if !ok {
			continue
		}
		if node.Type == "paragraph" && shouldFlattenInlineParagraph(n.Kind()) {
			nodes = append(nodes, node.Content...)
		} else {
			nodes = append(nodes, node)
		}
	}
	return nodes
}

func shouldFlattenInlineParagraph(kind ast.NodeKind) bool {
	return kind == ast.KindParagraph || kind == ast.KindHeading || kind == ast.KindEmphasis || kind == ast.KindLink
}
