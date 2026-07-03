// Package adf — registry of supported ADF nodes/marks.
//
// This table is the single source of truth for ADF support metadata.
// The contracts/adf-support-matrix.md doc is generated from it, and
// `jira agent adf-matrix --json` emits these rows. Every row's
// official_url points to an Atlassian source only. Every MVP node/mark
// MUST have a row. The envelope shape is shared with pkg/jira/customfield;
// submit_description disambiguates the submit capability per registry.

package adf

import "encoding/json"

// Kind tags whether a registry row describes a node or a mark.
type Kind int

const (
	KindNode Kind = iota
	KindMark
)

func (k Kind) String() string {
	switch k {
	case KindNode:
		return "node"
	case KindMark:
		return "mark"
	default:
		return "unknown"
	}
}

func (k Kind) MarshalJSON() ([]byte, error) {
	return json.Marshal(k.String())
}

// Capabilities is the shared envelope's per-row capability struct.
// All booleans default to false; explicit support is opt-in.
type Capabilities struct {
	Author   bool `json:"author"`
	Render   bool `json:"render"`
	Preserve bool `json:"preserve"`
	Validate bool `json:"validate"`
	Submit   bool `json:"submit"`
}

// Status tags the support tier a row currently sits in. New rows start at
// "preserve-only" and graduate to "mvp" or higher tiers as authoring lands.
type Status string

const (
	StatusMVP          Status = "mvp"
	StatusPreserveOnly Status = "preserve-only"
)

// Entry is one row in the registry — the shared envelope shape.
// input_shape and output_shape are JSON Schema 2020-12 fragments.
type Entry struct {
	Kind              Kind            `json:"kind"`
	Name              string          `json:"name"`
	Status            Status          `json:"status"`
	Capabilities      Capabilities    `json:"capabilities"`
	InputShape        json.RawMessage `json:"input_shape,omitempty"`
	OutputShape       json.RawMessage `json:"output_shape,omitempty"`
	Warnings          []string        `json:"warnings,omitempty"`
	OfficialURL       string          `json:"official_url"`
	Notes             string          `json:"notes,omitempty"`
	SubmitDescription string          `json:"submit_description"`
}

// RegistryView is the read-only handle commands and tests use.
type RegistryView struct {
	entries []Entry
	index   map[string]Entry // key = "kind:name"
}

func (r RegistryView) All() []Entry {
	out := make([]Entry, len(r.entries))
	copy(out, r.entries)
	return out
}

func (r RegistryView) Lookup(kind Kind, name string) (Entry, bool) {
	e, ok := r.index[kindKey(kind, name)]
	return e, ok
}

func kindKey(k Kind, name string) string { return k.String() + ":" + name }

const (
	adfDocsBase  = "https://developer.atlassian.com/cloud/jira/platform/apis/document/nodes/"
	adfMarksBase = "https://developer.atlassian.com/cloud/jira/platform/apis/document/marks/"
)

// nodeRow / markRow keep the table below visually scannable.
func nodeRow(name string, caps Capabilities, status Status, notes string) Entry {
	return Entry{
		Kind:              KindNode,
		Name:              name,
		Status:            status,
		Capabilities:      caps,
		OfficialURL:       adfDocsBase + name + "/",
		Notes:             notes,
		SubmitDescription: "ADF: included in a Jira rich-text field payload after validation passes.",
	}
}

func markRow(name string, caps Capabilities, status Status, notes string) Entry {
	return Entry{
		Kind:              KindMark,
		Name:              name,
		Status:            status,
		Capabilities:      caps,
		OfficialURL:       adfMarksBase + name + "/",
		Notes:             notes,
		SubmitDescription: "ADF: applied to text inside a rich-text field payload after validation passes.",
	}
}

// fullSupport / authorOnly are the most common capability presets used in
// the table below. preserveOnly is reserved for opaque-passthrough rows
// (currently empty since full MVP support is mandated).
var (
	fullSupport = Capabilities{Author: true, Render: true, Preserve: true, Validate: true, Submit: true}
	// renderOnly is for nodes whose author surface lives elsewhere (e.g.,
	// tableRow is built by table authoring, not directly).
	renderOnly = Capabilities{Render: true, Preserve: true, Validate: true, Submit: true}
)

// registry holds all MVP rows. Adding a new entry MUST also update
// the customfield registry per the "new shared keys land in both"
// rule.
var registryEntries = []Entry{
	// MVP nodes.
	nodeRow("doc", fullSupport, StatusMVP, "ADF root."),
	nodeRow("paragraph", fullSupport, StatusMVP, ""),
	nodeRow("text", fullSupport, StatusMVP, ""),
	nodeRow("heading", fullSupport, StatusMVP, "attrs.level required (1-6)."),
	nodeRow("bulletList", fullSupport, StatusMVP, ""),
	nodeRow("orderedList", fullSupport, StatusMVP, ""),
	nodeRow("listItem", renderOnly, StatusMVP, "Authored as part of a bullet/ordered list."),
	nodeRow("codeBlock", fullSupport, StatusMVP, "attrs.language optional."),
	nodeRow("blockquote", fullSupport, StatusMVP, ""),
	nodeRow("hardBreak", fullSupport, StatusMVP, ""),
	nodeRow("rule", fullSupport, StatusMVP, ""),
	nodeRow("mention", fullSupport, StatusMVP, "attrs.id (accountId) and attrs.text required."),
	nodeRow("emoji", fullSupport, StatusMVP, "attrs.shortName required."),
	nodeRow("date", fullSupport, StatusMVP, "attrs.timestamp required (epoch ms as string)."),
	nodeRow("status", fullSupport, StatusMVP, "attrs.text and attrs.color required."),
	nodeRow("inlineCard", Capabilities{Author: true, Render: true, Preserve: true, Validate: true, Submit: true}, StatusMVP, "Field compatibility enforced; degrades to text+link in best-effort."),
	nodeRow("panel", fullSupport, StatusMVP, "attrs.panelType required (info/warning/error/success/note)."),
	nodeRow("table", fullSupport, StatusMVP, ""),
	nodeRow("tableRow", renderOnly, StatusMVP, "Authored as part of a table."),
	nodeRow("tableCell", renderOnly, StatusMVP, "Authored as part of a table."),
	nodeRow("tableHeader", renderOnly, StatusMVP, "Authored as part of a table."),

	// MVP marks.
	markRow("strong", fullSupport, StatusMVP, ""),
	markRow("em", fullSupport, StatusMVP, ""),
	markRow("strike", fullSupport, StatusMVP, ""),
	markRow("code", fullSupport, StatusMVP, "Combines only with the link mark; any other mark on code text is invalid."),
	markRow("link", fullSupport, StatusMVP, "attrs.href required."),
	markRow("textColor", fullSupport, StatusMVP, "attrs.color required (#RRGGBB)."),
	markRow("backgroundColor", fullSupport, StatusMVP, "attrs.color required (#RRGGBB)."),
	markRow("subsup", fullSupport, StatusMVP, "attrs.type=sub|sup."),
	markRow("underline", fullSupport, StatusMVP, ""),
}

// Registry returns the read-only registry view. The view is built once on
// first call (lazy index build) and shared.
func Registry() RegistryView {
	idx := make(map[string]Entry, len(registryEntries))
	for _, e := range registryEntries {
		idx[kindKey(e.Kind, e.Name)] = e
	}
	return RegistryView{entries: registryEntries, index: idx}
}
