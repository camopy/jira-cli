package editor

// Known-non-blocking-editor refusal: these binaries fork a child
// process and return immediately by default, racing the parent's
// tempfile cleanup. Run() must refuse to spawn them without a wait
// flag and direct the user to the canonical fix.
//
// We test refuseIfNonBlocking directly rather than driving Run() —
// driving Run() would actually spawn `code`/`vim` against a real path
// (`code -w` is blocking, so the test would hang on the user's open
// VS Code instance). The gate is a pure function of the parsed argv,
// so unit-testing it in isolation is both safer and more precise.

import (
	"strings"
	"testing"
)

func TestRefuseIfNonBlockingDetectsAllKnownEditors(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name             string
		parts            []string
		wantErrSubstring []string
	}{
		{
			name:             "vanilla code (no flags)",
			parts:            []string{"code"},
			wantErrSubstring: []string{"non-blocking", "code", "--wait"},
		},
		{
			name:             "code-insiders bare",
			parts:            []string{"code-insiders"},
			wantErrSubstring: []string{"non-blocking", "code-insiders", "--wait"},
		},
		{
			name:             "subl bare",
			parts:            []string{"subl"},
			wantErrSubstring: []string{"non-blocking", "subl", "--wait"},
		},
		{
			name:             "mate bare",
			parts:            []string{"mate"},
			wantErrSubstring: []string{"non-blocking", "mate", "--wait"},
		},
		{
			name:             "gvim bare (forks by default)",
			parts:            []string{"gvim"},
			wantErrSubstring: []string{"non-blocking", "gvim", "-f"},
		},
		{
			name:             "absolute path bare",
			parts:            []string{"/usr/bin/code"},
			wantErrSubstring: []string{"non-blocking", "code", "--wait"},
		},
		{
			name:             "binary with .exe suffix bare",
			parts:            []string{"code.exe"},
			wantErrSubstring: []string{"non-blocking", "code", "--wait"},
		},
		{
			name:             "code with unrelated flag (no wait)",
			parts:            []string{"code", "--new-window"},
			wantErrSubstring: []string{"non-blocking", "code", "--wait"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := refuseIfNonBlocking(tc.parts)
			if err == nil {
				t.Fatalf("refuseIfNonBlocking(%v) = nil; want refusal", tc.parts)
			}
			for _, want := range tc.wantErrSubstring {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error missing %q: %v", want, err)
				}
			}
		})
	}
}

func TestRefuseIfNonBlockingAcceptsWaitFlags(t *testing.T) {
	t.Parallel()

	cases := [][]string{
		{"code", "--wait"},
		{"code", "-w"},
		{"code-insiders", "--wait"},
		{"subl", "-w"},
		{"mate", "--wait"},
		{"gvim", "-f"},
		{"gvim", "--nofork"},
		{"/usr/bin/code", "--wait"},
		{"code", "--new-window", "--wait"}, // wait flag mixed with others
	}
	for _, parts := range cases {
		t.Run(strings.Join(parts, " "), func(t *testing.T) {
			t.Parallel()
			if err := refuseIfNonBlocking(parts); err != nil {
				t.Errorf("refuseIfNonBlocking(%v) = %v; want nil (wait flag present)", parts, err)
			}
		})
	}
}

func TestRefuseIfNonBlockingPassesUnknownEditors(t *testing.T) {
	t.Parallel()
	// Editors not in the catalog (vi, vim, nano, emacs, etc.) must
	// pass straight through — they're either truly blocking, or the
	// timing-based safety net in EditMarkdown will catch them.
	cases := [][]string{
		{"vi"},
		{"vim"},
		{"nano"},
		{"emacs"},
		{"/usr/bin/nano"},
		{"hx"},
		{"helix"},
		{"micro"},
	}
	for _, parts := range cases {
		t.Run(parts[0], func(t *testing.T) {
			t.Parallel()
			if err := refuseIfNonBlocking(parts); err != nil {
				t.Errorf("refuseIfNonBlocking(%v) = %v; want nil (not cataloged)", parts, err)
			}
		})
	}
}

func TestRefuseIfNonBlockingHandlesEmptyArgs(t *testing.T) {
	t.Parallel()
	if err := refuseIfNonBlocking(nil); err != nil {
		t.Errorf("refuseIfNonBlocking(nil) = %v; want nil", err)
	}
	if err := refuseIfNonBlocking([]string{}); err != nil {
		t.Errorf("refuseIfNonBlocking([]) = %v; want nil", err)
	}
}
