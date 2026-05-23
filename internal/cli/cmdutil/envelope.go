package cmdutil

import (
	"encoding/json"
	"io"
	"time"

	"github.com/matcra587/jira-cli/internal/adf"
	"github.com/matcra587/jira-cli/internal/cli"
	"github.com/matcra587/jira-cli/internal/jira"
	"github.com/spf13/cobra"
)

// WriteEnvelope emits the standard envelope shape for a command with no
// warnings.
func WriteEnvelope(cmd *cobra.Command, command string, data any) error {
	return WriteEnvelopeWithWarnings(cmd, command, data, nil)
}

// WriteEnvelopeWithRawWarnings emits the standard envelope shape with a
// caller-supplied list of free-form warning maps. Necessary for warnings
// whose schema (cache-truncated, rate-limit-during-paginate) carries
// fields outside the cli.Warning struct's Type/Message/Field/Path/etc.
// surface — see contracts/envelope-shapes.md.
func WriteEnvelopeWithRawWarnings(cmd *cobra.Command, command string, data any, warnings []map[string]any) error {
	for _, cw := range collectedCredentialWarnings(cmd) {
		warnings = append(warnings, map[string]any{
			"type":    cw.Type,
			"message": cw.Message,
			"lossy":   cw.Lossy,
		})
	}
	if UseCompactOutput(cmd) {
		// compact is the data payload without the envelope. Warnings have
		// no envelope to ride in, so fold any non-empty warning set into
		// the data so credential-cleanup and pagination notices stay
		// visible to agents (they would otherwise be silently dropped).
		return cli.WriteCompact(cmd.OutOrStdout(), FoldRawWarningsIntoData(data, warnings))
	}
	if UsePlainOutput(cmd) {
		if err := cli.WriteCommandPlain(cmd.OutOrStdout(), command, data, PlainOptionsForCommand(cmd)...); err != nil {
			return err
		}
		return MirrorADFWarningsToStderr(cmd.ErrOrStderr(), RawWarningsToCLI(warnings))
	}
	body := map[string]any{
		"ok": true,
		"meta": map[string]any{
			"command":    command,
			"timestamp":  time.Now().UTC().Format(time.RFC3339),
			"request_id": cli.NewRequestID(),
		},
		"data":     data,
		"errors":   []any{},
		"warnings": rawWarningsOrEmpty(warnings),
	}
	enc := json.NewEncoder(cmd.OutOrStdout())
	return enc.Encode(body)
}

// foldWarnings merges a non-empty warning slice into a compact-mode data
// payload so a correctness warning survives a mode that has no envelope
// to carry it. A map payload gets a "warnings" key alongside its existing
// fields; a non-map payload (slice or scalar) is wrapped as
// {"data": ..., "warnings": ...} so the warning is never silently
// dropped. An empty warning set returns the data unchanged.
func foldWarnings[T any](data any, warnings []T) any {
	if len(warnings) == 0 {
		return data
	}
	if m, ok := data.(map[string]any); ok {
		out := CopyAnyMap(m)
		out["warnings"] = warnings
		return out
	}
	return map[string]any{"data": data, "warnings": warnings}
}

// FoldRawWarningsIntoData folds a raw map-shaped warning slice into a
// compact payload. See foldWarnings.
func FoldRawWarningsIntoData(data any, warnings []map[string]any) any {
	return foldWarnings(data, warnings)
}

// FoldWarningsIntoData folds a typed cli.Warning slice into a compact
// payload. See foldWarnings.
func FoldWarningsIntoData(data any, warnings []cli.Warning) any {
	return foldWarnings(data, warnings)
}

func rawWarningsOrEmpty(w []map[string]any) []map[string]any {
	if w == nil {
		return []map[string]any{}
	}
	return w
}

// RawWarningsToCLI converts free-form map-shaped warnings into typed
// cli.Warning values, dropping empty entries.
func RawWarningsToCLI(warnings []map[string]any) []cli.Warning {
	out := make([]cli.Warning, 0, len(warnings))
	for _, warning := range warnings {
		if len(warning) == 0 {
			continue
		}
		out = append(out, cli.Warning{
			Type:    stringField(warning, "type"),
			Message: rawWarningMessage(warning),
			Field:   stringField(warning, "field"),
			Path:    stringField(warning, "path"),
			Lossy:   boolField(warning, "lossy"),
		})
	}
	return out
}

func rawWarningMessage(warning map[string]any) string {
	for _, key := range []string{"message", "remediation", "reason", "type"} {
		if value := stringField(warning, key); value != "" {
			return value
		}
	}
	return "warning"
}

// WriteEnvelopeWithWarnings is the warning-emitting envelope entry
// point — every command emitting structured warnings (typically from
// pipeline.RunMutation) calls this so warnings travel in the envelope
// under JSON mode and mirror to stderr under TTY/--plain (via the
// route helper).
func WriteEnvelopeWithWarnings(cmd *cobra.Command, command string, data any, warnings []adf.Warning) error {
	cliWarnings := make([]cli.Warning, 0, len(warnings))
	for _, w := range warnings {
		cliWarnings = append(cliWarnings, cli.WarningFrom(w))
	}
	cliWarnings = append(cliWarnings, collectedCredentialWarnings(cmd)...)
	if UseCompactOutput(cmd) {
		// compact has no envelope; fold warnings into the data so a failed
		// credential cleanup or other correctness notice is not lost.
		return cli.WriteCompact(cmd.OutOrStdout(), FoldWarningsIntoData(data, cliWarnings))
	}
	if UsePlainOutput(cmd) {
		// Data on stdout, warnings on stderr as clog WRN.
		if err := cli.WriteCommandPlain(cmd.OutOrStdout(), command, data, PlainOptionsForCommand(cmd)...); err != nil {
			return err
		}
		return MirrorADFWarningsToStderr(cmd.ErrOrStderr(), cliWarnings)
	}
	env := cli.Envelope{
		OK: true,
		Meta: cli.Meta{
			Command:   command,
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			RequestID: cli.NewRequestID(),
		},
		Data:     data,
		Errors:   []cli.Error{},
		Warnings: cliWarnings,
	}
	if env.Warnings == nil {
		env.Warnings = []cli.Warning{}
	}
	return cli.WriteEnvelope(cmd.OutOrStdout(), env)
}

// WriteEnvelopeWithResponse emits the standard envelope shape with
// pagination derived from an HTTP response and no pipeline warnings.
func WriteEnvelopeWithResponse(cmd *cobra.Command, command string, data any, resp *jira.Response) error {
	return WriteEnvelopeWithResponseAndWarnings(cmd, command, data, resp, nil)
}

// WriteEnvelopeWithResponseAndWarnings is the warning-emitting
// envelope entry point for commands that BOTH have a paginated/HTTP
// response AND need to surface
// pipeline warnings (e.g., live-submit issue create / edit / comment /
// worklog where the pipeline has already validated and produced
// best-effort warnings before the API call). Mirrors
// WriteEnvelopeWithWarnings's TTY routing for plain mode so the data
// stays on stdout and warnings mirror to stderr as clog WRN lines.
func WriteEnvelopeWithResponseAndWarnings(cmd *cobra.Command, command string, data any, resp *jira.Response, warnings []adf.Warning) error {
	if resp == nil {
		// WriteEnvelopeWithWarnings collects the credential warnings itself.
		return WriteEnvelopeWithWarnings(cmd, command, data, warnings)
	}
	cliWarnings := make([]cli.Warning, 0, len(warnings))
	for _, w := range warnings {
		cliWarnings = append(cliWarnings, cli.WarningFrom(w))
	}
	cliWarnings = append(cliWarnings, collectedCredentialWarnings(cmd)...)
	if UseCompactOutput(cmd) {
		if m, ok := data.(map[string]any); ok {
			m["pagination"] = paginationFromResponse(resp)
			return cli.WriteCompact(cmd.OutOrStdout(), FoldWarningsIntoData(m, cliWarnings))
		}
		return cli.WriteCompact(cmd.OutOrStdout(), data)
	}
	if UsePlainOutput(cmd) {
		if err := cli.WriteCommandPlain(cmd.OutOrStdout(), command, data, PlainOptionsForCommand(cmd)...); err != nil {
			return err
		}
		return MirrorADFWarningsToStderr(cmd.ErrOrStderr(), cliWarnings)
	}
	env := cli.Envelope{
		OK: true,
		Meta: cli.Meta{
			Command:    command,
			Timestamp:  time.Now().UTC().Format(time.RFC3339),
			RequestID:  cli.NewRequestID(),
			Pagination: paginationFromResponse(resp),
		},
		Data:     data,
		Errors:   []cli.Error{},
		Warnings: cliWarnings,
	}
	if env.Warnings == nil {
		env.Warnings = []cli.Warning{}
	}
	return cli.WriteEnvelope(cmd.OutOrStdout(), env)
}

// MirrorADFWarningsToStderr is a thin wrapper around cli.RouteWarnings
// for the plain-mode warning-only path used by both
// WriteEnvelopeWithWarnings and WriteEnvelopeWithResponseAndWarnings.
func MirrorADFWarningsToStderr(stderr io.Writer, warnings []cli.Warning) error {
	if len(warnings) == 0 || stderr == nil {
		return nil
	}
	return cli.RouteWarnings(cli.RouteOptions{
		Stderr:   stderr,
		Stdout:   io.Discard, // data was already written above
		Mode:     cli.RoutePlain,
		Command:  "",
		Data:     map[string]any{}, // no-op data, we only want the WRN lines
		Warnings: warnings,
	})
}

// paginationFromResponse derives the envelope pagination block from an HTTP
// response, or nil when resp is nil.
func paginationFromResponse(resp *jira.Response) *cli.Pagination {
	if resp == nil {
		return nil
	}
	return &cli.Pagination{
		StartAt:    resp.StartAt, // pagination-exempt: output-shape, not consumer cursor
		MaxResults: resp.MaxResults,
		Total:      resp.Total,
		IsLast:     resp.NextCursor() == "",
		NextCursor: resp.NextCursor(),
	}
}
