package mcp

import (
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wswhiteboard"
)

func TestWhiteboardNudgeBurstSharesDiscoveryAndRetainsNewBoards(t *testing.T) {
	f := newNudgeFixture(t)
	start := time.Now()
	var clock atomic.Int64
	clock.Store(start.UnixNano())
	f.n.now = func() time.Time { return time.Unix(0, clock.Load()) }
	f.board(t, `<wb:topic name="Work"><wb:state value="waiting"/></wb:topic>`, start)
	meta := filepath.Join(whiteboardFolder(f.target.root, f.target.key).Dir, "meta.json")
	var scans atomic.Int64
	f.n.stat = func(path string) (os.FileInfo, error) {
		if path == meta {
			scans.Add(1) // one probe of this known board per complete discovery
		}
		return os.Stat(path)
	}
	const burst = 100
	// Reproduce the old scheduler's discovery on each consumed poke.
	for range burst {
		f.n.discover()
	}
	before := scans.Swap(0)
	processed := make(chan string, 8)
	f.n.tickets = func(root string) wswhiteboard.TicketResolver {
		processed <- root
		return nil
	}
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
	wait() // startup discovery and process
	if scans.Load() != 1 {
		t.Fatalf("startup scans = %d, want one", scans.Load())
	}
	// A board first created after startup must not lose its poke while full
	// discovery is throttled. Its delivery still uses the uncached gates.
	added := nudgeTarget{"new-burst-lead", t.TempDir()}
	nudgeWriteJSON(t, f.n.dirs().partPath(added.key, partKey), sessionKeyRecord{SchemaVersion: 2, Scope: string(roleLead), Root: added.root})
	if _, err := whiteboardFolder(added.root, added.key).Open(&wswhiteboard.State{}); err != nil {
		t.Fatal(err)
	}
	for range burst {
		f.n.Poke(f.n.cache, f.target.root, f.target.key)
	}
	f.n.Poke(f.n.cache, added.root, added.key)
	if scans.Load() != 1 {
		t.Fatal("a burst inside the discovery window rescanned keys")
	}
	clock.Store(start.Add(whiteboardNudgeDiscoveryInterval).UnixNano())
	select {
	case f.n.changed <- struct{}{}:
	default:
	}
	seen := map[string]bool{wait(): true}
	seen[wait()] = true
	if !seen[added.root] || !seen[f.target.root] {
		t.Fatalf("trailing discovery dropped pending targets: %v", seen)
	}
	if got := scans.Load(); got != 2 {
		t.Fatalf("startup + burst scans = %d, want two", got)
	}
	t.Logf("MEASURE pokes=%d discovery_scans_before=%d discovery_scans_after=1 (plus one startup scan); new board retained", burst, before)
}

func TestWhiteboardNudgePokeDuringDiscoveryRetainsNewBoard(t *testing.T) {
	f := newNudgeFixture(t)
	start := time.Now()
	var clock atomic.Int64
	clock.Store(start.UnixNano())
	f.n.now = func() time.Time { return time.Unix(0, clock.Load()) }
	f.board(t, `<wb:topic name="Work"><wb:state value="waiting"/></wb:topic>`, start)
	meta := filepath.Join(whiteboardFolder(f.target.root, f.target.key).Dir, "meta.json")
	entered, release := make(chan struct{}), make(chan struct{})
	var blocked atomic.Bool
	f.n.stat = func(path string) (os.FileInfo, error) {
		if path == meta && blocked.CompareAndSwap(false, true) {
			close(entered) // key enumeration has already finished
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
	select {
	case <-processed: // original board completes the initial pass
	case <-time.After(5 * time.Second):
		t.Fatal("initial discovery did not finish")
	}
	clock.Store(start.Add(whiteboardNudgeDiscoveryInterval).UnixNano())
	select {
	case f.n.changed <- struct{}{}:
	default:
	}
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

func TestWhiteboardNudgeRecoverySharesDiscoveryDeadline(t *testing.T) {
	f := newNudgeFixture(t)
	start := time.Now()
	var clock atomic.Int64
	clock.Store(start.UnixNano())
	var calls atomic.Int64
	clockCalls := make(chan int64, 64)
	f.n.now = func() time.Time {
		n := calls.Add(1)
		clockCalls <- n
		return time.Unix(0, clock.Load())
	}
	f.board(t, `<wb:topic name="Work"><wb:state value="waiting"/></wb:topic>`, start)
	meta := filepath.Join(whiteboardFolder(f.target.root, f.target.key).Dir, "meta.json")
	old := start.Add(-time.Hour)
	if err := os.Chtimes(meta, old, old); err != nil {
		t.Fatal(err)
	}
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
					return // timer.Reset's clock read closes this scheduler turn
				}
			case <-time.After(5 * time.Second):
				t.Fatal("scheduler turn did not finish")
			}
		}
	}
	f.n.Start()
	defer f.n.Close()
	waitProcessed()
	waitCalls(3) // initial scan clock, first turn, timer reset
	nearRecovery := start.Add(whiteboardNudgeRecoveryInterval - 100*time.Millisecond)
	clock.Store(nearRecovery.UnixNano())
	f.n.Poke(f.n.cache, f.target.root, f.target.key)
	waitProcessed()
	waitCalls(5)
	clock.Store(start.Add(whiteboardNudgeRecoveryInterval).UnixNano())
	f.n.changed <- struct{}{}
	waitCalls(7)
	if got := scans.Load(); got != 2 {
		t.Fatalf("recovery bypassed the shared discovery deadline: %d scans, want two", got)
	}
	clock.Store(nearRecovery.Add(whiteboardNudgeDiscoveryInterval).UnixNano())
	f.n.changed <- struct{}{}
	waitProcessed() // unchanged metadata is revisited only by full recovery
	waitCalls(9)
	if got := scans.Load(); got != 3 {
		t.Fatalf("deferred recovery scans = %d, want three", got)
	}
}
