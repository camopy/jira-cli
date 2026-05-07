package adf

import (
	"strconv"
	"strings"
)

type Segment struct {
	Text string
	Kind string
}

func ToPlain(doc Document) string {
	var parts []string
	for _, node := range doc.Content {
		collectPlain(&parts, node)
	}
	return strings.TrimSpace(strings.Join(parts, " "))
}

func ToMarkdown(doc Document) string {
	var lines []string
	for _, node := range doc.Content {
		if rendered := markdownBlock(node); rendered != "" {
			lines = append(lines, rendered)
		}
	}
	return strings.TrimSpace(strings.Join(lines, "\n\n")) + "\n"
}

func ToFormatted(doc Document) []Segment {
	var segments []Segment
	for _, node := range doc.Content {
		collectFormatted(&segments, node, node.Type)
	}
	return segments
}

func collectPlain(parts *[]string, node Node) {
	if node.Text != "" {
		*parts = append(*parts, node.Text)
	}
	for _, child := range node.Content {
		collectPlain(parts, child)
	}
}

func markdownBlock(node Node) string {
	switch node.Type {
	case "paragraph":
		return markdownChildren(node)
	case "text":
		return markdownText(node)
	case "heading":
		level := attrInt(node.Attrs, "level", 1)
		return strings.Repeat("#", level) + " " + markdownChildren(node)
	case "bulletList":
		return markdownList(node, "-")
	case "orderedList":
		return markdownList(node, "1.")
	case "listItem":
		return markdownChildren(node)
	case "codeBlock":
		return "```\n" + strings.TrimSuffix(markdownChildren(node), "\n") + "\n```"
	default:
		return markdownChildren(node)
	}
}

func attrInt(attrs map[string]any, key string, fallback int) int {
	switch raw := attrs[key].(type) {
	case int:
		if raw > 0 {
			return raw
		}
	case float64:
		if raw > 0 {
			return int(raw)
		}
	}
	return fallback
}

func markdownChildren(node Node) string {
	var parts []string
	for _, child := range node.Content {
		if child.Type == "text" {
			parts = append(parts, markdownText(child))
			continue
		}
		parts = append(parts, markdownBlock(child))
	}
	return strings.Join(parts, "")
}

func markdownText(node Node) string {
	text := node.Text
	for i := len(node.Marks) - 1; i >= 0; i-- {
		mark := node.Marks[i]
		switch mark.Type {
		case "strong":
			text = "**" + text + "**"
		case "em":
			text = "*" + text + "*"
		case "code":
			text = "`" + text + "`"
		case "link":
			if href, ok := mark.Attrs["href"].(string); ok && href != "" {
				text = "[" + text + "](" + href + ")"
			}
		}
	}
	return text
}

func markdownList(node Node, marker string) string {
	lines := make([]string, 0, len(node.Content))
	for i, child := range node.Content {
		prefix := marker
		if node.Type == "orderedList" {
			prefix = strconv.Itoa(i+1) + "."
		}
		item := strings.TrimSpace(markdownBlock(child))
		lines = append(lines, prefix+" "+item)
	}
	return strings.Join(lines, "\n")
}

func collectFormatted(segments *[]Segment, node Node, inheritedKind string) {
	kind := inheritedKind
	if node.Type != "" && node.Type != "text" {
		kind = node.Type
	}
	if node.Type != "" && node.Type != "text" && len(node.Content) > 0 {
		plain := strings.TrimSpace(markdownChildren(node))
		if plain != "" {
			*segments = append(*segments, Segment{Text: plain, Kind: node.Type})
		}
	}
	if node.Text != "" {
		textKind := kind
		if textKind == "" {
			textKind = "text"
		}
		*segments = append(*segments, Segment{Text: node.Text, Kind: textKind})
		for _, mark := range node.Marks {
			*segments = append(*segments, Segment{Text: node.Text, Kind: mark.Type})
		}
	}
	for _, child := range node.Content {
		collectFormatted(segments, child, kind)
	}
}
