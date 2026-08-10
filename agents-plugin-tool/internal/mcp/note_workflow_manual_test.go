package mcp

import (
	"fmt"
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

// TestWorkflowManualNotesBlockAbsentWhenNoNotesExist verifies the injection is
// a true no-op (scopeAnnouncement/computeManuals-style silent case) when no
// note has ever been written on either layer.
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
