// Package keybind is the name-indexed key-rebinding registry: a map of
// stable lower-case names to bubbles key.Binding pointers, with config-driven
// overrides and enumeration. The semantics are deliberate and shared by any
// app that adopts it: an empty override is ignored so a partial config never
// silently unbinds an action, the first override key becomes the help label
// while the description is kept, and an unknown name is an error rather than
// a silent no-op so a config typo surfaces. The package knows nothing about
// which bindings exist — the owner declares its own key map struct and hands
// in the pointers — so it lifts cleanly into any Bubble Tea app.
package keybind
