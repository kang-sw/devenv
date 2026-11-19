package mcp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsstate"
)

// subleadMarkerReads counts read_file attempts at the marker read site.
func subleadMarkerReads() int64 {
	var n int64
	for site, count := range wsstate.FileIOCounters() {
		if strings.HasPrefix(site, "file_io_site:mcp/session_sublead.go:") && strings.HasSuffix(site, ":read_file") {
			n += count
		}
	}
	return n
}

// A sub-lead's steady-state whiteboard calls do not reopen its marker, and a
// parented lead without a marker is memoized as unmarked too.
func TestSubleadMarkerReadIsMemoized(t *testing.T) {
	f := newWhiteboardFixture(t)
	defer f.s.Close()
	seedLeadBoard(t, f.s, f.key, subleadSeed)
	sub := mintSublead(t, f.s, f.root, f.key, "ticket-runner")
	plain, err := f.s.sessions.mint(f.root, roleLead, f.key)
	if err != nil {
		t.Fatal(err)
	}
	// Model a restarted daemon: the marker exists on disk, nothing cached.
	f.s.sessions.sublead.Delete(sub)

	query := func(key string) string {
		t.Helper()
		return f.call(t, "whiteboard.query", map[string]any{"session_key": key})
	}
	before := subleadMarkerReads()
	if resp := query(sub); isToolError(resp) {
		t.Fatalf("sub-lead whiteboard.query: %s", toolText(t, resp))
	}
	if got := subleadMarkerReads() - before; got != 1 {
		t.Fatalf("first sub-lead query read the marker %d times, want 1", got)
	}
	before = subleadMarkerReads()
	query(sub)
	if got := subleadMarkerReads() - before; got != 0 {
		t.Fatalf("second sub-lead query read the marker %d times, want 0", got)
	}

	before = subleadMarkerReads()
	query(plain)
	query(plain)
	if got := subleadMarkerReads() - before; got != 1 {
		t.Fatalf("two unmarked parented-lead queries read the marker %d times, want 1", got)
	}
	if _, ok := f.s.sessions.subleadPlaybook(plain); ok {
		t.Fatal("an unmarked key must stay unmarked")
	}
	if author, ok := f.s.sessions.subleadPlaybook(sub); !ok || author != "ticket-runner" {
		t.Fatalf("cached marker = %q, %v", author, ok)
	}
}

func TestSubleadMarkerRetriesReadError(t *testing.T) {
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	s := newSessionStore()
	d, err := s.dirs()
	if err != nil {
		t.Fatal(err)
	}
	key := "retry-marker-00"
	path := filepath.Join(d.keyDir(key), subleadMarkerName)
	// A directory gives a portable non-ENOENT read failure, even as root.
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	before := subleadMarkerReads()
	if _, ok := s.subleadPlaybook(key); ok {
		t.Fatal("an unreadable marker authorized the key")
	}
	if _, cached := s.sublead.Load(key); cached {
		t.Fatal("a read failure must not be cached as unmarked")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	// Write directly so writeSubleadMarker cannot seed the cache for us.
	if err := os.WriteFile(path, []byte(`{"playbook":"ticket-runner"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if author, ok := s.subleadPlaybook(key); !ok || author != "ticket-runner" {
		t.Fatalf("retry = %q, %v", author, ok)
	}
	s.subleadPlaybook(key)
	if got := subleadMarkerReads() - before; got != 2 {
		t.Fatalf("failed read plus successful retry and cached hit read %d times, want 2", got)
	}
}

func TestSubleadMarkerReclaimedKeyInvalidatesCache(t *testing.T) {
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	s := newSessionStore()
	d, err := s.dirs()
	if err != nil {
		t.Fatal(err)
	}
	key := "reclaimed-marker-00"
	payload := []byte(`{"schema_version":2,"root":"/w","scope":"lead"}`)
	for _, marked := range []bool{true, false} {
		if claimed, err := s.claimKey(d, key, payload); err != nil || !claimed {
			t.Fatalf("initial claim = %v, %v", claimed, err)
		}
		if marked {
			if err := s.writeSubleadMarker(key, "old-playbook"); err != nil {
				t.Fatal(err)
			}
		}
		if _, ok := s.subleadPlaybook(key); ok != marked {
			t.Fatalf("initial marker = %v, want %v", ok, marked)
		}
		// Model retention pruning without discarding the process's memo.
		if err := os.RemoveAll(d.keyDir(key)); err != nil {
			t.Fatal(err)
		}
		if claimed, err := s.claimKey(d, key, payload); err != nil || !claimed {
			t.Fatalf("reclaim = %v, %v", claimed, err)
		}
		if _, cached := s.sublead.Load(key); cached {
			t.Fatal("reclaim retained the previous key's marker cache")
		}
		// An unmarked successor must not inherit the old key's authority.
		if _, ok := s.subleadPlaybook(key); ok {
			t.Fatal("reclaimed key inherited sub-lead authority")
		}
		if err := os.RemoveAll(d.keyDir(key)); err != nil {
			t.Fatal(err)
		}
	}
}
