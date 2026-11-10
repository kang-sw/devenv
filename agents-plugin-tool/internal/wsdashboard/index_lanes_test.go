package wsdashboard

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// blockingSource is a fakeSource whose reads of a blocked session wait,
// after announcing themselves on entered, until the session is released.
type blockingSource struct {
	*fakeSource
	entered chan string

	mu      sync.Mutex
	blocked map[string]chan struct{}
}

func newBlockingSource() *blockingSource {
	return &blockingSource{fakeSource: newFakeSource(), entered: make(chan string, 64), blocked: map[string]chan struct{}{}}
}

func (b *blockingSource) ReadSession(reg Registration) (SessionData, bool) {
	b.mu.Lock()
	release := b.blocked[reg.ReplyID]
	b.mu.Unlock()
	if release != nil {
		b.entered <- reg.ReplyID
		<-release
	}
	return b.fakeSource.ReadSession(reg)
}

// block makes the next reads of id wait until the returned release runs.
func (b *blockingSource) block(id string) (release func()) {
	ch := make(chan struct{})
	b.mu.Lock()
	b.blocked[id] = ch
	b.mu.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			b.mu.Lock()
			delete(b.blocked, id)
			b.mu.Unlock()
			close(ch)
		})
	}
}

// awaitEntered waits until a blocked read of id began.
func (b *blockingSource) awaitEntered(t *testing.T, id string) {
	t.Helper()
	select {
	case got := <-b.entered:
		if got != id {
			t.Fatalf("blocked read of %s began, want %s", got, id)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("no blocked read of %s began", id)
	}
}

// returns runs fn on its own goroutine and fails the test when fn has not
// returned within a generous bound: the paths under test must not wait on
// the blocked read at all, so reaching the bound means they do.
func returns(t *testing.T, what string, fn func()) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		fn()
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatalf("%s waited on another session's blocked re-read", what)
	}
}

func TestBlockedReadOfOneSessionDelaysNoOtherSession(t *testing.T) {
	dir := t.TempDir()
	project := t.TempDir()
	now := testNow
	src := newBlockingSource()
	register(t, dir, "alpha", project, now)
	register(t, dir, "bravo", project, now)
	idx, _ := newTestIndex(t, dir, src, &now)
	idx.Refresh()

	release := src.block("alpha")
	defer release()
	idx.Reread("alpha")
	src.awaitEntered(t, "alpha")

	// Another session's nudge is scheduled and lands while alpha's read is
	// still blocked; a nudge for alpha itself returns without waiting.
	src.set("bravo", SessionData{HasBoard: true, Fingerprint: "b2"})
	returns(t, "a nudge for another session", func() {
		if !idx.Nudge("bravo", project) {
			t.Error("nudge for bravo was ignored")
		}
	})
	waitFor(t, "bravo's re-read", func() bool { s, _ := idx.Session("bravo"); return s.HasBoard && s.Rev == 2 })
	returns(t, "a nudge for the blocked session", func() {
		if !idx.Nudge("alpha", project) {
			t.Error("nudge for alpha was ignored")
		}
	})

	release()
	idx.Settle()
	if s, _ := idx.Session("alpha"); s.Rev != 1 {
		t.Fatalf("alpha's unchanged re-reads raised its revision: %+v", s)
	}
}

func TestNudgesForOneSessionCoalesceWhileItsReadRuns(t *testing.T) {
	dir := t.TempDir()
	project := t.TempDir()
	now := testNow
	src := newBlockingSource()
	register(t, dir, "alpha", project, now)
	idx, changes := newTestIndex(t, dir, src, &now)
	idx.Refresh()
	before := src.readCounts()["alpha"]
	*changes = nil

	release := src.block("alpha")
	idx.Reread("alpha")
	src.awaitEntered(t, "alpha")
	src.set("alpha", SessionData{Fingerprint: "a2"})
	for i := 0; i < 5; i++ {
		if !idx.Nudge("alpha", project) {
			t.Fatal("nudge was ignored")
		}
		idx.Reread("alpha")
	}
	release()
	idx.Settle()
	if got := src.readCounts()["alpha"] - before; got != 2 {
		t.Fatalf("alpha was read %d times; want the blocked read and one more for the ten requests behind it", got)
	}
	if s, _ := idx.Session("alpha"); s.Rev != 2 || len(*changes) != 1 {
		t.Fatalf("alpha = %+v, changes %v", s, *changes)
	}
}

// TestRefreshSweepNeverUndoesANewerRead pins the per-session ordering
// across a Refresh sweep: a session registered after the sweep listed the
// registry is not removed by it, and a session resumed after the sweep
// listed it as ended is not rolled back to ended.
func TestRefreshSweepNeverUndoesANewerRead(t *testing.T) {
	dir := t.TempDir()
	project := t.TempDir()
	now := testNow
	src := newBlockingSource()
	register(t, dir, "alpha", project, now)
	charlie := register(t, dir, "charlie", project, now)
	idx, _ := newTestIndex(t, dir, src, &now)
	idx.Refresh()
	charlie.EndedAt = stampFor(now)
	if err := WriteRegistration(dir, charlie); err != nil {
		t.Fatal(err)
	}
	idx.Refresh() // applies charlie's end
	if s, _ := idx.Session("charlie"); s.EndedAt == "" {
		t.Fatalf("charlie's end was not applied: %+v", s)
	}

	// The sweep lists alpha and an ended charlie, then blocks on alpha.
	release := src.block("alpha")
	defer release()
	swept := make(chan struct{})
	go func() {
		defer close(swept)
		idx.Refresh()
	}()
	src.awaitEntered(t, "alpha")

	register(t, dir, "bravo", project, now)
	charlie.EndedAt = ""
	if err := WriteRegistration(dir, charlie); err != nil {
		t.Fatal(err)
	}
	returns(t, "nudges during a sweep", func() {
		if !idx.Nudge("bravo", project) || !idx.Nudge("charlie", project) {
			t.Error("a nudge was ignored")
		}
		idx.Settle()
	})
	if _, ok := idx.Session("bravo"); !ok {
		t.Fatal("bravo was not indexed while the sweep ran")
	}
	if s, _ := idx.Session("charlie"); s.EndedAt != "" {
		t.Fatalf("charlie's resume was not applied while the sweep ran: %+v", s)
	}

	release()
	<-swept
	if _, ok := idx.Session("bravo"); !ok {
		t.Fatal("the sweep removed a session registered after it listed the registry")
	}
	if s, _ := idx.Session("charlie"); s.EndedAt != "" {
		t.Fatalf("the sweep rolled charlie back to the ended registration it listed: %+v", s)
	}
}

func TestDaemonAnswersPastABlockedRead(t *testing.T) {
	dir := t.TempDir()
	project := t.TempDir()
	src := newBlockingSource()
	register(t, dir, "alpha", project, time.Now())
	register(t, dir, "bravo", project, time.Now())
	d := startIndexedDaemon(t, dir, src, time.Hour)
	waitFor(t, "first poll", func() bool {
		_, a := d.Index().Session("alpha")
		_, b := d.Index().Session("bravo")
		return a && b
	})
	release := src.block("alpha")
	defer release()
	d.Index().Reread("alpha")
	src.awaitEntered(t, "alpha")
	alpha, _ := d.Index().Session("alpha")

	nudge := func(id string) int {
		body, _ := json.Marshal(NudgeBody{ReplyID: id, Project: project})
		req := httptest.NewRequest(http.MethodPost, "/nudge", bytes.NewReader(body))
		req.Host = "127.0.0.1"
		rec := httptest.NewRecorder()
		d.Handler().ServeHTTP(rec, req)
		return rec.Code
	}
	src.set("bravo", SessionData{HasBoard: true, Fingerprint: "b2"})
	returns(t, "a nudge for another session", func() {
		if code := nudge("bravo"); code != http.StatusNoContent {
			t.Errorf("nudge bravo = %d, want 204", code)
		}
	})
	returns(t, "a nudge for the blocked session", func() {
		if code := nudge("alpha"); code != http.StatusNoContent {
			t.Errorf("nudge alpha = %d, want 204", code)
		}
	})
	wakes := d.stats.WakeAgent.Load()
	returns(t, "an agent write", func() { d.agentChanged("bravo", project) })
	if d.stats.WakeAgent.Load() != wakes+1 {
		t.Fatal("the agent write did not wake its page")
	}
	returns(t, "an inbox write's answer", func() {
		rec := httptest.NewRecorder()
		d.inboxAnswered(rec, alpha, http.StatusOK, map[string]any{"ok": true})
		if rec.Code != http.StatusOK {
			t.Errorf("inbox answer = %d", rec.Code)
		}
	})
	waitFor(t, "bravo's re-read", func() bool { s, _ := d.Index().Session("bravo"); return s.HasBoard })
	release()
	d.Index().Settle()
}
