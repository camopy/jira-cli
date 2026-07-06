// Plain-text renderer for `jira alias list`: one aligned `name → expansion`
// line per alias, natural-ordered by name, so the human output shows the
// same map the JSON envelope carries.
package cli

import (
	"io"

	"github.com/gechr/clog"
	termansi "github.com/gechr/x/ansi"
	xslices "github.com/gechr/x/slices"
)

// WriteAliasListPlain renders the `alias.list` envelope data — the config's
// alias name→expansion map — for human consumption. An empty map says so
// explicitly, so "no aliases" is distinguishable from broken output.
func WriteAliasListPlain(w io.Writer, command string, data any, opts ...PlainOption) error {
	cfg := defaultPlainConfig()
	for _, opt := range opts {
		opt(&cfg)
	}
	logger := clog.New(clog.NewOutput(w, clog.ColorAuto))
	logger.SetStyles(plainLoggerStyles())

	aliases, ok := data.(map[string]string)
	if !ok {
		return writeGenericPlain(logger, cfg, messageForCommand(command), data)
	}
	style := authPlainStyle{tty: cfg.tty, theme: cfg.theme}

	logger.Info().Parts(clog.PartMessage).Msg(style.bold("Aliases"))
	if len(aliases) == 0 {
		logger.Info().Parts(clog.PartMessage).Msg(style.dim("  (no aliases configured)"))
		return nil
	}

	names := make([]string, 0, len(aliases))
	nameCol := 0
	for name := range aliases {
		names = append(names, name)
		if width := termansi.StringWidth(name); width > nameCol {
			nameCol = width
		}
	}
	xslices.SortNatural(names)
	for _, name := range names {
		line := style.bold(padRight(name, nameCol)) + style.dim("  →  ") + aliases[name]
		logger.Info().Parts(clog.PartMessage).Msg(line)
	}
	return nil
}
