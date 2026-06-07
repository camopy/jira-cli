package completion

import (
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/gechr/clib/complete"

	"github.com/matcra587/jira-cli/internal/cache"
	"github.com/matcra587/jira-cli/internal/cli"
	"github.com/matcra587/jira-cli/internal/cli/cmdutil"
	"github.com/matcra587/jira-cli/internal/cli/jql"
	"github.com/matcra587/jira-cli/internal/cli/startup"
	"github.com/matcra587/jira-cli/internal/config"
	"github.com/matcra587/jira-cli/internal/jira"
)

// predictorEmitter writes the completion candidates for one predictor kind.
type predictorEmitter func(globals startup.Globals, args []string)

// completionEmitters is the single source of truth for dynamic completion.
// Its keys are every predictor CompletionHandler implements; its values do the
// emitting. A flag's `Complete: "predictor=X"` or a command's
// `dynamic-args='X'` with no key here ships a silently-empty completion — the
// gap that left `--fields` dead. Because dispatch and the published key set are
// the same map, they cannot drift; the guard test TestDeclaredPredictorsAreHandled
// checks every predictor declared across the command tree against these keys.
var completionEmitters = map[string]predictorEmitter{
	"profile":       func(g startup.Globals, _ []string) { emitProfiles(g) },
	"configkey":     func(g startup.Globals, _ []string) { emitConfigKeys(g) },
	"configvalue":   func(_ startup.Globals, args []string) { emitConfigValues(args) },
	"alias":         func(g startup.Globals, _ []string) { emitAliases(g) },
	"cacheresource": func(_ startup.Globals, _ []string) { emitCacheResources() },
	"cachefield":    func(g startup.Globals, _ []string) { emitCachedFields(completionCacheKey(g)) },
	"cacheproject":  func(g startup.Globals, _ []string) { emitCachedProjects(completionCacheKey(g)) },
	"cacheepic":     func(g startup.Globals, _ []string) { emitCachedEpics(completionCacheKey(g)) },
	"cachelabel":    func(g startup.Globals, _ []string) { emitCachedLabels(completionCacheKey(g)) },
	"cacheissuetype": func(g startup.Globals, _ []string) {
		emitCachedNames(completionCacheKey(g), "issuetypes")
	},
	"cachelinktype": func(g startup.Globals, _ []string) { emitCachedLinkTypes(completionCacheKey(g)) },
	"cacheboard":    func(g startup.Globals, _ []string) { emitCachedBoards(completionCacheKey(g)) },
	"cachestatus":   func(g startup.Globals, _ []string) { emitCachedNames(completionCacheKey(g), "statuses") },
	"cachepriority": func(g startup.Globals, _ []string) { emitCachedNames(completionCacheKey(g), "priorities") },
	// issuekey has no cache yet: every command taking a KEY positionally carries
	// dynamic-args='issuekey' so the predictor is wired, but until an issue-key
	// cache lands it emits nothing and the shell falls back to free-form input.
	"issuekey": func(startup.Globals, []string) {},
}

// HandledPredictors is the sorted set of predictor names completionEmitters
// implements. Sourced from the map, so the dispatch and this published list can
// never disagree.
var HandledPredictors = sortedPredictorNames()

func sortedPredictorNames() []string {
	out := make([]string, 0, len(completionEmitters))
	for name := range completionEmitters {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// CompletionHandler dispatches dynamic completion requests routed through
// `--@complete=<kind>` (clib's predictor mechanism). The shell completion
// script invokes `jira --@complete=foo -- arg1 arg2` for positional args;
// this handler emits one candidate per line, optionally `value\tdesc`.
//
// Each predictor name corresponds either to a flag's
// `clib.FlagExtra{Complete: "predictor=foo"}` or to an entry in a command's
// `Annotations["clib"]` `dynamic-args='foo,bar'` list. Unknown kinds emit
// nothing, leaving the shell to fall back to free-form input.
func CompletionHandler(globals startup.Globals) complete.Handler {
	return func(_, kind string, args []string) {
		if emit, ok := completionEmitters[kind]; ok {
			emit(globals, args)
		}
	}
}

func emitProfiles(globals startup.Globals) {
	cfg, err := config.Load(config.WithPath(globals.ConfigPath))
	if err != nil {
		return
	}
	for _, p := range cfg.Profiles {
		_, _ = fmt.Fprintln(os.Stdout, p.Name)
	}
}

func emitConfigKeys(globals startup.Globals) {
	cfg, _ := config.Load(config.WithPath(globals.ConfigPath))
	for _, k := range config.Keys(cfg) {
		_, _ = fmt.Fprintf(os.Stdout, "%s\t%s\n", k.Name, k.Description)
	}
}

func emitConfigValues(args []string) {
	// args[0] is the key the user has typed in arg 0 of `config set`.
	if len(args) == 0 {
		return
	}
	for _, choice := range config.KeyChoices(args[0]) {
		_, _ = fmt.Fprintln(os.Stdout, choice)
	}
}

func emitAliases(globals startup.Globals) {
	cfg, err := config.Load(config.WithPath(globals.ConfigPath))
	if err != nil {
		return
	}
	for name := range cfg.Aliases {
		_, _ = fmt.Fprintln(os.Stdout, name)
	}
}

func emitCacheResources() {
	for _, r := range []string{"labels", "projects", "epics", "fields", "issuetypes", "linktypes", "boards", "statuses", "priorities"} {
		_, _ = fmt.Fprintln(os.Stdout, r)
	}
}

// completionCacheKey picks the cache namespace for cache-backed completion.
// Falls back to the default profile identity when the config is missing.
func completionCacheKey(globals startup.Globals) string {
	cfg, err := config.Load(config.WithPath(globals.ConfigPath))
	if globals.Profile != "" {
		if err != nil {
			return cmdutil.CacheKeyFromStartup(globals, nil, globals.Profile)
		}
		return cmdutil.CacheKeyFromStartup(globals, cfg, globals.Profile)
	}
	if err != nil || cfg.DefaultProfile == "" {
		return cmdutil.CacheKeyFromStartup(globals, nil, "default")
	}
	return cmdutil.CacheKeyFromStartup(globals, cfg, cfg.DefaultProfile)
}

// emitCachedFields emits one candidate per cached Jira field for the
// `--fields` predictor. The candidate is the field id (what Jira's search
// API expects, e.g. `summary` or `customfield_10010`); the display name
// rides along as the completion description. Null-safe: emits nothing when
// the cache is missing or malformed so completion never blocks the shell.
func emitCachedFields(profile string) {
	type field struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	var fields []field
	if !jql.ReadCacheJSON(profile, "fields", &fields) {
		return
	}
	for _, f := range fields {
		if f.ID == "" {
			continue
		}
		_, _ = fmt.Fprintf(os.Stdout, "%s\t%s\n",
			cli.SanitizeCompletionField(f.ID), cli.SanitizeCompletionField(f.Name))
	}
}

func emitCachedProjects(profile string) {
	var projects []jira.ProjectSummary
	if !jql.ReadCacheJSON(profile, "projects", &projects) {
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
	if !jql.ReadCacheJSON(profile, "epics", &epics) {
		return
	}
	for _, e := range epics {
		_, _ = fmt.Fprintf(os.Stdout, "%s\t%s\n",
			cli.SanitizeCompletionField(e.Key), cli.SanitizeCompletionField(e.Summary))
	}
}

func emitCachedLabels(profile string) {
	var labels []string
	if !jql.ReadCacheJSON(profile, "labels", &labels) {
		return
	}
	for _, l := range labels {
		_, _ = fmt.Fprintln(os.Stdout, cli.SanitizeCompletionField(l))
	}
}

// namedCacheValue is the shape the name-keyed metadata caches share. Statuses
// and priorities also store an id, but completion filters by name, so only the
// name is read here.
type namedCacheValue struct {
	Name string `json:"name"`
}

// uniqueCachedNames returns the cached names in first-seen order with blanks
// and duplicates dropped. Jira returns statuses and issue types *per workflow
// or project*, so the same display name recurs under different ids; a JQL name
// filter only needs each name once, and offering "To Do" three times is noise.
func uniqueCachedNames(values []namedCacheValue) []string {
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, v := range values {
		if v.Name == "" {
			continue
		}
		if _, dup := seen[v.Name]; dup {
			continue
		}
		seen[v.Name] = struct{}{}
		out = append(out, v.Name)
	}
	return out
}

// emitCachedNames emits one candidate per unique cached name for the
// issuetype, status and priority predictors. These all back JQL filters that
// match by name, so the id is irrelevant and is dropped; the flag's short
// Terse supplies the completion description. Null-safe: emits nothing when the
// cache is missing or malformed so completion never blocks the shell.
func emitCachedNames(profile, resource string) {
	var values []namedCacheValue
	if !jql.ReadCacheJSON(profile, resource, &values) {
		return
	}
	for _, name := range uniqueCachedNames(values) {
		_, _ = fmt.Fprintln(os.Stdout, cli.SanitizeCompletionField(name))
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
	if !jql.ReadCacheJSON(profile, "linktypes", &types) {
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
