package wsagent

import (
	"io"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wslive"
)

// lockSeamClock is a race-safe settable clock for the summary lock seams.
type lockSeamClock struct{ ns atomic.Int64 }

func newLockSeamClock(at time.Time) *lockSeamClock {
	k := &lockSeamClock{}
	k.ns.Store(at.UnixNano())
	return k
}

func (k *lockSeamClock) now() time.Time { return time.Unix(0, k.ns.Load()).UTC() }

func (k *lockSeamClock) advance(d time.Duration) { k.ns.Add(int64(d)) }

// ringStall parks a liveWriter's ring append inside withOwner, so the
// session's registration flock, summary lock, and slot owner lock stay held
// until release.
type ringStall struct {
	entered     chan struct{}
	release     chan struct{}
	enterOnce   sync.Once
	releaseOnce sync.Once
}

func newRingStall(t *testing.T) *ringStall {
	s := &ringStall{entered: make(chan struct{}), release: make(chan struct{})}
	// A failed assertion must not leave the stalled writer parked.
	t.Cleanup(s.open)
	return s
}

func (s *ringStall) append(dir string, entries []wslive.RingEntry) error {
	s.enterOnce.Do(func() { close(s.entered) })
	<-s.release
	return wslive.AppendRing(dir, entries)
}

func (s *ringStall) open() { s.releaseOnce.Do(func() { close(s.release) }) }

// awaitLockSeam fails unless ch closes promptly; it bounds a wait, it does not
// pace the test.
func awaitLockSeam(t *testing.T, ch <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(5 * time.Second):
		t.Fatalf("%s did not complete", what)
	}
}

// stillBlocked fails if ch closes within a short window: the step is fenced.
func stillBlocked(t *testing.T, ch <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-ch:
		t.Fatalf("%s bypassed an active fence", what)
	case <-time.After(100 * time.Millisecond):
	}
}

func startLockSeamDaemon(t *testing.T, dir string) *wsdashboard.Daemon {
	t.Helper()
	d, err := wsdashboard.StartDaemon(wsdashboard.DaemonOptions{Dir: dir, Port: 0, Sessions: leadSessions{}, PollInterval: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(d.Close)
	return d
}

// newLockSeamLead builds a hosted lead conversation the way startConvWith
// wires its summary writer, without a loop, and writes its hosted row.
func newLockSeamLead(t *testing.T, r *Runner, id string) *conv {
	t.Helper()
	rec := Record{ReplyID: id, Root: t.TempDir(), Backend: BackendPi, Lead: true, HostPID: 4242}
	c := &conv{r: r, rec: rec}
	c.live = newLiveWriter(wsdashboard.SessionDir(r.opts.Dir, id), id, rec.Backend, r.opts.Now, io.Discard)
	c.live.owner = c.lockSummaryWriter
	r.setSummaryOwner(id, c)
	if !c.writeRegistration() {
		t.Fatalf("%s: hosted registration not written", id)
	}
	return c
}

func openLockSeamSlot(t *testing.T, d *wsdashboard.Daemon, c *conv) wsdashboard.HostedSlot {
	t.Helper()
	slot, ok := d.OpenHostedSlot(wsdashboard.SessionDir(c.r.opts.Dir, c.rec.ReplyID))
	if !ok {
		t.Fatalf("%s: hosted slot refused", c.rec.ReplyID)
	}
	c.live.setSlot(slot)
	return slot
}

// startStalledRingWrite runs one ring append of c's writer on its own
// goroutine and returns once it is parked inside withOwner. done closes when
// the write returns; *appended is its result, readable after done.
func startStalledRingWrite(t *testing.T, c *conv, stall *ringStall) (done chan struct{}, appended *bool) {
	t.Helper()
	c.live.appendRing = stall.append
	done, appended = make(chan struct{}), new(bool)
	res := wslive.Result{Ring: []wslive.RingEntry{{Kind: wslive.RingUser, Origin: wslive.OriginTerminal, Text: "stalled ring append"}}}
	go func() {
		defer close(done)
		*appended = c.live.write(res, nil, "test", false)
	}()
	// Registered after the slot cleanups, so it runs first: a failed test
	// lets the parked write finish before its slot closes.
	t.Cleanup(func() {
		stall.open()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
		}
	})
	awaitLockSeam(t, stall.entered, c.rec.ReplyID+" ring append reaching its stall")
	return done, appended
}

// TestStalledRingWriteDoesNotBlockAnotherHostedSession is Verification (a)
// on the real daemon slot (Trap D): one session's ring I/O, held under its
// registration flock, its per-reply-id summary lock, and its daemon slot
// owner lock, must not stall another session's snapshot write, its slot
// publish, or its registration heartbeat.
func TestStalledRingWriteDoesNotBlockAnotherHostedSession(t *testing.T) {
	base := t.TempDir()
	clock := newLockSeamClock(time.Unix(1000, 0))
	r := NewRunner(Options{Dir: base, Now: clock.now, Log: io.Discard})
	d := startLockSeamDaemon(t, base)

	a := newLockSeamLead(t, r, "lockalpha")
	b := newLockSeamLead(t, r, "lockbeta")
	openLockSeamSlot(t, d, a)
	t.Cleanup(a.live.closeSlot)
	slotB := openLockSeamSlot(t, d, b)
	t.Cleanup(b.live.closeSlot)
	r.mu.Lock()
	r.convs[a.rec.ReplyID] = a
	r.convs[b.rec.ReplyID] = b
	r.mu.Unlock()
	start := clock.now().Format(time.RFC3339)

	var bWrites atomic.Int32
	b.live.writeSnapshotFile = func(path string, raw []byte, mode os.FileMode) error {
		err := writeFileAtomic(path, raw, mode)
		if err == nil {
			bWrites.Add(1)
		}
		return err
	}

	stall := newRingStall(t)
	aDone, aAppended := startStalledRingWrite(t, a, stall)

	// B's hosted snapshot write: summary lock, slot owner lock, and publish.
	snapDone := make(chan struct{})
	go func() {
		defer close(snapDone)
		b.live.heartbeatSnapshot()
	}()
	awaitLockSeam(t, snapDone, "lockbeta snapshot write while lockalpha's ring append is stalled")
	if bWrites.Load() != 1 {
		t.Fatalf("lockbeta snapshot writes = %d, want 1 (ownership refused while another session wrote)", bWrites.Load())
	}

	// The daemon slot publish takes only the brief daemon-wide mutex.
	publishDone := make(chan struct{})
	go func() {
		defer close(publishDone)
		slotB.Publish(wslive.Snapshot{SchemaVersion: 1, Adapter: BackendPi, UpdatedAt: clock.now().UnixMilli()})
	}()
	awaitLockSeam(t, publishDone, "lockbeta slot publish while lockalpha's ring append is stalled")

	// The sweep skips the stalled session's held row and still refreshes B.
	clock.advance(r.sweepThreshold())
	sweepDone := make(chan struct{})
	go func() {
		defer close(sweepDone)
		r.sweepHeartbeats()
	}()
	awaitLockSeam(t, sweepDone, "heartbeat sweep while lockalpha's ring append is stalled")
	want := clock.now().Format(time.RFC3339)
	if row, err := wsdashboard.ReadRegistration(base, b.rec.ReplyID); err != nil || row.HeartbeatAt != want {
		t.Fatalf("lockbeta HeartbeatAt = %q (err %v), want %q", row.HeartbeatAt, err, want)
	}
	if row, err := wsdashboard.ReadRegistration(base, a.rec.ReplyID); err != nil || row.HeartbeatAt != start {
		t.Fatalf("lockalpha HeartbeatAt = %q (err %v), want untouched %q while its row lock is held", row.HeartbeatAt, err, start)
	}

	stall.open()
	awaitLockSeam(t, aDone, "lockalpha ring append after release")
	if !*aAppended {
		t.Fatal("lockalpha ring append reported no change")
	}
}

// TestHostedSlotTakeoverWaitsForStalledRingWrite pins the per-session slot
// owner lock (Trap D) as still fencing a takeover of the same session: the
// takeover waits for the stalled write, and the replaced generation is
// refused afterwards.
func TestHostedSlotTakeoverWaitsForStalledRingWrite(t *testing.T) {
	base := t.TempDir()
	clock := newLockSeamClock(time.Unix(1000, 0))
	r := NewRunner(Options{Dir: base, Now: clock.now, Log: io.Discard})
	d := startLockSeamDaemon(t, base)

	a := newLockSeamLead(t, r, "lockalpha")
	first := openLockSeamSlot(t, d, a)
	t.Cleanup(a.live.closeSlot)

	stall := newRingStall(t)
	aDone, aAppended := startStalledRingWrite(t, a, stall)

	taken := make(chan struct{})
	var second wsdashboard.HostedSlot
	go func() {
		defer close(taken)
		second, _ = d.OpenHostedSlot(wsdashboard.SessionDir(base, a.rec.ReplyID))
	}()
	stillBlocked(t, taken, "takeover of lockalpha during its stalled ring append")

	stall.open()
	awaitLockSeam(t, aDone, "lockalpha ring append after release")
	if !*aAppended {
		t.Fatal("lockalpha ring append reported no change")
	}
	awaitLockSeam(t, taken, "takeover of lockalpha after its ring append returned")
	if second == nil {
		t.Fatal("takeover refused")
	}
	t.Cleanup(second.Close)

	if release, ok := first.(wsdashboard.LiveSlotOwner).LockOwner(); ok {
		release()
		t.Fatal("replaced generation still admitted as slot owner")
	}
	release, ok := second.(wsdashboard.LiveSlotOwner).LockOwner()
	if !ok {
		t.Fatal("new generation not admitted as slot owner")
	}
	release()
}

// TestSummaryOwnerRolloverWaitsForHeldWriter pins the per-reply-id summary
// lock across conversation generations: a newer generation of the same
// session takes ownership only after the older writer releases, the older
// one is refused afterwards, and another session's owner change never waits
// on it.
func TestSummaryOwnerRolloverWaitsForHeldWriter(t *testing.T) {
	base := t.TempDir()
	clock := newLockSeamClock(time.Unix(1000, 0))
	r := NewRunner(Options{Dir: base, Now: clock.now, Log: io.Discard})

	c1 := newLockSeamLead(t, r, "lockalpha")
	release1, ok := c1.lockSummaryWriter()
	if !ok {
		t.Fatal("generation 1 not admitted as summary writer")
	}
	released := false
	defer func() {
		if !released {
			release1()
		}
	}()

	// Another session's owner change takes only the brief map guard.
	other := &conv{r: r, rec: Record{ReplyID: "lockbeta", Backend: BackendPi, Lead: true, HostPID: 4242}}
	otherDone := make(chan struct{})
	go func() {
		defer close(otherDone)
		r.setSummaryOwner("lockbeta", other)
	}()
	awaitLockSeam(t, otherDone, "lockbeta owner change while lockalpha's writer is held")

	c2 := &conv{r: r, rec: c1.rec}
	swapped := make(chan struct{})
	go func() {
		defer close(swapped)
		r.setSummaryOwner(c1.rec.ReplyID, c2)
	}()
	stillBlocked(t, swapped, "generation 2 owner change of lockalpha while generation 1 writes")
	if got := r.summaryOwner(c1.rec.ReplyID); got != c1 {
		t.Fatal("summary owner changed under a held writer")
	}

	release1()
	released = true
	awaitLockSeam(t, swapped, "generation 2 owner change after generation 1 released")

	if release, ok := c1.lockSummaryWriter(); ok {
		release()
		t.Fatal("generation 1 still admitted as summary writer after rollover")
	}
	release2, ok := c2.lockSummaryWriter()
	if !ok {
		t.Fatal("generation 2 not admitted as summary writer")
	}
	release2()
}
