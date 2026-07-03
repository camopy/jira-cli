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

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"strings"

	"github.com/gechr/clive"
	"github.com/gechr/clive/updater/brew"
	"github.com/gechr/clive/updater/github"
	cliveversion "github.com/gechr/clive/version"

	"github.com/matcra587/jira-cli/internal/version"
)

// Module is the Go module path releases are published under; it locates the
// GitHub repository for release lookups and version links.
const Module = "github.com/matcra587/jira-cli"

// brewTap and brewFormula name the Homebrew distribution
// (github.com/matcra587/homebrew-tap, installed as matcra587/tap/jira).
const (
	brewTap     = "matcra587/tap"
	brewFormula = "jira"
)

// Hints are the exact update commands for channels whose installer owns the
// binary; jira never self-replaces these.
const (
	// ScoopHint updates a Scoop-managed install.
	ScoopHint = "scoop update jira"
	// MiseHint updates a mise-managed install.
	MiseHint = "mise up github:matcra587/jira-cli"
	// GoInstallHint rebuilds a go-install binary. The main package lives under
	// cmd/jira, which clive's goinstall backend (module-root installs only)
	// cannot target — hence a hint rather than a self-update.
	GoInstallHint = "go install " + Module + "/cmd/jira@latest"
)

// Channel identifies how the running binary was installed.
type Channel string

const (
	// ChannelBrew is a Homebrew-managed binary (Cellar or a brew prefix path);
	// self-updated via `brew upgrade` through clive's brew backend.
	ChannelBrew Channel = "brew"
	// ChannelScoop is a binary running out of a Scoop apps or shims directory.
	ChannelScoop Channel = "scoop"
	// ChannelMise is a mise-managed release-archive install.
	ChannelMise Channel = "mise"
	// ChannelGoInstall is a module-proxy build (`go install ...@version`),
	// recognized by a real module version in the embedded build info.
	ChannelGoInstall Channel = "go-install"
	// ChannelArchive is a GoReleaser release-archive binary outside any
	// installer-managed tree, recognized by the BuildBy=goreleaser ldflag stamp.
	ChannelArchive Channel = "github-archive"
	// ChannelUnknown is anything else — typically a from-source `go build`.
	ChannelUnknown Channel = "unknown"
)

// SelfUpdates reports whether jira replaces this channel's binary itself
// (rather than deferring to the channel's own installer or refusing).
func (c Channel) SelfUpdates() bool {
	return c == ChannelBrew || c == ChannelArchive
}

// Detect resolves the running binary's install channel from its executable
// path and embedded build metadata. Symlinks are resolved first so a brew
// prefix symlink (e.g. /opt/homebrew/bin/jira -> ../Cellar/...) is seen as
// its Cellar target.
func Detect() Channel {
	exe, err := os.Executable()
	if err != nil {
		exe = ""
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	return detect(exe, mainModuleVersion(), version.BuildBy, version.Commit() != "unknown")
}

// detect is the pure core of Detect. Installer-managed paths are checked
// before build stamps because Scoop, mise, and Homebrew all distribute the
// same GoReleaser-stamped binaries: the path says who owns the install, the
// stamp only says who built it. A module version alone does not mean
// `go install`: since Go 1.24 a plain `go build` inside a checkout stamps
// Main.Version from the VCS tags too — but such builds carry a VCS revision,
// which a module-proxy install never does, so vcsStamped disambiguates.
func detect(exe, mainVersion, buildBy string, vcsStamped bool) Channel {
	// Normalize both separators explicitly: ToSlash only converts the
	// host separator, and the heuristics must read Windows paths anywhere.
	p := strings.ToLower(strings.ReplaceAll(filepath.ToSlash(exe), `\`, "/"))
	switch {
	case isScoopPath(p):
		return ChannelScoop
	case isBrewPath(p):
		return ChannelBrew
	case isMisePath(p):
		return ChannelMise
	case mainVersion != "" && mainVersion != "(devel)" && !vcsStamped:
		return ChannelGoInstall
	case buildBy == "goreleaser":
		return ChannelArchive
	default:
		return ChannelUnknown
	}
}

// isScoopPath reports whether the normalized path lives under a Scoop install
// tree. Scoop runs binaries from <root>/apps/<app>/<version>/ behind
// <root>/shims/, and both segments survive custom SCOOP roots.
func isScoopPath(p string) bool {
	return strings.Contains(p, "/scoop/apps/") || strings.Contains(p, "/scoop/shims/")
}

// isBrewPath reports whether the normalized path lives under a Homebrew
// prefix: the Cellar (any prefix), /opt/homebrew (macOS ARM), or
// /home/linuxbrew/.linuxbrew (Linux).
func isBrewPath(p string) bool {
	return strings.Contains(p, "/cellar/") ||
		strings.Contains(p, "/homebrew/") ||
		strings.Contains(p, "/linuxbrew/")
}

// isMisePath reports whether the normalized path is a mise-managed tool
// install. mise's own Go toolchain directory (installs/go/<version>/bin, the
// GOBIN for go install under a mise-managed Go) is excluded: binaries there
// were placed by `go install` or a local build, not by `mise install`.
func isMisePath(p string) bool {
	return strings.Contains(p, "/mise/installs/") && !strings.Contains(p, "/mise/installs/go/")
}

// mainModuleVersion returns the main module's version from the embedded build
// info: a real version for a module-proxy build, "(devel)" for a source
// build, or "" when no build info is present.
func mainModuleVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	return info.Main.Version
}

// Updater checks for and installs the latest release through the backend that
// matches an install channel. Both implementations report progress and the
// old→new result line through clog on stderr.
type Updater interface {
	// Latest returns the newest installable release ref, fetched over the
	// network without a Go toolchain.
	Latest(ctx context.Context) (string, error)
	// Update installs the latest release over the current install.
	Update(ctx context.Context) error
}

// NewUpdater builds the Updater for a self-updating channel: clive's brew
// backend for ChannelBrew, clive's github release-asset backend (replacing
// the running executable in place) for ChannelArchive. Any other channel is
// an error — callers gate on Channel.SelfUpdates first. It returns the
// Updater interface deliberately: the implementation is a runtime choice
// between two backends the caller must stay agnostic of.
func NewUpdater(ch Channel) (Updater, error) {
	info := clive.Info{Module: Module}
	switch ch {
	case ChannelBrew:
		return brewUpdater{cfg: brew.New(
			info,
			brew.WithName("jira-cli"),
			brew.WithBinary("jira"),
			brew.WithFormula(brewFormula),
			brew.WithTap(brewTap),
			brew.WithResolveVersionFunc(resolveBinaryVersion),
		)}, nil
	case ChannelArchive:
		exe, err := os.Executable()
		if err != nil {
			return nil, fmt.Errorf("resolve executable path: %w", err)
		}
		return archiveUpdater{cfg: github.New(
			info,
			github.WithName("jira-cli"),
			github.WithBinary(filepath.Base(exe)),
			github.WithInstallDirectory(filepath.Dir(exe)),
		)}, nil
	case ChannelScoop, ChannelMise, ChannelGoInstall, ChannelUnknown:
	}
	return nil, fmt.Errorf("channel %s does not self-update", ch)
}

// resolveBinaryVersion reads the version the freshly-installed binary at bin
// reports. clive's default probe runs `<bin> version` and takes the raw
// output, but jira's version command emits a JSON envelope off a TTY — so ask
// for compact output and pull the version field out of the data object.
func resolveBinaryVersion(ctx context.Context, bin string) (string, error) {
	out, err := exec.CommandContext(ctx, bin, "version", "--output=compact").Output()
	if err != nil {
		return "", fmt.Errorf("probe installed binary version: %w", err)
	}
	var data struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(out, &data); err != nil {
		return "", fmt.Errorf("parse version envelope: %w", err)
	}
	return data.Version, nil
}

// brewUpdater upgrades a Homebrew-managed install: formula refresh, then
// `brew upgrade` (a --HEAD source build stays on --HEAD). Stray non-Homebrew
// copies on PATH are warned about, never removed.
type brewUpdater struct {
	cfg brew.Config
}

func (u brewUpdater) Latest(ctx context.Context) (string, error) {
	return u.cfg.LatestRef(ctx, nil)
}

func (u brewUpdater) Update(ctx context.Context) error {
	return brew.Update(ctx, u.cfg, brew.Upgrade)
}

// archiveUpdater swaps the running executable in place with the latest GitHub
// release asset (rollback-safe, verified against GoReleaser's checksums.txt).
// Cosign release signatures are not verified — only the checksum manifest is.
type archiveUpdater struct {
	cfg github.Config
}

func (u archiveUpdater) Latest(ctx context.Context) (string, error) {
	return u.cfg.LatestRef(ctx, nil)
}

func (u archiveUpdater) Update(ctx context.Context) error {
	return github.Update(ctx, u.cfg, github.Latest)
}

// UpdateAvailable reports whether latest is a strictly newer version than
// current, using clive's dev-build-aware semver rules. Unparseable input
// reports false: a from-source build never auto-updates on a bad compare.
func UpdateAvailable(current, latest string) bool {
	cur, err := cliveversion.Parse(current)
	if err != nil {
		return false
	}
	lat, err := cliveversion.Parse(latest)
	if err != nil {
		return false
	}
	return cliveversion.GreaterThan(lat, cur)
}
