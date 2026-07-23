package settings

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	termansi "github.com/gechr/x/ansi"
	xos "github.com/gechr/x/os"
	xslices "github.com/gechr/x/slices"
	xstrings "github.com/gechr/x/strings"

	"github.com/matcra587/jira-cli/internal/config"
	"github.com/matcra587/jira-cli/internal/tui/components/form"
	"github.com/matcra587/jira-cli/internal/tui/components/input"
	"github.com/matcra587/jira-cli/internal/tui/components/picker"
	"github.com/matcra587/jira-cli/internal/tui/core"
	"github.com/matcra587/jira-cli/internal/tui/theme"
)

// ID is the section identifier.
const ID core.SectionID = "settings"

var _ core.Section = (*Model)(nil)

// Model is the settings section: an interactive menu over the config keys.
// Enum rows cycle in place and apply live — a theme change re-skins the
// dashboard on the spot — while numbers and lists edit through a small form.
// Every change validates, saves atomically, and hot-reloads. The literal
// config.toml stays one keypress away (e) for the structured tables the menu
// doesn't model (sections, lenses, keys).
type Model struct {
	ctx *core.ProgramContext
	// lastMod is the config file's mtime at the last load, the change signal
	// for auto-reload. The zero value means "unknown" and never triggers.
	lastMod time.Time
	notice  string
	fail    string

	// The menu.
	menu   []setting
	cursor int
	// valueEdit collects a number/text row's new value; editingValue is the
	// row it belongs to, -1 when closed.
	valueEdit    form.Model
	editingValue int
	// pick chooses an enum row's value from a type-to-filter list — the same
	// picker as transitions, so enums never overload the arrow keys the App
	// uses for tab switching. pickingRow is its row, -1 when closed.
	pick       picker.Model
	pickingRow int
	// pickOriginal is the value before the picker opened; pickShown tracks
	// what a previewing row currently renders, so backing out restores the
	// original and unchanged selections never re-preview.
	pickOriginal string
	pickShown    string

	// editor is the whole-file editing form (one multiline field). Its dirty
	// guard means a stray esc can never eat config edits; editBaseline is
	// the file text the session started from, so a mid-edit rebuild (a
	// resize, a failed save) never re-seeds the guard with the draft itself.
	editor       form.Model
	editing      bool
	editBaseline string
}

// New builds the settings section.
func New(ctx *core.ProgramContext) core.Section {
	return &Model{ctx: ctx, menu: settingsMenu(), editingValue: -1, pickingRow: -1}
}

// ID returns the settings section's identifier.
func (m *Model) ID() core.SectionID { return ID }

// Title returns the tab-bar label.
func (m *Model) Title() string { return "Settings" }

// Init records the config file's current mtime as the auto-reload baseline.
func (m *Model) Init(ctx *core.ProgramContext) tea.Cmd {
	m.ctx = ctx
	m.lastMod = m.mtime()
	m.applySize()
	return nil
}

// headerRows is what the chrome above the body costs: the path line, the
// status line, and the hint row.
const headerRows = 3

// applySize re-fits an open raw editor to the body; the menu itself sizes
// per render.
func (m *Model) applySize() {
	if m.editing {
		// Rebuilding on resize keeps the textarea inside the body. The form
		// re-seeds from the session baseline and the draft carries over as
		// the value — never as Initial, or the dirty guard would compare the
		// draft to itself and esc would discard without asking.
		draft := m.editor.Value(0)
		m.openEditor(m.editBaseline)
		m.editor.SetValue(0, draft)
	}
}

// rawConfig reads the config file at edit time — always the freshest bytes,
// no cache to invalidate. A missing file is an empty buffer, not an error
// (saving it creates the file); any other read failure is one, so an
// existing-but-unreadable config can never silently open as a blank buffer
// a later save would clobber.
func (m *Model) rawConfig() (string, error) {
	if m.ctx.ConfigPath == "" {
		return "", nil
	}
	data, err := os.ReadFile(m.ctx.ConfigPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return string(data), nil
}

// mtime returns the config file's modification time, or the zero time when
// there is no path or the file is unreadable.
func (m *Model) mtime() time.Time {
	if m.ctx.ConfigPath == "" {
		return time.Time{}
	}
	fi, err := os.Stat(m.ctx.ConfigPath)
	if err != nil {
		return time.Time{}
	}
	return fi.ModTime()
}

// maybeReload fires a reload when the file's mtime moved since the last load.
// It runs on the shared refresh heartbeat, so auto-reload is only as live as
// tui.refresh_interval (and off with it) — r always reloads immediately.
// An open editor or picker suppresses it: an external write must not fight
// a draft, and a reload naming a different theme would drop every section
// instance — including the one driving an open preview.
func (m *Model) maybeReload() tea.Cmd {
	if m.editing || m.editingValue >= 0 || m.pickingRow >= 0 {
		return nil
	}
	cur := m.mtime()
	if cur.IsZero() || cur.Equal(m.lastMod) {
		return nil
	}
	m.lastMod = cur
	return m.reloadCmd()
}

// reload is the manual path (the r key): it refreshes the mtime baseline and
// re-reads the file. maybeReload records the mtime it already observed and
// calls reloadCmd directly — a second stat here could see a newer write than
// the one the load reads, and the follow-up change would then be skipped.
func (m *Model) reload() tea.Cmd {
	if m.ctx.ConfigPath == "" {
		m.notice = "no config file to reload"
		return nil
	}
	m.lastMod = m.mtime()
	return m.reloadCmd()
}

// reloadCmd re-reads the config file and hands it to the App as a
// ConfigReloadedMsg; a parse failure surfaces in the footer instead.
func (m *Model) reloadCmd() tea.Cmd {
	path := m.ctx.ConfigPath
	return func() tea.Msg {
		cfg, err := config.Load(config.WithPath(path))
		if err != nil {
			return core.ErrorMsg{Err: fmt.Errorf("reload config: %w", err)}
		}
		return core.ConfigReloadedMsg{Config: cfg}
	}
}

// applyAndSave applies one row's change to the file-backed config and saves
// it. The change lands on a fresh LoadOrInit copy — never the runtime config:
// that carries the JIRA_* env overlay, and saving it would persist transient
// env values into config.toml (the exact corruption LoadOrInit exists to
// prevent). The runtime view updates through the reload, so a failed save
// leaves both the file and the running dashboard untouched.
func (m *Model) applyAndSave(s setting, value string) tea.Cmd {
	if m.ctx.ConfigPath == "" {
		m.fail = "no config file — press e to create one"
		return nil
	}
	onDisk, err := config.LoadOrInit(config.WithPath(m.ctx.ConfigPath))
	if err != nil {
		m.fail = "load config: " + err.Error()
		return nil
	}
	if err := s.apply(onDisk, value); err != nil {
		m.fail = err.Error()
		return nil
	}
	if err := config.Save(m.ctx.ConfigPath, onDisk); err != nil {
		m.fail = "write config: " + err.Error()
		return nil
	}
	m.fail = ""
	m.notice = "saved + applied " + time.Now().Format("15:04:05")
	m.lastMod = m.mtime()
	return m.reloadCmd()
}

// openValueEdit starts the inline form for a number/text row, seeded with the
// current value and completing tokens when the row declares a completer.
func (m *Model) openValueEdit(i int) {
	s := m.menu[i]
	spec := form.FieldSpec{Initial: s.current(m.ctx.Config), Optional: true}
	if s.complete != nil {
		spec.Autocomplete = s.complete(m)
	}
	m.valueEdit = form.New(form.Config{
		Title:  strings.ToLower(s.name) + " — " + s.hint,
		Width:  max(m.ctx.ScreenWidth-6, 1),
		Fields: []form.FieldSpec{spec},
		Styles: menuFormStyles(m.ctx.Styles),
	})
	m.editingValue = i
	m.fail = ""
}

// menuFormStyles wires the chrome styles into the inline value form.
func menuFormStyles(st core.Styles) form.Styles {
	return form.Styles{
		Title:    theme.DetailDim,
		HintKey:  st.HintKey,
		HintText: st.HintDesc,
		Question: theme.PillWarning,
		Error:    st.Error,
	}
}

// openEditor starts (or rebuilds) the raw-file editor over the given text.
func (m *Model) openEditor(text string) {
	w := max(m.ctx.ScreenWidth-2, 1)
	h := max(m.ctx.BodyHeight-headerRows, 1)
	m.editor = form.New(form.Config{
		Fields: []form.FieldSpec{{
			Initial:   text,
			Multiline: true,
			Rows:      h,
			Optional:  true, // emptiness is caught by validation, not the form
		}},
		EditorHatch: input.EditorCommand() != "",
		Width:       w,
		Styles:      menuFormStyles(m.ctx.Styles),
	})
	m.editing = true
	m.fail = ""
}

// saveRaw validates the raw draft with the loader's own strict decode and
// writes it atomically; a validation failure keeps the editor open with the
// reason. On success the running dashboard reloads immediately.
func (m *Model) saveRaw(text string) tea.Cmd {
	path := m.ctx.ConfigPath
	if path == "" {
		m.fail = "no config path to write"
		return nil
	}
	if xstrings.IsBlank(text) {
		m.fail = "refusing to write an empty config — esc to discard the edit instead"
		return nil
	}
	cleanupErr, err := validate(text)
	if err != nil {
		// Keep the draft and the editor: the user fixes the line instead of
		// losing the whole edit to a typo.
		m.fail = errors.Join(err, cleanupErr).Error()
		return nil
	}
	if err := xos.AtomicWrite(path, []byte(text), 0o600); err != nil {
		m.fail = "write config: " + err.Error()
		return nil
	}
	m.editing = false
	m.fail = ""
	m.notice = "saved + reloaded " + time.Now().Format("15:04:05")
	if cleanupErr != nil {
		m.notice += "; " + cleanupErr.Error()
	}
	m.lastMod = m.mtime()
	return m.reloadCmd()
}

// validate round-trips the draft through the real loader — same strict
// decode, same unknown-key rejection — via a temp file, so "it saved" always
// means "it loads".
func validate(text string) (cleanupErr, validationErr error) {
	tmp, err := os.CreateTemp("", "jira-config-*.toml")
	if err != nil {
		return nil, err
	}
	defer func() {
		if removeErr := os.Remove(tmp.Name()); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			cleanupErr = fmt.Errorf("remove validation temp file: %w", removeErr)
		}
	}()
	if _, err := tmp.WriteString(text); err != nil {
		return nil, errors.Join(err, tmp.Close())
	}
	if err := tmp.Close(); err != nil {
		return nil, err
	}
	_, err = config.Load(config.WithPath(tmp.Name()))
	return nil, err
}

// Update handles the menu, both edit modes, the reload key, the heartbeat
// poll, and the reload echo.
func (m *Model) Update(msg tea.Msg) (core.Section, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.applySize()
		return m, nil
	case core.RefreshTickMsg:
		return m, m.maybeReload()
	case core.ConfigReloadedMsg:
		if m.notice == "" {
			m.notice = "config reloaded " + time.Now().Format("15:04:05")
		}
		return m, nil
	case input.EditorFinishedMsg:
		return m, m.handleEditor(msg)
	case form.SuggestionsMsg:
		// Tab-name completion results route back into the open value form.
		if m.editingValue >= 0 {
			cmd, _, _ := m.valueEdit.Update(msg)
			return m, cmd
		}
		return m, nil
	case tea.PasteMsg:
		switch {
		case m.editing:
			cmd, _, _ := m.editor.Update(msg)
			return m, cmd
		case m.editingValue >= 0:
			cmd, _, _ := m.valueEdit.Update(msg)
			return m, cmd
		}
		return m, nil
	case tea.KeyPressMsg:
		m.notice = ""
		switch {
		case m.editing:
			return m, m.updateRawEditing(msg)
		case m.editingValue >= 0:
			return m, m.updateValueEditing(msg)
		case m.pickingRow >= 0:
			return m, m.updatePicking(msg)
		}
		return m, m.updateMenu(msg)
	}
	return m, nil
}

// updateMenu drives the row cursor; enter opens the row's editor — a picker
// for enums, a one-line form otherwise. Left/right are deliberately not
// bound: the App owns them for tab switching, and overloading them here made
// the two fight.
func (m *Model) updateMenu(msg tea.KeyPressMsg) tea.Cmd {
	k := m.ctx.Keys
	switch {
	case key.Matches(msg, k.Up):
		if m.cursor > 0 {
			m.cursor--
		}
	case key.Matches(msg, k.Down):
		if m.cursor < len(m.menu)-1 {
			m.cursor++
		}
	case key.Matches(msg, k.Edit):
		raw, err := m.rawConfig()
		if err != nil {
			m.fail = "read config: " + err.Error()
			return nil
		}
		m.editBaseline = raw
		m.openEditor(raw)
	case key.Matches(msg, k.Refresh):
		return m.reload()
	case key.Matches(msg, k.Open):
		if m.ctx.Config == nil {
			m.fail = "no config file — press e to create one"
			return nil
		}
		if m.menu[m.cursor].kind == kindEnum {
			m.openPicker(m.cursor)
			return nil
		}
		m.openValueEdit(m.cursor)
	}
	return nil
}

// openPicker starts the enum chooser for row i.
func (m *Model) openPicker(i int) {
	s := m.menu[i]
	items := xslices.Map(s.options(m), func(o string) picker.Item {
		return picker.Item{Label: o, Value: o}
	})
	m.pick = picker.New(strings.ToLower(s.name)+":", items)
	m.pickingRow = i
	m.pickOriginal = s.current(m.ctx.Config)
	m.pickShown = m.pickOriginal
	m.fail = ""
}

// updatePicking drives the enum chooser: enter applies the selection and
// saves, esc closes (previewing the original value back), everything else
// types into the picker's filter — and on a previewing row, every selection
// change shows that value live before anything is saved.
func (m *Model) updatePicking(msg tea.KeyPressMsg) tea.Cmd {
	s := m.menu[m.pickingRow]
	switch msg.String() {
	case "esc":
		m.pickingRow = -1
		if s.preview != nil && m.pickShown != m.pickOriginal {
			return s.preview(m, m.pickOriginal)
		}
		return nil
	case "enter":
		sel, ok := m.pick.Selected()
		if !ok {
			return nil
		}
		m.pickingRow = -1
		save := m.applyAndSave(s, sel.Value)
		if save == nil && s.preview != nil && m.pickShown != m.pickOriginal {
			// The save failed with a candidate still previewing: restore the
			// original live view, or the dashboard would keep rendering a
			// value the file never got.
			return s.preview(m, m.pickOriginal)
		}
		return save
	}
	cmd := m.pick.Update(msg)
	if s.preview != nil {
		if sel, ok := m.pick.Selected(); ok && sel.Value != m.pickShown {
			m.pickShown = sel.Value
			return tea.Batch(cmd, s.preview(m, sel.Value))
		}
	}
	return cmd
}

// updateValueEditing routes keys into the inline value form and applies a
// submitted value onto the config.
func (m *Model) updateValueEditing(msg tea.KeyPressMsg) tea.Cmd {
	cmd, ev, _ := m.valueEdit.Update(msg)
	switch ev {
	case form.EventSubmit:
		save := m.applyAndSave(m.menu[m.editingValue], m.valueEdit.Value(0))
		if save == nil {
			return nil // stay editing; the reason is on screen
		}
		m.editingValue = -1
		return save
	case form.EventCancel:
		m.editingValue = -1
		m.fail = ""
	case form.EventEditor, form.EventNone:
	}
	return cmd
}

// updateRawEditing routes keys into the whole-file editor and executes its
// events.
func (m *Model) updateRawEditing(msg tea.KeyPressMsg) tea.Cmd {
	cmd, ev, _ := m.editor.Update(msg)
	switch ev {
	case form.EventSubmit:
		return m.saveRaw(m.editor.Value(0))
	case form.EventCancel:
		m.editing = false
		m.fail = ""
	case form.EventEditor:
		// Hand the draft to $EDITOR; the form stays open so a failed launch
		// loses nothing (same contract as comments).
		return input.Edit("config:"+m.ctx.ConfigPath, m.editor.Value(0))
	case form.EventNone:
	}
	return cmd
}

// handleEditor resumes an external-editor round-trip: the buffer comes back
// as the draft and saves through the same validation as ctrl+s.
func (m *Model) handleEditor(msg input.EditorFinishedMsg) tea.Cmd {
	kind, _, ok := strings.Cut(msg.ID, ":")
	if !ok || kind != "config" || !m.editing {
		return nil
	}
	if msg.Err != nil {
		m.fail = msg.Err.Error()
		return nil
	}
	// Show what came back, then save it. The baseline stays the session's:
	// if the save is rejected (bad TOML), the editor stays open with the
	// returned text as a dirty draft the guard still protects.
	m.openEditor(m.editBaseline)
	m.editor.SetValue(0, msg.Text)
	return m.saveRaw(msg.Text)
}

// View renders the path header, the status line, the hint row, and the body —
// the menu (with any inline value form), or the raw editor when open.
func (m *Model) View() string {
	dim := theme.DetailDim
	path := m.ctx.ConfigPath
	if path == "" {
		path = dim.Render("(no config file)")
	}
	header := theme.DetailLabel.Render("Config ") + path
	status := " "
	switch {
	case m.fail != "":
		status = theme.StatusErr.Render("✗ " + m.fail)
	case m.notice != "":
		status = theme.StatusOK.Render(m.notice)
	case m.editing:
		status = dim.Render("editing config.toml — changes apply live on save")
	}

	var body string
	var hints []string
	switch {
	case m.editing:
		body = m.editor.View() // the form renders its own hint row
	default:
		body = m.menuView()
		hints = []string{
			core.HintSegment(m.ctx.Styles, "↑/↓", "move"),
			core.HintSegment(m.ctx.Styles, "enter", "change"),
			core.HintSegment(m.ctx.Styles, "e", "edit file"),
			core.HintSegment(m.ctx.Styles, "r", "reload"),
		}
		switch {
		case m.editingValue >= 0:
			body += "\n" + m.valueEdit.View()
		case m.pickingRow >= 0:
			body += "\n" + m.pick.View()
		}
	}
	out := header + "\n" + status + "\n"
	if len(hints) > 0 {
		out += strings.Join(hints, "  ") + "\n"
	}
	out += body
	return lipgloss.NewStyle().Padding(0, 1).Render(out)
}

// menuView renders the settings rows: cursor marker, name, current value,
// and a dim hint, in columns sized to the widest entry so a long value
// (a full tabs list, a custom theme name) can never crash into the hint.
// With no config loaded the menu is inert and says why.
func (m *Model) menuView() string {
	if m.ctx.Config == nil {
		return theme.DetailDim.Render("no config loaded — press e to create one")
	}
	nameW, valueW := 0, 0
	values := make([]string, len(m.menu))
	for i, s := range m.menu {
		values[i] = displayValue(s, m.ctx.Config)
		nameW = max(nameW, lipgloss.Width(s.name))
		valueW = max(valueW, lipgloss.Width(values[i]))
	}
	// Cap the value column so one huge list doesn't push every hint off
	// screen; a capped value truncates with an ellipsis.
	valueW = min(valueW, maxValueCol)
	var b strings.Builder
	for i, s := range m.menu {
		marker := "  "
		if i == m.cursor {
			marker = theme.DetailHeader.Render("› ")
		}
		value := termansi.Truncate(values[i], valueW, "…")
		row := fmt.Sprintf("%s%s  %s  %s", marker,
			theme.DetailLabel.Render(pad(s.name, nameW)),
			theme.DetailValue.Render(pad(value, valueW)),
			theme.DetailDim.Render(s.hint))
		b.WriteString(termansi.Truncate(row, m.ctx.ScreenWidth-2, "…") + "\n")
	}
	return b.String()
}

// maxValueCol caps the menu's value column width in cells.
const maxValueCol = 36

// pad space-pads s to w display cells.
func pad(s string, w int) string {
	if n := lipgloss.Width(s); n < w {
		return s + strings.Repeat(" ", w-n)
	}
	return s
}

// CapturesInput reports whether an editor or picker owns the keyboard.
func (m *Model) CapturesInput() bool {
	return m.editing || m.editingValue >= 0 || m.pickingRow >= 0
}

// HelpBindings lists the section's contextual bindings.
func (m *Model) HelpBindings() []key.Binding {
	return []key.Binding{
		key.NewBinding(key.WithKeys(m.ctx.Keys.Open.Keys()...), key.WithHelp(m.ctx.Keys.Open.Help().Key, "change setting")),
		key.NewBinding(key.WithKeys(m.ctx.Keys.Edit.Keys()...), key.WithHelp(m.ctx.Keys.Edit.Help().Key, "edit config file")),
		m.ctx.Keys.Refresh,
	}
}
