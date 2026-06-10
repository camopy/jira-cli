package core

import "testing"

func TestRegistryBuildsRegisteredSection(t *testing.T) {
	r := NewRegistry()
	r.Register("issues", NewPlaceholderSection("issues", "Issues"))

	ctx := NewProgramContext(nil, nil)
	s, ok := r.Build("issues", ctx)
	if !ok {
		t.Fatal("registered section did not build")
	}
	if s.ID() != "issues" {
		t.Errorf("section ID = %q, want %q", s.ID(), "issues")
	}
}

func TestRegistryReportsUnknownSection(t *testing.T) {
	r := NewRegistry()
	if r.Has("boards") {
		t.Error("empty registry reports an unregistered section as present")
	}
	if _, ok := r.Build("boards", NewProgramContext(nil, nil)); ok {
		t.Error("Build returned ok for an unregistered section")
	}
}
