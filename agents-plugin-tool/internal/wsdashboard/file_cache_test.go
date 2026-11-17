package wsdashboard

import (
	"crypto/sha256"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wslive"
	"github.com/kang-sw/devenv/internal/wswake"
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
// read once; a rewrite, even one keeping the size, is read again at the next
// validation, as is a modification time that moved backwards; a file modified
// within the racy window is reloaded at validation; failed loads and missing
// files are never kept;
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
	clock := time.Now()
	c := FileCache[string]{now: func() time.Time { return clock }}
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
	expect("before validation", "one", 1)
	clock = clock.Add(fileCacheValidationInterval)
	expect("rewritten", "two", 2)
	expect("rewritten, unchanged", "two", 2)
	write("tri", 2*time.Hour) // the same size, an older time
	clock = clock.Add(fileCacheValidationInterval)
	expect("time moved back", "tri", 3)

	// Put this write on the fake clock so it remains inside the racy window.
	write("racy", 0)
	clock = clock.Add(fileCacheValidationInterval)
	if err := os.Chtimes(path, clock, clock); err != nil {
		t.Fatal(err)
	}
	expect("racy", "racy", 4)
	expect("racy, within interval", "racy", 4)
	clock = clock.Add(fileCacheValidationInterval)
	expect("still racy", "racy", 5)
	ageFile(t, path, time.Hour)
	clock = clock.Add(fileCacheValidationInterval)
	expect("aged", "racy", 6)
	expect("aged, unchanged", "racy", 6)

	write("bad!", 40*time.Minute)
	clock = clock.Add(fileCacheValidationInterval)
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

func TestFileCachePathIOIsolationAndInvalidation(t *testing.T) {
	for _, invalidation := range []string{"invalidate", "forget", "forget under"} {
		t.Run(invalidation, func(t *testing.T) {
			dir := t.TempDir()
			for _, id := range []string{"a", "b"} {
				if err := os.MkdirAll(SessionDir(dir, id), 0700); err != nil {
					t.Fatal(err)
				}
				writeSnapshot(t, dir, id, &wslive.Snapshot{SchemaVersion: 1, Adapter: "pi"})
			}
			pathA := wslive.SnapshotPath(SessionDir(dir, "a"))
			started, release := make(chan struct{}), make(chan struct{})
			var once sync.Once
			unblock := func() { once.Do(func() { close(release) }) }
			defer unblock()
			var readsA, readsB atomic.Int64
			cache := SnapshotCache{load: func(path string) (parsedSnapshot, error) {
				value, err := loadSnapshot(path)
				if path == pathA {
					if readsA.Add(1) == 1 {
						close(started)
						<-release // The old result is already captured.
					}
				} else {
					readsB.Add(1)
				}
				return value, err
			}}
			clock := time.Now()
			cache.files.now = func() time.Time { return clock }
			if cache.snapshotWithoutObserved(dir, "b") == nil {
				t.Fatal("warm B snapshot missing")
			}
			loaded := make(chan *wslive.Snapshot, 1)
			go func() { loaded <- cache.snapshotWithoutObserved(dir, "a") }()
			select {
			case <-started:
			case <-time.After(5 * time.Second):
				t.Fatal("A loader did not start")
			}
			finish := func(name string, fn func()) {
				t.Helper()
				done := make(chan struct{})
				go func() { fn(); close(done) }()
				select {
				case <-done:
				case <-time.After(5 * time.Second):
					t.Fatalf("%s blocked behind A's I/O", name)
				}
			}
			var b *wslive.Snapshot
			finish("warm B load", func() { b = cache.snapshotWithoutObserved(dir, "b") })
			if b == nil || b.Adapter != "pi" || readsB.Load() != 1 {
				t.Fatal("warm B did not reuse its cached value")
			}
			finish("B invalidation", func() { cache.Invalidate(dir, "b") })
			finish("last-known B lookup", func() { b = cache.lastKnown(dir, "b") })
			if b == nil || b.Adapter != "pi" {
				t.Fatal("B invalidation discarded last-known evidence")
			}
			writeSnapshot(t, dir, "a", &wslive.Snapshot{SchemaVersion: 1, Adapter: "claude"})
			finish("in-flight A invalidation", func() {
				switch invalidation {
				case "invalidate":
					cache.Invalidate(dir, "a")
				case "forget":
					cache.files.forget(pathA)
				case "forget under":
					cache.files.ForgetUnder(SessionDir(dir, "a"))
				}
			})
			unblock()
			select {
			case old := <-loaded:
				if old == nil || old.Adapter != "pi" {
					t.Fatal("blocked reader did not retain its captured result")
				}
			case <-time.After(5 * time.Second):
				t.Fatal("A loader did not finish")
			}
			if cache.lastKnown(dir, "a") != nil {
				t.Fatal("old in-flight read repopulated invalidated evidence")
			}
			if fresh := cache.snapshotWithoutObserved(dir, "a"); fresh == nil || fresh.Adapter != "claude" || readsA.Load() != 2 {
				t.Fatal("invalidation was undone by the older load")
			}
		})
	}
}

func TestFileCacheRecentAndMissingValidation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "recent")
	clock := time.Now()
	stats, reads := 0, 0
	cache := FileCache[string]{
		now:  func() time.Time { return clock },
		stat: func(path string) (os.FileInfo, error) { stats++; return os.Stat(path) },
	}
	load := func(path string) (string, error) {
		reads++
		raw, err := os.ReadFile(path)
		return string(raw), err
	}
	write := func(value string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(value), 0600); err != nil {
			t.Fatal(err)
		}
		// Preserve the exact stamp to exercise the coarse-mtime collision.
		if err := os.Chtimes(path, clock, clock); err != nil {
			t.Fatal(err)
		}
	}
	write("one")
	cache.Load(path, load)
	write("two")
	for i := 0; i < 64; i++ {
		if got, _ := cache.Load(path, load); got != "one" {
			t.Fatal("recent value was not shared within interval")
		}
	}
	if stats != 1 || reads != 1 {
		t.Fatalf("recent burst: stats=%d reads=%d", stats, reads)
	}
	clock = clock.Add(fileCacheValidationInterval)
	if got, err := cache.Load(path, load); err != nil || got != "two" || stats != 2 || reads != 2 {
		t.Fatalf("racy validation: %q, %v; stats=%d reads=%d", got, err, stats, reads)
	}
	missing := filepath.Join(dir, "missing")
	for i := 0; i < 64; i++ {
		if _, err := cache.Load(missing, load); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("missing file: %v", err)
		}
	}
	if stats != 3 || reads != 66 {
		t.Fatalf("missing burst: stats=%d reads=%d, want 3 and 66", stats, reads)
	}
	clock = clock.Add(fileCacheValidationInterval)
	cache.Load(missing, load)
	if stats != 4 || reads != 67 {
		t.Fatalf("missing validation: stats=%d reads=%d", stats, reads)
	}
	cache.ForgetUnder(dir)
	cache.Load(missing, load)
	if stats != 5 || reads != 68 {
		t.Fatalf("missing invalidation: stats=%d reads=%d", stats, reads)
	}
}

func TestFileCacheValidationSharedAcrossReaders(t *testing.T) {
	for _, identity := range []bool{false, true} {
		t.Run(map[bool]string{false: "stamp", true: "identity"}[identity], func(t *testing.T) {
			dir := t.TempDir()
			paths := []string{filepath.Join(dir, "one"), filepath.Join(dir, "two")}
			for _, path := range paths {
				if err := os.WriteFile(path, []byte("old"), 0600); err != nil {
					t.Fatal(err)
				}
				ageFile(t, path, time.Hour)
			}
			clock := time.Now()
			var stats, reads atomic.Int64
			cache := FileCache[string]{
				now: func() time.Time { return clock },
				stat: func(path string) (os.FileInfo, error) {
					stats.Add(1)
					return os.Stat(path)
				},
			}
			load := func(path string) (string, error) {
				reads.Add(1)
				raw, err := os.ReadFile(path)
				return string(raw), err
			}
			read := cache.Load
			if identity {
				read = cache.LoadIdentity
			}
			burst := func() {
				t.Helper()
				var wg sync.WaitGroup
				for i := 0; i < 64; i++ {
					wg.Add(1)
					go func() {
						defer wg.Done()
						for _, path := range paths {
							if got, err := read(path, load); err != nil || got != "old" {
								t.Errorf("read = %q, %v", got, err)
							}
						}
					}()
				}
				wg.Wait()
			}
			burst() // Cold concurrent loads share both the stat and loader.
			clock = clock.Add(fileCacheValidationInterval - time.Nanosecond)
			burst()
			if stats.Load() != 2 || reads.Load() != 2 {
				t.Fatalf("within interval: stats=%d reads=%d, want 2 each", stats.Load(), reads.Load())
			}
			clock = clock.Add(time.Nanosecond)
			burst()
			if stats.Load() != 4 || reads.Load() != 2 {
				t.Fatalf("at boundary: stats=%d reads=%d, want 4 and 2", stats.Load(), reads.Load())
			}
			clock = clock.Add(-time.Hour)
			burst()
			if stats.Load() != 6 || reads.Load() != 2 {
				t.Fatalf("backward clock: stats=%d reads=%d, want 6 and 2", stats.Load(), reads.Load())
			}
			if err := os.WriteFile(paths[0], []byte("own write"), 0600); err != nil {
				t.Fatal(err)
			}
			if got, _ := read(paths[0], load); got != "old" {
				t.Fatal("unvalidated write escaped the interval")
			}
			cache.forget(paths[0])
			if got, err := read(paths[0], load); err != nil || got != "own write" {
				t.Fatalf("forget did not expose own write immediately: %q, %v", got, err)
			}
			cache.ForgetUnder(dir)
			if got, err := read(paths[1], load); err != nil || got != "old" {
				t.Fatalf("ForgetUnder: %q, %v", got, err)
			}
			if stats.Load() != 8 || reads.Load() != 4 {
				t.Fatalf("invalidation: stats=%d reads=%d, want 8 and 4", stats.Load(), reads.Load())
			}
		})
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
	now := time.Now()
	cache := watchedStampCache{now: func() time.Time { return now }}
	stamp := func() string {
		h := sha256.New()
		cache.writeWatched(h, watch, time.Time{})
		return sumHex(h)
	}
	last := stamp()
	step := func(name string, change func()) {
		t.Helper()
		change()
		now = now.Add(watchedStampInterval)
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

	// A covered ticket that is a symbolic link follows its target.
	target := filepath.Join(root, "target.md")
	if err := os.WriteFile(target, []byte("text"), 0o644); err != nil {
		t.Fatal(err)
	}
	ageFile(t, target, time.Hour)
	link := filepath.Join(ready, "d.md")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("no symbolic links: %v", err)
	}
	watch = append(watch, link)
	now = now.Add(watchedStampInterval)
	last = stamp()
	step("a linked ticket's target", func() { ageFile(t, target, 20*time.Minute) })
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
		if code, body := get(t, f.base+"/s/livesession?frame=1"); code != 200 {
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
	reads int
}

func (m *mailFake) MailPrint(Registration) (string, bool) { m.reads++; return m.print, true }

// TestSessionStampFoldsTheMailPrint: the refresh stamp of a page follows
// its session's mail print, so an unread count or contact change that no
// watched file reports still re-reads the page. Idle stamps share one
// mailbox look; an in-daemon queue change or a nudge renews it at once, and
// a write neither reports shows at the next foreign look.
func TestSessionStampFoldsTheMailPrint(t *testing.T) {
	hub := &wswake.Hub{}
	f := startPageFixture(t, func(o *DaemonOptions) { o.MailWake = hub })
	src := &mailFake{pageFake: f.src, print: "unread 0"}
	r := f.d.pages()
	r.source = agentSessions{inner: src, dir: f.dir, now: time.Now}
	in := refreshInputs{ids: []string{"boardsession"}}
	first, ok := r.sessionStamp("boardsession", in)
	if !ok {
		t.Fatal("no stamp")
	}
	for range 10 {
		if again, _ := r.sessionStamp("boardsession", in); again != first {
			t.Fatal("an unchanged print stamped differently")
		}
	}
	if src.reads != 1 {
		t.Fatalf("11 idle stamps read the mail print %d times, want one", src.reads)
	}
	src.print = "unread 1"
	if next, _ := r.sessionStamp("boardsession", in); next != first {
		t.Fatal("an unreported print change was read before the foreign look")
	}
	hub.StoreChanged("queue.json")
	second, _ := r.sessionStamp("boardsession", in)
	if second == first {
		t.Fatal("a queue change hint kept the stamp")
	}
	src.print = "unread 2"
	f.d.foreign.mailChanged() // a nudge
	third, _ := r.sessionStamp("boardsession", in)
	if third == second {
		t.Fatal("a nudge kept the stamp")
	}
	src.print = "unread 3"
	f.d.foreign.mu.Lock()
	f.d.foreign.mailAt = f.d.foreign.mailAt.Add(-foreignSafetyInterval)
	f.d.foreign.mu.Unlock()
	if next, _ := r.sessionStamp("boardsession", in); next == third {
		t.Fatal("the foreign look kept the stamp")
	}
}
