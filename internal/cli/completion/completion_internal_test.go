package completion

import (
	"bytes"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/matcra587/jira-cli/internal/cli"
	"github.com/matcra587/jira-cli/internal/cli/startup"
)

type completionErrorWriter struct {
	err    error
	writes int
}

func (w *completionErrorWriter) Write([]byte) (int, error) {
	w.writes++
	return 0, w.err
}

func TestUniqueCachedNames(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   []namedCacheValue
		want []string
	}{
		{
			name: "collapses per-workflow duplicates in first-seen order",
			in: []namedCacheValue{
				{Name: "To Do"},
				{Name: "In Progress"},
				{Name: "Done"},
				{Name: "To Do"},
				{Name: "In Progress"},
				{Name: "Done"},
				{Name: "To Do"},
			},
			want: []string{"To Do", "In Progress", "Done"},
		},
		{
			name: "drops blank names",
			in:   []namedCacheValue{{Name: "High"}, {Name: ""}, {Name: "Low"}},
			want: []string{"High", "Low"},
		},
		{
			name: "passes a unique list through unchanged",
			in:   []namedCacheValue{{Name: "Highest"}, {Name: "High"}, {Name: "Medium"}},
			want: []string{"Highest", "High", "Medium"},
		},
		{
			name: "empty input yields an empty, non-nil slice",
			in:   nil,
			want: []string{},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := uniqueCachedNames(tc.in)
			if !slices.Equal(got, tc.want) {
				t.Fatalf("uniqueCachedNames = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestHandlerWritesDynamicCandidatesToInjectedWriter(t *testing.T) {
	var stdout bytes.Buffer
	handler := NewHandler(&stdout, startup.Globals{})

	handler.Complete("fish", "cacheresource", nil)
	if err := handler.Err(); err != nil {
		t.Fatalf("handler.Err() = %v", err)
	}
	got := stdout.String()
	if got == "" {
		t.Fatal("dynamic completion emitted no candidates")
	}
	for _, line := range strings.Split(strings.TrimSpace(got), "\n") {
		if strings.ContainsAny(line, "\r\t") {
			t.Fatalf("dynamic completion emitted non-candidate text %q", line)
		}
	}
}

func TestHandlerReturnsFirstDynamicCandidateWriteFailure(t *testing.T) {
	writeErr := errors.New("stdout closed")
	stdout := &completionErrorWriter{err: writeErr}
	handler := NewHandler(stdout, startup.Globals{})

	handler.Complete("fish", "cacheresource", nil)
	if !errors.Is(handler.Err(), writeErr) {
		t.Fatalf("handler.Err() = %v, want writer failure", handler.Err())
	}
	var outputErr *cli.OutputError
	if !errors.As(handler.Err(), &outputErr) {
		t.Fatalf("handler.Err() type = %T, want *cli.OutputError", handler.Err())
	}
	if stdout.writes != 1 {
		t.Fatalf("stdout writes = %d, want one attempt after the first failure", stdout.writes)
	}
}
