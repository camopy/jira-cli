package core

import (
	"testing"

	"github.com/matcra587/jira-cli/internal/config"
	"github.com/matcra587/jira-cli/internal/tui/keys"
)

func TestSetSizeFullWidthWhenSidebarClosed(t *testing.T) {
	ctx := NewProgramContext(nil, nil)
	ctx.SidebarOpen = false
	ctx.SetSize(100, 40)

	if ctx.MainWidth != 100 {
		t.Errorf("MainWidth = %d, want 100 (full width)", ctx.MainWidth)
	}
	if ctx.PreviewWidth != 0 || ctx.PreviewHeight != 0 {
		t.Errorf("preview should be empty when closed, got %dx%d", ctx.PreviewWidth, ctx.PreviewHeight)
	}
	if ctx.MainHeight != 40-chromeRows {
		t.Errorf("MainHeight = %d, want %d", ctx.MainHeight, 40-chromeRows)
	}
}

func TestSetSizeRightSidebarOnWideTerminal(t *testing.T) {
	ctx := NewProgramContext(nil, nil)
	ctx.SetSize(160, 50) // wide → auto resolves to right

	if ctx.PreviewPosition() != PreviewRight {
		t.Fatalf("position = %q, want right on a wide terminal", ctx.PreviewPosition())
	}
	if ctx.MainWidth+ctx.PreviewWidth != 160 {
		t.Errorf("main+preview width = %d, want 160", ctx.MainWidth+ctx.PreviewWidth)
	}
	if ctx.PreviewHeight != 50-chromeRows {
		t.Errorf("right sidebar should span body height: got %d, want %d", ctx.PreviewHeight, 50-chromeRows)
	}
}

func TestSetSizeBottomSidebarOnNarrowTerminal(t *testing.T) {
	ctx := NewProgramContext(nil, nil)
	ctx.SetSize(80, 40) // narrow → auto resolves to bottom

	if ctx.PreviewPosition() != PreviewBottom {
		t.Fatalf("position = %q, want bottom on a narrow terminal", ctx.PreviewPosition())
	}
	if ctx.PreviewWidth != 80 {
		t.Errorf("bottom sidebar should span full width: got %d", ctx.PreviewWidth)
	}
	if ctx.MainHeight+ctx.PreviewHeight != 40-chromeRows {
		t.Errorf("main+preview height = %d, want %d", ctx.MainHeight+ctx.PreviewHeight, 40-chromeRows)
	}
}

func TestSetPreviewPositionForcesPlacement(t *testing.T) {
	ctx := NewProgramContext(nil, nil)
	ctx.SetSize(200, 50)
	ctx.SetPreviewPosition(PreviewBottom) // force bottom even though wide

	if ctx.PreviewPosition() != PreviewBottom {
		t.Errorf("forced position not honored: got %q", ctx.PreviewPosition())
	}
	if ctx.PreviewWidth != 200 {
		t.Errorf("forced bottom sidebar should span width: got %d", ctx.PreviewWidth)
	}
}

// TestSetPreviewFromConfig pins the tui.preview mapping: side docks split the
// width, bottom splits the height, hidden closes, and unknown/empty values
// leave the runtime state alone (so a reload can't fight the p-key cycle).
func TestSetPreviewFromConfig(t *testing.T) {
	ctx := NewProgramContext(nil, nil)
	ctx.SetSize(100, 40)

	ctx.SetPreviewFromConfig("left")
	if got := ctx.PreviewPosition(); got != PreviewLeft {
		t.Errorf("position = %q, want left", got)
	}
	if ctx.MainWidth != 50 || ctx.PreviewWidth != 50 {
		t.Errorf("left dock split = main %d / preview %d, want 50/50", ctx.MainWidth, ctx.PreviewWidth)
	}

	ctx.SetPreviewFromConfig("hidden")
	if ctx.SidebarOpen || ctx.PreviewWidth != 0 {
		t.Errorf("hidden should close the sidebar (open=%v width=%d)", ctx.SidebarOpen, ctx.PreviewWidth)
	}

	ctx.SetPreviewFromConfig("BOTTOM") // case-insensitive
	if got := ctx.PreviewPosition(); !ctx.SidebarOpen || got != PreviewBottom {
		t.Errorf("position = %q open=%v, want bottom/open", got, ctx.SidebarOpen)
	}
	// Re-opening from hidden must recompute the split immediately (SetSize runs
	// inside SetPreviewPosition), not wait for the next terminal resize.
	if ctx.PreviewHeight == 0 || ctx.MainHeight+ctx.PreviewHeight != ctx.BodyHeight {
		t.Errorf("hidden→bottom split = main %d + preview %d, want %d total",
			ctx.MainHeight, ctx.PreviewHeight, ctx.BodyHeight)
	}

	ctx.SetPreviewFromConfig("") // absent key: no-op
	if got := ctx.PreviewPosition(); got != PreviewBottom {
		t.Errorf("empty value moved the sidebar to %q", got)
	}
}

// TestCyclePreviewVisitsAllDocks pins the p-key order: right → bottom → left →
// hidden → right.
func TestCyclePreviewVisitsAllDocks(t *testing.T) {
	ctx := NewProgramContext(nil, nil)
	ctx.SetSize(200, 50) // wide: auto resolves right
	app := NewApp(ctx, NewRegistry(), nil)

	want := []PreviewPosition{PreviewBottom, PreviewLeft}
	for _, w := range want {
		app.cyclePreview()
		if got := ctx.PreviewPosition(); !ctx.SidebarOpen || got != w {
			t.Fatalf("cycle landed on %q (open=%v), want %q", got, ctx.SidebarOpen, w)
		}
	}
	app.cyclePreview()
	if ctx.SidebarOpen {
		t.Fatal("cycle after left should hide the sidebar")
	}
	app.cyclePreview()
	if got := ctx.PreviewPosition(); !ctx.SidebarOpen || got != PreviewRight {
		t.Fatalf("cycle after hidden landed on %q, want right/open", got)
	}
}

func TestRebindKeysAppliesAndRestoresDefaults(t *testing.T) {
	ctx := NewProgramContext(nil, nil)

	cfg := &config.Config{}
	cfg.TUI.Keys = map[string][]string{"transition": {"x"}}
	if err := ctx.RebindKeys(cfg); err != nil {
		t.Fatalf("RebindKeys: %v", err)
	}
	if got := ctx.Keys.Transition.Keys(); len(got) != 1 || got[0] != "x" {
		t.Errorf("transition keys = %v, want [x]", got)
	}

	// Removing the override on a later reload restores the default binding.
	if err := ctx.RebindKeys(&config.Config{}); err != nil {
		t.Fatalf("RebindKeys(empty): %v", err)
	}
	def := keys.Default().Transition.Keys()
	if got := ctx.Keys.Transition.Keys(); len(got) != len(def) || got[0] != def[0] {
		t.Errorf("transition keys after reset = %v, want defaults %v", got, def)
	}
}

func TestRebindKeysUnknownActionKeepsCurrentMap(t *testing.T) {
	ctx := NewProgramContext(nil, nil)
	good := &config.Config{}
	good.TUI.Keys = map[string][]string{"transition": {"x"}}
	if err := ctx.RebindKeys(good); err != nil {
		t.Fatal(err)
	}

	bad := &config.Config{}
	bad.TUI.Keys = map[string][]string{"transitionz": {"y"}}
	if err := ctx.RebindKeys(bad); err == nil {
		t.Fatal("unknown action did not error")
	}
	if got := ctx.Keys.Transition.Keys(); len(got) != 1 || got[0] != "x" {
		t.Errorf("failed rebind must keep the previous map, got %v", got)
	}
}

func TestPreviewRatioResizesSplit(t *testing.T) {
	ctx := NewProgramContext(nil, nil)
	ctx.SetPreviewPosition(PreviewRight)
	ctx.SetSize(100, 40)
	if ctx.PreviewWidth != 50 {
		t.Fatalf("default split preview = %d, want 50", ctx.PreviewWidth)
	}
	ctx.AdjustPreviewRatio(-0.2) // 30% preview
	if ctx.PreviewWidth != 30 || ctx.MainWidth != 70 {
		t.Errorf("after shrink: preview=%d main=%d, want 30/70", ctx.PreviewWidth, ctx.MainWidth)
	}
	ctx.AdjustPreviewRatio(1) // clamps at the ceiling
	if ctx.PreviewWidth > 80 {
		t.Errorf("ratio ceiling breached: preview=%d", ctx.PreviewWidth)
	}
	ctx.AdjustPreviewRatio(-1) // clamps at the floor
	if ctx.PreviewWidth < 20 {
		t.Errorf("ratio floor breached: preview=%d", ctx.PreviewWidth)
	}
}

func TestPreviewRatioAppliesToBottomDock(t *testing.T) {
	ctx := NewProgramContext(nil, nil)
	ctx.SetPreviewPosition(PreviewBottom)
	ctx.SetSize(100, 42) // bodyH = 38
	half := ctx.PreviewHeight
	ctx.AdjustPreviewRatio(-0.2)
	if ctx.PreviewHeight >= half {
		t.Errorf("bottom dock did not shrink: %d → %d", half, ctx.PreviewHeight)
	}
}

func TestZoomCollapsesAndRestoresPreview(t *testing.T) {
	ctx := NewProgramContext(nil, nil)
	ctx.SetPreviewPosition(PreviewRight)
	ctx.SetSize(100, 40)
	ctx.ToggleZoom()
	if ctx.PreviewWidth != 0 || ctx.MainWidth != 100 {
		t.Errorf("zoomed: preview=%d main=%d, want 0/100", ctx.PreviewWidth, ctx.MainWidth)
	}
	if !ctx.SidebarOpen {
		t.Error("zoom must not change the sidebar-open state itself")
	}
	ctx.ToggleZoom()
	if ctx.PreviewWidth != 50 {
		t.Errorf("unzoom did not restore the split: preview=%d", ctx.PreviewWidth)
	}
}

func TestNarrowTerminalAutoHidesSideDock(t *testing.T) {
	ctx := NewProgramContext(nil, nil)
	ctx.SetPreviewPosition(PreviewRight)
	ctx.SetSize(45, 40) // below the side-dock floor
	if ctx.PreviewWidth != 0 || ctx.MainWidth != 45 {
		t.Errorf("narrow terminal should hide the side dock: preview=%d main=%d", ctx.PreviewWidth, ctx.MainWidth)
	}
	ctx.SetSize(120, 40) // wide again: the split returns
	if ctx.PreviewWidth == 0 {
		t.Error("split did not return at full width")
	}
}

func TestSetPreviewRatioFromConfigPercent(t *testing.T) {
	ctx := NewProgramContext(nil, nil)
	ctx.SetPreviewPosition(PreviewRight)
	ctx.SetPreviewRatioPercent(30)
	ctx.SetSize(100, 40)
	if ctx.PreviewWidth != 30 {
		t.Errorf("preview_size 30 → preview=%d, want 30", ctx.PreviewWidth)
	}
	ctx.SetPreviewRatioPercent(0) // absent config: no change
	ctx.SetSize(100, 40)
	if ctx.PreviewWidth != 30 {
		t.Errorf("absent preview_size must keep the ratio, got %d", ctx.PreviewWidth)
	}
	ctx.SetPreviewRatioPercent(99) // out of range clamps
	ctx.SetSize(100, 40)
	if ctx.PreviewWidth > 80 {
		t.Errorf("out-of-range percent must clamp, got %d", ctx.PreviewWidth)
	}
}

func TestZoomIgnoredWhileSidebarClosedAndClearedOnReopen(t *testing.T) {
	ctx := NewProgramContext(nil, nil)
	ctx.SetPreviewPosition(PreviewRight)
	ctx.SetSize(100, 40)

	ctx.SidebarOpen = false
	ctx.SetSize(100, 40)
	ctx.ToggleZoom() // nothing visible to zoom: must not latch
	ctx.SidebarOpen = true
	ctx.SetSize(100, 40)
	if ctx.PreviewWidth == 0 {
		t.Error("latent zoom collapsed a reopened preview")
	}

	// Zoom, then move the preview: showing it again must clear the zoom.
	ctx.ToggleZoom()
	if ctx.PreviewWidth != 0 {
		t.Fatal("zoom did not collapse the preview")
	}
	ctx.SetPreviewPosition(PreviewBottom)
	ctx.SetSize(100, 40)
	if ctx.PreviewHeight == 0 {
		t.Error("moving the preview should clear zoom and show it")
	}
}
