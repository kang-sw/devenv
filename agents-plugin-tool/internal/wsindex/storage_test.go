package wsindex

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

var bg = context.Background()

// A missing ref is reported as absence, never as an error, on every path.
func TestMissingRefIsAbsentNotError(t *testing.T) {
	h := newHarness(t)
	cl := h.clone("x", "x@example.com").client()

	view, err := cl.Read(bg)
	if err != nil || view.State != ViewAbsent {
		t.Fatalf("Read = %s, %v; want absent, nil", view.State, err)
	}
	state, err := cl.Check(bg)
	if err != nil || state != CheckUninitialized {
		t.Fatalf("Check = %s, %v; want uninitialized, nil", state, err)
	}
	res, _, err := cl.Submit(bg, &Submission{Entry: registerEntry("260924-feat-a"), Applier: &Applier{Now: h.clock.Now()}})
	if err != nil || res.Status != WriteAbsent {
		t.Fatalf("Submit = %+v, %v; want absent, nil", res, err)
	}
	if h.remoteTip() != "" {
		t.Fatal("an index-absent write created the remote ref")
	}
}

// A2: within the absence TTL, repeated calls make at most one remote
// discovery call.
func TestA2AbsenceCachedWithinTTL(t *testing.T) {
	h := newHarness(t)
	c := h.clone("x", "x@example.com")
	for i := 0; i < 3; i++ {
		cl := c.client()
		h.submit(cl, fmt.Sprintf("260924-feat-a%d", i))
		if view, err := cl.Read(bg); err != nil || view.State != ViewAbsent {
			t.Fatalf("Read = %s, %v", view.State, err)
		}
		h.clock.Advance(time.Minute)
	}
	if n := c.runner.remoteCount(); n != 1 {
		t.Fatalf("remote invocations = %d (%v), want 1", n, c.runner.counts)
	}
}

// A3: no origin remote means index-absent with no error and no remote call.
func TestA3NoOriginIsSilentlyAbsent(t *testing.T) {
	h := newHarness(t)
	root := filepath.Join(h.dir, "local-only")
	gitT(t, h.dir, "init", "--quiet", root)
	c := &testClone{h: h, root: root, runner: newCountingRunner()}
	cl := c.client()
	if view, err := cl.Read(bg); err != nil || view.State != ViewAbsent {
		t.Fatalf("Read = %s, %v", view.State, err)
	}
	if res, _, err := cl.Submit(bg, &Submission{Entry: registerEntry("260924-feat-a"), Applier: &Applier{Now: h.clock.Now()}}); err != nil || res.Status != WriteAbsent {
		t.Fatalf("Submit = %+v, %v", res, err)
	}
	if state, err := cl.Check(bg); err != nil || state != CheckNoOrigin {
		t.Fatalf("Check = %s, %v", state, err)
	}
	if n := c.runner.remoteCount(); n != 0 {
		t.Fatalf("remote invocations = %d, want 0", n)
	}
}

// hangingSSH makes every ssh transport hang, simulating a remote that never
// answers; the ssh command is configured through core.sshCommand so the
// BatchMode append path is exercised too.
func hangingSSH(t *testing.T, c *testClone) {
	t.Helper()
	script := filepath.Join(c.h.dir, "hang-ssh.sh")
	writeFile(t, script, "#!/bin/sh\nexec sleep 30\n")
	if err := os.Chmod(script, 0o755); err != nil {
		t.Fatal(err)
	}
	gitT(t, c.root, "config", "core.sshCommand", script)
	c.setOriginURL("ssh://git@wsindex.invalid/repo.git")
}

// A4 (library level): with a cache ref and a hanging remote, the read path
// returns within its bound serving the stale cache, and a mutating write
// returns within the write timeout with its registration in the pending log.
func TestA4UnreachableWithCacheIsBoundedAndPending(t *testing.T) {
	h := newHarness(t)
	c := h.clone("x", "x@example.com")
	c.opts.WriteTimeout = 1500 * time.Millisecond
	h.initIndex(c)
	h.clock.Advance(2 * DefaultReadTTL)
	hangingSSH(t, c)
	cl := c.client()

	start := time.Now()
	view, err := cl.Read(bg)
	if elapsed := time.Since(start); elapsed > 2500*time.Millisecond {
		t.Fatalf("Read took %v, want under the read bound", elapsed)
	}
	if err != nil || view.State != ViewStale || view.Index == nil {
		t.Fatalf("Read = %+v, %v; want stale cache", view, err)
	}

	start = time.Now()
	res, _, err := cl.Submit(bg, &Submission{Entry: registerEntry("260924-feat-offline"), Applier: &Applier{Now: h.clock.Now()}})
	if elapsed := time.Since(start); elapsed > 4*time.Second {
		t.Fatalf("Submit took %v, want within the write timeout", elapsed)
	}
	if err != nil || res.Status != WritePending {
		t.Fatalf("Submit = %+v, %v; want pending", res, err)
	}
	pending, err := cl.Pending(bg)
	if err != nil || len(pending) != 1 || pending[0].Stem != "260924-feat-offline" {
		t.Fatalf("pending = %+v, %v", pending, err)
	}
}

// A5: a credential-requiring remote never prompts and fails fast.
func TestA5CredentialRemoteFailsFastWithoutPrompt(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("WWW-Authenticate", `Basic realm="wsindex"`)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()
	h := newHarness(t)
	c := h.clone("x", "x@example.com")
	c.setOriginURL(srv.URL + "/repo.git")
	cl := c.client()

	start := time.Now()
	state, _ := cl.Check(bg)
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Fatalf("Check took %v; a credential prompt or hang is not allowed", elapsed)
	}
	if state != CheckUnreachable {
		t.Fatalf("Check = %s, want unreachable (a credential failure is never absence)", state)
	}
	// The environment every remote command ran under is non-interactive.
	for _, cmd := range c.runner.calls {
		env := strings.Join(cmd.Env, "\n")
		for _, want := range []string{"GIT_TERMINAL_PROMPT=0", "GIT_ASKPASS=", "BatchMode=yes"} {
			if !strings.Contains(env, want) {
				t.Fatalf("remote command %v env lacks %q", cmd.Args, want)
			}
		}
	}
}

// The configured ssh command is preserved, with BatchMode appended.
func TestBatchSSHCommandPreservesConfiguredCommand(t *testing.T) {
	h := newHarness(t)
	c := h.clone("x", "x@example.com")
	cl := c.client()
	if got := cl.batchSSHCommand(bg); got != "ssh -o BatchMode=yes -o ConnectTimeout=5" {
		t.Fatalf("default = %q", got)
	}
	gitT(t, c.root, "config", "core.sshCommand", "ssh -i /k/id")
	if got := cl.batchSSHCommand(bg); got != "ssh -i /k/id -o BatchMode=yes -o ConnectTimeout=5" {
		t.Fatalf("core.sshCommand = %q", got)
	}
	cl.getenv = func(k string) string {
		if k == "GIT_SSH_COMMAND" {
			return "myssh -p 22"
		}
		return ""
	}
	if got := cl.batchSSHCommand(bg); got != "myssh -p 22 -o BatchMode=yes -o ConnectTimeout=5" {
		t.Fatalf("GIT_SSH_COMMAND = %q", got)
	}
	cl.getenv = func(k string) string {
		if k == "GIT_SSH" {
			return `C:\tools\plink.exe`
		}
		return ""
	}
	gitT(t, c.root, "config", "--unset", "core.sshCommand")
	if got := cl.batchSSHCommand(bg); got != `'C:\tools\plink.exe' -batch` {
		t.Fatalf("GIT_SSH plink = %q", got)
	}
}

// A9: an index another clone creates after this clone cached absence is
// discovered once the absence TTL expires.
func TestA9IndexDiscoveredAfterAbsenceTTL(t *testing.T) {
	h := newHarness(t)
	x := h.clone("x", "x@example.com")
	y := h.clone("y", "y@example.com")
	if view, _ := x.client().Read(bg); view.State != ViewAbsent {
		t.Fatalf("x Read = %s, want absent", view.State)
	}
	h.initIndex(y)
	h.submit(y.client(), "260924-feat-y")

	h.clock.Advance(DefaultAbsenceTTL / 2)
	before := x.runner.remoteCount()
	if view, _ := x.client().Read(bg); view.State != ViewAbsent {
		t.Fatalf("x Read within TTL = %s, want cached absent", view.State)
	}
	if x.runner.remoteCount() != before {
		t.Fatal("a read within the absence TTL contacted the remote")
	}
	h.clock.Advance(DefaultAbsenceTTL)
	view, err := x.client().Read(bg)
	if err != nil || view.State != ViewFresh || view.Index.Registrations["260924-feat-y"] == nil {
		t.Fatalf("x Read after TTL = %+v, %v; want the new index", view, err)
	}
}

// assertLinearChain checks every index version's parent is its predecessor.
func assertLinearChain(t *testing.T, h *harness) int {
	t.Helper()
	lines := strings.Split(gitT(t, h.origin, "rev-list", "--parents", RemoteRef), "\n")
	for i, line := range lines {
		fields := strings.Fields(line)
		wantParents := 1
		if i == len(lines)-1 {
			wantParents = 0
		}
		if len(fields)-1 != wantParents {
			t.Fatalf("version %s has %d parents, want %d", fields[0], len(fields)-1, wantParents)
		}
		if wantParents == 1 && fields[1] != strings.Fields(lines[i+1])[0] {
			t.Fatalf("version %s parent %s is not its predecessor", fields[0], fields[1])
		}
	}
	return len(lines)
}

func concurrentWrites(t *testing.T, h *harness, clones []*testClone, perClone int) map[string]bool {
	t.Helper()
	want := map[string]bool{}
	var wg sync.WaitGroup
	errs := make(chan error, len(clones)*perClone)
	for ci, c := range clones {
		for op := 0; op < perClone; op++ {
			want[fmt.Sprintf("260924-feat-c%d-op%d", ci, op)] = true
		}
		wg.Add(1)
		go func(ci int, c *testClone) {
			defer wg.Done()
			cl := c.client()
			for op := 0; op < perClone; op++ {
				if _, _, err := cl.Submit(bg, &Submission{Entry: registerEntry(fmt.Sprintf("260924-feat-c%d-op%d", ci, op)), Applier: &Applier{Now: h.clock.Now()}}); err != nil {
					errs <- err
				}
			}
		}(ci, c)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("concurrent write: %v", err)
	}
	return want
}

// B2: two clones writing different stems concurrently both land.
func TestB2ConcurrentDifferentStemsBothLand(t *testing.T) {
	h := newHarness(t)
	a, b := h.clone("a", "a@example.com"), h.clone("b", "b@example.com")
	a.opts.MaxAttempts, b.opts.MaxAttempts = 50, 50
	h.initIndex(a)
	want := concurrentWrites(t, h, []*testClone{a, b}, 3)
	idx := h.remoteIndex()
	for stem := range want {
		if idx.Registrations[stem] == nil {
			t.Fatalf("lost update: %s missing", stem)
		}
	}
	assertLinearChain(t, h)
}

// B3: N clones x M operations converge to the serial union on a linear chain.
func TestB3ManyWritersConvergeToUnion(t *testing.T) {
	h := newHarness(t)
	var clones []*testClone
	for i := 0; i < 4; i++ {
		c := h.clone(fmt.Sprintf("c%d", i), fmt.Sprintf("c%d@example.com", i))
		c.opts.MaxAttempts = 100
		clones = append(clones, c)
	}
	h.initIndex(clones[0])
	want := concurrentWrites(t, h, clones, 4)
	idx := h.remoteIndex()
	if len(idx.Registrations) != len(want) {
		t.Fatalf("registrations = %d, want %d", len(idx.Registrations), len(want))
	}
	for stem := range want {
		if idx.Registrations[stem] == nil {
			t.Fatalf("lost update: %s missing", stem)
		}
	}
	if versions := assertLinearChain(t, h); versions != len(want)+1 {
		t.Fatalf("versions = %d, want %d (init + one per write)", versions, len(want)+1)
	}
}

// B6: a persistently lost CAS retries a bounded number of times, returns a
// clear error, and leaves the local cache pointing at a remote-observed
// version.
func TestB6PersistentRejectionIsBounded(t *testing.T) {
	h := newHarness(t)
	a, b := h.clone("a", "a@example.com"), h.clone("b", "b@example.com")
	h.initIndex(a)
	bClient := b.client()
	h.submit(bClient, "260924-feat-b-warmup")
	competitor := 0
	pushesBefore := a.runner.count("push")
	a.runner.setBefore(func(cmd Command) {
		if remoteSubcommand(cmd) != "push" {
			return
		}
		competitor++
		h.submit(bClient, fmt.Sprintf("260924-feat-b-%d", competitor))
	})
	_, _, err := a.client().Submit(bg, &Submission{Entry: registerEntry("260924-feat-a"), Applier: &Applier{Now: h.clock.Now()}})
	if !errors.Is(err, ErrRetryExhausted) {
		t.Fatalf("Submit error = %v, want ErrRetryExhausted", err)
	}
	if pushes := a.runner.count("push") - pushesBefore; pushes != DefaultMaxAttempts {
		t.Fatalf("pushes = %d, want %d", pushes, DefaultMaxAttempts)
	}
	cache := gitT(t, a.root, "rev-parse", CacheRef)
	gitT(t, h.origin, "cat-file", "-e", cache+"^{commit}")
	gitT(t, h.origin, "merge-base", "--is-ancestor", cache, RemoteRef)
	if pending, _ := a.client().Pending(bg); len(pending) != 0 {
		t.Fatalf("a lost race recorded %d pending entries; it is not an offline write", len(pending))
	}
	if h.remoteIndex().Registrations["260924-feat-a"] != nil {
		t.Fatal("the exhausted write landed anyway")
	}
}

// F1: within the read TTL, a read makes no remote fetch.
func TestF1ReadWithinTTLMakesNoFetch(t *testing.T) {
	h := newHarness(t)
	c := h.clone("x", "x@example.com")
	h.initIndex(c)
	cl := c.client()
	h.clock.Advance(2 * DefaultReadTTL)
	if view, err := cl.Read(bg); err != nil || view.State != ViewFresh {
		t.Fatalf("Read = %s, %v", view.State, err)
	}
	before := c.runner.remoteCount()
	h.clock.Advance(DefaultReadTTL / 2)
	if view, err := cl.Read(bg); err != nil || view.State != ViewFresh {
		t.Fatalf("Read = %s, %v", view.State, err)
	}
	if c.runner.remoteCount() != before {
		t.Fatal("a read within the TTL contacted the remote")
	}
}

// F2: TTL expired and the fetch fails: the stale cache is served with its age
// within the bound.
func TestF2StaleCacheServedWithAge(t *testing.T) {
	h := newHarness(t)
	c := h.clone("x", "x@example.com")
	h.initIndex(c)
	h.clock.Advance(5 * time.Minute)
	c.setOriginURL(unreachableURL)
	start := time.Now()
	view, err := c.client().Read(bg)
	if time.Since(start) > 2500*time.Millisecond {
		t.Fatal("stale fallback exceeded the read bound")
	}
	if err != nil || view.State != ViewStale || view.Index == nil {
		t.Fatalf("Read = %+v, %v; want stale", view, err)
	}
	if view.Age != 5*time.Minute {
		t.Fatalf("Age = %v, want 5m", view.Age)
	}
}

// F3: a write in worktree A is visible to worktree B of the same clone
// through the shared cache ref, without a fetch.
func TestF3SiblingWorktreeSeesWriteWithoutFetch(t *testing.T) {
	h := newHarness(t)
	a := h.clone("x", "x@example.com")
	h.initIndex(a)
	b := a.worktree("x-b", "track-b")
	h.submit(a.client(), "260924-feat-from-a")
	view, err := b.client().Read(bg)
	if err != nil || view.State != ViewFresh || view.Index.Registrations["260924-feat-from-a"] == nil {
		t.Fatalf("B Read = %+v, %v", view, err)
	}
	if n := b.runner.remoteCount(); n != 0 {
		t.Fatalf("B made %d remote calls, want 0", n)
	}
}

// F4: another clone's write is visible after TTL expiry.
func TestF4OtherCloneWriteVisibleAfterTTL(t *testing.T) {
	h := newHarness(t)
	x, y := h.clone("x", "x@example.com"), h.clone("y", "y@example.com")
	h.initIndex(x)
	xc := x.client()
	if _, err := xc.Read(bg); err != nil {
		t.Fatal(err)
	}
	h.submit(y.client(), "260924-feat-y")
	if view, _ := xc.Read(bg); view.Index.Registrations["260924-feat-y"] != nil {
		t.Fatal("within the TTL the read should serve the cache")
	}
	h.clock.Advance(DefaultReadTTL + time.Second)
	if view, _ := xc.Read(bg); view.State != ViewFresh || view.Index.Registrations["260924-feat-y"] == nil {
		t.Fatalf("after TTL = %+v, want y's registration", view)
	}
}

// C9: clone_id is generated once, shared by later worktrees, and distinct per
// clone, even when two worktrees race the first call.
func TestC9CloneIDOncePerClone(t *testing.T) {
	h := newHarness(t)
	x := h.clone("x", "x@example.com")
	w := x.worktree("x-w", "track-w")
	ids := make([]string, 8)
	var wg sync.WaitGroup
	for i := range ids {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			c := x
			if i%2 == 1 {
				c = w
			}
			id, err := c.client().CloneID(bg)
			if err != nil {
				t.Error(err)
			}
			ids[i] = id
		}(i)
	}
	wg.Wait()
	for _, id := range ids {
		if id == "" || id != ids[0] {
			t.Fatalf("clone ids diverged: %v", ids)
		}
	}
	later := x.worktree("x-later", "track-later")
	if id, _ := later.client().CloneID(bg); id != ids[0] {
		t.Fatalf("later worktree id %s != %s", id, ids[0])
	}
	y := h.clone("y", "x@example.com")
	if id, _ := y.client().CloneID(bg); id == ids[0] {
		t.Fatal("two clones of one person share a clone id")
	}
}

// offlineClone initializes the index from c, then points c at an unreachable
// origin so its writes go to the pending log.
func offlineClone(t *testing.T, h *harness, c *testClone) {
	t.Helper()
	h.initIndex(c)
	c.setOriginURL(unreachableURL)
}

// I5: two worktrees of one clone record pending entries concurrently while
// offline; both survive and flush in recorded order.
func TestI5ConcurrentPendingAppendsSurviveAndFlushInOrder(t *testing.T) {
	h := newHarness(t)
	a := h.clone("x", "x@example.com")
	offlineClone(t, h, a)
	b := a.worktree("x-b", "track-b")
	var wg sync.WaitGroup
	for _, c := range []*testClone{a, b} {
		wg.Add(1)
		go func(c *testClone) {
			defer wg.Done()
			cl := c.client()
			for i := 0; i < 5; i++ {
				res, _, err := cl.Submit(bg, &Submission{Entry: registerEntry(fmt.Sprintf("260924-feat-%s-%d", filepath.Base(c.root), i)), Applier: &Applier{Now: h.clock.Now()}})
				if err != nil || res.Status != WritePending {
					t.Errorf("Submit = %+v, %v; want pending", res, err)
				}
			}
		}(c)
	}
	wg.Wait()
	cl := a.client()
	pending, err := cl.Pending(bg)
	if err != nil || len(pending) != 10 {
		t.Fatalf("pending = %d, %v; want 10", len(pending), err)
	}
	a.setOriginURL(h.origin)
	var order []string
	apply := Apply(h.clock.Now())
	recording := func(idx *Index, e PendingEntry) ([]string, string) {
		order = append(order, e.ID)
		return apply(idx, e)
	}
	res, err := cl.Write(bg, WriteOp{Subject: "flush", Replay: recording})
	if err != nil || res.Status != WriteWritten || res.Flushed != 10 {
		t.Fatalf("flush = %+v, %v", res, err)
	}
	for i, e := range pending {
		if order[i] != e.ID {
			t.Fatalf("replay order %v differs from recorded order", order)
		}
	}
	if left, _ := cl.Pending(bg); len(left) != 0 {
		t.Fatalf("pending after flush = %d", len(left))
	}
	if n := len(h.remoteIndex().Registrations); n != 10 {
		t.Fatalf("remote registrations = %d, want 10", n)
	}
}

// I6: a crash after the remote push but before the local clear re-flushes
// idempotently: no duplicate records, no error, no report.
func TestI6FlushIsIdempotentAfterCrashBeforeClear(t *testing.T) {
	h := newHarness(t)
	c := h.clone("x", "x@example.com")
	offlineClone(t, h, c)
	cl := c.client()
	h.submit(cl, "260924-feat-one")
	h.submit(cl, "260924-feat-two")
	logBefore := gitT(t, c.root, "rev-parse", PendingRef)
	c.setOriginURL(h.origin)
	if res, err := cl.Write(bg, WriteOp{Subject: "flush", Replay: Apply(h.clock.Now())}); err != nil || res.Status != WriteWritten {
		t.Fatalf("flush = %+v, %v", res, err)
	}
	tipAfterFirst := h.remoteTip()
	// Simulate the crash: the push landed but the clear never happened.
	gitT(t, c.root, "update-ref", PendingRef, logBefore)

	res, err := cl.Write(bg, WriteOp{Subject: "flush", Replay: Apply(h.clock.Now())})
	if err != nil || res.Status != WriteNoChange || len(res.Reports) != 0 || res.Flushed != 2 {
		t.Fatalf("re-flush = %+v, %v; want a silent no-change clear", res, err)
	}
	if h.remoteTip() != tipAfterFirst {
		t.Fatal("the idempotent re-flush pushed a new version")
	}
	if n := len(h.remoteIndex().Registrations); n != 2 {
		t.Fatalf("registrations = %d, want 2", n)
	}
	if left, _ := cl.Pending(bg); len(left) != 0 {
		t.Fatalf("pending after re-flush = %d", len(left))
	}
}

// I7: an entry worktree B appends while worktree A flushes is not lost; it
// flushes on the next mutating call.
func TestI7AppendDuringFlushIsKept(t *testing.T) {
	h := newHarness(t)
	a := h.clone("x", "x@example.com")
	offlineClone(t, h, a)
	b := a.worktree("x-b", "track-b")
	h.submit(a.client(), "260924-feat-a")
	a.setOriginURL(h.origin)
	bClient := b.client()
	appended := false
	a.runner.setBefore(func(cmd Command) {
		if remoteSubcommand(cmd) == "push" && !appended {
			appended = true
			if _, err := bClient.RecordPending(bg, registerEntry("260924-feat-b")); err != nil {
				t.Errorf("RecordPending: %v", err)
			}
		}
	})
	res, err := a.client().Write(bg, WriteOp{Subject: "flush", Replay: Apply(h.clock.Now())})
	if err != nil || res.Status != WriteWritten || res.Flushed != 1 {
		t.Fatalf("flush = %+v, %v", res, err)
	}
	left, _ := a.client().Pending(bg)
	if len(left) != 1 || left[0].Stem != "260924-feat-b" {
		t.Fatalf("pending after flush = %+v, want b's entry kept", left)
	}
	a.runner.setBefore(nil)
	if res, err := a.client().Write(bg, WriteOp{Subject: "flush", Replay: Apply(h.clock.Now())}); err != nil || res.Flushed != 1 {
		t.Fatalf("second flush = %+v, %v", res, err)
	}
	if h.remoteIndex().Registrations["260924-feat-b"] == nil {
		t.Fatal("b's entry never reached the remote")
	}
}

// Create adopts an index another clone created first (A6 at library level).
func TestCreateAdoptsConcurrentCreation(t *testing.T) {
	h := newHarness(t)
	var clones []*testClone
	for i := 0; i < 4; i++ {
		clones = append(clones, h.clone(fmt.Sprintf("c%d", i), fmt.Sprintf("c%d@example.com", i)))
	}
	var wg sync.WaitGroup
	created := make([]bool, len(clones))
	for i, c := range clones {
		wg.Add(1)
		go func(i int, c *testClone) {
			defer wg.Done()
			res, err := c.client().Create(bg, NewIndex(), "ticket-index: init")
			if err != nil {
				t.Errorf("Create: %v", err)
			}
			created[i] = res.Created
		}(i, c)
	}
	wg.Wait()
	n := 0
	for _, ok := range created {
		if ok {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("%d clones created the ref, want exactly 1", n)
	}
	for _, c := range clones {
		if gitT(t, c.root, "rev-parse", CacheRef) != h.remoteTip() {
			t.Fatal("a clone did not adopt the created index")
		}
	}
}
