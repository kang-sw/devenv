package wsagent

import (
	"io"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wsstate"
)

func heartbeatRegistryIO(operation string) int64 {
	var n int64
	for site, count := range wsstate.FileIOCounters() {
		if !strings.HasPrefix(site, "file_io_site:wsdashboard/registry.go:") {
			continue
		}
		if operation == "" || strings.HasSuffix(site, ":"+operation) {
			n += count
		}
	}
	return n
}

func heartbeatRegistration(t *testing.T, dir, id string) wsdashboard.Registration {
	t.Helper()
	row, err := wsdashboard.ReadRegistration(dir, id)
	if err != nil {
		t.Fatalf("read registration %s: %v", id, err)
	}
	return row
}

func addHeartbeatSweepConv(t *testing.T, r *Runner, rec Record) *conv {
	t.Helper()
	if rec.CreatedAt == "" {
		rec.CreatedAt = r.opts.Now().UTC().Format(time.RFC3339)
	}
	c := &conv{r: r, rec: rec}
	if rec.Lead {
		r.setSummaryOwner(rec.ReplyID, c)
	}
	if !c.writeRegistration() {
		t.Fatalf("register %s", rec.ReplyID)
	}
	r.mu.Lock()
	r.convs[rec.ReplyID] = c
	r.mu.Unlock()
	return c
}

func TestHeartbeatSweepRefreshesBlockedLoopOnce(t *testing.T) {
	clock := &testClock{now: time.Unix(1_000, 0).UTC()}
	dir := t.TempDir()
	r := NewRunner(Options{Dir: dir, Now: clock.Now, Log: io.Discard, heartbeat: time.Hour})
	rec := Record{
		ReplyID:   "blockedlead",
		Root:      t.TempDir(),
		Backend:   BackendPi,
		WSKey:     "blocked-key",
		CreatedAt: clock.Now().Format(time.RFC3339),
		Lead:      true,
		HostPID:   42,
	}
	c := r.startConv(rec, nil)
	t.Cleanup(r.Close)
	before := heartbeatRegistration(t, dir, rec.ReplyID)
	exitCode, exitSignal := 17, 9
	before.GitCommonDir = "/git/common"
	before.HostExitCode = &exitCode
	before.HostExitSignal = &exitSignal
	before.HostExitReason = "sentinel exit"
	before.HostStoppedAt = "2026-11-20T00:00:00Z"
	before.LastCallAt = "2026-11-20T00:01:00Z"
	if err := wsdashboard.WriteRegistration(dir, before); err != nil {
		t.Fatal(err)
	}

	entered := make(chan struct{})
	release := make(chan struct{})
	blockedDone := make(chan struct{})
	var releaseOnce sync.Once
	releaseLoop := func() { releaseOnce.Do(func() { close(release) }) }
	go func() {
		c.do(func() {
			close(entered)
			<-release
		})
		close(blockedDone)
	}()
	<-entered
	t.Cleanup(func() {
		releaseLoop()
		<-blockedDone
	})

	clock.advance(wsdashboard.HeartbeatInterval)
	touches := wsstate.FileIOCounters()["file_io_total"]
	r.sweepHeartbeats()
	if got := wsstate.FileIOCounters()["file_io_total"] - touches; got != 0 {
		t.Fatalf("healthy-loop deadline sweep made %d file operations, want 0", got)
	}
	notDue := heartbeatRegistration(t, dir, rec.ReplyID)
	if notDue.HeartbeatAt != before.HeartbeatAt {
		t.Fatalf("sweep stole healthy loop deadline: before=%q after=%q", before.HeartbeatAt, notDue.HeartbeatAt)
	}

	clock.advance(r.sweepThreshold() - wsdashboard.HeartbeatInterval)
	writes := heartbeatRegistryIO("create_temp")
	r.sweepHeartbeats()
	if got := heartbeatRegistryIO("create_temp") - writes; got != 1 {
		t.Fatalf("due sweep registration writes = %d, want 1", got)
	}
	after := heartbeatRegistration(t, dir, rec.ReplyID)
	if after.HeartbeatAt != clock.Now().Format(time.RFC3339) || after.HeartbeatAt == before.HeartbeatAt {
		t.Fatalf("heartbeat did not advance: before=%q after=%q", before.HeartbeatAt, after.HeartbeatAt)
	}
	want := before
	want.HeartbeatAt = after.HeartbeatAt
	if after.HostPID != before.HostPID || after.HostStopped != before.HostStopped {
		t.Fatalf("sweep changed host state: before=%+v after=%+v", before, after)
	}
	if !reflect.DeepEqual(after, want) {
		t.Fatalf("sweep changed fields other than HeartbeatAt: want=%+v after=%+v", want, after)
	}

	// A second sweep in the same interval is entirely memory-only.
	touches = wsstate.FileIOCounters()["file_io_total"]
	r.sweepHeartbeats()
	if got := wsstate.FileIOCounters()["file_io_total"] - touches; got != 0 {
		t.Fatalf("non-due sweep made %d file operations, want 0", got)
	}

	releaseLoop()
	<-blockedDone
	// Once the loop runs again, its own heartbeat observes the sweeper's
	// lastReg and performs no duplicate registration write.
	writes = heartbeatRegistryIO("create_temp")
	if !c.do(func() { c.heartbeat() }) {
		t.Fatal("conversation loop ended before heartbeat")
	}
	if got := heartbeatRegistryIO("create_temp") - writes; got != 0 {
		t.Fatalf("loop heartbeat duplicated sweeper write %d times", got)
	}
}

func TestHeartbeatSweepLeavesHealthyDeadlineToLoopRepair(t *testing.T) {
	clock := &testClock{now: time.Unix(1_250, 0).UTC()}
	dir := t.TempDir()
	r := NewRunner(Options{Dir: dir, Now: clock.Now, Log: io.Discard, heartbeat: 30 * time.Second})
	rec := Record{ReplyID: "healthylead", Root: t.TempDir(), Backend: BackendPi, Lead: true, HostPID: 51}
	c := addHeartbeatSweepConv(t, r, rec)
	drifted := heartbeatRegistration(t, dir, rec.ReplyID)
	drifted.Project = "/stale/project"
	if err := wsdashboard.WriteRegistration(dir, drifted); err != nil {
		t.Fatal(err)
	}

	clock.advance(wsdashboard.HeartbeatInterval)
	touches := wsstate.FileIOCounters()["file_io_total"]
	r.sweepHeartbeats()
	if got := wsstate.FileIOCounters()["file_io_total"] - touches; got != 0 {
		t.Fatalf("fallback sweep made %d file operations at healthy loop deadline, want 0", got)
	}
	writes := heartbeatRegistryIO("create_temp")
	if !c.writeRegistration() {
		t.Fatal("healthy loop registration repair failed")
	}
	if got := heartbeatRegistryIO("create_temp") - writes; got != 1 {
		t.Fatalf("healthy loop registration writes = %d, want 1", got)
	}
	repaired := heartbeatRegistration(t, dir, rec.ReplyID)
	if repaired.Project != rec.Root || repaired.HeartbeatAt != clock.Now().Format(time.RFC3339) {
		t.Fatalf("healthy loop did not repair full row: %+v", repaired)
	}
}

func TestHeartbeatSweeperRunsFromStartAndStopsOnClose(t *testing.T) {
	clock := &testClock{now: time.Unix(1_500, 0).UTC()}
	dir := t.TempDir()
	rec := Record{
		ReplyID:   "lifecyclelead",
		Root:      t.TempDir(),
		Backend:   BackendPi,
		WSKey:     "lifecycle-key",
		CreatedAt: clock.Now().Format(time.RFC3339),
		Lead:      true,
		HostPID:   52,
	}
	if err := writeRecord(wsdashboard.SessionDir(dir, rec.ReplyID), rec); err != nil {
		t.Fatal(err)
	}

	r := NewRunner(Options{Dir: dir, Now: clock.Now, Log: io.Discard, heartbeat: 10 * time.Millisecond})
	notified := make(chan string, 16)
	r.Start(func(replyID, _ string) {
		select {
		case notified <- replyID:
		default:
		}
	})
	t.Cleanup(r.Close)
	c := r.conv(rec.ReplyID)
	if c == nil {
		t.Fatal("Start did not load hosted lead")
	}
	// Discard synchronous startup notifications before testing the ticker.
drainStartup:
	for {
		select {
		case <-notified:
		default:
			break drainStartup
		}
	}

	entered := make(chan struct{})
	release := make(chan struct{})
	blockedDone := make(chan struct{})
	var releaseOnce sync.Once
	releaseLoop := func() { releaseOnce.Do(func() { close(release) }) }
	go func() {
		c.do(func() {
			close(entered)
			<-release
		})
		close(blockedDone)
	}()
	<-entered
	t.Cleanup(func() {
		releaseLoop()
		<-blockedDone
	})

	before := heartbeatRegistration(t, dir, rec.ReplyID)
	clock.advance(r.sweepThreshold())
	select {
	case id := <-notified:
		if id != rec.ReplyID {
			t.Fatalf("ticker notified %q, want %q", id, rec.ReplyID)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("ticker-driven heartbeat sweep did not notify")
	}
	after := heartbeatRegistration(t, dir, rec.ReplyID)
	if after.HeartbeatAt == before.HeartbeatAt || after.HeartbeatAt != clock.Now().Format(time.RFC3339) {
		t.Fatalf("ticker heartbeat = %q, before %q, want %q", after.HeartbeatAt, before.HeartbeatAt, clock.Now().Format(time.RFC3339))
	}

	releaseLoop()
	<-blockedDone
	closed := make(chan struct{})
	go func() {
		r.Close()
		close(closed)
	}()
	select {
	case <-closed:
	case <-time.After(5 * time.Second):
		t.Fatal("Runner.Close did not stop heartbeat sweeper promptly")
	}
}

func TestHeartbeatSweepBusyEarlierRowDoesNotStallHealthyRow(t *testing.T) {
	clock := &testClock{now: time.Unix(2_000, 0).UTC()}
	dir := t.TempDir()
	r := NewRunner(Options{Dir: dir, Now: clock.Now, Log: io.Discard})
	// Hyphens are invalid reply-id characters; these retain the requested
	// lexical lead-a-before-lead-b ordering with valid IDs.
	a := addHeartbeatSweepConv(t, r, Record{ReplyID: "leada", Root: t.TempDir(), Backend: BackendPi, Lead: true, HostPID: 11})
	b := addHeartbeatSweepConv(t, r, Record{ReplyID: "leadb", Root: t.TempDir(), Backend: BackendPi, Lead: true, HostPID: 22})
	beforeA := heartbeatRegistration(t, dir, a.rec.ReplyID)
	beforeB := heartbeatRegistration(t, dir, b.rec.ReplyID)

	unlockA, err := wsdashboard.LockRegistration(dir, a.rec.ReplyID)
	if err != nil {
		t.Fatal(err)
	}
	var unlockOnce sync.Once
	releaseA := func() { unlockOnce.Do(unlockA) }
	t.Cleanup(releaseA)
	clock.advance(r.sweepThreshold())

	done := make(chan struct{})
	go func() {
		r.sweepHeartbeats()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(1500 * time.Millisecond):
		releaseA()
		<-done
		t.Fatal("healthy lead waited behind an earlier busy registration flock")
	}
	releaseA()

	afterA := heartbeatRegistration(t, dir, a.rec.ReplyID)
	afterB := heartbeatRegistration(t, dir, b.rec.ReplyID)
	if afterA.HeartbeatAt != beforeA.HeartbeatAt {
		t.Fatalf("busy row heartbeat changed from %q to %q", beforeA.HeartbeatAt, afterA.HeartbeatAt)
	}
	if afterB.HeartbeatAt == beforeB.HeartbeatAt || afterB.HeartbeatAt != clock.Now().Format(time.RFC3339) {
		t.Fatalf("healthy row heartbeat = %q, before %q, want %q", afterB.HeartbeatAt, beforeB.HeartbeatAt, clock.Now().Format(time.RFC3339))
	}
}

func TestHeartbeatSweepSkipsInMemoryIneligibleConversations(t *testing.T) {
	tests := []struct {
		name   string
		lead   bool
		mutate func(*testing.T, *conv)
	}{
		{
			name: "row held",
			lead: true,
			mutate: func(t *testing.T, c *conv) {
				c.rowHeld.Add(1)
				t.Cleanup(func() { c.rowHeld.Add(-1) })
			},
		},
		{name: "non-lead", lead: false},
		{
			name: "closed conversation",
			lead: true,
			mutate: func(_ *testing.T, c *conv) {
				c.mu.Lock()
				c.rec.ClosedAt = time.Unix(3_000, 0).UTC().Format(time.RFC3339)
				c.closed = true
				c.mu.Unlock()
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			clock := &testClock{now: time.Unix(3_000, 0).UTC()}
			dir := t.TempDir()
			r := NewRunner(Options{Dir: dir, Now: clock.Now, Log: io.Discard})
			c := addHeartbeatSweepConv(t, r, Record{ReplyID: "memoryskip", Root: t.TempDir(), Backend: BackendPi, Lead: tc.lead, HostPID: 31})
			before := heartbeatRegistration(t, dir, c.rec.ReplyID)
			if tc.mutate != nil {
				tc.mutate(t, c)
			}
			clock.advance(r.sweepThreshold())
			touches := wsstate.FileIOCounters()["file_io_total"]
			r.sweepHeartbeats()
			if got := wsstate.FileIOCounters()["file_io_total"] - touches; got != 0 {
				t.Fatalf("ineligible sweep made %d file operations, want 0", got)
			}
			after := heartbeatRegistration(t, dir, c.rec.ReplyID)
			if !reflect.DeepEqual(after, before) {
				t.Fatalf("ineligible sweep changed row: before=%+v after=%+v", before, after)
			}
		})
	}
}

func TestHeartbeatSweepRejectsUnsafeRows(t *testing.T) {
	tests := []struct {
		name        string
		mutate      func(*wsdashboard.Registration)
		remove      bool
		wantForeign bool
	}{
		{
			name: "foreign",
			mutate: func(row *wsdashboard.Registration) {
				row.Hosted, row.Agent, row.HostPID = false, false, 0
			},
			wantForeign: true,
		},
		{
			name:   "ended",
			mutate: func(row *wsdashboard.Registration) { row.EndedAt = "2026-11-20T00:00:00Z" },
		},
		{
			name:   "host pid mismatch",
			mutate: func(row *wsdashboard.Registration) { row.HostPID++ },
		},
		{name: "missing", remove: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			clock := &testClock{now: time.Unix(4_000, 0).UTC()}
			dir := t.TempDir()
			r := NewRunner(Options{Dir: dir, Now: clock.Now, Log: io.Discard})
			c := addHeartbeatSweepConv(t, r, Record{ReplyID: "unsaferow", Root: t.TempDir(), Backend: BackendPi, Lead: true, HostPID: 41})
			want := heartbeatRegistration(t, dir, c.rec.ReplyID)
			if tc.remove {
				if err := os.Remove(wsdashboard.RegistrationPath(dir, c.rec.ReplyID)); err != nil {
					t.Fatal(err)
				}
			} else {
				tc.mutate(&want)
				if err := wsdashboard.WriteRegistration(dir, want); err != nil {
					t.Fatal(err)
				}
			}
			clock.advance(r.sweepThreshold())
			writes := heartbeatRegistryIO("create_temp")
			r.sweepHeartbeats()
			if got := heartbeatRegistryIO("create_temp") - writes; got != 0 {
				t.Fatalf("unsafe row was overwritten %d times", got)
			}
			if tc.remove {
				if _, err := wsdashboard.ReadRegistration(dir, c.rec.ReplyID); !os.IsNotExist(err) {
					t.Fatalf("missing row recreated: %v", err)
				}
			} else if got := heartbeatRegistration(t, dir, c.rec.ReplyID); !reflect.DeepEqual(got, want) {
				t.Fatalf("unsafe row changed: want=%+v got=%+v", want, got)
			}
			if c.isForeign() != tc.wantForeign {
				t.Fatalf("foreign=%v, want %v", c.isForeign(), tc.wantForeign)
			}
			if tc.wantForeign {
				touches := wsstate.FileIOCounters()["file_io_total"]
				r.sweepHeartbeats()
				if got := wsstate.FileIOCounters()["file_io_total"] - touches; got != 0 {
					t.Fatalf("known foreign row made %d later file operations, want 0", got)
				}
			}
		})
	}
}

func TestTryLockRegistrationDoesNotWaitForHolder(t *testing.T) {
	dir := t.TempDir()
	id := "trylockrow"
	unlock, err := wsdashboard.LockRegistration(dir, id)
	if err != nil {
		t.Fatal(err)
	}
	tryUnlock, locked, err := wsdashboard.TryLockRegistration(dir, id)
	if err != nil || locked || tryUnlock != nil {
		if tryUnlock != nil {
			tryUnlock()
		}
		unlock()
		t.Fatalf("contended try-lock = unlock %v, locked %v, err %v; want nil, false, nil", tryUnlock != nil, locked, err)
	}
	unlock()

	tryUnlock, locked, err = wsdashboard.TryLockRegistration(dir, id)
	if err != nil || !locked || tryUnlock == nil {
		if tryUnlock != nil {
			tryUnlock()
		}
		t.Fatalf("released try-lock = unlock %v, locked %v, err %v; want non-nil, true, nil", tryUnlock != nil, locked, err)
	}
	tryUnlock()
}
