// Package cli is the cobra-free presentation and error layer the command
// packages share. Operation data shapes live in internal/envelope; this package
// places shared keyed-result objects at data or data.results[].data, owns
// machine and human/plain rendering, and preserves the documented issue-view
// and keyed pagination/warning exceptions. It returns destination failures
// from every output mode and also owns output detection, terminal sanitization,
// verbs and errtax-backed error mapping. Staying free of Cobra lets command
// packages and cmdutil build on it without an import cycle.
package cli
