package update

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/matcra587/jira-cli/internal/cli"
	"github.com/matcra587/jira-cli/internal/cli/cmdutil"
	"github.com/matcra587/jira-cli/internal/selfupdate"
)

// newTestCommand builds an update command attached to a minimal root so the
// headless gate can read the root --no-input persistent flag, with the given
// detection seeded on the context.
func newTestCommand(t *testing.T, det cli.Detection) *cobra.Command {
	t.Helper()
	root := &cobra.Command{Use: "jira"}
	root.PersistentFlags().Bool("no-input", false, "")
	cmd := NewCommand()
	root.AddCommand(cmd)
	cmd.SetContext(cmdutil.WithDetector(t.Context(), det))
	return cmd
}

func stubChannel(t *testing.T, ch selfupdate.Channel) {
	t.Helper()
	prev := detectChannel
	detectChannel = func() selfupdate.Channel { return ch }
	t.Cleanup(func() { detectChannel = prev })
}

// fakeUpdater stubs the archive/brew backend so the envelope paths run
// without a release or a writable install directory.
type fakeUpdater struct {
	latest  string
	updated bool
}

func (f *fakeUpdater) Latest(context.Context) (string, error) { return f.latest, nil }
func (f *fakeUpdater) Update(context.Context) error           { f.updated = true; return nil }

// stubUpdater pins the channel, current version, and backend in one call and
// returns the fake for post-run assertions.
func stubUpdater(t *testing.T, ch selfupdate.Channel, current, latest string) *fakeUpdater {
	t.Helper()
	stubChannel(t, ch)
	prevVersion := currentVersion
	currentVersion = func() string { return current }
	fake := &fakeUpdater{latest: latest}
	prevNew := newUpdater
	newUpdater = func(selfupdate.Channel) (updater, error) { return fake, nil }
	t.Cleanup(func() {
		currentVersion = prevVersion
		newUpdater = prevNew
	})
	return fake
}

// runJSON executes run() with a JSON-mode detector and decodes the envelope
// written to stdout.
func runJSON(t *testing.T, dryRun, force bool) (map[string]any, *bytes.Buffer) {
	t.Helper()
	cmd := newTestCommand(t, cli.Detection{IsTTY: false, Mode: cli.ModeJSON})
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := run(cmd, dryRun, force); err != nil {
		t.Fatalf("run() error = %v", err)
	}
	var env map[string]any
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("stdout is not a JSON envelope: %v\nout=%s", err, out.String())
	}
	return env, &out
}

func TestRunDryRunReportsUpdateAvailableWithoutInstalling(t *testing.T) {
	fake := stubUpdater(t, selfupdate.ChannelArchive, "v1.0.0", "v2.0.0")
	env, _ := runJSON(t, true, false)
	data, _ := env["data"].(map[string]any)
	if data["update_available"] != true || data["updated"] != false || data["dry_run"] != true {
		t.Errorf("data = %v, want update_available=true updated=false dry_run=true", data)
	}
	if fake.updated {
		t.Error("dry-run must not install")
	}
}

func TestRunUpToDateSkipsInstall(t *testing.T) {
	fake := stubUpdater(t, selfupdate.ChannelArchive, "v2.0.0", "v2.0.0")
	env, _ := runJSON(t, false, true)
	data, _ := env["data"].(map[string]any)
	if data["update_available"] != false || data["updated"] != false {
		t.Errorf("data = %v, want update_available=false updated=false", data)
	}
	if fake.updated {
		t.Error("up-to-date must not install")
	}
}

func TestRunForcedUpdateInstallsAndReports(t *testing.T) {
	fake := stubUpdater(t, selfupdate.ChannelArchive, "v1.0.0", "v2.0.0")
	env, _ := runJSON(t, false, true)
	data, _ := env["data"].(map[string]any)
	if data["updated"] != true {
		t.Errorf("data = %v, want updated=true", data)
	}
	if !fake.updated {
		t.Error("forced update must call the backend")
	}
}

func TestRunUnknownChannelFailsWithGuidance(t *testing.T) {
	stubChannel(t, selfupdate.ChannelUnknown)
	cmd := newTestCommand(t, cli.Detection{IsTTY: true})

	err := run(cmd, false, false)
	if err == nil {
		t.Fatal("run() = nil, want install-channel error")
	}
	if !strings.Contains(err.Error(), "install channel") {
		t.Errorf("run() error = %q, want install-channel guidance", err)
	}
	if !strings.Contains(err.Error(), selfupdate.GoInstallHint) {
		t.Errorf("run() error = %q, want the go install hint", err)
	}
}

func TestRunLiveArchiveUpdateRequiresForceWhenHeadless(t *testing.T) {
	stubChannel(t, selfupdate.ChannelArchive)
	for name, det := range map[string]cli.Detection{
		"agent":   {IsTTY: true, Agent: true},
		"non-tty": {IsTTY: false},
	} {
		t.Run(name, func(t *testing.T) {
			cmd := newTestCommand(t, det)
			err := run(cmd, false, false)
			if err == nil || !strings.Contains(err.Error(), "--force") {
				t.Errorf("run() error = %v, want --force requirement", err)
			}
		})
	}
}
