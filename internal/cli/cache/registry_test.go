package cache

import (
	"strconv"
	"strings"
	"testing"

	"github.com/matcra587/jira-cli/internal/cli/cache/registry"
)

// The registry is the single source of truth for cacheable resources. These
// guards fail CI if a primer subcommand is added or removed without the
// matching registry entry, or if a command's --ttl-minutes default drifts
// from the registry — the drift that previously let the resource list live
// in three separate places.

func TestRegistryCoversEveryPrimerSubcommand(t *testing.T) {
	subs := map[string]bool{}
	for _, c := range NewCommand().Commands() {
		name := strings.Fields(c.Use)[0]
		// clear and refresh are housekeeping; issuekeys is a local
		// write-through list, never primed from Jira, so it deliberately
		// lives outside the registry.
		if name == "clear" || name == "refresh" || name == "issuekeys" {
			continue
		}
		subs[name] = true
	}
	for _, r := range registry.Registry {
		if !subs[r.Name] {
			t.Errorf("registry resource %q has no `cache %s` subcommand", r.Name, r.Name)
		}
		delete(subs, r.Name)
	}
	for name := range subs {
		t.Errorf("`cache %s` subcommand has no registry entry", name)
	}
}

func TestRegistryTTLMatchesFlagDefaults(t *testing.T) {
	for _, c := range NewCommand().Commands() {
		name := strings.Fields(c.Use)[0]
		// issuekeys has no freshness window: the newest entry is current by
		// definition, so no --ttl-minutes.
		if name == "clear" || name == "refresh" || name == "issuekeys" {
			continue
		}
		f := c.Flags().Lookup("ttl-minutes")
		if f == nil {
			t.Errorf("`cache %s` has no --ttl-minutes flag", name)
			continue
		}
		if want := strconv.Itoa(registry.TTLMinutesFor(name)); f.DefValue != want {
			t.Errorf("`cache %s` --ttl-minutes default %s != registry %s", name, f.DefValue, want)
		}
	}
}
