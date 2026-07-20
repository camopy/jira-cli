// Plain-text renderer for `jira alias list`: one aligned `name → expansion`
// line per alias, natural-ordered by name, so the human output shows the
// same map the JSON envelope carries.

package cli

import (
	"io"

	"github.com/gechr/clog"
	termansi "github.com/gechr/x/ansi"
	xmaps "github.com/gechr/x/maps"
)

// WriteAliasListPlain renders the `alias.list` envelope data — the config's
// alias name→expansion map — for human consumption. An empty map says so
// explicitly, so "no aliases" is distinguishable from broken output.
func WriteAliasListPlain(w io.Writer, command string, data any, opts ...PlainOption) error {
	cfg := defaultPlainConfig()
	for _, opt := range opts {
		opt(&cfg)
	}
	logger := newPlainLogger(w)

	// alias.list data wraps the map: {aliases, count}. Unwrap; anything
	// else falls back to the generic renderer.
	wrapper, ok := data.(map[string]any)
	if !ok {
		return writeGenericPlain(logger, cfg, messageForCommand(command, data), data)
	}
	aliases, ok := wrapper["aliases"].(map[string]string)
	if !ok {
		return writeGenericPlain(logger, cfg, messageForCommand(command, data), data)
	}
	style := authPlainStyle{tty: cfg.tty, theme: cfg.theme}

	logger.Info().Parts(clog.PartMessage).Msg(style.bold("Aliases"))
	if len(aliases) == 0 {
		logger.Info().Parts(clog.PartMessage).Msg(style.dim("  (no aliases configured)"))
		return nil
	}

	names := xmaps.KeysNatural(aliases)
	nameCol := 0
	for _, name := range names {
		if width := termansi.StringWidth(name); width > nameCol {
			nameCol = width
		}
	}
	for _, name := range names {
		line := style.bold(padRight(name, nameCol)) + style.dim("  →  ") + aliases[name]
		logger.Info().Parts(clog.PartMessage).Msg(line)
	}
	return nil
}
