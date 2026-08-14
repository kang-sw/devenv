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

	for _, layer := range []string{"machine", "worktree", "clone"} {
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
// note.search finds it, and note.erase removes the file from disk. It also
// proves the reviewed lock-artifact fix: the tracked dir holds exactly the
// intended .json file (no ".lock"/"*.tmp" sidecar) while the note exists, and
// nothing at all — not even an orphaned lock file — once it is erased.
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
	// Exactly one entry, and it must be the .json file itself — no ".lock" or
	// "*.tmp" sidecar leaked into the tracked directory.
	if len(entries) != 1 {
		t.Fatalf("tracked dir %s has %d entries after note.write, want exactly 1 (.json only): %v", notesDir, len(entries), entries)
	}
	if !strings.HasSuffix(entries[0].Name(), ".json") {
		t.Fatalf("tracked dir entry %q is not a .json file — lock/temp artifact leaked into %s", entries[0].Name(), notesDir)
	}

	// -uall forces git to list the individual untracked file rather than
	// collapsing the wholly-untracked ai-docs/ directory into one line.
	status := string(runGitOutput(t, root, "status", "--porcelain", "-uall"))
	if !strings.Contains(status, "ai-docs/ws-notes/") {
		t.Fatalf("git status --porcelain = %q, want it to report the new file under ai-docs/ws-notes/", status)
	}
	if strings.Contains(status, ".lock") {
		t.Fatalf("git status --porcelain = %q, want no .lock artifact reported under the tracked dir", status)
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

	// After erase the tracked dir must be completely empty — not just free of
	// .json files, but free of any leftover .lock/*.tmp residue too.
	remaining, err := os.ReadDir(notesDir)
	if err != nil {
		t.Fatalf("ReadDir(%s) after erase: %v", notesDir, err)
	}
	if len(remaining) != 0 {
		t.Fatalf("tracked dir %s has %d leftover entries after note.erase, want 0: %v", notesDir, len(remaining), remaining)
	}

	statusAfterErase := string(runGitOutput(t, root, "status", "--porcelain", "-uall"))
	if strings.Contains(statusAfterErase, "ai-docs/ws-notes/") {
		t.Fatalf("git status --porcelain after erase = %q, want no residue reported under ai-docs/ws-notes/", statusAfterErase)
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

// TestNoteCloneLayerIsProjectScopedAndWorktreeAgnostic verifies the clone
// layer's headline contract from 260814 Phase 1: a clone-layer note written
// under one worktree of a project is visible from a SIBLING worktree of the
// SAME project (worktree-agnostic, unlike the worktree layer, which
// TestNoteWorktreeLayerIsolatedAcrossWorktrees proves is isolated at exactly
// this granularity), absent from a DIFFERENT project's key (project-scoped,
// unlike the machine layer), and never staged by git in the worktree working
// tree (stored outside it entirely, like machine/worktree — not the repo
// layer's tracked ai-docs/ws-notes/).
func TestNoteCloneLayerIsProjectScopedAndWorktreeAgnostic(t *testing.T) {
	setupNoteTestEnv(t)
	s := NewServer(t.TempDir(), "test")

	mainRoot, linkedRoot := twoWorktreesOfOneRepo(t)
	keyMain, _ := parseLoginResponse(t, callLogin(t, s, 1, mainRoot, nil))
	keyLinked, _ := parseLoginResponse(t, callLogin(t, s, 2, linkedRoot, nil))
	_, keyOther := mintRootKey(t, s, 3)

	callToolWithKey(t, s, 4, keyMain, "note.write", map[string]any{
		"layer": "clone",
		"notes": []any{map[string]any{"key": "clone.shared", "value": "visible across worktrees", "priority": 1}},
	})

	// Visible from the sibling worktree of the same project (worktree-agnostic).
	searchLinked := callToolWithKey(t, s, 5, keyLinked, "note.search", map[string]any{"layer": "clone", "glob": "clone.shared"})
	if !strings.Contains(searchLinked, "clone.shared") {
		t.Fatalf("note.search(clone, linked worktree of the same project) missing the note written from the main worktree: %s", searchLinked)
	}

	// Absent from a different, unrelated project on the same machine
	// (project-scoped).
	searchOther := callToolWithKey(t, s, 6, keyOther, "note.search", map[string]any{"layer": "clone", "glob": "clone.shared"})
	if strings.Contains(searchOther, "clone.shared") {
		t.Fatalf("note.search(clone, unrelated project) leaked a note from a different project: %s", searchOther)
	}

	// Never staged by git: the clone store lives outside the working tree,
	// same as machine/worktree.
	status := string(runGitOutput(t, mainRoot, "status", "--porcelain", "-uall"))
	if strings.TrimSpace(status) != "" {
		t.Fatalf("git status --porcelain in the worktree = %q, want clean (the clone store must live outside the working tree)", status)
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
	if !strings.Contains(resp, `must be "machine", "worktree", "clone", or "repo"`) {
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

// TestNoteMuteUnmuteRoundTrip verifies note.mute/note.unmute across all
// four layers: mute drops a note's visible state to false, unmute restores
// it to true, observed via note.search's json format (note.search itself
// never filters on visible, so this exercises the stored field directly).
func TestNoteMuteUnmuteRoundTrip(t *testing.T) {
	setupNoteTestEnv(t)
	s := NewServer(t.TempDir(), "test")
	_, key := mintRootKey(t, s, 1)

	id := 2
	for _, layer := range []string{"machine", "worktree", "clone", "repo"} {
		t.Run(layer, func(t *testing.T) {
			noteKey := "mute.roundtrip." + layer
			callToolWithKey(t, s, id, key, "note.write", map[string]any{
				"layer": layer,
				"notes": []any{map[string]any{"key": noteKey, "value": "v", "priority": 1}},
			})
			id++

			rec := searchSingleNoteRecord(t, s, id, key, layer, noteKey)
			id++
			if !rec.Visible {
				t.Fatalf("note.write(%s) new key Visible = false, want true", layer)
			}

			muteResp := callToolWithKey(t, s, id, key, "note.mute", map[string]any{
				"layer": layer,
				"keys":  []any{noteKey},
			})
			id++
			if !strings.Contains(muteResp, noteKey) {
				t.Fatalf("note.mute(%s) confirmation missing key: %s", layer, muteResp)
			}

			rec = searchSingleNoteRecord(t, s, id, key, layer, noteKey)
			id++
			if rec.Visible {
				t.Fatalf("note.mute(%s) did not set Visible = false", layer)
			}

			unmuteResp := callToolWithKey(t, s, id, key, "note.unmute", map[string]any{
				"layer": layer,
				"keys":  []any{noteKey},
			})
			id++
			if !strings.Contains(unmuteResp, noteKey) {
				t.Fatalf("note.unmute(%s) confirmation missing key: %s", layer, unmuteResp)
			}

			rec = searchSingleNoteRecord(t, s, id, key, layer, noteKey)
			id++
			if !rec.Visible {
				t.Fatalf("note.unmute(%s) did not restore Visible = true", layer)
			}
		})
	}
}

// TestNoteMuteUnmuteDoesNotRestampWrittenAt verifies mute/unmute never touch
// written_at, even when the injected clock visibly moves between the write
// and the mute/unmute call — matching
// TestNoteWriteRestampsWrittenAtOnOverwrite's clock-injection pattern.
func TestNoteMuteUnmuteDoesNotRestampWrittenAt(t *testing.T) {
	setupNoteTestEnv(t)
	s := NewServer(t.TempDir(), "test")
	_, key := mintRootKey(t, s, 1)

	writeInstant := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	laterInstant := time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC)
	originalNoteNow := noteNow
	t.Cleanup(func() { noteNow = originalNoteNow })

	noteNow = func() time.Time { return writeInstant }
	callToolWithKey(t, s, 2, key, "note.write", map[string]any{
		"layer": "machine",
		"notes": []any{map[string]any{"key": "no.restamp", "value": "v", "priority": 1}},
	})
	wantWrittenAt := writeInstant.Format(time.RFC3339)

	noteNow = func() time.Time { return laterInstant }
	callToolWithKey(t, s, 3, key, "note.mute", map[string]any{
		"layer": "machine",
		"keys":  []any{"no.restamp"},
	})
	muted := searchSingleNoteRecord(t, s, 4, key, "machine", "no.restamp")
	if muted.WrittenAt != wantWrittenAt {
		t.Fatalf("note.mute restamped written_at: got %q, want unchanged %q", muted.WrittenAt, wantWrittenAt)
	}
	if muted.Visible {
		t.Fatalf("note.mute did not set Visible = false")
	}

	callToolWithKey(t, s, 5, key, "note.unmute", map[string]any{
		"layer": "machine",
		"keys":  []any{"no.restamp"},
	})
	unmuted := searchSingleNoteRecord(t, s, 6, key, "machine", "no.restamp")
	if unmuted.WrittenAt != wantWrittenAt {
		t.Fatalf("note.unmute restamped written_at: got %q, want unchanged %q", unmuted.WrittenAt, wantWrittenAt)
	}
}

// TestNoteWriteOverMutedKeyPreservesMute is the MCP-level version of the
// write-over-a-muted-key regression: note.write/note.mute/note.write again
// must leave the key muted even though the second write changes content.
func TestNoteWriteOverMutedKeyPreservesMute(t *testing.T) {
	setupNoteTestEnv(t)
	s := NewServer(t.TempDir(), "test")
	_, key := mintRootKey(t, s, 1)

	callToolWithKey(t, s, 2, key, "note.write", map[string]any{
		"layer": "machine",
		"notes": []any{map[string]any{"key": "write.over.mute", "value": "v1", "priority": 1}},
	})
	callToolWithKey(t, s, 3, key, "note.mute", map[string]any{
		"layer": "machine",
		"keys":  []any{"write.over.mute"},
	})
	callToolWithKey(t, s, 4, key, "note.write", map[string]any{
		"layer": "machine",
		"notes": []any{map[string]any{"key": "write.over.mute", "value": "v2", "priority": 9}},
	})

	rec := searchSingleNoteRecord(t, s, 5, key, "machine", "write.over.mute")
	if rec.Visible {
		t.Fatalf("note.write over a muted key set Visible = true, want the mute to survive (false)")
	}
	if rec.Value != "v2" || rec.Priority != 9 {
		t.Fatalf("note.write over a muted key did not update content fields: %+v", rec)
	}
}

// TestNoteMuteAlreadyMutedKeyIsNoop verifies muting an already-muted key
// succeeds as a no-op (idempotent set-state), not an error.
func TestNoteMuteAlreadyMutedKeyIsNoop(t *testing.T) {
	setupNoteTestEnv(t)
	s := NewServer(t.TempDir(), "test")
	_, key := mintRootKey(t, s, 1)

	callToolWithKey(t, s, 2, key, "note.write", map[string]any{
		"layer": "machine",
		"notes": []any{map[string]any{"key": "double.mute", "value": "v", "priority": 1}},
	})
	callToolWithKey(t, s, 3, key, "note.mute", map[string]any{
		"layer": "machine",
		"keys":  []any{"double.mute"},
	})
	secondMuteResp := callToolWithKey(t, s, 4, key, "note.mute", map[string]any{
		"layer": "machine",
		"keys":  []any{"double.mute"},
	})
	if !strings.Contains(secondMuteResp, "double.mute") {
		t.Fatalf("note.mute on an already-muted key did not succeed: %s", secondMuteResp)
	}

	rec := searchSingleNoteRecord(t, s, 5, key, "machine", "double.mute")
	if rec.Visible {
		t.Fatalf("note.mute called twice left Visible = true, want false")
	}
}

// TestNoteMuteRejectsEmptyKeys mirrors note.erase's empty-keys rejection,
// confirming note.mute/note.unmute share the same required-non-empty
// validation via noteKeysArg.
func TestNoteMuteRejectsEmptyKeys(t *testing.T) {
	setupNoteTestEnv(t)
	s := NewServer(t.TempDir(), "test")
	_, key := mintRootKey(t, s, 1)

	resp := callToolWithKey(t, s, 2, key, "note.mute", map[string]any{
		"layer": "machine",
		"keys":  []any{},
	})
	if !strings.Contains(resp, "non-empty") {
		t.Fatalf("note.mute(empty keys) = %s, want a non-empty-array error", resp)
	}
}

// taggedNoteRecordJSON mirrors taggedNoteRecord's wire shape for test-side
// decoding. It does NOT embed wsnote.Record: Record's custom UnmarshalJSON
// gets promoted to any type that embeds it, which hijacks decoding of the
// OUTER type too (Go's embedding-plus-custom-Unmarshaler gotcha) and would
// silently leave Layer unset on every decode — this struct lists the fields
// explicitly instead, purely for test assertions. Production code never
// unmarshals taggedNoteRecord (only marshals it via toolJSONResponse), so
// this gotcha is test-only and does not affect the real response wire shape.
type taggedNoteRecordJSON struct {
	Key       string       `json:"key"`
	Value     string       `json:"value"`
	Priority  int          `json:"priority"`
	WrittenAt string       `json:"written_at"`
	Visible   bool         `json:"visible"`
	Layer     wsnote.Layer `json:"layer"`
}

// TestNoteSearchOmittedLayerSearchesAllFourTagged verifies note.search's
// headline multi-layer contract: omitting "layer" entirely searches all four
// layers and tags each returned record with its originating layer (sub-
// decision a), in priority-desc order (sub-decision b) — one note per layer,
// each at a distinct priority so the expected order is unambiguous.
func TestNoteSearchOmittedLayerSearchesAllFourTagged(t *testing.T) {
	setupNoteTestEnv(t)
	s := NewServer(t.TempDir(), "test")
	_, key := mintRootKey(t, s, 1)

	id := 2
	writeLayerNote := func(layer string, priority int) {
		callToolWithKey(t, s, id, key, "note.write", map[string]any{
			"layer": layer,
			"notes": []any{map[string]any{"key": "omit.all." + layer, "value": "v-" + layer, "priority": priority}},
		})
		id++
	}
	writeLayerNote("machine", 4)
	writeLayerNote("worktree", 3)
	writeLayerNote("clone", 2)
	writeLayerNote("repo", 1)

	resp := callToolWithKey(t, s, id, key, "note.search", map[string]any{
		"glob":   "omit.all.*",
		"format": "json",
	})
	var records []taggedNoteRecordJSON
	if err := json.Unmarshal([]byte(resp), &records); err != nil {
		t.Fatalf("unmarshal note.search json response: %v\nresp=%s", err, resp)
	}
	wantOrder := []struct {
		key   string
		layer wsnote.Layer
	}{
		{"omit.all.machine", wsnote.LayerMachine},
		{"omit.all.worktree", wsnote.LayerWorktree},
		{"omit.all.clone", wsnote.LayerClone},
		{"omit.all.repo", wsnote.LayerRepo},
	}
	if len(records) != len(wantOrder) {
		t.Fatalf("note.search(no layer) = %d records, want %d: %s", len(records), len(wantOrder), resp)
	}
	for i, want := range wantOrder {
		if records[i].Key != want.key || records[i].Layer != want.layer {
			t.Fatalf("note.search(no layer)[%d] = {key:%q layer:%q}, want {key:%q layer:%q}; full: %s",
				i, records[i].Key, records[i].Layer, want.key, want.layer, resp)
		}
	}
}

// TestNoteSearchArrayLayerScopesToThoseLayers verifies an array "layer"
// argument searches exactly the listed layers and no others.
func TestNoteSearchArrayLayerScopesToThoseLayers(t *testing.T) {
	setupNoteTestEnv(t)
	s := NewServer(t.TempDir(), "test")
	_, key := mintRootKey(t, s, 1)

	id := 2
	for _, layer := range []string{"machine", "worktree", "clone", "repo"} {
		callToolWithKey(t, s, id, key, "note.write", map[string]any{
			"layer": layer,
			"notes": []any{map[string]any{"key": "array.scope." + layer, "value": "v", "priority": 1}},
		})
		id++
	}

	resp := callToolWithKey(t, s, id, key, "note.search", map[string]any{
		"layer":  []any{"clone", "repo"},
		"glob":   "array.scope.*",
		"format": "json",
	})
	var records []taggedNoteRecordJSON
	if err := json.Unmarshal([]byte(resp), &records); err != nil {
		t.Fatalf("unmarshal note.search json response: %v\nresp=%s", err, resp)
	}
	if len(records) != 2 {
		t.Fatalf("note.search(layer:[clone,repo]) = %d records, want 2: %s", len(records), resp)
	}
	gotLayers := map[wsnote.Layer]bool{}
	for _, rec := range records {
		gotLayers[rec.Layer] = true
	}
	if !gotLayers[wsnote.LayerClone] || !gotLayers[wsnote.LayerRepo] {
		t.Fatalf("note.search(layer:[clone,repo]) layers = %v, want exactly {clone, repo}: %s", gotLayers, resp)
	}
	if gotLayers[wsnote.LayerMachine] || gotLayers[wsnote.LayerWorktree] {
		t.Fatalf("note.search(layer:[clone,repo]) leaked an unscoped layer: %v: %s", gotLayers, resp)
	}
}

// TestNoteSearchSingleStringLayerStaysUntagged is the sub-decision (a)
// regression: a single-string "layer" call must keep returning a plain
// []wsnote.Record with no "layer" key anywhere in the JSON — the exact
// pre-existing wire shape searchSingleNoteRecord already decodes against —
// even though the underlying comparator now matches Compute's 3-key order.
func TestNoteSearchSingleStringLayerStaysUntagged(t *testing.T) {
	setupNoteTestEnv(t)
	s := NewServer(t.TempDir(), "test")
	_, key := mintRootKey(t, s, 1)

	callToolWithKey(t, s, 2, key, "note.write", map[string]any{
		"layer": "clone",
		"notes": []any{map[string]any{"key": "single.untagged", "value": "v", "priority": 1}},
	})

	resp := callToolWithKey(t, s, 3, key, "note.search", map[string]any{
		"layer":  "clone",
		"glob":   "single.untagged",
		"format": "json",
	})
	if strings.Contains(resp, `"layer"`) {
		t.Fatalf("note.search(layer:\"clone\") response contains a \"layer\" key, want the untagged plain-Record shape: %s", resp)
	}
	var records []wsnote.Record
	if err := json.Unmarshal([]byte(resp), &records); err != nil {
		t.Fatalf("unmarshal note.search json response as plain []wsnote.Record: %v\nresp=%s", err, resp)
	}
	if len(records) != 1 || records[0].Key != "single.untagged" {
		t.Fatalf("note.search(layer:\"clone\") = %#v, want exactly the written record", records)
	}
}

// TestNoteSearchMultiLayerIncludesMutedNotes verifies note.search's
// no-visible-filtering contract (unchanged by this phase) extends to the
// multi-layer/array path: a muted note still surfaces via an array "layer"
// search, mirroring the single-layer mute-search precedent
// (TestNoteMuteUnmuteRoundTrip) at the multi-layer granularity.
func TestNoteSearchMultiLayerIncludesMutedNotes(t *testing.T) {
	setupNoteTestEnv(t)
	s := NewServer(t.TempDir(), "test")
	_, key := mintRootKey(t, s, 1)

	callToolWithKey(t, s, 2, key, "note.write", map[string]any{
		"layer": "clone",
		"notes": []any{map[string]any{"key": "muted.multilayer", "value": "v", "priority": 1}},
	})
	callToolWithKey(t, s, 3, key, "note.mute", map[string]any{
		"layer": "clone",
		"keys":  []any{"muted.multilayer"},
	})

	resp := callToolWithKey(t, s, 4, key, "note.search", map[string]any{
		"layer":  []any{"clone", "repo"},
		"glob":   "muted.multilayer",
		"format": "json",
	})
	var records []taggedNoteRecordJSON
	if err := json.Unmarshal([]byte(resp), &records); err != nil {
		t.Fatalf("unmarshal note.search json response: %v\nresp=%s", err, resp)
	}
	if len(records) != 1 || records[0].Key != "muted.multilayer" {
		t.Fatalf("note.search(layer:[clone,repo]) did not return the muted note: %s", resp)
	}
	if records[0].Visible {
		t.Fatalf("note.search(layer:[clone,repo]) returned the note with Visible=true, want the stored muted (false) state preserved: %s", resp)
	}
}

// TestNoteSearchArrayVsSingleLayerOrderParity verifies sub-decision (b):
// layer:"clone" and layer:["clone"] against identical fixture data return
// byte-identical record order, differing only in tag presence (sub-decision
// a) — proving both paths route through the same comparator rather than
// two independently-maintained sort implementations that could drift.
func TestNoteSearchArrayVsSingleLayerOrderParity(t *testing.T) {
	setupNoteTestEnv(t)
	s := NewServer(t.TempDir(), "test")
	_, key := mintRootKey(t, s, 1)

	notes := []any{
		map[string]any{"key": "parity.b", "value": "v", "priority": 1},
		map[string]any{"key": "parity.a", "value": "v", "priority": 1},
		map[string]any{"key": "parity.high", "value": "v", "priority": 9},
	}
	callToolWithKey(t, s, 2, key, "note.write", map[string]any{
		"layer": "clone",
		"notes": notes,
	})

	singleResp := callToolWithKey(t, s, 3, key, "note.search", map[string]any{
		"layer":  "clone",
		"glob":   "parity.*",
		"format": "json",
	})
	var singleRecords []wsnote.Record
	if err := json.Unmarshal([]byte(singleResp), &singleRecords); err != nil {
		t.Fatalf("unmarshal single-layer response: %v\nresp=%s", err, singleResp)
	}

	arrayResp := callToolWithKey(t, s, 4, key, "note.search", map[string]any{
		"layer":  []any{"clone"},
		"glob":   "parity.*",
		"format": "json",
	})
	var arrayRecords []taggedNoteRecordJSON
	if err := json.Unmarshal([]byte(arrayResp), &arrayRecords); err != nil {
		t.Fatalf("unmarshal array-layer response: %v\nresp=%s", err, arrayResp)
	}

	if len(singleRecords) != len(arrayRecords) {
		t.Fatalf("single-layer returned %d records, array-layer returned %d, want equal counts", len(singleRecords), len(arrayRecords))
	}
	for i := range singleRecords {
		if singleRecords[i].Key != arrayRecords[i].Key {
			t.Fatalf("order mismatch at [%d]: single=%q array=%q, want identical order (single: %s, array: %s)",
				i, singleRecords[i].Key, arrayRecords[i].Key, singleResp, arrayResp)
		}
		if arrayRecords[i].Layer != wsnote.LayerClone {
			t.Fatalf("array-layer record[%d] tagged %q, want %q: %s", i, arrayRecords[i].Layer, wsnote.LayerClone, arrayResp)
		}
	}
}

// TestNoteSearchRejectsEmptyLayerArray verifies an empty "layer" array is a
// caller error, not silently treated as "search nothing" or "search all".
func TestNoteSearchRejectsEmptyLayerArray(t *testing.T) {
	setupNoteTestEnv(t)
	s := NewServer(t.TempDir(), "test")
	_, key := mintRootKey(t, s, 1)

	resp := callToolWithKey(t, s, 2, key, "note.search", map[string]any{
		"layer": []any{},
	})
	if !strings.Contains(resp, "non-empty") {
		t.Fatalf("note.search(layer:[]) = %s, want a non-empty-array error", resp)
	}
}

// TestNoteUnmuteRejectsEmptyKeys is the symmetric counterpart of
// TestNoteMuteRejectsEmptyKeys: note.unmute shares handleNoteSetVisible's
// validation with note.mute, but that shared code path had no dedicated
// note.unmute-side assertion until now.
func TestNoteUnmuteRejectsEmptyKeys(t *testing.T) {
	setupNoteTestEnv(t)
	s := NewServer(t.TempDir(), "test")
	_, key := mintRootKey(t, s, 1)

	resp := callToolWithKey(t, s, 2, key, "note.unmute", map[string]any{
		"layer": "machine",
		"keys":  []any{},
	})
	if !strings.Contains(resp, "non-empty") {
		t.Fatalf("note.unmute(empty keys) = %s, want a non-empty-array error", resp)
	}
}
