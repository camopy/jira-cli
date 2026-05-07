package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/gechr/clib/complete"

	"github.com/matcra587/jira-cli/internal/cache"
	"github.com/matcra587/jira-cli/internal/config"
	"github.com/matcra587/jira-cli/pkg/jira"
)

// completionHandler dispatches dynamic completion requests routed through
// `--@complete=<kind>` (clib's predictor mechanism). The shell completion
// script invokes `jira --@complete=foo -- arg1 arg2` for positional args;
// this handler emits one candidate per line, optionally `value\tdesc`.
//
// Each predictor name corresponds either to a flag's
// `clib.FlagExtra{Complete: "predictor=foo"}` or to an entry in a command's
// `Annotations["clib"]` `dynamic-args='foo,bar'` list.
func completionHandler() complete.Handler {
	return func(shell, kind string, args []string) {
		_ = shell
		switch kind {
		case "profile":
			cfg, err := config.Load()
			if err != nil {
				return
			}
			for _, p := range cfg.Profiles {
				_, _ = fmt.Fprintln(os.Stdout, p.Name)
			}
		case "configkey":
			cfg, _ := config.Load()
			for _, k := range config.Keys(cfg) {
				_, _ = fmt.Fprintf(os.Stdout, "%s\t%s\n", k.Name, k.Description)
			}
		case "configvalue":
			// args[0] is the key the user has typed in arg 0 of `config set`.
			if len(args) == 0 {
				return
			}
			for _, choice := range config.KeyChoices(args[0]) {
				_, _ = fmt.Fprintln(os.Stdout, choice)
			}
		case "alias":
			cfg, err := config.Load()
			if err != nil {
				return
			}
			for name := range cfg.Aliases {
				_, _ = fmt.Fprintln(os.Stdout, name)
			}
		case "cacheresource":
			for _, r := range []string{"labels", "projects", "epics", "fields", "issuetypes", "linktypes", "boards"} {
				_, _ = fmt.Fprintln(os.Stdout, r)
			}
		case "cacheproject":
			emitCachedProjects()
		case "cacheepic":
			emitCachedEpics()
		case "cachelabel":
			emitCachedLabels()
		case "cacheissuetype":
			emitCachedIssueTypes()
		case "cachelinktype":
			emitCachedLinkTypes()
		case "cacheboard":
			emitCachedBoards()
		case "issuekey":
			// Every command taking an issue key positionally carries the
			// dynamic-args='issuekey' annotation so a future issue-key
			// cache layer can plug in here without further command-level
			// changes. Until the cache lands, the predictor returns empty
			// so the shell falls back to free-form input.
			return
		default:
			return
		}
	}
}

// completionProfile picks a profile name for cache-backed completion.
// Falls back to "default" when the config is missing.
func completionProfile() string {
	cfg, err := config.Load()
	if err != nil || cfg.DefaultProfile == "" {
		return "default"
	}
	return cfg.DefaultProfile
}

func emitCachedProjects() {
	var projects []jira.ProjectSummary
	if !readCacheJSON(completionProfile(), "projects", &projects) {
		return
	}
	for _, p := range projects {
		_, _ = fmt.Fprintf(os.Stdout, "%s\t%s\n", p.Key, p.Name)
	}
}

func emitCachedEpics() {
	type epic struct {
		Key     string `json:"key"`
		Summary string `json:"summary"`
	}
	var epics []epic
	if !readCacheJSON(completionProfile(), "epics", &epics) {
		return
	}
	for _, e := range epics {
		_, _ = fmt.Fprintf(os.Stdout, "%s\t%s\n", e.Key, e.Summary)
	}
}

func emitCachedLabels() {
	var labels []string
	if !readCacheJSON(completionProfile(), "labels", &labels) {
		return
	}
	for _, l := range labels {
		_, _ = fmt.Fprintln(os.Stdout, l)
	}
}

func emitCachedIssueTypes() {
	type issuetype struct {
		Name string `json:"name"`
	}
	var types []issuetype
	if !readCacheJSON(completionProfile(), "issuetypes", &types) {
		return
	}
	seen := make(map[string]struct{}, len(types))
	for _, t := range types {
		if _, dup := seen[t.Name]; dup || t.Name == "" {
			continue
		}
		seen[t.Name] = struct{}{}
		_, _ = fmt.Fprintln(os.Stdout, t.Name)
	}
}

// emitCachedBoards emits one candidate per cached board for the
// `--board NAME` predictor. Output format:
//
//	<id>\t<name> (<type>, <project[s]>)
//
// where the projects segment comma-joins up to two project keys and
// collapses to `+N` overflow for 3+ keys; an empty projects list drops
// the segment entirely (just `(<type>)`). Null-safe: emits nothing
// when the cache is missing or malformed so completion never blocks
// the shell.
func emitCachedBoards() {
	entry, ok, _, err := cache.Read(completionProfile(), "boards", 24*time.Hour*365)
	if err != nil || !ok {
		return
	}
	boards, err := jira.DecodeBoardsCache(entry.Data)
	if err != nil || len(boards) == 0 {
		return
	}
	for _, b := range boards {
		if b.ID == nil || b.Name == nil {
			continue
		}
		typ := ""
		if b.Type != nil {
			typ = *b.Type
		}
		descriptor := boardCompletionDescriptor(typ, b.ProjectKeys)
		// Replace newlines / tabs in the name with single spaces so
		// the descriptor stays single-line .
		safeName := strings.NewReplacer("\n", " ", "\r", " ", "\t", " ").Replace(*b.Name)
		_, _ = fmt.Fprintf(os.Stdout, "%d\t%s%s\n", *b.ID, safeName, descriptor)
	}
}

// boardCompletionDescriptor renders the parenthesized "(type, projects)"
// segment : <=2 keys joined with comma, 3+ keys → "K1, K2 +N",
// empty list → "(type)" (segment-less variant).
func boardCompletionDescriptor(typ string, keys []string) string {
	if typ == "" && len(keys) == 0 {
		return ""
	}
	if len(keys) == 0 {
		return " (" + typ + ")"
	}
	switch len(keys) {
	case 1:
		return " (" + typ + ", " + keys[0] + ")"
	case 2:
		return " (" + typ + ", " + keys[0] + ", " + keys[1] + ")"
	default:
		// 3+ keys → first two + overflow count.
		return fmt.Sprintf(" (%s, %s, %s +%d)", typ, keys[0], keys[1], len(keys)-2)
	}
}

// emitCachedLinkTypes emits one candidate per issue-link type cached
// under the active profile's `linktypes` resource. Output format:
// `<id>\t<name> (<inward> / <outward>)` so the shell tooltip carries
// the relationship phrasing alongside the canonical name.
// Null-safe: silently emits nothing when the cache is absent or
// malformed so completion never blocks the shell.
func emitCachedLinkTypes() {
	type linkType struct {
		ID      string `json:"id"`
		Name    string `json:"name"`
		Inward  string `json:"inward"`
		Outward string `json:"outward"`
	}
	var types []linkType
	if !readCacheJSON(completionProfile(), "linktypes", &types) {
		return
	}
	for _, t := range types {
		if t.Name == "" {
			continue
		}
		_, _ = fmt.Fprintf(os.Stdout, "%s\t%s (%s / %s)\n", t.ID, t.Name, t.Inward, t.Outward)
	}
}
