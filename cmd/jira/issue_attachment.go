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

	"github.com/matcra587/jira-cli/internal/cli"
	"github.com/matcra587/jira-cli/internal/cli/cmdutil"
	"github.com/matcra587/jira-cli/internal/jira"
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
	client, _, ok, err := cmdutil.JiraClientForCommand(cmd)
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
			return cmdutil.WriteEnvelope(cmd, "issue.attachment.list", data)
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 50, "Page size (max attachments returned without --all)")
	cmd.Flags().BoolVar(&all, "all", false, "Return every attachment regardless of --limit")
	cmdutil.ExtendPaginationFlags(cmd.Flags())
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
			sources, err := attachmentFileSources(paths)
			if err != nil {
				return err
			}
			previews := make([]map[string]any, 0, len(sources))
			for _, src := range sources {
				previews = append(previews, map[string]any{
					"path":          src.Path,
					"size":          src.Size,
					"mime_inferred": inferAttachmentMime(src.Path),
				})
			}
			if dryRun {
				return cmdutil.WriteEnvelope(cmd, "issue.attachment.add", map[string]any{
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
			fileSources := make([]jira.FileSource, 0, len(paths))
			for _, source := range sources {
				f, err := os.Open(source.Path)
				if err != nil {
					return fmt.Errorf("attachment add: validation: open %s: %w", source.Path, err)
				}
				handles = append(handles, f)
				fileSources = append(fileSources, jira.FileSource{
					Name:   filepath.Base(source.Path),
					Size:   source.Size,
					Reader: f,
				})
			}
			uploaded, _, err := service.Add(cmd.Context(), key, fileSources)
			if err != nil {
				return err
			}
			rows := make([]map[string]any, 0, len(uploaded))
			for _, a := range uploaded {
				rows = append(rows, attachmentToOutput(a))
			}
			return cmdutil.WriteEnvelope(cmd, "issue.attachment.add", map[string]any{
				"key":         key,
				"attachments": rows,
				"dry_run":     false,
			})
		},
	}
	// StringArrayVar, not StringSliceVar: a slice flag splits each value
	// on commas, which would shatter a legitimate filename like
	// "report,final.pdf" into two bogus paths. Each --file is one path.
	cmd.Flags().StringArrayVar(&files, "file", nil, "Path to attach (repeatable)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview without uploading")
	cmdutil.ExtendFileFlag(cmd.Flags(), "file", "Input", "PATH")
	cmdutil.ExtendDryRunFlag(cmd.Flags())
	return cmd
}

type attachmentFileSource struct {
	Path string
	Size int64
}

func attachmentFileSources(paths []string) ([]attachmentFileSource, error) {
	sources := make([]attachmentFileSource, 0, len(paths))
	var total int64
	for _, p := range paths {
		info, err := os.Stat(p)
		if err != nil {
			return nil, fmt.Errorf("attachment add: validation: %s: %w", p, err)
		}
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("attachment add: validation: %s is not a regular file", p)
		}
		if info.Size() > jira.MaxAttachmentUploadBytes() {
			return nil, fmt.Errorf("attachment add: validation: %s size %d exceeds %d bytes", p, info.Size(), jira.MaxAttachmentUploadBytes())
		}
		total += info.Size()
		if total > jira.MaxAttachmentUploadBytes() {
			return nil, fmt.Errorf("attachment add: validation: total upload size %d exceeds %d bytes", total, jira.MaxAttachmentUploadBytes())
		}
		sources = append(sources, attachmentFileSource{Path: p, Size: info.Size()})
	}
	return sources, nil
}

func jiraMaxAttachmentUploadBytes() int64 {
	return jira.MaxAttachmentUploadBytes()
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
				return cmdutil.WriteEnvelope(cmd, "issue.attachment.delete", map[string]any{
					"key":           key,
					"attachment_id": attachmentID,
					"dry_run":       true,
				})
			}
			det := cmdutil.DetectorFromContext(cmd)
			noInput := cmdutil.NoInputRequested(cmd)
			// Destructive op MUST require --force under --no-input
			// or any non-TTY / agent context. No interactive
			// fallback in headless mode.
			if !force {
				if !det.IsTTY || det.Agent || noInput {
					return fmt.Errorf("attachment delete requires --force in headless / agent / --no-input mode")
				}
				if ok, err := confirmDestructive(cmd, "attachment delete", attachmentID); err != nil {
					return err
				} else if !ok {
					return cli.NewPromptError(cli.PromptAborted, "attachment delete", nil)
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
			return cmdutil.WriteEnvelope(cmd, "issue.attachment.delete", map[string]any{
				"attachment_id": attachmentID,
				"deleted":       true,
			})
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "Confirm destructive removal under --no-input / non-TTY")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview without deleting")
	cmdutil.ExtendForceFlag(cmd.Flags())
	cmdutil.ExtendDryRunFlag(cmd.Flags())
	return cmd
}

func issueAttachmentDownloadCommand() *cobra.Command {
	var output string
	var force, dryRun bool
	cmd := &cobra.Command{
		Use:         "download KEY ATTACHMENT_ID [--to PATH]",
		Short:       "Download an attachment from an issue",
		Args:        cobra.ExactArgs(2),
		Annotations: issueKeyArg,
		RunE: func(cmd *cobra.Command, args []string) error {
			key, attachmentID := args[0], args[1]
			mode, target := resolveDownloadMode(output)
			// Clobber-protect for an explicit --to target happens BEFORE
			// any HTTP call.
			if target != "" {
				if _, err := os.Stat(target); err == nil && !force {
					return fmt.Errorf("attachment download: validation: %s already exists; pass --force to overwrite", target)
				} else if err != nil && !os.IsNotExist(err) {
					return fmt.Errorf("attachment download: validation: %s: %w", target, err)
				}
			}
			if dryRun {
				return cmdutil.WriteEnvelope(cmd, "issue.attachment.download", map[string]any{
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
			// Attachment binary content always writes to a file — it is
			// never streamed to stdout, in any output mode, so a JSON or
			// compact consumer never has binary bytes spliced into its
			// parseable stream.
			wrote, err := writeDownloadFile(target, body, force)
			if err != nil {
				return err
			}
			return cmdutil.WriteEnvelope(cmd, "issue.attachment.download", map[string]any{
				"attachment_id": attachmentID,
				"written_to":    target,
				"bytes":         wrote,
				"mode":          string(mode),
			})
		},
	}
	cmd.Flags().StringVar(&output, "to", "", "Write the attachment to PATH (default: current directory)")
	cmd.Flags().BoolVar(&force, "force", false, "Overwrite existing target file")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview without downloading")
	cmdutil.ExtendFileFlag(cmd.Flags(), "to", "Output", "PATH")
	cmdutil.ExtendForceFlag(cmd.Flags())
	cmdutil.ExtendDryRunFlag(cmd.Flags())
	return cmd
}

// downloadMode names the two dispatching paths for `attachment
// download`. Strings match the envelope's data.mode values. There is no
// stdout path: attachment binary content always writes to a file.
type downloadMode string

const (
	downloadModeOutput     downloadMode = "output"
	downloadModeCurrentDir downloadMode = "current-dir"
)

// resolveDownloadMode picks the download path from the --to flag.
// `target` is empty for current-dir mode (the server-provided filename
// is resolved AFTER the HTTP response so the caller can inspect
// Content-Disposition).
func resolveDownloadMode(output string) (downloadMode, string) {
	if output == "" {
		return downloadModeCurrentDir, ""
	}
	return downloadModeOutput, output
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
