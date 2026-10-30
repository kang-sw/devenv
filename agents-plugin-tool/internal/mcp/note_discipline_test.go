package mcp

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// TestNoteWriteReasonRequiredForCloneAndMachine: a clone or machine write
// without a non-empty reason is rejected and writes nothing; worktree and
// repo writes need none; the reason is never stored.
func TestNoteWriteReasonRequiredForCloneAndMachine(t *testing.T) {
	setupNoteTestEnv(t)
	s := NewServer("", "test")
	_, key := mintRootKey(t, s, 1)

	id := 10
	for _, layer := range []string{"clone", "machine"} {
		for _, reason := range []any{nil, "", "   "} {
			id++
			args := map[string]any{
				"layer": layer,
				"notes": []any{map[string]any{"key": "no.reason." + layer, "value": "v", "priority": 1}},
			}
			if reason != nil {
				args["reason"] = reason
			}
			resp := callToolWithKey(t, s, id, key, "note.write", args)
			if !strings.Contains(resp, fmt.Sprintf(`layer %q requires a non-empty reason`, layer)) || !strings.Contains(resp, `layer "worktree"`) {
				t.Fatalf("layer %s reason %#v: want reason rejection naming the worktree fallback, got %q", layer, reason, resp)
			}
			id++
			if got := callToolWithKey(t, s, id, key, "note.query", map[string]any{"layer": layer, "glob": "no.reason.*"}); got != "no notes matched\n" {
				t.Fatalf("layer %s: rejected write still stored a note: %q", layer, got)
			}
		}
	}

	for _, layer := range []string{"worktree", "repo"} {
		id++
		resp := callToolWithKey(t, s, id, key, "note.write", map[string]any{
			"layer": layer,
			"notes": []any{map[string]any{"key": "plain." + layer, "value": "v", "priority": 1}},
		})
		if !strings.Contains(resp, "wrote 1 note(s)") {
			t.Fatalf("layer %s without reason: want write, got %q", layer, resp)
		}
	}

	const reason = "every worktree's build reads this host path"
	id++
	resp := callToolWithKey(t, s, id, key, "note.write", map[string]any{
		"layer":  "clone",
		"reason": reason,
		"notes":  []any{map[string]any{"key": "with.reason", "value": "v", "priority": 1}},
	})
	if !strings.Contains(resp, "wrote 1 note(s)") {
		t.Fatalf("clone with reason: want write, got %q", resp)
	}
	id++
	stored := callToolWithKey(t, s, id, key, "note.query", map[string]any{"layer": "clone", "glob": "with.reason", "format": "json"})
	if !strings.Contains(stored, "with.reason") || strings.Contains(stored, reason) {
		t.Fatalf("clone note must be stored without its reason: %q", stored)
	}
}

// TestNoteMutationDeniedForNonTopLevelKeys: delegate, leaf, ticket-worker
// (lead scope, lead parent), and spawned-fork (lead scope, non-empty parent)
// keys can query notes but cannot write, erase, mute, or unmute them, and the
// denial names the report hand-off instead of the generic profile text.
func TestNoteMutationDeniedForNonTopLevelKeys(t *testing.T) {
	setupNoteTestEnv(t)
	s := NewServer("", "test")
	root, leadKey := mintRootKey(t, s, 1)
	canonical := canonicalRootForTest(t, root)
	callToolWithKey(t, s, 2, leadKey, "note.write", map[string]any{
		"layer": "worktree",
		"notes": []any{map[string]any{"key": "lead.owned", "value": "kept", "priority": 1}},
	})

	forkSpawner, err := s.sessions.mint(canonical, roleLead, leadKey)
	if err != nil {
		t.Fatalf("mint fork spawner: %v", err)
	}
	children := map[string]struct {
		scope  toolRole
		parent string
	}{
		"delegate":      {roleDelegate, leadKey},
		"leaf":          {roleLeaf, leadKey},
		"ticket-worker": {roleLead, leadKey},
		"spawned-fork":  {roleLead, forkSpawner},
	}
	mutations := map[string]map[string]any{
		"note.write":  {"layer": "worktree", "notes": []any{map[string]any{"key": "child.write", "value": "v", "priority": 1}}},
		"note.erase":  {"layer": "worktree", "keys": []any{"lead.owned"}},
		"note.mute":   {"layer": "worktree", "keys": []any{"lead.owned"}},
		"note.unmute": {"layer": "worktree", "keys": []any{"lead.owned"}},
	}

	id := 100
	for name, shape := range children {
		childKey, err := s.sessions.mint(canonical, shape.scope, shape.parent)
		if err != nil {
			t.Fatalf("mint %s: %v", name, err)
		}
		for tool, args := range mutations {
			id++
			call := map[string]any{"session_key": childKey}
			for k, v := range args {
				call[k] = v
			}
			raw := callToolOnce(t, s, id, tool, call)
			if !strings.Contains(raw, "-32601") || !strings.Contains(raw, tool+" is not available to a spawned session") {
				t.Fatalf("%s %s: want note read-only denial, got %s", name, tool, raw)
			}
			if !strings.Contains(raw, "final report so your lead can write it") || !strings.Contains(raw, "erasing, muting, and unmuting notes is also your lead's job") {
				t.Fatalf("%s %s: denial must name the report hand-off and the lead's erase duty: %s", name, tool, raw)
			}
			if strings.Contains(raw, "tool not available in current") {
				t.Fatalf("%s %s: denial must not be the generic profile text: %s", name, tool, raw)
			}
		}
		id++
		got := callToolWithKey(t, s, id, childKey, "note.query", map[string]any{"layer": "worktree"})
		if !strings.Contains(got, "lead.owned") || !strings.Contains(got, "kept") {
			t.Fatalf("%s: note.query must stay allowed and see the lead's note: %q", name, got)
		}
		if strings.Contains(got, "child.write") {
			t.Fatalf("%s: a denied note.write still stored a note: %q", name, got)
		}
	}

	// The lead itself still mutates freely, and the denied mute left the note visible.
	id++
	if got := callToolWithKey(t, s, id, leadKey, "workflow_state", nil); !strings.Contains(got, "lead.owned") {
		t.Fatalf("a denied child mute changed the lead's Notes block: %q", got)
	}
	id++
	if got := callToolWithKey(t, s, id, leadKey, "note.erase", map[string]any{"layer": "worktree", "keys": []any{"lead.owned"}}); !strings.Contains(got, "erased 1 note(s)") {
		t.Fatalf("top-level lead note.erase: want success, got %q", got)
	}
}

// overflowCloneNotes writes enough clone-layer notes under key that one is
// hidden past both display tiers in every worktree of the clone.
func overflowCloneNotes(t *testing.T, s *Server, id int, key string) {
	t.Helper()
	total := notesInjectionCap + notesKeyOnlyCap + 1
	notes := make([]any, 0, total)
	for i := 0; i < total; i++ {
		notes = append(notes, map[string]any{"key": fmt.Sprintf("flood.%02d", i), "value": "v", "priority": i})
	}
	resp := callToolWithKey(t, s, id, key, "note.write", map[string]any{
		"layer":  "clone",
		"reason": "test fixture: every worktree of the clone sees it",
		"notes":  notes,
	})
	if !strings.Contains(resp, fmt.Sprintf("wrote %d note(s)", total)) {
		t.Fatalf("overflow write failed: %q", resp)
	}
}

func setNotesCleanupClock(t *testing.T, at *time.Time) {
	t.Helper()
	orig := notesCleanupNow
	notesCleanupNow = func() time.Time { return *at }
	t.Cleanup(func() { notesCleanupNow = orig })
}

const notesOverflowMarker = "**Notes overflow: 1 hidden notes**"

func assertCleanupWarning(t *testing.T, label, body string, want bool) {
	t.Helper()
	got := strings.Contains(body, notesOverflowMarker)
	if got != want {
		t.Fatalf("%s: cleanup warning present = %v, want %v:\n%s", label, got, want, body)
	}
	if !strings.Contains(body, "+1 hidden — ") && strings.Contains(body, "# Notes") {
		t.Fatalf("%s: the +N hidden line must stay in the Notes block on every render:\n%s", label, body)
	}
	if !want {
		return
	}
	if !strings.Contains(body, "(clone 1)") || !strings.Contains(body, "`note.query` the hidden notes, `note.erase` every stale one, and move still-needed detail into a ticket or a manual. Muting does not fix this.") {
		t.Fatalf("%s: warning missing the per-layer breakdown or remediation:\n%s", label, body)
	}
	if strings.Index(body, notesOverflowMarker) > strings.Index(body, "## Session Key") {
		t.Fatalf("%s: warning must sit in the banner region, ahead of ## Session Key:\n%s", label, body)
	}
}

// TestNotesCleanupWarningThrottledPerCloneForTopLevelLeads: the warning
// renders on a top-level lead's workflow_manual at most once per day per
// clone; a ticket-worker key and workflow_state neither render nor consume
// it.
func TestNotesCleanupWarningThrottledPerCloneForTopLevelLeads(t *testing.T) {
	setupWorkflowManualNoteEnv(t)
	mainRoot, linkedRoot := twoWorktreesOfOneRepo(t)
	s := NewServer("", "test")
	mainKey, _ := parseLoginResponse(t, callLogin(t, s, 1, mainRoot, nil))
	linkedKey, _ := parseLoginResponse(t, callLogin(t, s, 2, linkedRoot, nil))
	workerKey, err := s.sessions.mint(canonicalRootForTest(t, mainRoot), roleLead, mainKey)
	if err != nil {
		t.Fatalf("mint ticket-worker key: %v", err)
	}
	overflowCloneNotes(t, s, 3, mainKey)

	now := time.Date(2026, 10, 31, 9, 0, 0, 0, time.UTC)
	setNotesCleanupClock(t, &now)

	assertCleanupWarning(t, "ticket-worker", callToolWithKey(t, s, 4, workerKey, "workflow_manual", nil), false)
	state := callToolWithKey(t, s, 5, mainKey, "workflow_state", nil)
	if strings.Contains(state, "Notes overflow") || !strings.Contains(state, "+1 hidden — ") {
		t.Fatalf("workflow_state must carry the +N hidden line but never the warning:\n%s", state)
	}
	assertCleanupWarning(t, "first top-level lead call", callToolWithKey(t, s, 6, mainKey, "workflow_manual", nil), true)
	assertCleanupWarning(t, "same lead, same day", callToolWithKey(t, s, 7, mainKey, "workflow_manual", nil), false)

	now = now.Add(23 * time.Hour)
	assertCleanupWarning(t, "other worktree of the clone, within the day", callToolWithKey(t, s, 8, linkedKey, "workflow_manual", nil), false)

	now = now.Add(2 * time.Hour)
	assertCleanupWarning(t, "ticket-worker after the day", callToolWithKey(t, s, 9, workerKey, "workflow_manual", nil), false)
	assertCleanupWarning(t, "other worktree after the day", callToolWithKey(t, s, 10, linkedKey, "workflow_manual", nil), true)
	assertCleanupWarning(t, "first worktree after the other consumed", callToolWithKey(t, s, 11, mainKey, "workflow_manual", nil), false)
}

// TestNotesCleanupWarningOnFreshSentinelWithRoot: the fresh-sentinel-with-root
// branch mints a top-level lead, so it renders and consumes the slot.
func TestNotesCleanupWarningOnFreshSentinelWithRoot(t *testing.T) {
	setupWorkflowManualNoteEnv(t)
	root := t.TempDir()
	initGit(t, root)
	s := NewServer(root, "test")
	key, _ := parseLoginResponse(t, callLogin(t, s, 1, root, nil))
	overflowCloneNotes(t, s, 2, key)

	now := time.Date(2026, 10, 31, 9, 0, 0, 0, time.UTC)
	setNotesCleanupClock(t, &now)

	assertCleanupWarning(t, "fresh sentinel with root", callToolWithKey(t, s, 3, freshBootstrapKey, "workflow_manual", map[string]any{"root": root}), true)
	assertCleanupWarning(t, "continue after fresh consumed", callToolWithKey(t, s, 4, key, "workflow_manual", nil), false)
}

// TestNotesCleanupWarningAbsentWithoutHiddenNotes: key-only overflow alone
// raises no warning and spends no slot.
func TestNotesCleanupWarningAbsentWithoutHiddenNotes(t *testing.T) {
	setupWorkflowManualNoteEnv(t)
	root := t.TempDir()
	initGit(t, root)
	s := NewServer(root, "test")
	key, _ := parseLoginResponse(t, callLogin(t, s, 1, root, nil))
	notes := make([]any, 0, notesInjectionCap+notesKeyOnlyCap)
	for i := 0; i < notesInjectionCap+notesKeyOnlyCap; i++ {
		notes = append(notes, map[string]any{"key": fmt.Sprintf("full.%02d", i), "value": "v", "priority": i})
	}
	callToolWithKey(t, s, 2, key, "note.write", map[string]any{"layer": "worktree", "notes": notes})

	now := time.Date(2026, 10, 31, 9, 0, 0, 0, time.UTC)
	setNotesCleanupClock(t, &now)
	if body := callToolWithKey(t, s, 3, key, "workflow_manual", nil); strings.Contains(body, "Notes overflow") || strings.Contains(body, " hidden — ") {
		t.Fatalf("50 notes fill both tiers exactly; want no hidden line and no warning:\n%s", body)
	}

	callToolWithKey(t, s, 4, key, "note.write", map[string]any{
		"layer":  "clone",
		"reason": "test fixture: the overflowing note",
		"notes":  []any{map[string]any{"key": "full.extra", "value": "v", "priority": -1}},
	})
	assertCleanupWarning(t, "first hidden note, same instant", callToolWithKey(t, s, 5, key, "workflow_manual", nil), true)
}
