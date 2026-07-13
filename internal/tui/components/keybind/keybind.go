package keybind

import (
	"fmt"
	"maps"
	"slices"

	"charm.land/bubbles/v2/key"
)

// Registry maps stable lower-case names to the owner's binding fields.
// Values are pointers so Rebind mutates the owner's key map in place; build
// the registry from the same struct the app reads bindings off.
type Registry map[string]*key.Binding

// Names returns every rebindable binding name, sorted. Useful for docs and
// for validating a config file's keybinding overrides.
func (r Registry) Names() []string {
	return slices.Sorted(maps.Keys(r))
}

// Rebind applies user overrides keyed by binding name (e.g. {"transition":
// {"x"}}). An empty key slice is ignored so a partial config never silently
// unbinds an action. The first key becomes the help label; the description is
// kept. An unknown name is an error rather than a silent no-op, so a typo in
// a config file is surfaced — and it is checked before anything mutates, so a
// failed Rebind leaves every binding exactly as it was.
func (r Registry) Rebind(overrides map[string][]string) error {
	for name := range overrides {
		if _, ok := r[name]; !ok {
			return fmt.Errorf("keybind: unknown binding %q", name)
		}
	}
	for name, ks := range overrides {
		if len(ks) == 0 {
			continue
		}
		b := r[name]
		desc := b.Help().Desc
		b.SetKeys(ks...)
		b.SetHelp(ks[0], desc)
	}
	return nil
}
