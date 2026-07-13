package core

import (
	"errors"
	"strings"
	"testing"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/matcra587/jira-cli/internal/config"
	"github.com/matcra587/jira-cli/internal/tui/theme"
)

// countingSection records how it is driven so tests can assert routing.
type countingSection struct {
	id       SectionID
	inits    int
	updates  int
	finished int
	captures bool
}

func (s *countingSection) ID() SectionID               { return s.id }
func (s *countingSection) Title() string               { return string(s.id) }
func (s *countingSection) View() string                { return "" }
func (s *countingSection) HelpBindings() []key.Binding { return nil }
func (s *countingSection) CapturesInput() bool         { return s.captures }

func (s *countingSection) Init(*ProgramContext) tea.Cmd {
	s.inits++
	return nil
}

func (s *countingSection) Update(msg tea.Msg) (Section, tea.Cmd) {
	s.updates++
	if _, ok := msg.(TaskFinishedMsg); ok {
		s.finished++
	}
	return s, nil
}

func TestAppDropsStaleTaskResults(t *testing.T) {
	ctx := NewProgramContext(nil, nil)
	reg := NewRegistry()
	cs := &countingSection{id: "issues"}
	reg.Register("issues", func(*ProgramContext) Section { return cs })

	a := NewApp(ctx, reg, []SectionID{"issues"})
	a.Init() // build and cache the issues section so a broadcast can reach it

	// Advance the issues generation to 1 (simulating a refresh).
	ctx.StartTask(TaskSpec{Scope: "issues"})

	// A result from the superseded generation 0 must be dropped, not forwarded.
	m, _ := a.Update(TaskFinishedMsg{Scope: "issues", Gen: 0})
	a = m.(App)
	if cs.finished != 0 {
		t.Errorf("stale task result was forwarded to the section (finished=%d)", cs.finished)
	}

	// The latest generation 1 is accepted and forwarded.
	m, _ = a.Update(TaskFinishedMsg{Scope: "issues", Gen: 1})
	_ = m
	if cs.finished != 1 {
		t.Errorf("latest task result was not forwarded (finished=%d)", cs.finished)
	}
}

func TestAppSwitchSectionWraps(t *testing.T) {
	ctx := NewProgramContext(nil, nil)
	reg := NewRegistry()
	reg.Register("issues", NewPlaceholderSection("issues", "Issues"))
	reg.Register("search", NewPlaceholderSection("search", "Search"))

	a := NewApp(ctx, reg, []SectionID{"issues", "search"})
	if a.CurrentSection().ID() != "issues" {
		t.Fatalf("initial section = %q, want issues", a.CurrentSection().ID())
	}

	a, _ = a.activate(1)
	if a.CurrentSection().ID() != "search" {
		t.Errorf("after next: section = %q, want search", a.CurrentSection().ID())
	}
	if ctx.View != "search" {
		t.Errorf("context view = %q, want search", ctx.View)
	}

	a, _ = a.activate(2) // wraps forward to index 0
	if a.CurrentSection().ID() != "issues" {
		t.Errorf("forward wrap: section = %q, want issues", a.CurrentSection().ID())
	}

	a, _ = a.activate(-1) // wraps backward to last
	if a.CurrentSection().ID() != "search" {
		t.Errorf("backward wrap: section = %q, want search", a.CurrentSection().ID())
	}
}

func TestAppInitsOncePerSectionAndReusesInstance(t *testing.T) {
	ctx := NewProgramContext(nil, nil)
	reg := NewRegistry()
	issues := &countingSection{id: "issues"}
	search := &countingSection{id: "search"}
	reg.Register("issues", func(*ProgramContext) Section { return issues })
	reg.Register("search", func(*ProgramContext) Section { return search })

	a := NewApp(ctx, reg, []SectionID{"issues", "search"})
	a.Init() // activates issues

	if issues.inits != 1 {
		t.Fatalf("issues Init ran %d times, want 1", issues.inits)
	}

	a, _ = a.activate(1) // → search
	if search.inits != 1 {
		t.Errorf("search Init ran %d times, want 1", search.inits)
	}

	first := a.CurrentSection()
	a, _ = a.activate(0) // back to issues
	if issues.inits != 1 {
		t.Errorf("returning to issues re-ran Init (%d); cached instance must persist", issues.inits)
	}
	a, _ = a.activate(1) // back to search: same instance, no re-init
	if search.inits != 1 {
		t.Errorf("returning to search re-ran Init (%d)", search.inits)
	}
	if a.CurrentSection() != first {
		t.Error("section instance was rebuilt on re-activation; state would be lost")
	}
}

func TestCapturingSectionGetsKeysBeforeGlobalShortcuts(t *testing.T) {
	ctx := NewProgramContext(nil, nil)
	reg := NewRegistry()
	cs := &countingSection{id: "issues", captures: true}
	reg.Register("issues", func(*ProgramContext) Section { return cs })

	a := NewApp(ctx, reg, []SectionID{"issues"})
	a.Init()

	before := cs.updates
	// "q" is the quit shortcut; a capturing section must receive it instead.
	a.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	if cs.updates != before+1 {
		t.Error("capturing section did not receive the key; a global shortcut stole it")
	}

	// With capture off, the same key is handled globally (not forwarded).
	cs.captures = false
	before = cs.updates
	a.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	if cs.updates != before {
		t.Error("non-capturing section received a global shortcut key")
	}
}

func TestAppStoresErrorFromMessage(t *testing.T) {
	ctx := NewProgramContext(nil, nil)
	a := NewApp(ctx, NewRegistry(), nil)

	m, _ := a.Update(ErrorMsg{Err: errFake})
	_ = m
	if !errors.Is(ctx.Err, errFake) {
		t.Errorf("ErrorMsg not stored on context: got %v", ctx.Err)
	}
}

var errFake = fakeErr("boom")

type fakeErr string

func (e fakeErr) Error() string { return string(e) }

// TestAppInitStartsEverySection pins the eager-start behavior: Init runs every
// registered section's Init (so background tabs fetch and report counts), and a
// later tab switch must not re-Init an already-started section.
func TestAppInitStartsEverySection(t *testing.T) {
	ctx := NewProgramContext(nil, nil)
	reg := NewRegistry()
	a := &countingSection{id: "a"}
	b := &countingSection{id: "b"}
	reg.Register("a", func(*ProgramContext) Section { return a })
	reg.Register("b", func(*ProgramContext) Section { return b })

	app := NewApp(ctx, reg, []SectionID{"a", "b"})
	app.Init()
	if a.inits != 1 || b.inits != 1 {
		t.Fatalf("inits = %d,%d; want 1,1 (background sections start eagerly)", a.inits, b.inits)
	}
	_, _ = app.activate(1)
	if b.inits != 1 {
		t.Errorf("activating an already-started section re-ran Init (%d)", b.inits)
	}
}

// TestAppRefreshTick verifies the heartbeat: a tick is broadcast to every
// section and re-arms itself; with no config the timer is disabled.
func TestAppRefreshTick(t *testing.T) {
	cfg := &config.Config{TUI: config.TUI{RefreshInterval: 1}}
	ctx := NewProgramContext(nil, cfg)
	reg := NewRegistry()
	a := &countingSection{id: "a"}
	b := &countingSection{id: "b"}
	reg.Register("a", func(*ProgramContext) Section { return a })
	reg.Register("b", func(*ProgramContext) Section { return b })

	app := NewApp(ctx, reg, []SectionID{"a", "b"})
	app.Init()
	au, bu := a.updates, b.updates
	_, cmd := app.Update(RefreshTickMsg{})
	if a.updates != au+1 || b.updates != bu+1 {
		t.Errorf("tick not broadcast to every section (a=%d b=%d)", a.updates-au, b.updates-bu)
	}
	if cmd == nil {
		t.Error("tick should re-arm the timer")
	}

	noCfg := NewApp(NewProgramContext(nil, nil), reg, []SectionID{"a"})
	if noCfg.refreshTick() != nil {
		t.Error("auto-refresh must be disabled without config")
	}
}

// TestHelpSheetTogglesOnAnyKey pins the help overlay: ? opens the full keymap,
// the view carries the dismiss hint, and any key closes it (consumed — the
// dismissing key must not also act).
func TestHelpSheetTogglesOnAnyKey(t *testing.T) {
	ctx := NewProgramContext(nil, nil)
	ctx.SetSize(100, 30)
	reg := NewRegistry()
	a := &countingSection{id: "a"}
	b := &countingSection{id: "b"}
	reg.Register("a", func(*ProgramContext) Section { return a })
	reg.Register("b", func(*ProgramContext) Section { return b })

	app := NewApp(ctx, reg, []SectionID{"a", "b"})
	app.Init()

	m, _ := app.Update(tea.KeyPressMsg{Code: '?', Text: "?"})
	app = m.(App)
	if !app.helpOpen {
		t.Fatal("? should open the help sheet")
	}
	if sheet := app.helpSheet(); !strings.Contains(sheet, "press any key to close") {
		t.Fatalf("help sheet missing dismiss hint:\n%s", sheet)
	}

	// The dismissing key is consumed: tab closes help, does NOT switch section.
	m, _ = app.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	app = m.(App)
	if app.helpOpen {
		t.Error("any key should close the help sheet")
	}
	if app.CurrentSection().ID() != "a" {
		t.Error("the dismissing key must not also switch sections")
	}
}

// reconfigure test fixture: a registry whose factories count builds, so the
// test can assert an invalidated section is rebuilt after a config reload.
func TestAppConfigReloadRebuildsSections(t *testing.T) {
	ctx := NewProgramContext(nil, nil)
	ctx.SetSize(80, 24)
	reg := NewRegistry()
	builds := map[SectionID]int{}
	register := func(id SectionID) *countingSection {
		cs := &countingSection{id: id}
		reg.Register(id, func(*ProgramContext) Section { builds[id]++; return cs })
		return cs
	}
	register("issues")
	register("jql:0")

	app := NewApp(ctx, reg, []SectionID{"issues", "jql:0"})
	app.Reconfigure = func(_ *ProgramContext, r *Registry, _ *config.Config) ([]SectionID, []SectionID) {
		// The reloaded config redefines the query section and adds another.
		register("jql:1")
		return []SectionID{"issues", "jql:0", "jql:1"}, []SectionID{"jql:0"}
	}
	app.Init()

	m, _ := app.Update(ConfigReloadedMsg{Config: &config.Config{}})
	app = m.(App)

	if builds["jql:0"] != 2 {
		t.Errorf("invalidated section built %d times, want 2 (rebuilt on reload)", builds["jql:0"])
	}
	if builds["jql:1"] != 1 {
		t.Errorf("new section built %d times, want 1", builds["jql:1"])
	}
	if builds["issues"] != 1 {
		t.Errorf("untouched section built %d times, want 1 (instance preserved)", builds["issues"])
	}
	if got := app.CurrentSection().ID(); got != "issues" {
		t.Errorf("active section after reload = %q, want issues (kept by ID)", got)
	}
}

// TestConfigReloadThemeChangeRebuildsAllSections pins the theme hot-reload:
// an unchanged theme.name preserves section instances (no flicker), a changed
// one drops every instance so cached derived styles re-derive.
func TestConfigReloadThemeChangeRebuildsAllSections(t *testing.T) {
	ctx := NewProgramContext(nil, nil)
	ctx.SetSize(80, 24)
	reg := NewRegistry()
	builds := 0
	reg.Register("issues", func(*ProgramContext) Section {
		builds++
		return &countingSection{id: "issues"}
	})
	app := NewApp(ctx, reg, []SectionID{"issues"})
	app.Init()
	snap := theme.Theme // restore the pre-test theme, not the process default
	t.Cleanup(func() { theme.Reload(snap) })

	m, _ := app.Update(ConfigReloadedMsg{Config: &config.Config{}})
	app = m.(App)
	if builds != 1 {
		t.Fatalf("same-theme reload rebuilt sections: %d builds, want 1", builds)
	}

	m, _ = app.Update(ConfigReloadedMsg{Config: &config.Config{Theme: config.Theme{Name: "light"}}})
	app = m.(App)
	if builds != 2 {
		t.Fatalf("theme change did not rebuild sections: %d builds, want 2", builds)
	}
	if got := app.CurrentSection().ID(); got != SectionID("issues") {
		t.Errorf("active section lost across theme reload: %q", got)
	}
}

// TestThemeReloadDetectsInPlaceConfigMutation pins the change detection's
// independence from the shared config pointer: even if the runtime config
// already carries the new name when the reload lands (any caller may have
// mutated it), the comparison must run against the theme the styles were
// actually derived from, so the change still rebuilds every section.
func TestThemeReloadDetectsInPlaceConfigMutation(t *testing.T) {
	cfg := &config.Config{}
	ctx := NewProgramContext(nil, cfg)
	ctx.SetSize(80, 24)
	reg := NewRegistry()
	builds := 0
	reg.Register("issues", func(*ProgramContext) Section {
		builds++
		return &countingSection{id: "issues"}
	})
	app := NewApp(ctx, reg, []SectionID{"issues"})
	app.Init()
	snap := theme.Theme // restore the pre-test theme, not the process default
	t.Cleanup(func() { theme.Reload(snap) })

	// Mutate the shared pointer first, then reload the same values — the
	// shape that would fool a config-vs-config comparison.
	cfg.Theme.Name = "light"
	reloaded := &config.Config{}
	reloaded.Theme.Name = "light"
	m, _ := app.Update(ConfigReloadedMsg{Config: reloaded})
	app = m.(App)
	if builds != 2 {
		t.Fatalf("in-place theme change did not rebuild sections: %d builds, want 2", builds)
	}
	// The same theme arriving again is a no-op — no flicker loop.
	m, _ = app.Update(ConfigReloadedMsg{Config: reloaded})
	_ = m
	if builds != 2 {
		t.Fatalf("unchanged theme rebuilt sections again: %d builds", builds)
	}
}

// TestRestyleMsgBroadcastsToEverySection pins RestyleMsg's contract: any
// producer's restyle reaches all sections, not just the active one — the
// icons preview is a non-App producer and silently missed background
// sections before this routing existed.
func TestRestyleMsgBroadcastsToEverySection(t *testing.T) {
	ctx := NewProgramContext(nil, nil)
	ctx.SetSize(80, 24)
	reg := NewRegistry()
	a := &countingSection{id: "a"}
	b := &countingSection{id: "b"}
	reg.Register("a", func(*ProgramContext) Section { return a })
	reg.Register("b", func(*ProgramContext) Section { return b })
	app := NewApp(ctx, reg, []SectionID{"a", "b"})
	app.Init()
	au, bu := a.updates, b.updates
	app.Update(RestyleMsg{})
	if a.updates != au+1 || b.updates != bu+1 {
		t.Errorf("RestyleMsg not broadcast (a=%d b=%d)", a.updates-au, b.updates-bu)
	}
}

// TestConfigReloadArmsTickWhenEnabled pins the disabled→enabled edge: with no
// tick loop running, a reload that turns refresh on must arm the timer.
func TestConfigReloadArmsTickWhenEnabled(t *testing.T) {
	ctx := NewProgramContext(nil, nil) // nil config: ticks disabled
	ctx.SetSize(80, 24)
	app := NewApp(ctx, NewRegistry(), nil)
	app.Init()

	_, cmd := app.Update(ConfigReloadedMsg{Config: &config.Config{TUI: config.TUI{RefreshInterval: 1}}})
	if cmd == nil {
		t.Fatal("enabling refresh via reload should arm the tick timer")
	}
}

// TestFocusAwareRefresh pins the focus gating: ticks while blurred keep the
// timer alive but refetch nothing; regaining focus refreshes immediately.
func TestFocusAwareRefresh(t *testing.T) {
	cfg := &config.Config{TUI: config.TUI{RefreshInterval: 1}}
	ctx := NewProgramContext(nil, cfg)
	reg := NewRegistry()
	a := &countingSection{id: "a"}
	reg.Register("a", func(*ProgramContext) Section { return a })
	app := NewApp(ctx, reg, []SectionID{"a"})
	app.Init()

	m, _ := app.Update(tea.BlurMsg{})
	app = m.(App)
	before := a.updates
	m, cmd := app.Update(RefreshTickMsg{})
	app = m.(App)
	if a.updates != before {
		t.Error("a tick while blurred must not reach sections")
	}
	if cmd != nil {
		t.Error("a blurred tick must not re-arm the timer — focus return starts a fresh one")
	}

	m, _ = app.Update(tea.FocusMsg{})
	_ = m.(App)
	if a.updates == before {
		t.Error("regaining focus should refresh sections immediately")
	}

	// With refresh disabled, regaining focus must not sneak a refetch in.
	ctxOff := NewProgramContext(nil, nil)
	b := &countingSection{id: "b"}
	regOff := NewRegistry()
	regOff.Register("b", func(*ProgramContext) Section { return b })
	off := NewApp(ctxOff, regOff, []SectionID{"b"})
	off.Init()
	m, _ = off.Update(tea.BlurMsg{})
	off = m.(App)
	bBefore := b.updates
	m, _ = off.Update(tea.FocusMsg{})
	_ = m
	if b.updates != bBefore {
		t.Error("focus regain must not refresh when auto-refresh is disabled")
	}
}

func TestClickOnTabRowActivatesSection(t *testing.T) {
	ctx := NewProgramContext(nil, nil)
	ctx.SetSize(120, 40)
	reg := NewRegistry()
	reg.Register("issues", NewPlaceholderSection("issues", "Issues"))
	reg.Register("search", NewPlaceholderSection("search", "Search"))
	a := NewApp(ctx, reg, []SectionID{"issues", "search"})

	firstW := lipgloss.Width(ctx.Styles.TabActive.Render("Issues"))
	m, _ := a.Update(tea.MouseClickMsg{X: firstW + 2, Y: 0, Button: tea.MouseLeft})
	a = m.(App)
	if a.CurrentSection().ID() != "search" {
		t.Errorf("section after tab click = %q, want search", a.CurrentSection().ID())
	}

	// A click on dead space right of the tabs changes nothing.
	m, _ = a.Update(tea.MouseClickMsg{X: 119, Y: 0, Button: tea.MouseLeft})
	a = m.(App)
	if a.CurrentSection().ID() != "search" {
		t.Errorf("dead-space click switched section to %q", a.CurrentSection().ID())
	}
}

func TestApplyConfigRebindsKeysAndRestoresOnRemoval(t *testing.T) {
	ctx := NewProgramContext(nil, nil)
	ctx.SetSize(100, 40)
	reg := NewRegistry()
	reg.Register("issues", NewPlaceholderSection("issues", "Issues"))
	a := NewApp(ctx, reg, []SectionID{"issues"})

	withKeys := &config.Config{}
	withKeys.TUI.Keys = map[string][]string{"quit": {"Q"}}
	m, _ := a.Update(ConfigReloadedMsg{Config: withKeys})
	a = m.(App)
	if got := ctx.Keys.Quit.Keys(); len(got) != 1 || got[0] != "Q" {
		t.Errorf("quit binding after reload = %v, want [Q]", got)
	}

	// Removing the override restores the default on the next reload.
	m, _ = a.Update(ConfigReloadedMsg{Config: &config.Config{}})
	a = m.(App)
	if got := ctx.Keys.Quit.Keys(); len(got) < 1 || got[0] == "Q" {
		t.Errorf("quit binding after removal = %v, want defaults", got)
	}

	// A bad override surfaces in the footer error and keeps the working map.
	bad := &config.Config{}
	bad.TUI.Keys = map[string][]string{"nope": {"z"}}
	_, _ = a.Update(ConfigReloadedMsg{Config: bad})
	if ctx.Err == nil {
		t.Error("invalid tui.keys override should surface as a footer error")
	}
	// The map must stay usable after a failed rebind.
	if got := ctx.Keys.Quit.Keys(); len(got) == 0 {
		t.Error("key map unusable after a failed rebind")
	}
}

func TestZoomAndResizeKeysRelayout(t *testing.T) {
	ctx := NewProgramContext(nil, nil)
	reg := NewRegistry()
	reg.Register("issues", NewPlaceholderSection("issues", "Issues"))
	a := NewApp(ctx, reg, []SectionID{"issues"})
	ctx.SetPreviewPosition(PreviewRight)
	m, _ := a.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	a = m.(App)

	m, _ = a.Update(tea.KeyPressMsg{Code: 'z', Text: "z"})
	a = m.(App)
	if ctx.PreviewWidth != 0 || ctx.MainWidth != 100 {
		t.Errorf("z did not zoom: preview=%d main=%d", ctx.PreviewWidth, ctx.MainWidth)
	}
	m, _ = a.Update(tea.KeyPressMsg{Code: 'z', Text: "z"})
	a = m.(App)
	if ctx.PreviewWidth != 50 {
		t.Errorf("second z did not restore the split: preview=%d", ctx.PreviewWidth)
	}

	m, _ = a.Update(tea.KeyPressMsg{Code: '-', Text: "-"})
	a = m.(App)
	if ctx.PreviewWidth != 45 {
		t.Errorf("- did not shrink the preview: %d, want 45", ctx.PreviewWidth)
	}
	m, _ = a.Update(tea.KeyPressMsg{Code: '+', Text: "+"})
	a = m.(App)
	_ = a
	if ctx.PreviewWidth != 50 {
		t.Errorf("+ did not grow the preview back: %d, want 50", ctx.PreviewWidth)
	}
}

// TestPauseGatesRefresh pins the R toggle: ticks while paused keep the timer
// alive but refetch nothing, pause survives focus loss and regain, and
// resuming refreshes immediately rather than waiting out the interval.
func TestPauseGatesRefresh(t *testing.T) {
	cfg := &config.Config{TUI: config.TUI{RefreshInterval: 1}}
	ctx := NewProgramContext(nil, cfg)
	reg := NewRegistry()
	a := &countingSection{id: "a"}
	reg.Register("a", func(*ProgramContext) Section { return a })
	app := NewApp(ctx, reg, []SectionID{"a"})
	app.Init()

	pauseKey := tea.KeyPressMsg{Code: 'R', Text: "R", Mod: tea.ModShift}
	m, _ := app.Update(pauseKey)
	app = m.(App)
	before := a.updates
	m, cmd := app.Update(RefreshTickMsg{})
	app = m.(App)
	if a.updates != before {
		t.Error("a tick while paused must not reach sections")
	}
	if cmd != nil {
		t.Error("a paused tick must not re-arm the timer — resume starts a fresh one")
	}

	// Pause must survive a blur/focus round-trip — the user chose it.
	m, _ = app.Update(tea.BlurMsg{})
	app = m.(App)
	m, _ = app.Update(tea.FocusMsg{})
	app = m.(App)
	if a.updates != before {
		t.Error("regaining focus while paused must not refetch")
	}

	m, _ = app.Update(pauseKey)
	_ = m.(App)
	if a.updates == before {
		t.Error("resuming should refresh sections immediately")
	}
}
