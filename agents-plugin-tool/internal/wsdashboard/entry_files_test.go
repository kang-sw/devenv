package wsdashboard

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsinbox"
	"github.com/kang-sw/devenv/internal/wslive"
	"github.com/kang-sw/devenv/internal/wsstate"
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

func ringDiskOps() int64 {
	var n int64
	for site, count := range wsstate.FileIOCounters() {
		if strings.HasPrefix(site, "file_io_site:wslive/ring") && (strings.HasSuffix(site, ":read_dir") || strings.HasSuffix(site, ":read_file")) {
			n += count
		}
	}
	return n
}

// TestSessionPageReadsEachEntryFileOnce pins the entry-file cache and RAM ring:
// hosted ring reads are served from RAM with zero disk listings/reads after
// initial load, each new drained record is parsed once, the live view caps at
// RingCap, and a session that leaves the index drops its cached entries.
func TestSessionPageReadsEachEntryFileOnce(t *testing.T) {
	f := startPageFixture(t)
	slot, ok := f.d.OpenHostedSlot(SessionDir(f.dir, "livesession"))
	if !ok {
		t.Fatal("OpenHostedSlot returned false")
	}
	defer slot.Close()
	drainOne(t, f, "livesession", "first answer")
	render := func() {
		t.Helper()
		if code, body := get(t, f.base+"/s/livesession?frame=1"); code != http.StatusOK {
			t.Fatalf("GET /s/livesession = %d: %s", code, body)
		}
	}
	files := f.d.entryFiles

	render()
	first := files.parsedCount()
	if first != 1 { // the drained record (ring is served from daemon RAM)
		t.Fatalf("the first render parsed %d drained entry files, want 1", first)
	}
	ringOpsBefore := ringDiskOps()
	render()
	if n := files.parsedCount(); n != first {
		t.Fatalf("a render of an unchanged session parsed %d drained files again", n-first)
	}
	if n := ringDiskOps() - ringOpsBefore; n != 0 {
		t.Fatalf("a render of an unchanged session made %d ring disk ops, want 0", n)
	}

	appendUsers(t, f, "livesession", 0, 1)
	render()
	render()
	if n := ringDiskOps() - ringOpsBefore; n != 0 {
		t.Fatalf("appending and rendering ring made %d ring disk read/list ops, want 0", n)
	}
	drainOne(t, f, "livesession", "second answer")
	render()
	render()
	if n := files.parsedCount(); n != first+1 {
		t.Fatalf("a new drained record was parsed %d times, want once", n-first)
	}

	appendUsers(t, f, "livesession", 1, wslive.RingCap+5)
	render()
	ring, err := files.readRing("livesession", wslive.RingDir(SessionDir(f.dir, "livesession")))
	if err != nil || len(ring) != wslive.RingCap {
		t.Fatalf("the live ring view returned %d entries (%v), want %d", len(ring), err, wslive.RingCap)
	}
	if n := ringDiskOps() - ringOpsBefore; n != 0 {
		t.Fatalf("ring burst and renders made %d ring disk read/list ops, want 0", n)
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
	local := viewer{peer: "127.0.0.1:50000", host: "127.0.0.1:" + port, localCookie: true}
	projectJSON, _ := json.Marshal(project)
	for i := 0; i < 2; i++ {
		if rec := serve(d, local, http.MethodPost, "/nudge", `{"reply_id": "alpha", "project": `+string(projectJSON)+`}`); rec.Code != http.StatusNoContent {
			t.Fatalf("nudge = %d", rec.Code)
		}
	}
	d.Index().Settle()
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
	for _, k := range []string{"snapshot_cache_hits", "snapshot_cache_misses"} {
		if _, ok := got[k]; !ok {
			t.Errorf("streams lacks %s (%v)", k, got)
		}
	}
	if got["signals"] == 999 {
		t.Error("a seam's counter replaced the daemon's own")
	}
}

// TestHostedSlotPinUnpinAndExternalNudgeSync verifies R1 Findings 4 & 6 at the
// daemon boundary: only active HostedSlot sessions pin their ring in RAM,
// closed/unpinned sessions are never pinned across index refresh, and an
// external legacy .json append to a hosted session becomes visible on reread.
func TestHostedSlotPinUnpinAndExternalNudgeSync(t *testing.T) {
	f := startPageFixture(t)
	closedDir := wslive.RingDir(SessionDir(f.dir, "livesession"))
	if wslive.RingPinned(closedDir) {
		t.Fatal("unhosted session ring pinned before OpenHostedSlot")
	}
	f.d.Index().Refresh()
	if wslive.RingPinned(closedDir) {
		t.Fatal("Index.Refresh pinned unhosted session ring")
	}

	slot, ok := f.d.OpenHostedSlot(SessionDir(f.dir, "livesession"))
	if !ok {
		t.Fatal("OpenHostedSlot failed")
	}
	if !wslive.RingPinned(closedDir) {
		t.Fatal("OpenHostedSlot did not pin hosted ring")
	}
	appendUsers(t, f, "livesession", 0, 1)

	// Simulate external process writing a legacy .json entry directly on disk.
	extName := time.Now().UTC().Add(time.Hour).Format("20060102T150405.000000000Z") + "-f00d.json"
	if err := os.WriteFile(filepath.Join(closedDir, extName), []byte(`{"kind":"user","text":"from-external-proc"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	// External nudge triggers authoritative ReadSession -> SyncExternalRing.
	f.d.Index().Reread("livesession")
	f.d.Index().Settle()
	ring, err := wslive.ReadRing(closedDir)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, rec := range ring {
		if rec.Text == "from-external-proc" {
			found = true
		}
	}
	if !found {
		t.Fatalf("external append not merged into pinned hosted ring after Reread: %+v", ring)
	}

	slot.Close()
	if wslive.RingPinned(closedDir) {
		t.Fatal("slot.Close left hosted ring pinned")
	}
}
