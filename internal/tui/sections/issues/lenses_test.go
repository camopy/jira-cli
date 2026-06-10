package issues

import (
	"strings"
	"testing"

	"github.com/matcra587/jira-cli/internal/config"
	"github.com/matcra587/jira-cli/internal/tui/core"
)

func lensConfig() *config.Config {
	return &config.Config{TUI: config.TUI{
		Lenses: []config.TUISection{
			{Title: "Team", JQL: "project = JCT ORDER BY updated DESC"},
			{Title: "Blocked", JQL: "status = Blocked"},
			{Title: "", JQL: "missing title is skipped"},
			{Title: "no jql is skipped", JQL: "  "},
		},
		DefaultLens: "blocked",
	}}
}

func TestSetLensesFiltersInvalidEntries(t *testing.T) {
	ctx := core.NewProgramContext(nil, nil)
	ctx.SetLenses(lensConfig())
	if len(ctx.Lenses) != 2 {
		t.Fatalf("lenses = %+v, want the 2 valid entries", ctx.Lenses)
	}
	if ctx.Lenses[0].Name != "Team" || ctx.Lenses[1].Name != "Blocked" {
		t.Errorf("lens order/titles wrong: %+v", ctx.Lenses)
	}
	if ctx.DefaultLens != "blocked" {
		t.Errorf("default lens = %q", ctx.DefaultLens)
	}
}

func TestConfiguredLensesReplaceBuiltinsAndSetLanding(t *testing.T) {
	ctx := newTestCtx(fakeServices{issue: fakeIssueSvc{}})
	ctx.SetLenses(lensConfig())
	m := New(ctx).(*Model)
	m.Init(ctx)
	if m.lens != 1 {
		t.Errorf("landing lens = %d, want Blocked (default_lens, case-insensitive)", m.lens)
	}
	if got := m.lenses(); len(got) != 2 || got[0].Name != "Team" {
		t.Errorf("lenses = %+v, want configured set", got)
	}
	if v := m.View(); !strings.Contains(v, "[Blocked]") || !strings.Contains(v, "Team") {
		t.Errorf("chips missing configured lenses:\n%s", v)
	}
}

func TestNoConfiguredLensesKeepsBuiltins(t *testing.T) {
	ctx := newTestCtx(fakeServices{issue: fakeIssueSvc{}})
	m := New(ctx).(*Model)
	m.Init(ctx)
	if got := m.lenses(); len(got) != len(Lenses()) || got[0].Name != "Mine" {
		t.Errorf("without config the built-ins must stand: %+v", got)
	}
	if m.lens != 0 {
		t.Errorf("landing lens = %d, want first built-in", m.lens)
	}
}

func TestUnmatchedDefaultLensLandsOnFirst(t *testing.T) {
	cfg := lensConfig()
	cfg.TUI.DefaultLens = "nope"
	ctx := newTestCtx(fakeServices{issue: fakeIssueSvc{}})
	ctx.SetLenses(cfg)
	m := New(ctx).(*Model)
	m.Init(ctx)
	if m.lens != 0 {
		t.Errorf("unmatched default lens landed on %d, want 0", m.lens)
	}
}

func TestHotReloadShrinkingLensSetClampsActiveIndex(t *testing.T) {
	ctx := newTestCtx(fakeServices{issue: fakeIssueSvc{}})
	ctx.SetLenses(lensConfig())
	m := New(ctx).(*Model)
	m.Init(ctx)
	m.setLens(1)
	// Reload drops to one lens; the next fetch must clamp, not panic.
	ctx.SetLenses(&config.Config{TUI: config.TUI{
		Lenses: []config.TUISection{{Title: "Only", JQL: "project = JCT"}},
	}})
	if cmd := m.fetch(); cmd == nil {
		t.Fatal("fetch after shrink returned nothing")
	}
	if m.lens != 0 {
		t.Errorf("active lens after shrink = %d, want clamped to 0", m.lens)
	}
}

func TestViewSurvivesLensShrinkBeforeAnyKeypress(t *testing.T) {
	// A hot-reload can shrink the lens set between renders; the very next
	// View() must clamp the stale active index, not panic on lenses[active].
	ctx := newTestCtx(fakeServices{issue: fakeIssueSvc{}})
	ctx.SetLenses(lensConfig())
	m := New(ctx).(*Model)
	m.Init(ctx)
	m.setLens(1)
	ctx.SetLenses(&config.Config{TUI: config.TUI{
		Lenses: []config.TUISection{{Title: "Only", JQL: "project = JCT"}},
	}})
	if v := m.View(); !strings.Contains(v, "Only") {
		t.Errorf("view after shrink missing the surviving lens:\n%s", v)
	}
}

func TestSearchPresetsFollowConfiguredLenses(t *testing.T) {
	ctx := newTestCtx(fakeServices{issue: fakeIssueSvc{}})
	ctx.SetLenses(lensConfig())
	s := NewSearch(ctx).(*SearchModel)
	s.Init(ctx)
	if len(s.saved()) != 2 || s.saved()[0].Name != "Team" {
		t.Errorf("search presets = %+v, want configured lenses", s.saved())
	}
}

func TestSearchPresetsRefreshOnLensReload(t *testing.T) {
	ctx := newTestCtx(fakeServices{issue: fakeIssueSvc{}})
	s := NewSearch(ctx).(*SearchModel)
	s.Init(ctx)
	if s.saved()[0].Name != "Mine" {
		t.Fatalf("presets before reload = %+v, want built-ins", s.saved())
	}
	ctx.SetLenses(lensConfig()) // hot-reload swaps the lens set
	if got := s.saved(); len(got) != 2 || got[0].Name != "Team" {
		t.Errorf("presets after reload = %+v, want configured lenses without re-Init", got)
	}
}
