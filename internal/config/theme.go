package config

import (
	"fmt"
	"strings"

	clibtheme "github.com/gechr/clib/theme"
)

var ThemeNameValues = []string{
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

func ValidateThemeName(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil
	}
	var th clibtheme.Theme
	if err := th.UnmarshalText([]byte(name)); err != nil {
		return fmt.Errorf("theme.name: %w", err)
	}
	return nil
}
