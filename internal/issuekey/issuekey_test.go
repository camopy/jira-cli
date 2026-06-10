package issuekey

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseExpressionsExpandsListsAndRanges(t *testing.T) {
	tests := []struct {
		name   string
		input  []string
		expect []string
	}{
		{
			name:   "comma separated keys",
			input:  []string{"ABC-1,ABC-2,ABC-3"},
			expect: []string{"ABC-1", "ABC-2", "ABC-3"},
		},
		{
			name:   "comma separated keys can cross projects",
			input:  []string{"ABC-123,XYZ-123"},
			expect: []string{"ABC-123", "XYZ-123"},
		},
		{
			name:   "repeated arguments",
			input:  []string{"ABC-1", "ABC-2", "ABC-3"},
			expect: []string{"ABC-1", "ABC-2", "ABC-3"},
		},
		{
			name:   "colon range with numeric end",
			input:  []string{"ABC-1:3"},
			expect: []string{"ABC-1", "ABC-2", "ABC-3"},
		},
		{
			name:   "colon range with full key end",
			input:  []string{"ABC-1:ABC-3"},
			expect: []string{"ABC-1", "ABC-2", "ABC-3"},
		},
		{
			name:   "double dot range with numeric end",
			input:  []string{"ABC-1..3"},
			expect: []string{"ABC-1", "ABC-2", "ABC-3"},
		},
		{
			name:   "double dot range with full key end",
			input:  []string{"ABC-1..ABC-3"},
			expect: []string{"ABC-1", "ABC-2", "ABC-3"},
		},
		{
			name:   "comma separated ranges can cross projects",
			input:  []string{"ABC-1:3,XYZ-1:2"},
			expect: []string{"ABC-1", "ABC-2", "ABC-3", "XYZ-1", "XYZ-2"},
		},
		{
			name:   "duplicates removed in first seen order",
			input:  []string{"ABC-1,ABC-1,ABC-2..3", "ABC-2"},
			expect: []string{"ABC-1", "ABC-2", "ABC-3"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseExpressions(tt.input, Options{MaxExpansion: 100})
			if err != nil {
				t.Fatalf("ParseExpressions() error = %v", err)
			}
			if !reflect.DeepEqual(got, tt.expect) {
				t.Fatalf("ParseExpressions() = %#v, want %#v", got, tt.expect)
			}
		})
	}
}

func TestParseExpressionsRejectsAmbiguousOrUnsafeInput(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want string
	}{
		{name: "empty list member", in: []string{"ABC-1,,ABC-2"}, want: "empty"},
		{name: "whitespace in list member", in: []string{"ABC-1, ABC-2"}, want: "whitespace"},
		{name: "malformed key", in: []string{"abc-1"}, want: "issue key"},
		{name: "zero issue number", in: []string{"ABC-0"}, want: "positive"},
		{name: "descending range", in: []string{"ABC-3..1"}, want: "descending"},
		{name: "mismatched range projects", in: []string{"ABC-1:XYZ-3"}, want: "same project"},
		{name: "open ended range", in: []string{"ABC-1.."}, want: "range"},
		{name: "range over cap", in: []string{"ABC-1..4"}, want: "exceeds"},
		{name: "large range over cap", in: []string{"ABC-1:1000000"}, want: "exceeds"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseExpressions(tt.in, Options{MaxExpansion: 3})
			if err == nil {
				t.Fatal("ParseExpressions() error = nil")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("ParseExpressions() error = %q, want substring %q", err, tt.want)
			}
		})
	}
}

func TestIsExpression(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want bool
	}{
		{"JCT-1", true},
		{"JCT-1..JCT-5", true},
		{"JCT-1:5", true},
		{"JCT-1,JCT-2", true},
		{"  JCT-3  ", true},
		{"In Progress", false},
		{"Done", false},
		{"To Do", false},
		{"Code Review", false},
		{"31", false},
		{"", false},
	} {
		if got := IsExpression(tc.in); got != tc.want {
			t.Errorf("IsExpression(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}
