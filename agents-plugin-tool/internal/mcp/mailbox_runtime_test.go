package mcp

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsmailbox"
)

// TestReapStaleReplyIDsRemovesOnlyStaleEmptyEntries verifies Decision 11's
// lazy-expiry reaping: an entry past mailboxReplyIDRetention with an empty
// queue is removed; a stale entry with a NON-empty queue is kept
// (undelivered mail is never dropped by reaping); a fresh entry is kept
// regardless of its queue state.
func TestReapStaleReplyIDsRemovesOnlyStaleEmptyEntries(t *testing.T) {
	now := time.Now().UTC()
	stale := now.Add(-mailboxReplyIDRetention - time.Hour).Format(time.RFC3339)
	fresh := now.Add(-time.Minute).Format(time.RFC3339)

	store := &wsmailbox.ReplyStore{
		Entries: map[string]wsmailbox.ReplyEntry{
			"stale-empty":     {LastSeen: stale},
			"stale-with-mail": {LastSeen: stale},
			"fresh-empty":     {LastSeen: fresh},
		},
		Queues: map[string][]wsmailbox.Envelope{
			"stale-with-mail": {{Content: "still here", SentAt: fresh}},
		},
	}

	reapStaleReplyIDs(store, now)

	if _, ok := store.Entries["stale-empty"]; ok {
		t.Fatalf("stale entry with an empty queue was not reaped")
	}
	if _, ok := store.Entries["stale-with-mail"]; !ok {
		t.Fatalf("stale entry with a non-empty queue was incorrectly reaped, risking mail loss")
	}
	if _, ok := store.Entries["fresh-empty"]; !ok {
		t.Fatalf("fresh entry was incorrectly reaped")
	}
	if len(store.Queues["stale-with-mail"]) != 1 {
		t.Fatalf("reaping must never touch queue contents: %#v", store.Queues["stale-with-mail"])
	}
}

// mailboxPresenceLive's restart-false-positive fix delegates its actual
// process-liveness syscall logic to wsstate.ProcessAlive (see
// mailbox_runtime.go's doc comment on mailboxPresenceLive for why: a
// hand-rolled probe here previously had a platform-specific bug this
// package cannot re-introduce without noticing). That primitive's own
// liveness behavior — including the exited-PID case this fix depends on —
// is exercised directly in internal/wsstate's
// TestProcessAliveDetectsLiveAndExitedProcess, not duplicated here.

// driveMailboxPresenceTicks runs s's presence ticker loop against a manual
// tick channel, delivers n ticks, then cancels and waits for the loop to
// exit. An unbuffered send completes only once the loop has received the
// tick, and the loop finishes that tick's write before it can observe the
// cancellation, so every tick's write is visible when this returns — no
// wall-clock waits.
func driveMailboxPresenceTicks(t *testing.T, s *Server, n int) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	tick := make(chan time.Time)
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.runMailboxPresenceTicker(ctx, tick)
	}()
	for i := 0; i < n; i++ {
		tick <- time.Time{}
	}
	cancel()
	<-done
}

// advanceMailboxClock moves mailboxNow forward by d for the rest of the test.
func advanceMailboxClock(t *testing.T, d time.Duration) time.Time {
	t.Helper()
	original := mailboxNow
	t.Cleanup(func() { mailboxNow = original })
	later := original().Add(d)
	mailboxNow = func() time.Time { return later }
	return later
}

// exitedChildPID runs a short-lived child process to completion and returns
// its PID, a PID wsstate.ProcessAlive reports dead: the fixture for a
// presence record left behind by a departed holder.
func exitedChildPID(t *testing.T) int {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to run a short-lived child process: %v", err)
	}
	if cmd.ProcessState == nil || !cmd.ProcessState.Exited() {
		t.Fatalf("child process did not reach an Exited state: %v", cmd.ProcessState)
	}
	pid := cmd.Process.Pid
	if pid == os.Getpid() {
		t.Fatalf("exited child reused this process's PID %d", pid)
	}
	return pid
}

func loadPresence(t *testing.T, path, name string) wsmailbox.Presence {
	t.Helper()
	store, err := wsmailbox.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	p, ok := store.Presence[name]
	if !ok {
		t.Fatalf("no presence record for %q in %s", name, path)
	}
	return p
}

// TestMailboxPresenceTickerRefreshesIdleOwner verifies an identity-holding
// server keeps its presence LastSeen fresh with no tool calls at all: the
// idle owner of an armed mailbox wait must not read as dead to lookup_peers.
func TestMailboxPresenceTickerRefreshesIdleOwner(t *testing.T) {
	setupMailboxTestEnv(t)
	t.Setenv(envMailbox, "idle@machine")
	s := NewServer(t.TempDir(), "test")
	s.ensureMailboxRegistered("")

	path, err := wsmailbox.MachinePath()
	if err != nil {
		t.Fatal(err)
	}
	later := advanceMailboxClock(t, mailboxLivenessThreshold-time.Minute)
	driveMailboxPresenceTicks(t, s, 1)

	if got, want := loadPresence(t, path, "idle").LastSeen, later.Format(time.RFC3339); got != want {
		t.Fatalf("ticker did not refresh an idle owner's LastSeen: got %q, want %q", got, want)
	}
}

// TestMailboxPresenceTickerUsesRegisteredRoot verifies a worktree/clone
// identity heartbeats the store under the root it registered with, not the
// process-level Server.root (unreliable for plugin-managed launches).
func TestMailboxPresenceTickerUsesRegisteredRoot(t *testing.T) {
	for _, tc := range []struct {
		scope string
		path  func(string) (string, error)
	}{
		{"worktree", wsmailbox.WorktreePath},
		{"clone", wsmailbox.ClonePath},
	} {
		t.Run(tc.scope, func(t *testing.T) {
			setupMailboxTestEnv(t)
			root := t.TempDir()
			initGit(t, root)
			t.Setenv(envMailbox, "scoped@"+tc.scope)
			s := NewServer(t.TempDir(), "test")
			s.ensureMailboxRegistered(root)

			path, err := tc.path(root)
			if err != nil {
				t.Fatal(err)
			}
			later := advanceMailboxClock(t, 5*time.Minute)
			driveMailboxPresenceTicks(t, s, 1)

			if got, want := loadPresence(t, path, "scoped").LastSeen, later.Format(time.RFC3339); got != want {
				t.Fatalf("ticker did not refresh LastSeen under the registered root: got %q, want %q", got, want)
			}
		})
	}
}

// TestMailboxPresenceTickerSkipsBeforeRegistration verifies a worktree
// identity whose owning login has not registered it yet writes nothing: the
// ticker has no root to resolve its store from.
func TestMailboxPresenceTickerSkipsBeforeRegistration(t *testing.T) {
	setupMailboxTestEnv(t)
	root := t.TempDir()
	initGit(t, root)
	t.Setenv(envMailbox, "early@worktree")
	s := NewServer(root, "test")
	s.ensureMailboxRegistered("") // root-less: deferred, not registered

	driveMailboxPresenceTicks(t, s, 1)

	path, err := wsmailbox.WorktreePath(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("ticker touched the worktree store before registration: %s", path)
	}
}

// TestMailboxPresenceTickerNeverRefreshesAnotherPIDsRecord verifies the
// never-resurrect rule: once a different process holds the record (this
// process lost the name), the ticker leaves its LastSeen alone.
func TestMailboxPresenceTickerNeverRefreshesAnotherPIDsRecord(t *testing.T) {
	setupMailboxTestEnv(t)
	root := t.TempDir()
	initGit(t, root)
	t.Setenv(envMailbox, "taken@worktree")
	s := NewServer(root, "test")
	s.ensureMailboxRegistered(root)

	path, err := wsmailbox.WorktreePath(root)
	if err != nil {
		t.Fatal(err)
	}
	otherPID := os.Getppid()
	if otherPID == os.Getpid() || otherPID <= 0 {
		t.Skipf("cannot obtain a distinct parent PID (ppid=%d)", otherPID)
	}
	stamped := mailboxNowString()
	if err := wsmailbox.WithLock(path, func(store *wsmailbox.StoreFile) error {
		p := store.Presence["taken"]
		p.PID = otherPID
		p.LastSeen = stamped
		store.Presence["taken"] = p
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	advanceMailboxClock(t, 5*time.Minute)
	driveMailboxPresenceTicks(t, s, 1)

	if got := loadPresence(t, path, "taken").LastSeen; got != stamped {
		t.Fatalf("ticker refreshed a record another PID holds: LastSeen %q, want untouched %q", got, stamped)
	}
}

// TestServeStdioStopsMailboxPresenceTicker verifies the ticker's lifetime is
// bounded by ServeStdio: the ticker is stopped, and its goroutine has
// exited, before ServeStdio returns. A mailbox-inert server starts none.
func TestServeStdioStopsMailboxPresenceTicker(t *testing.T) {
	setupMailboxTestEnv(t)
	var started, stopped int
	original := newMailboxPresenceTicker
	t.Cleanup(func() { newMailboxPresenceTicker = original })
	newMailboxPresenceTicker = func() (<-chan time.Time, func()) {
		started++
		return make(chan time.Time), func() { stopped++ }
	}

	t.Setenv(envMailbox, "")
	t.Setenv(envMailboxAuto, "")
	if err := NewServer(t.TempDir(), "test").ServeStdio(context.Background(), strings.NewReader(""), &bytes.Buffer{}); err != nil {
		t.Fatalf("ServeStdio returned error: %v", err)
	}
	if started != 0 {
		t.Fatalf("mailbox-inert server started a presence ticker")
	}

	t.Setenv(envMailbox, "serving@machine")
	input := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}` + "\n"
	if err := NewServer(t.TempDir(), "test").ServeStdio(context.Background(), strings.NewReader(input), &bytes.Buffer{}); err != nil {
		t.Fatalf("ServeStdio returned error: %v", err)
	}
	// The ticker goroutine's deferred stop runs before it signals done, and
	// ServeStdio waits for done, so these reads need no synchronization.
	if started != 1 || stopped != 1 {
		t.Fatalf("presence ticker not bounded by ServeStdio: started=%d stopped=%d, want 1/1", started, stopped)
	}
}

// TestServeStdioStopsMailboxPresenceTickerBeforeHandlersDrain verifies the
// ticker stops at EOF (client gone) without waiting for in-flight handlers:
// the handler below blocks until the ticker is stopped, so ServeStdio would
// only return via the handler's safety timeout if the stop waited on it.
func TestServeStdioStopsMailboxPresenceTickerBeforeHandlersDrain(t *testing.T) {
	setupMailboxTestEnv(t)
	t.Setenv(envMailbox, "draining@machine")
	stopped := make(chan struct{})
	original := newMailboxPresenceTicker
	t.Cleanup(func() { newMailboxPresenceTicker = original })
	newMailboxPresenceTicker = func() (<-chan time.Time, func()) {
		return make(chan time.Time), func() { close(stopped) }
	}
	handlerSawStop := make(chan bool, 1)
	testPanicHook = func(name string) {
		if name != "runtime.read" {
			return
		}
		select {
		case <-stopped:
			handlerSawStop <- true
		case <-time.After(10 * time.Second):
			handlerSawStop <- false
		}
	}
	t.Cleanup(func() { testPanicHook = nil })

	input := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"runtime.read","arguments":{}}}` + "\n"
	if err := NewServer(t.TempDir(), "test").ServeStdio(context.Background(), strings.NewReader(input), &bytes.Buffer{}); err != nil {
		t.Fatalf("ServeStdio returned error: %v", err)
	}
	select {
	case saw := <-handlerSawStop:
		if !saw {
			t.Fatalf("presence ticker kept running while an in-flight handler drained after EOF")
		}
	case <-time.After(15 * time.Second):
		t.Fatalf("runtime.read handler hook never ran; the test no longer observes the in-flight handler")
	}
}

// TestMailboxPresenceTickerFollowsRebuiltRecord verifies the ticker follows
// the record the process holds now, not the one it registered: after a
// parent-less lead rebind rebuilds a lost record under this process's PID,
// ticks keep that record fresh.
func TestMailboxPresenceTickerFollowsRebuiltRecord(t *testing.T) {
	setupMailboxTestEnv(t)
	root := t.TempDir()
	initGit(t, root)
	t.Setenv(envMailbox, "rebuilt@worktree")
	s := NewServer(root, "test")
	s.ensureMailboxRegistered(root)

	path, err := wsmailbox.WorktreePath(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := wsmailbox.WithLock(path, func(store *wsmailbox.StoreFile) error {
		delete(store.Presence, "rebuilt")
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	s.rebindMailboxOwnerAtFerrule("owner-key", "", roleLead, root)
	if got := loadPresence(t, path, "rebuilt").PID; got != os.Getpid() {
		t.Fatalf("rebind did not rebuild the record under this process: PID %d", got)
	}

	later := advanceMailboxClock(t, 5*time.Minute)
	driveMailboxPresenceTicks(t, s, 1)

	if got, want := loadPresence(t, path, "rebuilt").LastSeen, later.Format(time.RFC3339); got != want {
		t.Fatalf("ticker did not refresh the rebuilt record: got %q, want %q", got, want)
	}
}

// TestMailboxPresenceTickerFollowsLatestLeadRebindRoot verifies the ticker
// heartbeats the store of the most recent successful lead rebind: a
// worktree identity registered under root A whose lead re-logs in under root
// B holds its record in B's store, and a tick must keep that record fresh
// rather than keep refreshing A's.
func TestMailboxPresenceTickerFollowsLatestLeadRebindRoot(t *testing.T) {
	setupMailboxTestEnv(t)
	rootA, rootB := t.TempDir(), t.TempDir()
	initGit(t, rootA)
	initGit(t, rootB)
	t.Setenv(envMailbox, "roaming@worktree")
	s := NewServer(rootA, "test")
	s.ensureMailboxRegistered(rootA)
	s.rebindMailboxOwnerAtFerrule("lead-key", "", roleLead, rootB)

	pathB, err := wsmailbox.WorktreePath(rootB)
	if err != nil {
		t.Fatal(err)
	}
	if got := loadPresence(t, pathB, "roaming"); got.PID != os.Getpid() || got.Owner != "lead-key" {
		t.Fatalf("rebind under root B did not bind this process: PID %d, owner %q", got.PID, got.Owner)
	}

	later := advanceMailboxClock(t, 5*time.Minute)
	driveMailboxPresenceTicks(t, s, 1)

	if got, want := loadPresence(t, pathB, "roaming").LastSeen, later.Format(time.RFC3339); got != want {
		t.Fatalf("ticker did not refresh the record under the latest rebind root: got %q, want %q", got, want)
	}
}

// TestMailboxPresenceTickerBypassesHeartbeatThrottle verifies a tick writes
// even when a tool call has just claimed the heartbeat throttle window: a
// tick routed through the throttled refresh would be skipped whenever
// root-less tool calls keep claiming the window without writing.
func TestMailboxPresenceTickerBypassesHeartbeatThrottle(t *testing.T) {
	setupMailboxTestEnv(t)
	t.Setenv(envMailbox, "busy@machine")
	s := NewServer(t.TempDir(), "test")
	s.ensureMailboxRegistered("")

	path, err := wsmailbox.MachinePath()
	if err != nil {
		t.Fatal(err)
	}
	if !s.mailboxHeartbeatDue() {
		t.Fatalf("fixture: heartbeat throttle window was already claimed")
	}
	later := advanceMailboxClock(t, mailboxHeartbeatThrottle/2)
	driveMailboxPresenceTicks(t, s, 1)

	if got, want := loadPresence(t, path, "busy").LastSeen, later.Format(time.RFC3339); got != want {
		t.Fatalf("ticker honored the heartbeat throttle: LastSeen %q, want %q", got, want)
	}
}

// TestMailboxPresenceTickerRefreshesReclaimedDeadPIDRecord verifies the
// ticker keeps a reclaimed record fresh: a record left by a different, exited
// process with a stale LastSeen is taken over by a parent-less lead rebind,
// which rewrites its PID to this process, and the following tick refreshes
// it. The ticker only writes a record carrying this process's PID, so without
// that rewrite the reclaimed inbox would go stale while its owner sits idle.
func TestMailboxPresenceTickerRefreshesReclaimedDeadPIDRecord(t *testing.T) {
	setupMailboxTestEnv(t)
	root := t.TempDir()
	initGit(t, root)
	t.Setenv(envMailbox, "reclaimed@worktree")
	s := NewServer(root, "test")
	s.ensureMailboxRegistered(root)

	deadPID := exitedChildPID(t)

	path, err := wsmailbox.WorktreePath(root)
	if err != nil {
		t.Fatal(err)
	}
	stale := mailboxNow().Add(-(mailboxLivenessThreshold + time.Minute)).Format(time.RFC3339)
	if err := wsmailbox.WithLock(path, func(store *wsmailbox.StoreFile) error {
		p := store.Presence["reclaimed"]
		p.PID = deadPID
		p.Owner = "departed-child-key"
		p.LastSeen = stale
		store.Presence["reclaimed"] = p
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	s.rebindMailboxOwnerAtFerrule("lead-key", "", roleLead, root)
	if got := loadPresence(t, path, "reclaimed"); got.Owner != "lead-key" || got.PID != os.Getpid() {
		t.Fatalf("lead rebind did not reclaim the dead-PID record: owner = %q, PID = %d; want owner %q, PID %d", got.Owner, got.PID, "lead-key", os.Getpid())
	}

	later := advanceMailboxClock(t, 5*time.Minute)
	driveMailboxPresenceTicks(t, s, 1)

	if got, want := loadPresence(t, path, "reclaimed").LastSeen, later.Format(time.RFC3339); got != want {
		t.Fatalf("ticker did not refresh the reclaimed record: got %q, want %q", got, want)
	}
}

// TestMailboxPresenceTickerSkipsAfterRegistrationConflict drives a real
// duplicate-live-name conflict: the losing process is registered (its
// ticker is armed) but must never refresh the winner's record.
func TestMailboxPresenceTickerSkipsAfterRegistrationConflict(t *testing.T) {
	setupMailboxTestEnv(t)
	root := t.TempDir()
	initGit(t, root)
	path, err := wsmailbox.WorktreePath(root)
	if err != nil {
		t.Fatal(err)
	}
	otherPID := os.Getppid()
	if otherPID == os.Getpid() || otherPID <= 0 {
		t.Skipf("cannot obtain a distinct parent PID (ppid=%d)", otherPID)
	}
	stamped := mailboxNowString()
	if err := wsmailbox.WithLock(path, func(store *wsmailbox.StoreFile) error {
		store.Presence["contested"] = wsmailbox.Presence{Name: "contested", Scope: wsmailbox.ScopeWorktree, PID: otherPID, LastSeen: stamped}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	t.Setenv(envMailbox, "contested@worktree")
	s := NewServer(root, "test")
	s.ensureMailboxRegistered(root)
	if !s.mailboxHasConflict() {
		t.Fatalf("fixture did not produce a registration conflict")
	}

	advanceMailboxClock(t, 5*time.Minute)
	driveMailboxPresenceTicks(t, s, 1)

	if got := loadPresence(t, path, "contested").LastSeen; got != stamped {
		t.Fatalf("conflict loser's ticker refreshed the winner's record: LastSeen %q, want %q", got, stamped)
	}
}
