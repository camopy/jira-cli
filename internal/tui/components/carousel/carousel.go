// Package carousel renders a horizontal strip of labels with one active, and
// wraps navigation around the ends. It drives both the top-level section tabs
// and the per-issue detail sub-tabs, so the same selection behavior is shared.
package carousel

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// Model is a wrapping tab strip.
type Model struct {
	items  []string
	active int

	// Separator sits between items (default two spaces).
	Separator string
	// ActiveStyle / InactiveStyle style the labels. Zero values render plainly.
	ActiveStyle   lipgloss.Style
	InactiveStyle lipgloss.Style
}

// New returns a carousel over the given items with the first active. It returns
// a pointer because every mutating method has a pointer receiver.
func New(items ...string) *Model {
	return &Model{
		items:       items,
		Separator:   "  ",
		ActiveStyle: lipgloss.NewStyle().Bold(true).Underline(true),
	}
}

// SetItems replaces the items, clamping the active index into range.
func (m *Model) SetItems(items []string) {
	m.items = items
	m.clampActive()
}

// Items returns a copy of the current labels so external mutation cannot bypass
// SetItems and corrupt the active index.
func (m *Model) Items() []string {
	out := make([]string, len(m.items))
	copy(out, m.items)
	return out
}

// Active returns the active index, or -1 when empty.
func (m *Model) Active() int {
	if len(m.items) == 0 {
		return -1
	}
	return m.active
}

// ActiveItem returns the active label, or "" when empty.
func (m *Model) ActiveItem() string {
	if len(m.items) == 0 {
		return ""
	}
	return m.items[m.active]
}

// SetActive selects index i, clamping into range.
func (m *Model) SetActive(i int) {
	m.active = i
	m.clampActive()
}

// Next moves to the following item, wrapping to the first after the last.
func (m *Model) Next() {
	if len(m.items) == 0 {
		return
	}
	m.active = (m.active + 1) % len(m.items)
}

// Prev moves to the previous item, wrapping to the last before the first.
func (m *Model) Prev() {
	if len(m.items) == 0 {
		return
	}
	n := len(m.items)
	m.active = (m.active - 1 + n) % n
}

func (m *Model) clampActive() {
	if len(m.items) == 0 {
		m.active = 0
		return
	}
	if m.active < 0 {
		m.active = 0
	}
	if m.active >= len(m.items) {
		m.active = len(m.items) - 1
	}
}

// View renders the strip with the active label emphasized.
func (m *Model) View() string {
	if len(m.items) == 0 {
		return ""
	}
	parts := make([]string, len(m.items))
	for i, item := range m.items {
		if i == m.active {
			parts[i] = m.ActiveStyle.Render(item)
		} else {
			parts[i] = m.InactiveStyle.Render(item)
		}
	}
	return strings.Join(parts, m.Separator)
}
