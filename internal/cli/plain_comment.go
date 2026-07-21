package cli

import (
	"io"
	"strings"

	"github.com/gechr/clog"
	termansi "github.com/gechr/x/ansi"
	"github.com/gechr/x/human"
	xstrings "github.com/gechr/x/strings"
	"github.com/matcra587/jira-cli/internal/adf"
	"github.com/matcra587/jira-cli/internal/envelope"
)

// WriteCommentListPlain renders the `data` payload from `comment list` as
// a TTY-friendly clog table. Designed to be wired into WriteCommandPlain's
// dispatcher (lead's integration commit handles the wiring).
//
// Expected data shape (from cmd/jira/issue_comment.go's runCommentList):
//
//	{
//	  "comments": [
//	    {
//	      "id": "100",
//	      "body": "...markdown...",
//	      "author": {"account_id": "...", "display_name": "Alice"},
//	      "update_author": {...} | null,
//	      "created": "2026-04-01T10:00:00.000+0000",
//	      "updated": "2026-04-01T10:00:00.000+0000",
//	      "visibility": {"type": "role", "value": "Developers"} | null
//	    }, ...
//	  ]
//	}
//
// The renderer prints one line per comment with:
//   - id (bold)
//   - author display_name
//   - created (raw, no relative-time math — Atlassian's RFC 3339 is enough)
//   - "(edited)" marker when updated > created
//   - visibility tag when set
//   - body preview (first ~80 runes, newlines flattened)
//
// Lossy-conversion warnings travel through the envelope's warnings[] array;
// the renderer doesn't repeat them inline (they're routed to stderr).
func WriteCommentListPlain(w io.Writer, command string, data any, opts ...PlainOption) error {
	cfg := defaultPlainConfig()
	for _, opt := range opts {
		opt(&cfg)
	}
	logger := newPlainLogger(w)

	// Read the typed IssueCommentListOutput directly so comment bodies keep
	// their native adf.Document type — a mapFromAny round-trip would marshal
	// each body to a JSON map, which commentBodyText renders from too but only
	// after a redundant re-parse. A keyed child arrives as a raw map (its body
	// already a JSON map, folded by the parent's keyedResultRows walk) and a
	// legacy caller may pass a map as well, so both fall to normalizeMapList.
	var comments []map[string]any
	switch d := data.(type) {
	case envelope.IssueCommentListOutput:
		comments = d.Comments
	case map[string]any:
		comments = normalizeMapList(d["comments"])
	default:
		if m := mapFromAny(data); m != nil {
			comments = normalizeMapList(m["comments"])
		} else {
			return writeGenericPlain(logger, cfg, messageForCommand(command, data), data)
		}
	}
	style := authPlainStyle{tty: cfg.tty, theme: cfg.theme}

	title := "Comments"
	if cfg.resultKey != "" {
		title += " on " + cfg.resultKey
	}
	header := style.bold(title)
	if count := len(comments); count > 0 {
		header += style.dim("  (" + human.Pluralize(count, "comment", "comments") + ")")
	}
	logger.Info().Parts(clog.PartMessage).Msg(header)

	if len(comments) == 0 {
		logger.Info().Parts(clog.PartMessage).Msg(style.dim("  (no comments)"))
		return nil
	}

	for _, c := range comments {
		logger.Info().Parts(clog.PartMessage).Msg(commentPlainLine(c, style))
	}
	return nil
}

func commentPlainLine(c map[string]any, style authPlainStyle) string {
	id, _ := c["id"].(string)
	body := commentBodyText(c["body"])
	created, _ := c["created"].(string)
	updated, _ := c["updated"].(string)

	authorName := "(unknown)"
	if author, ok := c["author"].(map[string]any); ok {
		if n, ok := author["display_name"].(string); ok && !xstrings.IsBlank(n) {
			authorName = n
		}
	}

	parts := []string{
		style.bold(padRight("#"+id, 8)),
		padRight(authorName, 18),
		style.dim(padRight(created, 26)),
	}
	if updated != "" && updated != created {
		parts = append(parts, style.dim("(edited)"))
	}
	if vis, ok := c["visibility"].(map[string]any); ok && vis != nil {
		visType, _ := vis["type"].(string)
		visValue, _ := vis["value"].(string)
		if xstrings.AnyNonEmpty(visType, visValue) {
			parts = append(parts, style.warn("["+visType+":"+visValue+"]"))
		}
	}
	preview := flattenPreview(body, 80)
	if preview != "" {
		parts = append(parts, preview)
	}
	return strings.Join(parts, "  ")
}

// commentBodyText flattens a comment body for the one-line preview. Bodies
// arrive as native ADF documents on the single-key path (machine-mode parity
// with issue view). On a keyed child the parent's JSON walk has already
// marshaled the body to a map[string]any, so that shape is reconstructed and
// rendered too; strings pass through for any legacy shape.
func commentBodyText(v any) string {
	switch b := v.(type) {
	case string:
		return b
	case adf.Document:
		return adf.ToMarkdown(b)
	case *adf.Document:
		if b != nil {
			return adf.ToMarkdown(*b)
		}
	case map[string]any:
		if doc, ok := adfDocumentFromMap(b); ok {
			return adf.ToMarkdown(doc)
		}
	}
	return ""
}

// flattenPreview collapses whitespace and truncates `s` to at most n display
// cells so multi-line Markdown bodies render as a one-line preview. The body
// arrives as a typed ADF document, which the up-front sanitizePlainData walk
// leaves untouched, so this is its terminal-sanitizer boundary. It uses the
// block variant because strings.Fields needs the tabs and newlines to
// survive — the text variant would drop them and glue adjacent words
// together.
func flattenPreview(s string, n int) string {
	flat := strings.Join(strings.Fields(SanitizeTerminalBlock(s)), " ")
	return termansi.Truncate(flat, n, "…")
}
