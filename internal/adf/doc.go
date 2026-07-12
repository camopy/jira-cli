// Package adf parses, validates, normalizes, and renders Atlassian Document
// Format — the JSON rich-text schema behind Jira descriptions, comments, and
// worklog notes. A registry of node and mark entries (registry.go) is the
// single source of truth for what is supported at which tier; parsing
// preserves unknown content for lossless round-trips, validation encodes the
// pinned @atlaskit/adf-schema contract, and the renderers (Markdown, plain
// text, styled segments) degrade unsupported nodes loudly rather than
// dropping them. Conversion in the other direction — Markdown convenience
// input to ADF — reports anything lossy as typed warnings so a mutation
// never silently rewrites what the user wrote.
package adf
