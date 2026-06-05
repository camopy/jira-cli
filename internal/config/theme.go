package config

import (
	"fmt"
	"os"
	"strings"

	clibtheme "github.com/gechr/clib/theme"
)

// EnvThemeName overrides the default theme process-wide. It matches the prefix
// set via clibtheme.SetEnvPrefix("JIRA") at help-renderer setup, so the same
// variable drives help, plain output, and the TUI.
const EnvThemeName = "JIRA_THEME"

// ThemeNameAuto selects clib's light or dark theme by detecting the terminal
// background, so hash-based entity colors stay readable on either surface. It
// is opt-in, never the default: background detection misfires silently under
// tmux, SSH, and container exec — all common developer environments — and a
// wrong guess is actively worse than the fixed palette, so the default stays
// dark until detection has a release cycle of field reports. A user whose
// terminal is misdetected pins "light" or "dark" instead.
const ThemeNameAuto = "auto"

var ThemeNameValues = []string{
	"auto",
	"dark",
	"light",
	"default",
	"plain",
	"catppuccin-frappe",
	"catppuccin-latte",
	"catppuccin-macchiato",
	"catppuccin-mocha",
	"dracula",
	"gruvbox-dark",
	"gruvbox-light",
	"monochrome",
	"monokai",
	"nord",
	"one-dark",
	"synthwave",
	"solarized",
	"tokyo-night",
}

// canonicalThemeName rewrites the theme names jira-cli accepted before the clib
// v0.5 upgrade to the names v0.5 understands. v0.5 split several single themes
// into explicit light/dark variants and dropped the "default" alias; without
// this a config carrying one of these names — including "default", the value
// fresh installs have written for years — would fail to load. Each target
// reproduces the exact palette the old name resolved to, verified against clib
// v0.4.15: "default" is Dark, "solarized" is solarized-light (its base01
// comment color), and "plain"/"monochrome" render identically on either
// background. Current and unknown names pass through untouched.
func canonicalThemeName(name string) string {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "default":
		return "dark"
	case "plain":
		return "plain-dark"
	case "monochrome":
		return "monochrome-dark"
	case "solarized":
		return "solarized-light"
	default:
		return name
	}
}

// IsAutoTheme reports whether name selects the background-detecting theme.
func IsAutoTheme(name string) bool {
	return strings.EqualFold(strings.TrimSpace(name), ThemeNameAuto)
}

func ValidateThemeName(name string) error {
	name = strings.TrimSpace(name)
	if name == "" || IsAutoTheme(name) {
		return nil
	}
	var th clibtheme.Theme
	if err := th.UnmarshalText([]byte(canonicalThemeName(name))); err != nil {
		return fmt.Errorf("theme.name: %w", err)
	}
	return nil
}

// themeFromName resolves a single theme name, accepting legacy pre-v0.5 names.
// It returns nil for an empty or unrecognized name so callers can fall back.
func themeFromName(name string) *clibtheme.Theme {
	if strings.TrimSpace(name) == "" {
		return nil
	}
	var th clibtheme.Theme
	if th.UnmarshalText([]byte(canonicalThemeName(name))) != nil {
		return nil
	}
	return &th
}

// DefaultTheme resolves the process default theme: the JIRA_THEME override when
// set and valid, otherwise the dark built-in. This restores clib v0.4.15's
// Default(), which the v0.5 upgrade removed, keeping the documented JIRA_THEME
// override working for plain output, help, and the TUI.
func DefaultTheme() *clibtheme.Theme {
	if th := themeFromName(os.Getenv(EnvThemeName)); th != nil {
		return th
	}
	return clibtheme.Dark()
}

// ThemeForName resolves an explicit theme name (e.g. config theme.name),
// falling back to DefaultTheme when the name is empty or unrecognized. Legacy
// pre-v0.5 names are accepted.
func ThemeForName(name string) *clibtheme.Theme {
	if th := themeFromName(name); th != nil {
		return th
	}
	return DefaultTheme()
}

// AutoTheme resolves the opt-in "auto" theme: clib's light or dark theme chosen
// by the terminal background of out (the stream styled output goes to). The
// JIRA_THEME override still wins, matching every other resolution path. When
// there is nothing to detect — out is not a terminal, e.g. --color=always piped
// into a pager — it falls back to dark: dark matches the convention of every
// major ANSI CLI (git, ripgrep, bat) and the dark-terminal skew of users who
// deliberately force color through a pipe.
func AutoTheme(out *os.File) *clibtheme.Theme {
	if th := themeFromName(os.Getenv(EnvThemeName)); th != nil {
		return th
	}
	bg, ok := clibtheme.DetectBackground(out)
	if !ok {
		bg = clibtheme.BackgroundDark
	}
	return clibtheme.DefaultPair().ForBackground(bg)
}
