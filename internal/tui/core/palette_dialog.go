// The command palette: a fuzzy list of the commands available right now — the
// active section's contextual bindings plus the global navigation keys — where
// picking an entry replays its literal keybinding. Selection can therefore
// never drift from the keyboard path: the palette is a discoverable front end
// to the exact keys a user could press directly.

package core

import (
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"

	pkey "github.com/gechr/primer/key"

	"github.com/matcra587/jira-cli/internal/tui/components/dialog"
	"github.com/matcra587/jira-cli/internal/tui/components/palette"
	"github.com/matcra587/jira-cli/internal/tui/theme"
)

// paletteDialog wraps the palette component as a stack dialog. Like Pick it
// leaves accept (enter) and cancel (esc) to this wrapper and routes everything
// else — filter typing, cursor movement — into the palette. On accept the App
// reads the chosen entry's key and replays it.
type paletteDialog struct {
	palette palette.Model
}

// newPaletteDialog builds the palette over the commands available in the
// current context, snapshotting them at open time.
func (a App) newPaletteDialog() *paletteDialog {
	return &paletteDialog{palette: palette.New("Commands", a.paletteEntries(), paletteStyles())}
}

// Selected returns the entry under the cursor once the Stack has popped the
// dialog with ResultSubmit — the App reads its key to replay.
func (d *paletteDialog) Selected() (palette.Entry, bool) { return d.palette.Selected() }

// Title omits the Shell heading: the palette renders its own title as the first
// line of its View, matching Pick.
func (d *paletteDialog) Title() string { return "" }

// Content renders the palette — title, filter line, and matching rows.
func (d *paletteDialog) Content(int) string { return d.palette.View() }

// Hints are none: the palette pins its foot row through Footer (dialog.Footered)
// so the accept/cancel row survives the body scrolling on a short terminal.
func (d *paletteDialog) Hints() []pkey.Hint { return nil }

// Footer pins the accept/cancel row below the scrolling command list, so a
// palette taller than the box (25 commands on an 80x24 screen) keeps "enter
// run / esc close" visible instead of scrolling it off. Implements
// dialog.Footered.
func (d *paletteDialog) Footer() string {
	return dialog.RenderHints(theme.HelpKey, theme.HelpDesc, []pkey.Hint{
		{Key: "enter", Desc: "run"},
		{Key: "esc", Desc: "close"},
	})
}

// ScrollTo keeps the selected row in view as the cursor moves through a list
// taller than the box. The offset is measured within Content (title, query,
// rows), which is exactly the body the Footered frame scrolls. Implements
// dialog.ScrollHint.
func (d *paletteDialog) ScrollTo() (top, height int, ok bool) {
	return d.palette.CursorLine(), 1, true
}

// Update resolves the dialog on enter (accept) or esc (cancel) and routes
// everything else into the palette. Matching on Code keeps a modified enter or
// escape working and mirrors Pick.
func (d *paletteDialog) Update(msg tea.Msg) (dialog.Dialog, tea.Cmd, dialog.Result) {
	if k, ok := msg.(tea.KeyPressMsg); ok {
		switch k.Code {
		case tea.KeyEnter:
			return d, nil, dialog.ResultSubmit
		case tea.KeyEscape:
			return d, nil, dialog.ResultClose
		}
	}
	return d, d.palette.Update(msg), dialog.ResultNone
}

// paletteEntries collects the commands the palette offers: a curated set of
// global navigation and chrome keys first, then the active section's contextual
// bindings. It is deliberately narrower than the help sheet — the continuous
// cursor keys, quit, and the palette's own trigger are omitted, since a fuzzy
// "run a command" list should not put j/k or a stray q one enter away. Every
// entry is still a real binding, so the palette can never offer a command the
// keyboard doesn't. Each entry's key is the binding's first (canonical) key,
// which the App replays verbatim on selection.
func (a App) paletteEntries() []palette.Entry {
	k := a.ctx.Keys
	bindings := []key.Binding{
		k.NextSection, k.PrevSection,
		k.Refresh, k.TogglePause,
		k.CopyKey, k.CopyURL, k.OpLog, k.Help,
	}
	if s := a.build(a.activeID()); s != nil {
		bindings = append(bindings, s.HelpBindings()...)
	}

	seen := make(map[string]bool, len(bindings))
	entries := make([]palette.Entry, 0, len(bindings))
	for _, b := range bindings {
		if !b.Enabled() {
			continue
		}
		keys, hb := b.Keys(), b.Help()
		if len(keys) == 0 || hb.Desc == "" || seen[keys[0]] {
			continue
		}
		seen[keys[0]] = true
		entries = append(entries, palette.Entry{Name: hb.Desc, Key: keys[0]})
	}
	return entries
}

// paletteStyles maps the shared theme onto the palette's injected styles.
func paletteStyles() palette.Styles {
	return palette.Styles{
		Title:    theme.DetailHeader,
		Query:    theme.HelpDesc,
		Name:     theme.DetailValue,
		Match:    theme.HelpKey,
		Desc:     theme.HelpDesc,
		Selected: lipgloss.NewStyle().Bold(true).Reverse(true),
		KeyHint:  theme.HelpKey,
	}
}

// namedKeys maps each non-text key's canonical name back to its code, so
// replayKey can rebuild a KeyPressMsg whose String round-trips to the binding
// it came from. The names are sourced from bubbletea itself — every code is run
// through KeyPressMsg.String() — so this table can never disagree with the
// library's own spelling, and a version bump that renames a key updates it for
// free. This matters because [tui.keys] rebinding is free-form: a user can bind
// a verb to "ctrl+f5", and the palette must replay that literally, not a lossy
// approximation.
var namedKeys = buildNamedKeys()

// buildNamedKeys derives the name→code table by round-tripping every named key
// bubbletea exports (control, editing, navigation, and the function keys) back
// through String(). The function keys are consecutive codes, so they generate
// in a loop. Keypad, media, lock, and modifier keys are omitted deliberately:
// nobody binds a Jira verb to "kpenter" or "mute", and replayKey's fallback
// turns any such rebind into a safe no-op rather than a wrong match.
func buildNamedKeys() map[string]rune {
	codes := []rune{
		tea.KeyTab, tea.KeyEnter, tea.KeyEscape, tea.KeySpace, tea.KeyBackspace,
		tea.KeyDelete, tea.KeyInsert,
		tea.KeyUp, tea.KeyDown, tea.KeyLeft, tea.KeyRight,
		tea.KeyHome, tea.KeyEnd, tea.KeyPgUp, tea.KeyPgDown,
		tea.KeyBegin, tea.KeyFind, tea.KeySelect,
	}
	// KeyF1..KeyF63 are consecutive, so add them without listing all 63.
	for f := rune(0); tea.KeyF1+f <= tea.KeyF63; f++ {
		codes = append(codes, tea.KeyF1+f)
	}
	m := make(map[string]rune, len(codes))
	for _, c := range codes {
		m[tea.KeyPressMsg{Code: c}.String()] = c
	}
	return m
}

// replayKey rebuilds the KeyPressMsg for a binding's key string ("t", "tab",
// "shift+tab", "ctrl+u", "ctrl+f5"), so replaying a palette selection is
// byte-for-byte the same event pressing the key would raise — key.Matches
// compares String(), and this reconstructs exactly that. Modifier prefixes
// strip left to right; the remainder is a named key, a lone rune, or — for a
// multi-rune name we don't know — an extended key.
//
// Text is set only when unmodified. A KeyPressMsg's String() returns its Text
// verbatim when present, ignoring the modifier, so setting Text on a modified
// key would collapse "ctrl+f99" to a bare "f99" and could fire a command bound
// to that literal. Leaving Text empty lets String() fall through to the
// modifier-prefixed keystroke form, so an unsupported modified rebind renders
// as a stub that matches nothing rather than misfiring.
func replayKey(s string) tea.KeyPressMsg {
	var mod tea.KeyMod
	for {
		switch {
		case strings.HasPrefix(s, "ctrl+"):
			mod |= tea.ModCtrl
			s = s[len("ctrl+"):]
		case strings.HasPrefix(s, "alt+"):
			mod |= tea.ModAlt
			s = s[len("alt+"):]
		case strings.HasPrefix(s, "shift+"):
			mod |= tea.ModShift
			s = s[len("shift+"):]
		default:
			if code, ok := namedKeys[s]; ok {
				return tea.KeyPressMsg{Mod: mod, Code: code}
			}
			code := rune(tea.KeyExtended)
			if r := []rune(s); len(r) == 1 {
				code = r[0]
			}
			km := tea.KeyPressMsg{Mod: mod, Code: code}
			if mod == 0 {
				km.Text = s
			}
			return km
		}
	}
}
