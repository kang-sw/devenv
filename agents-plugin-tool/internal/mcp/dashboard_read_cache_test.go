package mcp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wsmailbox"
)

// ageTree sets every file under dir to an hour ago, outside the file
// caches' racy window.
func ageTree(t *testing.T, dir string) {
	t.Helper()
	at := time.Now().Add(-time.Hour)
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		return os.Chtimes(path, at, at)
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestDashboardRecordCacheServesUnchangedParts pins the source's record
// cache: re-reads of an unchanged record read no part file again, a
// rewritten part is read again and shows, and a forgotten session drops
// its cached files. The cache counters are served.
func TestDashboardRecordCacheServesUnchangedParts(t *testing.T) {
	f := newWhiteboardFixture(t)
	src := dashboardPageSource(t, nil)
	reg := wsdashboard.Registration{ReplyID: f.replyID(), Project: f.root}
	f.update(t, `<wb:topic name="t"><wb:add class="technical"><wb:title>q one</wb:title></wb:add></wb:topic>`)
	d, err := src.store.dirs()
	if err != nil {
		t.Fatal(err)
	}
	keyDir := d.keyDir(f.key)
	ageTree(t, keyDir)

	readFingerprint(t, src, reg)
	_, misses := src.records.Counts()
	first := readFingerprint(t, src, reg)
	readPageOK(t, src, reg)
	hits, after := src.records.Counts()
	if after != misses || hits == 0 {
		t.Fatalf("re-reads of an unchanged record: %d more part reads, %d hits", after-misses, hits)
	}

	f.update(t, `<wb:topic name="t"><wb:add class="technical"><wb:title>q two</wb:title></wb:add></wb:topic>`)
	if readFingerprint(t, src, reg) == first {
		t.Fatal("a rewritten board part kept the fingerprint")
	}
	if !strings.Contains(string(readPageOK(t, src, reg).Board.Board), "q two") {
		t.Fatal("the page does not show the rewritten board")
	}

	ageTree(t, keyDir)
	readFingerprint(t, src, reg)
	cachedUnder := func() bool {
		_, before := src.records.Counts()
		readFingerprint(t, src, reg)
		_, now := src.records.Counts()
		return now == before
	}
	if !cachedUnder() {
		t.Fatal("an aged record is not served from the cache")
	}
	src.ForgetSession(reg.ReplyID)
	if cachedUnder() {
		t.Fatal("a forgotten session kept its cached record files")
	}

	c := src.Counters()
	for _, kind := range []string{"record", "reply_registry", "scope_store", "source_snapshot"} {
		for _, k := range []string{kind + "_cache_hits", kind + "_cache_misses"} {
			if _, ok := c[k]; !ok {
				t.Errorf("Counters lacks %s: %v", k, c)
			}
		}
	}
}

// TestDashboardReplyRegistryKeysTheBoardByWhatItShows: a reply-registry
// rewrite that changes only another session's entry, or this session's
// LastSeen, rebuilds no board and keeps the page's mail print, while a new
// message queued for this session's reply-id, or a new contact, rebuilds
// the board (its unread count) and moves the print. The print follows only
// this session's own entry; a contact's own entry is the contacts stamp's.
func TestDashboardReplyRegistryKeysTheBoardByWhatItShows(t *testing.T) {
	hf := newHistoryFixture(t, time.Date(2026, 11, 6, 9, 0, 0, 0, time.UTC))
	if out := callToolWithKey(t, hf.serverB, 40, hf.keyB, "whiteboard.update", map[string]any{"commands": `<wb:topic name="t"><wb:add class="technical"><wb:title>q</wb:title></wb:add></wb:topic>`}); strings.Contains(out, "error") {
		t.Fatalf("whiteboard.update: %s", out)
	}
	replyB := wsmailbox.ReplyID(hf.keyB)
	mustSend(t, hf.serverB, 50, hf.keyB, wsmailbox.ReplyIDPrefix+wsmailbox.ReplyID(hf.keyA), "hello")
	recB, ok := hf.serverB.sessions.readState(hf.keyB, partAll)
	if !ok {
		t.Fatal("no record for B")
	}
	reg := wsdashboard.Registration{ReplyID: replyB, Project: recB.Root}
	src := dashboardPageSource(t, nil)
	replyPath, err := wsmailbox.ReplyRegistryPath()
	if err != nil {
		t.Fatal(err)
	}
	edit := func(fn func(st *wsmailbox.ReplyStore)) {
		t.Helper()
		if err := wsmailbox.WithReplyLock(replyPath, func(st *wsmailbox.ReplyStore) error {
			fn(st)
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	print := func() string {
		t.Helper()
		p, ok := src.MailPrint(reg)
		if !ok {
			t.Fatal("no mail print")
		}
		return p
	}
	readPageOK(t, src, reg)
	readPageOK(t, src, reg)
	last := print()
	step := func(name string, rebuilds bool, fn func(st *wsmailbox.ReplyStore)) {
		t.Helper()
		edit(fn)
		before := boardRenders(src)
		readPageOK(t, src, reg)
		if got := boardRenders(src) != before; got != rebuilds {
			t.Fatalf("%s: board rebuilt = %t, want %t", name, got, rebuilds)
		}
		next := print()
		if moved := next != last; moved != rebuilds {
			t.Fatalf("%s: mail print moved = %t, want %t", name, moved, rebuilds)
		}
		last = next
	}
	step("another session's entry", false, func(st *wsmailbox.ReplyStore) {
		st.Entries["unrelatedsession"] = wsmailbox.ReplyEntry{LastSeen: time.Now().UTC().Format(time.RFC3339)}
	})
	step("this session's LastSeen", false, func(st *wsmailbox.ReplyStore) {
		e := st.Entries[replyB]
		e.LastSeen = time.Now().Add(time.Minute).UTC().Format(time.RFC3339)
		st.Entries[replyB] = e
	})
	step("a message for this session", true, func(st *wsmailbox.ReplyStore) {
		st.Queues[replyB] = append(st.Queues[replyB], wsmailbox.Envelope{From: "x@machine", Content: "hi"})
	})
	step("a new contact", true, func(st *wsmailbox.ReplyStore) {
		e := st.Entries[replyB]
		if e.Contacts == nil {
			e.Contacts = map[string]wsmailbox.Contact{}
		}
		e.Contacts["new@machine"] = wsmailbox.Contact{LastExchange: time.Now().UTC().Format(time.RFC3339)}
		st.Entries[replyB] = e
	})
}
