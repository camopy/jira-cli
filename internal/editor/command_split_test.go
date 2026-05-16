package editor

import (
	"slices"
	"testing"
)

// splitEditorCommand must parse the editor command line with shell
// grammar: a quoted path with spaces is one argument, and quoted flag
// values survive. strings.Fields would shatter "/opt/My Editor/bin"
// into three tokens and try to exec "/opt/My".
func TestSplitEditorCommandHonorsQuoting(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{
			"plain binary",
			"vim",
			[]string{"vim"},
		},
		{
			"binary with flag",
			"code --wait",
			[]string{"code", "--wait"},
		},
		{
			"quoted path with spaces",
			`"/opt/My Editor/bin/edit" --wait`,
			[]string{"/opt/My Editor/bin/edit", "--wait"},
		},
		{
			"single-quoted argument",
			`emacs --eval '(message "hi there")'`,
			[]string{"emacs", "--eval", `(message "hi there")`},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := splitEditorCommand(tc.in)
			if err != nil {
				t.Fatalf("splitEditorCommand(%q) error = %v", tc.in, err)
			}
			if !slices.Equal(got, tc.want) {
				t.Fatalf("splitEditorCommand(%q) = %#v, want %#v", tc.in, got, tc.want)
			}
		})
	}
}

// An unterminated quote is a malformed editor command — it must error
// rather than silently exec a truncated argument list.
func TestSplitEditorCommandRejectsUnterminatedQuote(t *testing.T) {
	if _, err := splitEditorCommand(`code --wait "/opt/Unclosed`); err == nil {
		t.Fatal("splitEditorCommand accepted an unterminated quote; want an error")
	}
}
