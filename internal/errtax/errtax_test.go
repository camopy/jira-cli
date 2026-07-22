package errtax_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/matcra587/jira-cli/internal/errtax"
)

// genericHint mirrors the package's fail-closed fallback; frozen here so a
// reworded fallback is a conscious, test-visible change.
const genericHint = "Rerun with `--debug` and report the failure if it persists."

func TestLookup(t *testing.T) {
	t.Parallel()
	t.Run("known code returns its row", func(t *testing.T) {
		t.Parallel()
		spec, ok := errtax.Lookup(errtax.CodeReadOnly)
		if !ok {
			t.Fatal("read_only is not registered")
		}
		if spec.Type != errtax.TypeValidation || spec.Exit != 3 {
			t.Errorf("read_only spec = %+v, want validation/3", spec)
		}
	})
	t.Run("unknown code misses", func(t *testing.T) {
		t.Parallel()
		if _, ok := errtax.Lookup(errtax.Code("no_such_code")); ok {
			t.Error("unregistered code reported as registered")
		}
	})
	t.Run("zero value misses", func(t *testing.T) {
		t.Parallel()
		if _, ok := errtax.Lookup(errtax.CodeUnknown); ok {
			t.Error("CodeUnknown must miss so unclassified errors fail closed")
		}
	})
	t.Run("canceled and timeout pin custom exits", func(t *testing.T) {
		t.Parallel()
		if spec, _ := errtax.Lookup(errtax.CodeCanceled); spec.Exit != 6 || !spec.Retryable {
			t.Errorf("canceled spec = %+v, want exit 6 retryable", spec)
		}
		if spec, _ := errtax.Lookup(errtax.CodeTimeout); spec.Exit != 7 || !spec.Retryable {
			t.Errorf("timeout spec = %+v, want exit 7 retryable", spec)
		}
	})
}

// TestHintTotality is red-line #2 as a registry property: every registered
// code carries a non-empty canonical hint.
func TestHintTotality(t *testing.T) {
	t.Parallel()
	for _, code := range errtax.Codes() {
		spec, ok := errtax.Lookup(code)
		if !ok {
			t.Fatalf("Codes() returned unregistered code %q", code)
		}
		if spec.Hint == "" {
			t.Errorf("code %q has an empty registry hint", code)
		}
		if spec.Type == errtax.TypeUnknown {
			t.Errorf("code %q has no type", code)
		}
		if spec.Exit == 0 {
			t.Errorf("code %q has exit 0", code)
		}
	}
}

// TestHintsAvoidEnvelopeJargon enforces the CONTRIBUTING hint rule: a hint
// renders to a human who cannot see the JSON envelope, so it must never point
// at an envelope-only field name. Adding a hint that references one (e.g.
// "pick from candidates[]") fails the build — put the specific in the error
// message or a structured field instead.
func TestHintsAvoidEnvelopeJargon(t *testing.T) {
	t.Parallel()
	banned := []string{
		"candidates[", "suggestions[", "upstream_",
		"retry_after_seconds", "rate_limit_remaining", "http_status",
	}
	for _, code := range errtax.Codes() {
		hint := errtax.HintFor(code)
		for _, tok := range banned {
			if strings.Contains(hint, tok) {
				t.Errorf("code %q hint names envelope-only field %q; put the specific in the message or a structured field instead: %q", code, tok, hint)
			}
		}
	}
}

// TestSharedNotFoundHintIsResourceNeutral guards the catch-all 404 hint.
// CodeJiraNotFound is the code EVERY Jira 404 maps to — bad board ids,
// attachment ids, account ids, comment/link ids, project keys, not just issue
// keys. Naming one resource in its hint misleads on all the others, so the
// shared hint must stay resource-neutral; per-resource specifics belong in the
// error message.
func TestSharedNotFoundHintIsResourceNeutral(t *testing.T) {
	t.Parallel()
	hint := strings.ToLower(errtax.HintFor(errtax.CodeJiraNotFound))
	for _, tok := range []string{"issue key", "board", "attachment", "comment", "link", "project key", "account id"} {
		if strings.Contains(hint, tok) {
			t.Errorf("CodeJiraNotFound hint names the specific resource %q but the code fires for every 404; keep it resource-neutral: %q", tok, hint)
		}
	}
}

func TestHintFor(t *testing.T) {
	t.Parallel()
	t.Run("registered code returns its hint", func(t *testing.T) {
		t.Parallel()
		spec, _ := errtax.Lookup(errtax.CodeValidationFailed)
		if got := errtax.HintFor(errtax.CodeValidationFailed); got != spec.Hint {
			t.Errorf("HintFor = %q, want the registry hint %q", got, spec.Hint)
		}
	})
	t.Run("unregistered code returns the exact generic fallback", func(t *testing.T) {
		t.Parallel()
		if got := errtax.HintFor(errtax.CodeUnknown); got != genericHint {
			t.Errorf("HintFor(CodeUnknown) = %q, want %q", got, genericHint)
		}
	})
}

func TestExitFor(t *testing.T) {
	t.Parallel()
	cases := map[errtax.Type]int{
		errtax.TypeAuth:       1,
		errtax.TypeNotFound:   2,
		errtax.TypeValidation: 3,
		errtax.TypeRateLimit:  4,
		errtax.TypeServer:     5,
		errtax.TypeUnknown:    5,
	}
	for typ, want := range cases {
		if got := errtax.ExitFor(typ); got != want {
			t.Errorf("ExitFor(%q) = %d, want %d", typ, got, want)
		}
	}
}

func TestDefaultCode(t *testing.T) {
	t.Parallel()
	cases := map[errtax.Type]errtax.Code{
		errtax.TypeAuth:       errtax.CodeAuthFailed,
		errtax.TypeNotFound:   errtax.CodeNotFound,
		errtax.TypeValidation: errtax.CodeValidationFailed,
		errtax.TypeRateLimit:  errtax.CodeRateLimited,
		errtax.TypeServer:     errtax.CodeServerError,
		errtax.TypeUnknown:    errtax.CodeServerError,
	}
	for typ, want := range cases {
		if got := errtax.DefaultCode(typ); got != want {
			t.Errorf("DefaultCode(%q) = %q, want %q", typ, got, want)
		}
		// A type's default code must resolve back to that type in the
		// registry, or the fallback path would relabel the failure.
		spec, ok := errtax.Lookup(errtax.DefaultCode(typ))
		if !ok {
			t.Errorf("DefaultCode(%q) = %q is not registered", typ, errtax.DefaultCode(typ))
		} else if typ != errtax.TypeUnknown && spec.Type != typ {
			t.Errorf("DefaultCode(%q) resolves to registry type %q", typ, spec.Type)
		}
	}
}

func TestCodes(t *testing.T) {
	t.Parallel()
	codes := errtax.Codes()
	if codes == nil {
		t.Fatal("Codes() returned nil")
	}
	if !slices.IsSorted(codes) {
		t.Error("Codes() is not sorted")
	}
	if dup := slices.Compact(slices.Clone(codes)); len(dup) != len(codes) {
		t.Error("Codes() contains duplicates")
	}
	// The registered code count moves only when the taxonomy contract does.
	if len(codes) != 60 {
		t.Errorf("registry has %d codes, want 60 — update the contract and this count together", len(codes))
	}
	// Codes() must be a fresh allocation each call: sorting or mutating one
	// return value must not affect the next.
	first := errtax.Codes()
	first[0] = errtax.Code("mutated")
	if second := errtax.Codes(); second[0] == errtax.Code("mutated") {
		t.Error("Codes() shares a backing array across calls")
	}
}
