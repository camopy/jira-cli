package main

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"syscall"

	"github.com/matcra587/jira-cli/internal/cli/root"
)

func main() {
	// One root context for the whole process, canceled on the first
	// SIGINT/SIGTERM so an interrupted command unwinds in-flight HTTP
	// calls, prompts, and credential lookups instead of being killed
	// mid-write.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// main is the sole owner of process exit. root.Execute builds the
	// runtime and command tree, runs the requested command, and returns
	// nil, a command error, or ErrCompletionHandled when a shell-completion
	// preflight was fully serviced (no command runs).
	if err := root.Execute(ctx); err != nil {
		if errors.Is(err, root.ErrCompletionHandled) {
			os.Exit(0)
		}
		os.Exit(root.ExitCode(err))
	}
}
