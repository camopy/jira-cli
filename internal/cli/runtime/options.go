package runtime

import (
	"errors"
	"io"
)

// WithStdout sets the writer commands render successful output to. A nil
// writer is rejected so a misconfigured runtime fails fast at New rather
// than panicking mid-command.
func WithStdout(w io.Writer) Option {
	return func(rt *Runtime) error {
		if w == nil {
			return errors.New("runtime: stdout writer must not be nil")
		}
		rt.stdout = w
		return nil
	}
}

// WithStderr sets the writer commands render diagnostics to.
func WithStderr(w io.Writer) Option {
	return func(rt *Runtime) error {
		if w == nil {
			return errors.New("runtime: stderr writer must not be nil")
		}
		rt.stderr = w
		return nil
	}
}

// WithStdin sets the reader commands consume piped input from.
func WithStdin(r io.Reader) Option {
	return func(rt *Runtime) error {
		if r == nil {
			return errors.New("runtime: stdin reader must not be nil")
		}
		rt.stdin = r
		return nil
	}
}
