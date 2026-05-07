package cli

import (
	"encoding/json"
	"io"

	"github.com/gechr/clog"
)

func WriteRaw(w io.Writer, data any) error {
	switch v := data.(type) {
	case nil:
		clog.New(clog.NewOutput(w, clog.ColorAuto)).Print().Mode(clog.JSONFlat).JSON(nil)
		return nil
	case []byte:
		if !json.Valid(v) {
			return WriteCompact(w, string(v))
		}
		clog.New(clog.NewOutput(w, clog.ColorAuto)).Print().Mode(clog.JSONPreserve).RawJSON(v)
		return nil
	case json.RawMessage:
		if !json.Valid(v) {
			return WriteCompact(w, string(v))
		}
		clog.New(clog.NewOutput(w, clog.ColorAuto)).Print().Mode(clog.JSONPreserve).RawJSON(v)
		return nil
	default:
		return WriteCompact(w, data)
	}
}
