package wsdashboard

import (
	"encoding/json"
	"net/http"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsinbox"
	"github.com/kang-sw/devenv/internal/wslive"
)

// drainOne holds one board answer in a fixture session's inbox and drains
// it, leaving one more drained record.
func drainOne(t *testing.T, f *pageFixture, id, text string) {
	t.Helper()
	inbox := f.inbox(id)
	if _, _, err := wsinbox.Send(inbox, wsinbox.Entry{Parts: []string{text}}, time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := wsinbox.Drain(inbox); err != nil {
		t.Fatal(err)
	}
}

// TestSessionPageReadsEachEntryFileOnce pins the entry-file cache: a second
// render of an unchanged session reads no ring or drained entry file again,
// a new ring entry and a new drained record are each read once, a session
// keeps only the ring entries the page shows, and a session that leaves the
// index drops its entries.
func TestSessionPageReadsEachEntryFileOnce(t *testing.T) {
	f := startPageFixture(t)
	drainOne(t, f, "livesession", "first answer")
	render := func() {
		t.Helper()
		if code, body := get(t, f.base+"/s/livesession"); code != http.StatusOK {
			t.Fatalf("GET /s/livesession = %d: %s", code, body)
		}
	}
	files := f.d.entryFiles

	render()
	first := files.parsedCount()
	if first != 5 { // the fixture's 4 ring entries and the drained record
		t.Fatalf("the first render read %d entry files, want 5", first)
	}
	render()
	if n := files.parsedCount(); n != first {
		t.Fatalf("a render of an unchanged session read %d entry files again", n-first)
	}

	appendUsers(t, f, "livesession", 0, 1)
	render()
	render()
	if n := files.parsedCount(); n != first+1 {
		t.Fatalf("a new ring entry was read %d times, want once", n-first)
	}
	drainOne(t, f, "livesession", "second answer")
	render()
	render()
	if n := files.parsedCount(); n != first+2 {
		t.Fatalf("a new drained record was read %d times, want once", n-first-1)
	}

	appendUsers(t, f, "livesession", 1, wslive.RingCap+5)
	render()
	files.mu.Lock()
	kept := len(files.sessions["livesession"].ring)
	files.mu.Unlock()
	if kept != wslive.RingCap {
		t.Fatalf("the session keeps %d ring entries, want the %d the page shows", kept, wslive.RingCap)
	}

	if err := RemoveRegistration(f.dir, "livesession"); err != nil {
		t.Fatal(err)
	}
	f.d.Index().Refresh()
	if files.cached("livesession") {
		t.Fatal("a session that left the index must drop its entry files")
	}
}

// forgetFake is a page source that records ForgetSession calls and serves
// counters of its own.
type forgetFake struct {
	*pageFake
	mu     sync.Mutex
	forgot []string
}

func (f *forgetFake) ForgetSession(replyID string) {
	f.mu.Lock()
	f.forgot = append(f.forgot, replyID)
	f.mu.Unlock()
}

func (f *forgetFake) Counters() map[string]int64 {
	return map[string]int64{"session_reads_changed": 7, "signals": 999}
}

func (f *forgetFake) forgotten() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.forgot...)
}

// trackerCounters is a fake tracker host with reload counters.
type trackerCounters struct{ *fakeTracker }

func (trackerCounters) Counters() map[string]int64 {
	return map[string]int64{"tracker_reloads": 3, "tracker_late_reloads": 2, "tracker_late_reloads_changed": 1}
}

// TestSessionSourceForgetsARemovedSession: the daemon tells a source that
// keeps per-session caches when a session left the index, and only then.
func TestSessionSourceForgetsARemovedSession(t *testing.T) {
	dir, project := t.TempDir(), t.TempDir()
	src := &forgetFake{pageFake: newPageFake()}
	register(t, dir, "alpha", project, time.Now())
	register(t, dir, "bravo", project, time.Now())
	d := startIndexedDaemon(t, dir, src, time.Hour)
	waitFor(t, "first poll", func() bool { _, ok := d.Index().Session("bravo"); return ok })
	src.set("alpha", SessionData{Fingerprint: "changed"})
	d.Index().Refresh()
	if got := src.forgotten(); len(got) != 0 {
		t.Fatalf("a changed session was forgotten: %v", got)
	}
	if err := RemoveRegistration(dir, "alpha"); err != nil {
		t.Fatal(err)
	}
	d.Index().Refresh()
	if got := src.forgotten(); len(got) != 1 || got[0] != "alpha" {
		t.Fatalf("forgotten = %v, want [alpha]", got)
	}
}

// TestStreamsServesSeamCounters: the streams route serves the nudges
// received and the counters the tracker host and the session source keep,
// never letting a seam's key replace one of the daemon's own.
func TestStreamsServesSeamCounters(t *testing.T) {
	dir, project := t.TempDir(), t.TempDir()
	src := &forgetFake{pageFake: newPageFake()}
	register(t, dir, "alpha", project, time.Now())
	d := startIndexedDaemon(t, dir, src, time.Hour, func(o *DaemonOptions) {
		o.Pprof = true
		o.Tracker = trackerCounters{newFakeTracker()}
	})
	waitFor(t, "first poll", func() bool { _, ok := d.Index().Session("alpha"); return ok })
	port := strconv.Itoa(d.Endpoint().Port)
	local := viewer{peer: "127.0.0.1:50000", host: "127.0.0.1:" + port}
	projectJSON, _ := json.Marshal(project)
	for i := 0; i < 2; i++ {
		if rec := serve(d, local, http.MethodPost, "/nudge", `{"reply_id": "alpha", "project": `+string(projectJSON)+`}`); rec.Code != http.StatusNoContent {
			t.Fatalf("nudge = %d", rec.Code)
		}
	}
	rec := serve(d, local, http.MethodGet, "/debug/pprof/streams", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("streams = %d", rec.Code)
	}
	var got map[string]int64
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	want := map[string]int64{"nudges": 2, "session_reads_changed": 7, "tracker_reloads": 3, "tracker_late_reloads": 2, "tracker_late_reloads_changed": 1}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("streams %s = %d, want %d (%v)", k, got[k], v, got)
		}
	}
	if got["signals"] == 999 {
		t.Error("a seam's counter replaced the daemon's own")
	}
}
