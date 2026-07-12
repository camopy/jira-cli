package modalstack

import "github.com/gechr/primer/overlay"

// Layer renders a modal's content for the given viewport size.
type Layer func(width, height int) string

// Model is a LIFO stack of overlay layers.
type Model struct {
	layers []Layer
}

// Push adds a layer on top.
func (m *Model) Push(layer Layer) {
	m.layers = append(m.layers, layer)
}

// Pop removes and returns the top layer, reporting false when empty.
func (m *Model) Pop() (Layer, bool) {
	if len(m.layers) == 0 {
		return nil, false
	}
	top := m.layers[len(m.layers)-1]
	m.layers = m.layers[:len(m.layers)-1]
	return top, true
}

// Top returns the top layer without removing it.
func (m *Model) Top() (Layer, bool) {
	if len(m.layers) == 0 {
		return nil, false
	}
	return m.layers[len(m.layers)-1], true
}

// Len returns the number of open layers.
func (m *Model) Len() int { return len(m.layers) }

// Active reports whether any layer is open.
func (m *Model) Active() bool { return len(m.layers) > 0 }

// Clear removes all layers.
func (m *Model) Clear() { m.layers = nil }

// Render centers the top layer (rendered at the current size) over the
// background. With no layers it returns the background unchanged, so callers can
// render unconditionally.
func (m *Model) Render(background string, width, height int) string {
	top, ok := m.Top()
	if !ok {
		return background
	}
	return overlay.Place(background, top(width, height), width, height, overlay.Center)
}
