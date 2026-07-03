package adf

import (
	"encoding/json"
	"fmt"
)

// Warning is a structured non-fatal diagnostic emitted by best-effort ADF
// processing. Strict mode promotes warnings to errors. The shape matches
// cli.Warning byte-for-byte but the type lives here to avoid an import
// cycle; commands convert via cli.WarningFrom.
type Warning struct {
	Type     string `json:"type"`
	Message  string `json:"message"`
	Field    string `json:"field,omitempty"`
	Path     string `json:"path,omitempty"`
	NodeType string `json:"node_type,omitempty"`
	MarkType string `json:"mark_type,omitempty"`
	Lossy    bool   `json:"lossy"`
}

// LossyConversionError is the strict-mode abort for a lossy Markdown→ADF
// conversion: it carries the source-mapped warning so the error mapper can
// surface the offending Markdown line and a remediation hint instead of a
// bare message with none.
type LossyConversionError struct {
	Warning Warning
}

func (e LossyConversionError) Error() string { return e.Warning.Message }

// Implement cli.WarningSource so commands can do cli.WarningFrom(adfW)
// without either package importing the other's concrete type.
func (w Warning) WarningType() string     { return w.Type }
func (w Warning) WarningMessage() string  { return w.Message }
func (w Warning) WarningField() string    { return w.Field }
func (w Warning) WarningPath() string     { return w.Path }
func (w Warning) WarningNodeType() string { return w.NodeType }
func (w Warning) WarningMarkType() string { return w.MarkType }
func (w Warning) WarningIsLossy() bool    { return w.Lossy }

// Parse decodes ADF JSON into a typed Document and returns any best-effort
// warnings collected during the decode. Unknown nodes/marks are preserved
// opaquely and surface no warnings on parse — they only matter at
// render/submit time.
func Parse(data []byte) (Document, []Warning, error) {
	var doc Document
	if err := json.Unmarshal(data, &doc); err != nil {
		return Document{}, nil, fmt.Errorf("adf parse: %w", err)
	}
	return doc, nil, nil
}

// Marshal serializes a Document back to JSON, preserving opaque subtrees.
func Marshal(doc Document) ([]byte, error) {
	return json.Marshal(doc)
}
