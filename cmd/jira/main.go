package main

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	// One root context for the whole process, canceled on the first
	// SIGINT/SIGTERM so an interrupted command unwinds in-flight HTTP
	// calls, prompts, and credential lookups instead of being killed
	// mid-write.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// main is the sole owner of process exit. Execute builds the runtime
	// and root command, runs the requested command, and either returns
	// nil, returns a command error, or returns errCompletionHandled when
	// a shell-completion preflight request was fully serviced (no command
	// runs). main translates each outcome into an exit code.
	if err := Execute(ctx); err != nil {
		if errors.Is(err, errCompletionHandled) {
			os.Exit(0)
		}
		os.Exit(exitCodeForError(err))
	}
}
