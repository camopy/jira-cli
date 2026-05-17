// Package runtime is the dependency boundary between the binary shell in
// cmd/jira and the command implementations. It carries the IO streams a
// command invocation needs that are otherwise process-global: stdout,
// stderr, and stdin.
//
// Runtime is constructed once per process by main (and once per test by
// the cmd/jira test helper). It is deliberately small: it owns inputs,
// not behavior. It does NOT store a context.Context — main owns the root
// context via signal.NotifyContext and threads it through ExecuteContext.
// Runtime never calls os.Exit and exposes no broad service-locator
// surface; command-domain logic lives in the command packages.
//
// TTY detection is derived from the stdout stream (an *os.File in
// production) rather than carried as a separate field — see
// runtimeStdoutIsTTY in cmd/jira. Environment, config-path, timeout,
// clock, and request-ID inputs are intentionally absent: their consumers
// are not yet wired through the runtime, and an option that does not
// reach its consumer is a correctness trap. They return to this boundary
// when each one is wired to its consumer.
package runtime

import (
	"io"
	"os"
)

// Runtime holds the per-invocation IO streams shared across the command
// tree. All fields have safe defaults applied by New; callers override
// them with Option values.
type Runtime struct {
	stdout io.Writer
	stderr io.Writer
	stdin  io.Reader
}

// Option configures a Runtime during construction. An Option returns an
// error to reject invalid input (e.g. a nil writer).
type Option func(*Runtime) error

// New builds a Runtime, applying defaults first and then each Option in
// order. Defaults wire the process streams so a zero-Option call yields a
// production-ready runtime.
func New(options ...Option) (*Runtime, error) {
	rt := &Runtime{
		stdout: os.Stdout,
		stderr: os.Stderr,
		stdin:  os.Stdin,
	}
	for _, opt := range options {
		if opt == nil {
			continue
		}
		if err := opt(rt); err != nil {
			return nil, err
		}
	}
	return rt, nil
}

// Stdout returns the writer commands render successful output to.
func (rt *Runtime) Stdout() io.Writer { return rt.stdout }

// Stderr returns the writer commands render diagnostics to.
func (rt *Runtime) Stderr() io.Writer { return rt.stderr }

// Stdin returns the reader commands consume piped input from.
func (rt *Runtime) Stdin() io.Reader { return rt.stdin }
