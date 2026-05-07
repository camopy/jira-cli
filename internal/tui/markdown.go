package tui

import (
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/matcra587/jira-cli/internal/tui/theme"
	"github.com/matcra587/jira-cli/pkg/adf"
)

// renderADFPlain returns a plain-text rendering for non-styled callers
// (used by --plain output paths). Most TUI rendering goes through
// renderADFStyled.
func renderADFPlain(doc adf.Document, _ int) string {
	if len(doc.Content) == 0 {
		return ""
	}
	return strings.TrimRight(adf.ToMarkdown(doc), "\n")
}

// renderADFStyled walks the typed ADF document and emits terminal-styled
// text: headings/bold/em/code/links rendered as formatted terminal
// text, lists indented, code blocks faintly fenced. When
// isTerminal=false the function returns the same content with no ANSI
// escapes — caller decides which to use based on TTY detection so a
// piped --plain consumer never sees stray escape codes.
func renderADFStyled(doc adf.Document, _ int, isTerminal bool) string {
	if len(doc.Content) == 0 {
		return ""
	}
	var (
		headingStyle = lipgloss.NewStyle().Bold(true).Foreground(theme.ColorHeaderFg)
		strongStyle  = lipgloss.NewStyle().Bold(true)
		emStyle      = lipgloss.NewStyle().Italic(true)
		codeStyle    = lipgloss.NewStyle().Faint(true)
		bulletStyle  = lipgloss.NewStyle().Foreground(theme.ColorHeaderFg)
		ruleStyle    = lipgloss.NewStyle().Faint(true)
	)

	if !isTerminal {
		// Strip the fields that emit ANSI; plain output goes through
		// renderADFPlain's deterministic path instead so we don't have
		// two sources of truth for the non-styled rendering.
		return renderADFPlain(doc, 0)
	}

	var b strings.Builder
	for i, block := range doc.Content {
		if i > 0 {
			b.WriteString("\n")
		}
		switch block.Type {
		case "heading":
			level := attrIntOrDefault(block.Attrs, "level", 1)
			prefix := strings.Repeat("#", level) + " "
			b.WriteString(headingStyle.Render(prefix + inlineStyled(block.Content, strongStyle, emStyle, codeStyle)))
		case "paragraph":
			b.WriteString(inlineStyled(block.Content, strongStyle, emStyle, codeStyle))
		case "bulletList":
			renderListStyled(&b, block, "•", bulletStyle, strongStyle, emStyle, codeStyle)
		case "orderedList":
			renderListStyled(&b, block, "1.", bulletStyle, strongStyle, emStyle, codeStyle)
		case "codeBlock":
			lang, _ := block.Attrs["language"].(string)
			text := concatText(block.Content)
			header := "code"
			if lang != "" {
				header = "code " + lang
			}
			b.WriteString(codeStyle.Render("┌─ " + header + " ─┐"))
			b.WriteString("\n")
			for _, line := range strings.Split(strings.TrimRight(text, "\n"), "\n") {
				b.WriteString(codeStyle.Render("│ " + line))
				b.WriteString("\n")
			}
			b.WriteString(codeStyle.Render("└────────┘"))
		case "blockquote":
			lines := strings.Split(strings.TrimRight(inlineStyled(block.Content, strongStyle, emStyle, codeStyle), "\n"), "\n")
			for j, line := range lines {
				if j > 0 {
					b.WriteString("\n")
				}
				b.WriteString(emStyle.Render("│ ") + line)
			}
		case "rule":
			b.WriteString(ruleStyle.Render("────────"))
		case "panel":
			panelType, _ := block.Attrs["panelType"].(string)
			b.WriteString(headingStyle.Render("[" + panelType + "] "))
			b.WriteString(inlineStyled(block.Content, strongStyle, emStyle, codeStyle))
		default:
			// Opaque or unsupported block — fall back to plain text rendering
			// so the user sees content rather than nothing.
			b.WriteString(strings.TrimRight(adf.ToMarkdown(adf.Document{Type: "doc", Version: 1, Content: []adf.Node{block}}), "\n"))
		}
	}
	return b.String()
}

// inlineStyled renders a slice of inline nodes — text, hardBreak, mention,
// emoji, date, status, inlineCard — applying the requested mark styles.
func inlineStyled(nodes []adf.Node, strongStyle, emStyle, codeStyle lipgloss.Style) string {
	var b strings.Builder
	for _, n := range nodes {
		switch n.Type {
		case "text":
			b.WriteString(applyMarks(n, strongStyle, emStyle, codeStyle))
		case "hardBreak":
			b.WriteString("\n")
		case "mention":
			text, _ := n.Attrs["text"].(string)
			if text == "" {
				if id, _ := n.Attrs["id"].(string); id != "" {
					text = "@" + id
				}
			}
			b.WriteString(strongStyle.Render(text))
		case "emoji":
			text, _ := n.Attrs["text"].(string)
			if text == "" {
				if name, _ := n.Attrs["shortName"].(string); name != "" {
					text = name
				}
			}
			b.WriteString(text)
		case "date":
			ts, _ := n.Attrs["timestamp"].(string)
			b.WriteString(emStyle.Render(ts))
		case "status":
			text, _ := n.Attrs["text"].(string)
			b.WriteString(strongStyle.Render("[" + text + "]"))
		case "inlineCard":
			url, _ := n.Attrs["url"].(string)
			b.WriteString(emStyle.Render(url))
		default:
			// Recurse for any wrapper container (e.g. paragraph inside listItem).
			b.WriteString(inlineStyled(n.Content, strongStyle, emStyle, codeStyle))
		}
	}
	return b.String()
}

// applyMarks layers mark styles onto a text node. Strong/em/code map to
// their lipgloss styles; link uses OSC 8 hyperlinks so the URL is
// activatable in modern terminals AND visible in older ones.
func applyMarks(n adf.Node, strongStyle, emStyle, codeStyle lipgloss.Style) string {
	out := n.Text
	for _, m := range n.Marks {
		switch m.Type {
		case "strong":
			out = strongStyle.Render(out)
		case "em":
			out = emStyle.Render(out)
		case "code":
			out = codeStyle.Render(out)
		case "link":
			if href, ok := m.Attrs["href"].(string); ok && href != "" {
				out = "\x1b]8;;" + href + "\x1b\\" + out + " (" + href + ")" + "\x1b]8;;\x1b\\"
			}
		}
	}
	return out
}

// renderListStyled writes either a bulleted or numbered list. Nested
// blocks inside list items render with two-space indentation per level.
func renderListStyled(b *strings.Builder, list adf.Node, marker string, bulletStyle, strongStyle, emStyle, codeStyle lipgloss.Style) {
	for i, item := range list.Content {
		if i > 0 {
			b.WriteString("\n")
		}
		prefix := marker
		if list.Type == "orderedList" {
			prefix = numberMarker(i + 1)
		}
		body := inlineStyled(item.Content, strongStyle, emStyle, codeStyle)
		b.WriteString(bulletStyle.Render(prefix) + " " + body)
	}
}

func numberMarker(n int) string {
	// Inline two-rune itoa to avoid a strconv dependency cycle.
	if n < 10 {
		return string(rune('0'+n)) + "."
	}
	hi := n / 10
	lo := n % 10
	return string([]rune{rune('0' + hi), rune('0' + lo), '.'})
}

// concatText returns the concatenated text content of a node tree.
func concatText(nodes []adf.Node) string {
	var b strings.Builder
	for _, n := range nodes {
		if n.Type == "text" {
			b.WriteString(n.Text)
		}
		b.WriteString(concatText(n.Content))
	}
	return b.String()
}

// attrIntOrDefault reads an int attr robustly across JSON's float64 default.
func attrIntOrDefault(attrs map[string]any, key string, fallback int) int {
	switch v := attrs[key].(type) {
	case int:
		return v
	case float64:
		return int(v)
	}
	return fallback
}
