package mcp

import (
	"path/filepath"
	"strings"
	"testing"
)

// setupNoteTestEnv isolates WS_CACHE_HOME/WS_CONFIG_HOME per test so note
// stores never touch the real user home directory, mirroring the isolation
// pattern used by manuals_workflow_manual_test.go.
func setupNoteTestEnv(t *testing.T) {
	t.Helper()
	useLeadProfile(t)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))
}

// mintRootKey initializes a git repo at a fresh temp dir, mints a lead
// session key bound to it via ws.ferrule, and returns both.
func mintRootKey(t *testing.T, s *Server, id int) (root, key string) {
	t.Helper()
	root = t.TempDir()
	initGit(t, root)
	key, _ = parseLoginResponse(t, callLogin(t, s, id, root, nil))
	return root, key
}

// TestNoteWriteSearchEraseRoundTripPerLayer verifies the ticket's core
// verification boundary for both non-tracked layers: a note.write ->
// note.search round trip returns the written record, and note.erase removes
// it so a subsequent search no longer returns it.
func TestNoteWriteSearchEraseRoundTripPerLayer(t *testing.T) {
	setupNoteTestEnv(t)
	s := NewServer(t.TempDir(), "test")
	_, key := mintRootKey(t, s, 1)

	for _, layer := range []string{"machine", "worktree"} {
		t.Run(layer, func(t *testing.T) {
			writeResp := callToolWithKey(t, s, 2, key, "note.write", map[string]any{
				"layer": layer,
				"notes": []any{
					map[string]any{"key": "roundtrip." + layer, "value": "hello " + layer, "priority": 3},
				},
			})
			if !strings.HasPrefix(writeResp, "wrote 1 note") {
				t.Fatalf("note.write(%s) unexpected response: %s", layer, writeResp)
			}
			if !strings.Contains(writeResp, "roundtrip."+layer) {
				t.Fatalf("note.write(%s) confirmation missing key: %s", layer, writeResp)
			}

			searchResp := callToolWithKey(t, s, 3, key, "note.search", map[string]any{
				"layer": layer,
				"glob":  "roundtrip.*",
			})
			if !strings.Contains(searchResp, "roundtrip."+layer) || !strings.Contains(searchResp, "hello "+layer) {
				t.Fatalf("note.search(%s) did not return the written record: %s", layer, searchResp)
			}

			eraseResp := callToolWithKey(t, s, 4, key, "note.erase", map[string]any{
				"layer": layer,
				"keys":  []any{"roundtrip." + layer},
			})
			if !strings.Contains(eraseResp, "roundtrip."+layer) {
				t.Fatalf("note.erase(%s) confirmation missing key: %s", layer, eraseResp)
			}

			searchAfterErase := callToolWithKey(t, s, 5, key, "note.search", map[string]any{
				"layer": layer,
				"glob":  "roundtrip.*",
			})
			if strings.Contains(searchAfterErase, "roundtrip."+layer) {
				t.Fatalf("note.search(%s) after erase still returned the record: %s", layer, searchAfterErase)
			}
		})
	}
}

// TestNoteWriteFullOverwriteUpdatesPriority verifies the MCP-level note.write
// contract matches wsnote.Write's full-overwrite (updates priority) storage
// semantics: a second write to the same key replaces value and priority.
func TestNoteWriteFullOverwriteUpdatesPriority(t *testing.T) {
	setupNoteTestEnv(t)
	s := NewServer(t.TempDir(), "test")
	_, key := mintRootKey(t, s, 1)

	callToolWithKey(t, s, 2, key, "note.write", map[string]any{
		"layer": "machine",
		"notes": []any{map[string]any{"key": "overwrite.me", "value": "v1", "priority": 1}},
	})
	callToolWithKey(t, s, 3, key, "note.write", map[string]any{
		"layer": "machine",
		"notes": []any{map[string]any{"key": "overwrite.me", "value": "v2", "priority": 9}},
	})

	searchResp := callToolWithKey(t, s, 4, key, "note.search", map[string]any{
		"layer": "machine",
		"glob":  "overwrite.me",
	})
	if !strings.Contains(searchResp, "v2") || !strings.Contains(searchResp, "priority 9") {
		t.Fatalf("note.search after overwrite = %s, want v2/priority 9 only", searchResp)
	}
	if strings.Contains(searchResp, "v1") {
		t.Fatalf("note.search after overwrite still shows stale v1: %s", searchResp)
	}
}

// TestNoteWorktreeLayerIsolatedAcrossWorktrees verifies the ticket's
// cross-worktree isolation requirement: a worktree-layer note written under
// one root's session key is invisible to note.search under a different
// root's key, while a machine-layer note written under either key is visible
// to both.
func TestNoteWorktreeLayerIsolatedAcrossWorktrees(t *testing.T) {
	setupNoteTestEnv(t)
	s := NewServer(t.TempDir(), "test")
	_, keyA := mintRootKey(t, s, 1)
	_, keyB := mintRootKey(t, s, 2)

	callToolWithKey(t, s, 3, keyA, "note.write", map[string]any{
		"layer": "worktree",
		"notes": []any{map[string]any{"key": "wt.only.a", "value": "root A worktree note", "priority": 1}},
	})
	callToolWithKey(t, s, 4, keyA, "note.write", map[string]any{
		"layer": "machine",
		"notes": []any{map[string]any{"key": "machine.shared", "value": "visible everywhere", "priority": 1}},
	})

	// The worktree note must be visible under root A's own key...
	searchAWorktree := callToolWithKey(t, s, 5, keyA, "note.search", map[string]any{"layer": "worktree", "glob": "wt.only.a"})
	if !strings.Contains(searchAWorktree, "wt.only.a") {
		t.Fatalf("note.search(worktree, root A) missing its own note: %s", searchAWorktree)
	}

	// ...but absent under root B's key (different worktree note store).
	searchBWorktree := callToolWithKey(t, s, 6, keyB, "note.search", map[string]any{"layer": "worktree", "glob": "wt.only.a"})
	if strings.Contains(searchBWorktree, "wt.only.a") {
		t.Fatalf("note.search(worktree, root B) leaked root A's worktree note: %s", searchBWorktree)
	}

	// The machine-layer note is visible under both keys.
	searchAMachine := callToolWithKey(t, s, 7, keyA, "note.search", map[string]any{"layer": "machine", "glob": "machine.shared"})
	if !strings.Contains(searchAMachine, "machine.shared") {
		t.Fatalf("note.search(machine, root A) missing shared machine note: %s", searchAMachine)
	}
	searchBMachine := callToolWithKey(t, s, 8, keyB, "note.search", map[string]any{"layer": "machine", "glob": "machine.shared"})
	if !strings.Contains(searchBMachine, "machine.shared") {
		t.Fatalf("note.search(machine, root B) missing shared machine note: %s", searchBMachine)
	}
}

// TestNoteWriteRejectsInvalidLayer verifies layer validation runs before any
// store I/O.
func TestNoteWriteRejectsInvalidLayer(t *testing.T) {
	setupNoteTestEnv(t)
	s := NewServer(t.TempDir(), "test")
	_, key := mintRootKey(t, s, 1)

	resp := callToolWithKey(t, s, 2, key, "note.write", map[string]any{
		"layer": "repo",
		"notes": []any{map[string]any{"key": "a", "value": "b"}},
	})
	if !strings.Contains(resp, `must be "machine" or "worktree"`) {
		t.Fatalf("note.write(layer=repo) = %s, want a layer-validation error", resp)
	}
}

// TestNoteWriteMachineLayerRequiresKnownSessionKey verifies the machine layer
// still enforces the "every ws tool call carries a session key" invariant
// even though it needs no resolved root.
func TestNoteWriteMachineLayerRequiresKnownSessionKey(t *testing.T) {
	setupNoteTestEnv(t)
	s := NewServer(t.TempDir(), "test")

	resp := callToolWithKey(t, s, 1, "not-a-real-key", "note.write", map[string]any{
		"layer": "machine",
		"notes": []any{map[string]any{"key": "a", "value": "b"}},
	})
	if !strings.Contains(resp, "unknown_session") {
		t.Fatalf("note.write(machine, unknown key) = %s, want unknown_session error", resp)
	}
}
