package settings

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/matcra587/jira-cli/internal/config"
	"github.com/matcra587/jira-cli/internal/tui/components/form"
	"github.com/matcra587/jira-cli/internal/tui/core"
)

func writeConfig(t *testing.T, dir, body string) string {
	t.Helper()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

const sampleConfig = `default_profile = "default"

[tui]
refresh_interval = 60
default_tab = "issues"
tabs = ["issues", "search"]

[[tui.sections]]
title = "Bugs"
jql = "type = Bug"

[[profiles]]
name = "default"
auth_type = "token"
secret_backend = "keyring"
`

// TestReloadEmitsFreshConfig pins the manual path: r re-reads the file and
// hands the parsed config to the App.
func TestReloadEmitsFreshConfig(t *testing.T) {
	path := writeConfig(t, t.TempDir(), sampleConfig)
	ctx := core.NewProgramContext(nil, nil)
	ctx.ConfigPath = path
	m := New(ctx).(*Model)
	m.Init(ctx)

	_, cmd := m.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
	if cmd == nil {
		t.Fatal("r should start a reload")
	}
	got := cmd()
	msg, ok := got.(core.ConfigReloadedMsg)
	if !ok {
		t.Fatalf("reload produced %T, want ConfigReloadedMsg", got)
	}
	if len(msg.Config.TUI.Sections) != 1 || msg.Config.TUI.Sections[0].Title != "Bugs" {
		t.Errorf("reloaded config sections = %+v", msg.Config.TUI.Sections)
	}
}

// TestAutoReloadOnTickWhenFileChanges pins the watcher: an unchanged file is a
// no-op on the heartbeat; a touched file triggers exactly one reload.
func TestAutoReloadOnTickWhenFileChanges(t *testing.T) {
	path := writeConfig(t, t.TempDir(), sampleConfig)
	ctx := core.NewProgramContext(nil, nil)
	ctx.ConfigPath = path
	m := New(ctx).(*Model)
	m.Init(ctx)

	if _, cmd := m.Update(core.RefreshTickMsg{}); cmd != nil {
		t.Error("tick with an unchanged file should not reload")
	}

	// Move the mtime decisively (filesystem clocks can be coarse).
	future := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(path, future, future); err != nil {
		t.Fatal(err)
	}
	_, cmd := m.Update(core.RefreshTickMsg{})
	if cmd == nil {
		t.Fatal("tick after a file change should reload")
	}
	if _, cmd := m.Update(core.RefreshTickMsg{}); cmd != nil {
		t.Error("the same change must not reload twice")
	}
}

// TestReloadWithBadFileSurfacesError pins the failure path: a broken file
// becomes a footer error, not a crash or a half-applied config.
func TestReloadWithBadFileSurfacesError(t *testing.T) {
	path := writeConfig(t, t.TempDir(), "this is not toml [[[")
	ctx := core.NewProgramContext(nil, nil)
	ctx.ConfigPath = path
	m := New(ctx).(*Model)
	m.Init(ctx)

	_, cmd := m.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
	if cmd == nil {
		t.Fatal("r should attempt the reload")
	}
	if _, ok := cmd().(core.ErrorMsg); !ok {
		t.Errorf("broken config produced %T, want ErrorMsg", cmd())
	}
}

// TestMenuShowsSettingsRows pins the default view: an interactive menu of
// the config keys with their current values.
func TestMenuShowsSettingsRows(t *testing.T) {
	path := writeConfig(t, t.TempDir(), sampleConfig)
	ctx := newSizedCtx(t, path)
	m := New(ctx).(*Model)
	m.Init(ctx)
	// Strip styling first: the inline mnemonic hints intersperse SGR codes
	// inside words, so raw Contains would miss them.
	out := ansi.Strip(m.View())
	for _, want := range []string{path, "Theme", "Icons", "Preview size", "Refresh interval", "Default tab", "issues"} {
		if !strings.Contains(out, want) {
			t.Errorf("settings menu missing %q in:\n%s", want, out)
		}
	}
}

// TestRawEditorShowsTheLiteralFile pins the escape hatch: e opens the
// literal config.toml for whole-file editing.
func TestRawEditorShowsTheLiteralFile(t *testing.T) {
	path := writeConfig(t, t.TempDir(), sampleConfig)
	ctx := newSizedCtx(t, path)
	m := New(ctx).(*Model)
	m.Init(ctx)
	m.Update(tea.KeyPressMsg{Code: 'e', Text: "e"})
	out := ansi.Strip(m.View())
	for _, want := range []string{`default_profile = "default"`, `jql = "type = Bug"`} {
		if !strings.Contains(out, want) {
			t.Errorf("raw editor missing %q in:\n%s", want, out)
		}
	}
}

// TestMenuPickerSavesAndReloads pins the enum flow: enter opens the picker,
// choosing a value applies it, writes the file, and hands back a reload.
func TestMenuPickerSavesAndReloads(t *testing.T) {
	path := writeConfig(t, t.TempDir(), sampleConfig)
	ctx := newSizedCtx(t, path)
	m := New(ctx).(*Model)
	m.Init(ctx)
	// Move to the Icons row, open its picker, and choose "nerd".
	m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !m.CapturesInput() || m.pickingRow < 0 {
		t.Fatal("enter on an enum row did not open the picker")
	}
	m.Update(tea.KeyPressMsg{Text: "nerd"}) // type-to-filter narrows to one
	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("picking a value did not save+reload")
	}
	msg, ok := cmd().(core.ConfigReloadedMsg)
	if !ok {
		t.Fatalf("pick produced %T, want ConfigReloadedMsg", cmd())
	}
	if msg.Config.TUI.Icons != "nerd" {
		t.Errorf("reloaded icons = %q, want nerd", msg.Config.TUI.Icons)
	}
	onDisk, _ := os.ReadFile(path)
	if !strings.Contains(string(onDisk), `icons = "nerd"`) {
		t.Errorf("pick not persisted; file:\n%s", onDisk)
	}
}

// TestMenuSaveNeverPersistsEnvOverlay pins the save path's most important
// property: a menu edit writes the file-backed config, never the runtime one
// — the runtime carries the JIRA_* env overlay, and persisting it would bake
// transient env values into config.toml.
func TestMenuSaveNeverPersistsEnvOverlay(t *testing.T) {
	t.Setenv("JIRA_DEFAULT_PROFILE", "env-only-profile")
	path := writeConfig(t, t.TempDir(), sampleConfig)
	ctx := newSizedCtx(t, path) // loads with the overlay, like the real wiring
	m := New(ctx).(*Model)
	m.Init(ctx)
	// Change Icons through the picker and let the save land.
	m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m.Update(tea.KeyPressMsg{Text: "nerd"})
	if _, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter}); cmd == nil {
		t.Fatal("pick did not save")
	}
	onDisk, _ := os.ReadFile(path)
	if strings.Contains(string(onDisk), "env-only-profile") {
		t.Fatalf("env overlay persisted into config.toml:\n%s", onDisk)
	}
	if !strings.Contains(string(onDisk), `default_profile = "default"`) {
		t.Fatalf("file-backed default_profile lost:\n%s", onDisk)
	}
	if !strings.Contains(string(onDisk), `icons = "nerd"`) {
		t.Fatalf("the actual edit missing:\n%s", onDisk)
	}
}

// TestMenuPickerEscCloses pins the cancel path: esc drops the picker with
// nothing applied.
func TestMenuPickerEscCloses(t *testing.T) {
	path := writeConfig(t, t.TempDir(), sampleConfig)
	ctx := newSizedCtx(t, path)
	m := New(ctx).(*Model)
	m.Init(ctx)
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter}) // Theme row picker
	m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.CapturesInput() {
		t.Fatal("esc did not close the picker")
	}
	if onDisk, _ := os.ReadFile(path); string(onDisk) != sampleConfig {
		t.Error("canceled pick reached the file")
	}
}

// TestThemePickerPreviewsLive pins the preview loop: moving the selection
// emits a ThemePreviewMsg for the highlighted theme, and esc previews the
// original back instead of leaving the dashboard on the abandoned candidate.
func TestThemePickerPreviewsLive(t *testing.T) {
	path := writeConfig(t, t.TempDir(), sampleConfig)
	ctx := newSizedCtx(t, path)
	m := New(ctx).(*Model)
	m.Init(ctx)
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter}) // Theme row picker
	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if cmd == nil {
		t.Fatal("selection change emitted no preview")
	}
	preview := findPreview(t, cmd())
	if preview.Name == "" {
		t.Fatal("preview carried no theme name")
	}
	// esc restores the original ("" — the config sets no theme).
	_, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if cmd == nil {
		t.Fatal("esc after a preview did not restore the original")
	}
	restore := findPreview(t, cmd())
	if restore.Name != "" {
		t.Errorf("esc previewed %q, want the original empty name", restore.Name)
	}
	if onDisk, _ := os.ReadFile(path); string(onDisk) != sampleConfig {
		t.Error("previewing wrote the file")
	}
}

// findPreview digs a ThemePreviewMsg out of a possibly-batched message.
func findPreview(t *testing.T, msg tea.Msg) core.ThemePreviewMsg {
	t.Helper()
	switch v := msg.(type) {
	case core.ThemePreviewMsg:
		return v
	case tea.BatchMsg:
		for _, c := range v {
			if c == nil {
				continue
			}
			if p, ok := c().(core.ThemePreviewMsg); ok {
				return p
			}
		}
	}
	t.Fatalf("no ThemePreviewMsg in %T", msg)
	return core.ThemePreviewMsg{}
}

// TestMenuValueEditValidates pins the number form: a bad value stays open
// with the reason and never reaches the file.
func TestMenuValueEditValidates(t *testing.T) {
	path := writeConfig(t, t.TempDir(), sampleConfig)
	ctx := newSizedCtx(t, path)
	m := New(ctx).(*Model)
	m.Init(ctx)
	m.openValueEdit(previewSizeRow(t, m))
	m.valueEdit.Update(tea.KeyPressMsg{Text: "9"}) // seeded "0" → "09": out of range
	if _, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter}); cmd != nil {
		t.Fatal("out-of-range preview size still saved")
	}
	if m.fail == "" {
		t.Error("no validation failure shown")
	}
	if m.editingValue < 0 {
		t.Error("form closed on a rejected value")
	}
	if onDisk, _ := os.ReadFile(path); string(onDisk) != sampleConfig {
		t.Error("rejected value reached the file")
	}
}

// TestMenuValueEditAppliesAndSaves pins the happy path for a number row.
func TestMenuValueEditAppliesAndSaves(t *testing.T) {
	path := writeConfig(t, t.TempDir(), sampleConfig)
	ctx := newSizedCtx(t, path)
	m := New(ctx).(*Model)
	m.Init(ctx)
	m.openValueEdit(previewSizeRow(t, m))
	m.valueEdit.Update(tea.KeyPressMsg{Code: tea.KeyBackspace}) // clear the seeded "0"
	m.valueEdit.Update(tea.KeyPressMsg{Text: "35"})
	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("valid value did not save")
	}
	if m.editingValue >= 0 {
		t.Error("form still open after a successful save")
	}
	onDisk, _ := os.ReadFile(path)
	if !strings.Contains(string(onDisk), "preview_size = 35") {
		t.Errorf("preview size not persisted; file:\n%s", onDisk)
	}
}

// TestTabsRowCompletesKnownNames pins the tabs autocomplete: typing a prefix
// in the tabs form offers the known tab names and accepting swaps the token.
func TestTabsRowCompletesKnownNames(t *testing.T) {
	path := writeConfig(t, t.TempDir(), sampleConfig)
	ctx := newSizedCtx(t, path)
	m := New(ctx).(*Model)
	m.Init(ctx)
	tabsRow := -1
	for i, s := range m.menu {
		if s.name == "Tabs" {
			tabsRow = i
			break
		}
	}
	if tabsRow < 0 {
		t.Fatal("Tabs row missing")
	}
	m.openValueEdit(tabsRow)
	// Extend the seeded "issues, search" and let the fetch land like the
	// real Update loop would.
	cmd, _, _ := m.valueEdit.Update(tea.KeyPressMsg{Text: ", ep"})
	if cmd != nil {
		if batch, ok := cmd().(tea.BatchMsg); ok {
			for _, c := range batch {
				if c == nil {
					continue
				}
				if sug, ok := c().(form.SuggestionsMsg); ok {
					m.Update(sug)
				}
			}
		} else if sug, ok := cmd().(form.SuggestionsMsg); ok {
			m.Update(sug)
		}
	}
	if !strings.Contains(ansi.Strip(m.View()), "epics") {
		t.Fatalf("tab suggestions not rendered:\n%s", ansi.Strip(m.View()))
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter}) // accept "epics"
	if got := m.valueEdit.Value(0); got != "issues, search, epics" {
		t.Fatalf("acceptance produced %q", got)
	}
}

// previewSizeRow locates the Preview size row by name so the tests don't pin
// menu ordering.
func previewSizeRow(t *testing.T, m *Model) int {
	t.Helper()
	for i, s := range m.menu {
		if s.name == "Preview size" {
			return i
		}
	}
	t.Fatal("Preview size row missing from the menu")
	return -1
}

func newSizedCtx(t *testing.T, path string) *core.ProgramContext {
	t.Helper()
	cfg, err := config.Load(config.WithPath(path))
	if err != nil {
		t.Fatalf("test config does not load: %v", err)
	}
	ctx := core.NewProgramContext(nil, cfg)
	ctx.SetSize(100, 40)
	ctx.ConfigPath = path
	return ctx
}

// editAndType opens the editor and replaces the whole buffer with text.
func editAndType(t *testing.T, m *Model, text string) {
	t.Helper()
	m.Update(tea.KeyPressMsg{Code: 'e', Text: "e"})
	if !m.editing || !m.CapturesInput() {
		t.Fatal("e did not open the editor")
	}
	m.openEditor("") // clear the seeded buffer for a deterministic draft
	m.Update(tea.KeyPressMsg{Text: text})
}

func TestEditSaveWritesAtomicallyAndReloads(t *testing.T) {
	path := writeConfig(t, t.TempDir(), sampleConfig)
	ctx := newSizedCtx(t, path)
	m := New(ctx).(*Model)
	m.Init(ctx)

	next := strings.Replace(sampleConfig, `default_tab = "issues"`, `default_tab = "search"`, 1)
	editAndType(t, m, next)
	_, cmd := m.Update(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
	if cmd == nil {
		t.Fatal("save produced no reload")
	}
	if m.editing {
		t.Error("editor still open after a successful save")
	}
	msg, ok := cmd().(core.ConfigReloadedMsg)
	if !ok {
		t.Fatalf("save produced %T, want ConfigReloadedMsg", cmd())
	}
	if msg.Config.TUI.DefaultTab != "search" {
		t.Errorf("reloaded default_tab = %q, want the edit applied", msg.Config.TUI.DefaultTab)
	}
	onDisk, _ := os.ReadFile(path)
	if !strings.Contains(string(onDisk), `default_tab = "search"`) {
		t.Error("edit not written to disk")
	}
}

func TestEditSaveRejectsInvalidTOMLAndKeepsDraft(t *testing.T) {
	path := writeConfig(t, t.TempDir(), sampleConfig)
	ctx := newSizedCtx(t, path)
	m := New(ctx).(*Model)
	m.Init(ctx)

	editAndType(t, m, "not toml [[[")
	if _, cmd := m.Update(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl}); cmd != nil {
		t.Fatal("invalid TOML still produced a reload")
	}
	if !m.editing {
		t.Error("editor closed on a failed save — the draft is gone")
	}
	if m.fail == "" {
		t.Error("no failure reason shown")
	}
	onDisk, _ := os.ReadFile(path)
	if string(onDisk) != sampleConfig {
		t.Error("invalid draft reached the file")
	}
}

func TestEditSaveRefusesEmptyBuffer(t *testing.T) {
	path := writeConfig(t, t.TempDir(), sampleConfig)
	ctx := newSizedCtx(t, path)
	m := New(ctx).(*Model)
	m.Init(ctx)

	editAndType(t, m, "   ")
	if _, cmd := m.Update(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl}); cmd != nil {
		t.Fatal("blank buffer still saved")
	}
	if onDisk, _ := os.ReadFile(path); string(onDisk) != sampleConfig {
		t.Error("blank draft reached the file")
	}
}

func TestEditEscGuardsDirtyDraft(t *testing.T) {
	path := writeConfig(t, t.TempDir(), sampleConfig)
	ctx := newSizedCtx(t, path)
	m := New(ctx).(*Model)
	m.Init(ctx)

	m.Update(tea.KeyPressMsg{Code: 'e', Text: "e"})
	m.Update(tea.KeyPressMsg{Text: "# a change"})
	m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if !m.editing {
		t.Fatal("dirty esc closed the editor without asking")
	}
	m.Update(tea.KeyPressMsg{Text: "y", Code: 'y'})
	if m.editing {
		t.Fatal("confirmed discard did not close the editor")
	}
}

func TestExternalWriteDoesNotFightOpenEditor(t *testing.T) {
	path := writeConfig(t, t.TempDir(), sampleConfig)
	ctx := newSizedCtx(t, path)
	m := New(ctx).(*Model)
	m.Init(ctx)

	m.Update(tea.KeyPressMsg{Code: 'e', Text: "e"})
	future := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(path, future, future); err != nil {
		t.Fatal(err)
	}
	if _, cmd := m.Update(core.RefreshTickMsg{}); cmd != nil {
		t.Error("heartbeat reloaded under an open editor")
	}
}
