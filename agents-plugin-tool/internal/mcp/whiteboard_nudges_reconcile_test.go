package mcp

import (
	"io"
	"os"
	"path/filepath"
	"sync/atomic"

	"github.com/kang-sw/devenv/internal/wswhiteboard"
	"testing"
	"time"
)

func TestWhiteboardNudgeAbsentMemoryAndPoke(t *testing.T) {
	n := NewWhiteboardNudges(filepath.Join(t.TempDir(), "dashboard"), nil, io.Discard)
	defer n.Close()
	target := nudgeTarget{"absent-memory-lead", t.TempDir()}
	nudgeWriteJSON(t, n.dirs().partPath(target.key, partKey), sessionKeyRecord{SchemaVersion: 2, Scope: string(roleLead), Root: target.root})
	start := time.Now()
	var clock, calls atomic.Int64
	clock.Store(start.UnixNano())
	turns := make(chan int64, 16)
	n.now = func() time.Time { turns <- calls.Add(1); return time.Unix(0, clock.Load()) }
	processed := make(chan struct{}, 8)
	n.tickets = func(string) wswhiteboard.TicketResolver { processed <- struct{}{}; return nil }
	waitTurn := func(want int64) {
		t.Helper()
		for {
			select {
			case got := <-turns:
				if got >= want {
					return
				}
			case <-time.After(5 * time.Second):
				t.Fatal("scheduler did not finish turn")
			}
		}
	}
	n.Start()
	waitTurn(3)
	select {
	case <-processed:
	default:
		t.Fatal("cold identity was not probed")
	}
	if _, err := whiteboardFolder(target.root, target.key).Open(&wswhiteboard.State{}); err != nil {
		t.Fatal(err)
	}
	clock.Store(start.Add(whiteboardNudgeRecoveryInterval).UnixNano())
	n.changed <- struct{}{}
	waitTurn(5)
	select {
	case <-processed:
		t.Fatal("recovery reprobed known absent board")
	default:
	}
	n.Poke(n.cache, target.root, target.key)
	waitTurn(7)
	select {
	case <-processed:
	default:
		t.Fatal("poke did not reprobe absent board")
	}
	if err := n.stores.use(target.root, target.key, nil, false, func(slot *whiteboardSlot) error {
		if slot.board == nil {
			t.Error("poke did not open newly present board")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestWhiteboardNudgeProbeErrorRemainsUnknown(t *testing.T) {
	n := NewWhiteboardNudges(filepath.Join(t.TempDir(), "dashboard"), nil, io.Discard)
	defer n.Close()
	if err := n.stores.Close(); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	next, presence := n.processBoard(nudgeTarget{"error-lead", t.TempDir()}, now)
	if presence != nudgeUnknown || !next.Equal(now.Add(n.interval)) {
		t.Fatalf("failed probe: presence=%v next=%v", presence, next)
	}
}

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

func TestWhiteboardNudgeRefreshUsesLiveBoard(t *testing.T) {
	f := newNudgeFixture(t)
	start := time.Now().Truncate(time.Second)
	f.board(t, `<wb:topic name="Work"><wb:state value="waiting"/><wb:state value="implementing" owner="first" eta_minutes="10"/></wb:topic>`, start)
	first := f.n.process(f.target, start)
	f.board(t, `<wb:topic name="Work"><wb:state value="waiting"/><wb:state value="implementing" owner="second" eta_minutes="1"/></wb:topic>`, start)
	second := f.n.process(f.target, start)
	if !first.Equal(start.Add(10*time.Minute)) || !second.Equal(start.Add(time.Minute)) {
		t.Fatalf("live board deadlines: first=%v second=%v", first, second)
	}
}
