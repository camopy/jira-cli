// Package selfupdate resolves which install channel delivered the running
// binary and builds the matching gechr/clive updater for it.
//
// jira-cli ships six ways. Two self-update in place: Homebrew installs go
// through clive's brew backend (formula refresh + upgrade), and release-archive
// installs (the one-line installer, mise-free manual downloads) go through
// clive's github backend (checksum-verified, rollback-safe binary swap). The
// rest are owned by their installer and are pointed at the exact command
// instead: Scoop and mise manage versioned install trees an in-place swap
// would desynchronize, and clive's goinstall backend installs `<module>@latest`,
// which cannot target this module's cmd/jira main package.
package selfupdate
