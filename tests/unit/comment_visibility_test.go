package unit

import (
	"strings"
	"testing"

	"github.com/matcra587/jira-cli/internal/jira"
)

// Visibility flag-combination semantics.
//
// The CLI maps three flags onto a single VisibilityChange value:
//   --visibility-role X    → Replace(Role, X)
//   --visibility-group Y   → Replace(Group, Y)
//   --clear-visibility     → Clear
//   none of the three      → Keep (preserve-when-omitted)
//
// Any combination of two or more is mutually exclusive (validation, exit 3).
//
// This file pins the behavior at the type layer so the cmd binding can rely
// on a single tested function. ParseVisibilityChange is the seam: it accepts
// the raw flag values and the cmd.Changed() booleans (so an empty string
// passed positionally distinguishes from an unset flag), and returns either
// a VisibilityChange or an error.

func TestParseVisibilityChangePreserveWhenAllOmitted(t *testing.T) {
	got, err := jira.ParseVisibilityChange(jira.VisibilityFlags{})
	if err != nil {
		t.Fatalf("err = %v; want nil", err)
	}
	if got.Mode != jira.VisibilityKeep {
		t.Errorf("mode = %v; want VisibilityKeep", got.Mode)
	}
}

func TestParseVisibilityChangeReplaceRole(t *testing.T) {
	got, err := jira.ParseVisibilityChange(jira.VisibilityFlags{
		RoleSet: true, Role: "Developers",
	})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if got.Mode != jira.VisibilityReplace {
		t.Errorf("mode = %v; want VisibilityReplace", got.Mode)
	}
	if got.Type != "role" || got.Value != "Developers" {
		t.Errorf("got = %+v; want {role, Developers}", got)
	}
}

func TestParseVisibilityChangeReplaceGroup(t *testing.T) {
	got, err := jira.ParseVisibilityChange(jira.VisibilityFlags{
		GroupSet: true, Group: "Eng",
	})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if got.Mode != jira.VisibilityReplace {
		t.Errorf("mode = %v; want VisibilityReplace", got.Mode)
	}
	if got.Type != "group" || got.Value != "Eng" {
		t.Errorf("got = %+v; want {group, Eng}", got)
	}
}

func TestParseVisibilityChangeClear(t *testing.T) {
	got, err := jira.ParseVisibilityChange(jira.VisibilityFlags{Clear: true})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if got.Mode != jira.VisibilityClear {
		t.Errorf("mode = %v; want VisibilityClear", got.Mode)
	}
}

func TestParseVisibilityChangeRoleAndGroupMutuallyExclusive(t *testing.T) {
	_, err := jira.ParseVisibilityChange(jira.VisibilityFlags{
		RoleSet: true, Role: "R", GroupSet: true, Group: "G",
	})
	if err == nil {
		t.Fatal("err = nil; want validation error")
	}
	if !strings.Contains(err.Error(), "visibility-role") || !strings.Contains(err.Error(), "visibility-group") {
		t.Errorf("err = %v; want a message naming both flags", err)
	}
}

func TestParseVisibilityChangeRoleAndClearMutuallyExclusive(t *testing.T) {
	_, err := jira.ParseVisibilityChange(jira.VisibilityFlags{
		RoleSet: true, Role: "R", Clear: true,
	})
	if err == nil {
		t.Fatal("err = nil; want validation error")
	}
	if !strings.Contains(err.Error(), "clear-visibility") {
		t.Errorf("err = %v; want a message naming clear-visibility", err)
	}
}

func TestParseVisibilityChangeGroupAndClearMutuallyExclusive(t *testing.T) {
	_, err := jira.ParseVisibilityChange(jira.VisibilityFlags{
		GroupSet: true, Group: "G", Clear: true,
	})
	if err == nil {
		t.Fatal("err = nil; want validation error")
	}
}

func TestParseVisibilityChangeRoleSetButEmptyValueIsValidationError(t *testing.T) {
	// `--visibility-role ""` is unambiguously broken — no role name to send.
	_, err := jira.ParseVisibilityChange(jira.VisibilityFlags{RoleSet: true, Role: ""})
	if err == nil {
		t.Fatal("err = nil; want error on empty role value")
	}
}
