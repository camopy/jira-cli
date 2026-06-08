package cache

// Resource describes one cacheable Jira metadata resource. It is the single
// source of truth for the resource's identity and default freshness window.
// The Name is used four ways that must stay in lock-step: the `cache <name>`
// subcommand, the on-disk cache file name, the `cache.<name>` operation
// label, and the envelope data key. As later steps land, the fetch function
// and schema version move here too; today the per-resource command bodies
// still inline their own fetch and envelope assembly.
type Resource struct {
	// Name identifies the resource everywhere — subcommand, cache file,
	// op label, and envelope key.
	Name string
	// TTLMinutes is the default --ttl-minutes for the resource's primer.
	TTLMinutes int
}

// Registry is the canonical, ordered list of cacheable resources — the order
// the primer subcommands are registered on the `cache` group. `cache clear`,
// the primers, and (later) the refresh-all runner all derive from it, so a
// new resource is one entry here rather than three scattered edits.
//
// Every TTL is 60 minutes today, matching the historical per-command
// default; the per-resource TTL ladder is a separate, deliberate change.
var Registry = []Resource{
	{Name: "labels", TTLMinutes: 60},
	{Name: "projects", TTLMinutes: 60},
	{Name: "epics", TTLMinutes: 60},
	{Name: "fields", TTLMinutes: 60},
	{Name: "issuetypes", TTLMinutes: 60},
	{Name: "linktypes", TTLMinutes: 60},
	{Name: "boards", TTLMinutes: 60},
	{Name: "statuses", TTLMinutes: 60},
	{Name: "priorities", TTLMinutes: 60},
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
