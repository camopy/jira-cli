// Package cli is the cobra-free presentation and error layer the command
// packages share. It owns the JSON envelope and its byte-clean writers, the
// human/plain renderers, output-mode detection, terminal sanitization, the verb
// and output-schema registries, and the errtax-backed error mapping. Staying
// free of any cobra dependency lets the command packages under internal/cli/*
// and cmdutil build on it without an import cycle.
package cli
