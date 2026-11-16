package wsdashboard

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type hireTestTimer struct {
	mu    sync.Mutex
	now   time.Time
	armed chan time.Duration
	fire  chan time.Time
}

func (c *hireTestTimer) clock() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *hireTestTimer) after(d time.Duration) (<-chan time.Time, func()) {
	c.mu.Lock()
	c.fire = make(chan time.Time, 1)
	ch := c.fire
	c.mu.Unlock()
	c.armed <- d
	return ch, func() {}
}

func (c *hireTestTimer) advance(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	c.fire <- c.now
	c.mu.Unlock()
}

func hireArmed(t *testing.T, c *hireTestTimer) time.Duration {
	t.Helper()
	select {
	case d := <-c.armed:
		return d
	case <-time.After(5 * time.Second):
		t.Fatal("hire loop did not arm")
		return 0
	}
}

func TestHireLoopIdleSafetyPokeAndRetry(t *testing.T) {
	clock := &hireTestTimer{now: time.Date(2026, 11, 16, 0, 0, 0, 0, time.UTC), armed: make(chan time.Duration, 8)}
	var reads atomic.Int64
	var foreign atomic.Bool
	var observed atomic.Bool
	var fail atomic.Bool
	d := &Daemon{hireNow: clock.clock, hireAfter: clock.after, hireWake: make(chan struct{}, 1), hireDone: make(chan struct{}), stop: make(chan struct{}), log: io.Discard}
	d.hireRead = func() ([]HireProposal, error) {
		reads.Add(1)
		observed.Store(foreign.Load())
		if fail.Swap(false) {
			return nil, errors.New("temporary read error")
		}
		return nil, nil
	}
	go d.hireLoop()
	defer func() { close(d.stop); <-d.hireDone }()
	if got := hireArmed(t, clock); got != hireSafetyPoll || reads.Load() != 1 {
		t.Fatalf("startup timer %v, reads %d", got, reads.Load())
	}
	// No intermediate timer exists, so an idle daemon cannot reread.
	if reads.Load() != 1 {
		t.Fatal("idle reread")
	}
	foreign.Store(true) // unnotified external edit
	clock.advance(hireSafetyPoll)
	if got := hireArmed(t, clock); got != hireSafetyPoll || reads.Load() != 2 || !observed.Load() {
		t.Fatalf("safety timer %v, reads %d, observed %v", got, reads.Load(), observed.Load())
	}
	fail.Store(true)
	d.pokeHires()
	if got := hireArmed(t, clock); got != hireRetry || reads.Load() != 3 {
		t.Fatalf("retry timer %v, reads %d", got, reads.Load())
	}
	clock.advance(hireRetry)
	if got := hireArmed(t, clock); got != hireSafetyPoll-hireRetry || reads.Load() != 4 {
		t.Fatalf("recovered timer %v, reads %d", got, reads.Load())
	}
}

func TestHireSavePokesReconciler(t *testing.T) {
	d := &Daemon{dir: t.TempDir(), hireWake: make(chan struct{}, 1)}
	if err := d.saveHires([]HireProposal{}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-d.hireWake:
	default:
		t.Fatal("successful proposal save did not poke the reconciler")
	}
}

func TestHireDeadlineRereadsHeartbeat(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 11, 16, 0, 0, 0, 0, time.UTC)
	d := &Daemon{dir: dir, hireNow: func() time.Time { return now }}
	reg := Registration{SchemaVersion: 1, ReplyID: "livesession", Project: dir, HeartbeatAt: now.Format(time.RFC3339)}
	if err := WriteRegistration(dir, reg); err != nil {
		t.Fatal(err)
	}
	p := []HireProposal{{ID: "request", Requester: reg.ReplyID, Status: "pending", ExpiresAt: now.Add(hireLifetime)}}
	next, err := d.settleHiresDeadlineLocked(p)
	want := now.Add(LiveWindow + time.Nanosecond)
	if err != nil || !next.Equal(want) {
		t.Fatalf("deadline %v want %v: %v", next, want, err)
	}
	// A foreign heartbeat is not notified. The deadline must reread it,
	// rather than withdrawing based on the old in-memory timestamp.
	now = want
	reg.HeartbeatAt = now.Format(time.RFC3339)
	if err := WriteRegistration(dir, reg); err != nil {
		t.Fatal(err)
	}
	next, err = d.settleHiresDeadlineLocked(p)
	want = now.Truncate(time.Second).Add(LiveWindow + time.Nanosecond)
	if err != nil || p[0].Status != "pending" || !next.Equal(want) {
		t.Fatalf("after heartbeat: %+v deadline %v want %v: %v", p[0], next, want, err)
	}
	// A nearer proposal expiry wins over the liveness deadline.
	p[0].ExpiresAt = now.Add(time.Second)
	next, err = d.settleHiresDeadlineLocked(p)
	if err != nil || !next.Equal(p[0].ExpiresAt) {
		t.Fatalf("expiry deadline %v: %v", next, err)
	}
	// Immediate startup recovery must retain the approval journal until the
	// index has the hierarchy authority used by hiringEligible.
	d.index = NewIndex(IndexOptions{Dir: dir})
	p[0].Launch = &HireLaunch{}
	next, err = d.settleHiresDeadlineLocked(p)
	if err != nil || p[0].Status != "pending" || p[0].Launch == nil || !next.Equal(p[0].ExpiresAt) {
		t.Fatalf("approval before index ready: %+v next=%v err=%v", p[0], next, err)
	}
	d.index = nil
	if next, err = d.settleHiresDeadlineLocked(p); err != nil || p[0].Status != "pending" || p[0].Launch == nil || next.IsZero() {
		t.Fatalf("approval with no index: %+v next=%v err=%v", p[0], next, err)
	}
}

type hireRecoverySource struct{ failed string }

func (s *hireRecoverySource) ReadSession(reg Registration) (SessionData, bool, error) {
	if reg.ReplyID == s.failed {
		return SessionData{}, false, errors.New("transient source failure")
	}
	return SessionData{}, true, nil
}

func TestHireStartupPartialIndexRetainsApprovalJournal(t *testing.T) {
	for _, failure := range []string{"requester-source", "parent-source", "parent-registration"} {
		t.Run(failure, func(t *testing.T) {
			f := startPageFixture(t, hireRegistryFixture)
			enableHiring(t, f.d, "livesession")
			p := requestHiring(t, f.d, "livesession")
			f.d.Close() // replay deterministically, without the fixture's loop
			p.Launch = &HireLaunch{Harness: "claude"}
			if err := f.d.saveHires([]HireProposal{p}); err != nil {
				t.Fatal(err)
			}
			answer, err := f.d.layout.apply(railLayoutRequest{Op: "attach", RID: p.Requester, Parent: "boardsession"}, railFactsOf(f.d.railBaseView(f.d.pages())), true)
			if err != nil || answer.Reason != "" {
				t.Fatalf("persist nested relationship: %+v %v", answer, err)
			}
			source := &hireRecoverySource{}
			if failure == "requester-source" {
				source.failed = p.Requester
			}
			if failure == "parent-source" {
				source.failed = "boardsession"
			}
			regPath := filepath.Join(SessionDir(f.dir, "boardsession"), registrationFileName)
			original, err := os.ReadFile(regPath)
			if err != nil {
				t.Fatal(err)
			}
			if failure == "parent-registration" {
				if err := os.WriteFile(regPath, []byte("{incomplete"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			f.d.index = NewIndex(IndexOptions{Dir: f.dir, Source: source})
			f.d.index.Refresh()
			if !f.d.index.Loaded() || f.d.index.hierarchyReady(p.Requester) {
				t.Fatal("partial refresh supplied hierarchy authority")
			}
			f.d.settleHires()
			proposals, err := f.d.readHires()
			if err != nil || len(proposals) != 1 || proposals[0].Status != "pending" || proposals[0].Launch == nil || proposals[0].CreationAttempted {
				t.Fatalf("partial index lost approval journal: %+v %v", proposals, err)
			}
			source.failed = ""
			if err := os.WriteFile(regPath, original, 0o600); err != nil {
				t.Fatal(err)
			}
			f.d.index.Refresh()
			if !f.d.index.hierarchyReady(p.Requester) {
				t.Fatal("complete refresh did not recover authority")
			}
			f.d.settleHires()
			proposals, err = f.d.readHires()
			if err != nil || proposals[0].Status != "withdrawn" || f.d.agents.(*hireFake).created != 0 {
				t.Fatalf("recovered nested request authorized launch: %+v %v", proposals, err)
			}
		})
	}
}
