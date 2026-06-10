package core

import "github.com/matcra587/jira-cli/internal/config"

// ErrorMsg carries a non-fatal error to the App, which stores it on the context
// and renders it in the footer. Errors never block the loop and are cleared by
// the next successful task.
//
// The action/selection/search message vocabulary (transition, comment, assign,
// issue-selected, JQL-submitted, ...) is intentionally not defined here: those
// messages are introduced alongside the sections and the action controller that
// produce and consume them, so the foundation never carries receiver-less types.
type ErrorMsg struct{ Err error }

// RefreshTickMsg is the dashboard's auto-refresh heartbeat. The App arms a
// timer from tui.refresh_interval, broadcasts this on each firing so every
// section may refetch if idle, then re-arms.
type RefreshTickMsg struct{}

// ConfigReloadedMsg carries a freshly re-read config file. The App swaps the
// shared config, asks its Reconfigure hook to rebuild the section order, and
// re-broadcasts the message so sections (e.g. settings) can refresh their view.
type ConfigReloadedMsg struct{ Config *config.Config }
