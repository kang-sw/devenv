package wsdashboard

import (
	"crypto/sha256"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wslive"
)

// ageFile sets path's modification time to ago before now, outside the
// file cache's racy window.
func ageFile(t *testing.T, path string, ago time.Duration) {
	t.Helper()
	at := time.Now().Add(-ago)
	if err := os.Chtimes(path, at, at); err != nil {
		t.Fatal(err)
	}
}

// TestFileCacheServesUnchangedFiles pins FileCache: an unchanged file is
// read once; a rewrite, even one keeping the size, is read again on the next
// load, as is a modification time that moved backwards; a file modified
// within the racy window, a failed load, and a missing file are never kept;
// ForgetUnder drops a directory's files.
func TestFileCacheServesUnchangedFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.json")
	write := func(content string, ago time.Duration) {
		t.Helper()
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		if ago > 0 {
			ageFile(t, path, ago)
		}
	}
	var c FileCache[string]
	reads := 0
	fail := false
	load := func(p string) (string, error) {
		reads++
		raw, err := os.ReadFile(p)
		if err == nil && fail {
			err = errors.New("parse failed")
		}
		return string(raw), err
	}
	expect := func(step, want string, wantReads int) {
		t.Helper()
		got, err := c.Load(path, load)
		if err != nil {
			t.Fatalf("%s: %v", step, err)
		}
		if got != want || reads != wantReads {
			t.Fatalf("%s: got %q after %d reads, want %q after %d", step, got, reads, want, wantReads)
		}
	}

	write("one", time.Hour)
	expect("first", "one", 1)
	expect("unchanged", "one", 1)
	expect("unchanged again", "one", 1)
	if hits, misses := c.Counts(); hits != 2 || misses != 1 {
		t.Fatalf("counts = %d hits, %d misses, want 2 and 1", hits, misses)
	}

	write("two", 50*time.Minute) // the same size, a newer time
	expect("rewritten", "two", 2)
	expect("rewritten, unchanged", "two", 2)
	write("tri", 2*time.Hour) // the same size, an older time
	expect("time moved back", "tri", 3)

	write("racy", 0)
	expect("racy", "racy", 4)
	expect("still racy", "racy", 5)
	ageFile(t, path, time.Hour)
	expect("aged", "racy", 6)
	expect("aged, unchanged", "racy", 6)

	write("bad!", 40*time.Minute)
	fail = true
	if _, err := c.Load(path, load); err == nil {
		t.Fatal("a failed load returned no error")
	}
	fail = false
	expect("after a failed load", "bad!", 8)

	if len(c.cachedPaths()) != 1 {
		t.Fatalf("cached = %v", c.cachedPaths())
	}
	c.ForgetUnder(dir)
	if len(c.cachedPaths()) != 0 {
		t.Fatalf("ForgetUnder left %v", c.cachedPaths())
	}

	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if _, err := c.Load(path, load); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("a missing file: %v", err)
		}
	}
	if reads != 10 {
		t.Fatalf("a missing file must reach the loader each time: %d reads", reads)
	}
}

// TestStampWatchedListsTicketDirectories: a watched directory whose files
// are watched too is stamped by its listing, so an in-place edit of one of
// those files (same name, a new size or time) moves the stamp, as does an
// added file; a watched path outside any listed directory keeps its stat.
func TestStampWatchedListsTicketDirectories(t *testing.T) {
	root := t.TempDir()
	ready := filepath.Join(root, "ready")
	if err := os.MkdirAll(ready, 0o755); err != nil {
		t.Fatal(err)
	}
	a, b := filepath.Join(ready, "a.md"), filepath.Join(ready, "b.md")
	other := filepath.Join(root, "marker.json")
	for _, p := range []string{a, b, other} {
		if err := os.WriteFile(p, []byte("text"), 0o644); err != nil {
			t.Fatal(err)
		}
		ageFile(t, p, time.Hour)
	}
	watch := []string{ready, a, b, other}
	stamp := func() string {
		h := sha256.New()
		StampWatched(h, watch)
		return sumHex(h)
	}
	last := stamp()
	step := func(name string, change func()) {
		t.Helper()
		change()
		next := stamp()
		if next == last {
			t.Fatalf("%s: the stamp did not change", name)
		}
		last = next
	}
	if stamp() != last {
		t.Fatal("an unchanged watch list stamped differently")
	}
	step("covered file's time", func() { ageFile(t, a, 30*time.Minute) })
	step("covered file's size", func() {
		if err := os.WriteFile(b, []byte("longer text"), 0o644); err != nil {
			t.Fatal(err)
		}
		ageFile(t, b, time.Hour)
	})
	step("added file", func() {
		if err := os.WriteFile(filepath.Join(ready, "c.md"), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	})
	step("unlisted file's time", func() { ageFile(t, other, 10*time.Minute) })
}

// TestSnapshotCacheFollowsTheSession: the daemon's page renders read an
// unchanged snapshot once, and a session that leaves the index drops its
// cached snapshot; the streams route serves the cache's counters.
func TestSnapshotCacheFollowsTheSession(t *testing.T) {
	f := startPageFixture(t)
	// The page looks up its contact's snapshot too; a missing one is read
	// (stat and load) every time.
	if err := os.MkdirAll(SessionDir(f.dir, "boardsession"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeSnapshot(t, f.dir, "boardsession", &wslive.Snapshot{SchemaVersion: 1})
	for _, id := range []string{"livesession", "boardsession"} {
		ageFile(t, wslive.SnapshotPath(SessionDir(f.dir, id)), time.Hour)
	}
	render := func() {
		t.Helper()
		if code, body := get(t, f.base+"/s/livesession"); code != 200 {
			t.Fatalf("GET /s/livesession = %d: %s", code, body)
		}
	}
	render()
	_, before := f.d.snapshots.Counts()
	render()
	if _, misses := f.d.snapshots.Counts(); misses != before {
		t.Fatalf("a render of an unchanged snapshot read it %d more times", misses-before)
	}
	sessionDir := SessionDir(f.dir, "livesession")
	held := func() bool {
		for _, p := range f.d.snapshots.files.cachedPaths() {
			if strings.HasPrefix(p, sessionDir+string(filepath.Separator)) {
				return true
			}
		}
		return false
	}
	if !held() {
		t.Fatal("the rendered session's snapshot is not cached")
	}
	if err := RemoveRegistration(f.dir, "livesession"); err != nil {
		t.Fatal(err)
	}
	f.d.Index().Refresh()
	if held() {
		t.Fatal("a session that left the index must drop its cached snapshot")
	}
}

// mailFake is a page source with a mail print (MailPrintSource).
type mailFake struct {
	*pageFake
	print string
}

func (m *mailFake) MailPrint(Registration) (string, bool) { return m.print, true }

// TestSessionStampFoldsTheMailPrint: the refresh stamp of a page follows
// its session's mail print, so an unread count or contact change that no
// watched file reports still re-reads the page.
func TestSessionStampFoldsTheMailPrint(t *testing.T) {
	f := startPageFixture(t)
	src := &mailFake{pageFake: f.src, print: "unread 0"}
	r := f.d.pages()
	r.source = agentSessions{inner: src, dir: f.dir, now: time.Now}
	in := refreshInputs{ids: []string{"boardsession"}}
	first, ok := r.sessionStamp("boardsession", in)
	if !ok {
		t.Fatal("no stamp")
	}
	if again, _ := r.sessionStamp("boardsession", in); again != first {
		t.Fatal("an unchanged print stamped differently")
	}
	src.print = "unread 1"
	if next, _ := r.sessionStamp("boardsession", in); next == first {
		t.Fatal("a changed mail print kept the stamp")
	}
}
