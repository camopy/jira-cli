package main

import (
	"context"
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

	if err := Execute(ctx); err != nil {
		os.Exit(exitCodeForError(err))
	}
}
