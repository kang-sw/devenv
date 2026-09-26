package wsindex

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

// Scenario IDs in test names (TestA2..., TestB3..., TestI5..., and so on)
// come from ticket 260924-feat-origin-ticket-ownership-index, whose Results
// map each ID to its test. The IDs are the traceability key; keep them.

var bg = context.Background()

// boundSlack loosens the wall-clock bounds below so they survive -race and a
// loaded machine. Each bound still asserts that a timeout cut the call short:
// the hanging transport never answers until git is killed.
const boundSlack = 3

// A missing ref is reported as absence, never as an error, on every path.
func TestMissingRefIsAbsentNotError(t *testing.T) {
	h := newHarness(t)
	cl := h.clone("x", "x@example.com").client()

	view, err := cl.Read(bg)
	if err != nil || view.State != ViewAbsent {
		t.Fatalf("Read = %s, %v; want absent, nil", view.State, err)
	}
	state, _, err := cl.Check(bg)
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
	if state, _, err := cl.Check(bg); err != nil || state != CheckNoOrigin {
		t.Fatalf("Check = %s, %v", state, err)
	}
	if n := c.runner.remoteCount(); n != 0 {
		t.Fatalf("remote invocations = %d, want 0", n)
	}
}

// hangingSSH makes every ssh transport hang, simulating a remote that never
// answers; the ssh command is configured through core.sshCommand so the
// BatchMode append path is exercised too.
//
// git runs the ssh command through sh, so the path is quoted with forward
// slashes: a raw Windows path loses its backslashes to the shell and the
// "hang" fails fast as an unreachable origin instead of timing out.
func hangingSSH(t *testing.T, c *testClone) {
	t.Helper()
	script := filepath.Join(c.h.dir, "hang-ssh.sh")
	writeExecutable(t, script, hangTransport(t, c.h.dir))
	gitT(t, c.root, "config", "core.sshCommand", shellQuote(filepath.ToSlash(script)))
	c.setOriginURL("ssh://git@wsindex.invalid/repo.git")
}

// hangTransport returns the body of a hanging ssh transport that keeps its
// state under dir, and registers the cleanup that ends every such transport.
// internal/mcp's ixCheckout.hang carries a copy; keep the two in step.
//
// The transport blocks reading git's stdin, so on POSIX the hang lasts until
// the timeout SIGKILLs git's process group, transport included. Windows kills
// only the process started: when that is Git for Windows' git.exe redirector,
// the real git.exe survives holding the pipe open, and it and the transport
// would block forever inside the test tree, failing TempDir's cleanup ("being
// used by another process"). So the transport also ends when the stop file
// appears, or after a backstop well past any timeout a test uses, and the
// cleanup raises the stop file and waits for the live markers to clear. A
// surviving git then reads EOF and exits; TempDir's own retry covers that.
//
// cat runs in the background so the shell can poll; an async command's stdin
// defaults to /dev/null, so git's pipe travels on fd 3. cat writes to
// /dev/null and only the shell holds git's read pipe: closing that pipe early
// (as exec with a redirected stdout does) reads as a dead remote at once. The
// shell leaves the test tree before it blocks.
func hangTransport(t *testing.T, dir string) string {
	t.Helper()
	live := filepath.Join(dir, "hang-live")
	stop := filepath.Join(dir, "hang-stop")
	if err := os.MkdirAll(live, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { stopHangTransports(t, live, stop) })
	return fmt.Sprintf(`live=%s
stop=%s
: >"$live/$$"
exec 3<&0 </dev/null
cd /
cat <&3 >/dev/null 3<&- &
c=$!
exec 3<&-
n=0
while kill -0 "$c" 2>/dev/null; do
	if [ -e "$stop" ] || [ "$n" -ge 120 ]; then
		kill "$c" 2>/dev/null
		break
	fi
	sleep 1
	n=$((n + 1))
done
wait "$c" 2>/dev/null
rm -f "$live/$$"
`, shellQuote(filepath.ToSlash(live)), shellQuote(filepath.ToSlash(stop)))
}

// stopHangTransports raises the stop file and, on Windows, waits for every
// transport to clear its live marker. POSIX has nothing to wait for: the
// process-group kill already ended each transport (before it could clear its
// marker, so the markers there are stale by design).
//
// On POSIX the stop file is therefore not waited on, and TempDir's removal
// deletes it right after. That is safe because every remote call is
// deadline-bound through ExecRunner, and on timeout proc_unix.go SIGKILLs
// git's whole process group, so no transport survives to need the stop
// file. A transport that somehow escaped that kill would miss the stop file
// and end only at the script's 120 s backstop; the backstop is the fallback.
func stopHangTransports(t *testing.T, live, stop string) {
	if err := os.WriteFile(stop, nil, 0o644); err != nil {
		t.Errorf("raise the hang stop file: %v", err)
		return
	}
	if runtime.GOOS != "windows" {
		return
	}
	deadline := time.Now().Add(30 * time.Second)
	for {
		entries, err := os.ReadDir(live)
		if err != nil || len(entries) == 0 {
			return
		}
		if time.Now().After(deadline) {
			t.Errorf("%d hanging ssh transports still running after the stop file", len(entries))
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
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
	// The read bound: the 1.5 s read timeout plus local work (~2.5 s).
	if elapsed := time.Since(start); elapsed > boundSlack*2500*time.Millisecond {
		t.Fatalf("Read took %v, want under the read bound", elapsed)
	}
	if err != nil || view.State != ViewStale || view.Index == nil {
		t.Fatalf("Read = %+v, %v; want stale cache", view, err)
	}
	if view.TimedOut != DefaultReadTimeout {
		t.Fatalf("TimedOut = %v, want the read timeout: a slow origin is not refreshed, not unreachable", view.TimedOut)
	}

	start = time.Now()
	res, _, err := cl.Submit(bg, &Submission{Entry: registerEntry("260924-feat-offline"), Applier: &Applier{Now: h.clock.Now()}})
	// The write bound: the 1.5 s write timeout plus local work (~4 s).
	if elapsed := time.Since(start); elapsed > boundSlack*4*time.Second {
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
	state, _, _ := cl.Check(bg)
	// Fail-fast bound (~3 s): a credential prompt would block until killed.
	if elapsed := time.Since(start); elapsed > boundSlack*3*time.Second {
		t.Fatalf("Check took %v; a credential prompt or hang is not allowed", elapsed)
	}
	if state != CheckUnreachable {
		t.Fatalf("Check = %s, want unreachable (a credential failure is never absence)", state)
	}
	// The environment every remote command ran under is non-interactive.
	if len(c.runner.calls) == 0 {
		t.Fatal("Check made no remote call; the env assertions below would pass vacuously")
	}
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
		// Opened here: client() fails the test with t.Fatalf, which must not
		// run on a spawned goroutine.
		cl := c.client()
		wg.Add(1)
		go func(ci int, cl *Client) {
			defer wg.Done()
			for op := 0; op < perClone; op++ {
				if _, _, err := cl.Submit(bg, &Submission{Entry: registerEntry(fmt.Sprintf("260924-feat-c%d-op%d", ci, op)), Applier: &Applier{Now: h.clock.Now()}}); err != nil {
					errs <- err
				}
			}
		}(ci, cl)
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
	// The read bound (~2.5 s); a fast transport failure is well inside it.
	if time.Since(start) > boundSlack*2500*time.Millisecond {
		t.Fatal("stale fallback exceeded the read bound")
	}
	if err != nil || view.State != ViewStale || view.Index == nil {
		t.Fatalf("Read = %+v, %v; want stale", view, err)
	}
	if view.Age != 5*time.Minute {
		t.Fatalf("Age = %v, want 5m", view.Age)
	}
	if view.TimedOut != 0 {
		t.Fatalf("TimedOut = %v on a fast transport failure, want 0 (unreachable)", view.TimedOut)
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
	clients := make([]*Client, len(ids))
	for i := range clients {
		// Opened before the race: client() may t.Fatalf, which must not run
		// on a spawned goroutine. Open does not touch the clone id.
		c := x
		if i%2 == 1 {
			c = w
		}
		clients[i] = c.client()
	}
	var wg sync.WaitGroup
	for i := range ids {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id, err := clients[i].CloneID(bg)
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
		cl := c.client() // t.Fatalf must not run on a spawned goroutine
		wg.Add(1)
		go func(c *testClone, cl *Client) {
			defer wg.Done()
			for i := 0; i < 5; i++ {
				res, _, err := cl.Submit(bg, &Submission{Entry: registerEntry(fmt.Sprintf("260924-feat-%s-%d", filepath.Base(c.root), i)), Applier: &Applier{Now: h.clock.Now()}})
				if err != nil || res.Status != WritePending {
					t.Errorf("Submit = %+v, %v; want pending", res, err)
				}
			}
		}(c, cl)
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
		cl := c.client() // t.Fatalf must not run on a spawned goroutine
		wg.Add(1)
		go func(i int, cl *Client) {
			defer wg.Done()
			res, err := cl.Create(bg, NewIndex(), "ticket-index: init")
			if err != nil {
				t.Errorf("Create: %v", err)
			}
			created[i] = res.Created
		}(i, cl)
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

// A never-seen clone whose read-path discovery times out caches absence for
// reads only: later reads make no remote call, and the next write still runs
// its own discovery and finds an index present on origin.
func TestNeverSeenReadTimeoutSilencesReadsNotWrites(t *testing.T) {
	h := newHarness(t)
	y := h.clone("y", "y@example.com")
	h.initIndex(y)
	x := h.clone("x", "x@example.com")
	x.opts.ReadTimeout = time.Nanosecond // every read-path discovery times out
	cl := x.client()
	for i := 0; i < 3; i++ {
		if view, err := cl.Read(bg); err != nil || view.State != ViewAbsent {
			t.Fatalf("Read = %s, %v; want absent", view.State, err)
		}
	}
	if n := x.runner.remoteCount(); n != 1 {
		t.Fatalf("remote calls after three reads = %v, want one discovery", x.runner.counts)
	}
	res := h.submit(cl, "260924-feat-x")
	if res.Status != WriteWritten || h.remoteIndex().Registrations["260924-feat-x"] == nil {
		t.Fatalf("write after a read-timeout absence = %+v, want it to discover and register", res)
	}
	if st := cl.loadState(); st.AbsentAt != nil {
		t.Fatalf("a successful discovery left the cached absence: %+v", st)
	}
}

// A discovery that sees the index ref clears a cached absence even when the
// fetch that follows fails: sync's explicit clear is the only thing that
// drops it on that path (markSynced never runs), so without it the next read
// would trust the stale absence and never look again within the TTL.
func TestDiscoveryClearsAbsenceWhenFetchFails(t *testing.T) {
	h := newHarness(t)
	y := h.clone("y", "y@example.com")
	h.initIndex(y)
	x := h.clone("x", "x@example.com")
	x.opts.ReadTimeout = time.Nanosecond // the read-path discovery times out
	if view, err := x.client().Read(bg); err != nil || view.State != ViewAbsent {
		t.Fatalf("Read = %s, %v; want absent", view.State, err)
	}
	x.opts.ReadTimeout = 0
	cl := x.client()
	if st := cl.loadState(); st.AbsentAt == nil || st.AbsentSource != absenceReadTimeout {
		t.Fatalf("state after the timed-out read = %+v, want a cached read-timeout absence", st)
	}

	// The write path's own discovery sees the ref; the fetch after it fails.
	x.runner.setBefore(func(cmd Command) {
		if remoteSubcommand(cmd) == "fetch" {
			x.setOriginURL(unreachableURL)
		}
	})
	if res, _, err := cl.Submit(bg, &Submission{Entry: registerEntry("260924-feat-x"), Applier: &Applier{Now: h.clock.Now()}}); err == nil || !strings.Contains(err.Error(), "could not be fetched") {
		t.Fatalf("Submit = %+v, %v; want the discovered-but-unfetched failure", res, err)
	}
	x.runner.setBefore(nil)
	x.setOriginURL(h.origin)
	if st := cl.loadState(); st.AbsentAt != nil {
		t.Fatalf("a discovery that saw the ref left the cached absence: %+v", st)
	}

	discoveries := x.runner.count("ls-remote")
	view, err := cl.Read(bg)
	if err != nil || view.State != ViewFresh || view.Index == nil {
		t.Fatalf("Read after the failed fetch = %s, %v; want fresh from a new discovery", view.State, err)
	}
	if n := x.runner.count("ls-remote"); n != discoveries+1 {
		t.Fatalf("ls-remote calls by the read = %d, want one discovery", n-discoveries)
	}
}

// A never-seen clone on a hanging origin pays at most one read-path and one
// write-path discovery per absence TTL, however many reads and writes run.
func TestNeverSeenTimeoutsCacheAbsencePerPath(t *testing.T) {
	h := newHarness(t)
	c := h.clone("x", "x@example.com")
	c.opts.ReadTimeout = 200 * time.Millisecond
	c.opts.WriteTimeout = 300 * time.Millisecond
	hangingSSH(t, c)
	cl := c.client()
	read := func() {
		t.Helper()
		if view, err := cl.Read(bg); err != nil || view.State != ViewAbsent {
			t.Fatalf("Read = %s, %v; want absent", view.State, err)
		}
	}
	write := func(stem string) {
		t.Helper()
		if res := h.submit(cl, stem); res.Status != WriteAbsent {
			t.Fatalf("Submit = %+v, want absent", res)
		}
	}
	read()
	read()
	if n := c.runner.count("ls-remote"); n != 1 {
		t.Fatalf("read-path discoveries = %d, want 1", n)
	}
	write("260924-feat-a")
	write("260924-feat-b")
	read()
	write("260924-feat-c")
	if n := c.runner.count("ls-remote"); n != 2 || c.runner.remoteCount() != n {
		t.Fatalf("remote calls = %v, want one read-path and one write-path discovery", c.runner.counts)
	}
	h.clock.Advance(DefaultAbsenceTTL)
	read()
	if n := c.runner.count("ls-remote"); n != 3 {
		t.Fatalf("discoveries after the absence TTL = %d, want a fresh one", n)
	}
}

// C3: re-flushing an override close after a crash between push and pending
// clear finds the lease already closed and writes no second version.
func TestOverrideCloseReflushWritesNothing(t *testing.T) {
	h := newHarness(t)
	y := h.clone("y", "y@example.com")
	h.initIndex(y)
	holder := Owner{Email: "y@example.com", CloneID: "cy", Track: "develop"}
	if _, _, err := y.client().Submit(bg, &Submission{Entry: PendingEntry{Op: OpAcquire, Stem: "260924-feat-a", Owner: holder}, Applier: &Applier{Now: h.clock.Now()}}); err != nil {
		t.Fatal(err)
	}
	x := h.clone("x", "x@example.com")
	cl := x.client()
	if view, err := cl.Read(bg); err != nil || view.State != ViewFresh {
		t.Fatalf("Read = %s, %v", view.State, err)
	}
	x.setOriginURL(unreachableURL)
	actor := Owner{Email: "x@example.com", CloneID: "cx", Track: "develop"}
	res, _, err := cl.Submit(bg, &Submission{
		Entry:   PendingEntry{Op: OpClose, Stem: "260924-feat-a", Owner: actor, Override: &Override{Reason: "user closed it"}},
		Applier: &Applier{Now: h.clock.Now()},
	})
	if err != nil || res.Status != WritePending {
		t.Fatalf("offline override close = %+v, %v", res, err)
	}
	logBefore := gitT(t, x.root, "rev-parse", PendingRef)
	x.setOriginURL(h.origin)
	flush := WriteOp{Subject: "flush", Replay: (&Applier{Now: h.clock.Now()}).Replay}
	if res, err := cl.Write(bg, flush); err != nil || res.Status != WriteWritten {
		t.Fatalf("flush = %+v, %v", res, err)
	}
	if l := h.remoteIndex().Registrations["260924-feat-a"].Lease; l == nil || l.Owner() != holder || l.Phase != PhaseClosed {
		t.Fatalf("the first flush did not close the holder's lease: %+v", l)
	}
	if n := strings.Count(gitT(t, h.origin, "log", "--format=%B", RemoteRef), "under override"); n != 1 {
		t.Fatalf("override audit lines after the first flush = %d, want 1", n)
	}
	tip := h.remoteTip()
	gitT(t, x.root, "update-ref", PendingRef, logBefore) // the crash: pushed, never cleared
	res, err = cl.Write(bg, flush)
	if err != nil || res.Status != WriteNoChange || res.Flushed != 1 || len(res.Reports) != 0 {
		t.Fatalf("re-flush = %+v, %v; want a silent no-change clear", res, err)
	}
	if h.remoteTip() != tip {
		t.Fatal("the re-flushed override close wrote a second version")
	}
}

// A non-acquire online LoadContext (no fetchTrack, no pending acquire) fills
// the origin-closed set from the local remote-tracking ref of the
// review-track with no remote call: no fetch, and no default-branch probe
// since the clone has a local refs/remotes/origin/HEAD.
func TestLoadContextReadsClosedFromLocalTrackingRefWithoutFetch(t *testing.T) {
	h := newHarness(t)
	x := h.clone("x", "x@example.com")
	seed := filepath.Join(h.dir, "seed")
	if err := os.MkdirAll(filepath.Join(seed, "ai-docs", "tickets", ".done"), 0o755); err != nil {
		t.Fatal(err)
	}
	gitT(t, seed, "mv", "ai-docs/tickets/ready/260101-feat-seed.md", "ai-docs/tickets/.done/260101-feat-seed.md")
	gitT(t, seed, "commit", "--quiet", "-m", "close seed")
	gitT(t, seed, "push", "--quiet", "origin", "main")
	gitT(t, x.root, "fetch", "--quiet", "origin") // x's tracking ref sees the first closure
	writeFile(t, filepath.Join(seed, "ai-docs", "tickets", ".done", "260102-feat-later.md"), "# later\n")
	gitT(t, seed, "add", "-A")
	gitT(t, seed, "commit", "--quiet", "-m", "close later")
	gitT(t, seed, "push", "--quiet", "origin", "main") // x never fetches this one

	cl := x.client()
	sub := &Submission{Entry: registerEntry("260924-feat-x"), Applier: &Applier{Now: h.clock.Now()}}
	if err := cl.LoadContext(bg, sub, true, false); err != nil {
		t.Fatal(err)
	}
	if !sub.Applier.Closed["260101-feat-seed"] {
		t.Fatalf("Closed = %v, want the stem closed in the local tracking ref", sub.Applier.Closed)
	}
	if sub.Applier.Closed["260102-feat-later"] {
		t.Fatal("Closed holds a closure only origin has: the tracking ref was fetched")
	}
	if x.runner.count("fetch") != 0 || x.runner.count("ls-remote") != 0 || x.runner.remoteCount() != 0 {
		t.Fatalf("remote calls = %v, want none", x.runner.counts)
	}

	// Contrast: an acquire (fetchTrack) refreshes the tracking ref first.
	sub = &Submission{Entry: registerEntry("260924-feat-x"), Applier: &Applier{Now: h.clock.Now()}}
	if err := cl.LoadContext(bg, sub, true, true); err != nil {
		t.Fatal(err)
	}
	if x.runner.count("fetch") != 1 || !sub.Applier.Closed["260102-feat-later"] {
		t.Fatalf("fetchTrack: remote calls = %v, Closed = %v; want one fetch and the later closure", x.runner.counts, sub.Applier.Closed)
	}
}

// writeExecutable writes an executable shell script (a hook, a fake ssh or
// signing program).
func writeExecutable(t *testing.T, path, body string) {
	t.Helper()
	writeFile(t, path, "#!/bin/sh\n"+body)
	if err := os.Chmod(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

// Index pushes bypass the clone's pre-push hook (push --no-verify): a hook
// that vetoes every push still lets the index write land.
func TestIndexPushBypassesPrePushHook(t *testing.T) {
	h := newHarness(t)
	c := h.clone("x", "x@example.com")
	writeExecutable(t, filepath.Join(c.root, ".git", "hooks", "pre-push"), "echo 'pre-push: blocked' >&2\nexit 1\n")
	if out, err := exec.Command("git", "-C", c.root, "push", "--quiet", "origin", "HEAD:refs/heads/probe").CombinedOutput(); err == nil {
		t.Fatalf("the pre-push hook did not veto a plain push: %s", out)
	}
	h.initIndex(c)
	if res := h.submit(c.client(), "260924-feat-hooked"); res.Status != WriteWritten {
		t.Fatalf("Submit = %+v, want written past the pre-push hook", res)
	}
	if h.remoteIndex().Registrations["260924-feat-hooked"] == nil {
		t.Fatal("the index write did not land")
	}
}

// Index commits are never signed: commit.gpgSign with a signing program that
// always fails still lets the index write land. Current git's commit-tree
// already ignores commit.gpgSign, so the outcome alone passes without the
// flag; the recorded commit-tree arguments pin --no-gpg-sign itself, which is
// what guards against a git that honors commit.gpgSign there.
func TestIndexCommitIgnoresSigningConfig(t *testing.T) {
	h := newHarness(t)
	c := h.clone("x", "x@example.com")
	signer := filepath.Join(h.dir, "bogus-gpg.sh")
	writeExecutable(t, signer, "echo 'bogus-gpg: no key' >&2\nexit 1\n")
	gitT(t, c.root, "config", "commit.gpgSign", "true")
	gitT(t, c.root, "config", "gpg.program", signer)
	if out, err := exec.Command("git", "-C", c.root, "commit", "--quiet", "--allow-empty", "-m", "probe").CombinedOutput(); err == nil {
		t.Fatalf("the signing config did not break a plain commit: %s", out)
	}
	var mu sync.Mutex
	var commitTrees [][]string
	c.runner.setBefore(func(cmd Command) {
		if remoteSubcommand(cmd) == "commit-tree" {
			mu.Lock()
			commitTrees = append(commitTrees, cmd.Args)
			mu.Unlock()
		}
	})
	h.initIndex(c)
	if res := h.submit(c.client(), "260924-feat-signed"); res.Status != WriteWritten {
		t.Fatalf("Submit = %+v, want written despite the signing config", res)
	}
	if h.remoteIndex().Registrations["260924-feat-signed"] == nil {
		t.Fatal("the index write did not land")
	}
	mu.Lock()
	defer mu.Unlock()
	if len(commitTrees) == 0 {
		t.Fatal("no commit-tree invocation was recorded; the argument check below would pass vacuously")
	}
	for _, args := range commitTrees {
		if !slices.Contains(args, "--no-gpg-sign") {
			t.Errorf("commit-tree ran without --no-gpg-sign: %v", args)
		}
	}
}

// A permanent remote refusal (a pre-receive hook declining the index ref) is
// reported as refused after exactly one push; it is not a lost CAS race, so
// it is neither retried nor recorded offline.
func TestIndexPushRefusalIsNotRetried(t *testing.T) {
	h := newHarness(t)
	c := h.clone("x", "x@example.com")
	h.initIndex(c)
	tip := h.remoteTip()
	writeExecutable(t, filepath.Join(h.origin, "hooks", "pre-receive"),
		"while read old new ref; do\n\tif [ \"$ref\" = \""+RemoteRef+"\" ]; then\n\t\techo 'index writes are disabled here' >&2\n\t\texit 1\n\tfi\ndone\nexit 0\n")
	cl := c.client()
	pushesBefore := c.runner.count("push")
	res, _, err := cl.Submit(bg, &Submission{Entry: registerEntry("260924-feat-refused"), Applier: &Applier{Now: h.clock.Now()}})
	if err == nil || errors.Is(err, ErrRetryExhausted) || !strings.Contains(err.Error(), "refused") {
		t.Fatalf("Submit = %+v, %v; want a refused error", res, err)
	}
	if pushes := c.runner.count("push") - pushesBefore; pushes != 1 {
		t.Fatalf("pushes = %d, want exactly 1 (a permanent refusal is not retried; max attempts %d)", pushes, DefaultMaxAttempts)
	}
	if h.remoteTip() != tip {
		t.Fatal("the refused write moved the remote index")
	}
	if pending, _ := cl.Pending(bg); len(pending) != 0 {
		t.Fatalf("a refusal recorded %d pending entries; it is not an offline write", len(pending))
	}
}
