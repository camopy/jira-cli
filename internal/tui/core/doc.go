// Package core defines the contracts every TUI view is built on: the Section
// interface, the shared ProgramContext, the async task manager, the section
// registry and the typed message set. The root App orchestrates Sections
// without knowing what data any of them holds, so adding a view (boards,
// epics, worklogs) is a matter of implementing Section and registering a
// factory — no change to the orchestration.
package core
