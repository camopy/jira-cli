package adf

import (
	"bytes"
	"encoding/json"
	"fmt"

	xmaps "github.com/gechr/x/maps"
)

// Document is the canonical ADF root.
type Document struct {
	Type    string `json:"type"`
	Version int    `json:"version"`
	Content []Node `json:"content,omitempty"`
}

// Node is one ADF block or inline element. Unknown JSON keys outside the
// well-known fields are preserved opaquely in extra so re-marshaling never
// loses semantics (lossless preservation).
type Node struct {
	Type    string         `json:"type"`
	Text    string         `json:"text,omitempty"`
	Attrs   map[string]any `json:"attrs,omitempty"`
	Marks   []Mark         `json:"marks,omitempty"`
	Content []Node         `json:"content,omitempty"`
	// extra holds keys we don't recognize — e.g. attrs of unknown nodes that
	// the schema may add later. Preserved on Marshal so opaque subtrees
	// round-trip byte-equivalently.
	extra map[string]json.RawMessage
}

// Mark is an inline annotation. Same opaque-preservation rules as Node.
type Mark struct {
	Type  string         `json:"type"`
	Attrs map[string]any `json:"attrs,omitempty"`
	extra map[string]json.RawMessage
}

// nodeKnownKeys / markKnownKeys are the JSON keys the typed model decodes
// into struct fields. Anything else is captured into extra.
var (
	nodeKnownKeys = map[string]struct{}{
		"type": {}, "text": {}, "attrs": {}, "marks": {}, "content": {},
	}
	markKnownKeys = map[string]struct{}{
		"type": {}, "attrs": {},
	}
)

func (n *Node) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*n = Node{}
	if v, ok := raw["type"]; ok {
		if err := json.Unmarshal(v, &n.Type); err != nil {
			return fmt.Errorf("adf: node.type: %w", err)
		}
	}
	if v, ok := raw["text"]; ok {
		if err := json.Unmarshal(v, &n.Text); err != nil {
			return fmt.Errorf("adf: node.text: %w", err)
		}
	}
	if v, ok := raw["attrs"]; ok {
		if err := json.Unmarshal(v, &n.Attrs); err != nil {
			return fmt.Errorf("adf: node.attrs: %w", err)
		}
	}
	if v, ok := raw["marks"]; ok {
		if err := json.Unmarshal(v, &n.Marks); err != nil {
			return fmt.Errorf("adf: node.marks: %w", err)
		}
	}
	if v, ok := raw["content"]; ok {
		if err := json.Unmarshal(v, &n.Content); err != nil {
			return fmt.Errorf("adf: node.content: %w", err)
		}
	}
	for k, v := range raw {
		if _, known := nodeKnownKeys[k]; known {
			continue
		}
		if n.extra == nil {
			n.extra = map[string]json.RawMessage{}
		}
		n.extra[k] = v
	}
	return nil
}

func (n Node) MarshalJSON() ([]byte, error) {
	buf := &bytes.Buffer{}
	buf.WriteByte('{')
	first := true
	emit := func(key string, value any) error {
		if !first {
			buf.WriteByte(',')
		}
		first = false
		k, _ := json.Marshal(key)
		buf.Write(k)
		buf.WriteByte(':')
		v, err := json.Marshal(value)
		if err != nil {
			return err
		}
		buf.Write(v)
		return nil
	}
	emitRaw := func(key string, value json.RawMessage) {
		if !first {
			buf.WriteByte(',')
		}
		first = false
		k, _ := json.Marshal(key)
		buf.Write(k)
		buf.WriteByte(':')
		buf.Write(value)
	}
	if err := emit("type", n.Type); err != nil {
		return nil, err
	}
	if n.Text != "" {
		if err := emit("text", n.Text); err != nil {
			return nil, err
		}
	}
	if len(n.Attrs) > 0 {
		if err := emit("attrs", n.Attrs); err != nil {
			return nil, err
		}
	}
	if len(n.Marks) > 0 {
		if err := emit("marks", n.Marks); err != nil {
			return nil, err
		}
	}
	if len(n.Content) > 0 {
		if err := emit("content", n.Content); err != nil {
			return nil, err
		}
	}
	if len(n.extra) > 0 {
		for k, v := range xmaps.Sorted(n.extra) {
			emitRaw(k, v)
		}
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

func (m *Mark) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*m = Mark{}
	if v, ok := raw["type"]; ok {
		if err := json.Unmarshal(v, &m.Type); err != nil {
			return fmt.Errorf("adf: mark.type: %w", err)
		}
	}
	if v, ok := raw["attrs"]; ok {
		if err := json.Unmarshal(v, &m.Attrs); err != nil {
			return fmt.Errorf("adf: mark.attrs: %w", err)
		}
	}
	for k, v := range raw {
		if _, known := markKnownKeys[k]; known {
			continue
		}
		if m.extra == nil {
			m.extra = map[string]json.RawMessage{}
		}
		m.extra[k] = v
	}
	return nil
}

func (m Mark) MarshalJSON() ([]byte, error) {
	buf := &bytes.Buffer{}
	buf.WriteByte('{')
	first := true
	emit := func(key string, value any) error {
		if !first {
			buf.WriteByte(',')
		}
		first = false
		k, _ := json.Marshal(key)
		buf.Write(k)
		buf.WriteByte(':')
		v, err := json.Marshal(value)
		if err != nil {
			return err
		}
		buf.Write(v)
		return nil
	}
	emitRaw := func(key string, value json.RawMessage) {
		if !first {
			buf.WriteByte(',')
		}
		first = false
		k, _ := json.Marshal(key)
		buf.Write(k)
		buf.WriteByte(':')
		buf.Write(value)
	}
	if err := emit("type", m.Type); err != nil {
		return nil, err
	}
	if len(m.Attrs) > 0 {
		if err := emit("attrs", m.Attrs); err != nil {
			return nil, err
		}
	}
	if len(m.extra) > 0 {
		for k, v := range xmaps.Sorted(m.extra) {
			emitRaw(k, v)
		}
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}
