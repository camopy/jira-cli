//go:build live

// Package livekit is the shared harness for the live Jira end-to-end
// suites under tests/live/. It builds candidate HEAD by default or selects a
// frozen executable through JIRA_LIVETEST_BINARY, drives that binary against a
// real tenant, parses JSON envelopes, and tracks disposable issues for
// marker-gated cleanup. Each tests/live/<type> package imports this kit and
// supplies its own scenarios.
package livekit
