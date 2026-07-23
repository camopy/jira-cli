package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	xerrors "github.com/gechr/x/errors"
	"github.com/itchyny/gojq"

	"github.com/matcra587/jira-cli/internal/errtax"
)

// resolvedJQ is the compiled --jq program for this invocation, set once by
// root's PersistentPreRunE like resolvedColorMode: the machine-output
// writers below sit under package cli where no *cobra.Command is in scope,
// so the resolved filter is package state, cleared between invocations by
// the next resolve.
var resolvedJQ *gojq.Code

// resolvedJQCtx is the command context the filter runs under, captured at
// resolve time with the program. gojq only honors cancellation through
// RunWithContext with a non-background context, so without this a
// user-supplied infinite expression would ignore --timeout and survive
// SIGTERM.
var resolvedJQCtx context.Context

// SetJQProgram installs the invocation's compiled --jq filter and the
// context it runs under.
func SetJQProgram(ctx context.Context, code *gojq.Code) {
	resolvedJQCtx = ctx
	resolvedJQ = code
}

// ClearJQProgram removes any active filter — the fresh-invocation reset,
// and the disarm writeJQ performs before reporting a runtime failure so
// the resulting error envelope prints unfiltered.
func ClearJQProgram() {
	resolvedJQCtx = nil
	resolvedJQ = nil
}

// JQEnabled reports whether a --jq filter is active for this invocation.
func JQEnabled() bool { return resolvedJQ != nil }

// CompileJQ parses and compiles a --jq expression. Failures come back as
// the typed input error (exit 3, code jq_expression_invalid) with gojq's
// position-carrying message.
func CompileJQ(expr string) (*gojq.Code, error) {
	query, err := gojq.Parse(expr)
	if err != nil {
		return nil, NewCLIInputError(InputJQExpressionInvalid, fmt.Sprintf("invalid --jq expression: %v", err))
	}
	code, err := gojq.Compile(query)
	if err != nil {
		return nil, NewCLIInputError(InputJQExpressionInvalid, fmt.Sprintf("invalid --jq expression: %v", err))
	}
	return code, nil
}

// JQEvalError is a --jq expression that compiled but failed while running
// against the emitted document — a type error, an error/1 call, a
// halt_error. It is validation (exit 3): the document was produced fine,
// the filter did not fit it.
type JQEvalError struct {
	Err error
}

func (e *JQEvalError) Error() string     { return "jq: " + e.Err.Error() }
func (e *JQEvalError) Unwrap() error     { return e.Err }
func (e *JQEvalError) Code() errtax.Code { return errtax.CodeJQEvalFailed }

var _ errtax.Coded = (*JQEvalError)(nil)

// writeJQ runs the invocation's compiled filter over doc and writes the
// results to w: string values print raw (jq -r style), every other value
// prints as jq-flavored JSON, one result per line. doc round-trips through
// encoding/json first because gojq accepts only plain JSON types while
// envelopes carry typed structs. A halt(0)/empty stream writes nothing —
// that is the expression's prerogative.
func writeJQ(w io.Writer, doc any) error {
	raw, err := json.Marshal(doc)
	if err != nil {
		return err
	}
	var input any
	if err := json.Unmarshal(raw, &input); err != nil {
		return err
	}
	ctx := resolvedJQCtx
	if ctx == nil {
		ctx = context.Background()
	}
	// Results buffer until the iterator completes cleanly: gojq is lazy, so
	// an expression that emits then errors would otherwise leave partial
	// filtered lines on stdout ahead of the error envelope — neither clean
	// output nor a clean envelope. Envelopes are small; buffering is cheap.
	var buf bytes.Buffer
	iter := resolvedJQ.RunWithContext(ctx, input)
	for {
		v, ok := iter.Next()
		if !ok {
			break
		}
		if err, isErr := v.(error); isErr {
			var halt *gojq.HaltError
			if errors.As(err, &halt) && halt.Value() == nil {
				// Bare halt: stop emitting, not a failure.
				break
			}
			// Disarm the filter before reporting: the failure envelope this
			// error becomes would otherwise be re-filtered by the same broken
			// expression and the diagnostic would vanish.
			ClearJQProgram()
			// Cancellation is the command's deadline or the user's signal,
			// not a filter failure — surface the bare context error so it
			// maps to the canceled/timeout exit codes.
			if xerrors.IsAny(err, context.Canceled, context.DeadlineExceeded) {
				return err
			}
			return &JQEvalError{Err: err}
		}
		if s, isStr := v.(string); isStr {
			buf.WriteString(s + "\n")
			continue
		}
		out, err := gojq.Marshal(v)
		if err != nil {
			ClearJQProgram()
			return &JQEvalError{Err: err}
		}
		buf.Write(out)
		buf.WriteByte('\n')
	}
	tracker := &writeTracker{w: w}
	if _, err := tracker.Write(buf.Bytes()); err != nil {
		return NewOutputError(err)
	}
	return nil
}
