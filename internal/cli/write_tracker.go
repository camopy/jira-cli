package cli

import (
	"errors"
	"io"
	"os"
)

// writeTracker records the first destination failure from output libraries
// whose finalizers do not return write errors. It is command-local and
// intentionally unsynchronized because rendering is sequential.
type writeTracker struct {
	w   io.Writer
	err error
}

func (t *writeTracker) Write(p []byte) (int, error) {
	if t.err != nil {
		return 0, t.err
	}
	n, err := t.w.Write(p)
	if err == nil && n != len(p) {
		err = io.ErrShortWrite
	}
	if err != nil {
		t.err = err
	}
	return n, err
}

// trackedWriter preserves file-descriptor behavior for output libraries that
// use it to detect terminals while still recording destination failures.
func newTrackedWriter(w io.Writer) (*writeTracker, io.Writer) {
	tracker := &writeTracker{w: w}
	if _, ok := w.(interface{ Fd() uintptr }); ok {
		return tracker, fdTrackedWriter{writeTracker: tracker}
	}
	return tracker, tracker
}

func withTrackedWriter(w io.Writer, render func(io.Writer) error) error {
	return TrackWrites(w, render)
}

// TrackWrites runs render with a writer that records the first destination
// failure. A destination failure is returned as an OutputError while any
// renderer error remains separately discoverable.
func TrackWrites(w io.Writer, render func(io.Writer) error) error {
	tracker, out := newTrackedWriter(w)
	renderErr := render(out)
	var outputErr error
	if tracker.err != nil {
		outputErr = NewOutputError(tracker.err)
	}
	return errors.Join(renderErr, outputErr)
}

type fdTrackedWriter struct {
	*writeTracker
}

func (w fdTrackedWriter) Fd() uintptr {
	return w.w.(interface{ Fd() uintptr }).Fd()
}

func (w fdTrackedWriter) outputFile() *os.File {
	if f, ok := w.w.(*os.File); ok {
		return f
	}
	if carrier, ok := w.w.(interface{ outputFile() *os.File }); ok {
		return carrier.outputFile()
	}
	return nil
}

func (w fdTrackedWriter) Read(p []byte) (int, error) {
	if r, ok := w.w.(io.Reader); ok {
		return r.Read(p)
	}
	return 0, os.ErrInvalid
}

func (w fdTrackedWriter) Close() error {
	return nil
}
