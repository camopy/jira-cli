package main

import (
	"fmt"
	"os"
	"time"

	"github.com/gechr/clib/complete"

	"github.com/matcra587/jira-cli/internal/cache"
	"github.com/matcra587/jira-cli/internal/cli"
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
func completionHandler(startup startupGlobals) complete.Handler {
	return func(shell, kind string, args []string) {
		_ = shell
		switch kind {
		case "profile":
			cfg, err := config.Load(config.WithPath(startup.ConfigPath))
			if err != nil {
				return
			}
			for _, p := range cfg.Profiles {
				_, _ = fmt.Fprintln(os.Stdout, p.Name)
			}
		case "configkey":
			cfg, _ := config.Load(config.WithPath(startup.ConfigPath))
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
			cfg, err := config.Load(config.WithPath(startup.ConfigPath))
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
			emitCachedProjects(completionProfile(startup))
		case "cacheepic":
			emitCachedEpics(completionProfile(startup))
		case "cachelabel":
			emitCachedLabels(completionProfile(startup))
		case "cacheissuetype":
			emitCachedIssueTypes(completionProfile(startup))
		case "cachelinktype":
			emitCachedLinkTypes(completionProfile(startup))
		case "cacheboard":
			emitCachedBoards(completionProfile(startup))
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
func completionProfile(startup startupGlobals) string {
	if startup.Profile != "" {
		return startup.Profile
	}
	cfg, err := config.Load(config.WithPath(startup.ConfigPath))
	if err != nil || cfg.DefaultProfile == "" {
		return "default"
	}
	return cfg.DefaultProfile
}

func emitCachedProjects(profile string) {
	var projects []jira.ProjectSummary
	if !readCacheJSON(profile, "projects", &projects) {
		return
	}
	for _, p := range projects {
		_, _ = fmt.Fprintf(os.Stdout, "%s\t%s\n",
			cli.SanitizeCompletionField(p.Key), cli.SanitizeCompletionField(p.Name))
	}
}

func emitCachedEpics(profile string) {
	type epic struct {
		Key     string `json:"key"`
		Summary string `json:"summary"`
	}
	var epics []epic
	if !readCacheJSON(profile, "epics", &epics) {
		return
	}
	for _, e := range epics {
		_, _ = fmt.Fprintf(os.Stdout, "%s\t%s\n",
			cli.SanitizeCompletionField(e.Key), cli.SanitizeCompletionField(e.Summary))
	}
}

func emitCachedLabels(profile string) {
	var labels []string
	if !readCacheJSON(profile, "labels", &labels) {
		return
	}
	for _, l := range labels {
		_, _ = fmt.Fprintln(os.Stdout, cli.SanitizeCompletionField(l))
	}
}

func emitCachedIssueTypes(profile string) {
	type issuetype struct {
		Name string `json:"name"`
	}
	var types []issuetype
	if !readCacheJSON(profile, "issuetypes", &types) {
		return
	}
	seen := make(map[string]struct{}, len(types))
	for _, t := range types {
		if _, dup := seen[t.Name]; dup || t.Name == "" {
			continue
		}
		seen[t.Name] = struct{}{}
		_, _ = fmt.Fprintln(os.Stdout, cli.SanitizeCompletionField(t.Name))
	}
}

// emitCachedBoards emits one candidate per cached board for the
// `--board NAME` predictor. Output format:
//
//	<name>\t<id> (<type>, <project[s]>)
//
// where the projects segment comma-joins up to two project keys and
// collapses to `+N` overflow for 3+ keys; an empty projects list drops
// the segment entirely (just `(<type>)`). Null-safe: emits nothing
// when the cache is missing or malformed so completion never blocks
// the shell.
func emitCachedBoards(profile string) {
	entry, ok, _, err := cache.Read(profile, "boards", 24*time.Hour*365)
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
		// Sanitize the name so embedded tabs, newlines and control
		// bytes cannot corrupt the one-candidate-per-line grammar.
		safeName := cli.SanitizeCompletionField(*b.Name)
		_, _ = fmt.Fprintf(os.Stdout, "%s\t%d%s\n", safeName, *b.ID, descriptor)
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
// `<name>\t<id> (<inward> / <outward>)` so the shell tooltip carries
// the relationship phrasing alongside the canonical name.
// Null-safe: silently emits nothing when the cache is absent or
// malformed so completion never blocks the shell.
func emitCachedLinkTypes(profile string) {
	type linkType struct {
		ID      string `json:"id"`
		Name    string `json:"name"`
		Inward  string `json:"inward"`
		Outward string `json:"outward"`
	}
	var types []linkType
	if !readCacheJSON(profile, "linktypes", &types) {
		return
	}
	for _, t := range types {
		if t.Name == "" {
			continue
		}
		_, _ = fmt.Fprintf(os.Stdout, "%s\t%s (%s / %s)\n",
			cli.SanitizeCompletionField(t.Name), cli.SanitizeCompletionField(t.ID),
			cli.SanitizeCompletionField(t.Inward), cli.SanitizeCompletionField(t.Outward))
	}
}
