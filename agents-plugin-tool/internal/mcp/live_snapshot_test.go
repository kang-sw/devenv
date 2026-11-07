package mcp

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wslive"
	"github.com/kang-sw/devenv/internal/wsmailbox"
)

// liveFixture is two lead sessions on one root whose launcher loaded a
// package manifest and its dashboard manifest; enabled picks the package
// (ws: dashboard on by default, wsflow: off).
type liveFixture struct {
	root           string
	stateDir       string
	server         *Server
	keyA, keyB     string
	replyA, replyB string
}

func newLiveFixture(t *testing.T, enabled bool) liveFixture {
	t.Helper()
	setupMailboxTestEnv(t)
	stateDir, err := wsdashboard.Dir("")
	if err != nil {
		t.Fatal(err)
	}
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
	return liveFixture{root: root, stateDir: stateDir, server: s, keyA: keyA, keyB: keyB, replyA: wsmailbox.ReplyID(keyA), replyB: wsmailbox.ReplyID(keyB)}
}

// sessionDir is a session's registry directory, which holds its live stores.
func (f liveFixture) sessionDir(replyID string) string {
	return wsdashboard.SessionDir(f.stateDir, replyID)
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

func readLiveRing(t *testing.T, sessionDir string) []wslive.RingRecord {
	t.Helper()
	records, err := wslive.ReadRing(wslive.RingDir(sessionDir))
	if err != nil {
		t.Fatal(err)
	}
	return records
}

// TestDashboardIntakeWritesSnapshotAndRing pins the intake end to end: the
// snapshot lands at the session's live path with the lead root fields and
// the adapter name, finished messages and the turn summary land in the ring,
// a bad event is skipped without failing the call, and both stores sit in the
// session's registry directory beside its registration.
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

	path := wslive.SnapshotPath(f.sessionDir(f.replyA))
	waitFor(t, "the snapshot", func() bool { _, err := os.Stat(path); return err == nil })
	snap, err := wslive.ReadSnapshot(path)
	if err != nil {
		t.Fatal(err)
	}
	if snap.ReplyID != f.replyA || snap.Adapter != "pi" || snap.Lead.TurnStartedAt != 1000 || snap.Lead.TurnOrigin != "whiteboard" || snap.Lead.Done != 1 || snap.Lead.Failed != 1 || len(snap.Agents) != 1 {
		t.Fatalf("snapshot = %+v", snap)
	}

	intake(t, f.server, f.keyA, map[string]any{"kind": "reply", "text": "done.", "final": true}, map[string]any{"kind": "turn_end", "at": 4000})
	ring := readLiveRing(t, f.sessionDir(f.replyA))
	if len(ring) != 4 || ring[0].Kind != wslive.RingUser || ring[0].Origin != "whiteboard" || ring[1].Kind != wslive.RingTools || ring[1].Tools.Calls != 2 || ring[1].Tools.Failed != 1 ||
		ring[2].Kind != wslive.RingReply || ring[2].Text != "done." ||
		ring[3].Kind != wslive.RingTurn || ring[3].DurationMs != 3000 || ring[3].Done != 1 || ring[3].Failed != 1 {
		t.Fatalf("ring = %+v", ring)
	}
	waitFor(t, "the turn end in the snapshot", func() bool {
		snap, err := wslive.ReadSnapshot(path)
		return err == nil && snap.Lead.PreviousTurnMs == 3000 && snap.Lead.Phase == wslive.PhaseIdle
	})

	if _, ok := readRegistration(t, f.stateDir, f.keyA); !ok {
		t.Fatal("the session wrote live stores without a registration beside them")
	}
}

// TestDashboardIntakeDebounceCoalescesABurst pins the debounce: a burst of
// intake calls inside one window is one snapshot write.
func TestDashboardIntakeDebounceCoalescesABurst(t *testing.T) {
	f := newLiveFixture(t, true)
	const window = 2 * time.Second
	fastLive(t, window)
	var writes atomic.Int32
	orig := currentLiveSeams().writeSnapshot
	setLiveSeams(func(s *liveSeams) {
		s.writeSnapshot = func(path string, data []byte) error { writes.Add(1); return orig(path, data) }
	})

	// The first keyed call registers the session; keep that out of the burst.
	callToolWithKey(t, f.server, 3, f.keyA, "mailbox.lookup_peers", nil)
	started := time.Now()
	for i := 0; i < 15; i++ {
		intake(t, f.server, f.keyA, map[string]any{"kind": "reply", "text": strings.Repeat("x", i+1)})
	}
	if burst := time.Since(started); burst > window/2 {
		t.Skipf("the burst took %v, too long to fit one %v window on this machine", burst, window)
	}
	waitFor(t, "the coalesced write", func() bool { return writes.Load() >= 1 })
	time.Sleep(300 * time.Millisecond)
	if n := writes.Load(); n != 1 {
		t.Fatalf("%d snapshot writes for one burst, want 1", n)
	}
	snap, err := wslive.ReadSnapshot(wslive.SnapshotPath(f.sessionDir(f.replyA)))
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
	path := wslive.SnapshotPath(f.sessionDir(f.replyA))
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
	for _, reply := range []string{f.replyA, f.replyB} {
		for _, path := range []string{wslive.SnapshotPath(f.sessionDir(reply)), wslive.RingDir(f.sessionDir(reply))} {
			if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("dashboard off left %s: %v", path, err)
			}
		}
	}
	if names := historyEntryFiles(t, f.keyA); len(names) != 1 {
		t.Fatalf("mailbox history changed with the dashboard off: %v", names)
	}
}

// TestLiveWriteFailuresNeverFailOrDelayCalls pins resilience: a failing
// snapshot write or ring write, and a daemon that never answers a nudge,
// leave intake and mailbox calls succeeding promptly, and the stores recover
// once writes succeed again.
func TestLiveWriteFailuresNeverFailOrDelayCalls(t *testing.T) {
	f := newLiveFixture(t, true)
	fastLive(t, 10*time.Millisecond)
	startFakeDaemon(t, f.stateDir, 0, true)
	orig := currentLiveSeams()

	var snapshotAttempts atomic.Int32
	setLiveSeams(func(s *liveSeams) {
		s.writeSnapshot = func(string, []byte) error { snapshotAttempts.Add(1); return errors.New("disk full") }
		s.appendRing = func(string, []wslive.RingEntry) error { return errors.New("disk full") }
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

	// Writes succeed again; every nudge now meets a daemon that hangs.
	setLiveSeams(func(s *liveSeams) { s.writeSnapshot, s.appendRing = orig.writeSnapshot, orig.appendRing })
	intake(t, f.server, f.keyA, map[string]any{"kind": "user", "text": "again"})
	mustSend(t, f.server, 5, f.keyA, wsmailbox.ReplyIDPrefix+f.replyB, "hello again")
	callToolWithKey(t, f.server, 6, f.keyB, "mailbox.recv", nil)
	intake(t, f.server, f.keyA, map[string]any{"kind": "turn_end"})
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("calls took %v with a hanging daemon", elapsed)
	}
	if ring := readLiveRing(t, f.sessionDir(f.replyA)); len(ring) != 3 {
		t.Fatalf("ring after recovery = %+v", ring)
	}
}

// TestLiveWritesNudgeTheDaemon pins the nudge wiring: a snapshot write and
// a ring write (from intake or from the mailbox path) each nudge the daemon
// with the session's reply-id, after the nudge its registration sent.
func TestLiveWritesNudgeTheDaemon(t *testing.T) {
	f := newLiveFixture(t, true)
	fastLive(t, 10*time.Millisecond)
	fake := startFakeDaemon(t, f.stateDir, http.StatusNoContent, false)
	count := func() int {
		n := 0
		for _, body := range fake.received() {
			if body.ReplyID == f.replyA {
				n++
			}
		}
		return n
	}
	callToolWithKey(t, f.server, 3, f.keyA, "mailbox.lookup_peers", nil)
	waitForNudge(t, fake, f.replyA) // the registration's
	time.Sleep(50 * time.Millisecond)

	base := count()
	intake(t, f.server, f.keyA, map[string]any{"kind": "turn_start"})
	waitFor(t, "the snapshot write's nudge", func() bool { return count() > base })
	time.Sleep(50 * time.Millisecond)

	base = count()
	mustSend(t, f.server, 4, f.keyA, wsmailbox.ReplyIDPrefix+f.replyB, "hello")
	waitFor(t, "the mail line's nudge", func() bool { return count() > base })
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

// TestDashboardSessionSourceTracksLiveStores pins what the daemon's session
// index reads from the live stores: the snapshot's updatedAt as its age, and
// a fingerprint that a snapshot change, a ring entry, or the snapshot going
// stale changes while a heartbeat rewrite does not.
func TestDashboardSessionSourceTracksLiveStores(t *testing.T) {
	f := newLiveFixture(t, true)
	fastLive(t, 10*time.Millisecond)
	path := wslive.SnapshotPath(f.sessionDir(f.replyA))
	intake(t, f.server, f.keyA, map[string]any{"kind": "reset"})
	waitFor(t, "the snapshot", func() bool { _, err := os.Stat(path); return err == nil })

	clock := time.Now()
	src := &DashboardSessionSource{store: newSessionStore(), stateDir: func() (string, error) { return f.stateDir, nil }, now: func() time.Time { return clock }}
	reg, ok := readRegistration(t, f.stateDir, f.keyA)
	if !ok {
		t.Fatal("no registration")
	}
	read := func() wsdashboard.SessionData {
		t.Helper()
		data, ok := src.ReadSession(reg)
		if !ok {
			t.Fatal("ReadSession found no session")
		}
		return data
	}
	rewrite := func(change func(map[string]any)) {
		t.Helper()
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var doc map[string]any
		if err := json.Unmarshal(raw, &doc); err != nil {
			t.Fatal(err)
		}
		change(doc)
		raw, _ = json.Marshal(doc)
		if err := os.WriteFile(path, raw, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	first := read()
	snap, _ := wslive.ReadSnapshot(path)
	if !first.SnapshotUpdatedAt.Equal(time.UnixMilli(snap.UpdatedAt)) {
		t.Fatalf("age = %v, want %v", first.SnapshotUpdatedAt, time.UnixMilli(snap.UpdatedAt))
	}

	rewrite(func(doc map[string]any) { doc["updatedAt"] = float64(snap.UpdatedAt + 1000) })
	beat := read()
	if beat.Fingerprint != first.Fingerprint || !beat.SnapshotUpdatedAt.Equal(time.UnixMilli(snap.UpdatedAt+1000)) {
		t.Fatalf("a heartbeat rewrite: %+v vs %+v", beat, first)
	}

	rewrite(func(doc map[string]any) { doc["lead"] = map[string]any{"phase": "thinking"} })
	changed := read()
	if changed.Fingerprint == beat.Fingerprint {
		t.Fatal("a snapshot change kept the fingerprint")
	}

	if err := wslive.AppendRing(wslive.RingDir(f.sessionDir(f.replyA)), []wslive.RingEntry{{Kind: wslive.RingUser, Text: "x"}}); err != nil {
		t.Fatal(err)
	}
	ringed := read()
	if ringed.Fingerprint == changed.Fingerprint {
		t.Fatal("a ring entry kept the fingerprint")
	}

	clock = clock.Add(time.Hour)
	if stale := read(); stale.Fingerprint == ringed.Fingerprint {
		t.Fatal("the snapshot going stale kept the fingerprint")
	}
}

// TestDashboardTurnedOffStopsTheHeartbeat pins the key-off rule for a
// session that was on: once its dashboard.enabled turns off, the heartbeat
// writes nothing more, and an intake call writes nothing either.
func TestDashboardTurnedOffStopsTheHeartbeat(t *testing.T) {
	f := newLiveFixture(t, true)
	fastLive(t, 10*time.Millisecond)
	setLiveSeams(func(s *liveSeams) { s.heartbeat = 30 * time.Millisecond })
	var writes atomic.Int32
	orig := currentLiveSeams().writeSnapshot
	setLiveSeams(func(s *liveSeams) {
		s.writeSnapshot = func(path string, data []byte) error { writes.Add(1); return orig(path, data) }
	})
	intake(t, f.server, f.keyA, map[string]any{"kind": "reset"})
	waitFor(t, "heartbeat writes", func() bool { return writes.Load() >= 2 })

	resp := dashCall(t, f.server, "config.tune", map[string]any{"session_key": f.keyA, "key": DashboardEnabledKey, "value": "false", "scope": "session"})
	if isToolError(resp) {
		t.Fatalf("config.tune: %s", resp)
	}
	time.Sleep(100 * time.Millisecond) // a heartbeat at most is in flight
	settled := writes.Load()
	time.Sleep(200 * time.Millisecond)
	if n := writes.Load(); n != settled {
		t.Fatalf("%d snapshot writes after the key turned off", n-settled)
	}
	if out := intake(t, f.server, f.keyA, map[string]any{"kind": "turn_start"}); !strings.Contains(out, "dashboard off") {
		t.Fatalf("intake after the key turned off = %q", out)
	}
	f.server.live.mu.Lock()
	_, kept := f.server.live.sessions[f.keyA]
	f.server.live.mu.Unlock()
	if kept {
		t.Fatal("the stopped session stayed in the hub")
	}
}

// TestDashboardResolveFailureKeepsTheLiveSession pins the cached key: a
// transient dashboard.enabled resolve failure keeps the last known value, so
// the session is neither stopped nor rebuilt empty (its turn start survives),
// while a session never resolved stays off until a resolve succeeds.
func TestDashboardResolveFailureKeepsTheLiveSession(t *testing.T) {
	f := newLiveFixture(t, true)
	fastLive(t, 10*time.Millisecond)
	intake(t, f.server, f.keyA, map[string]any{"kind": "reset"}, map[string]any{"kind": "turn_start", "at": 1000})
	f.server.live.mu.Lock()
	before := f.server.live.sessions[f.keyA]
	f.server.live.mu.Unlock()

	var resolves atomic.Int32
	f.server.live.enabledMu.Lock()
	f.server.live.resolveEnabled = func(string) (bool, error) {
		resolves.Add(1)
		return false, errors.New("transient")
	}
	// Age the cached value past its recheck window.
	for key, c := range f.server.live.enabled {
		c.at = c.at.Add(-2 * dashboardRecheck)
		f.server.live.enabled[key] = c
	}
	f.server.live.enabledMu.Unlock()

	if out := intake(t, f.server, f.keyA, map[string]any{"kind": "agent", "caption": "still here"}); strings.Contains(out, "dashboard off") {
		t.Fatalf("intake after a failed resolve = %q", out)
	}
	if resolves.Load() == 0 {
		t.Fatal("the aged value was not resolved again")
	}
	f.server.live.mu.Lock()
	after := f.server.live.sessions[f.keyA]
	f.server.live.mu.Unlock()
	if after != before {
		t.Fatal("a failed resolve replaced the live session")
	}
	path := wslive.SnapshotPath(f.sessionDir(f.replyA))
	waitFor(t, "the snapshot after the failed resolve", func() bool {
		snap, err := wslive.ReadSnapshot(path)
		return err == nil && snap.Lead.TurnStartedAt == 1000
	})

	if out := intake(t, f.server, f.keyB, map[string]any{"kind": "reset"}); !strings.Contains(out, "dashboard off") {
		t.Fatalf("never-resolved session with a failing resolve = %q", out)
	}
}

// TestDashboardIntakeReportsAcceptedKindsAndStatus pins the support report
// and the status kind end to end: an intake result names the optional kinds
// this ws-mcp accepts, the "dashboard off" result names none, and a status
// event's figures land in the snapshot's usage.
func TestDashboardIntakeReportsAcceptedKindsAndStatus(t *testing.T) {
	f := newLiveFixture(t, true)
	fastLive(t, 10*time.Millisecond)
	out := intake(t, f.server, f.keyA, map[string]any{"kind": "status",
		"context": map[string]any{"tokens": 50_000, "window": 200_000, "cacheHit": 91.5},
		"limits":  []any{map[string]any{"kind": "five_hour", "percent": 42, "resetsAt": 1_800_000_000_000}},
		"cost":    map[string]any{"usd": 1.25, "estimate": true}})
	if !strings.Contains(out, "ok: 1 of 1 events applied\n") || !strings.Contains(out, "\naccepts: status, compact\n") {
		t.Fatalf("intake reply = %q, want an accepts: status, compact line", out)
	}
	path := wslive.SnapshotPath(f.sessionDir(f.replyA))
	waitFor(t, "the usage in the snapshot", func() bool {
		snap, err := wslive.ReadSnapshot(path)
		return err == nil && snap.Usage != nil && snap.Usage.Context != nil && *snap.Usage.Context.Percent == 25 &&
			len(snap.Usage.Limits) == 1 && snap.Usage.Cost != nil && snap.Usage.Cost.Estimate
	})

	off := newLiveFixture(t, false)
	if out := intake(t, off.server, off.keyA, map[string]any{"kind": "reset"}); !strings.Contains(out, "dashboard off") || strings.Contains(out, "accepts:") {
		t.Fatalf("dashboard off reply = %q, want no accepts line", out)
	}
}

// TestDashboardIntakeCompaction pins the compact kind end to end: a start
// shows the lead compacting in the snapshot, and an end clears it and
// writes one compaction line to the ring.
func TestDashboardIntakeCompaction(t *testing.T) {
	f := newLiveFixture(t, true)
	fastLive(t, 10*time.Millisecond)
	out := intake(t, f.server, f.keyA, map[string]any{"kind": "compact", "phase": "start", "trigger": "auto"})
	if !strings.Contains(out, "ok: 1 of 1 events applied\n") {
		t.Fatalf("start reply = %q", out)
	}
	path := wslive.SnapshotPath(f.sessionDir(f.replyA))
	waitFor(t, "the lead compacting in the snapshot", func() bool {
		snap, err := wslive.ReadSnapshot(path)
		return err == nil && snap.Lead.Phase == wslive.PhaseCompacting
	})
	if out := intake(t, f.server, f.keyA, map[string]any{"kind": "compact", "phase": "end"}); !strings.Contains(out, "ok: 1 of 1 events applied\n") {
		t.Fatalf("end reply = %q", out)
	}
	waitFor(t, "the lead idle after the compaction", func() bool {
		snap, err := wslive.ReadSnapshot(path)
		return err == nil && snap.Lead.Phase == wslive.PhaseIdle
	})
	records, err := wslive.ReadRing(wslive.RingDir(f.sessionDir(f.replyA)))
	if err != nil || len(records) != 1 || records[0].Kind != wslive.RingCompact || records[0].Trigger != wslive.TriggerAuto || records[0].CompactFailed {
		t.Fatalf("ring = %+v (%v), want one auto compaction line", records, err)
	}
}

// TestDashboardSnapshotRepoFields pins the repo fields ws-mcp reads for the
// session root: a repository without an upstream shows its branch with no
// upstream, one with an upstream shows the upstream and its standing, and a
// root that is not a repository shows the folder alone.
func TestDashboardSnapshotRepoFields(t *testing.T) {
	base := t.TempDir()
	origin := filepath.Join(base, "origin")
	work := filepath.Join(base, "work")
	gitCmd(t, base, "init", "-q", "-b", "main", origin)
	gitCmd(t, origin, "commit", "-q", "--allow-empty", "-m", "a")
	gitCmd(t, base, "clone", "-q", origin, work)
	gitCmd(t, work, "commit", "-q", "--allow-empty", "-m", "b")

	repo := readSessionRepo(work)
	if repo.Folder != "work" || repo.Branch != "main" || repo.Upstream != "origin/main" || repo.Ahead != 1 || repo.Behind != 0 || repo.Detached {
		t.Fatalf("with upstream: %+v", repo)
	}
	gitCmd(t, work, "checkout", "-q", "-b", "topic")
	if repo = readSessionRepo(work); repo.Branch != "topic" || repo.Upstream != "" || repo.Ahead != 0 {
		t.Fatalf("no upstream: %+v", repo)
	}
	if repo.Modified != 0 || repo.Staged != 0 || repo.Untracked != 0 {
		t.Fatalf("clean tree has counts: %+v", repo)
	}
	writeFile := func(name string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(work, name), []byte(name), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	writeFile("tracked")
	gitCmd(t, work, "add", "tracked")
	gitCmd(t, work, "commit", "-q", "-m", "tracked")
	writeFile("tracked") // same bytes: still clean
	if repo = readSessionRepo(work); repo.Modified != 0 {
		t.Fatalf("rewritten identical file counts: %+v", repo)
	}
	if err := os.WriteFile(filepath.Join(work, "tracked"), []byte("changed"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeFile("staged")
	gitCmd(t, work, "add", "staged")
	writeFile("loose")
	if repo = readSessionRepo(work); repo.Modified != 1 || repo.Staged != 1 || repo.Untracked != 1 || repo.Branch != "topic" {
		t.Fatalf("dirty tree: %+v, want 1 modified, 1 staged, 1 untracked on topic", repo)
	}
	plain := filepath.Join(base, "plain")
	if err := os.Mkdir(plain, 0o755); err != nil {
		t.Fatal(err)
	}
	if repo = readSessionRepo(plain); repo != (wslive.Repo{Folder: "plain"}) {
		t.Fatalf("not a repository: %+v", repo)
	}

	// End to end: the fixture's root is a fresh repository with no commit,
	// so its snapshot carries the folder alone.
	f := newLiveFixture(t, true)
	fastLive(t, 10*time.Millisecond)
	intake(t, f.server, f.keyA, map[string]any{"kind": "reset"})
	path := wslive.SnapshotPath(f.sessionDir(f.replyA))
	waitFor(t, "the repo in the snapshot", func() bool {
		snap, err := wslive.ReadSnapshot(path)
		return err == nil && snap.Repo != nil && snap.Repo.Folder == filepath.Base(f.root)
	})
}

// TestDashboardRepoReadOncePerHeartbeat pins the git cost: change snapshots
// inside one heartbeat interval reuse the cached repo read, and only a
// write a heartbeat interval later reads again.
func TestDashboardRepoReadOncePerHeartbeat(t *testing.T) {
	f := newLiveFixture(t, true)
	fastLive(t, 5*time.Millisecond)
	var reads, writes atomic.Int32
	orig := currentLiveSeams()
	setLiveSeams(func(s *liveSeams) {
		s.readRepo = func(root string) wslive.Repo {
			reads.Add(1)
			return wslive.Repo{Folder: filepath.Base(root), Branch: "main"}
		}
		s.writeSnapshot = func(path string, data []byte) error { writes.Add(1); return orig.writeSnapshot(path, data) }
	})
	for i := 0; i < 4; i++ {
		intake(t, f.server, f.keyA, map[string]any{"kind": "reply", "text": strings.Repeat("y", i+1)})
		target := int32(i + 1)
		waitFor(t, "a change snapshot", func() bool { return writes.Load() >= target })
	}
	if n := reads.Load(); n != 1 {
		t.Fatalf("%d repo reads over %d change snapshots in one heartbeat interval, want 1", n, writes.Load())
	}
	snap, err := wslive.ReadSnapshot(wslive.SnapshotPath(f.sessionDir(f.replyA)))
	if err != nil || snap.Repo == nil || snap.Repo.Branch != "main" {
		t.Fatalf("snapshot repo = %+v, %v", snap.Repo, err)
	}

	// Once the cached read is a heartbeat interval old, the next write reads again.
	setLiveSeams(func(s *liveSeams) { s.heartbeat = 40 * time.Millisecond })
	time.Sleep(60 * time.Millisecond)
	intake(t, f.server, f.keyA, map[string]any{"kind": "reply", "text": "z"})
	waitFor(t, "a refreshed repo read", func() bool { return reads.Load() >= 2 })
}

func gitCmd(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, out)
	}
}
