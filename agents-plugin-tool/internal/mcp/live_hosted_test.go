package mcp

import (
	"encoding/json"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wslive"
)

// fakeLiveHost is a dashboard daemon hosting the server: it records what
// each slot was handed. serve false declines every slot, as a daemon does
// for a session directory it does not serve.
type fakeLiveHost struct {
	serve bool
	mu    sync.Mutex
	slots []*fakeHostedSlot
}

func (h *fakeLiveHost) OpenHostedSlot(sessionDir string) (wsdashboard.HostedSlot, bool) {
	if !h.serve {
		return nil, false
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	slot := &fakeHostedSlot{dir: sessionDir}
	h.slots = append(h.slots, slot)
	return slot, true
}

func (h *fakeLiveHost) slot(t *testing.T) *fakeHostedSlot {
	t.Helper()
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.slots) != 1 {
		t.Fatalf("%d slots opened, want 1", len(h.slots))
	}
	return h.slots[0]
}

type fakeHostedSlot struct {
	dir    string
	mu     sync.Mutex
	snaps  []wslive.Snapshot
	rings  int
	closed int
}

func (s *fakeHostedSlot) Publish(snap wslive.Snapshot) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.snaps = append(s.snaps, snap)
}

func (s *fakeHostedSlot) RingAppended() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rings++
}

func (s *fakeHostedSlot) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed++
}

func (s *fakeHostedSlot) published() []wslive.Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]wslive.Snapshot(nil), s.snaps...)
}

// countWrites counts the live layer's snapshot file writes.
func countWrites(t *testing.T) *atomic.Int32 {
	t.Helper()
	var writes atomic.Int32
	setLiveSeams(func(s *liveSeams) {
		inner := s.writeSnapshot
		s.writeSnapshot = func(path string, data []byte) error {
			writes.Add(1)
			return inner(path, data)
		}
	})
	return &writes
}

// directIntake is intake through the handler alone: a call through the
// one-shot serve loop of callToolWithKey ends the session's registration
// when the loop stops, so the next call registers again and nudges, which
// would hide the nudges the live layer sends.
func directIntake(t *testing.T, s *Server, key string, events ...map[string]any) {
	t.Helper()
	list := make([]any, len(events))
	for i, ev := range events {
		list[i] = ev
	}
	resp := s.handleDashboardIntake(json.RawMessage("1"), map[string]any{"session_key": key, "adapter": "pi", "events": list})
	if responseIsError(resp) {
		t.Fatalf("intake: %+v", resp)
	}
}

// TestHostedLiveHandsSnapshotsOverInMemory pins the hosted path: each
// debounced change is published to the daemon's slot without a file write
// or a nudge; the file is written for the first snapshot, at a turn's end,
// and once more when the server closes, which releases the slot.
func TestHostedLiveHandsSnapshotsOverInMemory(t *testing.T) {
	f := newLiveFixture(t, true)
	fastLive(t, 5*time.Millisecond)
	host := &fakeLiveHost{serve: true}
	f.server.live.host = host
	writes := countWrites(t)
	fake := startFakeDaemon(t, f.stateDir, http.StatusNoContent, false)
	callToolWithKey(t, f.server, 3, f.keyA, "mailbox.lookup_peers", nil)
	waitForNudge(t, fake, f.replyA) // the registration's
	time.Sleep(50 * time.Millisecond)
	nudges := len(fake.received())

	directIntake(t, f.server, f.keyA, map[string]any{"kind": "turn_start"})
	waitFor(t, "the first snapshot write", func() bool { return writes.Load() == 1 })
	slot := host.slot(t)
	if slot.dir != f.sessionDir(f.replyA) {
		t.Fatalf("slot opened for %q, want %q", slot.dir, f.sessionDir(f.replyA))
	}
	for i := 0; i < 3; i++ {
		n := len(slot.published())
		directIntake(t, f.server, f.keyA, map[string]any{"kind": "agent", "caption": string(rune('a' + i))})
		waitFor(t, "a published change", func() bool { return len(slot.published()) > n })
	}
	time.Sleep(30 * time.Millisecond)
	if n := writes.Load(); n != 1 {
		t.Fatalf("%d snapshot writes after the first snapshot and three changes in one heartbeat interval, want 1", n)
	}

	directIntake(t, f.server, f.keyA, map[string]any{"kind": "turn_end"})
	waitFor(t, "the turn end's flush", func() bool { return writes.Load() == 2 })
	time.Sleep(30 * time.Millisecond)
	if got := len(fake.received()); got != nudges {
		t.Fatalf("%d nudges after hosted snapshots, want %d (none): %+v", got, nudges, fake.received())
	}

	last := slot.published()
	f.server.Close()
	if n := writes.Load(); n != 3 {
		t.Fatalf("%d snapshot writes after close, want 3 (the close flush)", n)
	}
	slot.mu.Lock()
	closed := slot.closed
	slot.mu.Unlock()
	if closed != 1 {
		t.Fatalf("slot closed %d times, want 1", closed)
	}
	snap, err := wslive.ReadSnapshot(wslive.SnapshotPath(f.sessionDir(f.replyA)))
	if err != nil || snap.UpdatedAt < last[len(last)-1].UpdatedAt || snap.Lead.Phase != last[len(last)-1].Lead.Phase {
		t.Fatalf("flushed snapshot = %+v, %v; last published %+v", snap.Lead, err, last[len(last)-1].Lead)
	}
}

// TestHostedLiveIdleHeartbeatWritesNothing pins the idle cost of a hosted
// session: a heartbeat with nothing changed writes no file, yet its publish
// moves the copy's updatedAt, so the session does not read as stale long
// past StaleAfterBeats heartbeat intervals of the file's age.
func TestHostedLiveIdleHeartbeatWritesNothing(t *testing.T) {
	f := newLiveFixture(t, true)
	fastLive(t, 5*time.Millisecond)
	host := &fakeLiveHost{serve: true}
	f.server.live.host = host
	writes := countWrites(t)
	var clock atomic.Int64
	clock.Store(time.Now().UnixMilli())
	setLiveSeams(func(s *liveSeams) {
		s.heartbeat = 20 * time.Millisecond
		// Each reading of the clock is 40 s on: a heartbeat apart is far more
		// than StaleAfterBeats real heartbeats.
		s.now = func() time.Time { return time.UnixMilli(clock.Add(40_000)) }
	})
	t.Cleanup(func() { f.server.Close() })

	intake(t, f.server, f.keyA, map[string]any{"kind": "turn_start"})
	waitFor(t, "the first snapshot write", func() bool { return writes.Load() == 1 })
	slot := host.slot(t)
	base := len(slot.published())
	waitFor(t, "idle heartbeats", func() bool { return len(slot.published()) >= base+4 })
	if n := writes.Load(); n != 1 {
		t.Fatalf("%d snapshot writes over idle heartbeats, want 1", n)
	}
	snaps := slot.published()
	first, last := snaps[base-1], snaps[len(snaps)-1]
	if last.UpdatedAt-first.UpdatedAt < 3*40_000 {
		t.Fatalf("idle heartbeats moved updatedAt %d -> %d, want it to follow the clock", first.UpdatedAt, last.UpdatedAt)
	}
	// Judged at the clock's time now, well past StaleAfterBeats heartbeats
	// after the only file write, the copy is fresh: it is the last publish.
	at := time.UnixMilli(clock.Load())
	if !first.Stale(at) || last.Stale(at) {
		t.Fatalf("at %d: the first copy (updatedAt %d) should read stale and the last (%d) fresh", at.UnixMilli(), first.UpdatedAt, last.UpdatedAt)
	}
}

// TestHostedLiveBusyTurnFlushesEachHeartbeat pins the recovery bound of a
// long turn: while changes keep arriving, the file is written once a
// heartbeat interval passed since the last write, with no turn end.
func TestHostedLiveBusyTurnFlushesEachHeartbeat(t *testing.T) {
	f := newLiveFixture(t, true)
	fastLive(t, 5*time.Millisecond)
	host := &fakeLiveHost{serve: true}
	f.server.live.host = host
	writes := countWrites(t)
	var clock atomic.Int64
	clock.Store(time.Now().UnixMilli())
	setLiveSeams(func(s *liveSeams) {
		// The heartbeat timer itself never fires within the test; the
		// injected clock decides when an interval passed.
		s.heartbeat = time.Hour
		s.now = func() time.Time { return time.UnixMilli(clock.Load()) }
	})
	t.Cleanup(func() { f.server.Close() })

	directIntake(t, f.server, f.keyA, map[string]any{"kind": "turn_start"})
	waitFor(t, "the first snapshot write", func() bool { return writes.Load() == 1 })
	slot := host.slot(t)
	change := func(caption string) {
		t.Helper()
		n := len(slot.published())
		directIntake(t, f.server, f.keyA, map[string]any{"kind": "agent", "caption": caption})
		waitFor(t, "a published change", func() bool { return len(slot.published()) > n })
	}
	change("a")
	clock.Add((30 * time.Minute).Milliseconds())
	change("b")
	if n := writes.Load(); n != 1 {
		t.Fatalf("%d writes within one heartbeat interval of a busy turn, want 1", n)
	}
	clock.Add((30 * time.Minute).Milliseconds())
	change("c")
	waitFor(t, "the busy turn's heartbeat flush", func() bool { return writes.Load() == 2 })
	snap, err := wslive.ReadSnapshot(wslive.SnapshotPath(f.sessionDir(f.replyA)))
	if err != nil || snap.Lead.Caption != "c" {
		t.Fatalf("flushed snapshot caption = %q, %v; want the newest change", snap.Lead.Caption, err)
	}
}

// TestHostedLiveUnservedDirStaysDirect pins the fallback: a session whose
// directory the daemon does not serve writes the file at every fire.
func TestHostedLiveUnservedDirStaysDirect(t *testing.T) {
	f := newLiveFixture(t, true)
	fastLive(t, 5*time.Millisecond)
	f.server.live.host = &fakeLiveHost{serve: false}
	writes := countWrites(t)
	intake(t, f.server, f.keyA, map[string]any{"kind": "turn_start"})
	waitFor(t, "the first snapshot write", func() bool { return writes.Load() == 1 })
	intake(t, f.server, f.keyA, map[string]any{"kind": "agent", "caption": "b"})
	waitFor(t, "the change's snapshot write", func() bool { return writes.Load() == 2 })
}
