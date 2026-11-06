package mcp

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsmailbox"
)

// historyOut is mailbox.history's JSON entry shape as a caller parses it;
// Body is a pointer so a test can tell an omitted body from an empty one.
type historyOut struct {
	Direction    string  `json:"direction"`
	Counterparty string  `json:"counterparty"`
	Time         string  `json:"time"`
	Title        string  `json:"title"`
	Body         *string `json:"body"`
}

func historyJSON(t *testing.T, s *Server, id int, key string, args map[string]any) []historyOut {
	t.Helper()
	if args == nil {
		args = map[string]any{}
	}
	args["format"] = "json"
	raw := callToolWithKey(t, s, id, key, "mailbox.history", args)
	var out []historyOut
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatalf("mailbox.history JSON: %v: %s", err, raw)
	}
	if out == nil {
		t.Fatalf("mailbox.history JSON is null, want an array: %s", raw)
	}
	return out
}

// resetMailboxHistoryClock clears the process-wide strictly-increasing stamp
// guard, so an earlier test's later pinned clock cannot push this test's
// stamps (and so its reported times) forward.
func resetMailboxHistoryClock(t *testing.T) {
	t.Helper()
	mailboxHistoryClock.Lock()
	mailboxHistoryClock.last = time.Time{}
	mailboxHistoryClock.Unlock()
}

// historyFixture is two sessions sharing one root: A owns alice@worktree,
// B has no named inbox.
type historyFixture struct {
	root             string
	serverA, serverB *Server
	keyA, keyB       string
}

func newHistoryFixture(t *testing.T, at time.Time) historyFixture {
	t.Helper()
	setupMailboxTestEnv(t)
	t.Setenv(envMailboxAuto, "")
	resetMailboxHistoryClock(t)
	setMailboxClock(t, at)
	root := t.TempDir()
	initGit(t, root)
	t.Setenv(envMailbox, "alice@worktree")
	serverA := NewServer(root, "test")
	keyA := mailboxLogin(t, serverA, 1, root)
	t.Setenv(envMailbox, "")
	serverB := NewServer(root, "test")
	keyB := mailboxLogin(t, serverB, 1, root)
	return historyFixture{root: root, serverA: serverA, serverB: serverB, keyA: keyA, keyB: keyB}
}

func mustSend(t *testing.T, s *Server, id int, key, to, content string) {
	t.Helper()
	if out := callToolWithKey(t, s, id, key, "mailbox.send", map[string]any{"to": to, "content": content}); !strings.HasPrefix(out, "sent to ") {
		t.Fatalf("mailbox.send to %s failed: %s", to, out)
	}
}

func historyEntryFiles(t *testing.T, key string) []string {
	t.Helper()
	dir, err := mailboxHistoryDir(key)
	if err != nil {
		t.Fatal(err)
	}
	names, err := mailboxHistoryEntryNames(dir)
	if err != nil {
		t.Fatal(err)
	}
	return names
}

func debugEventCount(event string) int {
	n := 0
	for _, e := range recentDebugEvents(0) {
		if e["event"] == event {
			n++
		}
	}
	return n
}

// TestMailboxHistoryRecordsSendAndRecvFromBothQueues pins the ring's
// sources: a send appends the sent message to the sender's ring with the
// normalized target as counterparty, and a recv appends what it drained
// from both the named inbox and the reply-id queue, with the sender's return
// handle as counterparty and this session's drain time, not sent_at.
func TestMailboxHistoryRecordsSendAndRecvFromBothQueues(t *testing.T) {
	t0 := time.Date(2026, 11, 6, 9, 0, 0, 0, time.UTC)
	f := newHistoryFixture(t, t0)
	replyA := wsmailbox.ReplyIDPrefix + wsmailbox.ReplyID(f.keyA)
	replyB := wsmailbox.ReplyIDPrefix + wsmailbox.ReplyID(f.keyB)

	if got := historyJSON(t, f.serverA, 2, f.keyA, nil); len(got) != 0 {
		t.Fatalf("a session that exchanged nothing has history: %#v", got)
	}

	// B -> alice@worktree (named inbox). A -> B's reply-id opens A's own
	// reply-id; B -> A's reply-id fills A's reply-id queue.
	mustSend(t, f.serverB, 3, f.keyB, "alice@worktree", "to the inbox\nsecond line")
	mustSend(t, f.serverA, 4, f.keyA, replyB, "to b")
	mustSend(t, f.serverB, 5, f.keyB, replyA, "to the reply id")

	t1 := t0.Add(10 * time.Minute)
	setMailboxClock(t, t1)
	recv := callToolWithKey(t, f.serverA, 6, f.keyA, "mailbox.recv", nil)
	if !strings.Contains(recv, "to the inbox") || !strings.Contains(recv, "to the reply id") {
		t.Fatalf("A did not drain both queues: %s", recv)
	}

	gotA := historyJSON(t, f.serverA, 7, f.keyA, map[string]any{"body": true})
	if len(gotA) != 3 {
		t.Fatalf("A history = %d entries, want 3 (one sent, two drained): %#v", len(gotA), gotA)
	}
	// Newest first: the two drained (drain order reversed), then the send.
	// Every fixture session is a top-level lead, so each stored body carries
	// the peer-lead note line in front of what the sender wrote.
	if gotA[2].Direction != "sent" || gotA[2].Counterparty != replyB || !strings.HasSuffix(*gotA[2].Body, "\nto b") || gotA[2].Time != t0.Format(time.RFC3339) {
		t.Fatalf("A's sent entry = %#v", gotA[2])
	}
	recvBodies := map[string]historyOut{}
	for _, e := range gotA[:2] {
		if e.Direction != "recv" || e.Counterparty != replyB || e.Time != t1.Format(time.RFC3339) {
			t.Fatalf("A's recv entry = %#v, want direction recv, counterparty %s, time %s (drain time)", e, replyB, t1.Format(time.RFC3339))
		}
		recvBodies[e.Title] = e
	}
	if e, ok := recvBodies["to the inbox"]; !ok || !strings.HasSuffix(*e.Body, "\nto the inbox\nsecond line") {
		t.Fatalf("named-inbox message missing from A's history: %#v", gotA)
	}
	if e, ok := recvBodies["to the reply id"]; !ok || !strings.HasSuffix(*e.Body, "\nto the reply id") {
		t.Fatalf("reply-id message missing from A's history: %#v", gotA)
	}

	gotB := historyJSON(t, f.serverB, 8, f.keyB, nil)
	if len(gotB) != 2 || gotB[0].Counterparty != replyA || gotB[1].Counterparty != "alice@worktree" || gotB[0].Direction != "sent" || gotB[1].Direction != "sent" {
		t.Fatalf("B history = %#v, want its two sends newest first", gotB)
	}
	if gotB[1].Title != "to the inbox" {
		t.Fatalf("title is not the first body line: %#v", gotB[1])
	}

	text := callToolWithKey(t, f.serverB, 9, f.keyB, "mailbox.history", nil)
	want := "[" + t0.Format(time.RFC3339) + "] sent to alice@worktree: to the inbox\n"
	if !strings.Contains(text, want) || strings.Contains(text, "second line") {
		t.Fatalf("text history = %q, want it to contain %q and no body", text, want)
	}
	textBody := callToolWithKey(t, f.serverB, 10, f.keyB, "mailbox.history", map[string]any{"body": true})
	if !strings.Contains(textBody, want) || !strings.Contains(textBody, "\nto the inbox\nsecond line\n---\n") {
		t.Fatalf("text history with body = %q", textBody)
	}
}

// TestMailboxHistoryCapOrderAndArguments pins the ring's capacity and
// order (one multi-message drain keeps queue order; only the newest 10
// survive, on disk too) and the read arguments: body, direction, limit
// (including below 1 and above the stored count), and that history
// consumes and changes nothing.
func TestMailboxHistoryCapOrderAndArguments(t *testing.T) {
	f := newHistoryFixture(t, time.Date(2026, 11, 6, 10, 0, 0, 0, time.UTC))
	for i := 0; i < 12; i++ {
		mustSend(t, f.serverB, 10+i, f.keyB, "alice@worktree", fmt.Sprintf("m%02d\nbody %02d", i, i))
	}
	recv := callToolWithKey(t, f.serverA, 30, f.keyA, "mailbox.recv", nil)
	if !strings.Contains(recv, "m00") || !strings.Contains(recv, "m11") {
		t.Fatalf("A did not drain all 12: %s", recv)
	}

	if files := historyEntryFiles(t, f.keyA); len(files) != mailboxHistoryCap {
		t.Fatalf("A ring on disk = %d entry files, want %d: %v", len(files), mailboxHistoryCap, files)
	}
	if files := historyEntryFiles(t, f.keyB); len(files) != mailboxHistoryCap {
		t.Fatalf("B ring on disk = %d entry files, want %d: %v", len(files), mailboxHistoryCap, files)
	}

	got := historyJSON(t, f.serverA, 31, f.keyA, nil)
	if len(got) != mailboxHistoryCap {
		t.Fatalf("history = %d entries, want %d", len(got), mailboxHistoryCap)
	}
	for i, e := range got {
		want := fmt.Sprintf("m%02d", 11-i)
		if e.Title != want || e.Direction != "recv" {
			t.Fatalf("history[%d] = %#v, want title %s (newest first, queue order kept)", i, e, want)
		}
		if e.Body != nil {
			t.Fatalf("body:false returned a body: %#v", e)
		}
	}

	withBody := historyJSON(t, f.serverA, 32, f.keyA, map[string]any{"body": true})
	if withBody[0].Body == nil || !strings.HasSuffix(*withBody[0].Body, "\nm11\nbody 11") {
		t.Fatalf("body:true did not return the full body: %#v", withBody[0])
	}

	if got := historyJSON(t, f.serverA, 33, f.keyA, map[string]any{"limit": 3}); len(got) != 3 || got[0].Title != "m11" || got[2].Title != "m09" {
		t.Fatalf("limit 3 = %#v", got)
	}
	for i, limit := range []any{0, -4} {
		if got := historyJSON(t, f.serverA, 34+i, f.keyA, map[string]any{"limit": limit}); len(got) != mailboxHistoryCap {
			t.Fatalf("limit %v = %d entries, want the default %d", limit, len(got), mailboxHistoryCap)
		}
	}
	if got := historyJSON(t, f.serverA, 36, f.keyA, map[string]any{"limit": 50}); len(got) != mailboxHistoryCap {
		t.Fatalf("limit above the stored count = %d entries, want every entry", len(got))
	}

	// direction: A has only recv entries; one send makes the filter visible.
	mustSend(t, f.serverA, 37, f.keyA, wsmailbox.ReplyIDPrefix+wsmailbox.ReplyID(f.keyB), "reply\nmore")
	if got := historyJSON(t, f.serverA, 38, f.keyA, map[string]any{"direction": "sent"}); len(got) != 1 || got[0].Title != "reply" {
		t.Fatalf("direction sent = %#v", got)
	}
	recvOnly := historyJSON(t, f.serverA, 39, f.keyA, map[string]any{"direction": "recv"})
	if len(recvOnly) != 9 {
		t.Fatalf("direction recv = %d entries, want 9 (the send pushed one recv out): %#v", len(recvOnly), recvOnly)
	}
	for _, e := range recvOnly {
		if e.Direction != "recv" {
			t.Fatalf("direction recv returned %#v", e)
		}
	}
	if got := historyJSON(t, f.serverA, 40, f.keyA, map[string]any{"direction": "all"}); len(got) != mailboxHistoryCap || got[0].Direction != "sent" {
		t.Fatalf("direction all = %#v", got)
	}
	if out := callToolWithKey(t, f.serverA, 41, f.keyA, "mailbox.history", map[string]any{"direction": "both"}); !strings.Contains(out, "direction must be") {
		t.Fatalf("an invalid direction was not refused: %s", out)
	}

	// Read-only: history neither drains a queue nor changes the ring.
	mustSend(t, f.serverB, 42, f.keyB, "alice@worktree", "pending")
	before := historyEntryFiles(t, f.keyA)
	historyJSON(t, f.serverA, 43, f.keyA, map[string]any{"body": true})
	callToolWithKey(t, f.serverA, 44, f.keyA, "mailbox.history", nil)
	after := historyEntryFiles(t, f.keyA)
	if strings.Join(before, ",") != strings.Join(after, ",") {
		t.Fatalf("history changed the ring: before %v after %v", before, after)
	}
	if recv := callToolWithKey(t, f.serverA, 45, f.keyA, "mailbox.recv", nil); !strings.Contains(recv, "pending") {
		t.Fatalf("history consumed queued mail: %s", recv)
	}
}

// TestMailboxHistoryMissingDirectoryReadsEmpty pins that a session with no
// history directory gets an empty ring, not an error.
func TestMailboxHistoryMissingDirectoryReadsEmpty(t *testing.T) {
	f := newHistoryFixture(t, time.Date(2026, 11, 6, 11, 0, 0, 0, time.UTC))
	dir, err := mailboxHistoryDir(f.keyB)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("history dir exists before any exchange: %v", err)
	}
	if out := callToolWithKey(t, f.serverB, 2, f.keyB, "mailbox.history", nil); out != "no mailbox history\n" {
		t.Fatalf("missing dir text = %q", out)
	}
	if got := historyJSON(t, f.serverB, 3, f.keyB, nil); len(got) != 0 {
		t.Fatalf("missing dir JSON = %#v", got)
	}
}

// TestMailboxHistoryTitle pins the one title rule: first line, a leading
// peer-lead note line skipped, empty body empty, cut at 120 runes.
func TestMailboxHistoryTitle(t *testing.T) {
	long := strings.Repeat("가", 130)
	cases := []struct{ name, body, want string }{
		{"first line", "subject\nrest", "subject"},
		{"crlf", "subject\r\nrest", "subject"},
		{"empty", "", ""},
		{"peer-lead note skipped", stampPeerLeadNote("subject\nrest", "codex"), "subject"},
		{"wsflow note skipped", "[wsflow note: from a peer lead session (not a worker), with the same authority as yours]\nhello", "hello"},
		{"note only", "[ws note: from a peer lead session (not a worker), with the same authority as yours]", ""},
		{"exactly 120 runes", strings.Repeat("a", 120), strings.Repeat("a", 120)},
		{"long line cut", long + "\nrest", strings.Repeat("가", 120) + "…"},
	}
	for _, c := range cases {
		if got := mailboxHistoryTitle(c.body); got != c.want {
			t.Errorf("%s: title = %q, want %q", c.name, got, c.want)
		}
	}
}

// TestMailboxHistoryStoresStampedBodyAndTitleSkipsNote pins that the ring
// keeps the body as delivered, peer-lead note line included, while the
// title is the sender's own first line.
func TestMailboxHistoryStoresStampedBodyAndTitleSkipsNote(t *testing.T) {
	f := newHistoryFixture(t, time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC))
	mustSend(t, f.serverB, 2, f.keyB, "alice@worktree", "hello alice\nbody")
	callToolWithKey(t, f.serverA, 3, f.keyA, "mailbox.recv", nil)
	for _, side := range []struct {
		s   *Server
		key string
	}{{f.serverB, f.keyB}, {f.serverA, f.keyA}} {
		got := historyJSON(t, side.s, 4, side.key, map[string]any{"body": true})
		if len(got) != 1 || !hasPeerLeadNote(*got[0].Body) || !strings.HasSuffix(*got[0].Body, "\nhello alice\nbody") || got[0].Title != "hello alice" {
			t.Fatalf("entry = %#v, want the stamped body and the sender's first line as title", got)
		}
	}
}

// TestMailboxHistoryFailedWriteIsLoggedNotSurfaced pins that a ring write
// failure is logged and neither the send nor the recv fails.
func TestMailboxHistoryFailedWriteIsLoggedNotSurfaced(t *testing.T) {
	f := newHistoryFixture(t, time.Date(2026, 11, 6, 13, 0, 0, 0, time.UTC))
	root, err := mailboxHistoryRoot()
	if err != nil {
		t.Fatal(err)
	}
	// A regular file where the history root belongs makes every ring write
	// fail at directory creation.
	if err := os.MkdirAll(filepath.Dir(root), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(root, []byte("not a dir"), 0o644); err != nil {
		t.Fatal(err)
	}

	before := debugEventCount("mailbox.history_write_error")
	mustSend(t, f.serverB, 2, f.keyB, "alice@worktree", "still delivered")
	if got := debugEventCount("mailbox.history_write_error"); got != before+1 {
		t.Fatalf("failed send-side ring write not logged: %d events, want %d", got, before+1)
	}
	if recv := callToolWithKey(t, f.serverA, 3, f.keyA, "mailbox.recv", nil); !strings.Contains(recv, "still delivered") {
		t.Fatalf("recv failed with a broken ring: %s", recv)
	}
	if got := debugEventCount("mailbox.history_write_error"); got != before+2 {
		t.Fatalf("failed recv-side ring write not logged: %d events, want %d", got, before+2)
	}
}

// TestMailboxHistoryFailedTrimDoesNotFailSend pins best-effort trimming: an
// oldest entry that cannot be removed (a non-empty directory) neither fails
// the send nor hides the newest 10 real entries.
func TestMailboxHistoryFailedTrimDoesNotFailSend(t *testing.T) {
	f := newHistoryFixture(t, time.Date(2026, 11, 6, 14, 0, 0, 0, time.UTC))
	dir, err := mailboxHistoryDir(f.keyB)
	if err != nil {
		t.Fatal(err)
	}
	stuck := filepath.Join(dir, "20000101T000000.000000000Z-00000000.json")
	if err := os.MkdirAll(filepath.Join(stuck, "child"), 0o755); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 11; i++ {
		mustSend(t, f.serverB, 2+i, f.keyB, "alice@worktree", fmt.Sprintf("m%02d", i))
	}
	if _, err := os.Stat(stuck); err != nil {
		t.Fatalf("the unremovable entry vanished: %v", err)
	}
	if files := historyEntryFiles(t, f.keyB); len(files) != mailboxHistoryCap+1 {
		t.Fatalf("ring on disk = %v, want the newest %d plus the stuck entry", files, mailboxHistoryCap)
	}
	got := historyJSON(t, f.serverB, 20, f.keyB, nil)
	if len(got) != mailboxHistoryCap || got[0].Title != "m10" || got[9].Title != "m01" {
		t.Fatalf("history = %#v, want m10..m01", got)
	}
}

// TestMailboxHistorySkipsUnreadableAndUnparsableEntries pins that a read
// skips an entry it cannot read or parse instead of failing.
func TestMailboxHistorySkipsUnreadableAndUnparsableEntries(t *testing.T) {
	setupMailboxTestEnv(t)
	resetMailboxHistoryClock(t)
	setMailboxClock(t, time.Date(2026, 11, 6, 15, 0, 0, 0, time.UTC))
	const key = "history-skip-key"
	if err := appendMailboxHistory(key, []mailboxHistoryEntry{
		{Direction: "sent", Counterparty: "a@machine", Body: "one"},
		{Direction: "recv", Counterparty: "a@machine", Body: "two"},
	}); err != nil {
		t.Fatal(err)
	}
	dir, err := mailboxHistoryDir(key)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "29990101T000000.000000000Z-badjson0.json"), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "29990101T000000.000000001Z-adirectr.json"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".tmp-inflight"), []byte(`{"direction":"sent","body":"partial"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := readMailboxHistory(key)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Body != "two" || got[1].Body != "one" {
		t.Fatalf("read = %#v, want the two real entries newest first", got)
	}
}

// TestMailboxHistoryConcurrentAppendsAllLand pins that two writers to one
// session's ring lose no entry.
func TestMailboxHistoryConcurrentAppendsAllLand(t *testing.T) {
	setupMailboxTestEnv(t)
	resetMailboxHistoryClock(t)
	const key = "history-concurrent-key"
	var wg sync.WaitGroup
	errs := make(chan error, 10)
	for w := 0; w < 2; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < 5; i++ {
				errs <- appendMailboxHistory(key, []mailboxHistoryEntry{{Direction: "sent", Counterparty: "x@machine", Body: fmt.Sprintf("w%d-%d", w, i)}})
			}
		}(w)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	got, err := readMailboxHistory(key)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 10 {
		t.Fatalf("concurrent appends landed %d entries, want 10", len(got))
	}
	seen := map[string]bool{}
	for _, r := range got {
		seen[r.Body] = true
	}
	if len(seen) != 10 {
		t.Fatalf("concurrent appends lost or duplicated entries: %v", seen)
	}
}

// TestMaybePruneRemovesStaleMailboxHistory pins that the key-record prune
// pass also reaps a history directory idle past keyRetentionAge and keeps a
// fresh one.
func TestMaybePruneRemovesStaleMailboxHistory(t *testing.T) {
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	resetMailboxHistoryClock(t)
	for _, key := range []string{"stale-history", "fresh-history"} {
		if err := appendMailboxHistory(key, []mailboxHistoryEntry{{Direction: "sent", Counterparty: "x@machine", Body: key}}); err != nil {
			t.Fatal(err)
		}
	}
	staleDir, _ := mailboxHistoryDir("stale-history")
	freshDir, _ := mailboxHistoryDir("fresh-history")
	backdateMtime(t, staleDir, keyRetentionAge+time.Hour)

	newSessionStore().maybePrune()

	if _, err := os.Stat(staleDir); !os.IsNotExist(err) {
		t.Fatalf("stale history dir was not pruned: stat err = %v", err)
	}
	if _, err := os.Stat(freshDir); err != nil {
		t.Fatalf("fresh history dir was pruned: %v", err)
	}
}
