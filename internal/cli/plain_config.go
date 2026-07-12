package cli

import (
	"io"
	"strings"

	clibtheme "github.com/gechr/clib/theme"
	clogtheme "github.com/gechr/clog/theme"
)

// printThemeFor maps the resolved clib table theme onto the clog print
// palette used for syntax-highlighted printer output — the same light/dark
// split cmdutil.HumanJSONPrintTheme applies on the JSON path, derived here
// from the theme the plain renderer already carries instead of a second
// config load. A nil theme (config load failure upstream) keeps clog's
// built-in dark palette.
func printThemeFor(theme *clibtheme.Theme) *clogtheme.Theme {
	if theme == nil {
		return nil
	}
	if theme.Background == clibtheme.BackgroundLight {
		return clogtheme.Light()
	}
	return clogtheme.Dark()
}

// writeConfigGetPlain renders one config value as syntax-highlighted TOML,
// nesting the dotted key path into real TOML tables so the human output has
// the same shape the value has in the config file itself:
//
//	$ jira config get theme.name
//	[theme]
//	name = 'dark'
//
// A payload that does not carry the expected key/value shape falls back to
// the generic renderer rather than printing an empty document.
func writeConfigGetPlain(w io.Writer, data any, cfg plainConfig) error {
	m, _ := data.(map[string]any)
	key, _ := m["key"].(string)
	value, hasValue := m["value"]
	if key == "" || !hasValue || value == nil {
		return writeGenericPlain(newPlainLogger(w), cfg, "result", data)
	}
	doc := value
	parts := strings.Split(key, ".")
	for i := len(parts) - 1; i >= 0; i-- {
		doc = map[string]any{parts[i]: doc}
	}
	return WriteHumanTOML(w, doc, printThemeFor(cfg.theme))
}

// writeConfigProfilePlain renders the profile listing as syntax-highlighted
// TOML — active_profile up top, one [[profiles]] block per profile —
// mirroring the config file section the data came from. Profiles are
// metadata-only by design, so nothing credential-shaped can transit this
// surface. An unexpected payload falls back to the generic renderer.
func writeConfigProfilePlain(w io.Writer, data any, cfg plainConfig) error {
	m, ok := data.(map[string]any)
	if !ok {
		return writeGenericPlain(newPlainLogger(w), cfg, "result", data)
	}
	return WriteHumanTOML(w, m, printThemeFor(cfg.theme))
}
