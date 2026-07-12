package cli

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/gechr/clog"
	"github.com/gechr/x/human"
)

// WriteAttachmentListPlain renders the `issue.attachment.list` envelope
// for human consumption. Layout: a header line with the attachment
// count, followed by one row per attachment with id, filename, human
// bytes, uploader displayName, and a relative-time created marker.
//
// Mirrors the plain_watcher / plain_link signature so the dispatcher
// in plain.go can wire it with the same `(w, command, data, opts)`
// shape.
func WriteAttachmentListPlain(w io.Writer, command string, data any, opts ...PlainOption) error {
	cfg := defaultPlainConfig()
	for _, opt := range opts {
		opt(&cfg)
	}
	logger := newPlainLogger(w)

	m, ok := data.(map[string]any)
	if !ok {
		return writeGenericPlain(logger, cfg, messageForCommand(command, data), data)
	}
	rows := normalizeMapList(m["attachments"])
	style := authPlainStyle{tty: cfg.tty, theme: cfg.theme}

	count := len(rows)
	if pagination, ok := m["pagination"].(map[string]any); ok {
		if t, ok := pagination["total"].(float64); ok {
			count = int(t)
		} else if t, ok := pagination["total"].(int); ok {
			count = t
		}
	}
	title := "Attachments"
	if cfg.resultKey != "" {
		title += " on " + cfg.resultKey
	}
	header := style.bold(title) + style.dim("  ("+human.Pluralize(count, "attachment", "attachments")+")")
	logger.Info().Parts(clog.PartMessage).Msg(header)

	if len(rows) == 0 {
		logger.Info().Parts(clog.PartMessage).Msg(style.dim("  (no attachments)"))
		return nil
	}

	for _, row := range rows {
		logger.Info().Parts(clog.PartMessage).Msg(attachmentPlainLine(row, style))
	}
	return nil
}

// attachmentPlainLine renders one attachment row.
func attachmentPlainLine(m map[string]any, style authPlainStyle) string {
	id := attachmentStringField(m, "id")
	filename := attachmentStringField(m, "filename")
	if filename == "" {
		filename = "(unnamed)"
	}
	size := attachmentInt64Field(m, "size")
	created := attachmentStringField(m, "created")

	author := ""
	if a, ok := m["author"].(map[string]any); ok {
		author = attachmentStringField(a, "display_name")
		if author == "" {
			author = attachmentStringField(a, "account_id")
		}
	}
	if author == "" {
		author = "(unknown)"
	}

	return fmt.Sprintf(
		"  %s  %s  %s  by %s  %s",
		style.bold(id),
		filename,
		style.dim(attachmentHumanBytes(size)),
		author,
		style.dim(attachmentHumanCreated(created)),
	)
}

// attachmentStringField is a nil-safe map[string]any string extractor.
func attachmentStringField(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return strings.TrimSpace(v)
	}
	return ""
}

// attachmentInt64Field tolerates the float64 that JSON round-trips
// produce while also accepting an int64 from native struct conversion.
func attachmentInt64Field(m map[string]any, key string) int64 {
	switch v := m[key].(type) {
	case int64:
		return v
	case int:
		return int64(v)
	case float64:
		return int64(v)
	}
	return 0
}

func attachmentHumanBytes(n int64) string {
	return human.FormatIECBytes(float64(n))
}

// attachmentHumanCreated returns "Xm ago" / "Xh ago" / "Xd ago" /
// "YYYY-MM-DD" depending on how recent the timestamp is. Falls back to
// the raw string when parsing fails so we never lose information.
func attachmentHumanCreated(ts string) string {
	return attachmentHumanCreatedFrom(ts, time.Now().UTC())
}

func attachmentHumanCreatedFrom(ts string, now time.Time) string {
	if ts == "" {
		return ""
	}
	for _, layout := range []string{
		time.RFC3339,
		"2006-01-02T15:04:05.000-0700",
		"2006-01-02T15:04:05.000+0000",
	} {
		t, err := time.Parse(layout, ts)
		if err != nil {
			continue
		}
		delta := now.Sub(t)
		if delta < 0 || delta >= 7*24*time.Hour {
			return t.Format("2006-01-02")
		}
		return human.FormatTimeAgoCompactFrom(t, now)
	}
	return ts
}
