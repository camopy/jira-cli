package main

// jira issue attachment {list,add,delete,download}.
//
// Service layer is pkg/jira.AttachmentService; command layer is thin.
// All four subcommands carry the dynamic-args='issuekey' annotation
// (via the shared issueKeyArg defined in issue_watcher.go) so the
// future issue-key cache layer plugs in without further changes.

import (
	"fmt"
	"io"
	"mime"
	"os"
	"path/filepath"
	"strings"

	"github.com/matcra587/jira-cli/pkg/jira"
	"github.com/spf13/cobra"
)

// IssueAttachmentCommand returns the `attachment` sub-command group
// for registration under `jira issue`. The lead wires this in
// commands.go's issueCommand() so the four sub-paths
// (list/add/delete/download) become available end-to-end.
func IssueAttachmentCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "attachment",
		Short: "Manage attachments on an issue (list/add/delete/download)",
	}
	cmd.AddCommand(
		issueAttachmentListCommand(),
		issueAttachmentAddCommand(),
		issueAttachmentDeleteCommand(),
		issueAttachmentDownloadCommand(),
	)
	return cmd
}

// attachmentClient resolves the active profile's HTTP client and the
// service binding. Returns ok=false when no base URL is configured
// (matches the pattern used elsewhere in this package).
func attachmentClient(cmd *cobra.Command) (jira.AttachmentService, bool, error) {
	client, _, ok, err := jiraClientForCommand(cmd)
	if err != nil {
		return nil, false, err
	}
	if !ok {
		return nil, false, nil
	}
	return jira.NewAttachmentService(client), true, nil
}

func issueAttachmentListCommand() *cobra.Command {
	var limit int
	var all bool
	cmd := &cobra.Command{
		Use:         "list KEY",
		Short:       "List attachments on an issue",
		Args:        cobra.ExactArgs(1),
		Annotations: issueKeyArg,
		RunE: func(cmd *cobra.Command, args []string) error {
			service, ok, err := attachmentClient(cmd)
			if err != nil {
				return err
			}
			if !ok {
				return fmt.Errorf("jira base URL is required for issue.attachment.list")
			}
			attachments, _, err := service.List(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			// Atlassian returns attachments oldest-first natively.
			// Apply the requested page slice client-side: there's
			// no dedicated /attachments list endpoint, so the full
			// set always returns and the CLI bounds the visible
			// window with --limit (default 50).
			windowed := attachments
			pageSize := limit
			if pageSize <= 0 {
				pageSize = 50
			}
			if !all && len(windowed) > pageSize {
				windowed = windowed[:pageSize]
			}
			rows := make([]map[string]any, 0, len(windowed))
			for _, a := range windowed {
				rows = append(rows, attachmentToOutput(a))
			}
			data := map[string]any{
				"attachments": rows,
				"pagination": map[string]any{
					"total":           len(attachments),
					"start_at":        0,
					"max_results":     pageSize,
					"is_last":         all || len(attachments) <= pageSize,
					"next_page_token": nil,
				},
			}
			return writeEnvelope(cmd, "issue.attachment.list", data)
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 50, "Page size (max attachments returned without --all)")
	cmd.Flags().BoolVar(&all, "all", false, "Return every attachment regardless of --limit")
	return cmd
}

func issueAttachmentAddCommand() *cobra.Command {
	var files []string
	var dryRun bool
	cmd := &cobra.Command{
		Use:         "add KEY [--file PATH ...] [PATH ...]",
		Short:       "Upload one or more attachments to an issue",
		Args:        cobra.MinimumNArgs(1),
		Annotations: issueKeyArg,
		RunE: func(cmd *cobra.Command, args []string) error {
			key := args[0]
			paths := append([]string{}, files...)
			paths = append(paths, args[1:]...) // positional file paths after KEY
			if len(paths) == 0 {
				return fmt.Errorf("attachment add: validation: at least one --file PATH or positional file argument is required")
			}
			// Pre-flight every path before any HTTP call. os.Stat
			// catches both missing and permission-denied paths.
			previews := make([]map[string]any, 0, len(paths))
			for _, p := range paths {
				info, err := os.Stat(p)
				if err != nil {
					return fmt.Errorf("attachment add: validation: %s: %w", p, err)
				}
				if info.IsDir() {
					return fmt.Errorf("attachment add: validation: %s is a directory", p)
				}
				previews = append(previews, map[string]any{
					"path":          p,
					"size":          info.Size(),
					"mime_inferred": inferAttachmentMime(p),
				})
			}
			if dryRun {
				return writeEnvelope(cmd, "issue.attachment.add", map[string]any{
					"key":     key,
					"files":   previews,
					"dry_run": true,
				})
			}
			service, ok, err := attachmentClient(cmd)
			if err != nil {
				return err
			}
			if !ok {
				return fmt.Errorf("jira base URL is required for issue.attachment.add")
			}
			handles := make([]*os.File, 0, len(paths))
			defer func() {
				for _, h := range handles {
					_ = h.Close()
				}
			}()
			sources := make([]jira.FileSource, 0, len(paths))
			for _, p := range paths {
				f, err := os.Open(p) //nolint:gosec // user-supplied attachment path, pre-validated by os.Stat above
				if err != nil {
					return fmt.Errorf("attachment add: validation: open %s: %w", p, err)
				}
				handles = append(handles, f)
				sources = append(sources, jira.FileSource{
					Name:   filepath.Base(p),
					Reader: f,
				})
			}
			uploaded, _, err := service.Add(cmd.Context(), key, sources)
			if err != nil {
				return err
			}
			rows := make([]map[string]any, 0, len(uploaded))
			for _, a := range uploaded {
				rows = append(rows, attachmentToOutput(a))
			}
			return writeEnvelope(cmd, "issue.attachment.add", map[string]any{
				"key":         key,
				"attachments": rows,
				"dry_run":     false,
			})
		},
	}
	cmd.Flags().StringSliceVar(&files, "file", nil, "Path to attach (repeatable)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview without uploading")
	return cmd
}

func issueAttachmentDeleteCommand() *cobra.Command {
	var force, dryRun bool
	cmd := &cobra.Command{
		Use:         "delete KEY ATTACHMENT_ID",
		Short:       "Delete an attachment from an issue",
		Args:        cobra.ExactArgs(2),
		Annotations: issueKeyArg,
		RunE: func(cmd *cobra.Command, args []string) error {
			key, attachmentID := args[0], args[1]
			if dryRun {
				return writeEnvelope(cmd, "issue.attachment.delete", map[string]any{
					"key":           key,
					"attachment_id": attachmentID,
					"dry_run":       true,
				})
			}
			det := DetectorFromContext(cmd)
			noInput, _ := cmd.Root().PersistentFlags().GetBool("no-input")
			// Destructive op MUST require --force under --no-input
			// or any non-TTY / agent context. No interactive
			// fallback in headless mode.
			if !force {
				if !det.IsTTY || det.Agent || noInput {
					return fmt.Errorf("attachment delete requires --force in headless / agent / --no-input mode")
				}
				if !confirmDestructive("attachment delete", attachmentID) {
					return fmt.Errorf("aborted by user")
				}
			}
			service, ok, err := attachmentClient(cmd)
			if err != nil {
				return err
			}
			if !ok {
				return fmt.Errorf("jira base URL is required for issue.attachment.delete")
			}
			if _, err := service.Delete(cmd.Context(), attachmentID); err != nil {
				return err
			}
			return writeEnvelope(cmd, "issue.attachment.delete", map[string]any{
				"attachment_id": attachmentID,
				"deleted":       true,
			})
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "Confirm destructive removal under --no-input / non-TTY")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview without deleting")
	return cmd
}

func issueAttachmentDownloadCommand() *cobra.Command {
	var output string
	var force, dryRun bool
	cmd := &cobra.Command{
		Use:         "download KEY ATTACHMENT_ID [--output PATH]",
		Short:       "Download an attachment from an issue",
		Args:        cobra.ExactArgs(2),
		Annotations: issueKeyArg,
		RunE: func(cmd *cobra.Command, args []string) error {
			key, attachmentID := args[0], args[1]
			det := DetectorFromContext(cmd)
			mode, target := resolveDownloadMode(output, det.IsTTY)
			// Clobber-protect for `--output` and TTY current-dir
			// modes happens BEFORE any HTTP call.
			if mode != downloadModeStdoutPiped && target != "" {
				if _, err := os.Stat(target); err == nil && !force {
					return fmt.Errorf("attachment download: validation: %s already exists; pass --force to overwrite", target)
				} else if err != nil && !os.IsNotExist(err) {
					return fmt.Errorf("attachment download: validation: %s: %w", target, err)
				}
			}
			if dryRun {
				return writeEnvelope(cmd, "issue.attachment.download", map[string]any{
					"key":           key,
					"attachment_id": attachmentID,
					"mode":          string(mode),
					"target":        target,
					"dry_run":       true,
				})
			}
			service, ok, err := attachmentClient(cmd)
			if err != nil {
				return err
			}
			if !ok {
				return fmt.Errorf("jira base URL is required for issue.attachment.download")
			}
			body, resp, err := service.Download(cmd.Context(), attachmentID)
			if err != nil {
				return err
			}
			defer func() { _ = body.Close() }()
			// In TTY current-dir mode, derive the filename from
			// the Content-Disposition the server sent.
			if mode == downloadModeCurrentDir && target == "" {
				target = filenameFromContentDisposition(resp)
				if target == "" {
					target = "attachment-" + attachmentID
				}
				if _, err := os.Stat(target); err == nil && !force {
					return fmt.Errorf("attachment download: validation: %s already exists; pass --force to overwrite", target)
				}
			}
			switch mode {
			case downloadModeStdoutPiped:
				// Pipe mode: raw bytes, no envelope. Binary-safe.
				if _, err := io.Copy(cmd.OutOrStdout(), body); err != nil {
					return err
				}
				return nil
			case downloadModeOutput, downloadModeCurrentDir:
				wrote, err := writeDownloadFile(target, body, force)
				if err != nil {
					return err
				}
				return writeEnvelope(cmd, "issue.attachment.download", map[string]any{
					"attachment_id": attachmentID,
					"written_to":    target,
					"bytes":         wrote,
					"mode":          string(mode),
				})
			}
			return fmt.Errorf("attachment download: unknown output mode %q", mode)
		},
	}
	cmd.Flags().StringVar(&output, "output", "", "Write to PATH (use - for stdout)")
	cmd.Flags().BoolVar(&force, "force", false, "Overwrite existing target file")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview without downloading")
	return cmd
}

// downloadMode names the three dispatching paths for `attachment
// download`. Strings match the envelope's data.mode values.
type downloadMode string

const (
	downloadModeOutput      downloadMode = "output"
	downloadModeCurrentDir  downloadMode = "current-dir"
	downloadModeStdoutPiped downloadMode = "stdout"
)

// resolveDownloadMode picks one of the three download paths from the
// command-line state. `target` is empty for current-dir mode (the
// server-provided filename is resolved AFTER the HTTP response so the
// caller can inspect Content-Disposition).
func resolveDownloadMode(output string, isTTY bool) (downloadMode, string) {
	switch output {
	case "":
		if isTTY {
			return downloadModeCurrentDir, ""
		}
		return downloadModeStdoutPiped, ""
	case "-":
		return downloadModeStdoutPiped, ""
	default:
		return downloadModeOutput, output
	}
}

// writeDownloadFile streams body bytes to target via io.Copy. Uses
// O_EXCL to enforce no-clobber and swaps to O_TRUNC under --force.
// Returns bytes written.
func writeDownloadFile(target string, body io.Reader, force bool) (int64, error) {
	flag := os.O_CREATE | os.O_WRONLY | os.O_EXCL
	if force {
		flag = os.O_CREATE | os.O_WRONLY | os.O_TRUNC
	}
	dst, err := os.OpenFile(target, flag, 0o600) //nolint:gosec // user-supplied download target, clobber-protected above
	if err != nil {
		return 0, err
	}
	defer func() { _ = dst.Close() }()
	return io.Copy(dst, body)
}

// filenameFromContentDisposition pulls `filename=` out of the response
// header. Empty when the header is absent or unparseable.
func filenameFromContentDisposition(resp *jira.Response) string {
	if resp == nil || resp.Response == nil {
		return ""
	}
	cd := resp.Response.Header.Get("Content-Disposition")
	if cd == "" {
		return ""
	}
	if _, params, err := mime.ParseMediaType(cd); err == nil {
		if name := params["filename"]; name != "" {
			return filepath.Base(name)
		}
	}
	return ""
}

// inferAttachmentMime returns a best-effort mime type for a local file
// based on the extension. Used for the `attachment add --dry-run`
// preview only; Jira does its own server-side detection on the actual
// upload.
func inferAttachmentMime(path string) string {
	ext := filepath.Ext(path)
	if ext == "" {
		return "application/octet-stream"
	}
	if mt := mime.TypeByExtension(ext); mt != "" {
		return strings.SplitN(mt, ";", 2)[0]
	}
	return "application/octet-stream"
}

// attachmentToOutput projects pkg/jira.Attachment to the envelope shape
// per envelope-shapes.md. Pointer-based source means we render empty
// strings for nil fields rather than panicking.
func attachmentToOutput(a jira.Attachment) map[string]any {
	out := map[string]any{
		"id":        derefString(a.ID),
		"filename":  derefString(a.Filename),
		"mime_type": derefString(a.MimeType),
		"size":      derefInt64(a.Size),
		"created":   derefString(a.Created),
	}
	out["author"] = attachmentUserToOutput(a.Author)
	return out
}

func attachmentUserToOutput(u *jira.User) map[string]any {
	if u == nil {
		return map[string]any{"account_id": "", "display_name": ""}
	}
	return map[string]any{
		"account_id":   derefString(u.AccountID),
		"display_name": derefString(u.DisplayName),
	}
}
