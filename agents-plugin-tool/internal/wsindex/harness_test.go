package wsindex

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestMain isolates every git invocation from the developer's global and
// system config (credential helpers, url rewrites, hooks paths).
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "wsindex-gitconfig")
	if err != nil {
		panic(err)
	}
	global := filepath.Join(dir, "gitconfig")
	if err := os.WriteFile(global, []byte("[init]\n\tdefaultBranch = main\n[protocol \"file\"]\n\tallow = always\n"), 0o644); err != nil {
		panic(err)
	}
	os.Setenv("GIT_CONFIG_GLOBAL", global)
	os.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	os.Setenv("GIT_AUTHOR_NAME", "test")
	os.Setenv("GIT_AUTHOR_EMAIL", "test@example.com")
	os.Setenv("GIT_COMMITTER_NAME", "test")
	os.Setenv("GIT_COMMITTER_EMAIL", "test@example.com")
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// clock is a controllable time source shared by the clients of one test.
type clock struct {
	mu  sync.Mutex
	now time.Time
}

func newClock() *clock { return &clock{now: time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)} }

func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *clock) Advance(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	c.mu.Unlock()
}

// countingRunner wraps ExecRunner, counts remote invocations by subcommand,
// and runs an optional hook before each command.
type countingRunner struct {
	mu     sync.Mutex
	counts map[string]int
	calls  []Command
	before func(cmd Command)
}

func newCountingRunner() *countingRunner { return &countingRunner{counts: map[string]int{}} }

func remoteSubcommand(cmd Command) string {
	for i := 0; i < len(cmd.Args); i++ {
		if cmd.Args[i] == "-c" {
			i++
			continue
		}
		return cmd.Args[i]
	}
	return ""
}

func (r *countingRunner) Run(ctx context.Context, dir string, cmd Command) ([]byte, error) {
	r.mu.Lock()
	hook := r.before
	if cmd.Remote {
		r.counts[remoteSubcommand(cmd)]++
		r.calls = append(r.calls, cmd)
	}
	r.mu.Unlock()
	if hook != nil {
		hook(cmd)
	}
	return ExecRunner{}.Run(ctx, dir, cmd)
}

func (r *countingRunner) remoteCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, v := range r.counts {
		n += v
	}
	return n
}

func (r *countingRunner) count(sub string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.counts[sub]
}

func (r *countingRunner) setBefore(fn func(Command)) {
	r.mu.Lock()
	r.before = fn
	r.mu.Unlock()
}

func gitT(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// harness is a local bare origin plus clones of it.
type harness struct {
	t      *testing.T
	dir    string
	origin string
	clock  *clock
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	dir := t.TempDir()
	h := &harness{t: t, dir: dir, origin: filepath.Join(dir, "origin.git"), clock: newClock()}
	gitT(t, dir, "init", "--quiet", "--bare", "-b", "main", h.origin)
	seed := filepath.Join(dir, "seed")
	gitT(t, dir, "init", "--quiet", "-b", "main", seed)
	writeFile(t, filepath.Join(seed, "AGENTS.md"), "# seed\n")
	writeFile(t, filepath.Join(seed, "ai-docs", "tickets", "ready", "260101-feat-seed.md"), "# seed\n")
	gitT(t, seed, "add", "-A")
	gitT(t, seed, "commit", "--quiet", "-m", "seed")
	gitT(t, seed, "remote", "add", "origin", h.origin)
	gitT(t, seed, "push", "--quiet", "origin", "main")
	return h
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// testClone is one clone of the harness origin.
type testClone struct {
	h      *harness
	root   string
	runner *countingRunner
	opts   Options
}

func (h *harness) clone(name, email string) *testClone {
	h.t.Helper()
	root := filepath.Join(h.dir, name)
	gitT(h.t, h.dir, "clone", "--quiet", h.origin, root)
	gitT(h.t, root, "config", "user.email", email)
	gitT(h.t, root, "config", "user.name", name)
	return &testClone{h: h, root: root, runner: newCountingRunner()}
}

// worktree adds a worktree of this clone on a new branch.
func (c *testClone) worktree(name, branch string) *testClone {
	c.h.t.Helper()
	path := filepath.Join(c.h.dir, name)
	gitT(c.h.t, c.root, "worktree", "add", "--quiet", "-b", branch, path)
	return &testClone{h: c.h, root: path, runner: newCountingRunner(), opts: c.opts}
}

func (c *testClone) client() *Client {
	c.h.t.Helper()
	opts := c.opts
	opts.Runner = c.runner
	opts.Now = c.h.clock.Now
	opts.Getenv = func(string) string { return "" }
	cl, err := Open(context.Background(), c.root, opts)
	if err != nil {
		c.h.t.Fatalf("Open: %v", err)
	}
	return cl
}

func (c *testClone) setOriginURL(url string) {
	gitT(c.h.t, c.root, "remote", "set-url", "origin", url)
}

// remoteTip returns the origin index tip, or "".
func (h *harness) remoteTip() string {
	out, err := exec.Command("git", "-C", h.origin, "rev-parse", "--verify", "--quiet", RemoteRef).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// remoteIndex decodes the origin index tip.
func (h *harness) remoteIndex() *Index {
	h.t.Helper()
	tip := h.remoteTip()
	if tip == "" {
		h.t.Fatal("remote index ref is absent")
	}
	data := gitT(h.t, h.origin, "cat-file", "blob", tip+":"+indexFile)
	idx, err := DecodeIndex([]byte(data))
	if err != nil {
		h.t.Fatal(err)
	}
	return idx
}

// initIndex creates the remote index from clone c.
func (h *harness) initIndex(c *testClone) {
	h.t.Helper()
	res, err := c.client().Create(context.Background(), NewIndex(), "ticket-index: init")
	if err != nil || !res.Created {
		h.t.Fatalf("Create = %+v, %v", res, err)
	}
}

func registerEntry(stem string) PendingEntry {
	return PendingEntry{Op: OpRegister, Stem: stem, Owner: Owner{Email: "x@example.com", CloneID: "c", Track: "develop"}}
}

func (h *harness) submit(cl *Client, stem string) WriteResult {
	h.t.Helper()
	res, err := cl.Submit(context.Background(), registerEntry(stem), Apply(h.clock.Now()))
	if err != nil {
		h.t.Fatalf("Submit(%s): %v", stem, err)
	}
	return res
}

const unreachableURL = "/nonexistent/wsindex-unreachable.git"
