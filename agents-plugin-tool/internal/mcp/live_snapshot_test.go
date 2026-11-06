package mcp

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wslive"
	"github.com/kang-sw/devenv/internal/wsmailbox"
)

// liveFixture is two lead sessions on one root whose launcher loaded a
// package manifest and its dashboard manifest; enabled picks the package
// (ws: dashboard on by default, wsflow: off).
type liveFixture struct {
	root           string
	server         *Server
	keyA, keyB     string
	replyA, replyB string
}

func newLiveFixture(t *testing.T, enabled bool) liveFixture {
	t.Helper()
	setupMailboxTestEnv(t)
	t.Setenv(envMailbox, "")
	t.Setenv(envMailboxAuto, "")
	pkg := dashboardPackages[0]
	if !enabled {
		pkg = dashboardPackages[1]
	}
	t.Setenv(envConfigManifests, pkg.own+";"+pkg.dashboard)
	root := t.TempDir()
	initGit(t, root)
	s := NewServer(root, "test")
	keyA := mailboxLogin(t, s, 1, root)
	keyB := mailboxLogin(t, s, 2, root)
	return liveFixture{root: root, server: s, keyA: keyA, keyB: keyB, replyA: wsmailbox.ReplyID(keyA), replyB: wsmailbox.ReplyID(keyB)}
}

// fastLive shortens the debounce windows and pushes the heartbeat out of a
// test's way.
func fastLive(t *testing.T, debounce time.Duration) {
	t.Helper()
	previous := setLiveSeams(func(s *liveSeams) { s.debounceUrgent, s.debounce, s.heartbeat = debounce, debounce, time.Hour })
	t.Cleanup(func() { setLiveSeams(func(s *liveSeams) { *s = previous }) })
}

func intake(t *testing.T, s *Server, key string, events ...map[string]any) string {
	t.Helper()
	list := make([]any, len(events))
	for i, ev := range events {
		list[i] = ev
	}
	return callToolWithKey(t, s, 9, key, dashboardIntakeTool, map[string]any{"adapter": "pi", "events": list})
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func readLiveRing(t *testing.T, root, replyID string) []wslive.RingRecord {
	t.Helper()
	records, err := wslive.ReadRing(wslive.RingDir(root, replyID))
	if err != nil {
		t.Fatal(err)
	}
	return records
}

// TestDashboardIntakeWritesSnapshotAndRing pins the intake end to end: the
// snapshot lands at the session's live path with the lead root fields and
// the adapter name, finished messages and the turn summary land in the ring,
// a bad event is skipped without failing the call, and the live folder sits
// inside the whiteboard folder's git exclude.
func TestDashboardIntakeWritesSnapshotAndRing(t *testing.T) {
	f := newLiveFixture(t, true)
	fastLive(t, 20*time.Millisecond)

	out := intake(t, f.server, f.keyA,
		map[string]any{"kind": "reset"},
		map[string]any{"kind": "turn_start", "at": 1000, "origin": "whiteboard"},
		map[string]any{"kind": "user", "text": "please check", "origin": "whiteboard"},
		map[string]any{"kind": "agent", "batches": []any{map[string]any{"epoch": "e", "seq": 1, "toolNames": []any{"Read"}, "callCount": 2, "failed": 1, "startedAt": 1100, "endedAt": 1200}}},
		map[string]any{"kind": "agent", "agentId": "sub-1", "label": "explorer"},
		map[string]any{"kind": "bogus"},
	)
	if !strings.HasPrefix(out, "ok: 5 of 6 events applied\n") || !strings.Contains(out, "skipped event 5: unknown event kind") {
		t.Fatalf("intake reply = %q", out)
	}
	if strings.Contains(out, "unread") {
		t.Fatalf("intake reply carries a mailbox badge: %q", out)
	}

	path := wslive.SnapshotPath(f.root, f.replyA)
	waitFor(t, "the snapshot", func() bool { _, err := os.Stat(path); return err == nil })
	snap, err := wslive.ReadSnapshot(path)
	if err != nil {
		t.Fatal(err)
	}
	if snap.ReplyID != f.replyA || snap.Adapter != "pi" || snap.Lead.TurnStartedAt != 1000 || snap.Lead.TurnOrigin != "whiteboard" || snap.Lead.Done != 1 || snap.Lead.Failed != 1 || len(snap.Agents) != 1 {
		t.Fatalf("snapshot = %+v", snap)
	}

	intake(t, f.server, f.keyA, map[string]any{"kind": "reply", "text": "done.", "final": true}, map[string]any{"kind": "turn_end", "at": 4000})
	ring := readLiveRing(t, f.root, f.replyA)
	if len(ring) != 3 || ring[0].Kind != wslive.RingUser || ring[0].Origin != "whiteboard" || ring[1].Kind != wslive.RingReply || ring[1].Text != "done." ||
		ring[2].Kind != wslive.RingTurn || ring[2].DurationMs != 3000 || ring[2].Done != 1 || ring[2].Failed != 1 {
		t.Fatalf("ring = %+v", ring)
	}
	waitFor(t, "the turn end in the snapshot", func() bool {
		snap, err := wslive.ReadSnapshot(path)
		return err == nil && snap.Lead.PreviousTurnMs == 3000 && snap.Lead.Phase == wslive.PhaseIdle
	})

	if !gitIgnored(t, f.root, filepath.Join(wslive.DirRel, f.replyA, wslive.SnapshotName)) {
		t.Fatal("the live snapshot is not git-excluded")
	}
}

func gitIgnored(t *testing.T, root, rel string) bool {
	t.Helper()
	cmd := exec.Command("git", "check-ignore", "-q", rel)
	cmd.Dir = root
	return cmd.Run() == nil
}

// TestDashboardIntakeDebounceCoalescesABurst pins the debounce: a burst of
// intake calls inside one window is one snapshot write.
func TestDashboardIntakeDebounceCoalescesABurst(t *testing.T) {
	f := newLiveFixture(t, true)
	fastLive(t, 300*time.Millisecond)
	var writes atomic.Int32
	orig := currentLiveSeams().writeSnapshot
	setLiveSeams(func(s *liveSeams) {
		s.writeSnapshot = func(path string, data []byte) error { writes.Add(1); return orig(path, data) }
	})

	for i := 0; i < 15; i++ {
		intake(t, f.server, f.keyA, map[string]any{"kind": "reply", "text": strings.Repeat("x", i+1)})
	}
	waitFor(t, "the coalesced write", func() bool { return writes.Load() >= 1 })
	time.Sleep(400 * time.Millisecond)
	if n := writes.Load(); n != 1 {
		t.Fatalf("%d snapshot writes for one burst, want 1", n)
	}
	snap, err := wslive.ReadSnapshot(wslive.SnapshotPath(f.root, f.replyA))
	if err != nil || snap.Lead.Reply == nil || len(snap.Lead.Reply.Text) != 15 {
		t.Fatalf("coalesced snapshot = %+v, %v", snap.Lead.Reply, err)
	}
}

// TestDashboardHeartbeatRefreshesUpdatedAt pins the heartbeat: with no
// change, the snapshot is rewritten with a newer updatedAt, which is what
// keeps an idle session from reading stale.
func TestDashboardHeartbeatRefreshesUpdatedAt(t *testing.T) {
	f := newLiveFixture(t, true)
	fastLive(t, 10*time.Millisecond)
	setLiveSeams(func(s *liveSeams) { s.heartbeat = 50 * time.Millisecond })
	path := wslive.SnapshotPath(f.root, f.replyA)
	intake(t, f.server, f.keyA, map[string]any{"kind": "reset"})
	waitFor(t, "the snapshot", func() bool { _, err := os.Stat(path); return err == nil })
	first, _ := wslive.ReadSnapshot(path)
	waitFor(t, "a heartbeat", func() bool {
		snap, err := wslive.ReadSnapshot(path)
		return err == nil && snap.UpdatedAt > first.UpdatedAt
	})
}

// TestDashboardOffWritesNothing pins the opt-out: with dashboard.enabled
// off, intake calls and a mailbox send and receive leave no snapshot and no
// ring entries.
func TestDashboardOffWritesNothing(t *testing.T) {
	f := newLiveFixture(t, false)
	fastLive(t, 10*time.Millisecond)
	out := intake(t, f.server, f.keyA,
		map[string]any{"kind": "turn_start"},
		map[string]any{"kind": "user", "text": "hi"},
		map[string]any{"kind": "turn_end"},
	)
	if !strings.Contains(out, "dashboard off") {
		t.Fatalf("intake reply = %q", out)
	}
	mustSend(t, f.server, 3, f.keyA, wsmailbox.ReplyIDPrefix+f.replyB, "hello")
	callToolWithKey(t, f.server, 4, f.keyB, "mailbox.recv", nil)
	time.Sleep(100 * time.Millisecond)
	if _, err := os.Stat(filepath.Join(f.root, filepath.FromSlash(wslive.DirRel))); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("dashboard off left a live folder: %v", err)
	}
	if names := historyEntryFiles(t, f.keyA); len(names) != 1 {
		t.Fatalf("mailbox history changed with the dashboard off: %v", names)
	}
}

// TestLiveWriteFailuresNeverFailOrDelayCalls pins resilience: a failing
// snapshot write, a failing ring write, and a nudge that never returns (an
// unreachable daemon) or panics leave intake and mailbox calls succeeding
// promptly.
func TestLiveWriteFailuresNeverFailOrDelayCalls(t *testing.T) {
	f := newLiveFixture(t, true)
	fastLive(t, 10*time.Millisecond)
	orig := currentLiveSeams()

	var snapshotAttempts atomic.Int32
	block := make(chan struct{})
	t.Cleanup(func() { close(block) })
	var nudges atomic.Int32
	setLiveSeams(func(s *liveSeams) {
		s.writeSnapshot = func(string, []byte) error { snapshotAttempts.Add(1); return errors.New("disk full") }
		s.appendRing = func(string, []wslive.RingEntry) error { return errors.New("disk full") }
		s.nudge = func(root, replyID string) { nudges.Add(1); <-block }
	})

	start := time.Now()
	out := intake(t, f.server, f.keyA, map[string]any{"kind": "user", "text": "hi"}, map[string]any{"kind": "turn_start"})
	if !strings.HasPrefix(out, "ok: 2 of 2 events applied") {
		t.Fatalf("intake with failing writes = %q", out)
	}
	mustSend(t, f.server, 3, f.keyA, wsmailbox.ReplyIDPrefix+f.replyB, "hello")
	if got := callToolWithKey(t, f.server, 4, f.keyB, "mailbox.recv", nil); !strings.Contains(got, "hello") {
		t.Fatalf("recv with a failing ring write = %q", got)
	}
	waitFor(t, "a failed snapshot attempt", func() bool { return snapshotAttempts.Load() >= 1 })
	if nudges.Load() != 0 {
		t.Fatal("a failed write sent a nudge")
	}

	// Writes succeed again; the nudge hangs like an unreachable daemon.
	setLiveSeams(func(s *liveSeams) { s.writeSnapshot, s.appendRing = orig.writeSnapshot, orig.appendRing })
	intake(t, f.server, f.keyA, map[string]any{"kind": "user", "text": "again"})
	mustSend(t, f.server, 5, f.keyA, wsmailbox.ReplyIDPrefix+f.replyB, "hello again")
	waitFor(t, "hanging nudges", func() bool { return nudges.Load() >= 2 })
	intake(t, f.server, f.keyA, map[string]any{"kind": "turn_end"})

	var mu sync.Mutex
	panicked := false
	setLiveSeams(func(s *liveSeams) {
		s.nudge = func(string, string) { mu.Lock(); panicked = true; mu.Unlock(); panic("nudge bug") }
	})
	intake(t, f.server, f.keyA, map[string]any{"kind": "user", "text": "panic"})
	waitFor(t, "the panicking nudge", func() bool { mu.Lock(); defer mu.Unlock(); return panicked })
	if elapsed := time.Since(start); elapsed > 2500*time.Millisecond {
		t.Fatalf("calls took %v with hanging nudges", elapsed)
	}
}

// TestDashboardIntakeIsAdapterOnlyAndValidated pins the tool surface: its
// description marks it adapter-only, and missing arguments are refused.
func TestDashboardIntakeIsAdapterOnlyAndValidated(t *testing.T) {
	f := newLiveFixture(t, true)
	var desc string
	for _, tool := range tools() {
		if tool["name"] == dashboardIntakeTool {
			desc, _ = tool["description"].(string)
		}
	}
	if !strings.HasPrefix(desc, "Adapter-only:") {
		t.Fatalf("description = %q", desc)
	}
	if out := callToolWithKey(t, f.server, 3, f.keyA, dashboardIntakeTool, map[string]any{"adapter": "pi", "events": "nope"}); !strings.Contains(out, "events must be an array") {
		t.Fatalf("non-array events = %q", out)
	}
	if out := callToolWithKey(t, f.server, 4, "no-such-key-here", dashboardIntakeTool, map[string]any{"adapter": "pi", "events": []any{}}); !strings.Contains(out, "unknown_session") {
		t.Fatalf("unknown key = %q", out)
	}
}

// TestWhiteboardCollectionReapsDeadLiveFolders pins the daily orphan scan
// over the live folders: a folder whose reply id has no session record goes,
// a live session's stays.
func TestWhiteboardCollectionReapsDeadLiveFolders(t *testing.T) {
	f := newLiveFixture(t, true)
	if err := wslive.AppendRing(wslive.RingDir(f.root, f.replyA), []wslive.RingEntry{{Kind: wslive.RingUser, Text: "x"}}); err != nil {
		t.Fatal(err)
	}
	dead := wslive.Dir(f.root, "deadreplyid")
	if err := os.MkdirAll(dead, 0o755); err != nil {
		t.Fatal(err)
	}
	f.server.maybeCollectWhiteboards(f.root)
	if _, err := os.Stat(dead); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("dead live folder kept: %v", err)
	}
	if _, err := os.Stat(wslive.Dir(f.root, f.replyA)); err != nil {
		t.Fatalf("live session's folder reaped: %v", err)
	}
}
