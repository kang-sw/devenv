package wsdoc

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// countTicketReads counts board reads under root for the test's duration.
func countTicketReads(t *testing.T, root string) *atomic.Int64 {
	t.Helper()
	var n atomic.Int64
	orig := readTicketFile
	readTicketFile = func(path string) ([]byte, error) {
		if strings.HasPrefix(path, root) {
			n.Add(1)
		}
		return orig(path)
	}
	t.Cleanup(func() { readTicketFile = orig })
	return &n
}

func writeOldTicket(t *testing.T, root, rel, text string, age time.Duration) {
	t.Helper()
	mustWrite(t, root, rel, text)
	when := time.Now().Add(-age)
	if err := os.Chtimes(filepath.Join(root, rel), when, when); err != nil {
		t.Fatal(err)
	}
}

func TestTicketInventoryMatchesFreshScanAndSkipsRereads(t *testing.T) {
	root := t.TempDir()
	reads := countTicketReads(t, root)
	board := "ai-docs/tickets/"
	writeOldTicket(t, root, board+"ready/260504-ready-demo.md", "---\ntitle: Ready\n---\n# Ready\n", time.Hour)
	writeOldTicket(t, root, board+"todo/260504-todo-demo.md", "---\ntitle: Todo\nrelated:\n  260504-ready-demo: source\n---\n# Todo\n", time.Hour)
	writeOldTicket(t, root, board+".done/260504-done-demo.md", "---\ntitle: Done\n---\n# Done\n", time.Hour)
	all := TicketListOptions{IncludeDone: true, IncludeDropped: true}

	list := func() []TicketInfo {
		t.Helper()
		got, err := TicketsList(root, all)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	assertFresh := func(step string, got []TicketInfo) {
		t.Helper()
		before := reads.Load()
		ticketCache.Lock()
		saved := ticketCache.dirs
		ticketCache.dirs = map[string]map[string]ticketCacheEntry{}
		ticketCache.Unlock()
		fresh := list()
		ticketCache.Lock()
		ticketCache.dirs = saved
		ticketCache.Unlock()
		reads.Store(before)
		if !reflect.DeepEqual(got, fresh) {
			t.Fatalf("%s: cached %#v != fresh %#v", step, got, fresh)
		}
	}
	step := func(name string, wantReads int64) {
		t.Helper()
		reads.Store(0)
		got := list()
		if n := reads.Load(); n != wantReads {
			t.Fatalf("%s: %d reads, want %d", name, n, wantReads)
		}
		assertFresh(name, got)
	}

	step("cold", 3)
	step("repeat", 0)

	reads.Store(0)
	if _, err := TicketsStatus(root, TicketStatusOptions{TicketStem: "260504-todo-demo"}); err != nil {
		t.Fatal(err)
	}
	if found, err := TicketsFind(root, TicketFindOptions{TicketStem: "260504-ready-demo"}); err != nil || len(found) != 1 {
		t.Fatalf("find by stem = %v, %v", found, err)
	}
	if n := reads.Load(); n != 0 {
		t.Fatalf("stem queries read %d files, want 0", n)
	}

	// A caller editing a returned ticket must not reach the cache.
	got := list()
	got[0].Related = map[string]string{"x": "y"}
	got[1].Related["260504-ready-demo"] = "mutated"
	step("after caller mutation", 0)

	writeOldTicket(t, root, board+"idea/260504-idea-demo.md", "---\ntitle: Idea\n---\n# Idea\n", time.Hour)
	step("create", 1)

	if err := os.Rename(filepath.Join(root, board, "ready/260504-ready-demo.md"), filepath.Join(root, board, "todo/260504-ready-demo.md")); err != nil {
		t.Fatal(err)
	}
	step("move", 1)

	if err := os.MkdirAll(filepath.Join(root, board, ".dropped"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(root, board, "todo/260504-todo-demo.md"), filepath.Join(root, board, ".dropped/260504-todo-demo.md")); err != nil {
		t.Fatal(err)
	}
	step("close", 1)

	// An external same-size edit with a new mtime is reread.
	writeOldTicket(t, root, board+".done/260504-done-demo.md", "---\ntitle: Dune\n---\n# Dune\n", 30*time.Minute)
	step("external edit", 1)
	if findTicket(t, list(), "260504-done-demo").Title != "Dune" {
		t.Fatal("external edit not visible")
	}

	// A file modified inside the racy window is never served from cache.
	mustWrite(t, root, board+"idea/260504-idea-demo.md", "---\ntitle: Idea2\n---\n# Idea\n")
	step("racy write", 1)
	step("racy write again", 1)
}
