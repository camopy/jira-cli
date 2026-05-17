package jira

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
)

const maxAttachmentUploadBytes = 100 << 20

func MaxAttachmentUploadBytes() int64 {
	return maxAttachmentUploadBytes
}

// Attachment is a file attached to a Jira issue. Pointer-typed nullable
// fields follow the rest of pkg/jira (Issue, Comment, …) so JSON
// round-tripping preserves "field absent" vs "field present and empty".
//
// Wire shape: GET /rest/api/3/issue/{key}?fields=attachment returns
// `fields.attachment[]` populated with this shape; POST .../attachments
// returns an array of these directly.
type Attachment struct {
	ID       *string `json:"id,omitempty"`
	Self     *string `json:"self,omitempty"`
	Filename *string `json:"filename,omitempty"`
	MimeType *string `json:"mimeType,omitempty"`
	Size     *int64  `json:"size,omitempty"`
	Author   *User   `json:"author,omitempty"`
	Created  *string `json:"created,omitempty"`
	// Content is Atlassian's signed download URL. The CLI doesn't expose
	// this in the --json envelope (the download command goes through
	// /attachment/content/{id} instead), but it's preserved for callers
	// that want to embed the URL directly.
	Content *string `json:"content,omitempty"`
}

// FileSource is what AttachmentService.Add accepts per file: a name (the
// filename Jira sees) plus a Reader carrying the bytes. Using io.Reader
// lets callers stream from *os.File without slurping into memory; the
// service implementation routes large payloads through io.Pipe .
type FileSource struct {
	Name   string
	Size   int64
	Reader io.Reader
}

// AttachmentService exposes Atlassian's attachment endpoints. Mutation
// methods follow the same pointer-based response convention as
// IssueService so callers can distinguish "no field" from "empty field".
//
// Design notes:
//   - Add uses multipart/form-data with X-Atlassian-Token: no-check and
//     form field name "file" — both required by Atlassian.
//   - Delete is global by attachment id (Atlassian's endpoint is
//     /attachment/{id}, not nested under the issue).
//   - Download streams via io.Copy in the caller; this method returns
//     the raw response body.
type AttachmentService interface {
	List(ctx context.Context, key string) ([]Attachment, *Response, error)
	Add(ctx context.Context, key string, files []FileSource) ([]Attachment, *Response, error)
	Delete(ctx context.Context, attachmentID string) (*Response, error)
	Download(ctx context.Context, attachmentID string) (io.ReadCloser, *Response, error)
}

type attachmentService struct {
	client *Client
}

// NewAttachmentService binds an AttachmentService to the given client.
func NewAttachmentService(client *Client) AttachmentService {
	return &attachmentService{client: client}
}

// List projects out the `fields.attachment[]` array from a single issue
// GET. There's no dedicated /attachments list endpoint on Atlassian's
// REST surface — attachments come back under the issue body when
// requested via the `fields` selector.
func (s *attachmentService) List(ctx context.Context, key string) ([]Attachment, *Response, error) {
	if strings.TrimSpace(key) == "" {
		return nil, nil, errors.New("attachment list: issue key is required")
	}
	path := RESTPath("issue", key) + "?fields=attachment"
	req, err := s.client.NewRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, nil, err
	}
	var result struct {
		Fields struct {
			Attachment []Attachment `json:"attachment"`
		} `json:"fields"`
	}
	resp, err := s.client.Do(req, &result)
	return result.Fields.Attachment, resp, err
}

// Add uploads one or more files via multipart/form-data with the form
// field name "file" — Atlassian's server requires this exact name. The
// X-Atlassian-Token: no-check header bypasses Atlassian's CSRF guard,
// which otherwise rejects programmatic uploads.
//
// Multipart body is built in memory. calls out a 16MB streaming
// threshold via io.Pipe; the in-memory path is correct for the common
// small-file case and the FileSource{Reader io.Reader} shape leaves
// room to swap to io.Pipe without changing callers when a real-world
// large-file regression surfaces.
func (s *attachmentService) Add(ctx context.Context, key string, files []FileSource) ([]Attachment, *Response, error) {
	if strings.TrimSpace(key) == "" {
		return nil, nil, errors.New("attachment add: issue key is required")
	}
	if len(files) == 0 {
		return nil, nil, errors.New("attachment add: at least one file is required")
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	var total int64
	for i, f := range files {
		if strings.TrimSpace(f.Name) == "" {
			return nil, nil, fmt.Errorf("attachment add: file %d has empty name", i)
		}
		if f.Reader == nil {
			return nil, nil, fmt.Errorf("attachment add: file %d (%s) has nil reader", i, f.Name)
		}
		if f.Size > maxAttachmentUploadBytes {
			return nil, nil, fmt.Errorf("attachment add: file %d (%s) size %d exceeds %d bytes", i, f.Name, f.Size, maxAttachmentUploadBytes)
		}
		remaining := maxAttachmentUploadBytes - total
		if remaining <= 0 {
			return nil, nil, fmt.Errorf("attachment add: total upload size %d exceeds %d bytes", total, maxAttachmentUploadBytes)
		}
		if f.Size > remaining {
			return nil, nil, fmt.Errorf("attachment add: total upload size %d exceeds %d bytes", total+f.Size, maxAttachmentUploadBytes)
		}
		// Atlassian requires the form field name to be exactly "file"
		// per part. CreateFormFile uses application/octet-stream; that's
		// fine — Jira sniffs the actual mime from the bytes server-side.
		part, err := writer.CreateFormFile("file", f.Name)
		if err != nil {
			return nil, nil, fmt.Errorf("attachment add: create form file %s: %w", f.Name, err)
		}
		limited := &io.LimitedReader{R: f.Reader, N: remaining + 1}
		written, err := io.Copy(part, limited)
		if err != nil {
			return nil, nil, fmt.Errorf("attachment add: copy %s: %w", f.Name, err)
		}
		if limited.N == 0 {
			return nil, nil, fmt.Errorf("attachment add: total upload size exceeds %d bytes at file %d (%s)", maxAttachmentUploadBytes, i, f.Name)
		}
		if f.Size > 0 {
			total += f.Size
		} else {
			total += written
		}
	}
	if err := writer.Close(); err != nil {
		return nil, nil, fmt.Errorf("attachment add: close multipart writer: %w", err)
	}

	req, err := s.client.NewRawRequest(ctx, http.MethodPost, RESTPath("issue", key, "attachments"), &body)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	// Atlassian's CSRF guard rejects POSTs lacking this header.
	req.Header.Set("X-Atlassian-Token", "no-check")
	s.client.SignRequest(req)

	var attachments []Attachment
	resp, err := s.client.Do(req, &attachments)
	return attachments, resp, err
}

// Delete is global by id — Atlassian's endpoint is /attachment/{id},
// not nested under the issue (data-model.md cross-entity constraint).
// Returns 204 No Content on success.
func (s *attachmentService) Delete(ctx context.Context, attachmentID string) (*Response, error) {
	if strings.TrimSpace(attachmentID) == "" {
		return nil, errors.New("attachment delete: attachment id is required")
	}
	req, err := s.client.NewRequest(ctx, http.MethodDelete, RESTPath("attachment", attachmentID), nil)
	if err != nil {
		return nil, err
	}
	return s.client.Do(req, nil)
}

// Download returns the raw response body for the caller to stream via
// io.Copy. : streaming is the single code path — no in-memory
// buffering, no size threshold. The caller MUST close the returned
// ReadCloser.
//
// We don't go through Client.Do here because Do reads and discards the
// body for JSON unmarshalling; we need the unread body to stream into
// the caller's destination.
func (s *attachmentService) Download(ctx context.Context, attachmentID string) (io.ReadCloser, *Response, error) {
	if strings.TrimSpace(attachmentID) == "" {
		return nil, nil, errors.New("attachment download: attachment id is required")
	}
	req, err := s.client.NewRawRequest(ctx, http.MethodGet, RESTPath("attachment", "content", attachmentID), nil)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Accept", "*/*")

	res, err := s.client.client.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("jira request %s %s: %w", req.Method, req.URL.EscapedPath(), err)
	}
	resp := &Response{Response: res, Rate: parseRate(res)}
	if res.StatusCode < 200 || res.StatusCode > 299 {
		// Drain the error body so the caller can route by APIError;
		// mirror Client.Do's body cap to keep stderr / log output
		// bounded.
		body, _, readErr := readLimitedBody(res.Body, maxErrorBodyBytes)
		_ = res.Body.Close()
		if readErr != nil {
			return nil, resp, &APIError{
				StatusCode: res.StatusCode,
				Type:       ErrorTypeServer,
				Message:    "read response body: " + readErr.Error(),
				Cause:      readErr,
			}
		}
		msgBody := redactSensitiveBytes(body)
		ec := parseErrorCollection(msgBody)
		return nil, resp, &APIError{
			StatusCode:         res.StatusCode,
			Type:               classifyStatus(res.StatusCode),
			Message:            strings.TrimSpace(string(msgBody)),
			ErrorMessages:      ec.ErrorMessages,
			FieldErrors:        ec.Errors,
			UpstreamStatus:     ec.Status,
			RetryAfterSeconds:  resp.Rate.RetryAfterSeconds,
			RateLimitRemaining: resp.Rate.Remaining,
		}
	}
	return res.Body, resp, nil
}
