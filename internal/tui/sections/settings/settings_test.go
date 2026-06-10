package settings

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/matcra587/jira-cli/internal/config"
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

// TestViewShowsResolvedConfig sanity-checks the render.
func TestViewShowsResolvedConfig(t *testing.T) {
	cfg := &config.Config{TUI: config.TUI{
		RefreshInterval: 60,
		DefaultTab:      "issues",
		Tabs:            []string{"issues", "search"},
		Sections:        []config.TUISection{{Title: "Bugs", JQL: "type = Bug"}},
	}}
	ctx := core.NewProgramContext(nil, cfg)
	ctx.ConfigPath = "/tmp/config.toml"
	ctx.ProfileName = "work"
	m := New(ctx).(*Model)
	m.Init(ctx)
	out := m.View()
	for _, want := range []string{"/tmp/config.toml", "work", "60s", "Bugs", "type = Bug", "reload now"} {
		if !strings.Contains(out, want) {
			t.Errorf("settings view missing %q in:\n%s", want, out)
		}
	}
}
