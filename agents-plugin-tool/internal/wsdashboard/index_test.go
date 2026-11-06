package wsdashboard

import (
	"bytes"
	"context"
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
	if after := src.readCounts(); len(after) != len(before) || after["alpha"] != before["alpha"] {
		t.Fatalf("ignored nudges read sessions: before %v after %v", before, after)
	}
	if len(*changes) != 0 {
		t.Fatalf("ignored nudges changed %v", *changes)
	}
}

func startIndexedDaemon(t *testing.T, dir string, src SessionSource, poll time.Duration) *Daemon {
	t.Helper()
	d, err := StartDaemon(DaemonOptions{Dir: dir, Port: 0, Version: "1.0.0", LockWait: 200 * time.Millisecond, Sessions: src, PollInterval: poll})
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
	if code := post("127.0.0.1", `{"reply_id": "alpha", "project": "`+project+`", "extra": [1]}`); code != http.StatusNoContent {
		t.Fatalf("nudge with an unknown field = %d, want 204", code)
	}
	if code := post("127.0.0.1", `{not json`); code != http.StatusBadRequest {
		t.Fatalf("malformed nudge = %d, want 400", code)
	}
	if code := post("evil.example", `{"reply_id": "alpha", "project": "`+project+`"}`); code != http.StatusForbidden {
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
