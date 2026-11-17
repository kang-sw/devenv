package mcp

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
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
	step("an unread reply-id message", func() {
		if err := (wsmailbox.Paths{}).Store().WithReplyLock(func(st *wsmailbox.ReplyStore) error {
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

// TestDashboardPageServesHistoryFromStore pins the served page's mailbox
// history: page reads take each entry's body from the memory store's
// window, a new entry shows at the next read, the page shows only the
// window, and ForgetSession drops everything the source kept of the
// session.
func TestDashboardPageServesHistoryFromStore(t *testing.T) {
	f := newWhiteboardFixture(t)
	src := dashboardPageSource(t, nil)
	reg := wsdashboard.Registration{ReplyID: f.replyID(), Project: f.root}
	f.update(t, `<wb:topic name="t"><wb:add class="technical"><wb:title>a question</wb:title></wb:add></wb:topic>`)
	appendEntries := func(n int) {
		t.Helper()
		entries := make([]mailboxHistoryEntry, n)
		for i := range entries {
			entries[i] = mailboxHistoryEntry{Direction: mailboxHistorySent, Counterparty: "id:peer", Body: fmt.Sprintf("message body %d", i)}
		}
		if _, err := appendMailboxHistory("", f.key, entries); err != nil {
			t.Fatal(err)
		}
	}
	appendEntries(2)

	page := readPageOK(t, src, reg)
	if len(page.MailBodies) != 2 {
		t.Fatalf("mail bodies = %v", page.MailBodies)
	}
	appendEntries(1)
	page = readPageOK(t, src, reg)
	if len(page.MailBodies) != 3 {
		t.Fatalf("a new entry: %d bodies, want 3", len(page.MailBodies))
	}
	want, err := readMailboxHistory("", f.key)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range want {
		if page.MailBodies[r.Name] != r.Body {
			t.Fatalf("the body of %s = %q, want %q", r.Name, page.MailBodies[r.Name], r.Body)
		}
	}

	appendEntries(mailboxHistoryCap + 3)
	if page = readPageOK(t, src, reg); len(page.MailBodies) != mailboxHistoryCap {
		t.Fatalf("the page shows %d history entries, want the window's %d", len(page.MailBodies), mailboxHistoryCap)
	}

	src.ForgetSession(reg.ReplyID)
	src.mu.Lock()
	_, print := src.prints[reg.ReplyID]
	src.mu.Unlock()
	src.boards.mu.Lock()
	boards := len(src.boards.entries)
	src.boards.mu.Unlock()
	if print || boards != 0 {
		t.Fatalf("ForgetSession kept fingerprint %t, %d board renders", print, boards)
	}
}

// TestDashboardTopicTicketMoveInvalidatesTheBoard pins the preserved resolver
// watch: every status directory stays watched, so moving a linked ticket
// between directories the ticket panel never lists (idea/ to .done/) still
// renders the board again, its ticket icon's row now carrying the moved
// status. Neither render gives the topic a ticket-derived glow, and stored
// links remain queryable.
func TestDashboardTopicTicketMoveInvalidatesTheBoard(t *testing.T) {
	f := newWhiteboardFixture(t)
	src := dashboardPageSource(t, nil)
	reg := wsdashboard.Registration{ReplyID: f.replyID(), Project: f.root}
	writeTicket(t, f.root, "idea", "261101-feat-linked", "Linked")
	f.update(t, `<wb:topic name="t" tickets="261101-feat-linked"><wb:log>working</wb:log></wb:topic>`)

	checkBoard := func(board, status string) {
		t.Helper()
		for _, want := range []string{`data-topic="t"`, `<span class="tname" title="t">t</span>`, `<span class="dw wbtk">`, `data-ticket="261101-feat-linked" data-status="` + status + `"`, `data-kind="agent"`, "working"} {
			if !strings.Contains(board, want) {
				t.Errorf("the board lacks %q", want)
			}
		}
		if regexp.MustCompile(`<section class="topic[^"]*\blive\b`).MatchString(board) {
			t.Error("the board renders a ticket-derived topic glow")
		}
	}
	page := readPageOK(t, src, reg)
	checkBoard(string(page.Board.Board), "idea")
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
	// This external move has no Index signal; watched stamps revalidate
	// within one second rather than on every direct page read.
	waitForDashboardValidation(t)
	page = readPageOK(t, src, reg)
	if boardRenders(src) == before {
		t.Fatal("a linked ticket's move reused the memoized board")
	}
	checkBoard(string(page.Board.Board), "done")
	resp := f.call(t, "whiteboard.query", map[string]any{"payload": map[string]any{"topic": "t", "depth": "topics"}})
	if text := toolText(t, resp); isToolError(resp) || !strings.Contains(text, "tickets: 261101-feat-linked") {
		t.Fatalf("stored links missing from query after ticket move:\n%s", text)
	}
}
