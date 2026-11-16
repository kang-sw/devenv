package mcp

import (
	"os"
	"path/filepath"
	"sync/atomic"

	"github.com/kang-sw/devenv/internal/wswhiteboard"
	"testing"
	"time"
)

func TestWhiteboardNudgeSafetyScanCadenceAndMissedPoke(t *testing.T) {
	f := newNudgeFixture(t)
	start := time.Now()
	var clock atomic.Int64
	clock.Store(start.UnixNano())
	f.n.now = func() time.Time { return time.Unix(0, clock.Load()) }
	f.board(t, `<wb:topic name="Work"><wb:state value="waiting"/><wb:state value="implementing" owner="executor" eta_minutes="10"/></wb:topic>`, start)
	var stats atomic.Int64
	f.n.stat = func(path string) (os.FileInfo, error) { stats.Add(1); return os.Stat(path) }
	processed := make(chan struct{}, 8)
	f.n.tickets = func(string) wswhiteboard.TicketResolver { processed <- struct{}{}; return nil }
	f.n.Start()
	defer f.n.Close()
	wait := func() {
		t.Helper()
		select {
		case <-processed:
		case <-time.After(5 * time.Second):
			t.Fatal("board not processed")
		}
	}
	wait()
	// No scheduled deadline is due yet. Merely waking the scheduler at the
	// former 30-second cadence must not rediscover every key or board.
	before := stats.Load()
	clock.Store(start.Add(30 * time.Second).UnixNano())
	f.n.changed <- struct{}{}
	select {
	case <-processed:
		t.Fatal("idle board processed at 30 seconds")
	case <-time.After(50 * time.Millisecond):
	}
	if stats.Load() != before {
		t.Fatal("idle wake rediscovered the filesystem")
	}
	// An external transaction whose poke was lost is still recovered by the
	// five-minute safety scan, even though its old deadline was ten minutes.
	f.board(t, `<wb:topic name="Work"><wb:state value="waiting"/><wb:state value="implementing" owner="replacement" eta_minutes="1"/></wb:topic>`, start)
	clock.Store(start.Add(whiteboardNudgeRecoveryInterval).UnixNano())
	f.n.changed <- struct{}{}
	wait()
	if stats.Load() <= before {
		t.Fatal("safety scan did not rediscover boards")
	}
	t.Log("idle discovery cadence: 10 scans / 5 minutes -> 1; exact deadlines and explicit pokes remain separate")
}

func TestWhiteboardNudgeReconcileStampsAndDeadlines(t *testing.T) {
	n, target, _ := nudgeCacheFixture(t)
	meta := filepath.Join(whiteboardFolder(target.root, target.key).Dir, "meta.json")
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(meta, old, old); err != nil {
		t.Fatal(err)
	}
	found := n.discover()
	deadline := time.Now().Add(17 * time.Minute)
	boards := map[nudgeTarget]time.Time{target: deadline}
	stamps := n.boardStamps
	found = n.discover()
	boards, stamps = nudgeReconcile(boards, stamps, found, n.boardStamps, false)
	if !boards[target].Equal(deadline) {
		t.Fatal("unchanged scan lost precise deadline", boards[target])
	}
	// A transaction replaces session metadata, even when its size is equal.
	replacement := meta + ".new"
	if err := os.WriteFile(replacement, []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(replacement, old.Add(time.Second), old.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(replacement, meta); err != nil {
		t.Fatal(err)
	}
	found = n.discover()
	boards, stamps = nudgeReconcile(boards, stamps, found, n.boardStamps, false)
	if !boards[target].IsZero() {
		t.Fatal("replacement did not invalidate schedule")
	}
	boards[target] = deadline
	found = n.discover()
	boards, stamps = nudgeReconcile(boards, stamps, found, n.boardStamps, true)
	if !boards[target].IsZero() {
		t.Fatal("recovery did not revisit unchanged board")
	}
	// A missing session meta but surviving pending intent has no safe stamp.
	if err := os.Remove(meta); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(filepath.Dir(meta), "pending.json"), []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	boards[target] = deadline
	found = n.discover()
	boards, _ = nudgeReconcile(boards, stamps, found, n.boardStamps, false)
	if !boards[target].IsZero() {
		t.Fatal("pending-only board reused schedule")
	}
}

func TestWhiteboardNudgeReconcileRecentAndRemoved(t *testing.T) {
	n, target, _ := nudgeCacheFixture(t)
	found := n.discover()
	stamps := n.boardStamps
	boards := map[nudgeTarget]time.Time{target: time.Now().Add(time.Hour)}
	found, _ = nudgeReconcile(boards, stamps, found, stamps, false)
	if !found[target].IsZero() {
		t.Fatal("recent metadata trusted")
	}
	found, _ = nudgeReconcile(boards, stamps, map[nudgeTarget]time.Time{}, nil, false)
	if len(found) != 0 {
		t.Fatal("removed board retained")
	}
}
