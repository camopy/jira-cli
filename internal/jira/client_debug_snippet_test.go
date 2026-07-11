package jira

import (
	"strings"
	"testing"
)

// Debug snippets render server-controlled text (response bodies, the
// RateLimit-Reason header) to stderr, so oneLineSnippet strips ANSI
// escapes and C0/C1 control runes in addition to flattening whitespace.
func TestOneLineSnippetStripsServerControlledEscapes(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "csi and osc sequences stripped",
			in:   "ok \x1b[31mred\x1b[0m \x1b]0;owned\x07tail",
			want: "ok red tail",
		},
		{
			name: "c1 csi and del runes dropped in place",
			in:   "del\u007fete tail\u009b",
			want: "delete tail",
		},
		{
			name: "bell backspace and bare cr dropped, crlf collapses",
			in:   "a\x07b\x08c\rd\r\ne",
			want: "abcd e",
		},
		{
			name: "whitespace flattened and truncated",
			in:   "  one\ttwo\nthree  ",
			want: "one two three",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := oneLineSnippet(tt.in, 2048)
			if got != tt.want {
				t.Fatalf("oneLineSnippet(%q) = %q, want %q", tt.in, got, tt.want)
			}
			if strings.ContainsAny(got, "\x1b\x07\x08\r\u007f") || strings.Contains(got, "\u009b") {
				t.Fatalf("oneLineSnippet(%q) leaked a control byte: %q", tt.in, got)
			}
		})
	}
}
