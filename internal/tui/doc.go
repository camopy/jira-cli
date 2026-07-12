// Package tui is the umbrella for the `jira tui` full-screen dashboard
// (alpha — the headless commands are the stable surface). The directory
// holds no code of its own; this file maps how the subpackages layer so a
// new view or widget starts in the right place.
//
// Layers, bottom-up by import direction:
//
//   - theme: lipgloss styles derived from the clib theme, applied once per
//     process so every layer above shares one look.
//   - keys: the global key map and the config-driven rebinding.
//   - components: reusable Bubble Tea widgets (inputs, pickers, list
//     viewport, markdown renderer, …) — Jira-free and core-free, so they
//     stay testable in isolation.
//   - core: the data-agnostic orchestration — the Section contract, the
//     shared ProgramContext, the task manager, the registry, and the root
//     App that routes messages without knowing what any view displays.
//   - sections: the actual views (issues, settings), each implementing
//     core.Section.
//
// Command wiring — resolving a client, registering sections, running the
// program — lives in internal/cli/tui, keeping this tree free of cobra and
// profile concerns. goldens pins rendered frames across all layers.
package tui
