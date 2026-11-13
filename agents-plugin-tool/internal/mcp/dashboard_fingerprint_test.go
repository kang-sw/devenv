package mcp

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wsdoc"
	"github.com/kang-sw/devenv/internal/wsmailbox"
)

// countTicketLists counts the ticket panel's ticket reads for one test.
func countTicketLists(t *testing.T) *atomic.Int64 {
	t.Helper()
	var n atomic.Int64
	prev := whiteboardTicketsList
	whiteboardTicketsList = func(root string, opts wsdoc.TicketListOptions) ([]wsdoc.TicketInfo, error) {
		n.Add(1)
		return prev(root, opts)
	}
	t.Cleanup(func() { whiteboardTicketsList = prev })
	return &n
}

// TestDashboardFingerprintReadsNoTicket pins the nudge-path fingerprint:
// ReadSession lists no ticket (no panel build), however often it runs, yet
// its fingerprint changes on a ticket add, edit (a size change, or the same
// size with a newer modification time), rename, move between the listed
// directories, and remove, on the reply-id unread count, and on the
// listening marker. The counters tell its changed reads from its unchanged
// ones, and the render path still builds the panels.
func TestDashboardFingerprintReadsNoTicket(t *testing.T) {
	f := newWhiteboardFixture(t)
	now := time.Now()
	src := dashboardPageSource(t, &now)
	reg := wsdashboard.Registration{ReplyID: f.replyID(), Project: f.root}
	writeTicket(t, f.root, "ready", "261101-feat-one", "One")
	f.update(t, `<wb:topic name="t"><wb:add class="technical"><wb:title>a question</wb:title></wb:add></wb:topic>`)
	lists := countTicketLists(t)

	fp := readFingerprint(t, src, reg)
	for i := 0; i < 5; i++ {
		if again := readFingerprint(t, src, reg); again != fp {
			t.Fatal("an unchanged session changed its fingerprint")
		}
	}
	if c := src.Counters(); c["session_reads_unchanged"] != 5 || c["session_reads_changed"] != 1 {
		t.Fatalf("counters after 1 first and 5 unchanged reads = %v", c)
	}
	ticket := func(status, stem string) string {
		return filepath.Join(f.root, "ai-docs", "tickets", status, stem+".md")
	}
	changed := int64(1)
	step := func(what string, change func()) {
		t.Helper()
		change()
		next := readFingerprint(t, src, reg)
		if next == fp {
			t.Fatalf("%s kept the fingerprint", what)
		}
		fp = next
		changed++
		if got := src.Counters()["session_reads_changed"]; got != changed {
			t.Fatalf("%s: session_reads_changed = %d, want %d", what, got, changed)
		}
	}
	step("a ticket added", func() { writeTicket(t, f.root, "todo", "261102-feat-two", "Two") })
	step("a ticket edited (size)", func() { writeTicket(t, f.root, "todo", "261102-feat-two", "Two, longer") })
	step("a ticket edited (same size, newer mtime)", func() {
		path := ticket("todo", "261102-feat-two")
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		raw[len(raw)-2] = 'X'
		if err := os.WriteFile(path, raw, 0o644); err != nil {
			t.Fatal(err)
		}
		at := time.Now().Add(2 * time.Second)
		if err := os.Chtimes(path, at, at); err != nil {
			t.Fatal(err)
		}
	})
	step("a ticket renamed", func() {
		if err := os.Rename(ticket("todo", "261102-feat-two"), ticket("todo", "261102-feat-renamed")); err != nil {
			t.Fatal(err)
		}
	})
	step("a ticket moved to ready", func() {
		if err := os.Rename(ticket("todo", "261102-feat-renamed"), ticket("ready", "261102-feat-renamed")); err != nil {
			t.Fatal(err)
		}
	})
	step("a ticket removed", func() {
		if err := os.Remove(ticket("ready", "261102-feat-renamed")); err != nil {
			t.Fatal(err)
		}
	})
	replyPath, err := wsmailbox.ReplyRegistryPath()
	if err != nil {
		t.Fatal(err)
	}
	step("an unread reply-id message", func() {
		if err := wsmailbox.WithReplyLock(replyPath, func(st *wsmailbox.ReplyStore) error {
			st.Queues = wsmailbox.AppendQueue(st.Queues, f.replyID(), wsmailbox.Envelope{From: "id:peer", Content: "hi", SentAt: now.UTC().Format(time.RFC3339)})
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	})
	step("a listening marker", func() {
		if err := wsmailbox.WriteListeningMarker(wsmailbox.ListeningMarker{SessionKey: f.key, PID: os.Getpid(), StartedAt: now.Format(time.RFC3339)}); err != nil {
			t.Fatal(err)
		}
	})
	if n := lists.Load(); n != 0 {
		t.Fatalf("ReadSession listed the tickets %d times, want none", n)
	}
	readPageOK(t, src, reg)
	if lists.Load() == 0 {
		t.Fatal("the page read must still build the ticket panel")
	}
}

// TestDashboardHistoryCacheReadsEachEntryOnce pins the served page's
// mailbox history cache: page reads parse each history entry once, a new
// entry once more, the session keeps only the entries the ring holds, and
// ForgetSession drops everything the source kept of the session.
func TestDashboardHistoryCacheReadsEachEntryOnce(t *testing.T) {
	f := newWhiteboardFixture(t)
	src := dashboardPageSource(t, nil)
	reg := wsdashboard.Registration{ReplyID: f.replyID(), Project: f.root}
	f.update(t, `<wb:topic name="t"><wb:add class="technical"><wb:title>a question</wb:title></wb:add></wb:topic>`)
	appendEntries := func(n int) {
		t.Helper()
		entries := make([]mailboxHistoryEntry, n)
		for i := range entries {
			entries[i] = mailboxHistoryEntry{Direction: mailboxHistorySent, Counterparty: "id:peer", Body: "message body"}
		}
		if _, err := appendMailboxHistory("", f.key, entries); err != nil {
			t.Fatal(err)
		}
	}
	parsed := func() int {
		src.history.mu.Lock()
		defer src.history.mu.Unlock()
		return src.history.parsed
	}
	appendEntries(2)

	page := readPageOK(t, src, reg)
	if got := parsed(); got != 2 {
		t.Fatalf("the first page read parsed %d history entries, want 2", got)
	}
	if len(page.MailBodies) != 2 {
		t.Fatalf("mail bodies = %v", page.MailBodies)
	}
	readPageOK(t, src, reg)
	readFingerprint(t, src, reg)
	if got := parsed(); got != 2 {
		t.Fatalf("re-reads parsed %d history entries again", got-2)
	}
	appendEntries(1)
	page = readPageOK(t, src, reg)
	if got := parsed(); got != 3 || len(page.MailBodies) != 3 {
		t.Fatalf("a new entry: parsed %d (want 3), %d bodies", got, len(page.MailBodies))
	}
	want, err := readMailboxHistory("", f.key)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range want {
		if page.MailBodies[r.Name] != r.Body {
			t.Fatalf("the cached body of %s = %q, want %q", r.Name, page.MailBodies[r.Name], r.Body)
		}
	}

	appendEntries(mailboxHistoryCap + 3)
	readPageOK(t, src, reg)
	src.history.mu.Lock()
	kept := len(src.history.sessions[reg.ReplyID].byName)
	src.history.mu.Unlock()
	if kept != mailboxHistoryCap {
		t.Fatalf("the session keeps %d history entries, want the ring's %d", kept, mailboxHistoryCap)
	}

	src.ForgetSession(reg.ReplyID)
	src.history.mu.Lock()
	_, history := src.history.sessions[reg.ReplyID]
	src.history.mu.Unlock()
	src.mu.Lock()
	_, print := src.prints[reg.ReplyID]
	src.mu.Unlock()
	src.boards.mu.Lock()
	boards := len(src.boards.entries)
	src.boards.mu.Unlock()
	if history || print || boards != 0 {
		t.Fatalf("ForgetSession kept history %t, fingerprint %t, %d board renders", history, print, boards)
	}
}

// TestDashboardTopicTicketMoveInvalidatesTheBoard pins Decision 17: the
// served board's ticket resolver names every status directory it consults
// as a watched file, so a linked ticket moving between two directories the
// ticket panel never lists (idea/ to .done/) renders the board again, now
// with the moved status.
func TestDashboardTopicTicketMoveInvalidatesTheBoard(t *testing.T) {
	f := newWhiteboardFixture(t)
	src := dashboardPageSource(t, nil)
	reg := wsdashboard.Registration{ReplyID: f.replyID(), Project: f.root}
	writeTicket(t, f.root, "idea", "261101-feat-linked", "Linked")
	f.update(t, `<wb:topic name="t" tickets="261101-feat-linked"><wb:log>working</wb:log></wb:topic>`)

	page := readPageOK(t, src, reg)
	if !strings.Contains(string(page.Board.Board), `class="wbtk idea"`) {
		t.Fatalf("the board lacks the idea badge:\n%s", page.Board.Board)
	}
	for _, dir := range wsdoc.TicketStatusDirs {
		want := filepath.Join(f.root, "ai-docs", "tickets", dir)
		if !slices.Contains(page.Watch, want) {
			t.Errorf("the board does not watch %s", want)
		}
	}
	readPageOK(t, src, reg)
	before := boardRenders(src)
	if readPageOK(t, src, reg); boardRenders(src) != before {
		t.Fatal("an unchanged re-read rendered the board")
	}
	from := filepath.Join(f.root, "ai-docs", "tickets", "idea", "261101-feat-linked.md")
	to := filepath.Join(f.root, "ai-docs", "tickets", ".done", "261101-feat-linked.md")
	if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(from, to); err != nil {
		t.Fatal(err)
	}
	page = readPageOK(t, src, reg)
	if boardRenders(src) == before {
		t.Fatal("a linked ticket's move reused the memoized board")
	}
	if !strings.Contains(string(page.Board.Board), `class="wbtk done"`) {
		t.Fatalf("the re-rendered board lacks the done badge:\n%s", page.Board.Board)
	}
}
