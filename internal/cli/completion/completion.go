package completion

import (
	"fmt"
	"io"
	"maps"
	"slices"

	"github.com/gechr/clib/complete"
	xmaps "github.com/gechr/x/maps"
	xslices "github.com/gechr/x/slices"

	"github.com/matcra587/jira-cli/internal/cache"
	"github.com/matcra587/jira-cli/internal/cli"
	"github.com/matcra587/jira-cli/internal/cli/cache/registry"
	"github.com/matcra587/jira-cli/internal/cli/cmdutil"
	"github.com/matcra587/jira-cli/internal/cli/jql"
	"github.com/matcra587/jira-cli/internal/cli/startup"
	"github.com/matcra587/jira-cli/internal/config"
	"github.com/matcra587/jira-cli/internal/jira"
)

// predictorEmitter writes the completion candidates for one predictor kind.
type predictorEmitter func(w io.Writer, globals startup.Globals, args []string) error

// completionEmitters is the single source of truth for dynamic completion.
// Its keys are every predictor CompletionHandler implements; its values do the
// emitting. A flag's `Complete: "predictor=X"` or a command's
// `dynamic-args='X'` with no key here ships a silently-empty completion — the
// gap that left `--fields` dead. Because dispatch and the published key set are
// the same map, they cannot drift; the guard test TestDeclaredPredictorsAreHandled
// checks every predictor declared across the command tree against these keys.
var completionEmitters = map[string]predictorEmitter{
	"profile":       func(w io.Writer, g startup.Globals, _ []string) error { return emitProfiles(w, g) },
	"configkey":     func(w io.Writer, g startup.Globals, _ []string) error { return emitConfigKeys(w, g) },
	"configvalue":   func(w io.Writer, _ startup.Globals, args []string) error { return emitConfigValues(w, args) },
	"alias":         func(w io.Writer, g startup.Globals, _ []string) error { return emitAliases(w, g) },
	"savedquery":    func(w io.Writer, g startup.Globals, _ []string) error { return emitSavedQueries(w, g) },
	"cacheresource": func(w io.Writer, _ startup.Globals, _ []string) error { return emitCacheResources(w) },
	"cachefield": func(w io.Writer, g startup.Globals, _ []string) error {
		return emitCachedFields(w, completionCacheKey(g))
	},
	"cacheproject": func(w io.Writer, g startup.Globals, _ []string) error {
		return emitCachedProjects(w, completionCacheKey(g))
	},
	"cacheepic": func(w io.Writer, g startup.Globals, _ []string) error {
		return emitCachedEpics(w, completionCacheKey(g))
	},
	"cachelabel": func(w io.Writer, g startup.Globals, _ []string) error {
		return emitCachedLabels(w, completionCacheKey(g))
	},
	"cacheissuetype": func(w io.Writer, g startup.Globals, _ []string) error {
		return emitCachedNames(w, completionCacheKey(g), "issuetypes")
	},
	"cachelinktype": func(w io.Writer, g startup.Globals, _ []string) error {
		return emitCachedLinkTypes(w, completionCacheKey(g))
	},
	"cacheboard": func(w io.Writer, g startup.Globals, _ []string) error {
		return emitCachedBoards(w, completionCacheKey(g))
	},
	"cachestatus": func(w io.Writer, g startup.Globals, _ []string) error {
		return emitCachedNames(w, completionCacheKey(g), "statuses")
	},
	"cachepriority": func(w io.Writer, g startup.Globals, _ []string) error {
		return emitCachedNames(w, completionCacheKey(g), "priorities")
	},
	// issuekey completes from the per-profile recently-used key cache,
	// written as a side effect of commands that touch keys. A profile with
	// no recorded keys emits nothing and the shell falls back to free-form
	// input.
	"issuekey": func(w io.Writer, g startup.Globals, _ []string) error {
		return emitCachedIssueKeys(w, completionCacheKey(g))
	},
}

// HandledPredictors is the sorted set of predictor names completionEmitters
// implements. Sourced from the map, so the dispatch and this published list can
// never disagree.
var HandledPredictors = sortedPredictorNames()

func sortedPredictorNames() []string {
	return slices.Sorted(maps.Keys(completionEmitters))
}

// Handler dispatches dynamic completion requests routed through
// `--@complete=<kind>` (clib's predictor mechanism). It retains the first
// candidate write failure because Clib's callback does not return an error.
type Handler struct {
	w       io.Writer
	globals startup.Globals
	err     error
}

// NewHandler returns a dynamic completion handler that writes candidates to w.
func NewHandler(w io.Writer, globals startup.Globals) *Handler {
	return &Handler{w: w, globals: globals}
}

// Complete implements Clib's shell-first completion callback. The shell
// completion script invokes `jira --@complete=foo -- arg1 arg2` for positional
// args; each emitter writes one candidate per line, optionally `value\tdesc`.
//
// Each predictor name corresponds either to a flag's
// `clib.FlagExtra{Complete: "predictor=foo"}` or to an entry in a command's
// `Annotations["clib"]` `dynamic-args='foo,bar'` list. Unknown kinds emit
// nothing, leaving the shell to fall back to free-form input.
func (h *Handler) Complete(_, kind string, args []string) {
	if h.err != nil {
		return
	}
	if emit, ok := completionEmitters[kind]; ok {
		if err := emit(h.w, h.globals, args); err != nil {
			h.err = cli.NewOutputError(err)
		}
	}
}

// Err returns the first dynamic-candidate write failure.
func (h *Handler) Err() error {
	return h.err
}

var _ complete.Handler = (*Handler)(nil).Complete

func emitProfiles(w io.Writer, globals startup.Globals) error {
	cfg, err := config.Load(config.WithPath(globals.ConfigPath))
	if err != nil {
		return nil
	}
	for _, p := range cfg.Profiles {
		if _, err := fmt.Fprintln(w, p.Name); err != nil {
			return err
		}
	}
	return nil
}

func emitConfigKeys(w io.Writer, globals startup.Globals) error {
	cfg, _ := config.Load(config.WithPath(globals.ConfigPath))
	for _, k := range config.Keys(cfg) {
		if _, err := fmt.Fprintf(w, "%s\t%s\n", k.Name, k.Description); err != nil {
			return err
		}
	}
	return nil
}

func emitConfigValues(w io.Writer, args []string) error {
	// args[0] is the key the user has typed in arg 0 of `config set`.
	if len(args) == 0 {
		return nil
	}
	for _, choice := range config.KeyChoices(args[0]) {
		if _, err := fmt.Fprintln(w, choice); err != nil {
			return err
		}
	}
	return nil
}

func emitAliases(w io.Writer, globals startup.Globals) error {
	cfg, err := config.Load(config.WithPath(globals.ConfigPath))
	if err != nil {
		return nil
	}
	for name := range cfg.Aliases {
		if _, err := fmt.Fprintln(w, name); err != nil {
			return err
		}
	}
	return nil
}

// emitSavedQueries emits one candidate per saved JQL query name for the
// `search saved NAME` positional. The name is the value; its description
// (falling back to the JQL when there is no description) rides along as the
// completion hint. Names are sorted for a stable order and sanitized, and the
// emitter is null-safe so a missing queries directory never blocks the shell.
func emitSavedQueries(w io.Writer, globals startup.Globals) error {
	cfg, err := config.Load(config.WithPath(globals.ConfigPath))
	if err != nil {
		return nil
	}
	queries, err := config.LoadQueries(cfg.QueriesPath)
	if err != nil {
		return nil
	}
	for name, q := range xmaps.Sorted(queries) {
		desc := q.Description
		if desc == "" {
			desc = q.JQL
		}
		if _, err := fmt.Fprintf(w, "%s\t%s\n",
			cli.SanitizeCompletionField(name), cli.SanitizeCompletionField(desc)); err != nil {
			return err
		}
	}
	return nil
}

func emitCacheResources(w io.Writer) error {
	for _, r := range registry.ResourceNames() {
		if _, err := fmt.Fprintln(w, r); err != nil {
			return err
		}
	}
	return nil
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
func emitCachedFields(w io.Writer, profile string) error {
	type field struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	var fields []field
	if !jql.ReadCacheJSON(profile, "fields", &fields) {
		return nil
	}
	for _, f := range fields {
		if f.ID == "" {
			continue
		}
		if _, err := fmt.Fprintf(w, "%s\t%s\n",
			cli.SanitizeCompletionField(f.ID), cli.SanitizeCompletionField(f.Name)); err != nil {
			return err
		}
	}
	return nil
}

func emitCachedProjects(w io.Writer, profile string) error {
	var projects []jira.ProjectSummary
	if !jql.ReadCacheJSON(profile, "projects", &projects) {
		return nil
	}
	for _, p := range projects {
		if _, err := fmt.Fprintf(w, "%s\t%s\n",
			cli.SanitizeCompletionField(p.Key), cli.SanitizeCompletionField(p.Name)); err != nil {
			return err
		}
	}
	return nil
}

func emitCachedEpics(w io.Writer, profile string) error {
	type epic struct {
		Key     string `json:"key"`
		Summary string `json:"summary"`
	}
	var epics []epic
	if !jql.ReadCacheJSON(profile, "epics", &epics) {
		return nil
	}
	for _, e := range epics {
		if _, err := fmt.Fprintf(w, "%s\t%s\n",
			cli.SanitizeCompletionField(e.Key), cli.SanitizeCompletionField(e.Summary)); err != nil {
			return err
		}
	}
	return nil
}

func emitCachedLabels(w io.Writer, profile string) error {
	var labels []string
	if !jql.ReadCacheJSON(profile, "labels", &labels) {
		return nil
	}
	for _, l := range labels {
		if _, err := fmt.Fprintln(w, cli.SanitizeCompletionField(l)); err != nil {
			return err
		}
	}
	return nil
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
	for _, v := range values {
		if v.Name == "" {
			continue
		}
		out = append(out, v.Name)
	}
	return xslices.Unique(out)
}

// emitCachedNames emits one candidate per unique cached name for the
// issuetype, status and priority predictors. These all back JQL filters that
// match by name, so the id is irrelevant and is dropped; the flag's short
// Terse supplies the completion description. Null-safe: emits nothing when the
// cache is missing or malformed so completion never blocks the shell.
func emitCachedNames(w io.Writer, profile, resource string) error {
	var values []namedCacheValue
	if !jql.ReadCacheJSON(profile, resource, &values) {
		return nil
	}
	for _, name := range uniqueCachedNames(values) {
		if _, err := fmt.Fprintln(w, cli.SanitizeCompletionField(name)); err != nil {
			return err
		}
	}
	return nil
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
func emitCachedBoards(w io.Writer, profile string) error {
	entry, ok, err := cache.ReadCachedOrEmpty(profile, "boards")
	if err != nil || !ok {
		return nil
	}
	boards, err := jira.DecodeBoardsCache(entry.Data)
	if err != nil || len(boards) == 0 {
		return nil
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
		if _, err := fmt.Fprintf(w, "%s\t%d%s\n", safeName, *b.ID, descriptor); err != nil {
			return err
		}
	}
	return nil
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
func emitCachedLinkTypes(w io.Writer, profile string) error {
	type linkType struct {
		ID      string `json:"id"`
		Name    string `json:"name"`
		Inward  string `json:"inward"`
		Outward string `json:"outward"`
	}
	var types []linkType
	if !jql.ReadCacheJSON(profile, "linktypes", &types) {
		return nil
	}
	for _, t := range types {
		if t.Name == "" {
			continue
		}
		if _, err := fmt.Fprintf(w, "%s\t%s (%s / %s)\n",
			cli.SanitizeCompletionField(t.Name), cli.SanitizeCompletionField(t.ID),
			cli.SanitizeCompletionField(t.Inward), cli.SanitizeCompletionField(t.Outward)); err != nil {
			return err
		}
	}
	return nil
}

// emitCachedIssueKeys prints the profile's recently used issue keys, newest
// first. Null-safe like every emitter: a missing or broken cache emits
// nothing, and candidates cross the completion sanitizer even though keys
// are written normalized — cache files are still local input.
func emitCachedIssueKeys(w io.Writer, profile string) error {
	for _, key := range cache.IssueKeys(profile) {
		if _, err := fmt.Fprintln(w, cli.SanitizeCompletionField(key)); err != nil {
			return err
		}
	}
	return nil
}
