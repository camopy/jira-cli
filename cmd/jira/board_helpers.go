// Helpers shared between commands that consume `--board` / `--board-id`
// (issue list, jql build) plus the future surfaces. The `boardScopeFromFlags`
// entrypoint is the single place that resolves the user's board input
// against the cache and decides which precedence path won (flag, default,
// or none).
package main

import (
	"context"
	stdliberrors "errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/matcra587/jira-cli/internal/cache"
	"github.com/matcra587/jira-cli/internal/config"
	"github.com/matcra587/jira-cli/pkg/jira"
)

// PrecedenceFlag / PrecedenceDefault / PrecedenceNone are the three
// values the envelope's `data.precedence` field carries when a `--board`
// scope is in play.
const (
	precedenceFlag    = "flag"
	precedenceDefault = "default_board"
	precedenceNone    = "none"
)

// boardValidationError is a typed error that asks outputErrorFor to
// classify the failure as ErrorTypeValidation (exit 3) — used for both
// default-board-missing and ambiguous-name disambiguation.
type boardValidationError struct {
	msg        string
	candidates []map[string]any
}

func (e boardValidationError) Error() string                     { return e.msg }
func (e boardValidationError) BoardCandidates() []map[string]any { return e.candidates }

// boardScopeFromFlags reads the active command's `--board NAME` /
// `--board-id N` flag values, falls through to the active profile's
// `default_board` config (when defined), resolves the requested board
// against the cache, and returns the resulting (scope, precedence, err).
//
// Precedence rules (data-model.md > Precedence with --board flags):
//
//	--board NAME explicit → flag wins, precedence "flag"
//	--board-id N  explicit → flag wins, precedence "flag"
//	--board ""    explicit → suppresses default, precedence "none"
//	neither + default set → resolves default, precedence "default_board"
//	neither + no default  → no scope, precedence "none"
//
// The default_board config field is owned by a later change; until then this
// helper treats a missing or empty value as "no default applied".
func boardScopeFromFlags(cmd *cobra.Command) (jira.BoardScope, string, error) {
	flags := cmd.Flags()

	// Distinguish "flag absent" from "flag present with empty value" —
	// the latter explicitly suppresses any configured default_board.
	var (
		boardName    string
		boardNameSet bool
		boardID      int
		boardIDSet   bool
	)
	if f := flags.Lookup("board"); f != nil {
		boardName = f.Value.String()
		boardNameSet = f.Changed
	}
	if f := flags.Lookup("board-id"); f != nil {
		raw := f.Value.String()
		if f.Changed && raw != "" {
			boardIDSet = true
			if v, err := strconv.Atoi(raw); err == nil {
				boardID = v
			}
		}
	}

	// Mutual exclusion is enforced by cobra at parse time via
	// MarkFlagsMutuallyExclusive; defensive double-check kept here so a
	// future caller wiring the flags without that bind still gets a
	// useful error.
	if boardNameSet && boardIDSet {
		return jira.BoardScope{}, precedenceNone, fmt.Errorf("--board and --board-id are mutually exclusive")
	}

	// Resolve config (active profile) — needed for the default_board
	// fallthrough AND to drive the resolver. Failing config load is
	// non-fatal here: we treat it as "no default" and only return an
	// error if the user explicitly asked for a board.
	cfg, _ := config.Load(config.WithPath(configPath(cmd)))
	profile := config.Profile{}
	profileName := "default"
	if cfg != nil {
		profile = activeProfile(cmd, cfg)
		if profile.Name != "" {
			profileName = profile.Name
		}
	}

	// Flag path: --board NAME (when non-empty) or --board-id N.
	switch {
	case boardIDSet:
		scope, err := resolveBoardByID(cmd.Context(), profileName, boardID)
		if err != nil {
			return jira.BoardScope{}, precedenceFlag, err
		}
		scope.Precedence = precedenceFlag
		return scope, precedenceFlag, nil
	case boardNameSet && strings.TrimSpace(boardName) == "":
		// `--board ""` explicitly suppresses any default. precedence "none".
		return jira.BoardScope{}, precedenceNone, nil
	case boardNameSet:
		// ResolveOne is cache-only — no client needed, no credential
		// backend touched. Pinned by tests/unit/board_resolver_test.go
		// which fails the test if the resolver hits the network.
		svc := jira.NewBoardService(nil)
		scope, err := svc.ResolveOne(cmd.Context(), profileName, boardName)
		if err != nil {
			return jira.BoardScope{}, precedenceFlag, classifyBoardErr(err)
		}
		scope.Precedence = precedenceFlag
		return scope, precedenceFlag, nil
	}

	// Default-board fallthrough — when no flag is supplied and the
	// profile carries a configured default_board, resolve that against
	// the cache. Empty / unset → no scope, precedence "none".
	if def := strings.TrimSpace(profile.DefaultBoard); def != "" {
		svc := jira.NewBoardService(nil)
		scope, err := svc.ResolveOne(cmd.Context(), profileName, def)
		if err != nil {
			// Pinned wording when default_board doesn't resolve.
			if stdliberrors.Is(err, jira.ErrBoardNotFound) {
				return jira.BoardScope{}, precedenceDefault, boardValidationError{
					msg: jira.DefaultBoardMissingMessage(profileName, def),
				}
			}
			return jira.BoardScope{}, precedenceDefault, classifyBoardErr(err)
		}
		scope.Precedence = precedenceDefault
		return scope, precedenceDefault, nil
	}

	return jira.BoardScope{}, precedenceNone, nil
}

// classifyBoardErr maps a BoardService.ResolveOne error onto the right
// CLI exit code. Ambiguous-name → validation (exit 3). The bare
// ErrBoardNotFound returned for explicit --board NAME / --board-id N
// remains classified as not_found (exit 2) by outputErrorFor's default
// substring rules.
func classifyBoardErr(err error) error {
	var ambig *jira.AmbiguousBoardError
	if stdliberrors.As(err, &ambig) {
		cands := make([]map[string]any, 0, len(ambig.Candidates))
		for _, b := range ambig.Candidates {
			row := map[string]any{}
			if b.ID != nil {
				row["id"] = *b.ID
			}
			if b.Name != nil {
				row["name"] = *b.Name
			}
			if b.Type != nil {
				row["type"] = *b.Type
			}
			row["project_keys"] = b.ProjectKeys
			cands = append(cands, row)
		}
		return boardValidationError{msg: err.Error(), candidates: cands}
	}
	return err
}

// resolveBoardByID does a numeric-id lookup against the local cache.
// Mirrors BoardService.ResolveOne but keyed off the unambiguous id
// rather than the name. Cache-only — never round-trips to the server.
func resolveBoardByID(_ context.Context, profile string, id int) (jira.BoardScope, error) {
	entry, ok, _, err := cache.Read(profile, "boards", 0)
	if err != nil {
		return jira.BoardScope{}, fmt.Errorf("board resolve by id: %w", err)
	}
	if !ok {
		return jira.BoardScope{}, fmt.Errorf("%w (boards cache empty — run `jira cache boards`)", jira.ErrBoardNotFound)
	}
	boards, err := jira.DecodeBoardsCache(entry.Data)
	if err != nil {
		return jira.BoardScope{}, fmt.Errorf("board resolve by id: decode cache: %w", err)
	}
	for _, b := range boards {
		if b.ID != nil && *b.ID == id {
			return jira.BoardScope{Board: b}, nil
		}
	}
	return jira.BoardScope{}, fmt.Errorf("%w: id=%d", jira.ErrBoardNotFound, id)
}

// boardScopeEnvelopeData renders a BoardScope into the envelope's
// `data.board_scope` map. The `applied` flag tracks whether a JQL
// clause was actually emitted — JQLClause returns empty when
// ProjectKeys is empty, so a resolved-but-unscoped board surfaces with
// `applied: false` plus the board metadata, letting the user see what
// matched without conflating identification with scoping.
func boardScopeEnvelopeData(scope jira.BoardScope) map[string]any {
	resolved := (scope.Board.ID != nil && *scope.Board.ID != 0) ||
		(scope.Board.Name != nil && *scope.Board.Name != "")
	applied := len(scope.Board.ProjectKeys) > 0
	if !resolved && !applied {
		return map[string]any{"applied": false}
	}
	out := map[string]any{
		"applied":      applied,
		"project_keys": scope.Board.ProjectKeys,
	}
	if out["project_keys"] == nil {
		out["project_keys"] = []string{}
	}
	if scope.Board.ID != nil {
		out["id"] = *scope.Board.ID
	}
	if scope.Board.Name != nil {
		out["name"] = *scope.Board.Name
	}
	if scope.Board.Type != nil {
		out["type"] = *scope.Board.Type
	}
	return out
}
