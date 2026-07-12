// Package version resolves build metadata for the running binary.
//
// Version resolution is delegated to clive: an ldflag-injected version when
// the build sets one (mise run install, GoReleaser), falling back to
// debug.BuildInfo so `go install ...@latest` and plain `go build` binaries
// report a real version instead of "dev". Commit, build time, and dirtiness
// come from the VCS metadata Go embeds at build time — no ldflags needed.
// Branch and BuildBy are jira-specific ldflags that clive does not model.
package version
