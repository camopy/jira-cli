package cmdutil

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/gechr/clog"
	"github.com/spf13/cobra"

	"github.com/matcra587/jira-cli/internal/jira"
)

func TestSpinRunsTaskAndStaysSilentInNonTTY(t *testing.T) {
	cmd := &cobra.Command{Use: "x"}
	cmd.SetContext(context.Background())
	ran := false
	stdout, stderr := captureProcessOutput(t, func() {
		err := Spin(cmd, "cache.boards", func(context.Context) error {
			ran = true
			return nil
		})
		if err != nil {
			t.Fatalf("Spin() error = %v", err)
		}
	})
	if !ran {
		t.Fatal("Spin did not run the task")
	}
	if stdout != "" || stderr != "" {
		t.Fatalf("Spin leaked output in non-TTY: stdout=%q stderr=%q", stdout, stderr)
	}
}

func TestSpinReturnsTaskErrorIncludingAPIError(t *testing.T) {
	cmd := &cobra.Command{Use: "x"}
	cmd.SetContext(context.Background())
	want := &jira.APIError{StatusCode: 403}
	captureProcessOutput(t, func() {
		got := Spin(cmd, "cache.boards", func(context.Context) error { return want })
		if !errors.Is(got, want) {
			t.Fatalf("Spin() error = %v, want %v", got, want)
		}
	})
}

// TestSpinUnderVerboseRunsTaskAndNarrates pins the verbose branch: under
// --debug the animated spinner is suppressed (it would strand its redraw frames
// between the request/response debug lines), but the task still runs and the
// debug lifecycle still narrates start and past-tense completion with a time=
// field. This is the behavior, not the visual artifact — the stranding itself
// only manifests on a real terminal.
func TestSpinUnderVerboseRunsTaskAndNarrates(t *testing.T) {
	var buf bytes.Buffer
	prev := clog.IsVerbose()
	clog.SetVerbose(true)
	clog.SetOutput(clog.NewOutput(&buf, clog.ColorNever))
	t.Cleanup(func() {
		clog.SetVerbose(prev)
		clog.SetOutput(clog.NewOutput(os.Stderr, clog.ColorAuto))
	})

	cmd := &cobra.Command{Use: "x"}
	cmd.SetContext(context.Background())
	ran := false
	if err := Spin(cmd, "cache.boards", func(context.Context) error {
		ran = true
		return nil
	}); err != nil {
		t.Fatalf("Spin() error = %v", err)
	}
	if !ran {
		t.Fatal("Spin did not run the task under verbose")
	}
	out := buf.String()
	if !strings.Contains(out, "cached boards") {
		t.Fatalf("verbose Spin missing past-tense lifecycle; got %q", out)
	}
	if !strings.Contains(out, "time=") {
		t.Fatalf("verbose Spin missing time= field; got %q", out)
	}
}
