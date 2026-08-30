package mcp

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// --- Touch: mtime refresh on successful keyed resolution -------------------

// backdateMtime rewrites path's mtime to age in the past, simulating a
// session record that has gone idle without needing an injectable clock on
// sessionStore.
func backdateMtime(t *testing.T, path string, age time.Duration) {
	t.Helper()
	then := time.Now().Add(-age)
	if err := os.Chtimes(path, then, then); err != nil {
		t.Fatalf("backdate mtime for %q: %v", path, err)
	}
}

func mtimeOf(t *testing.T, path string) time.Time {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %q: %v", path, err)
	}
	return info.ModTime()
}

// TestLookupTouchesStaleRecordAndGuardsRepeat covers the touch-guard
// requirement: lookup() on a stale record (mtime older than touchGuardWindow)
// refreshes the mtime, but an immediate second lookup does not advance it
// again (throttled within the guard window).
func TestLookupTouchesStaleRecordAndGuardsRepeat(t *testing.T) {
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	s := newSessionStore()
	key, err := s.mint("/some/root", roleLead, "")
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	dir, err := s.keysDir()
	if err != nil {
		t.Fatalf("keysDir: %v", err)
	}
	path := s.keyPath(dir, key)
	backdateMtime(t, path, touchGuardWindow+time.Minute)
	staleMtime := mtimeOf(t, path)

	if _, ok := s.lookup(key); !ok {
		t.Fatalf("lookup(%q) failed", key)
	}
	refreshed := mtimeOf(t, path)
	if !refreshed.After(staleMtime) {
		t.Fatalf("lookup on stale record did not refresh mtime: before=%v after=%v", staleMtime, refreshed)
	}

	// Immediate second lookup: within touchGuardWindow of the first touch, so
	// the mtime must not advance again.
	if _, ok := s.lookup(key); !ok {
		t.Fatalf("second lookup(%q) failed", key)
	}
	again := mtimeOf(t, path)
	if !again.Equal(refreshed) {
		t.Fatalf("lookup within touch guard window advanced mtime again: first=%v second=%v", refreshed, again)
	}
}

// TestReadStateTouchesRecord covers the read-only session-state seam
// (readState, used by ws.todo.list/ws.agenda.list) which bypasses lookup and
// must get its own touch call.
func TestReadStateTouchesRecord(t *testing.T) {
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	s := newSessionStore()
	key, err := s.mint("/some/root", roleLead, "")
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	dir, _ := s.keysDir()
	path := s.keyPath(dir, key)
	backdateMtime(t, path, touchGuardWindow+time.Minute)
	staleMtime := mtimeOf(t, path)

	if _, ok := s.readState(key); !ok {
		t.Fatalf("readState(%q) failed", key)
	}
	refreshed := mtimeOf(t, path)
	if !refreshed.After(staleMtime) {
		t.Fatalf("readState did not refresh mtime: before=%v after=%v", staleMtime, refreshed)
	}
}

// TestReadStateTouchesRecordViaMCPTodoList exercises the same readState seam
// through the actual MCP surface (ws.todo.list) rather than calling the Go
// method directly, matching how a real caller would trigger the touch.
func TestReadStateTouchesRecordViaMCPTodoList(t *testing.T) {
	useLeadProfile(t)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	root := t.TempDir()
	initGit(t, root)
	server := NewServer(root, "test")

	resp := callLogin(t, server, 1, root, nil)
	if toolIsError(t, resp) {
		t.Fatalf("ws.ferrule returned isError: %s", resp)
	}
	key, _ := parseLoginResponse(t, resp)

	dir, _ := server.sessions.keysDir()
	path := server.sessions.keyPath(dir, key)
	backdateMtime(t, path, touchGuardWindow+time.Minute)
	staleMtime := mtimeOf(t, path)

	listResp := callToolWithKey(t, server, 2, key, "todo.list", nil)
	if listResp == "" {
		t.Fatalf("todo.list returned empty response")
	}
	refreshed := mtimeOf(t, path)
	if !refreshed.After(staleMtime) {
		t.Fatalf("todo.list (readState) did not refresh mtime: before=%v after=%v", staleMtime, refreshed)
	}
}

// TestGetOverrideAndListOverrideKeysTouchRecord covers the two override-read
// seams (session_config_adapter's wsconfig.SessionReader path) named
// explicitly by the ticket as bypass seams.
func TestGetOverrideAndListOverrideKeysTouchRecord(t *testing.T) {
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	s := newSessionStore()
	key, err := s.mint("/some/root", roleLead, "")
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	if err := s.setOverride(key, "some.item", "value"); err != nil {
		t.Fatalf("setOverride: %v", err)
	}
	dir, _ := s.keysDir()
	path := s.keyPath(dir, key)

	// getOverride
	backdateMtime(t, path, touchGuardWindow+time.Minute)
	staleMtime := mtimeOf(t, path)
	if _, ok := s.getOverride(key, "some.item"); !ok {
		t.Fatalf("getOverride did not find the item")
	}
	refreshed := mtimeOf(t, path)
	if !refreshed.After(staleMtime) {
		t.Fatalf("getOverride did not refresh mtime: before=%v after=%v", staleMtime, refreshed)
	}

	// listOverrideKeys
	backdateMtime(t, path, touchGuardWindow+time.Minute)
	staleMtime = mtimeOf(t, path)
	if keys := s.listOverrideKeys(key); len(keys) != 1 {
		t.Fatalf("listOverrideKeys = %v, want 1 entry", keys)
	}
	refreshed = mtimeOf(t, path)
	if !refreshed.After(staleMtime) {
		t.Fatalf("listOverrideKeys did not refresh mtime: before=%v after=%v", staleMtime, refreshed)
	}
}

// TestChildrenTouchesOnlyParentKey confirms children() touches parentKey's own
// record but does not touch every record visited while walking the directory
// to build the adjacency map.
func TestChildrenTouchesOnlyParentKey(t *testing.T) {
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	s := newSessionStore()
	parentKey, err := s.mint("/parent/root", roleLead, "")
	if err != nil {
		t.Fatalf("mint parent: %v", err)
	}
	childKey, err := s.mint("/child/root", roleLead, parentKey)
	if err != nil {
		t.Fatalf("mint child: %v", err)
	}
	dir, _ := s.keysDir()
	parentPath := s.keyPath(dir, parentKey)
	childPath := s.keyPath(dir, childKey)

	backdateMtime(t, parentPath, touchGuardWindow+time.Minute)
	backdateMtime(t, childPath, touchGuardWindow+time.Minute)
	staleParent := mtimeOf(t, parentPath)
	staleChild := mtimeOf(t, childPath)

	if _, err := s.children(parentKey, 0); err != nil {
		t.Fatalf("children: %v", err)
	}

	if refreshed := mtimeOf(t, parentPath); !refreshed.After(staleParent) {
		t.Fatalf("children did not touch parentKey's own record: before=%v after=%v", staleParent, refreshed)
	}
	if still := mtimeOf(t, childPath); !still.Equal(staleChild) {
		t.Fatalf("children must not touch scanned child records: before=%v after=%v", staleChild, still)
	}
}

// --- Prune: cadence, stale deletion, retention, malformed resilience -------

// TestFerruleDoesNotDoubleScanWithinCadence calls ws.ferrule twice in quick
// succession and asserts the prune marker's mtime only advances once (the
// second call is inside pruneScanCadence and must skip the scan).
func TestFerruleDoesNotDoubleScanWithinCadence(t *testing.T) {
	useLeadProfile(t)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	root := t.TempDir()
	initGit(t, root)
	server := NewServer(root, "test")

	resp1 := callLogin(t, server, 1, root, nil)
	if toolIsError(t, resp1) {
		t.Fatalf("first ws.ferrule returned isError: %s", resp1)
	}
	dir, _ := server.sessions.keysDir()
	markerPath := filepath.Join(dir, pruneMarkerName)
	firstMarker := mtimeOf(t, markerPath)

	resp2 := callLogin(t, server, 2, root, nil)
	if toolIsError(t, resp2) {
		t.Fatalf("second ws.ferrule returned isError: %s", resp2)
	}
	secondMarker := mtimeOf(t, markerPath)
	if !secondMarker.Equal(firstMarker) {
		t.Fatalf("second ferrule within cadence re-scanned: first=%v second=%v", firstMarker, secondMarker)
	}

	// Backdate the marker beyond cadence and confirm the next ferrule call
	// scans again (marker mtime advances).
	backdateMtime(t, markerPath, pruneScanCadence+time.Minute)
	staleMarker := mtimeOf(t, markerPath)
	resp3 := callLogin(t, server, 3, root, nil)
	if toolIsError(t, resp3) {
		t.Fatalf("third ws.ferrule returned isError: %s", resp3)
	}
	thirdMarker := mtimeOf(t, markerPath)
	if !thirdMarker.After(staleMarker) {
		t.Fatalf("ferrule did not re-scan after marker aged past cadence: before=%v after=%v", staleMarker, thirdMarker)
	}
}

// TestMaybePruneDeletesStaleKeyRecords covers stale-key deletion: a record
// whose mtime exceeds keyRetentionAge is removed by a forced prune scan.
func TestMaybePruneDeletesStaleKeyRecords(t *testing.T) {
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	s := newSessionStore()
	staleKey, err := s.mint("/stale/root", roleLead, "")
	if err != nil {
		t.Fatalf("mint stale: %v", err)
	}
	dir, _ := s.keysDir()
	stalePath := s.keyPath(dir, staleKey)
	backdateMtime(t, stalePath, keyRetentionAge+time.Hour)

	s.maybePrune()

	if _, err := os.Stat(stalePath); !os.IsNotExist(err) {
		t.Fatalf("stale key record was not pruned: stat err = %v", err)
	}
}

// TestMaybePruneRetainsRecentlyTouchedRecords covers retention of recently
// touched records: a record freshly touched must survive a forced prune scan
// purely because mtime governs, even if it would otherwise look old by some
// other measure.
func TestMaybePruneRetainsRecentlyTouchedRecords(t *testing.T) {
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	s := newSessionStore()
	freshKey, err := s.mint("/fresh/root", roleLead, "")
	if err != nil {
		t.Fatalf("mint fresh: %v", err)
	}
	dir, _ := s.keysDir()
	freshPath := s.keyPath(dir, freshKey)
	// Freshly minted: mtime is "now", well within retention.

	s.maybePrune()

	if _, err := os.Stat(freshPath); err != nil {
		t.Fatalf("freshly touched key record was pruned: %v", err)
	}
}

// TestMaybePruneToleratesMalformedFiles drops a non-JSON-parseable .json file
// into keys/ and confirms a forced prune scan neither crashes nor special-
// cases it: it is pruned/kept purely by mtime like any other file, since the
// scan never parses record contents.
func TestMaybePruneToleratesMalformedFiles(t *testing.T) {
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	s := newSessionStore()
	dir, err := s.keysDir()
	if err != nil {
		t.Fatalf("keysDir: %v", err)
	}
	malformedPath := filepath.Join(dir, "not-valid-json.json")
	if err := os.WriteFile(malformedPath, []byte("{not valid json"), 0o644); err != nil {
		t.Fatalf("write malformed file: %v", err)
	}
	backdateMtime(t, malformedPath, keyRetentionAge+time.Hour)

	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("maybePrune panicked on malformed file: %v", r)
			}
		}()
		s.maybePrune()
	}()

	if _, err := os.Stat(malformedPath); !os.IsNotExist(err) {
		t.Fatalf("stale malformed file was not pruned: stat err = %v", err)
	}
}

// TestMaybePruneSkipsWithinCadence directly exercises maybePrune's cadence
// guard: a fresh marker (created by a prior call) prevents a second call from
// deleting a record it would otherwise remove.
func TestMaybePruneSkipsWithinCadence(t *testing.T) {
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	s := newSessionStore()
	staleKey, err := s.mint("/stale/root", roleLead, "")
	if err != nil {
		t.Fatalf("mint stale: %v", err)
	}
	dir, _ := s.keysDir()
	stalePath := s.keyPath(dir, staleKey)

	// First call claims the marker without anything to prune yet.
	s.maybePrune()

	// Now backdate the key record past retention, but the marker is fresh, so
	// a second call within cadence must skip the scan and leave it in place.
	backdateMtime(t, stalePath, keyRetentionAge+time.Hour)
	s.maybePrune()

	if _, err := os.Stat(stalePath); err != nil {
		t.Fatalf("record pruned despite being inside scan cadence: %v", err)
	}
}
