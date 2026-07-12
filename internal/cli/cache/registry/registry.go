// Package registry is the single source of truth for the cacheable Jira
// metadata resources: their identity, default freshness window, and fetch.
// It is deliberately free of cobra command wiring so any consumer — the cache
// command group, `jira boards list`, `jira issue link types`, the refresh-all
// runner — can depend on the resource metadata without pulling in the command
// builders.
package registry

import (
	"context"
	"encoding/json"

	xslices "github.com/gechr/x/slices"
	"github.com/matcra587/jira-cli/internal/cli/cmdutil"
	"github.com/matcra587/jira-cli/internal/jira"
)

// Resource describes one cacheable Jira metadata resource. The Name is used
// three ways that must stay in lock-step: the `cache <name>` subcommand, the
// on-disk cache file name, and the `cache.<name>` operation label. EnvelopeKey
// is the data key the list is emitted under — usually Name, but linktypes
// emits "link_types".
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
// primer factory, and the refresh-all runner all derive from it, so a new
// resource is one entry here rather than scattered edits.
//
// The per-resource TTL ladder runs long because no consumer blocks on
// staleness and a re-fetch is one --refresh away, so the only cost of a long
// TTL is a brand-new value staying invisible to autocomplete until the next
// refresh. User-generated resources (labels, epics) stay short; admin/config
// resources (statuses, priorities, link types, …) run weeks-to-months. The
// schema-version check (cache.SchemaVersion) handles shape changes separately,
// so age never risks a mis-parse. boards has a nil Fetch: its bespoke command
// carries truncation + per-board project metadata the generic factory cannot
// express.
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
	{Name: "resolutions", Short: "Cache and print the configured Jira resolution list", TTLMinutes: 90 * ttlDay, Fetch: fetchResolutionsForCache},
}

// ResourceNames returns the resource names in registry order.
func ResourceNames() []string {
	return xslices.Map(Registry, func(r Resource) string { return r.Name })
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

// ByName returns the registry resource with the given name and whether it was
// found.
func ByName(name string) (Resource, bool) {
	for _, r := range Registry {
		if r.Name == name {
			return r, true
		}
	}
	return Resource{}, false
}

func fetchLabelsForCache(ctx context.Context, client *jira.Client) (json.RawMessage, error) {
	labels, _, err := cmdutil.ServicesForClient(client).Label().List(ctx, nil)
	if err != nil {
		return nil, err
	}
	return cmdutil.MarshalNonNilSlice(labels)
}

func fetchProjectsForCache(ctx context.Context, client *jira.Client) (json.RawMessage, error) {
	projects, _, err := cmdutil.ServicesForClient(client).Project(0).List(ctx, nil)
	if err != nil {
		return nil, err
	}
	return cmdutil.MarshalNonNilSlice(projects)
}

// cacheEpic is the agent-friendly subset stored on disk. Distinct from
// jira.Epic to avoid the shape-fragility risk of caching internal types.
type cacheEpic struct {
	Key     string `json:"key"`
	Summary string `json:"summary"`
	Status  string `json:"status"`
}

func fetchEpicsForCache(ctx context.Context, client *jira.Client) (json.RawMessage, error) {
	issues, _, err := cmdutil.ServicesForClient(client).Epic().List(ctx, &jira.ListOptions{MaxResults: 200})
	if err != nil {
		return nil, err
	}
	out := make([]cacheEpic, 0, len(issues))
	for _, iss := range issues {
		e := cacheEpic{}
		if iss != nil && iss.Key != nil {
			e.Key = *iss.Key
		}
		if iss != nil && iss.Fields != nil {
			if iss.Fields.Summary != nil {
				e.Summary = *iss.Fields.Summary
			}
			if iss.Fields.Status != nil && iss.Fields.Status.Name != nil {
				e.Status = *iss.Fields.Status.Name
			}
		}
		out = append(out, e)
	}
	return cmdutil.MarshalNonNilSlice(out)
}

// cacheField is the agent-friendly subset of a Jira field. ID is the
// stable customfield_xxxxx identifier; name is the display label.
type cacheField struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type,omitempty"`
}

func fetchFieldsForCache(ctx context.Context, client *jira.Client) (json.RawMessage, error) {
	req, err := client.NewRequest(ctx, "GET", "rest/api/3/field", nil)
	if err != nil {
		return nil, err
	}
	var raw []struct {
		ID     string `json:"id"`
		Name   string `json:"name"`
		Schema struct {
			Type string `json:"type"`
		} `json:"schema"`
	}
	if _, err := client.Do(req, &raw); err != nil {
		return nil, err
	}
	out := make([]cacheField, 0, len(raw))
	for _, f := range raw {
		out = append(out, cacheField{ID: f.ID, Name: f.Name, Type: f.Schema.Type})
	}
	return cmdutil.MarshalNonNilSlice(out)
}

// cacheIssueType is the cached subset of a project's issue-type schema.
type cacheIssueType struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Subtask bool   `json:"subtask"`
}

func fetchIssueTypesForCache(ctx context.Context, client *jira.Client) (json.RawMessage, error) {
	req, err := client.NewRequest(ctx, "GET", "rest/api/3/issuetype", nil)
	if err != nil {
		return nil, err
	}
	var raw []struct {
		ID      string `json:"id"`
		Name    string `json:"name"`
		Subtask bool   `json:"subtask"`
	}
	if _, err := client.Do(req, &raw); err != nil {
		return nil, err
	}
	out := make([]cacheIssueType, 0, len(raw))
	for _, t := range raw {
		out = append(out, cacheIssueType{ID: t.ID, Name: t.Name, Subtask: t.Subtask})
	}
	return cmdutil.MarshalNonNilSlice(out)
}

func fetchLinkTypesForCache(ctx context.Context, client *jira.Client) (json.RawMessage, error) {
	types, _, err := cmdutil.ServicesForClient(client).IssueLinkType().List(ctx)
	if err != nil {
		return nil, err
	}
	return cmdutil.MarshalNonNilSlice(types)
}

// cacheNamedValue is the cached subset of a flat named Jira metadata value
// (priority, resolution): the name completion offers and the id for the
// shell tooltip.
type cacheNamedValue struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// cacheStatus is the cached subset of a workflow status. Status ids are NOT
// unique per display name: Jira defines statuses per workflow, so the same
// name ("To Do") recurs under different ids. StatusCategory carries the
// workflow category key (new, indeterminate, done) so a name or id can be
// mapped to its category without a live call.
type cacheStatus struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	StatusCategory string `json:"status_category,omitempty"`
}

func fetchStatusesForCache(ctx context.Context, client *jira.Client) (json.RawMessage, error) {
	req, err := client.NewRequest(ctx, "GET", "rest/api/3/status", nil)
	if err != nil {
		return nil, err
	}
	var raw []struct {
		ID             string `json:"id"`
		Name           string `json:"name"`
		StatusCategory struct {
			Key string `json:"key"`
		} `json:"statusCategory"`
	}
	if _, err := client.Do(req, &raw); err != nil {
		return nil, err
	}
	out := make([]cacheStatus, 0, len(raw))
	for _, v := range raw {
		if v.Name == "" {
			continue
		}
		out = append(out, cacheStatus{ID: v.ID, Name: v.Name, StatusCategory: v.StatusCategory.Key})
	}
	return cmdutil.MarshalNonNilSlice(out)
}

func fetchPrioritiesForCache(ctx context.Context, client *jira.Client) (json.RawMessage, error) {
	return fetchNamedValuesForCache(ctx, client, "rest/api/3/priority")
}

func fetchResolutionsForCache(ctx context.Context, client *jira.Client) (json.RawMessage, error) {
	return fetchNamedValuesForCache(ctx, client, "rest/api/3/resolution")
}

// fetchNamedValuesForCache fills the cache for a flat {id,name} Jira metadata
// list (priorities, resolutions). Both endpoints return an unpaginated array,
// so one GET fills the cache that completion and resolve transitions read.
func fetchNamedValuesForCache(ctx context.Context, client *jira.Client, path string) (json.RawMessage, error) {
	req, err := client.NewRequest(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}
	var raw []cacheNamedValue
	if _, err := client.Do(req, &raw); err != nil {
		return nil, err
	}
	out := make([]cacheNamedValue, 0, len(raw))
	for _, v := range raw {
		if v.Name == "" {
			continue
		}
		out = append(out, cacheNamedValue{ID: v.ID, Name: v.Name})
	}
	return cmdutil.MarshalNonNilSlice(out)
}
