package cli

import (
	"errors"
	"io"
	"testing"
)

func TestRouteWarningsReturnsWriterFailure(t *testing.T) {
	err := RouteWarnings(RouteOptions{
		Stdout:  &alwaysFailWriter{},
		Stderr:  &alwaysFailWriter{},
		Mode:    RoutePlain,
		Command: "auth.whoami",
		Data:    map[string]any{"account": "u@example.com"},
		Warnings: []Warning{{
			Type:    "test_warning",
			Message: "warning",
		}},
	})
	if !errors.Is(err, errWriteSentinel) {
		t.Fatalf("RouteWarnings() error = %v, want sentinel", err)
	}
}

func TestRouteWarningsReturnsWarningWriterFailure(t *testing.T) {
	err := RouteWarnings(RouteOptions{
		Stdout:  io.Discard,
		Stderr:  &alwaysFailWriter{},
		Mode:    RoutePlain,
		Command: "auth.whoami",
		Data:    map[string]any{"account": "u@example.com"},
		Warnings: []Warning{{
			Type:    "test_warning",
			Message: "warning",
		}},
	})
	if !errors.Is(err, errWriteSentinel) {
		t.Fatalf("RouteWarnings() error = %v, want sentinel", err)
	}
}
