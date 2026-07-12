package cli

import (
	"io"
	"sort"
	"strings"

	"github.com/gechr/clog"
	termansi "github.com/gechr/x/ansi"
	"github.com/gechr/x/human"
)

func WriteLinkListPlain(w io.Writer, command string, data any, opts ...PlainOption) error {
	cfg := defaultPlainConfig()
	for _, opt := range opts {
		opt(&cfg)
	}
	logger := newPlainLogger(w)

	m, ok := data.(map[string]any)
	if !ok {
		return writeGenericPlain(logger, cfg, messageForCommand(command, data), data)
	}
	links := normalizeMapList(m["links"])
	style := authPlainStyle{tty: cfg.tty, theme: cfg.theme}

	key, _ := m["key"].(string)
	header := style.bold("Links on " + key)
	if count := len(links); count > 0 {
		header += style.dim("  (" + human.Pluralize(count, "link", "links") + ")")
	}
	logger.Info().Parts(clog.PartMessage).Msg(header)

	if len(links) == 0 {
		logger.Info().Parts(clog.PartMessage).Msg(style.dim("  (no links)"))
		return nil
	}

	for _, link := range links {
		row := linkPlainRowFrom(link)
		logger.Info().Parts(clog.PartMessage).Msg(linkPlainLine(row, style, cfg))
	}
	return nil
}

type linkPlainRow struct {
	Direction string
	TypeName  string
	Verb      string
	OtherKey  string
	Summary   string
	Status    string
}

func linkPlainRowFrom(m map[string]any) linkPlainRow {
	row := linkPlainRow{}
	row.Direction, _ = m["direction"].(string)
	if t, ok := m["type"].(map[string]any); ok {
		row.TypeName, _ = t["name"].(string)
		switch row.Direction {
		case "outward":
			row.Verb, _ = t["outward"].(string)
		case "inward":
			row.Verb, _ = t["inward"].(string)
		}
	}
	if other, ok := m["other_issue"].(map[string]any); ok {
		row.OtherKey, _ = other["key"].(string)
		row.Summary, _ = other["summary"].(string)
		row.Status, _ = other["status"].(string)
	}
	return row
}

func linkPlainLine(r linkPlainRow, style authPlainStyle, cfg plainConfig) string {
	icon := "->"
	if r.Direction == "inward" {
		icon = "<-"
	}
	if cfg.tty {
		switch r.Direction {
		case "inward":
			icon = "←"
		default:
			icon = "→"
		}
	}
	parts := []string{
		style.bold(icon),
		padRight(r.Verb, 14),
		style.bold(r.OtherKey),
		padRight(termansi.Truncate(r.Summary, 40, "…"), 40),
	}
	if r.Status != "" {
		parts = append(parts, style.dim("["+r.Status+"]"))
	}
	return strings.Join(parts, "  ")
}

func WriteLinkTypesPlain(w io.Writer, command string, data any, opts ...PlainOption) error {
	cfg := defaultPlainConfig()
	for _, opt := range opts {
		opt(&cfg)
	}
	logger := newPlainLogger(w)

	m, ok := data.(map[string]any)
	if !ok {
		return writeGenericPlain(logger, cfg, messageForCommand(command, data), data)
	}
	types := normalizeMapList(m["link_types"])
	style := authPlainStyle{tty: cfg.tty, theme: cfg.theme}

	fromCache, _ := m["from_cache"].(bool)
	source := "fresh"
	if fromCache {
		source = "cache"
	}
	fetchedAt, _ := m["fetched_at"].(string)
	header := style.bold("Link types") + style.dim("  (source: "+source)
	if fetchedAt != "" {
		header += style.dim(", fetched_at: " + fetchedAt)
	}
	header += style.dim(")")
	logger.Info().Parts(clog.PartMessage).Msg(header)

	if len(types) == 0 {
		logger.Info().Parts(clog.PartMessage).Msg(style.dim("  (no link types configured)"))
		return nil
	}

	sort.SliceStable(types, func(i, j int) bool {
		ni, _ := types[i]["name"].(string)
		nj, _ := types[j]["name"].(string)
		return ni < nj
	})

	for _, t := range types {
		name, _ := t["name"].(string)
		out, _ := t["outward"].(string)
		in, _ := t["inward"].(string)
		line := style.bold(padRight(name, 12)) + "  " + style.dim("outward: ") + padRight(out, 18) + "  " + style.dim("inward: ") + in
		logger.Info().Parts(clog.PartMessage).Msg(line)
	}
	return nil
}

func padRight(s string, n int) string {
	if width := termansi.StringWidth(s); width < n {
		return s + strings.Repeat(" ", n-width)
	}
	return s
}
