//go:build live

// Package livekit is the shared harness for the live Jira end-to-end
// suites under tests/live/. It builds the jira binary, drives it as a
// subprocess against a real tenant, parses JSON envelopes, and tracks
// disposable issues for marker-gated cleanup. Each tests/live/<type>
// package imports this kit and supplies its own scenarios.
package livekit
