package core

import "testing"

func TestTaskManagerAcceptsLatestGenerationOnly(t *testing.T) {
	m := NewTaskManager()
	const scope TaskScope = "issues"

	m.Start(TaskSpec{Scope: scope})
	gen1 := m.Generation(scope)
	m.Start(TaskSpec{Scope: scope})
	gen2 := m.Generation(scope)

	if gen2 <= gen1 {
		t.Fatalf("expected generation to advance: gen1=%d gen2=%d", gen1, gen2)
	}
	if m.Accept(scope, gen1) {
		t.Errorf("stale generation %d was accepted; a superseded fetch must be dropped", gen1)
	}
	if !m.Accept(scope, gen2) {
		t.Errorf("latest generation %d was rejected", gen2)
	}
}

func TestTaskManagerScopesAreIndependent(t *testing.T) {
	m := NewTaskManager()
	m.Start(TaskSpec{Scope: "issues"})
	m.Start(TaskSpec{Scope: "search"})

	if got := m.Generation("issues"); got != 1 {
		t.Errorf("issues generation = %d, want 1", got)
	}
	if got := m.Generation("search"); got != 1 {
		t.Errorf("search generation = %d, want 1", got)
	}
	if !m.Accept("issues", 1) || !m.Accept("search", 1) {
		t.Error("each scope should accept its own latest generation")
	}
}

func TestTaskManagerStartRunsAndReportsResult(t *testing.T) {
	m := NewTaskManager()
	cmd := m.Start(TaskSpec{
		Scope: "issues",
		Run:   func() (any, error) { return 42, nil },
	})
	if cmd == nil {
		t.Fatal("Start returned a nil command")
	}
	// Start records the generation immediately; executing the command produces
	// the TaskFinishedMsg carrying that generation.
	if m.Generation("issues") != 1 {
		t.Errorf("generation = %d, want 1", m.Generation("issues"))
	}
	if fin, ok := cmd().(TaskFinishedMsg); !ok {
		t.Errorf("command did not produce a TaskFinishedMsg, got %T", cmd())
	} else if fin.Result != 42 || fin.Gen != 1 {
		t.Errorf("finished msg = %+v, want Result=42 Gen=1", fin)
	}
}
