// Package version resolves build metadata for the running binary.
//
// Version resolution is delegated to clive: an ldflag-injected version when
// the build sets one (mise run install, GoReleaser), falling back to
// debug.BuildInfo so `go install ...@latest` and plain `go build` binaries
// report a real version instead of "dev". Commit, build time, and dirtiness
// come from the VCS metadata Go embeds at build time — no ldflags needed.
// Branch and BuildBy are jira-specific ldflags that clive does not model.
package version

import (
	"fmt"
	"runtime/debug"
	"sync"

	"github.com/gechr/clive"
)

// Linker-injected build metadata clive does not model. Set via -ldflags;
// "unknown" otherwise.
var (
	Branch  = "unknown"
	BuildBy = "unknown"
)

// shortHashLen matches git's default --abbrev=7 so ldflag-era output and
// VCS-derived output render commits identically.
const shortHashLen = 7

type vcsInfo struct {
	revision string
	time     string
	modified bool
}

// readVCS extracts the embedded VCS settings once. A binary built outside a
// git checkout (module-proxy `go install`) has none; callers fall back to
// "unknown" and Version carries the release identity instead.
var readVCS = sync.OnceValue(func() vcsInfo {
	var v vcsInfo
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return v
	}
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			v.revision = s.Value
		case "vcs.time":
			v.time = s.Value
		case "vcs.modified":
			v.modified = s.Value == "true"
		}
	}
	return v
})

// Version returns the resolved version string (see the package comment for
// the resolution order). It falls back to "dev" when no source resolves —
// e.g. `go run`, which embeds neither ldflags nor VCS info — so the version
// envelope field is never empty.
func Version() string {
	if v := clive.Current(); v != "" {
		return v
	}
	return "dev"
}

// Commit returns the short VCS revision the binary was built from, or
// "unknown" when the build embedded no VCS info.
func Commit() string {
	r := readVCS().revision
	if r == "" {
		return "unknown"
	}
	if len(r) > shortHashLen {
		return r[:shortHashLen]
	}
	return r
}

// BuildTime returns the commit timestamp (RFC 3339) embedded by the build,
// or "unknown" when no VCS info is present.
func BuildTime() string {
	if t := readVCS().time; t != "" {
		return t
	}
	return "unknown"
}

// Dirty reports whether the binary was built from a tree with uncommitted
// changes.
func Dirty() bool {
	return readVCS().modified
}

// String returns the one-line "version (commit)" summary.
func String() string {
	return fmt.Sprintf("%s (%s)", Version(), Commit())
}
