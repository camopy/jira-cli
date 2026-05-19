// Plain-text renderer for `jira boards list`. See in
// the boards research notes for the column /
// truncation contract this file implements.
package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/gechr/clog"
	termansi "github.com/gechr/x/ansi"
)

// WriteBoardListPlain renders the `boards.list` envelope's data block
// for human consumption. Layout: a header line with the board count,
// followed by a four-column table — id (right-aligned), name, type,
// projects (comma-joined with `+N` overflow at 3+ keys ). Empty
// list surfaces a single advisory line directing the user at the
// cache primer (the affordance).
//
// Mirrors the plain_link / plain_watcher signature so the dispatcher
// in plain.go can wire it with the same `(w, command, data, opts)`
// shape.
func WriteBoardListPlain(w io.Writer, command string, data any, opts ...PlainOption) error {
	cfg := defaultPlainConfig()
	for _, opt := range opts {
		opt(&cfg)
	}
	logger := clog.New(clog.NewOutput(w, clog.ColorAuto))

	m, ok := data.(map[string]any)
	if !ok {
		return writeGenericPlain(logger, messageForCommand(command), data)
	}
	rows := normalizeMapList(m["boards"])
	style := authPlainStyle{tty: cfg.tty, theme: cfg.theme}

	source := "fresh"
	if v, _ := m["from_cache"].(bool); v {
		source = "cache"
	}
	fetchedAt, _ := m["fetched_at"].(string)
	header := style.bold("Boards") + style.dim("  ("+plainPluralize(len(rows), "board", "boards")+", source: "+source)
	if fetchedAt != "" {
		header += style.dim(", fetched_at: " + fetchedAt)
	}
	header += style.dim(")")
	logger.Info().Parts(clog.PartMessage).Msg(header)

	if len(rows) == 0 {
		// the affordance — one advisory line nudging the user at the
		// cache primer. Italic via dim styling (clib theme has no
		// dedicated italic token in the no-color path).
		logger.Info().Parts(clog.PartMessage).Msg(style.dim("  No boards visible to this profile — try `jira cache boards --refresh` if you expected results."))
		return nil
	}

	for _, row := range rows {
		logger.Info().Parts(clog.PartMessage).Msg(boardPlainLine(row, style))
	}
	return nil
}

// boardPlainLine renders a single board row using the  column
// budget: id (right-aligned 5-char min), name (flexible 20-40 char
// truncated), type (left 6 char to fit `simple`), projects .
func boardPlainLine(m map[string]any, style authPlainStyle) string {
	idStr := boardIDString(m["id"])
	name, _ := m["name"].(string)
	if strings.TrimSpace(name) == "" {
		name = "(unnamed)"
	}
	typeName, _ := m["type"].(string)
	projects := boardProjectKeys(m["project_keys"])

	idCell := padLeft(idStr, 5)
	nameCell := padRight(truncate(name, 40), 24)
	typeCell := padRight(typeName, 6)
	projectsCell := boardProjectDescriptor(projects)

	return fmt.Sprintf("  %s  %s  %s  %s",
		style.dim(idCell),
		style.bold(nameCell),
		style.dim(typeCell),
		style.dim(projectsCell),
	)
}

// boardIDString tolerates the float64 that JSON round-trips produce
// while accepting native int / int64 / string forms too.
func boardIDString(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case int:
		return fmt.Sprintf("%d", x)
	case int64:
		return fmt.Sprintf("%d", x)
	case float64:
		return fmt.Sprintf("%d", int64(x))
	}
	return ""
}

// boardProjectKeys normalizes the JSON-decoded project list into a
// concrete []string so the descriptor logic doesn't have to branch on
// every call.
func boardProjectKeys(v any) []string {
	switch x := v.(type) {
	case []string:
		return x
	case []any:
		out := make([]string, 0, len(x))
		for _, raw := range x {
			if s, ok := raw.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

// boardProjectDescriptor implements : comma-join up to two keys;
// collapse to "+N" overflow at 3+ keys. Empty list returns "—".
func boardProjectDescriptor(keys []string) string {
	switch len(keys) {
	case 0:
		return "—"
	case 1:
		return keys[0]
	case 2:
		return keys[0] + ", " + keys[1]
	default:
		return fmt.Sprintf("%s, %s +%d", keys[0], keys[1], len(keys)-2)
	}
}

// padLeft right-aligns s in an n-column slot using ASCII spaces.
func padLeft(s string, n int) string {
	if width := termansi.StringWidth(s); width < n {
		return strings.Repeat(" ", n-width) + s
	}
	return s
}
