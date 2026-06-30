package issue

// jira issue attachment {list,add,delete,download}.
//
// Service layer is pkg/jira.AttachmentService; command layer is thin.
// All four subcommands carry the dynamic-args='issuekey' annotation
// (via the shared issueKeyArg defined in issue_watcher.go) so the
// future issue-key cache layer plugs in without further changes.

import (
	"context"
	"fmt"
	"io"
	"mime"
	"os"
	"path/filepath"
	"strings"

	clib "github.com/gechr/clib/cli/cobra"
	"github.com/matcra587/jira-cli/internal/cli"
	"github.com/matcra587/jira-cli/internal/cli/cmdutil"
	"github.com/matcra587/jira-cli/internal/issuekey"
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
		Short: "Manage issue attachments",
		Long: "List, upload, delete, and download Jira issue attachments. Use this group " +
			"when working with files attached to issue comments or descriptions.\n\n" +
			"Upload, delete, and download subcommands each document their own dry-run or " +
			"force behavior. Attachment binary content is never written to stdout.",
		Example: `# List attachments on an issue
$ jira issue attachment list PROJ-123

# Preview uploading a file
$ jira issue attachment add PROJ-123 ./report.pdf --dry-run

# Write binary content to a file instead of stdout
$ jira issue attachment download PROJ-123 10500 --to ./report.pdf`,
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
	return cmdutil.ServicesForClient(client).Attachment(), true, nil
}

func issueAttachmentListCommand() *cobra.Command {
	var limit int
	var all bool
	var parallelism int
	cmd := &cobra.Command{
		Use:   "list KEY...",
		Short: "List attachments on an issue",
		Long: "List attachments on one or more issues. Use it to find attachment IDs before " +
			"downloading or deleting files.\n\n" +
			"Jira returns attachments as part of the issue data; the command applies " +
			"`--limit` client-side unless `--all` is set. Multiple issue keys are read " +
			"with bounded parallelism.",
		Args:        cobra.MinimumNArgs(1),
		Annotations: issueKeyArg,
		Example: `# List attachments on an issue
$ jira issue attachment list PROJ-123

# Return every attachment regardless of page size
$ jira issue attachment list PROJ-123 --all

# List attachments as JSON
$ jira issue attachment list PROJ-123 --output=json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			keys, err := issuekey.ParseExpressions(args, issuekey.Options{MaxExpansion: issuekey.DefaultMaxExpansion})
			if err != nil {
				return err
			}
			service, ok, err := attachmentClient(cmd)
			if err != nil {
				return err
			}
			if !ok {
				return fmt.Errorf("jira base URL is required for issue.attachment.list")
			}
			if len(keys) == 1 {
				var attachments []jira.Attachment
				if err := cmdutil.Spin(cmd, "issue.attachment.list", func(ctx context.Context) error {
					var e error
					attachments, _, e = service.List(ctx, keys[0])
					return e
				}); err != nil {
					return err
				}
				return cmdutil.WriteEnvelope(cmd, "issue.attachment.list", attachmentListEnvelopeData(attachments, limit, all))
			}
			results, err := cmdutil.FanOutKeys(cmd.Context(), keys, parallelism, func(ctx context.Context, key string) ([]jira.Attachment, error) {
				attachments, _, err := service.List(ctx, key)
				return attachments, err
			})
			if err != nil {
				return err
			}
			return cmdutil.WriteKeyedResultsEnvelope(cmd, "issue.attachment.list", results, func(_ string, attachments []jira.Attachment) any {
				return attachmentListEnvelopeData(attachments, limit, all)
			})
		},
	}
	cmdutil.AddIntVar(cmd.Flags(), &limit, "limit", 50, "Page size (max attachments returned without `--all`)", clib.FlagExtra{Group: "Pagination", Placeholder: "N", Terse: "page size"})
	cmdutil.AddBoolVar(cmd.Flags(), &all, "all", false, "Return every attachment regardless of `--limit`", clib.FlagExtra{Group: "Pagination", Terse: "fetch all pages"})
	cmdutil.AddParallelismFlag(cmd, &parallelism)
	return cmd
}

func attachmentListEnvelopeData(attachments []jira.Attachment, limit int, all bool) map[string]any {
	// Atlassian returns attachments oldest-first natively. Apply the requested
	// page slice client-side: there is no dedicated /attachments list endpoint.
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
	return map[string]any{
		"attachments": rows,
		"pagination": map[string]any{
			"total":           len(attachments),
			"start_at":        0,
			"max_results":     pageSize,
			"is_last":         all || len(attachments) <= pageSize,
			"next_page_token": nil,
		},
	}
}

func issueAttachmentAddCommand() *cobra.Command {
	var files []string
	var dryRun bool
	var parallelism int
	cmd := &cobra.Command{
		Use:   "add KEY... [PATH...]",
		Short: "Upload one or more attachments to an issue",
		Long: "Upload one or more local files to one or more issues. Use positional paths " +
			"for quick uploads or repeated `--file` flags when paths contain shell-sensitive " +
			"characters.\n\n" +
			"Before any HTTP request, the command checks every path, file type, and size. " +
			"`--dry-run` performs that local preflight and prints the inferred files without " +
			"uploading anything.",
		Args:        cobra.MinimumNArgs(1),
		Annotations: issueKeyArg,
		Example: `# Attach a file to an issue
$ jira issue attachment add PROJ-123 ./report.pdf

# Attach several files via repeated --file
$ jira issue attachment add PROJ-123 --file ./report.pdf --file ./logs.txt

# Preview local file checks without uploading
$ jira issue attachment add PROJ-123 ./report.pdf --dry-run`,
		RunE: func(cmd *cobra.Command, args []string) error {
			keys, paths, err := attachmentAddKeysAndPaths(args, files)
			if err != nil {
				return err
			}
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
				if len(keys) > 1 {
					return runAttachmentAddManyDryRun(cmd, keys, previews)
				}
				return cmdutil.WriteEnvelope(cmd, "issue.attachment.add", map[string]any{
					"key":     keys[0],
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
			if len(keys) > 1 {
				return runAttachmentAddMany(cmd, service, keys, sources, parallelism)
			}
			fileSources, closeFiles, err := openAttachmentFileSources(sources)
			if err != nil {
				return err
			}
			defer closeFiles()
			var uploaded []jira.Attachment
			if err := cmdutil.Spin(cmd, "issue.attachment.add", func(ctx context.Context) error {
				var e error
				uploaded, _, e = service.Add(ctx, keys[0], fileSources)
				return e
			}); err != nil {
				return err
			}
			rows := make([]map[string]any, 0, len(uploaded))
			for _, a := range uploaded {
				rows = append(rows, attachmentToOutput(a))
			}
			return cmdutil.WriteEnvelope(cmd, "issue.attachment.add", map[string]any{
				"key":         keys[0],
				"attachments": rows,
				"dry_run":     false,
			})
		},
	}
	// StringArrayVar, not StringSliceVar: a slice flag splits each value
	// on commas, which would shatter a legitimate filename like
	// "report,final.pdf" into two bogus paths. Each --file is one path.
	cmdutil.AddStringArrayVar(cmd.Flags(), &files, "file", nil, "Path to attach (repeatable)", clib.FlagExtra{Group: "Input", Placeholder: "PATH", Hint: "file"})
	cmdutil.AddDryRunFlag(cmd.Flags(), &dryRun, "Preview without uploading")
	cmdutil.AddParallelismFlag(cmd, &parallelism)
	return cmd
}

func attachmentAddKeysAndPaths(args, files []string) ([]string, []string, error) {
	if len(files) > 0 {
		keys, err := issuekey.ParseExpressions(args, issuekey.Options{MaxExpansion: issuekey.DefaultMaxExpansion})
		if err != nil {
			return nil, nil, err
		}
		return keys, append([]string{}, files...), nil
	}
	keys, err := issuekey.ParseExpressions([]string{args[0]}, issuekey.Options{MaxExpansion: issuekey.DefaultMaxExpansion})
	if err != nil {
		return nil, nil, err
	}
	paths := append([]string{}, args[1:]...)
	return keys, paths, nil
}

func runAttachmentAddManyDryRun(cmd *cobra.Command, keys []string, previews []map[string]any) error {
	results := make([]cmdutil.KeyResult[map[string]any], len(keys))
	for i, key := range keys {
		results[i] = cmdutil.KeyResult[map[string]any]{
			Key: key,
			Value: map[string]any{
				"key":     key,
				"files":   previews,
				"dry_run": true,
			},
		}
	}
	return cmdutil.WriteKeyedResultsEnvelope(cmd, "issue.attachment.add", results, func(_ string, data map[string]any) any { return data })
}

func runAttachmentAddMany(
	cmd *cobra.Command,
	service jira.AttachmentService,
	keys []string,
	sources []attachmentFileSource,
	parallelism int,
) error {
	results, err := cmdutil.FanOutKeys(cmd.Context(), keys, parallelism, func(ctx context.Context, key string) (map[string]any, error) {
		fileSources, closeFiles, err := openAttachmentFileSources(sources)
		if err != nil {
			return nil, err
		}
		defer closeFiles()
		uploaded, _, err := service.Add(ctx, key, fileSources)
		if err != nil {
			return nil, err
		}
		rows := make([]map[string]any, 0, len(uploaded))
		for _, a := range uploaded {
			rows = append(rows, attachmentToOutput(a))
		}
		return map[string]any{
			"key":         key,
			"attachments": rows,
			"dry_run":     false,
		}, nil
	})
	if err != nil {
		return err
	}
	return cmdutil.WriteKeyedResultsEnvelope(cmd, "issue.attachment.add", results, func(_ string, data map[string]any) any { return data })
}

func openAttachmentFileSources(sources []attachmentFileSource) ([]jira.FileSource, func(), error) {
	handles := make([]*os.File, 0, len(sources))
	closeFiles := func() {
		for _, h := range handles {
			_ = h.Close()
		}
	}
	fileSources := make([]jira.FileSource, 0, len(sources))
	for _, source := range sources {
		f, err := os.Open(source.Path)
		if err != nil {
			closeFiles()
			return nil, nil, fmt.Errorf("attachment add: validation: open %s: %w", source.Path, err)
		}
		handles = append(handles, f)
		fileSources = append(fileSources, jira.FileSource{
			Name:   filepath.Base(source.Path),
			Size:   source.Size,
			Reader: f,
		})
	}
	return fileSources, closeFiles, nil
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
		Use:   "delete KEY ATTACHMENT_ID",
		Short: "Delete an attachment from an issue",
		Long: "Delete one attachment by its Jira attachment ID. Use `attachment list` first " +
			"when you need to discover the ID for an issue.\n\n" +
			"`--dry-run` previews the deletion and never contacts Jira. Live deletes require " +
			"`--force` in headless, agent, or `--no-input` mode; an interactive terminal is " +
			"prompted when `--force` is omitted.",
		Args:        cobra.ExactArgs(2),
		Annotations: issueKeyArg,
		Example: `# Preview deleting an attachment
$ jira issue attachment delete PROJ-123 10500 --dry-run

# Delete an attachment by id without a prompt
$ jira issue attachment delete PROJ-123 10500 --force`,
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
			if err := cmdutil.Spin(cmd, "issue.attachment.delete", func(ctx context.Context) error {
				_, e := service.Delete(ctx, attachmentID)
				return e
			}); err != nil {
				return err
			}
			return cmdutil.WriteEnvelope(cmd, "issue.attachment.delete", map[string]any{
				"attachment_id": attachmentID,
				"deleted":       true,
			})
		},
	}
	cmdutil.AddForceFlag(cmd.Flags(), &force, "Confirm destructive removal under `--no-input` / non-TTY")
	cmdutil.AddDryRunFlag(cmd.Flags(), &dryRun, "Preview without deleting")
	return cmd
}

func issueAttachmentDownloadCommand() *cobra.Command {
	var output string
	var force, dryRun bool
	cmd := &cobra.Command{
		Use:   "download KEY ATTACHMENT_ID [--to PATH]",
		Short: "Download an attachment from an issue",
		Long: "Download one attachment by ID and write it to a file. Use it after " +
			"`attachment list` when you need the file content locally.\n\n" +
			"Binary content is always written to a file, never stdout, so JSON output stays " +
			"parseable. Existing target files are protected unless `--force` is set. " +
			"`--dry-run` validates the target path and prints the planned write without " +
			"downloading.",
		Args:        cobra.ExactArgs(2),
		Annotations: issueKeyArg,
		Example: `# Download an attachment to the current directory
$ jira issue attachment download PROJ-123 10500

# Download to a specific path, overwriting if it exists
$ jira issue attachment download PROJ-123 10500 --to ./report.pdf --force

# Preview the download target without contacting Jira
$ jira issue attachment download PROJ-123 10500 --to ./report.pdf --dry-run`,
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
			var body io.ReadCloser
			var resp *jira.Response
			if err := cmdutil.Spin(cmd, "issue.attachment.download", func(ctx context.Context) error {
				var e error
				body, resp, e = service.Download(ctx, attachmentID)
				return e
			}); err != nil {
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
	cmdutil.AddFileFlag(cmd.Flags(), &output, "to", "", "Write the attachment to PATH (default: current directory)", "Output", "PATH")
	cmdutil.AddForceFlag(cmd.Flags(), &force, "Overwrite existing target file")
	cmdutil.AddDryRunFlag(cmd.Flags(), &dryRun, "Preview without downloading")
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
