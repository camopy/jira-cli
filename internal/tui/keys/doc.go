// Package keys holds the global key map for the section-based TUI and the
// config-driven rebinding that lets users override any binding by name.
//
// The map is intentionally flat: every binding is a named field so it can be
// looked up for rebinding and listed for contextual help. Sections may expose
// their own additional bindings, but the navigation and Jira-verb bindings
// shared across every view live here so the experience stays consistent.
package keys
