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
