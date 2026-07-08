package cmdutil

import (
	"fmt"
	"sort"
	"strings"

	"github.com/matcra587/jira-cli/internal/adf"
	"github.com/matcra587/jira-cli/internal/cli"
	"github.com/spf13/cobra"
)

// KeyedResultsData is the common envelope payload for commands that operate
// on a set of issue keys independently.
type KeyedResultsData struct {
	Results   []KeyedResult `json:"results"`
	Succeeded int           `json:"succeeded"`
	Failed    int           `json:"failed"`
}

// KeyedResult carries one issue-key result. Data is command-specific and Error
// is the normalized per-key failure.
type KeyedResult struct {
	Key   string     `json:"key"`
	OK    bool       `json:"ok"`
	Data  any        `json:"data,omitempty"`
	Error *cli.Error `json:"error,omitempty"`
}

// WriteKeyedResultsEnvelope writes the shared multi-key result shape. It
// preserves requested-key order in data.results and returns a non-zero command
// error when any key failed while retaining successful keys in the envelope.
func WriteKeyedResultsEnvelope[T any](
	cmd *cobra.Command,
	command string,
	results []KeyResult[T],
	dataFor func(string, T) any,
) error {
	data, errorsOut, topErr := keyedResultsEnvelopeData(command, results, dataFor)
	if len(errorsOut) == 0 {
		return WriteEnvelope(cmd, command, data)
	}
	if err := WriteEnvelopeWithErrors(cmd, command, data, errorsOut); err != nil {
		return err
	}
	err := EnvelopeWritten(topErr)
	if UsePlainOutput(cmd) {
		return DiagnosticWritten(err)
	}
	return err
}

func keyedResultsEnvelopeData[T any](
	command string,
	results []KeyResult[T],
	dataFor func(string, T) any,
) (KeyedResultsData, []cli.Error, error) {
	data := KeyedResultsData{Results: make([]KeyedResult, len(results))}
	type mappedFailure struct {
		err  cli.Error
		exit int
	}
	failures := make([]mappedFailure, 0)
	for i, result := range results {
		entry := KeyedResult{Key: result.Key, OK: result.Err == nil}
		if result.Err != nil {
			mapped := cli.MapError(result.Err)
			entry.Error = &mapped
			data.Failed++
			failures = append(failures, mappedFailure{err: mapped, exit: cli.ExitCode(mapped)})
		} else {
			if dataFor != nil {
				entry.Data = dataFor(result.Key, result.Value)
			} else {
				entry.Data = result.Value
			}
			data.Succeeded++
		}
		data.Results[i] = entry
	}
	sort.SliceStable(failures, func(i, j int) bool {
		return failures[i].exit > failures[j].exit
	})
	errorsOut := make([]cli.Error, len(failures))
	var topErr error
	for i, failure := range failures {
		errorsOut[i] = failure.err
		if i == 0 {
			topErr = keyedResultsPartialFailureError(command, data, failure.err)
		}
	}
	return data, errorsOut, topErr
}

func keyedResultsPartialFailureError(command string, data KeyedResultsData, top cli.Error) error {
	total := data.Succeeded + data.Failed
	reason := keyedResultsFailureReason(top)
	status := "successful results are shown above"
	if data.Succeeded == 0 {
		status = "no keys completed successfully"
	}
	msg := fmt.Sprintf("%s completed with %d of %d failed (%s); %s", strings.ReplaceAll(command, ".", " "), data.Failed, total, reason, status)
	return cli.NewCodedError(cli.AggregateCode(top), msg)
}

func keyedResultsFailureReason(top cli.Error) string {
	if top.Code != "" {
		return strings.ReplaceAll(top.Code, "_", " ")
	}
	if top.Type != "" {
		return top.Type
	}
	return "error"
}

// KeyedDataWithWarnings folds command-wide mutation warnings into each
// successful per-key data payload. Batch mutation envelopes do not have a
// single persisted resource, so the warning travels with each resource result.
func KeyedDataWithWarnings(warnings []adf.Warning) func(string, map[string]any) any {
	return func(_ string, data map[string]any) any {
		if len(warnings) == 0 {
			return data
		}
		out := CopyAnyMap(data)
		out["warnings"] = warnings
		return out
	}
}
