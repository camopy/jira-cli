package cli

import (
	"io"
	"strings"

	"github.com/gechr/clog"
	"github.com/gechr/x/human"
	xstrings "github.com/gechr/x/strings"
)

// WriteWatcherListPlain renders the `issue.watchers.list` envelope for
// human consumption. Layout: a header line with the watcher count and a
// "(you are watching)" affordance when `is_watching: true`, followed by
// one row per watcher with displayName, truncated accountId, email (or
// `(hidden)` when the token can't surface it), and an active marker.
//
// Mirrors the `plain_link.go` renderer's shape so the dispatcher in
// plain.go can wire it with the same `command/data/opts` signature.
func WriteWatcherListPlain(w io.Writer, command string, data any, opts ...PlainOption) error {
	cfg := defaultPlainConfig()
	for _, opt := range opts {
		opt(&cfg)
	}
	logger := clog.New(clog.NewOutput(w, clog.ColorAuto))
	logger.SetStyles(plainLoggerStyles())

	m, ok := data.(map[string]any)
	if !ok {
		return writeGenericPlain(logger, cfg, messageForCommand(command, data), data)
	}
	watchers := normalizeMapList(m["watchers"])
	style := authPlainStyle{tty: cfg.tty, theme: cfg.theme}

	count := 0
	if c, ok := m["watch_count"].(float64); ok {
		count = int(c)
	} else {
		count = len(watchers)
	}
	title := "Watchers"
	if cfg.resultKey != "" {
		title += " on " + cfg.resultKey
	}
	header := style.bold(title) + style.dim("  ("+human.Pluralize(count, "watcher", "watchers")+")")
	if isWatching, _ := m["is_watching"].(bool); isWatching {
		header += "  " + style.emph("(you are watching)")
	}
	logger.Info().Parts(clog.PartMessage).Msg(header)

	if len(watchers) == 0 {
		logger.Info().Parts(clog.PartMessage).Msg(style.dim("  (no watchers visible)"))
		return nil
	}

	for _, watcher := range watchers {
		logger.Info().Parts(clog.PartMessage).Msg(watcherPlainLine(watcher, style))
	}
	return nil
}

// watcherPlainLine renders one watcher row. accountId is truncated to
// the trailing 8 runes (with a leading ellipsis) so the row stays
// terminal-friendly even when Atlassian Cloud emits the long
// "5e1f0…" form. Email collapses to `(hidden)` when the watcher's
// privacy settings or token scopes hide it.
func watcherPlainLine(m map[string]any, style authPlainStyle) string {
	displayName, _ := m["display_name"].(string)
	if displayName == "" {
		displayName = "(unknown)"
	}
	accountID, _ := m["account_id"].(string)
	email, _ := m["email_address"].(string)
	emailDisplay := email
	if xstrings.IsBlank(emailDisplay) {
		emailDisplay = "(hidden)"
	}

	parts := []string{
		"  " + style.bold(displayName),
		style.dim(truncateAccountID(accountID)),
	}
	if email == "" {
		parts = append(parts, style.dim(emailDisplay))
	} else {
		parts = append(parts, emailDisplay)
	}
	if active, ok := m["active"].(bool); ok && !active {
		parts = append(parts, style.warn("(inactive)"))
	}
	return strings.Join(parts, "  ")
}

// truncateAccountID compacts a Jira accountId to its trailing 8
// characters with a leading ellipsis. Atlassian Cloud accountIds are
// long opaque strings whose head is usually identical across users
// from the same site; the tail is what humans recognize.
func truncateAccountID(id string) string {
	if id == "" {
		return "(no id)"
	}
	const tail = 8
	runes := []rune(id)
	if len(runes) <= tail {
		return id
	}
	return "…" + string(runes[len(runes)-tail:])
}
