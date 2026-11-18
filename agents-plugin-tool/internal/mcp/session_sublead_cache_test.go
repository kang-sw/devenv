package mcp

import (
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
