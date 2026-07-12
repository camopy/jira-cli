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
