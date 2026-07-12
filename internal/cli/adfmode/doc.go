// Package adfmode resolves the strict vs best-effort ADF mode for a given
// CLI invocation, applying both the precedence ladder and per-path defaults.
//
// Precedence (highest first): command flag > JIRA_ADF_STRICT env > profile
// adf_strict > per-path default (read/render → best-effort, mutation submit →
// strict). The resolver is the only place mode selection happens; commands
// pass their resolved Mode into pkg/adf entry points.
package adfmode
