package integration

// service-level walkthrough for 003 boards. Drives the
// `BoardService` end-to-end against an httptest server: ListAll →
// ProjectsForBoard → ResolveOne. Mirrors the 002
// `issue_lifecycle_test.go` shape so the integration suite stays
// proportional across features.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/matcra587/jira-cli/internal/cache"
	"github.com/matcra587/jira-cli/pkg/jira"
)

func TestBoardsLifecycleEndToEnd(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/rest/agile/1.0/board", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"maxResults":50,"startAt":0,"isLast":true,"values":[
			{"id":42,"self":"http://x/board/42","name":"Engineering Sprint","type":"scrum"},
			{"id":99,"self":"http://x/board/99","name":"Platform Roadmap","type":"kanban"}
		]}`))
	})
	mux.HandleFunc("/rest/agile/1.0/board/42/project", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"maxResults":50,"startAt":0,"isLast":true,"values":[{"key":"ENG"},{"key":"PLAT"}]}`))
	})
	mux.HandleFunc("/rest/agile/1.0/board/99/project", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"maxResults":50,"startAt":0,"isLast":true,"values":[{"key":"PLAT"}]}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := jira.NewClient(jira.WithBaseURL(srv.URL + "/"))
	svc := jira.NewBoardService(client)
	ctx := context.Background()

	// 1. Drain the board list.
	res, err := svc.ListAll(ctx, jira.BoardDrainOptions{})
	if err != nil {
		t.Fatalf("ListAll: %v", err)
	}
	if len(res.Boards) != 2 {
		t.Fatalf("Boards len = %d; want 2", len(res.Boards))
	}

	// 2. Populate per-board project lists (cache-prime emulation).
	for _, b := range res.Boards {
		if b.ID == nil {
			t.Fatalf("board missing id: %+v", b)
		}
		keys, _, err := svc.ProjectsForBoard(ctx, *b.ID)
		if err != nil {
			t.Fatalf("ProjectsForBoard(%d): %v", *b.ID, err)
		}
		b.ProjectKeys = keys
	}

	// 3. Persist a cache file (mimics the prime command's tail).
	cachePath := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cachePath)
	boardsForCache := []jira.Board{}
	for _, b := range res.Boards {
		boardsForCache = append(boardsForCache, *b)
	}
	raw, err := json.Marshal(boardsForCache)
	if err != nil {
		t.Fatalf("marshal boards: %v", err)
	}
	if _, err := cache.Write("default", "boards", raw); err != nil {
		t.Fatalf("cache.Write: %v", err)
	}

	// 4. Resolve a board by exact-case-insensitive name.
	scope, err := svc.ResolveOne(ctx, "default", "engineering sprint")
	if err != nil {
		t.Fatalf("ResolveOne: %v", err)
	}
	if scope.Board.ID == nil || *scope.Board.ID != 42 {
		t.Fatalf("ResolveOne id = %v; want 42", scope.Board.ID)
	}
	clause, ok := scope.JQLClause()
	if !ok {
		t.Fatalf("JQLClause ok = false; want true (board has 2 project keys)")
	}
	if want := "project in (ENG, PLAT)"; clause != want {
		t.Fatalf("JQLClause = %q; want %q", clause, want)
	}

	// 5. Resolution failure paths: not-found + ambiguous absence.
	if _, err := svc.ResolveOne(ctx, "default", "Nonexistent"); err == nil {
		t.Fatalf("expected ErrBoardNotFound for missing board")
	}
}
