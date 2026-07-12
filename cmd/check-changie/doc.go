// Command check-changie fails a feat/fix commit that ships no changelog
// fragment (and no `Changelog: skip` trailer), so nothing user-facing is
// missed from the release notes. hk runs it from the commit-msg hook through
// the `check-changie` mise task.
//
// It is a Go program, not a POSIX-sh task script, so it runs identically under
// PowerShell, bash, and sh: mise resolves a `#!/usr/bin/env sh` task through an
// `sh` interpreter that Windows does not have on PATH, which failed every
// commit there. `go run` needs only the pinned Go toolchain.
//
// Usage: check-changie <commit-msg-file>
package main
