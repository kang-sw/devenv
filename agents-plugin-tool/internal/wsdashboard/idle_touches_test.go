package wsdashboard

import (
	"fmt"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wslive"
)

func TestIdleTouchesSharedRefresh(t *testing.T) {
	c := newPageCache(newBroker(&streamStats{}), &streamStats{})
	key := pageKey{replyID: "session"}
	p := c.acquire(key)
	defer c.release(key, p)
	stamps, renders := 0, 0
	value := "a"
	src := pageSource{key: key,
		stamp: func(refreshInputs) (string, bool) { stamps++; return value, true },
		render: func(*liveRender) pageRender {
			renders++
			return pageRender{ok: true, deps: []string{"session"}, regions: regionSet{}}
		},
	}
	c.get(p, src, false)
	c.get(p, src, true) // establish the pre-render stamp
	for range 28 {
		c.get(p, src, true)
	}
	if stamps != 1 || renders != 2 {
		t.Fatalf("shared burst: stamps=%d renders=%d", stamps, renders)
	}
	t.Logf("28 comparable idle refreshes: stamp calls 28 -> %d (after warmup)", stamps-1)
	// Expiry is bounded and not slid by those hits.
	p.checked = time.Now().Add(-pageStampInterval)
	value = "external edit"
	c.get(p, src, true)
	if stamps != 2 || renders != 3 {
		t.Fatalf("expiry: stamps=%d renders=%d", stamps, renders)
	}
	// A semantic wake bypasses the memo immediately, even inside its window.
	c.broker.wake("session")
	c.get(p, src, false)
	if stamps != 3 || renders != 4 {
		t.Fatalf("wake: stamps=%d renders=%d", stamps, renders)
	}
	// Errors and uncertain stamps must not gain a memo.
	src.stamp = func(refreshInputs) (string, bool) { stamps++; return "", false }
	p.checked = time.Time{}
	c.get(p, src, true)
	c.get(p, src, true)
	if stamps != 5 || renders != 6 {
		t.Fatalf("uncertain: stamps=%d renders=%d", stamps, renders)
	}
}

func TestIdleTouchesAdapterStampUsesLastKnownSnapshot(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	cache := &SnapshotCache{}
	clock := now
	cache.files.now = func() time.Time { return clock }
	r := pageReader{dir: dir, snapshots: cache}
	present := func(id string, at time.Time) bool { ok, _ := r.adapterStamp(id, at); return ok }
	register(t, dir, "alpha", dir, now)
	writeSnapshot(t, dir, "alpha", &wslive.Snapshot{SchemaVersion: 1, Adapter: "pi", UpdatedAt: now.UnixMilli(), HeartbeatMs: 30000})
	if present("alpha", now) {
		t.Fatal("unread snapshot gained authority")
	}
	if cache.Snapshot(dir, "alpha") == nil {
		t.Fatal("snapshot read failed")
	}
	for i := range 68 {
		id := fmt.Sprintf("session%d", i)
		register(t, dir, id, dir, now)
		writeSnapshot(t, dir, id, &wslive.Snapshot{SchemaVersion: 1, Adapter: "pi", UpdatedAt: now.UnixMilli(), HeartbeatMs: 30000})
		if cache.Snapshot(dir, id) == nil {
			t.Fatal("snapshot read failed")
		}
	}
	hits, misses := cache.Counts()
	for range 28 {
		for i := range 68 {
			if !present(fmt.Sprintf("session%d", i), now) {
				t.Fatal("known adapter absent")
			}
		}
	}
	if h, m := cache.Counts(); h != hits || m != misses {
		t.Fatalf("stamps probed snapshot cache: (%d,%d) -> (%d,%d)", hits, misses, h, m)
	}
	t.Log("68 sessions x 28 stamps: snapshot identity checks 1904 -> 0 after initial reads")
	if present("alpha", now.Add(2*time.Minute)) {
		t.Fatal("last-known evidence failed to age out")
	}
	// An external writer is discovered by the existing index read/backstop.
	fresh := now.Add(2 * time.Minute)
	writeSnapshot(t, dir, "alpha", &wslive.Snapshot{SchemaVersion: 1, Adapter: "pi", UpdatedAt: fresh.UnixMilli(), HeartbeatMs: 30000})
	clock = clock.Add(fileCacheValidationInterval)
	cache.Snapshot(dir, "alpha")
	if !present("alpha", fresh) {
		t.Fatal("backstop evidence was not used")
	}
	cache.SetLive(dir, "alpha", wslive.Snapshot{SchemaVersion: 1, Adapter: "pi", UpdatedAt: fresh.Add(time.Minute).UnixMilli(), HeartbeatMs: 30000})
	if !present("alpha", fresh.Add(time.Minute)) {
		t.Fatal("live evidence did not take precedence")
	}
	cache.ClearLive(dir, "alpha")
	if present("alpha", fresh.Add(time.Minute)) {
		t.Fatal("cleared live evidence survived")
	}
}

func TestIdleTouchesRegistrationHeartbeat(t *testing.T) {
	dir, project := t.TempDir(), t.TempDir()
	now := testNow
	register(t, dir, "alpha", project, now)
	idx, changes := newTestIndex(t, dir, newFakeSource(), &now)
	idx.Refresh()
	*changes = nil
	reg, err := ReadRegistration(dir, "alpha")
	if err != nil {
		t.Fatal(err)
	}
	for range 10 {
		now = now.Add(HeartbeatInterval)
		reg.HeartbeatAt = stampFor(now)
		if err := WriteRegistration(dir, reg); err != nil {
			t.Fatal(err)
		}
		idx.Refresh()
	}
	s, _ := idx.Session("alpha")
	if s.Rev != 1 || len(*changes) != 0 || s.HeartbeatAt != reg.HeartbeatAt || !s.Live {
		t.Fatalf("heartbeat evidence not retained quietly: %+v changes=%v", s, *changes)
	}
	t.Log("10 comparable heartbeat writes: index wakes 10 -> 0; heartbeat interval unchanged")
	now = now.Add(LiveWindow + time.Second)
	idx.Refresh()
	s, _ = idx.Session("alpha")
	if s.Live || s.Rev != 2 || len(*changes) != 1 {
		t.Fatalf("stale transition: %+v %v", s, *changes)
	}
	reg.HeartbeatAt = stampFor(now)
	if err := WriteRegistration(dir, reg); err != nil {
		t.Fatal(err)
	}
	idx.Refresh()
	s, _ = idx.Session("alpha")
	if !s.Live || s.Rev != 3 || len(*changes) != 2 {
		t.Fatalf("recovery transition: %+v %v", s, *changes)
	}
	reg.Harness = "pi"
	if err := WriteRegistration(dir, reg); err != nil {
		t.Fatal(err)
	}
	idx.Refresh()
	s, _ = idx.Session("alpha")
	if s.Rev != 4 || len(*changes) != 3 {
		t.Fatalf("semantic transition: %+v %v", s, *changes)
	}
}
