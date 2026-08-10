package mcp

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsnote"
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

// twoWorktreesOfOneRepo creates a git repo with an initial commit at one temp
// dir, then adds a SECOND LINKED WORKTREE of that same repository (`git
// worktree add`) at another temp dir, and returns both worktree roots. This
// is required (not two unrelated repos from mintRootKey) to test worktree-
// layer isolation at the correct granularity: two worktrees of one repo
// share a common git dir/ProjectKey but resolve to different WorktreeKeys,
// so this fixture would catch a bug that keyed the worktree note store off
// ProjectKey instead of WorktreeKey, whereas two unrelated repos (which
// already differ at the ProjectKey level) would mask that exact bug.
func twoWorktreesOfOneRepo(t *testing.T) (mainRoot, linkedRoot string) {
	t.Helper()
	mainRoot = t.TempDir()
	initGit(t, mainRoot)
	mustWrite(t, mainRoot, "README.md", "# test\n")
	runGit(t, mainRoot, "add", "README.md")
	runGit(t, mainRoot, "commit", "-m", "init")

	linkedRoot = filepath.Join(t.TempDir(), "linked")
	runGit(t, mainRoot, "worktree", "add", "-b", "note-isolation-linked", linkedRoot)
	return mainRoot, linkedRoot
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
// headline cross-worktree isolation requirement AT WORKTREE GRANULARITY: a
// worktree-layer note written under one linked worktree's session key is
// invisible to note.search under a DIFFERENT worktree OF THE SAME
// REPOSITORY's key, while a machine-layer note written under either key is
// visible to both. Using two unrelated repos here (as an earlier draft did)
// would already differ at the ProjectKey level and pass even for a bug that
// keyed the worktree store off ProjectKey instead of WorktreeKey — see
// twoWorktreesOfOneRepo's doc comment.
func TestNoteWorktreeLayerIsolatedAcrossWorktrees(t *testing.T) {
	setupNoteTestEnv(t)
	s := NewServer(t.TempDir(), "test")
	mainRoot, linkedRoot := twoWorktreesOfOneRepo(t)
	keyMain, _ := parseLoginResponse(t, callLogin(t, s, 1, mainRoot, nil))
	keyLinked, _ := parseLoginResponse(t, callLogin(t, s, 2, linkedRoot, nil))

	callToolWithKey(t, s, 3, keyMain, "note.write", map[string]any{
		"layer": "worktree",
		"notes": []any{map[string]any{"key": "wt.only.main", "value": "main worktree note", "priority": 1}},
	})
	callToolWithKey(t, s, 4, keyMain, "note.write", map[string]any{
		"layer": "machine",
		"notes": []any{map[string]any{"key": "machine.shared", "value": "visible everywhere", "priority": 1}},
	})

	// The worktree note must be visible under the main worktree's own key...
	searchMainWorktree := callToolWithKey(t, s, 5, keyMain, "note.search", map[string]any{"layer": "worktree", "glob": "wt.only.main"})
	if !strings.Contains(searchMainWorktree, "wt.only.main") {
		t.Fatalf("note.search(worktree, main) missing its own note: %s", searchMainWorktree)
	}

	// ...but absent under the linked worktree's key, even though both are the
	// SAME repository (different worktree note store, same project).
	searchLinkedWorktree := callToolWithKey(t, s, 6, keyLinked, "note.search", map[string]any{"layer": "worktree", "glob": "wt.only.main"})
	if strings.Contains(searchLinkedWorktree, "wt.only.main") {
		t.Fatalf("note.search(worktree, linked worktree of the same repo) leaked the main worktree's note: %s", searchLinkedWorktree)
	}

	// The machine-layer note is visible under both keys.
	searchMainMachine := callToolWithKey(t, s, 7, keyMain, "note.search", map[string]any{"layer": "machine", "glob": "machine.shared"})
	if !strings.Contains(searchMainMachine, "machine.shared") {
		t.Fatalf("note.search(machine, main) missing shared machine note: %s", searchMainMachine)
	}
	searchLinkedMachine := callToolWithKey(t, s, 8, keyLinked, "note.search", map[string]any{"layer": "machine", "glob": "machine.shared"})
	if !strings.Contains(searchLinkedMachine, "machine.shared") {
		t.Fatalf("note.search(machine, linked worktree) missing shared machine note: %s", searchLinkedMachine)
	}
}

// TestNoteWriteRestampsWrittenAtOnOverwrite verifies the MCP-layer
// server-side written_at stamping (noteRecordsArg in note_tools.go) actually
// re-stamps on a full-overwrite write, not just value/priority — the
// storage-layer wsnote tests exercise Write/Load directly with caller-chosen
// WrittenAt values and do not cover this handler-level behavior.
func TestNoteWriteRestampsWrittenAtOnOverwrite(t *testing.T) {
	setupNoteTestEnv(t)
	s := NewServer(t.TempDir(), "test")
	_, key := mintRootKey(t, s, 1)

	callToolWithKey(t, s, 2, key, "note.write", map[string]any{
		"layer": "machine",
		"notes": []any{map[string]any{"key": "restamp.me", "value": "v1", "priority": 1}},
	})
	first := searchSingleNoteRecord(t, s, 3, key, "machine", "restamp.me")

	callToolWithKey(t, s, 4, key, "note.write", map[string]any{
		"layer": "machine",
		"notes": []any{map[string]any{"key": "restamp.me", "value": "v2", "priority": 1}},
	})
	second := searchSingleNoteRecord(t, s, 5, key, "machine", "restamp.me")

	if second.WrittenAt < first.WrittenAt {
		t.Fatalf("note.write did not re-stamp written_at on overwrite: first=%q second=%q (want second >= first)", first.WrittenAt, second.WrittenAt)
	}
}

// searchSingleNoteRecord runs note.search with format:"json" and returns the
// single matched wsnote.Record, failing the test if the match count is not
// exactly 1.
func searchSingleNoteRecord(t *testing.T, s *Server, id int, key, layer, glob string) wsnote.Record {
	t.Helper()
	resp := callToolWithKey(t, s, id, key, "note.search", map[string]any{
		"layer":  layer,
		"glob":   glob,
		"format": "json",
	})
	var records []wsnote.Record
	if err := json.Unmarshal([]byte(resp), &records); err != nil {
		t.Fatalf("unmarshal note.search json response: %v\nresp=%s", err, resp)
	}
	if len(records) != 1 {
		t.Fatalf("note.search(%s) = %d records, want exactly 1: %s", glob, len(records), resp)
	}
	return records[0]
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

// TestNoteWriteMachineLayerRejectsEmptySessionKey covers the OTHER machine-
// layer rejection path in resolveNoteStorePath: an empty session_key must
// fail with "session_key is required" before an unknown_session lookup is
// ever attempted. TestNoteWriteMachineLayerRequiresKnownSessionKey only
// covers the known-but-unresolvable-key path.
func TestNoteWriteMachineLayerRejectsEmptySessionKey(t *testing.T) {
	setupNoteTestEnv(t)
	s := NewServer(t.TempDir(), "test")

	resp := callToolWithKey(t, s, 1, "", "note.write", map[string]any{
		"layer": "machine",
		"notes": []any{map[string]any{"key": "a", "value": "b"}},
	})
	if !strings.Contains(resp, "session_key is required") {
		t.Fatalf("note.write(machine, empty session_key) = %s, want a session_key-required error", resp)
	}
}
