package cache

import (
	"context"
	"encoding/json"

	"github.com/matcra587/jira-cli/internal/jira"
)

// Resource describes one cacheable Jira metadata resource. It is the single
// source of truth for the resource's identity, default freshness window, and
// fetch. The Name is used three ways that must stay in lock-step: the
// `cache <name>` subcommand, the on-disk cache file name, and the
// `cache.<name>` operation label. EnvelopeKey is the data key the list is
// emitted under — usually Name, but linktypes emits "link_types".
type Resource struct {
	// Name identifies the resource — subcommand, cache file, and op label.
	Name string
	// Short is the cobra one-line command summary.
	Short string
	// EnvelopeKey is the envelope data key the resource list is emitted
	// under. Empty means "same as Name".
	EnvelopeKey string
	// TTLMinutes is the default --ttl-minutes for the resource's primer.
	TTLMinutes int
	// Fetch retrieves the resource from Jira and returns the JSON list to
	// cache. A nil Fetch marks a resource with a bespoke command (boards),
	// which the generic primer factory skips.
	Fetch func(ctx context.Context, client *jira.Client) (json.RawMessage, error)
}

// Key returns the envelope data key for the resource — EnvelopeKey when set,
// otherwise Name.
func (r Resource) Key() string {
	if r.EnvelopeKey != "" {
		return r.EnvelopeKey
	}
	return r.Name
}

// Registry is the canonical, ordered list of cacheable resources — the order
// the subcommands are registered on the `cache` group. `cache clear`, the
// primer factory, and (later) the refresh-all runner all derive from it, so a
// new resource is one entry here rather than scattered edits.
//
// The per-resource TTL ladder runs long because no consumer blocks on
// staleness and a re-fetch is one --refresh away, so the only cost of a long
// TTL is a brand-new value staying invisible to autocomplete until the next
// refresh. User-generated resources (labels, epics) stay short; admin/config
// resources (statuses, priorities, link types, …) run weeks-to-months. The
// schema-version check ([[cache.SchemaVersion]]) handles shape changes
// separately, so age never risks a mis-parse. boards has a nil Fetch: its
// bespoke command carries truncation + per-board project metadata the generic
// factory cannot express.
const (
	ttlHour = 60
	ttlDay  = 24 * ttlHour
)

var Registry = []Resource{
	{Name: "labels", Short: "Cache and print the global Jira label list", TTLMinutes: ttlHour, Fetch: fetchLabelsForCache},
	{Name: "projects", Short: "Cache and print the visible Jira project list", TTLMinutes: 7 * ttlDay, Fetch: fetchProjectsForCache},
	{Name: "epics", Short: "Cache and print the visible epic list", TTLMinutes: 4 * ttlHour, Fetch: fetchEpicsForCache},
	{Name: "fields", Short: "Cache and print the visible Jira field list", TTLMinutes: 14 * ttlDay, Fetch: fetchFieldsForCache},
	{Name: "issuetypes", Short: "Cache and print the visible Jira issue-type list", TTLMinutes: 30 * ttlDay, Fetch: fetchIssueTypesForCache},
	{Name: "linktypes", Short: "Cache and print the configured issue-link types", EnvelopeKey: "link_types", TTLMinutes: 90 * ttlDay, Fetch: fetchLinkTypesForCache},
	{Name: "boards", Short: "Cache and print the visible Jira agile boards", TTLMinutes: 28 * ttlDay, Fetch: nil},
	{Name: "statuses", Short: "Cache and print the visible Jira status list", TTLMinutes: 30 * ttlDay, Fetch: fetchStatusesForCache},
	{Name: "priorities", Short: "Cache and print the visible Jira priority list", TTLMinutes: 90 * ttlDay, Fetch: fetchPrioritiesForCache},
}

// ResourceNames returns the resource names in registry order.
func ResourceNames() []string {
	names := make([]string, len(Registry))
	for i, r := range Registry {
		names[i] = r.Name
	}
	return names
}

// TTLMinutesFor returns the default freshness window in minutes for a
// resource. An unknown name falls back to 60 minutes — the historical
// default — so a lookup miss degrades to today's behavior rather than a
// zero TTL.
func TTLMinutesFor(name string) int {
	for _, r := range Registry {
		if r.Name == name {
			return r.TTLMinutes
		}
	}
	return 60
}
