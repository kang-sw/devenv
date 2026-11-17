package mcp

import (
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wswhiteboard"
)

func TestWhiteboardNudgeBurstTargetsOnlyPokedBoards(t *testing.T) {
	f := newNudgeFixture(t)
	unrelated := f.n.dirs().partPath("unrelated-burst-key", partKey)
	nudgeWriteJSON(t, unrelated, f.record)
	var unrelatedStats atomic.Int64
	f.n.stat = func(path string) (os.FileInfo, error) {
		if path == unrelated {
			unrelatedStats.Add(1)
		}
		return os.Stat(path)
	}
	processed := make(chan string, 512)
	f.n.tickets = func(root string) wswhiteboard.TicketResolver { processed <- root; return nil }
	wait := func() string {
		t.Helper()
		select {
		case root := <-processed:
			return root
		case <-time.After(5 * time.Second):
			t.Fatal("pending board was not processed")
			return ""
		}
	}
	f.n.Start()
	defer f.n.Close()
	wait() // startup recovery scans all keys once
	before := unrelatedStats.Load()
	if before == 0 {
		t.Fatal("startup did not discover unrelated key")
	}
	added := nudgeTarget{"new-burst-lead", t.TempDir()}
	nudgeWriteJSON(t, f.n.dirs().partPath(added.key, partKey), sessionKeyRecord{SchemaVersion: 2, Scope: string(roleLead), Root: added.root})
	if _, err := whiteboardFolder(added.root, added.key).Open(&wswhiteboard.State{}); err != nil {
		t.Fatal(err)
	}
	for range 100 {
		f.n.Poke(f.n.cache, f.target.root, f.target.key)
	}
	f.n.Poke(f.n.cache, added.root, added.key)
	seen := map[string]bool{}
	for !seen[added.root] || !seen[f.target.root] {
		seen[wait()] = true
	}
	if got := unrelatedStats.Load(); got != before {
		t.Fatalf("pokes revisited unrelated identity: %d -> %d", before, got)
	}
}

func TestWhiteboardNudgePokeDuringDiscoveryRetainsNewBoard(t *testing.T) {
	f := newNudgeFixture(t)
	meta := filepath.Join(whiteboardFolder(f.target.root, f.target.key).Dir, "meta.json")
	entered, release := make(chan struct{}), make(chan struct{})
	var blocked atomic.Bool
	f.n.stat = func(path string) (os.FileInfo, error) {
		if path == meta && blocked.CompareAndSwap(false, true) {
			close(entered) // startup key enumeration has already finished
			<-release
		}
		return os.Stat(path)
	}
	processed := make(chan string, 8)
	f.n.tickets = func(root string) wswhiteboard.TicketResolver { processed <- root; return nil }
	f.n.Start()
	defer f.n.Close()
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
	}()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("discovery did not reach its barrier")
	}
	added := nudgeTarget{"during-discovery-lead", t.TempDir()}
	nudgeWriteJSON(t, f.n.dirs().partPath(added.key, partKey), sessionKeyRecord{SchemaVersion: 2, Scope: string(roleLead), Root: added.root})
	if _, err := whiteboardFolder(added.root, added.key).Open(&wswhiteboard.State{}); err != nil {
		close(release)
		t.Fatal(err)
	}
	f.n.Poke(f.n.cache, added.root, added.key)
	close(release)
	timeout := time.After(5 * time.Second)
	for {
		select {
		case root := <-processed:
			if root == added.root {
				return
			}
		case <-timeout:
			t.Fatal("a poke arriving during enumeration was lost")
		}
	}
}

func TestWhiteboardNudgeRecoveryIsIndependentOfPokes(t *testing.T) {
	f := newNudgeFixture(t)
	start := time.Now()
	var clock atomic.Int64
	clock.Store(start.UnixNano())
	var calls atomic.Int64
	clockCalls := make(chan int64, 64)
	f.n.now = func() time.Time {
		clockCalls <- calls.Add(1)
		return time.Unix(0, clock.Load())
	}
	f.board(t, `<wb:topic name="Work"><wb:state value="waiting"/></wb:topic>`, start)
	meta := filepath.Join(whiteboardFolder(f.target.root, f.target.key).Dir, "meta.json")
	var scans atomic.Int64
	f.n.stat = func(path string) (os.FileInfo, error) {
		if path == meta {
			scans.Add(1)
		}
		return os.Stat(path)
	}
	processed := make(chan struct{}, 8)
	f.n.tickets = func(string) wswhiteboard.TicketResolver { processed <- struct{}{}; return nil }
	waitProcessed := func() {
		t.Helper()
		select {
		case <-processed:
		case <-time.After(5 * time.Second):
			t.Fatal("recovery did not process the board")
		}
	}
	waitCalls := func(want int64) {
		t.Helper()
		for {
			select {
			case got := <-clockCalls:
				if got >= want {
					return
				}
			case <-time.After(5 * time.Second):
				t.Fatal("scheduler turn did not finish")
			}
		}
	}
	f.n.Start()
	defer f.n.Close()
	waitProcessed()
	waitCalls(3)
	clock.Store(start.Add(whiteboardNudgeRecoveryInterval - 100*time.Millisecond).UnixNano())
	f.n.Poke(f.n.cache, f.target.root, f.target.key)
	waitProcessed()
	waitCalls(5)
	clock.Store(start.Add(whiteboardNudgeRecoveryInterval).UnixNano())
	f.n.changed <- struct{}{}
	waitProcessed()
	waitCalls(7)
	if got := scans.Load(); got != 3 {
		t.Fatalf("startup + targeted poke + full recovery = %d board probes, want 3", got)
	}
}
