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

func TestMutationCompletionUsesSuccessLevelOnlyWhenReal(t *testing.T) {
	var buf bytes.Buffer
	real := map[string]any{"issue": "PROJ-1", "added": true}
	if err := WriteCommandPlain(&buf, "epic.add", real); err != nil {
		t.Fatalf("WriteCommandPlain() error = %v", err)
	}
	if !strings.Contains(buf.String(), "SCS") {
		t.Fatalf("real mutation completion did not log at success level:\n%s", buf.String())
	}

	buf.Reset()
	preview := map[string]any{"issue": "PROJ-1", "dry_run": true}
	if err := WriteCommandPlain(&buf, "epic.add", preview); err != nil {
		t.Fatalf("WriteCommandPlain(dry-run) error = %v", err)
	}
	if strings.Contains(buf.String(), "SCS") {
		t.Fatalf("dry-run preview claimed success level:\n%s", buf.String())
	}

	buf.Reset()
	read := map[string]any{"count": 2}
	if err := WriteCommandPlain(&buf, "issue.list.count", read); err != nil {
		t.Fatalf("WriteCommandPlain(read) error = %v", err)
	}
	if strings.Contains(buf.String(), "SCS") {
		t.Fatalf("informational read claimed success level:\n%s", buf.String())
	}
}
