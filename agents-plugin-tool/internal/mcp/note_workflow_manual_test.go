package mcp

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// setupWorkflowManualNoteEnv isolates WS_CACHE_HOME/WS_CONFIG_HOME/WS_RSRC_ROOT
// per test, mirroring manuals_workflow_manual_test.go's fixture setup for
// exercising the full workflow_manual playbook render path.
func setupWorkflowManualNoteEnv(t *testing.T) {
	t.Helper()
	useLeadProfile(t)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))
	t.Setenv("WS_RSRC_ROOT", filepath.Join("..", "..", "..", "agents-plugin", "rsrc"))
}

// TestWorkflowManualCarriesNotesBlockOnFreshAndContinuePositionedAfterSessionState
// verifies the 260807 Phase 1 "# Notes" block is injected into workflow_manual
// output on both the FRESH-with-root branch and the CONTINUE branch, and that
// it is appended immediately after "## Session State" — not prepended as a
// top-of-body banner like the sibling scope/manuals/staleness injections.
func TestWorkflowManualCarriesNotesBlockOnFreshAndContinuePositionedAfterSessionState(t *testing.T) {
	setupWorkflowManualNoteEnv(t)
	root := t.TempDir()
	initGit(t, root)

	s := NewServer(root, "test")

	// Mint a key first (via the FRESH-with-root call itself) so we can write a
	// note, then re-fetch workflow_manual under that same key for the CONTINUE
	// assertions. But FRESH-with-root's own response must also carry the block
	// once a note exists — write the note first via a bootstrap key from a
	// throwaway ferrule call bound to the same root.
	bootstrapKey, _ := parseLoginResponse(t, callLogin(t, s, 1, root, nil))
	callToolWithKey(t, s, 2, bootstrapKey, "note.write", map[string]any{
		"layer": "worktree",
		"notes": []any{map[string]any{"key": "fresh.note", "value": "seen on fresh and continue", "priority": 1}},
	})

	freshResp := callToolWithKey(t, s, 3, freshBootstrapKey, "workflow_manual", map[string]any{
		"root": root,
	})
	assertNotesAfterSessionState(t, freshResp, "fresh.note", "seen on fresh and continue")

	continueResp := callToolWithKey(t, s, 4, bootstrapKey, "workflow_manual", nil)
	assertNotesAfterSessionState(t, continueResp, "fresh.note", "seen on fresh and continue")
}

func assertNotesAfterSessionState(t *testing.T, body, wantKey, wantValue string) {
	t.Helper()
	sessionIdx := strings.Index(body, "## Session State")
	if sessionIdx < 0 {
		t.Fatalf("workflow_manual output missing '## Session State': %s", body)
	}
	notesIdx := strings.Index(body, "# Notes")
	if notesIdx < 0 {
		t.Fatalf("workflow_manual output missing '# Notes' block: %s", body)
	}
	if notesIdx < sessionIdx {
		t.Fatalf("'# Notes' block (at %d) must come after '## Session State' (at %d), not before: %s", notesIdx, sessionIdx, body)
	}
	if !strings.Contains(body, wantKey) || !strings.Contains(body, wantValue) {
		t.Fatalf("workflow_manual output missing note %q=%q: %s", wantKey, wantValue, body)
	}
	// Not a top-of-body banner: the block must not appear before "## Session
	// Key", which itself is only injected after the manual body — unlike the
	// sibling scope/manuals/staleness injections, which prepend ahead of
	// everything including "## Session Key".
	sessionKeyIdx := strings.Index(body, "## Session Key")
	if sessionKeyIdx < 0 {
		t.Fatalf("workflow_manual output missing '## Session Key': %s", body)
	}
	if notesIdx < sessionKeyIdx {
		t.Fatalf("'# Notes' block (at %d) must come after '## Session Key' (at %d), not be prepended ahead of it: %s", notesIdx, sessionKeyIdx, body)
	}
}

// TestWorkflowManualCarriesRepoLayerNoteAndErasesCleanly verifies the
// ticket's end-to-end verification boundary for the repo layer:
// note.write(layer: "repo") lands a tracked file under
// <root>/ai-docs/ws-notes/, the record appears in the very next
// workflow_manual "# Notes" block tagged "[repo]", and note.erase removes
// the file so a subsequent workflow_manual no longer carries it.
func TestWorkflowManualCarriesRepoLayerNoteAndErasesCleanly(t *testing.T) {
	setupWorkflowManualNoteEnv(t)
	root := t.TempDir()
	initGit(t, root)

	s := NewServer(root, "test")
	key, _ := parseLoginResponse(t, callLogin(t, s, 1, root, nil))

	callToolWithKey(t, s, 2, key, "note.write", map[string]any{
		"layer": "repo",
		"notes": []any{map[string]any{"key": "repo.manual", "value": "tracked note", "priority": 1}},
	})

	notesDir := filepath.Join(root, "ai-docs", "ws-notes")
	entries, err := os.ReadDir(notesDir)
	if err != nil {
		t.Fatalf("ReadDir(%s): %v", notesDir, err)
	}
	var jsonFile string
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".json") {
			jsonFile = entry.Name()
		}
	}
	if jsonFile == "" {
		t.Fatalf("note.write(repo) did not create a .json file under %s", notesDir)
	}
	for _, r := range strings.TrimSuffix(jsonFile, ".json") {
		if !strings.ContainsRune("0123456789abcdef", r) {
			t.Fatalf("repo note filename %q is not a plain hex string", jsonFile)
		}
	}

	afterWrite := callToolWithKey(t, s, 3, key, "workflow_manual", nil)
	if !strings.Contains(afterWrite, "[repo] repo.manual") || !strings.Contains(afterWrite, "tracked note") {
		t.Fatalf("workflow_manual missing repo-layer note: %s", afterWrite)
	}

	callToolWithKey(t, s, 4, key, "note.erase", map[string]any{
		"layer": "repo",
		"keys":  []any{"repo.manual"},
	})
	if _, err := os.Stat(filepath.Join(notesDir, jsonFile)); !os.IsNotExist(err) {
		t.Fatalf("note.erase(repo) left the tracked file in place: stat err = %v", err)
	}

	afterErase := callToolWithKey(t, s, 5, key, "workflow_manual", nil)
	if strings.Contains(afterErase, "repo.manual") {
		t.Fatalf("workflow_manual still carries the erased repo-layer note: %s", afterErase)
	}
}

// TestWorkflowManualCarriesCloneLayerNote verifies the clone layer's
// end-to-end ambient-injection contract from 260814 Phase 1: a
// note.write(layer: "clone") note surfaces in the very next workflow_manual
// "# Notes" block tagged "[clone]", parallel to the machine/worktree/repo
// cases covered elsewhere in this file.
func TestWorkflowManualCarriesCloneLayerNote(t *testing.T) {
	setupWorkflowManualNoteEnv(t)
	root := t.TempDir()
	initGit(t, root)

	s := NewServer(root, "test")
	key, _ := parseLoginResponse(t, callLogin(t, s, 1, root, nil))

	callToolWithKey(t, s, 2, key, "note.write", map[string]any{
		"layer": "clone",
		"notes": []any{map[string]any{"key": "clone.manual", "value": "clone-scoped note", "priority": 1}},
	})

	resp := callToolWithKey(t, s, 3, key, "workflow_manual", nil)
	if !strings.Contains(resp, "[clone] clone.manual") || !strings.Contains(resp, "clone-scoped note") {
		t.Fatalf("workflow_manual missing clone-layer note: %s", resp)
	}
}

// TestWorkflowManualNotesBlockAbsentWhenNoNotesExist verifies the injection is
// a true no-op (scopeAnnouncement/computeManuals-style silent case) when no
// note has ever been written on any layer.
func TestWorkflowManualNotesBlockAbsentWhenNoNotesExist(t *testing.T) {
	setupWorkflowManualNoteEnv(t)
	root := t.TempDir()
	initGit(t, root)

	s := NewServer(root, "test")

	freshResp := callToolWithKey(t, s, 1, freshBootstrapKey, "workflow_manual", map[string]any{
		"root": root,
	})
	if strings.Contains(freshResp, "# Notes") {
		t.Fatalf("workflow_manual FRESH-with-root must stay silent with no notes: %s", freshResp)
	}

	key, _ := parseLoginResponse(t, callLogin(t, s, 2, root, nil))
	continueResp := callToolWithKey(t, s, 3, key, "workflow_manual", nil)
	if strings.Contains(continueResp, "# Notes") {
		t.Fatalf("workflow_manual CONTINUE must stay silent with no notes: %s", continueResp)
	}
}

// TestWorkflowManualNotesBlockElidesBeyondCapAndRemainsSearchable verifies
// more than notesInjectionCap notes cap the injected block at the
// highest-priority items, show the "N lower-priority notes elided" line, and
// that elided items are still retrievable via note.search.
func TestWorkflowManualNotesBlockElidesBeyondCapAndRemainsSearchable(t *testing.T) {
	setupWorkflowManualNoteEnv(t)
	root := t.TempDir()
	initGit(t, root)

	s := NewServer(root, "test")
	key, _ := parseLoginResponse(t, callLogin(t, s, 1, root, nil))

	total := notesInjectionCap + 3
	notes := make([]any, 0, total)
	for i := 0; i < total; i++ {
		notes = append(notes, map[string]any{
			"key":      fmt.Sprintf("elide.%02d", i),
			"value":    "v",
			"priority": i, // strictly increasing, so lowest-index keys are lowest priority
		})
	}
	callToolWithKey(t, s, 2, key, "note.write", map[string]any{
		"layer": "worktree",
		"notes": notes,
	})

	continueResp := callToolWithKey(t, s, 3, key, "workflow_manual", nil)
	if !strings.Contains(continueResp, "# Notes") {
		t.Fatalf("workflow_manual CONTINUE missing '# Notes' block: %s", continueResp)
	}
	wantElided := total - notesInjectionCap
	if !strings.Contains(continueResp, fmt.Sprintf("%d lower-priority notes elided", wantElided)) {
		t.Fatalf("workflow_manual CONTINUE missing elision line for %d elided notes: %s", wantElided, continueResp)
	}
	// The lowest-priority note (elide.00, priority 0) must be elided, not inlined.
	if strings.Contains(continueResp, "elide.00 ") {
		t.Fatalf("workflow_manual CONTINUE unexpectedly inlined the lowest-priority note: %s", continueResp)
	}
	// The highest-priority note must be inlined, not elided.
	highestKey := fmt.Sprintf("elide.%02d", total-1)
	if !strings.Contains(continueResp, highestKey) {
		t.Fatalf("workflow_manual CONTINUE dropped the highest-priority note %s: %s", highestKey, continueResp)
	}

	// The elided lowest-priority note must still be retrievable via note.search.
	searchResp := callToolWithKey(t, s, 4, key, "note.search", map[string]any{
		"layer": "worktree",
		"glob":  "elide.00",
	})
	if !strings.Contains(searchResp, "elide.00") {
		t.Fatalf("note.search did not retrieve the elided note elide.00: %s", searchResp)
	}
}

// TestWorkflowManualMutedNoteDropsFromBlockButStaysSearchable is the
// ticket's core end-to-end verification boundary: write a note, mute it, and
// confirm workflow_manual's "# Notes" block no longer carries the note's
// key/value while the muted-count line is present, but note.search still
// returns the record unchanged.
func TestWorkflowManualMutedNoteDropsFromBlockButStaysSearchable(t *testing.T) {
	setupWorkflowManualNoteEnv(t)
	root := t.TempDir()
	initGit(t, root)

	s := NewServer(root, "test")
	key, _ := parseLoginResponse(t, callLogin(t, s, 1, root, nil))

	callToolWithKey(t, s, 2, key, "note.write", map[string]any{
		"layer": "worktree",
		"notes": []any{map[string]any{"key": "mute.me", "value": "muted content", "priority": 1}},
	})

	beforeMute := callToolWithKey(t, s, 3, key, "workflow_manual", nil)
	if !strings.Contains(beforeMute, "mute.me") || !strings.Contains(beforeMute, "muted content") {
		t.Fatalf("workflow_manual before mute missing the note: %s", beforeMute)
	}

	callToolWithKey(t, s, 4, key, "note.mute", map[string]any{
		"layer": "worktree",
		"keys":  []any{"mute.me"},
	})

	afterMute := callToolWithKey(t, s, 5, key, "workflow_manual", nil)
	if strings.Contains(afterMute, "mute.me") || strings.Contains(afterMute, "muted content") {
		t.Fatalf("workflow_manual after mute still carries the muted note's key/value: %s", afterMute)
	}
	if !strings.Contains(afterMute, "1 muted") {
		t.Fatalf("workflow_manual after mute missing the muted-count line: %s", afterMute)
	}

	searchResp := callToolWithKey(t, s, 6, key, "note.search", map[string]any{
		"layer": "worktree",
		"glob":  "mute.me",
	})
	if !strings.Contains(searchResp, "mute.me") || !strings.Contains(searchResp, "muted content") {
		t.Fatalf("note.search after mute did not still return the muted note: %s", searchResp)
	}

	// Unmute restores the block.
	callToolWithKey(t, s, 7, key, "note.unmute", map[string]any{
		"layer": "worktree",
		"keys":  []any{"mute.me"},
	})
	afterUnmute := callToolWithKey(t, s, 8, key, "workflow_manual", nil)
	if !strings.Contains(afterUnmute, "mute.me") || !strings.Contains(afterUnmute, "muted content") {
		t.Fatalf("workflow_manual after unmute did not restore the note: %s", afterUnmute)
	}
	if strings.Contains(afterUnmute, "note.search to view") {
		t.Fatalf("workflow_manual after unmute unexpectedly still shows a muted-count line: %s", afterUnmute)
	}
}

// TestWorkflowManualMutingOverCapAdjacentNoteFreesSlotForElidedNote drives
// the cap-exclusion accounting through the REAL MCP path (note.write ->
// note.mute -> workflow_manual), not a direct wsnote.Compute call: with
// exactly notesInjectionCap+1 notes, the lowest-priority one starts elided;
// muting the highest-priority (cap-included) note must free its slot so the
// previously-elided lowest-priority note now renders.
func TestWorkflowManualMutingOverCapAdjacentNoteFreesSlotForElidedNote(t *testing.T) {
	setupWorkflowManualNoteEnv(t)
	root := t.TempDir()
	initGit(t, root)

	s := NewServer(root, "test")
	key, _ := parseLoginResponse(t, callLogin(t, s, 1, root, nil))

	total := notesInjectionCap + 1
	notes := make([]any, 0, total)
	for i := 0; i < total; i++ {
		notes = append(notes, map[string]any{
			"key":      fmt.Sprintf("cap.%02d", i),
			"value":    "v",
			"priority": i, // strictly increasing, so lowest-index keys are lowest priority
		})
	}
	callToolWithKey(t, s, 2, key, "note.write", map[string]any{
		"layer": "worktree",
		"notes": notes,
	})

	lowestKey := "cap.00"
	highestKey := fmt.Sprintf("cap.%02d", total-1)

	baseline := callToolWithKey(t, s, 3, key, "workflow_manual", nil)
	if strings.Contains(baseline, lowestKey+" ") {
		t.Fatalf("baseline workflow_manual unexpectedly included the lowest-priority note %s: %s", lowestKey, baseline)
	}
	if !strings.Contains(baseline, "1 lower-priority notes elided") {
		t.Fatalf("baseline workflow_manual missing the 1-elided line: %s", baseline)
	}

	callToolWithKey(t, s, 4, key, "note.mute", map[string]any{
		"layer": "worktree",
		"keys":  []any{highestKey},
	})

	afterMute := callToolWithKey(t, s, 5, key, "workflow_manual", nil)
	if strings.Contains(afterMute, highestKey) {
		t.Fatalf("workflow_manual after muting %s still rendered it: %s", highestKey, afterMute)
	}
	if !strings.Contains(afterMute, lowestKey) {
		t.Fatalf("workflow_manual after muting the highest-priority note did not free a slot for the previously-elided %s: %s", lowestKey, afterMute)
	}
	if strings.Contains(afterMute, "lower-priority notes elided") {
		t.Fatalf("workflow_manual after muting one of %d notes (now exactly at cap) unexpectedly still shows an elision line: %s", total, afterMute)
	}
	if !strings.Contains(afterMute, "1 muted") {
		t.Fatalf("workflow_manual after mute missing the muted-count line: %s", afterMute)
	}
}

// TestWorkflowManualRendersMutedLineAndElisionLineTogether drives the
// "both lines render independently and together" contract through the REAL
// MCP path: notesInjectionCap+3 notes written, the 2 lowest-priority muted
// via note.mute, leaving notesInjectionCap+1 visible notes — 1 over-cap
// elided and 2 muted — so both the elision line and the muted-count line
// must appear in the same workflow_manual response.
func TestWorkflowManualRendersMutedLineAndElisionLineTogether(t *testing.T) {
	setupWorkflowManualNoteEnv(t)
	root := t.TempDir()
	initGit(t, root)

	s := NewServer(root, "test")
	key, _ := parseLoginResponse(t, callLogin(t, s, 1, root, nil))

	total := notesInjectionCap + 3
	notes := make([]any, 0, total)
	for i := 0; i < total; i++ {
		notes = append(notes, map[string]any{
			"key":      fmt.Sprintf("both.%02d", i),
			"value":    "v",
			"priority": i, // strictly increasing, so lowest-index keys are lowest priority
		})
	}
	callToolWithKey(t, s, 2, key, "note.write", map[string]any{
		"layer": "worktree",
		"notes": notes,
	})

	// Mute the 2 lowest-priority notes: visible count drops to
	// notesInjectionCap+1, so 1 remains over-cap elided.
	callToolWithKey(t, s, 3, key, "note.mute", map[string]any{
		"layer": "worktree",
		"keys":  []any{"both.00", "both.01"},
	})

	resp := callToolWithKey(t, s, 4, key, "workflow_manual", nil)
	if !strings.Contains(resp, "1 lower-priority notes elided") {
		t.Fatalf("workflow_manual missing the elision line: %s", resp)
	}
	if !strings.Contains(resp, "2 muted") {
		t.Fatalf("workflow_manual missing the muted-count line: %s", resp)
	}
	elisionIdx := strings.Index(resp, "lower-priority notes elided")
	mutedIdx := strings.Index(resp, "note.search to view")
	if elisionIdx < 0 || mutedIdx < 0 || elisionIdx > mutedIdx {
		t.Fatalf("workflow_manual did not render the elision line before the muted line: %s", resp)
	}
	if strings.Contains(resp, "both.00") || strings.Contains(resp, "both.01") {
		t.Fatalf("workflow_manual rendered a muted note's bullet line, want it excluded entirely: %s", resp)
	}
}

// TestWorkflowManualAllMutedRendersHeadingOnly drives the all-muted edge
// case through the REAL MCP path: every note on a layer muted via
// note.mute must still render the "# Notes" heading and the muted-count
// line (zero bullet lines), distinct from the truly-empty case where the
// block is skipped entirely (TestWorkflowManualNotesBlockAbsentWhenNoNotesExist).
func TestWorkflowManualAllMutedRendersHeadingOnly(t *testing.T) {
	setupWorkflowManualNoteEnv(t)
	root := t.TempDir()
	initGit(t, root)

	s := NewServer(root, "test")
	key, _ := parseLoginResponse(t, callLogin(t, s, 1, root, nil))

	callToolWithKey(t, s, 2, key, "note.write", map[string]any{
		"layer": "worktree",
		"notes": []any{
			map[string]any{"key": "allmuted.a", "value": "va", "priority": 1},
			map[string]any{"key": "allmuted.b", "value": "vb", "priority": 2},
		},
	})
	callToolWithKey(t, s, 3, key, "note.mute", map[string]any{
		"layer": "worktree",
		"keys":  []any{"allmuted.a", "allmuted.b"},
	})

	resp := callToolWithKey(t, s, 4, key, "workflow_manual", nil)
	if !strings.Contains(resp, "# Notes") {
		t.Fatalf("workflow_manual with all notes muted = %s, want the block to still render (heading present)", resp)
	}
	if strings.Contains(resp, "- [worktree]") {
		t.Fatalf("workflow_manual with all notes muted rendered a bullet line, want zero: %s", resp)
	}
	if !strings.Contains(resp, "2 muted") {
		t.Fatalf("workflow_manual with all notes muted missing the muted-count line: %s", resp)
	}
	if strings.Contains(resp, "lower-priority notes elided") {
		t.Fatalf("workflow_manual with all notes muted (zero visible, zero over-cap) unexpectedly rendered an elision line: %s", resp)
	}
	if strings.Contains(resp, "allmuted.a") || strings.Contains(resp, "allmuted.b") {
		t.Fatalf("workflow_manual with all notes muted still shows a note key/value in the block: %s", resp)
	}
}
