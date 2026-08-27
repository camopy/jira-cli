package tui

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/spf13/cobra"

	"github.com/matcra587/jira-cli/internal/config"
	"github.com/matcra587/jira-cli/internal/tui/core"
	"github.com/matcra587/jira-cli/internal/tui/sections/issues"
)

// newApp must land on the issues triage view and expose the search view as the
// next section, so `jira tui` opens on the working queue with JQL one tab away.
func TestNewAppLandsOnIssuesAndReachesSearch(t *testing.T) {
	app := newApp(nil, nil, config.Profile{}, context.Background(), "", "", "", nil)
	if got := app.CurrentSection().ID(); got != issues.ID {
		t.Fatalf("landing section = %q, want %q", got, issues.ID)
	}
	m, _ := app.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	app = m.(core.App)
	if got := app.CurrentSection().ID(); got != issues.SearchID {
		t.Errorf("after tab, section = %q, want %q", got, issues.SearchID)
	}
}

// A nil service (unconfigured profile) must not panic: the dashboard still opens
// and renders, sections simply fetch nothing.
func TestNewAppWithoutServicesRenders(t *testing.T) {
	app := newApp(nil, nil, config.Profile{}, context.Background(), "", "", "", nil)
	if s := app.CurrentSection(); s == nil {
		t.Fatal("current section is nil with no services")
	}
}

func TestNewAppReloadsCustomFields(t *testing.T) {
	calls := 0
	load := func(cfg *config.Config) ([]core.CustomField, error) {
		calls++
		return []core.CustomField{{ID: cfg.TUI.CustomFields[0].Field}}, nil
	}
	initial := &config.Config{TUI: config.TUI{CustomFields: []config.TUICustomField{{Field: "customfield_10010"}}}}
	app := newApp(nil, initial, config.Profile{}, context.Background(), "", "", "", load)
	fresh := &config.Config{TUI: config.TUI{CustomFields: []config.TUICustomField{{Field: "customfield_10020"}}}}

	model, cmd := app.Update(core.ConfigReloadedMsg{Config: fresh})
	if calls != 2 {
		t.Fatalf("field loader calls = %d, want startup + reload", calls)
	}
	if model == nil || cmd == nil {
		t.Fatal("config reload must keep the app and schedule refreshed issue data")
	}
}

func TestApplyCustomFieldsKeepsValidFieldsAndReportsErrors(t *testing.T) {
	ctx := core.NewProgramContext(nil, nil)
	loadErr := errors.New("ambiguous custom field")
	load := func(*config.Config) ([]core.CustomField, error) {
		return []core.CustomField{{ID: "customfield_10010", Name: "Story Points"}}, loadErr
	}

	applyCustomFields(ctx, &config.Config{}, load)
	if len(ctx.CustomFields) != 1 || ctx.CustomFields[0].ID != "customfield_10010" {
		t.Fatalf("CustomFields = %#v", ctx.CustomFields)
	}
	if ctx.Err == nil || !strings.Contains(ctx.Err.Error(), loadErr.Error()) {
		t.Fatalf("Err = %v, want %v", ctx.Err, loadErr)
	}
}

// resolveOrder honors config tabs + default_tab: unregistered tabs are dropped
// (not opened blank), and the default tab becomes the landing view.
func TestResolveOrderHonorsConfigTabsAndDefault(t *testing.T) {
	reg := core.NewRegistry()
	reg.Register(issues.ID, issues.New)
	reg.Register(issues.SearchID, issues.NewSearch)

	t.Run("nil config falls back to built-in order", func(t *testing.T) {
		got := resolveOrder(nil, reg, nil)
		if len(got) != 2 || got[0] != issues.ID || got[1] != issues.SearchID {
			t.Errorf("order = %v, want [issues search]", got)
		}
	})

	t.Run("drops unregistered tabs and floats default to front", func(t *testing.T) {
		cfg := &config.Config{TUI: config.TUI{
			Tabs:       []string{"issues", "epics", "search", "activity"},
			DefaultTab: "search",
		}}
		got := resolveOrder(cfg, reg, nil)
		if len(got) != 2 || got[0] != issues.SearchID || got[1] != issues.ID {
			t.Errorf("order = %v, want [search issues] (epics/activity dropped, search first)", got)
		}
	})

	t.Run("all tabs unregistered falls back", func(t *testing.T) {
		cfg := &config.Config{TUI: config.TUI{Tabs: []string{"epics", "activity"}}}
		got := resolveOrder(cfg, reg, nil)
		if len(got) != 2 || got[0] != issues.ID {
			t.Errorf("order = %v, want fallback [issues search]", got)
		}
	})

	t.Run("default_tab not listed in tabs is ignored", func(t *testing.T) {
		// tabs is authoritative: a default_tab absent from tabs must not silently
		// prepend an unlisted view.
		cfg := &config.Config{TUI: config.TUI{
			Tabs:       []string{"issues"},
			DefaultTab: "search",
		}}
		got := resolveOrder(cfg, reg, nil)
		if len(got) != 1 || got[0] != issues.ID {
			t.Errorf("order = %v, want [issues] (unlisted default_tab ignored)", got)
		}
	})
}

// Configured query sections become tabs after Issues, and default_tab may name
// one by its title.
func TestResolveOrderInsertsConfiguredSections(t *testing.T) {
	cfg := &config.Config{TUI: config.TUI{
		Tabs: []string{"issues", "search"},
		Sections: []config.TUISection{
			{Title: "Team Board", JQL: "project = JCT"},
			{Title: "", JQL: ""}, // no JQL: skipped entirely
			{Title: "Bugs", JQL: "type = Bug"},
		},
	}}
	reg := core.NewRegistry()
	reg.Register(issues.ID, issues.New)
	reg.Register(issues.SearchID, issues.NewSearch)
	queries := registerQuerySections(reg, cfg)

	if len(queries) != 2 {
		t.Fatalf("registered %d query sections, want 2 (empty-JQL entry skipped)", len(queries))
	}
	// IDs are sequential over valid entries, not raw config positions.
	got := resolveOrder(cfg, reg, queries)
	want := []core.SectionID{issues.ID, issues.QueryID(0), issues.QueryID(1), issues.SearchID}
	if len(got) != len(want) {
		t.Fatalf("order = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order = %v, want %v", got, want)
		}
	}

	// default_tab by configured title floats that section to the front.
	cfg.TUI.DefaultTab = "bugs"
	got = resolveOrder(cfg, reg, queries)
	if got[0] != issues.QueryID(1) {
		t.Errorf("default_tab by title: order = %v, want Bugs first", got)
	}

	// tabs may position a query by title — and it must not be inserted twice.
	cfg.TUI.DefaultTab = ""
	cfg.TUI.Tabs = []string{"Bugs", "issues", "search"}
	got = resolveOrder(cfg, reg, queries)
	want = []core.SectionID{issues.QueryID(1), issues.ID, issues.QueryID(0), issues.SearchID}
	for i := range want {
		if i >= len(got) || got[i] != want[i] {
			t.Fatalf("tabs-by-title order = %v, want %v", got, want)
		}
	}
	if len(got) != len(want) {
		t.Fatalf("tabs-by-title order has duplicates: %v", got)
	}

	// A query titled like a builtin can't hijack default_tab: ID wins.
	cfgHijack := &config.Config{TUI: config.TUI{
		Tabs:       []string{"issues", "search"},
		DefaultTab: "search",
		Sections:   []config.TUISection{{Title: "Search", JQL: "project = X"}},
	}}
	regH := core.NewRegistry()
	regH.Register(issues.ID, issues.New)
	regH.Register(issues.SearchID, issues.NewSearch)
	qh := registerQuerySections(regH, cfgHijack)
	if got := resolveOrder(cfgHijack, regH, qh); got[0] != issues.SearchID {
		t.Errorf("default_tab=search landed on %v, want the builtin search tab", got[0])
	}

	// The registered section runs its configured query and titles its tab.
	app := core.NewApp(core.NewProgramContext(nil, cfg), reg, got)
	app.Init()
	if title := app.CurrentSection().Title(); title != "Bugs" {
		t.Errorf("section title = %q, want Bugs", title)
	}
}

// buildApp surfaces a missing credential as an error so the program never opens
// a dead dashboard against an unauthenticated profile.
func TestBuildAppMissingCredentialReturnsError(t *testing.T) {
	// Isolate the keyring lookup to an empty, throwaway service namespace so the
	// test is deterministic and can never read or mutate a real "work"
	// credential on the developer's machine.
	t.Setenv("JIRA_KEYRING_SERVICE", "jira-cli-tui-test-isolated")

	cfg := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(cfg, []byte(`default_profile = "work"

[[profiles]]
name = "work"
base_url = "https://company.atlassian.net"
auth_type = "token"
secret_backend = "keyring"
refresh_interval = 30
timeout = 30
workday_seconds = 28800
`), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	root := &cobra.Command{Use: "jira"}
	root.PersistentFlags().String("config", cfg, "")
	root.PersistentFlags().String("profile", "work", "")
	cmd := &cobra.Command{Use: "tui"}
	root.AddCommand(cmd)

	if _, err := buildApp(cmd); err == nil || !strings.Contains(err.Error(), `credential for profile "work" is required`) {
		t.Fatalf("buildApp err = %v, want credential-required", err)
	}
}
