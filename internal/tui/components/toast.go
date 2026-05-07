package components

import "github.com/gechr/primer/flash"

type Toast struct {
	state flash.State
}

func (t *Toast) Set(message string, isErr bool) flash.ClearMsg {
	return t.state.Set(message, isErr)
}

func (t *Toast) Clear(msg flash.ClearMsg) {
	t.state.Clear(msg)
}

func (t Toast) Active() bool {
	return t.state.Active()
}

func (t Toast) Render() string {
	return t.state.Msg
}
