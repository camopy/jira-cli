package pipeline_test

import (
	"testing"

	"github.com/matcra587/jira-cli/internal/cli/adfmode"
	"github.com/matcra587/jira-cli/internal/pipeline"
)

// CustomFieldDropPolicy is the single shared decision used by every
// mutation path (create/edit/clone/move): a malformed user-supplied
// custom field is fatal where dropping it would lose input. An opaque
// raw Jira id with no registered type may only be forwarded with a
// warning, and only in best-effort mode.

// A malformed value for a known customfield type is fatal in strict
// mode regardless of which command path supplied it.
func TestCustomFieldPolicyMalformedKnownTypeIsFatalStrict(t *testing.T) {
	decision := pipeline.CustomFieldDropPolicy("customfield_10001", "select", true /*malformed*/, adfmode.ModeStrict)
	if !decision.Fatal {
		t.Fatal("malformed known-type customfield must be fatal in strict mode")
	}
	if decision.Forward {
		t.Fatal("a fatal decision must not also forward the value")
	}
}

// In best-effort mode a malformed known-type customfield is dropped
// with a warning, never forwarded as-is.
func TestCustomFieldPolicyMalformedKnownTypeBestEffortDrops(t *testing.T) {
	decision := pipeline.CustomFieldDropPolicy("customfield_10001", "select", true /*malformed*/, adfmode.ModeBestEffort)
	if decision.Fatal {
		t.Fatal("best-effort must not be fatal on a malformed known-type customfield")
	}
	if decision.Forward {
		t.Fatal("malformed value must be dropped, not forwarded")
	}
	if decision.Warning == "" {
		t.Fatal("best-effort drop must carry a warning")
	}
}

// An opaque raw Jira id (no registered type) is forwarded with a
// warning in best-effort mode.
func TestCustomFieldPolicyOpaqueForwardedBestEffort(t *testing.T) {
	decision := pipeline.CustomFieldDropPolicy("customfield_99999", "" /*unknown type*/, false /*not malformed*/, adfmode.ModeBestEffort)
	if decision.Fatal {
		t.Fatal("opaque customfield must not be fatal in best-effort mode")
	}
	if !decision.Forward {
		t.Fatal("opaque customfield must be forwarded in best-effort mode")
	}
	if decision.Warning == "" {
		t.Fatal("forwarded opaque customfield must carry a warning")
	}
}

// In strict mode an opaque raw Jira id with an unknown type is
// forwarded opaquely with a warning — the CLI cannot validate a
// vendor/marketplace customfield shape, and forwarding-with-warning is
// the shipped contract. It is never silently dropped (that would lose
// user input) and never fatal on the absence of a known type alone.
func TestCustomFieldPolicyOpaqueForwardedStrict(t *testing.T) {
	decision := pipeline.CustomFieldDropPolicy("customfield_99999", "", false, adfmode.ModeStrict)
	if decision.Fatal {
		t.Fatal("opaque customfield must not be fatal solely for an unknown type")
	}
	if !decision.Forward {
		t.Fatal("opaque customfield must be forwarded, never silently dropped")
	}
	if decision.Warning == "" {
		t.Fatal("forwarded opaque customfield must carry a warning")
	}
}
