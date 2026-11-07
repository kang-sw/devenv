package mcp

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wsmailbox"
)

// dashboardPageSource is a source over the test's key store whose clock
// the test sets.
func dashboardPageSource(t *testing.T, now *time.Time) *DashboardSessionSource {
	t.Helper()
	dir := t.TempDir()
	src := &DashboardSessionSource{store: newSessionStore(), stateDir: func() (string, error) { return dir, nil }}
	if now != nil {
		src.now = func() time.Time { return *now }
	}
	return src
}

func readPageOK(t *testing.T, src *DashboardSessionSource, reg wsdashboard.Registration) wsdashboard.PageContent {
	t.Helper()
	page, ok := src.ReadPage(reg)
	if !ok {
		t.Fatalf("ReadPage found no session for %+v", reg)
	}
	return page
}

func readFingerprint(t *testing.T, src *DashboardSessionSource, reg wsdashboard.Registration) string {
	t.Helper()
	data, ok := src.ReadSession(reg)
	if !ok {
		t.Fatalf("ReadSession found no session for %+v", reg)
	}
	return data.Fingerprint
}

var hrefPattern = regexp.MustCompile(`(?:href|src)="([^"]*)"`)

// TestDashboardReadPageServesTheBoard pins the served board: the full
// layout, the same fragments the static page renders when no file target is
// involved, and the served link rule otherwise (file targets and ticket rows
// as path text, never a file: URL or a relative href), with no session key
// anywhere in what reaches the browser.
func TestDashboardReadPageServesTheBoard(t *testing.T) {
	f := newWhiteboardFixture(t)
	src := dashboardPageSource(t, nil)
	reg := wsdashboard.Registration{ReplyID: f.replyID(), Project: f.root}

	if _, ok := src.ReadPage(wsdashboard.Registration{ReplyID: "nosuchsession", Project: f.root}); ok {
		t.Fatal("ReadPage read an unknown reply-id")
	}

	// No board yet: the session still reads, with no board.
	page := readPageOK(t, src, reg)
	if page.HasBoard || page.Board.Board != "" || page.Board.Panels != "" {
		t.Fatalf("a session without a board = %+v", page)
	}

	// A board without file targets: the regions are what the static full
	// page shows.
	f.call(t, "todo.add", map[string]any{"key": "step", "title": "a step"})
	f.update(t, `{"add":[{"class":"technical","topic":"t","title":"a question","rationale":"plain reason"}]}`)
	page = readPageOK(t, src, reg)
	if !page.HasBoard || page.Seq == 0 || page.Board.Board == "" || page.Board.Panels == "" {
		t.Fatalf("board page = %+v", page)
	}
	static := f.fullPage(t)
	if !strings.Contains(static, string(page.Board.Board)) {
		t.Errorf("the static page does not hold the served board:\n%.600s", page.Board.Board)
	}
	if !strings.Contains(static, string(page.Board.Panels)) {
		t.Errorf("the static page does not hold the served panels:\n%.600s", page.Board.Panels)
	}

	// File targets and tickets: path text under the served rule.
	writeTicket(t, f.root, "ready", "261101-feat-ready-one", "Ready one")
	writeTicket(t, f.root, "todo", "261102-feat-todo-one", "Todo one")
	inside := filepath.ToSlash(filepath.Join(f.root, "docs", "inside.md"))
	outside := filepath.ToSlash(filepath.Join(t.TempDir(), "out.png"))
	f.update(t, fmt.Sprintf(`{"add":[{"class":"policy","topic":"t","title":"files","options":["a","b"],"rationale":"see [the plan](./ai-docs/plan.md) and [inside](%s) and ![shot](shots/a.png) and ![away](%s)"}]}`, inside, outside))
	page = readPageOK(t, src, reg)
	html := string(page.Board.Board) + string(page.Board.Panels)
	for _, want := range []string{
		`the plan <code class="fpath">ai-docs/plan.md</code>`,
		`inside <code class="fpath">docs/inside.md</code>`,
		// An image inside the root is the image route; one outside it stays
		// path text.
		`<img src="/s/` + reg.ReplyID + `/image?path=shots%2Fa.png" alt="shot">`,
		`<span class="img-text">away <code class="fpath">` + outside + `</code></span>`,
		`title="ai-docs/tickets/ready/261101-feat-ready-one.md">Ready one</span>`,
		`title="ai-docs/tickets/todo/261102-feat-todo-one.md">Todo one</span>`,
		"1 ready, 1 todo", // the full layout's ticket count
	} {
		if !strings.Contains(html, want) {
			t.Errorf("served page missing %q", want)
		}
	}
	if strings.Contains(html, "file:") {
		t.Error("served page carries a file: URL")
	}
	imageRoute := "/s/" + reg.ReplyID + "/image?path="
	for _, m := range hrefPattern.FindAllStringSubmatch(html, -1) {
		if !strings.HasPrefix(m[1], "#") && !strings.HasPrefix(m[1], "http://") && !strings.HasPrefix(m[1], "https://") && !strings.HasPrefix(m[1], imageRoute) {
			t.Errorf("served page carries a relative reference %q", m[0])
		}
	}
	raw, err := json.Marshal(page)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), f.key) {
		t.Fatal("the page content carries the session key")
	}

	// The stored board is untouched by a page read.
	before, _ := f.s.sessions.readState(f.key)
	readPageOK(t, src, reg)
	after, _ := f.s.sessions.readState(f.key)
	b1, _ := json.Marshal(before.Whiteboard)
	b2, _ := json.Marshal(after.Whiteboard)
	if string(b1) != string(b2) {
		t.Fatalf("ReadPage changed the stored board:\n%s\n%s", b1, b2)
	}
}

// TestDashboardServedFileText pins the path text rule for each path form.
func TestDashboardServedFileText(t *testing.T) {
	root := t.TempDir()
	text := servedFileText(root)
	outside := filepath.Join(filepath.Dir(root), "elsewhere", "x.md")
	for in, want := range map[string]string{
		"ai-docs/plan.md":                   "ai-docs/plan.md",
		"./a/../b.md":                       "b.md",
		filepath.Join(root, "docs", "c.md"): "docs/c.md",
		outside:                             filepath.ToSlash(outside),
	} {
		if got := text(filepath.ToSlash(in)); got != want {
			t.Errorf("servedFileText(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestDashboardReadPageContacts pins contact resolution: an id: contact and
// a named contact held by the same session resolve to one reply-id, a
// rebind of the named address moves it, and liveness follows the reply
// entry's LastSeen within 10 minutes (id:) or mailboxPresenceLive (named).
func TestDashboardReadPageContacts(t *testing.T) {
	t0 := time.Date(2026, 11, 6, 9, 0, 0, 0, time.UTC)
	hf := newHistoryFixture(t, t0)
	replyA := wsmailbox.ReplyID(hf.keyA)
	mustSend(t, hf.serverB, 50, hf.keyB, wsmailbox.ReplyIDPrefix+replyA, "to the reply id")
	mustSend(t, hf.serverB, 51, hf.keyB, "alice@worktree", "to the inbox")
	recB, ok := hf.serverB.sessions.readState(hf.keyB)
	if !ok {
		t.Fatal("no record for B")
	}
	reg := wsdashboard.Registration{ReplyID: wsmailbox.ReplyID(hf.keyB), Project: recB.Root}

	// Pin A's reply entry and alice's presence to t0, alice held by A.
	replyPath, err := wsmailbox.ReplyRegistryPath()
	if err != nil {
		t.Fatal(err)
	}
	if err := wsmailbox.WithReplyLock(replyPath, func(st *wsmailbox.ReplyStore) error {
		entry := st.Entries[replyA]
		entry.LastSeen = t0.Format(time.RFC3339)
		st.Entries[replyA] = entry
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	storePath, err := wsmailbox.PathForScope(wsmailbox.ScopeWorktree, recB.Root)
	if err != nil {
		t.Fatal(err)
	}
	setPresence := func(owner string) wsmailbox.Presence {
		t.Helper()
		var p wsmailbox.Presence
		if err := wsmailbox.WithLock(storePath, func(st *wsmailbox.StoreFile) error {
			p = st.Presence["alice"]
			p.Owner, p.PID, p.LastSeen = owner, os.Getpid(), t0.Format(time.RFC3339)
			st.Presence["alice"] = p
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		return p
	}
	presence := setPresence(hf.keyA)

	now := t0.Add(5 * time.Minute)
	src := dashboardPageSource(t, &now)
	byAddress := func() map[string]wsdashboard.Contact {
		t.Helper()
		out := map[string]wsdashboard.Contact{}
		for _, c := range readPageOK(t, src, reg).Contacts {
			out[c.Address] = c
		}
		return out
	}
	contacts := byAddress()
	idC, named := contacts[wsmailbox.ReplyIDPrefix+replyA], contacts["alice@worktree"]
	if len(contacts) != 2 || idC.ReplyID != replyA || named.ReplyID != replyA {
		t.Fatalf("contacts = %+v", contacts)
	}
	if !idC.LastExchange.Equal(t0) || !named.LastExchange.Equal(t0) {
		t.Fatalf("last exchange = %v, %v", idC.LastExchange, named.LastExchange)
	}
	if !idC.PresenceLive || named.PresenceLive != mailboxPresenceLive(presence, now) || !named.PresenceLive {
		t.Fatalf("liveness at +5m: id %v, named %v", idC.PresenceLive, named.PresenceLive)
	}

	now = t0.Add(11 * time.Minute)
	contacts = byAddress()
	if contacts[wsmailbox.ReplyIDPrefix+replyA].PresenceLive || contacts["alice@worktree"].PresenceLive {
		t.Fatalf("liveness at +11m: %+v", contacts)
	}

	// A rebind moves the named address to its new holder on the next read.
	setPresence("rebound-session-key")
	if got := byAddress()["alice@worktree"].ReplyID; got != wsmailbox.ReplyID("rebound-session-key") {
		t.Fatalf("named contact after rebind resolves to %q", got)
	}
	raw, _ := json.Marshal(readPageOK(t, src, reg))
	for _, key := range []string{hf.keyA, hf.keyB, "rebound-session-key"} {
		if strings.Contains(string(raw), key) {
			t.Fatalf("the page content carries a session key: %s", raw)
		}
	}
}

// TestDashboardReadPageMailBodies pins the mail bodies: a mail line's
// history key maps to the full body of that history entry.
func TestDashboardReadPageMailBodies(t *testing.T) {
	f := newLiveFixture(t, true)
	resetMailboxHistoryClock(t)
	mustSend(t, f.server, 3, f.keyA, wsmailbox.ReplyIDPrefix+f.replyB, "Status update\nbody line")
	sent := readLiveRing(t, f.sessionDir(f.replyA))
	if len(sent) != 1 || sent[0].HistoryKey == "" {
		t.Fatalf("sender ring = %+v", sent)
	}
	reg, ok := readRegistration(t, f.stateDir, f.keyA)
	if !ok {
		t.Fatal("no registration")
	}
	src := &DashboardSessionSource{store: newSessionStore(), stateDir: func() (string, error) { return f.stateDir, nil }}
	page := readPageOK(t, src, reg)
	if body := page.MailBodies[sent[0].HistoryKey]; !strings.Contains(body, "Status update\nbody line") {
		t.Fatalf("body for %s = %q (bodies %v)", sent[0].HistoryKey, body, page.MailBodies)
	}
	if page.HasBoard {
		t.Fatal("a session without a board reported one")
	}
}

// TestDashboardFingerprintCoversPageStores pins the widened fingerprint: a
// ticket file, a mailbox history entry, or a new contact changes it with
// the session record unchanged, while the clock alone (a contact's
// liveness ageing out) does not.
func TestDashboardFingerprintCoversPageStores(t *testing.T) {
	f := newWhiteboardFixture(t)
	now := time.Now()
	src := dashboardPageSource(t, &now)
	reg := wsdashboard.Registration{ReplyID: f.replyID(), Project: f.root}
	f.update(t, `{"add":[{"class":"technical","topic":"t","title":"a question"}]}`)

	fp := readFingerprint(t, src, reg)
	if again := readFingerprint(t, src, reg); again != fp {
		t.Fatal("an unchanged session changed its fingerprint")
	}
	step := func(what string, change func()) {
		t.Helper()
		change()
		next := readFingerprint(t, src, reg)
		if next == fp {
			t.Fatalf("%s kept the fingerprint", what)
		}
		fp = next
	}
	step("a ticket file", func() { writeTicket(t, f.root, "ready", "261103-feat-new", "New") })
	step("a mailbox history entry", func() {
		if _, err := appendMailboxHistory(f.key, []mailboxHistoryEntry{{Direction: mailboxHistorySent, Counterparty: "id:peer", Body: "hi"}}); err != nil {
			t.Fatal(err)
		}
	})
	replyPath, err := wsmailbox.ReplyRegistryPath()
	if err != nil {
		t.Fatal(err)
	}
	step("a contact", func() {
		if err := wsmailbox.WithReplyLock(replyPath, func(st *wsmailbox.ReplyStore) error {
			st.Entries["peerpeer"] = wsmailbox.ReplyEntry{LastSeen: now.UTC().Format(time.RFC3339)}
			entry := st.Entries[f.replyID()]
			entry.RecordContact(wsmailbox.ReplyIDPrefix+"peerpeer", wsmailbox.Contact{LastExchange: now.UTC().Format(time.RFC3339)})
			st.Entries[f.replyID()] = entry
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	})
	live := readPageOK(t, src, reg).Contacts
	now = now.Add(time.Hour)
	if aged := readPageOK(t, src, reg).Contacts; len(live) != 1 || !live[0].PresenceLive || aged[0].PresenceLive {
		t.Fatalf("contact liveness did not age: %+v -> %+v", live, aged)
	}
	if readFingerprint(t, src, reg) != fp {
		t.Fatal("a contact's liveness ageing changed the fingerprint")
	}
}

// TestDashboardPointerLineFollowsTheLastNudge pins whiteboard.update's full:
// line: the session's page URL once a nudge for it was answered 204, the
// file path before that, after a failed nudge, and for a key-off session;
// the call never fails either way.
func TestDashboardPointerLineFollowsTheLastNudge(t *testing.T) {
	dir := dashboardSessionEnv(t)
	startFakeDaemon(t, dir, http.StatusNoContent, false)
	ep, err := wsdashboard.ReadEndpoint(dir)
	if err != nil {
		t.Fatal(err)
	}
	root := dashboardGitRoot(t)
	s, key, _ := dashboardSessionServer(t, root, true)
	replyID := wsmailbox.ReplyID(key)
	path := "ai-docs/whiteboards/" + replyID + ".html"
	update := func(s *Server, key string) string {
		t.Helper()
		resp := dashCall(t, s, "whiteboard.update", map[string]any{"session_key": key, "payload": map[string]any{}})
		if isToolError(resp) {
			t.Fatalf("whiteboard.update failed: %s", resp)
		}
		return resultLine(t, toolText(t, resp), "full: ")
	}
	served := func() bool { _, ok := s.dashboardPageURL(key); return ok }

	dashCall(t, s, "todo.list", map[string]any{"session_key": key}) // registers and nudges
	waitFor(t, "the registration nudge's answer", served)
	if got, want := update(s, key), fmt.Sprintf("http://127.0.0.1:%d/s/%s", ep.Port, replyID); got != want {
		t.Fatalf("full: with the daemon serving = %q, want %q", got, want)
	}

	// A daemon that answers 500: the next nudge clears the URL.
	startFakeDaemon(t, dir, http.StatusInternalServerError, false)
	dashCall(t, s, "todo.add", map[string]any{"session_key": key, "key": "k", "title": "x"})
	waitFor(t, "the failed nudge", func() bool { return !served() })
	if got := update(s, key); got != path {
		t.Fatalf("full: after a failed nudge = %q, want %q", got, path)
	}

	// Served again, then no daemon at all.
	startFakeDaemon(t, dir, http.StatusNoContent, false)
	dashCall(t, s, "todo.add", map[string]any{"session_key": key, "key": "k2", "title": "y"})
	waitFor(t, "the nudge to the new daemon", served)
	if err := os.Remove(filepath.Join(dir, "endpoint.json")); err != nil {
		t.Fatal(err)
	}
	dashCall(t, s, "todo.add", map[string]any{"session_key": key, "key": "k3", "title": "z"})
	waitFor(t, "the nudge with no daemon", func() bool { return !served() })
	if got := update(s, key); got != path {
		t.Fatalf("full: with no daemon = %q, want %q", got, path)
	}

	// A key-off session never names the URL, whatever the daemon answers.
	startFakeDaemon(t, dir, http.StatusNoContent, false)
	sOff, keyOff, _ := dashboardSessionServer(t, root, false)
	dashCall(t, sOff, "todo.list", map[string]any{"session_key": keyOff})
	if got := update(sOff, keyOff); got != "ai-docs/whiteboards/"+wsmailbox.ReplyID(keyOff)+".html" {
		t.Fatalf("full: of a key-off session = %q", got)
	}
}
