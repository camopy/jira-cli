package unit

// profiles.<name>.default_board is a freeform string profile-scoped
// config key. This test exercises the Get/Set/unset round-trip: set a
// value, read it back, set to empty (unset), confirm read returns
// empty. Mirrors how default_project is exercised in
// TestConfigGet_ProfileScoped / TestConfigSet_ProfileScopedFlipsBackend.

import (
	"slices"
	"testing"

	"github.com/matcra587/jira-cli/internal/config"
)

func TestConfigDefaultBoardRoundTrip(t *testing.T) {
	cfg := fixtureConfig()

	// Initial state: unset.
	if got, ok := cfg.Get("profiles.default.default_board"); !ok {
		t.Fatalf("Get(profiles.default.default_board) ok=false; want ok=true (empty default)")
	} else if got != "" {
		t.Fatalf("initial default_board = %q; want empty", got)
	}

	// Set a value.
	if err := cfg.Set("profiles.default.default_board", "Engineering Sprint"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, ok := cfg.Get("profiles.default.default_board")
	if !ok {
		t.Fatal("Get after Set ok=false")
	}
	if got != "Engineering Sprint" {
		t.Fatalf("Get after Set = %q; want %q", got, "Engineering Sprint")
	}

	// Unset by setting to empty string.
	if err := cfg.Set("profiles.default.default_board", ""); err != nil {
		t.Fatalf("Set empty: %v", err)
	}
	got, ok = cfg.Get("profiles.default.default_board")
	if !ok {
		t.Fatal("Get after unset ok=false")
	}
	if got != "" {
		t.Fatalf("Get after unset = %q; want empty", got)
	}

	// Other profile is untouched.
	if cfg.Profile("work").DefaultProject != "" {
		t.Fatal("work profile bled state from default profile")
	}
}

func TestConfigDefaultBoardListedInKeys(t *testing.T) {
	// default_board appears in config.Keys output for each profile,
	// just like default_project does. Mirrors TestKeys_ExpandsAcrossProfiles.
	cfg := fixtureConfig()
	keys := config.Keys(cfg)

	names := make([]string, len(keys))
	for i, k := range keys {
		names[i] = k.Name
	}
	for _, want := range []string{
		"profiles.default.default_board",
		"profiles.work.default_board",
	} {
		if !slices.Contains(names, want) {
			t.Errorf("Keys missing %q", want)
		}
	}
}
