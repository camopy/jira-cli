package cli

import (
	"io"
	"regexp"

	"charm.land/glamour/v2/ansi"
	clibtheme "github.com/gechr/clib/theme"
	"github.com/gechr/clog"
	changelog "github.com/matcra587/jira-cli"
	"github.com/matcra587/jira-cli/internal/tui/components/markdown"
)

// ReleaseNotesResult is the envelope payload for `jira release-notes`: jira-cli's
// own embedded changelog. Releases carries the structured notes (newest first, or
// a single release when one is requested) and is what JSON consumers read — for a
// single release or --latest it holds one entry, so `.releases[0]` is that
// release. Markdown drives the human renderer and is excluded from JSON. It is
// shared with the releasenotes command package, which builds it, so the JSON
// envelope and the human renderer agree on one shape.
type ReleaseNotesResult struct {
	Releases []changelog.Release `json:"releases"`
	// Markdown is the notes to display in human output; excluded from JSON,
	// where Releases is the structured form.
	Markdown string `json:"-"`
}

// mdLinkRe matches a Markdown link. In the styled human render the version
// heading is shown as plain text (its URL is noise on a terminal); the raw
// Markdown keeps the link so a piped copy still lands cleanly in a release body.
var mdLinkRe = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`)

// writeReleaseNotesPlain renders the notes as Markdown. On a color terminal with
// a resolved theme it is styled through glamour — using a clib-derived style that
// gives each heading level its own hue, so it honors the user's theme and reads
// like a release page. Otherwise (piped, redirected, no theme, or NO_COLOR) it
// stays raw Markdown, dropping straight into a file or a GitHub release body.
func writeReleaseNotesPlain(w io.Writer, data any, cfg plainConfig) error {
	res, ok := data.(ReleaseNotesResult)
	if !ok {
		return writeGenericPlain(newPlainLogger(w), cfg, "release notes", data)
	}

	if cfg.tty && cfg.theme != nil && !clog.ColorsDisabled() {
		width := cfg.termWidth
		if width <= 0 {
			width = 100
		}
		md := mdLinkRe.ReplaceAllString(res.Markdown, "$1")
		rendered := markdown.NewRenderer(releaseNotesStyle(cfg.theme)).Render("release-notes", width, md)
		_, err := io.WriteString(w, rendered+"\n")
		return err
	}

	_, err := io.WriteString(w, res.Markdown)
	return err
}

// releaseNotesStyle builds on the shared clib-derived Markdown style, then gives
// each changelog heading level its own treatment so releases and change kinds
// are easy to tell apart at a glance regardless of theme: H1 (the title) keeps
// the base magenta; H2 (a release) becomes orange and underlined, so it reads as
// a divider even where a theme's hues sit close together; H3 (a change kind)
// stays blue.
func releaseNotesStyle(t *clibtheme.Theme) ansi.StyleConfig {
	cfg := markdown.StyleFromTheme(t)
	orange := markdown.ColorToken(t.Orange.GetForeground())
	underline := true
	cfg.H2.Color = &orange
	cfg.H2.Underline = &underline
	return cfg
}
