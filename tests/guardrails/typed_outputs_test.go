// MOTIVATION: envelope data built as ad-hoc map[string]any let builders,
// schemas, renderers, and docs drift independently — the contract-v2 audit
// found live paths dropping fields their dry-runs carried and schemas
// declaring strings the code emitted as objects. Typed Output structs in
// internal/envelope are the one-declaration fix, but only if every
// operation has one: this guardrail walks the verb registry and fails on
// any operation without a registered typed output (or explicit Dynamic
// exception), so an envelope can never again be born as an untyped map.
// During the migration it doubled as the done-meter, counting down the
// unconverted operations.
package guardrails

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/matcra587/jira-cli/internal/cli"
	"github.com/matcra587/jira-cli/internal/envelope"
)

// nonEnvelopeOps are verb-registry entries that never write an envelope of
// their own: spinner/progress labels and narration steps whose data (if
// any) rides inside another operation's envelope. Additions here need the
// same justification as a nolint — a real envelope op must never hide here.
var nonEnvelopeOps = map[string]string{
	"auth.login.discover": "clog narration label inside auth.login",
	"update.check":        "spinner label inside update",
	"user.resolve":        "spinner label inside issue.watchers.* flows",
}

func TestEveryOperationHasTypedEnvelopeOutput(t *testing.T) {
	registered := envelope.Registered()
	var missing []string
	for _, op := range cli.OperationNames() {
		if _, excluded := nonEnvelopeOps[op]; excluded {
			continue
		}
		if !slices.Contains(registered, op) {
			missing = append(missing, op)
		}
	}
	if len(missing) > 0 {
		t.Fatalf("%d of %d operations lack a typed envelope output in internal/envelope:\n  %s",
			len(missing), len(cli.OperationNames()), strings.Join(missing, "\n  "))
	}
	// The exclusion list must stay honest too: an op listed there but no
	// longer in the verb registry is stale and must be removed.
	for op := range nonEnvelopeOps {
		if !slices.Contains(cli.OperationNames(), op) {
			t.Fatalf("nonEnvelopeOps lists %q but the verb registry does not contain it", op)
		}
	}
}

// envelopeOpString matches the operation-name argument of a cmdutil envelope
// writer at a call site, e.g. `cmdutil.WriteEnvelope(cmd, "issue.edit", …)`.
var envelopeOpString = regexp.MustCompile(`cmdutil\.Write(?:Keyed[A-Za-z]*|Web)?Envelope[A-Za-z]*\(\s*[A-Za-z]+,\s*"([a-z][a-z0-9._-]*[a-z0-9])"[,)]`)

// TestEveryEnvelopeCommandStringIsRegistered widens the inventory beyond the
// verb registry: some envelope ops (alias.*, config.*, cache.clear) have no
// verb entry, so the verb-walk alone cannot see them. This scan extracts
// every literal operation string passed to a cmdutil envelope writer across
// the command packages and requires a typed-output registration for each —
// an envelope op cannot hide outside the registry.
func TestEveryEnvelopeCommandStringIsRegistered(t *testing.T) {
	registered := envelope.Registered()
	found := map[string][]string{}
	root := "../../internal/cli"
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		if strings.Contains(path, "/cmdutil/") {
			return nil // the writer layer itself
		}
		raw, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		for _, match := range envelopeOpString.FindAllStringSubmatch(string(raw), -1) {
			op := match[1]
			found[op] = append(found[op], path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	if len(found) == 0 {
		t.Fatal("scan found no envelope op strings — the regexp or layout drifted")
	}
	var missing []string
	for op, files := range found {
		if !slices.Contains(registered, op) {
			missing = append(missing, op+" ("+files[0]+")")
		}
	}
	if len(missing) > 0 {
		slices.Sort(missing)
		t.Fatalf("%d envelope operations lack a typed-output registration:\n  %s",
			len(missing), strings.Join(missing, "\n  "))
	}
}
