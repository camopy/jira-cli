package cli

import (
	"io"
	"strings"

	"github.com/gechr/clog"
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
//	  ],
//	  "pagination": {...}
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
	logger := clog.New(clog.NewOutput(w, clog.ColorAuto))

	m, ok := data.(map[string]any)
	if !ok {
		return writeGenericPlain(logger, messageForCommand(command), data)
	}
	comments := normalizeMapList(m["comments"])
	style := authPlainStyle{tty: cfg.tty, theme: cfg.theme}

	header := style.bold("Comments")
	if count := len(comments); count > 0 {
		header += style.dim("  (" + plainPluralize(count, "comment", "comments") + ")")
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
	body, _ := c["body"].(string)
	created, _ := c["created"].(string)
	updated, _ := c["updated"].(string)

	authorName := "(unknown)"
	if author, ok := c["author"].(map[string]any); ok {
		if n, ok := author["display_name"].(string); ok && strings.TrimSpace(n) != "" {
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
		if visType != "" || visValue != "" {
			parts = append(parts, style.warn("["+visType+":"+visValue+"]"))
		}
	}
	preview := flattenPreview(body, 80)
	if preview != "" {
		parts = append(parts, preview)
	}
	return strings.Join(parts, "  ")
}

// flattenPreview collapses whitespace and truncates `s` to at most n runes
// so multi-line Markdown bodies render as a one-line preview.
func flattenPreview(s string, n int) string {
	flat := strings.Join(strings.Fields(s), " ")
	return truncate(flat, n)
}
