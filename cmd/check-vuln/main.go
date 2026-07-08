// Command check-vuln runs govulncheck and fails CI on any advisory that
// affects our code, with one narrow exception: a documented allowlist of
// advisory IDs we have reviewed and accepted.
//
// It exists because govulncheck has no native allowlist. `-format json` never
// sets a non-zero exit code — it streams findings and leaves the verdict to
// the caller — so this program is that caller: it parses the stream, decides
// which advisories affect us, subtracts the allowlist, and owns the exit code.
//
// Like cmd/check-changie it is a Go program, not a POSIX-sh task, so it runs
// identically under PowerShell, bash, and sh; `go run` needs only the pinned
// toolchain. The `security` mise task invokes it in place of a bare
// `govulncheck ./...`.
//
// The allowlist is deliberately hostile to rot: an entry that no longer fires
// (upstream fixed it, the DB withdrew it, the import went away) fails the
// build until removed, the same discipline nolintlint enforces on //nolint.
package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"time"

	"github.com/gechr/clog"
)

// allowed lists advisory IDs we have reviewed and accept, each with the reason
// it is safe to ship. Keep the reason specific enough that a future reader can
// re-judge it without re-deriving the analysis.
var allowed = map[string]string{
	// x/crypto/openpgp is deprecated and unmaintained (whole-package advisory,
	// no fixed version). It enters transitively through
	// creativeprojects/go-selfupdate's validate.go, which imports it
	// unconditionally; every published go-selfupdate version still does, so no
	// dependency bump clears it. jira-cli's github release-archive updater
	// (internal/selfupdate) verifies assets against GoReleaser's checksums.txt
	// only and never invokes the PGP validation path — the flagged code is
	// reached solely by package init, never called. Revisit if go-selfupdate
	// moves to the ProtonMail fork or gates openpgp behind a build tag.
	"GO-2026-5932": "x/crypto/openpgp deprecation, unreachable PGP path via go-selfupdate (checksum-only verification)",
}

// frame is one node in a finding's call trace. trace[0] is the vulnerable
// symbol itself; a non-empty Function there means govulncheck resolved the
// advisory to a symbol our build reaches (init counts) — the same thing that
// makes the default text format report "your code is affected".
type frame struct {
	Function string `json:"function"`
}

// finding is one govulncheck result. Only symbol-level findings (Trace[0]
// carries a function) affect us; module- and package-level findings without a
// function are informational.
type finding struct {
	OSV   string  `json:"osv"`
	Trace []frame `json:"trace"`
}

// message is one line of the `-format json` stream. Every line is exactly one
// of these fields; we care only about findings.
type message struct {
	Finding *finding `json:"finding"`
}

// fail prints msg on the clog stderr path and exits with code.
func fail(code int, msg string) {
	clog.Error().Parts(clog.PartMessage).Msg(msg)
	os.Exit(code)
}

func main() {
	// govulncheck can walk the whole module; give it room but bound it so a
	// wedged network or filesystem cannot hang CI indefinitely.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// `-format json` exits 0 and streams the verdict; we own the exit code.
	cmd := exec.CommandContext(ctx, "go", "tool", "govulncheck", "-format", "json", "./...")
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		fail(2, "check-vuln: govulncheck failed: "+err.Error()+"\n"+stderr.String())
	}

	// affected maps each advisory that reaches a symbol in our build to a
	// sample function name, for a readable report.
	affected := map[string]string{}
	dec := json.NewDecoder(bufio.NewReader(&stdout))
	for {
		var m message
		if err := dec.Decode(&m); err != nil {
			if err.Error() == "EOF" {
				break
			}
			fail(2, "check-vuln: parsing govulncheck output: "+err.Error())
		}
		if m.Finding == nil || len(m.Finding.Trace) == 0 {
			continue
		}
		if fn := m.Finding.Trace[0].Function; fn != "" {
			if _, seen := affected[m.Finding.OSV]; !seen {
				affected[m.Finding.OSV] = fn
			}
		}
	}

	// A blocking advisory is one that affects us and is not allowlisted.
	var blocking []string
	for id := range affected {
		if _, ok := allowed[id]; !ok {
			blocking = append(blocking, id)
		}
	}
	sort.Strings(blocking)
	if len(blocking) > 0 {
		fail(1, "check-vuln: govulncheck reports advisories affecting this build: "+
			joinIDs(blocking, affected)+
			"\nFix the code, bump the dependency, or (only with a documented reason) add the ID to cmd/check-vuln.")
	}

	// A stale allowlist entry is one we suppress but that no longer fires;
	// remove it so the exception set stays honest.
	var stale []string
	for id := range allowed {
		if _, ok := affected[id]; !ok {
			stale = append(stale, id)
		}
	}
	sort.Strings(stale)
	if len(stale) > 0 {
		fail(1, "check-vuln: allowlisted advisories no longer fire — remove them from cmd/check-vuln: "+
			join(stale))
	}

	// Report what we suppressed so the acceptance is visible in CI logs.
	ids := make([]string, 0, len(allowed))
	for id := range allowed {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		clog.Warn().Parts(clog.PartMessage).Msg("check-vuln: suppressing " + id + " — " + allowed[id])
	}
	clog.Info().Parts(clog.PartMessage).Msg("check-vuln: no unaccepted vulnerabilities")
}

// joinIDs renders each blocking advisory with the sample symbol it reached.
func joinIDs(ids []string, fns map[string]string) string {
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = fmt.Sprintf("%s (reaches %s)", id, fns[id])
	}
	return join(parts)
}

// join renders a comma-separated list.
func join(parts []string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += ", "
		}
		out += p
	}
	return out
}
