// Package cli is the cobra-free presentation and error layer the command
// packages share. It owns the JSON envelope, human/plain renderers, and the
// command-local writer tracking that returns destination failures from every
// supported output mode. It also owns output-mode detection, terminal
// sanitization, the verb and output-schema registries, and the errtax-backed
// error mapping. Staying free of any cobra dependency lets the command packages
// under internal/cli/* and cmdutil build on it without an import cycle.
package cli
