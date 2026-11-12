package wsdashboard

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeSource serves SessionData by reply-id and counts reads.
type fakeSource struct {
	mu    sync.Mutex
	data  map[string]SessionData
	gone  map[string]bool
	reads map[string]int
}

func newFakeSource() *fakeSource {
	return &fakeSource{data: map[string]SessionData{}, gone: map[string]bool{}, reads: map[string]int{}}
}

func (f *fakeSource) ReadSession(reg Registration) (SessionData, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reads[reg.ReplyID]++
	if f.gone[reg.ReplyID] {
		return SessionData{}, false
	}
	return f.data[reg.ReplyID], true
}

func (f *fakeSource) set(id string, data SessionData) {
	f.mu.Lock()
	f.data[id] = data
	f.mu.Unlock()
}

func (f *fakeSource) readCounts() map[string]int {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := map[string]int{}
	for k, v := range f.reads {
		out[k] = v
	}
	return out
}

var testNow = time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)

func register(t *testing.T, dir, replyID, project string, beat time.Time) Registration {
	t.Helper()
	reg := Registration{ReplyID: replyID, Project: project, Harness: "claude", RegisteredAt: stampFor(beat), HeartbeatAt: stampFor(beat)}
	if err := WriteRegistration(dir, reg); err != nil {
		t.Fatalf("WriteRegistration: %v", err)
	}
	reg.SchemaVersion = registrationSchemaVersion
	return reg
}

func stampFor(t time.Time) string { return t.UTC().Format(time.RFC3339) }

func TestRegistrationRoundTripIgnoresUnknownFields(t *testing.T) {
	dir := t.TempDir()
	project := t.TempDir()
	want := register(t, dir, "alphabravo", project, testNow)
	got, err := ReadRegistration(dir, "alphabravo")
	if err != nil || got != want {
		t.Fatalf("ReadRegistration = %+v, %v; want %+v", got, err, want)
	}

	// A newer writer's extra fields are ignored and absent ones read empty.
	sessionDir := SessionDir(dir, "charlie")
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := `{"schema_version": 9, "reply_id": "charlie", "project": "/p", "future": {"x": 1}, "heartbeat_at": "2026-11-06T12:00:00Z"}`
	if err := os.WriteFile(filepath.Join(sessionDir, registrationFileName), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err = ReadRegistration(dir, "charlie")
	if err != nil {
		t.Fatal(err)
	}
	if got.Project != "/p" || got.Harness != "" || got.EndedAt != "" || got.HeartbeatAt != "2026-11-06T12:00:00Z" {
		t.Fatalf("registration with unknown fields read as %+v", got)
	}
	regs, err := ReadRegistrations(dir)
	if err != nil || len(regs) != 2 {
		t.Fatalf("ReadRegistrations = %+v, %v", regs, err)
	}

	// A registration naming another reply-id than its directory is refused.
	if err := os.WriteFile(filepath.Join(sessionDir, registrationFileName), []byte(`{"reply_id": "other", "project": "/p"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadRegistration(dir, "charlie"); err == nil {
		t.Fatal("mismatched reply-id was accepted")
	}
	if err := WriteRegistration(dir, Registration{ReplyID: "../escape", Project: "/p"}); err == nil {
		t.Fatal("path-like reply-id was accepted")
	}
}

func TestRegistrationLiveRule(t *testing.T) {
	cases := []struct {
		name string
		reg  Registration
		live bool
	}{
		{"fresh heartbeat", Registration{HeartbeatAt: stampFor(testNow.Add(-time.Minute))}, true},
		{"heartbeat at the window edge", Registration{HeartbeatAt: stampFor(testNow.Add(-LiveWindow))}, true},
		{"stale heartbeat", Registration{HeartbeatAt: stampFor(testNow.Add(-LiveWindow - time.Second))}, false},
		{"ended", Registration{HeartbeatAt: stampFor(testNow), EndedAt: stampFor(testNow)}, false},
		{"no heartbeat", Registration{RegisteredAt: stampFor(testNow)}, false},
		{"unparsable heartbeat", Registration{HeartbeatAt: "yesterday"}, false},
	}
	for _, tc := range cases {
		if got := tc.reg.Live(testNow); got != tc.live {
			t.Errorf("%s: Live = %v, want %v", tc.name, got, tc.live)
		}
	}
}

func TestPruneRegistrationsDropsOnlyExpiredSessions(t *testing.T) {
	dir := t.TempDir()
	register(t, dir, "fresh", "/p", testNow.Add(-time.Hour))
	register(t, dir, "expired", "/p", testNow.Add(-RegistryRetention-time.Hour))
	ended := Registration{ReplyID: "endedrecent", Project: "/p", HeartbeatAt: stampFor(testNow.Add(-RegistryRetention - time.Hour)), EndedAt: stampFor(testNow.Add(-time.Hour))}
	if err := WriteRegistration(dir, ended); err != nil {
		t.Fatal(err)
	}
	// A directory without a registration (another store only) ages by its
	// modification time.
	orphan := SessionDir(dir, "orphan")
	if err := os.MkdirAll(orphan, 0o755); err != nil {
		t.Fatal(err)
	}
	old := testNow.Add(-RegistryRetention - time.Hour)
	if err := os.Chtimes(orphan, old, old); err != nil {
		t.Fatal(err)
	}
	young := SessionDir(dir, "younglorphan")
	if err := os.MkdirAll(young, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(young, testNow, testNow); err != nil {
		t.Fatal(err)
	}

	if n := PruneRegistrations(dir, testNow, RegistryRetention); n != 2 {
		t.Fatalf("pruned %d, want 2", n)
	}
	for id, want := range map[string]bool{"fresh": true, "expired": false, "endedrecent": true, "orphan": false, "younglorphan": true} {
		_, err := os.Stat(SessionDir(dir, id))
		if exists := err == nil; exists != want {
			t.Errorf("%s exists = %v, want %v", id, exists, want)
		}
	}
}

// A lead session the runner serves no more (its record closed, or handed to
// a terminal) is pruned once its row is past retention, as any ended
// session; an open lead, a retired one whose row is recent, and every other
// agent record (the tracker's support agent keeps the runner's lifetime,
// closed or not) stay.
func TestPruneRegistrationsAgesRetiredLeadsByTheirRow(t *testing.T) {
	dir := t.TempDir()
	old := testNow.Add(-RegistryRetention - time.Hour)
	recent := testNow.Add(-time.Hour)
	record := func(id, raw string) {
		t.Helper()
		sessionDir := SessionDir(dir, id)
		if err := os.MkdirAll(sessionDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(sessionDir, AgentRecordFileName), []byte(raw), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	row := func(id string, at time.Time, edit func(reg *Registration)) {
		t.Helper()
		reg := Registration{ReplyID: id, Project: "/p", Harness: "claude", RegisteredAt: stampFor(at), HeartbeatAt: stampFor(at)}
		edit(&reg)
		if err := WriteRegistration(dir, reg); err != nil {
			t.Fatal(err)
		}
	}
	hosted := func(reg *Registration) { reg.Hosted = true }
	closedRow := func(reg *Registration) { reg.Hosted, reg.EndedAt = true, reg.HeartbeatAt }
	terminal := func(reg *Registration) { reg.EndedAt = reg.HeartbeatAt }
	agent := func(reg *Registration) { reg.Agent = true }

	row("closedold", old, closedRow)
	record("closedold", `{"lead":true,"closed_at":"2026-10-01T00:00:00Z"}`)
	row("handedold", old, terminal)
	record("handedold", `{"lead":true,"handed_off_at":"2026-10-01T00:00:00Z"}`)
	row("closedrecent", recent, closedRow)
	record("closedrecent", `{"lead":true,"closed_at":"2026-11-06T11:00:00Z"}`)
	// Handed off long ago, and the terminal still ran it an hour ago.
	row("handedrecent", recent, terminal)
	record("handedrecent", `{"lead":true,"handed_off_at":"2026-10-01T00:00:00Z"}`)
	row("openold", old, hosted)
	record("openold", `{"lead":true}`)
	row("supportold", old, agent)
	record("supportold", `{"closed_at":"2026-10-01T00:00:00Z"}`)
	// A closed agent conversation whose row lost its flag is still no lead.
	row("agentnoflag", old, terminal)
	record("agentnoflag", `{"closed_at":"2026-10-01T00:00:00Z"}`)
	row("unreadable", old, closedRow)
	record("unreadable", `{"lead":`)
	// A retired lead with no row to age it by.
	record("norow", `{"lead":true,"closed_at":"2026-10-01T00:00:00Z"}`)
	if err := os.Chtimes(SessionDir(dir, "norow"), old, old); err != nil {
		t.Fatal(err)
	}

	// A pi session the daemon started and handed to a terminal: the
	// terminal's conversation file is in the directory. One taken over from
	// a terminal runs on a file elsewhere, and a closed one has no terminal.
	row("handedpi", old, terminal)
	record("handedpi", `{"lead":true,"backend":"pi","handed_off_at":"2026-10-01T00:00:00Z"}`)
	row("handedpiback", old, terminal)
	record("handedpiback", `{"lead":true,"backend":"pi","session_file":"`+filepath.Join(SessionDir(dir, "handedpiback"), "pi", "session.jsonl")+`","handed_off_at":"2026-10-01T00:00:00Z"}`)
	row("handedpiown", old, terminal)
	record("handedpiown", `{"lead":true,"backend":"pi","session_file":"/home/u/.pi/s.jsonl","handed_off_at":"2026-10-01T00:00:00Z"}`)
	row("closedpi", old, closedRow)
	record("closedpi", `{"lead":true,"backend":"pi","closed_at":"2026-10-01T00:00:00Z"}`)

	if n := PruneRegistrations(dir, testNow, RegistryRetention); n != 4 {
		t.Errorf("pruned %d, want 4", n)
	}
	for id, want := range map[string]bool{
		"closedold": false, "handedold": false, "handedpiown": false, "closedpi": false, "handedpi": true, "handedpiback": true,
		"closedrecent": true, "handedrecent": true, "openold": true, "supportold": true, "agentnoflag": true, "unreadable": true, "norow": true,
	} {
		_, err := os.Stat(SessionDir(dir, id))
		if exists := err == nil; exists != want {
			t.Errorf("%s exists = %v, want %v", id, exists, want)
		}
	}
}

// An agent conversation outlives the prune window while the daemon is down:
// its directory (agent.json, held sends, the backend session) belongs to the
// runner, so prune spares it by registration flag or by its record file.
func TestPruneRegistrationsSparesAgentConversations(t *testing.T) {
	dir := t.TempDir()
	old := testNow.Add(-RegistryRetention - time.Hour)
	flagged := Registration{ReplyID: "agentreg", Project: "/p", HeartbeatAt: stampFor(old), Agent: true}
	if err := WriteRegistration(dir, flagged); err != nil {
		t.Fatal(err)
	}
	// A record file without a readable registration still marks the
	// directory as the runner's.
	recordOnly := SessionDir(dir, "agentrecord")
	if err := os.MkdirAll(recordOnly, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(recordOnly, AgentRecordFileName), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(recordOnly, old, old); err != nil {
		t.Fatal(err)
	}
	register(t, dir, "leadexpired", "/p", old)

	if n := PruneRegistrations(dir, testNow, RegistryRetention); n != 1 {
		t.Fatalf("pruned %d, want 1", n)
	}
	for id, want := range map[string]bool{"agentreg": true, "agentrecord": true, "leadexpired": false} {
		_, err := os.Stat(SessionDir(dir, id))
		if exists := err == nil; exists != want {
			t.Errorf("%s exists = %v, want %v", id, exists, want)
		}
	}
}

func newTestIndex(t *testing.T, dir string, src SessionSource, now *time.Time) (*Index, *[]string) {
	t.Helper()
	var mu sync.Mutex
	changes := &[]string{}
	idx := NewIndex(IndexOptions{
		Dir:    dir,
		Source: src,
		Now:    func() time.Time { return *now },
		OnChange: func(id string) {
			mu.Lock()
			*changes = append(*changes, id)
			mu.Unlock()
		},
	})
	return idx, changes
}

func TestIndexRefreshTracksTheRegistry(t *testing.T) {
	dir := t.TempDir()
	projectA, projectB := t.TempDir(), t.TempDir()
	now := testNow
	src := newFakeSource()
	src.set("alpha", SessionData{HasBoard: true, Fingerprint: "a1"})
	register(t, dir, "alpha", projectA, now)
	register(t, dir, "bravo", projectB, now.Add(-time.Hour))
	idx, changes := newTestIndex(t, dir, src, &now)

	idx.Refresh()
	sessions := idx.Sessions()
	if len(sessions) != 2 {
		t.Fatalf("sessions = %+v", sessions)
	}
	alpha, _ := idx.Session("alpha")
	bravo, _ := idx.Session("bravo")
	if !alpha.Live || !alpha.HasBoard || alpha.Project != projectA || alpha.Rev != 1 {
		t.Fatalf("alpha = %+v", alpha)
	}
	// A session with no board is listed all the same; its stale heartbeat
	// reads as ended.
	if bravo.Live || bravo.HasBoard {
		t.Fatalf("bravo = %+v", bravo)
	}
	if len(*changes) != 2 {
		t.Fatalf("changes after first refresh = %v", *changes)
	}

	// A refresh that finds nothing new changes nothing.
	*changes = nil
	idx.Refresh()
	if len(*changes) != 0 {
		t.Fatalf("idle refresh reported changes %v", *changes)
	}

	// Liveness follows the clock: once alpha's heartbeat ages out, the next
	// refresh reports it changed.
	now = now.Add(LiveWindow + time.Minute)
	if s, _ := idx.Session("alpha"); s.Live {
		t.Fatal("alpha still live after its heartbeat aged out")
	}
	idx.Refresh()
	if s, _ := idx.Session("alpha"); s.Rev != 2 || len(*changes) != 1 {
		t.Fatalf("alpha after aging = %+v, changes %v", s, *changes)
	}

	// Stale entries leave the index: a removed registration, a removed
	// project root, and a session whose record is gone.
	register(t, dir, "charlie", t.TempDir(), now)
	idx.Refresh()
	if _, ok := idx.Session("charlie"); !ok {
		t.Fatal("charlie not indexed")
	}
	if err := RemoveRegistration(dir, "alpha"); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(projectB); err != nil {
		t.Fatal(err)
	}
	src.mu.Lock()
	src.gone["charlie"] = true
	src.mu.Unlock()
	idx.Refresh()
	if got := idx.Sessions(); len(got) != 0 {
		t.Fatalf("stale sessions kept: %+v", got)
	}
}

func TestNudgeRereadsExactlyOneSession(t *testing.T) {
	dir := t.TempDir()
	project := t.TempDir()
	now := testNow
	src := newFakeSource()
	register(t, dir, "alpha", project, now)
	register(t, dir, "bravo", project, now)
	idx, changes := newTestIndex(t, dir, src, &now)
	idx.Refresh()
	before := src.readCounts()
	*changes = nil

	src.set("alpha", SessionData{HasBoard: true, Fingerprint: "a2"})
	if !idx.Nudge("alpha", project+string(filepath.Separator)) {
		t.Fatal("nudge for a registered session was ignored")
	}
	idx.Settle()
	after := src.readCounts()
	if after["alpha"] != before["alpha"]+1 || after["bravo"] != before["bravo"] {
		t.Fatalf("reads before %v after %v; want exactly one alpha re-read", before, after)
	}
	if s, _ := idx.Session("alpha"); !s.HasBoard || s.Rev != 2 {
		t.Fatalf("alpha after nudge = %+v", s)
	}
	if len(*changes) != 1 || (*changes)[0] != "alpha" {
		t.Fatalf("changes = %v", *changes)
	}

	// A nudge for a session registered since the last poll indexes it.
	register(t, dir, "delta", project, now)
	if !idx.Nudge("delta", project) {
		t.Fatal("nudge for a new registration was ignored")
	}
	idx.Settle()
	if _, ok := idx.Session("delta"); !ok {
		t.Fatal("delta not indexed after its nudge")
	}

	// Malformed and unknown nudges read nothing and change nothing.
	*changes = nil
	before = src.readCounts()
	for _, n := range []struct{ id, project string }{
		{"", project},
		{"../alpha", project},
		{"alpha", ""},
		{"alpha", t.TempDir()},
		{"unknown", project},
	} {
		if idx.Nudge(n.id, n.project) {
			t.Errorf("nudge %+v was accepted", n)
		}
	}
	idx.Settle()
	if after := src.readCounts(); len(after) != len(before) || after["alpha"] != before["alpha"] {
		t.Fatalf("ignored nudges read sessions: before %v after %v", before, after)
	}
	if len(*changes) != 0 {
		t.Fatalf("ignored nudges changed %v", *changes)
	}
}

// startIndexedDaemon starts a daemon over src; each option adjusts its
// options before the start.
func startIndexedDaemon(t *testing.T, dir string, src SessionSource, poll time.Duration, options ...func(*DaemonOptions)) *Daemon {
	t.Helper()
	opts := DaemonOptions{Dir: dir, Port: 0, Version: "1.0.0", LockWait: 200 * time.Millisecond, Sessions: src, PollInterval: poll}
	for _, option := range options {
		option(&opts)
	}
	d, err := StartDaemon(opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(d.Close)
	return d
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestPollingBackstopPicksUpAWriteWithoutANudge(t *testing.T) {
	dir := t.TempDir()
	project := t.TempDir()
	src := newFakeSource()
	register(t, dir, "alpha", project, time.Now())
	d := startIndexedDaemon(t, dir, src, 20*time.Millisecond)
	waitFor(t, "first poll", func() bool { _, ok := d.Index().Session("alpha"); return ok })

	src.set("alpha", SessionData{HasBoard: true, Fingerprint: "changed"})
	waitFor(t, "the poll to see the board", func() bool { s, _ := d.Index().Session("alpha"); return s.HasBoard })

	register(t, dir, "bravo", project, time.Now())
	waitFor(t, "the poll to see a new registration", func() bool { _, ok := d.Index().Session("bravo"); return ok })
}

func TestNudgeRoute(t *testing.T) {
	dir := t.TempDir()
	project := t.TempDir()
	src := newFakeSource()
	register(t, dir, "alpha", project, time.Now())
	d := startIndexedDaemon(t, dir, src, time.Hour)
	waitFor(t, "first poll", func() bool { _, ok := d.Index().Session("alpha"); return ok })
	ep := d.Endpoint()
	client := testClient()

	src.set("alpha", SessionData{HasBoard: true, Fingerprint: "x"})
	if err := SendNudge(context.Background(), client, ep, NudgeBody{ReplyID: "alpha", Project: project}); err != nil {
		t.Fatalf("SendNudge: %v", err)
	}
	d.Index().Settle()
	if s, _ := d.Index().Session("alpha"); !s.HasBoard {
		t.Fatalf("nudge did not re-read alpha: %+v", s)
	}
	if err := SendNudge(context.Background(), client, ep, NudgeBody{ReplyID: "nobody", Project: project}); err == nil || !strings.Contains(err.Error(), "404") {
		t.Fatalf("unknown session nudge err = %v, want 404", err)
	}

	post := func(host, body string) int {
		req := httptest.NewRequest(http.MethodPost, "/nudge", bytes.NewBufferString(body))
		req.Host = host
		rec := httptest.NewRecorder()
		d.Handler().ServeHTTP(rec, req)
		return rec.Code
	}
	projectJSON, _ := json.Marshal(project) // a Windows path's backslashes need escaping
	if code := post("127.0.0.1", `{"reply_id": "alpha", "project": `+string(projectJSON)+`, "extra": [1]}`); code != http.StatusNoContent {
		t.Fatalf("nudge with an unknown field = %d, want 204", code)
	}
	if code := post("127.0.0.1", `{not json`); code != http.StatusBadRequest {
		t.Fatalf("malformed nudge = %d, want 400", code)
	}
	if code := post("evil.example", `{"reply_id": "alpha", "project": `+string(projectJSON)+`}`); code != http.StatusForbidden {
		t.Fatalf("non-loopback Host nudge = %d, want 403", code)
	}
	req := httptest.NewRequest(http.MethodGet, "/nudge", nil)
	req.Host = "localhost"
	rec := httptest.NewRecorder()
	d.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET /nudge = %d, want 405", rec.Code)
	}

	// A daemon without a session source has no index: every nudge is 404.
	bare := startTestDaemon(t, t.TempDir(), "1.0.0")
	if bare.Index() != nil {
		t.Fatal("daemon without a source has an index")
	}
	if err := SendNudge(context.Background(), client, bare.Endpoint(), NudgeBody{ReplyID: "alpha", Project: project}); err == nil {
		t.Fatal("nudge to a daemon without an index succeeded")
	}
}

// TestIndexSnapshotAgeIsNotAChange pins the snapshot age: a re-read whose
// only change is the snapshot's updatedAt (a heartbeat) updates the age the
// index reports without raising Rev or reporting a change.
func TestIndexSnapshotAgeIsNotAChange(t *testing.T) {
	dir := t.TempDir()
	now := testNow
	src := newFakeSource()
	src.set("alpha", SessionData{Fingerprint: "a1", SnapshotUpdatedAt: now})
	register(t, dir, "alpha", t.TempDir(), now)
	idx, changes := newTestIndex(t, dir, src, &now)
	idx.Refresh()

	*changes = nil
	later := now.Add(30 * time.Second)
	src.set("alpha", SessionData{Fingerprint: "a1", SnapshotUpdatedAt: later})
	idx.Refresh()
	if s, _ := idx.Session("alpha"); s.Rev != 1 || !s.SnapshotUpdatedAt.Equal(later) || len(*changes) != 0 {
		t.Fatalf("after a heartbeat: %+v, changes %v", s, *changes)
	}

	src.set("alpha", SessionData{Fingerprint: "a2", SnapshotUpdatedAt: later})
	idx.Refresh()
	if s, _ := idx.Session("alpha"); s.Rev != 2 || len(*changes) != 1 {
		t.Fatalf("after a snapshot change: %+v, changes %v", s, *changes)
	}
}

// TestRefreshSkipsAnAppliedEndedRegistration pins the refresh skip: an
// ended registration is re-read by the first refresh after it ended and
// then skipped while unchanged; a nudge still re-reads it, a resumed
// session is re-read again, a session whose project root is gone is still
// dropped, and one that only stopped heartbeating is never skipped.
func TestRefreshSkipsAnAppliedEndedRegistration(t *testing.T) {
	dir := t.TempDir()
	project := t.TempDir()
	now := testNow
	src := newFakeSource()
	alpha := register(t, dir, "alpha", project, now)
	register(t, dir, "bravo", t.TempDir(), now.Add(-time.Hour)) // stale heartbeat, never ended
	idx, _ := newTestIndex(t, dir, src, &now)
	idx.Refresh()
	reads := func(id string) int { return src.readCounts()[id] }

	end := func(reg Registration) Registration {
		t.Helper()
		reg.EndedAt = stampFor(now)
		if err := WriteRegistration(dir, reg); err != nil {
			t.Fatal(err)
		}
		return reg
	}
	alpha = end(alpha)
	before := reads("alpha")
	idx.Refresh()
	if got := reads("alpha"); got != before+1 {
		t.Fatalf("the first refresh after the session ended read it %d times, want once", got-before)
	}
	if s, _ := idx.Session("alpha"); s.EndedAt == "" {
		t.Fatalf("the ended registration was not applied: %+v", s)
	}
	bravoBefore := reads("bravo")
	idx.Refresh()
	idx.Refresh()
	if got := reads("alpha"); got != before+1 {
		t.Fatalf("an applied ended registration was re-read %d times by later refreshes", got-before-1)
	}
	if got := reads("bravo"); got != bravoBefore+2 {
		t.Fatalf("a session that only stopped heartbeating was read %d times by 2 refreshes", got-bravoBefore)
	}

	if !idx.Nudge("alpha", project) {
		t.Fatal("a nudge for an ended session was ignored")
	}
	idx.Settle()
	if got := reads("alpha"); got != before+2 {
		t.Fatal("a nudge must still re-read an ended session")
	}

	// Resumed: the serving ws-mcp clears EndedAt and heartbeats.
	alpha.EndedAt = ""
	alpha.HeartbeatAt = stampFor(now)
	if err := WriteRegistration(dir, alpha); err != nil {
		t.Fatal(err)
	}
	idx.Refresh()
	idx.Refresh()
	if got := reads("alpha"); got != before+4 {
		t.Fatalf("a resumed session was read %d times by 2 refreshes, want 2", got-before-2)
	}

	end(alpha)
	idx.Refresh() // applies the ended registration
	if err := os.RemoveAll(project); err != nil {
		t.Fatal(err)
	}
	idx.Refresh()
	if _, ok := idx.Session("alpha"); ok {
		t.Fatal("an ended session whose project root is gone must still be dropped")
	}
}

// countingMailWake counts the daemon's mailbox wake signals.
type countingMailWake struct {
	mu            sync.Mutex
	wakes, nudges int
}

func (c *countingMailWake) Wake()  { c.mu.Lock(); c.wakes++; c.mu.Unlock() }
func (c *countingMailWake) Nudge() { c.mu.Lock(); c.nudges++; c.mu.Unlock() }

func (c *countingMailWake) counts() (wakes, nudges int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.wakes, c.nudges
}

// TestNudgeWakesMailWaitsBeforeTheRegistrationCheck pins the nudge's mail
// wake: every well-formed nudge signals it, a nudge naming no registered
// session included (still answered 404), and a malformed one does not.
func TestNudgeWakesMailWaitsBeforeTheRegistrationCheck(t *testing.T) {
	dir := t.TempDir()
	project := t.TempDir()
	src := newFakeSource()
	register(t, dir, "alpha", project, time.Now())
	wake := &countingMailWake{}
	d := startIndexedDaemon(t, dir, src, time.Hour, func(o *DaemonOptions) { o.MailWake = wake })
	waitFor(t, "first poll", func() bool { _, ok := d.Index().Session("alpha"); return ok })
	client := testClient()

	if err := SendNudge(context.Background(), client, d.Endpoint(), NudgeBody{ReplyID: "nobody", Project: project}); err == nil || !strings.Contains(err.Error(), "404") {
		t.Fatalf("unregistered nudge err = %v, want 404", err)
	}
	if wakes, nudges := wake.counts(); nudges != 1 || wakes != 0 {
		t.Fatalf("after an unregistered nudge: %d wakes, %d nudges; want 0, 1", wakes, nudges)
	}
	if err := SendNudge(context.Background(), client, d.Endpoint(), NudgeBody{ReplyID: "alpha", Project: project}); err != nil {
		t.Fatalf("SendNudge: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/nudge", bytes.NewBufferString(`{not json`))
	req.Host = "127.0.0.1"
	d.Handler().ServeHTTP(httptest.NewRecorder(), req)
	if wakes, nudges := wake.counts(); nudges != 2 || wakes != 0 {
		t.Fatalf("after a registered and a malformed nudge: %d wakes, %d nudges; want 0, 2", wakes, nudges)
	}
}
