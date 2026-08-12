package mcp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

// TestNoteRepoLayerRoundTripAndGitTracking verifies the ticket's headline
// verification boundary for the repo layer, distinct from the non-tracked
// layers covered above: note.write(layer: "repo") lands one real file on
// disk under <root>/ai-docs/ws-notes/, `git status --porcelain` reports it
// as an untracked/added path (genuinely tracked-location, unlike
// machine/worktree which live outside the working tree entirely),
// note.search finds it, and note.erase removes the file from disk.
func TestNoteRepoLayerRoundTripAndGitTracking(t *testing.T) {
	setupNoteTestEnv(t)
	s := NewServer(t.TempDir(), "test")
	root, key := mintRootKey(t, s, 1)

	writeResp := callToolWithKey(t, s, 2, key, "note.write", map[string]any{
		"layer": "repo",
		"notes": []any{
			map[string]any{"key": "repo.roundtrip", "value": "hello repo", "priority": 4},
		},
	})
	if !strings.HasPrefix(writeResp, "wrote 1 note") || !strings.Contains(writeResp, "repo.roundtrip") {
		t.Fatalf("note.write(repo) unexpected response: %s", writeResp)
	}

	notesDir := filepath.Join(root, "ai-docs", "ws-notes")
	entries, err := os.ReadDir(notesDir)
	if err != nil {
		t.Fatalf("ReadDir(%s): %v", notesDir, err)
	}
	var jsonFiles int
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".json") {
			jsonFiles++
		}
	}
	if jsonFiles != 1 {
		t.Fatalf("found %d .json files under %s, want exactly 1", jsonFiles, notesDir)
	}

	// -uall forces git to list the individual untracked file rather than
	// collapsing the wholly-untracked ai-docs/ directory into one line.
	status := string(runGitOutput(t, root, "status", "--porcelain", "-uall"))
	if !strings.Contains(status, "ai-docs/ws-notes/") {
		t.Fatalf("git status --porcelain = %q, want it to report the new file under ai-docs/ws-notes/", status)
	}

	searchResp := callToolWithKey(t, s, 3, key, "note.search", map[string]any{
		"layer": "repo",
		"glob":  "repo.roundtrip",
	})
	if !strings.Contains(searchResp, "repo.roundtrip") || !strings.Contains(searchResp, "hello repo") {
		t.Fatalf("note.search(repo) did not return the written record: %s", searchResp)
	}

	eraseResp := callToolWithKey(t, s, 4, key, "note.erase", map[string]any{
		"layer": "repo",
		"keys":  []any{"repo.roundtrip"},
	})
	if !strings.Contains(eraseResp, "repo.roundtrip") {
		t.Fatalf("note.erase(repo) confirmation missing key: %s", eraseResp)
	}

	remaining, err := os.ReadDir(notesDir)
	if err != nil {
		t.Fatalf("ReadDir(%s) after erase: %v", notesDir, err)
	}
	for _, entry := range remaining {
		if strings.HasSuffix(entry.Name(), ".json") {
			t.Fatalf("found leftover .json file %q under %s after note.erase", entry.Name(), notesDir)
		}
	}

	searchAfterErase := callToolWithKey(t, s, 5, key, "note.search", map[string]any{
		"layer": "repo",
		"glob":  "repo.roundtrip",
	})
	if strings.Contains(searchAfterErase, "repo.roundtrip") {
		t.Fatalf("note.search(repo) after erase still returned the record: %s", searchAfterErase)
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
// RECOMPUTES on a full-overwrite write, rather than echoing the prior
// record's written_at. The storage-layer wsnote tests exercise Write/Load
// directly with caller-chosen WrittenAt values and do not cover this
// handler-level behavior.
//
// This overrides the package-level noteNow clock to two distinct, controlled
// instants instead of relying on real wall-clock time: noteRecordsArg stamps
// at RFC3339 (second) granularity, so two sequential in-process writes almost
// always land in the same real second — first == second as strings — which
// would make a stale-echo regression (handler echoes the OLD written_at
// instead of recomputing) indistinguishable from a correct fresh same-second
// re-stamp. Injecting two instants a day apart makes the assertion exact and
// deterministic with no sleep and no flakiness.
func TestNoteWriteRestampsWrittenAtOnOverwrite(t *testing.T) {
	setupNoteTestEnv(t)
	s := NewServer(t.TempDir(), "test")
	_, key := mintRootKey(t, s, 1)

	firstInstant := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	secondInstant := time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC)
	originalNoteNow := noteNow
	t.Cleanup(func() { noteNow = originalNoteNow })

	noteNow = func() time.Time { return firstInstant }
	callToolWithKey(t, s, 2, key, "note.write", map[string]any{
		"layer": "machine",
		"notes": []any{map[string]any{"key": "restamp.me", "value": "v1", "priority": 1}},
	})
	first := searchSingleNoteRecord(t, s, 3, key, "machine", "restamp.me")
	if first.WrittenAt != firstInstant.Format(time.RFC3339) {
		t.Fatalf("first written_at = %q, want %q (the injected first instant)", first.WrittenAt, firstInstant.Format(time.RFC3339))
	}

	noteNow = func() time.Time { return secondInstant }
	callToolWithKey(t, s, 4, key, "note.write", map[string]any{
		"layer": "machine",
		"notes": []any{map[string]any{"key": "restamp.me", "value": "v2", "priority": 1}},
	})
	second := searchSingleNoteRecord(t, s, 5, key, "machine", "restamp.me")

	if second.WrittenAt != secondInstant.Format(time.RFC3339) {
		t.Fatalf("note.write did not re-stamp written_at on overwrite: got %q, want the injected second instant %q (a stale echo of the first write's %q would fail this exact check)",
			second.WrittenAt, secondInstant.Format(time.RFC3339), first.WrittenAt)
	}
	if second.WrittenAt == first.WrittenAt {
		t.Fatalf("first and second written_at unexpectedly equal (%q); the two injected clock instants must differ for this test to be load-bearing", first.WrittenAt)
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
		"layer": "bogus",
		"notes": []any{map[string]any{"key": "a", "value": "b"}},
	})
	if !strings.Contains(resp, `must be "machine", "worktree", or "repo"`) {
		t.Fatalf("note.write(layer=bogus) = %s, want a layer-validation error", resp)
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
