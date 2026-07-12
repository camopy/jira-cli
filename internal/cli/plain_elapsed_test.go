package cli

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestGenericPlainShowsElapsedAtOrAboveThreshold(t *testing.T) {
	var buf bytes.Buffer
	data := map[string]any{"issue": "PROJ-1", "added": true}
	err := WriteCommandPlain(&buf, "epic.add", data, WithPlainElapsed(2*time.Second))
	if err != nil {
		t.Fatalf("WriteCommandPlain() error = %v", err)
	}
	if !strings.Contains(buf.String(), "elapsed=2s") {
		t.Fatalf("slow command did not surface elapsed:\n%s", buf.String())
	}
}

func TestGenericPlainHidesSubSecondElapsed(t *testing.T) {
	var buf bytes.Buffer
	data := map[string]any{"issue": "PROJ-1", "added": true}
	err := WriteCommandPlain(&buf, "epic.add", data, WithPlainElapsed(200*time.Millisecond))
	if err != nil {
		t.Fatalf("WriteCommandPlain() error = %v", err)
	}
	if strings.Contains(buf.String(), "elapsed") {
		t.Fatalf("fast command leaked an elapsed field below clog's 1s minimum:\n%s", buf.String())
	}
}

func TestKeyedSummaryCarriesElapsedOnceNotPerChild(t *testing.T) {
	var buf bytes.Buffer
	data := map[string]any{
		"results": []any{
			map[string]any{"key": "PROJ-1", "ok": true, "data": map[string]any{"issue": "PROJ-1", "added": true}},
			map[string]any{"key": "PROJ-2", "ok": true, "data": map[string]any{"issue": "PROJ-2", "added": true}},
		},
		"succeeded": 2,
		"failed":    0,
	}
	err := WriteCommandPlain(&buf, "epic.add", data, WithPlainElapsed(3*time.Second))
	if err != nil {
		t.Fatalf("WriteCommandPlain() error = %v", err)
	}
	if got := strings.Count(buf.String(), "elapsed="); got != 1 {
		t.Fatalf("keyed output carried elapsed %d times, want exactly once on the summary line:\n%s", got, buf.String())
	}
}
