package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsindex"
)

// Tool-level scenarios for the origin ticket ownership index. Each test builds
// a local bare origin whose default branch (main) declares review-track
// develop, with develop carrying the open tickets alpha, beta, gamma and the
// closed ticket old.
//
// The scenario IDs in test names and comments (A*, B*, C*, D*, E*, I*) are the
// scenario contracts of ticket 260924-feat-origin-ticket-ownership-index.

const (
	stemAlpha = "260101-feat-alpha"
	stemBeta  = "260101-feat-beta"
	stemGamma = "260101-feat-gamma"
	stemOld   = "260101-feat-old"

	ixUnreachable = "/nonexistent/ticket-index-unreachable.git"
	ixCacheRef    = wsindex.CacheRef
)

var ixCallID atomic.Int64

type ixClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *ixClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *ixClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	c.mu.Unlock()
}

type ixEnv struct {
	t      *testing.T
	dir    string
	origin string
	clock  *ixClock
}

func ixTicket(title string) string { return "---\ntitle: " + title + "\n---\n# " + title + "\n" }

func newIxEnv(t *testing.T) *ixEnv {
	t.Helper()
	useLeadProfile(t)
	// Isolate from the developer's global and system git config (signing,
	// hooks, url rewrites); each repository sets its own identity.
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))
	dir := t.TempDir()
	e := &ixEnv{t: t, dir: dir, origin: filepath.Join(dir, "origin.git"), clock: &ixClock{now: time.Now().UTC().Truncate(time.Second)}}
	runGit(t, dir, "init", "--quiet", "--bare", "-b", "main", e.origin)
	seed := filepath.Join(dir, "seed")
	runGit(t, dir, "init", "--quiet", "-b", "main", seed)
	runGit(t, seed, "config", "user.email", "seed@example.com")
	runGit(t, seed, "config", "user.name", "seed")
	runGit(t, seed, "config", "commit.gpgsign", "false")
	mustWrite(t, seed, "AGENTS.md", "# Project\n\n### Review Policy\n\n```text\nreview-track: develop\n```\n")
	mustWrite(t, seed, "ai-docs/tickets/ready/"+stemAlpha+".md", ixTicket("Alpha"))
	mustWrite(t, seed, "ai-docs/tickets/ready/"+stemBeta+".md", ixTicket("Beta"))
	mustWrite(t, seed, "ai-docs/tickets/todo/"+stemGamma+".md", ixTicket("Gamma"))
	mustWrite(t, seed, "ai-docs/tickets/.done/"+stemOld+".md", ixTicket("Old"))
	runGit(t, seed, "add", "-A")
	runGit(t, seed, "commit", "--quiet", "-m", "seed")
	runGit(t, seed, "remote", "add", "origin", e.origin)
	runGit(t, seed, "push", "--quiet", "origin", "main", "main:develop")
	return e
}

// ixCheckout is one checkout (a clone or a worktree of one) with its own
// server and lead session.
type ixCheckout struct {
	env    *ixEnv
	root   string
	server *Server
	key    string
	runner *ixRunner
}

// ixRunner wraps wsindex.ExecRunner: it counts remote git invocations by
// subcommand and can fail index-ref fetches to simulate a transport failure
// after a successful discovery.
type ixRunner struct {
	mu        sync.Mutex
	counts    map[string]int
	failFetch atomic.Bool
}

func (r *ixRunner) Run(ctx context.Context, dir string, cmd wsindex.Command) ([]byte, error) {
	if cmd.Remote {
		sub := ""
		for i := 0; i < len(cmd.Args); i++ {
			if cmd.Args[i] == "-c" {
				i++
				continue
			}
			sub = cmd.Args[i]
			break
		}
		r.mu.Lock()
		r.counts[sub]++
		r.mu.Unlock()
		if sub == "fetch" && r.failFetch.Load() && strings.Contains(strings.Join(cmd.Args, " "), wsindex.RemoteRef) {
			return nil, errors.New("injected transport failure")
		}
	}
	return wsindex.ExecRunner{}.Run(ctx, dir, cmd)
}

func (r *ixRunner) count(sub string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.counts[sub]
}

func (r *ixRunner) total() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, v := range r.counts {
		n += v
	}
	return n
}

func (r *ixRunner) reset() {
	r.mu.Lock()
	r.counts = map[string]int{}
	r.mu.Unlock()
}

func (e *ixEnv) login(root string) *ixCheckout {
	e.t.Helper()
	s := NewServer(root, "test")
	runner := &ixRunner{counts: map[string]int{}}
	s.indexOpts = wsindex.Options{Runner: runner, Now: e.clock.Now, WriteTimeout: 10 * time.Second}
	key, _ := parseLoginResponse(e.t, callLogin(e.t, s, int(ixCallID.Add(1)), root, nil))
	return &ixCheckout{env: e, root: root, server: s, key: key, runner: runner}
}

// clone makes a clone on develop with its own identity.
func (e *ixEnv) clone(name, email string) *ixCheckout {
	e.t.Helper()
	root := filepath.Join(e.dir, name)
	runGit(e.t, e.dir, "clone", "--quiet", e.origin, root)
	runGit(e.t, root, "config", "user.email", email)
	runGit(e.t, root, "config", "user.name", name)
	runGit(e.t, root, "config", "commit.gpgsign", "false")
	runGit(e.t, root, "checkout", "--quiet", "develop")
	return e.login(root)
}

// worktree adds a worktree of c on a new branch started from start.
func (c *ixCheckout) worktree(name, branch, start string) *ixCheckout {
	c.env.t.Helper()
	path := filepath.Join(c.env.dir, name)
	runGit(c.env.t, c.root, "worktree", "add", "--quiet", "-b", branch, path, start)
	return c.env.login(path)
}

func (c *ixCheckout) git(args ...string) string {
	c.env.t.Helper()
	return strings.TrimSpace(string(runGitOutput(c.env.t, c.root, args...)))
}

func (c *ixCheckout) offline() { c.git("remote", "set-url", "origin", ixUnreachable) }
func (c *ixCheckout) online()  { c.git("remote", "set-url", "origin", c.env.origin) }

// call runs one tool and returns its text and whether it was an error.
func (c *ixCheckout) call(tool string, args map[string]any) (string, bool) {
	c.env.t.Helper()
	line := callToolLineWithKey(c.env.t, c.server, int(ixCallID.Add(1)), c.key, tool, args)
	var resp struct {
		Result struct {
			IsError bool `json:"isError"`
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"result"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(line), &resp); err != nil {
		c.env.t.Fatalf("%s: %v\n%s", tool, err, line)
	}
	if resp.Error != nil {
		return resp.Error.Message, true
	}
	if len(resp.Result.Content) != 1 {
		c.env.t.Fatalf("%s: unexpected response %s", tool, line)
	}
	return resp.Result.Content[0].Text, resp.Result.IsError
}

// mustCall fails the test on an error response.
func (c *ixCheckout) mustCall(tool string, args map[string]any) string {
	c.env.t.Helper()
	text, isErr := c.call(tool, args)
	if isErr {
		c.env.t.Fatalf("%s(%v) failed: %s", tool, args, text)
	}
	return text
}

// mustRefuse fails the test unless the call errors with every fragment.
func (c *ixCheckout) mustRefuse(tool string, args map[string]any, fragments ...string) string {
	c.env.t.Helper()
	text, isErr := c.call(tool, args)
	if !isErr {
		c.env.t.Fatalf("%s(%v) succeeded, want refusal: %s", tool, args, text)
	}
	for _, f := range fragments {
		if !strings.Contains(text, f) {
			c.env.t.Fatalf("%s refusal lacks %q: %s", tool, f, text)
		}
	}
	return text
}

func (c *ixCheckout) acquire(stem string, extra ...any) string {
	c.env.t.Helper()
	return c.mustCall("tickets.acquire", ixArgs(stem, extra...))
}

func ixArgs(stem string, extra ...any) map[string]any {
	args := map[string]any{"ticket_stem": stem}
	for i := 0; i+1 < len(extra); i += 2 {
		args[extra[i].(string)] = extra[i+1]
	}
	return args
}

func (c *ixCheckout) init() string {
	c.env.t.Helper()
	return c.mustCall("tickets.index_init", nil)
}

func (c *ixCheckout) pendingCount() int {
	c.env.t.Helper()
	cl, err := wsindex.Open(bgCtx(), c.root, wsindex.Options{})
	if err != nil {
		c.env.t.Fatal(err)
	}
	pending, err := cl.Pending(bgCtx())
	if err != nil {
		c.env.t.Fatal(err)
	}
	return len(pending)
}

func (c *ixCheckout) hasRef(ref string) bool {
	return exec.Command("git", "-C", c.root, "rev-parse", "--verify", "--quiet", ref).Run() == nil
}

func (e *ixEnv) remoteTip() string {
	out, err := exec.Command("git", "-C", e.origin, "rev-parse", "--verify", "--quiet", wsindex.RemoteRef).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func (e *ixEnv) index() *wsindex.Index {
	e.t.Helper()
	tip := e.remoteTip()
	if tip == "" {
		e.t.Fatal("the remote index ref is absent")
	}
	data := runGitOutput(e.t, e.origin, "cat-file", "blob", tip+":index.json")
	idx, err := wsindex.DecodeIndex(data)
	if err != nil {
		e.t.Fatal(err)
	}
	return idx
}

func (e *ixEnv) lease(stem string) *wsindex.Lease {
	e.t.Helper()
	reg := e.index().Registrations[stem]
	if reg == nil {
		e.t.Fatalf("%s is not registered", stem)
	}
	return reg.Lease
}

func (e *ixEnv) history() string {
	return string(runGitOutput(e.t, e.origin, "log", "--format=%B", wsindex.RemoteRef))
}

// landOnDevelop moves stem to dir on origin's develop through a scratch clone.
func (e *ixEnv) landOnDevelop(stem, from, to string) {
	e.t.Helper()
	scratch := filepath.Join(e.t.TempDir(), "land")
	runGit(e.t, e.dir, "clone", "--quiet", "--branch", "develop", e.origin, scratch)
	runGit(e.t, scratch, "config", "user.email", "lander@example.com")
	runGit(e.t, scratch, "config", "user.name", "lander")
	runGit(e.t, scratch, "config", "commit.gpgsign", "false")
	if err := os.MkdirAll(filepath.Join(scratch, "ai-docs/tickets", to), 0o755); err != nil {
		e.t.Fatal(err)
	}
	runGit(e.t, scratch, "mv", "ai-docs/tickets/"+from+"/"+stem+".md", "ai-docs/tickets/"+to+"/"+stem+".md")
	runGit(e.t, scratch, "commit", "--quiet", "-m", "land "+stem)
	runGit(e.t, scratch, "push", "--quiet", "origin", "develop")
}

// ---- A: index absence and init ------------------------------------------------

// A1, A3: with the index absent, acquire and release answer a plain "ok"
// with no validation; the mutating tools print exactly what they print
// without an origin.
func TestIndexAbsentVerbsAnswerPlainOK(t *testing.T) {
	e := newIxEnv(t)
	local := t.TempDir()
	initGit(t, local)
	mustWrite(t, local, "ai-docs/tickets/todo/"+stemGamma+".md", ixTicket("Gamma"))
	runGit(t, local, "add", "-A")
	runGit(t, local, "commit", "--quiet", "-m", "seed")
	noOrigin := e.login(local)
	withOrigin := e.clone("x", "x@example.com") // origin present, no index ref

	for _, c := range []*ixCheckout{noOrigin, withOrigin} {
		for _, tool := range []string{"tickets.acquire", "tickets.release"} {
			for _, args := range []map[string]any{
				{"ticket_stem": "260101-feat-no-such-ticket"},
				{"ticket_stem": "not a stem", "dangerously_override_lease_status": true},
				{"ticket_stem": stemGamma, "track": "x"},
			} {
				if text := c.mustCall(tool, args); text != "ok" {
					t.Fatalf("%s text = %q, want plain ok", tool, text)
				}
				args["format"] = "json"
				if text := c.mustCall(tool, args); text != "{\"status\":\"ok\"}\n" {
					t.Fatalf("%s json = %q", tool, text)
				}
			}
		}
	}
	// Mutating tools: identical output with and without an origin.
	for _, c := range []*ixCheckout{noOrigin, withOrigin} {
		c.git("checkout", "--quiet", "-b", "scratch")
	}
	want := noOrigin.mustCall("tickets.move", map[string]any{"stem": stemGamma, "to": "idea"})
	if got := withOrigin.mustCall("tickets.move", map[string]any{"stem": stemGamma, "to": "idea"}); got != want {
		t.Fatalf("move output differs with an index-absent origin:\n%s\nwant:\n%s", got, want)
	}
	want = noOrigin.mustCall("tickets.create_empty", map[string]any{"stem": "feat-absent-probe", "initial_state": "idea"})
	if got := withOrigin.mustCall("tickets.create_empty", map[string]any{"stem": "feat-absent-probe", "initial_state": "idea"}); got != want {
		t.Fatalf("create_empty output differs with an index-absent origin:\n%s\nwant:\n%s", got, want)
	}
	if strings.Contains(want, "ticket-index") {
		t.Fatalf("index text leaked into an index-absent project: %s", want)
	}
	// No index state was written: no clone id, no cache ref.
	if out, _ := exec.Command("git", "-C", withOrigin.root, "config", "--get", wsindex.CloneIDConfigKey).Output(); len(out) != 0 {
		t.Fatalf("an index-absent project got a clone id: %s", out)
	}
	if withOrigin.hasRef(ixCacheRef) || e.remoteTip() != "" {
		t.Fatal("an index-absent project got index refs")
	}
}

// A10: a clone that never saw an index, offline after its absence TTL,
// still gets the plain index-absent ok.
func TestA10NeverSeenOfflineAcquireIsIndexAbsent(t *testing.T) {
	e := newIxEnv(t)
	x := e.clone("x", "x@example.com")
	x.offline()
	e.clock.Advance(time.Hour)
	if text := x.acquire(stemAlpha); text != "ok" {
		t.Fatalf("acquire = %q, want ok", text)
	}
}

// A12: the check mode reports each state and leaves no trace outside
// initialized.
func TestA12InitCheckStates(t *testing.T) {
	e := newIxEnv(t)
	local := t.TempDir()
	initGit(t, local)
	noOrigin := e.login(local)
	if text := noOrigin.mustCall("tickets.index_init", map[string]any{"check": true}); text != "state: no-origin\n" {
		t.Fatalf("no-origin check = %q", text)
	}
	x := e.clone("x", "x@example.com")
	if text := x.mustCall("tickets.index_init", map[string]any{"check": true}); text != "state: uninitialized\n" {
		t.Fatalf("uninitialized check = %q", text)
	}
	if e.remoteTip() != "" || x.hasRef(ixCacheRef) || x.runner.count("push") != 0 {
		t.Fatal("an uninitialized check left a ref or pushed")
	}
	x.offline()
	if text := x.mustCall("tickets.index_init", map[string]any{"check": true, "format": "json"}); !strings.Contains(text, `"state":"unreachable"`) {
		t.Fatalf("unreachable check = %q", text)
	}
	if x.hasRef(ixCacheRef) {
		t.Fatal("an unreachable check left a cache ref")
	}
	x.online()
	x.init()
	x.runner.reset()
	if text := x.mustCall("tickets.index_init", map[string]any{"check": true}); text != "state: initialized\n" {
		t.Fatalf("initialized check = %q", text)
	}
	if n := x.runner.count("push"); n != 0 {
		t.Fatalf("the initialized check pushed %d times", n)
	}
}

// A6, A7, A8: concurrent init creates once and adopts once; registrations
// come from origin's declared review-track, not the local checkout.
func TestInitRegistersOriginReviewTrackOnce(t *testing.T) {
	e := newIxEnv(t)
	// develop on origin gains a ticket main does not have.
	scratch := filepath.Join(e.dir, "scratch")
	runGit(t, e.dir, "clone", "--quiet", "--branch", "develop", e.origin, scratch)
	runGit(t, scratch, "config", "user.email", "s@example.com")
	runGit(t, scratch, "config", "commit.gpgsign", "false")
	mustWrite(t, scratch, "ai-docs/tickets/idea/260102-feat-delta.md", ixTicket("Delta"))
	runGit(t, scratch, "add", "-A")
	runGit(t, scratch, "commit", "--quiet", "-m", "delta")
	runGit(t, scratch, "push", "--quiet", "origin", "develop")

	x := e.clone("x", "x@example.com")
	y := e.clone("y", "y@example.com")
	// x's local checkout declares another track and holds an unpushed ticket.
	mustWrite(t, x.root, "AGENTS.md", "# Project\n\n### Review Policy\n\n```text\nreview-track: main\n```\n")
	mustWrite(t, x.root, "ai-docs/tickets/ready/260103-feat-local.md", ixTicket("Local"))
	x.git("add", "-A")
	x.git("commit", "--quiet", "-m", "local only")

	var wg sync.WaitGroup
	outs := make([]string, 2)
	for i, c := range []*ixCheckout{x, y} {
		wg.Add(1)
		go func(i int, c *ixCheckout) {
			defer wg.Done()
			outs[i], _ = c.call("tickets.index_init", nil)
		}(i, c)
	}
	wg.Wait()
	joined := strings.Join(outs, "\n")
	if strings.Count(joined, "status: created") != 1 || strings.Count(joined, "status: adopted") != 1 {
		t.Fatalf("concurrent init outputs:\n%s", joined)
	}
	got := strings.Join(e.index().Stems(), ",")
	want := strings.Join([]string{stemAlpha, stemBeta, stemGamma, "260102-feat-delta"}, ",")
	if got != want {
		t.Fatalf("registrations = %s, want %s", got, want)
	}
	if !strings.Contains(joined, "review_track: develop") {
		t.Fatalf("init did not resolve origin's review-track:\n%s", joined)
	}
}

// ---- C, D: matrix and track resolution -----------------------------------------

// C1-C4, C8, B7 at tool level.
func TestAcquireMatrixToolLevel(t *testing.T) {
	e := newIxEnv(t)
	x := e.clone("x", "a@example.com")
	y := e.clone("y", "b@example.com")
	x2 := e.clone("x2", "a@example.com")
	x.init()

	out := x.acquire(stemAlpha)
	if !strings.Contains(out, "status: acquired") || !strings.Contains(out, "owner: a@example.com (track develop") {
		t.Fatalf("acquire = %s", out)
	}
	first := e.lease(stemAlpha).TouchedAt
	e.clock.Advance(time.Hour)
	if out := x.acquire(stemAlpha); !strings.Contains(out, "status: refreshed") {
		t.Fatalf("re-acquire = %s", out)
	}
	if !e.lease(stemAlpha).TouchedAt.After(first) {
		t.Fatal("C4: re-acquire did not refresh touched_at")
	}

	// C1.
	y.mustRefuse("tickets.acquire", ixArgs(stemAlpha), "a@example.com", "dangerously_override_lease_status")
	y.mustRefuse("tickets.acquire", ixArgs(stemAlpha, "dangerously_override_lease_status", true), "reason")
	if out := y.acquire(stemAlpha, "dangerously_override_lease_status", true, "reason", "user reassigned it"); !strings.Contains(out, "status: takeover") {
		t.Fatalf("override acquire = %s", out)
	}
	if got := e.lease(stemAlpha).Email; got != "b@example.com" {
		t.Fatalf("holder after override = %s", got)
	}

	// C2: same email, other clone.
	x.acquire(stemBeta)
	if out := x2.acquire(stemBeta); !strings.Contains(out, "status: takeover") || !strings.Contains(out, "warning:") {
		t.Fatalf("same-email acquire = %s", out)
	}

	// C3: same clone, other track.
	x.acquire(stemGamma)
	w := x.worktree("x-other", "track/other", "develop")
	w.mustRefuse("tickets.acquire", ixArgs(stemGamma), "a@example.com", "track develop")
	if out := w.acquire(stemGamma, "dangerously_override_lease_status", true, "reason", "moving work"); !strings.Contains(out, "status: takeover") {
		t.Fatalf("cross-track override = %s", out)
	}
	if got := e.lease(stemGamma).Track; got != "track/other" {
		t.Fatalf("track after cross-track override = %s", got)
	}

	// C8: a changed email is a different email.
	x2.git("config", "user.email", "renamed@example.com")
	x2.mustRefuse("tickets.acquire", ixArgs(stemBeta), "a@example.com")

	// B7: the takeover audit survives later writes in the commit chain.
	history := e.history()
	for _, want := range []string{"previous holder a@example.com", "reason: user reassigned it", "reason: moving work"} {
		if !strings.Contains(history, want) {
			t.Fatalf("index history lacks %q:\n%s", want, history)
		}
	}
}

// D1-D5 and the worker impl record.
func TestAcquireTrackResolution(t *testing.T) {
	e := newIxEnv(t)
	x := e.clone("x", "a@example.com")
	y := e.clone("y", "b@example.com")
	x.init()

	lead := x.worktree("x-view", "track/my-view", "develop")
	lead.acquire(stemAlpha)
	if got := e.lease(stemAlpha).Track; got != "track/my-view" {
		t.Fatalf("D1 track = %q", got)
	}
	worker := x.worktree("x-impl", "impl/track/my-view/k1", "develop")
	if out := worker.acquire(stemAlpha); !strings.Contains(out, "status: impl_recorded") {
		t.Fatalf("D2 impl record = %s", out)
	}
	lease := e.lease(stemAlpha)
	if lease.Track != "track/my-view" || lease.Impl == nil || lease.Impl.Branch != "impl/track/my-view/k1" {
		t.Fatalf("D2 lease = %+v", lease)
	}
	y.git("checkout", "--quiet", "-b", "impl/track/my-view/k1")
	y.mustRefuse("tickets.acquire", ixArgs(stemAlpha), "a@example.com")

	goal := x.worktree("x-goal", "goal/x", "develop")
	goal.acquire(stemBeta)
	if got := e.lease(stemBeta).Track; got != "goal/x" {
		t.Fatalf("D3 track = %q", got)
	}

	rootless := x.worktree("x-rootless", "impl/"+stemGamma, "develop")
	rootless.mustRefuse("tickets.acquire", ixArgs(stemGamma), "pass track explicitly")
	legacy := x.worktree("x-legacy", "implement/"+stemGamma, "develop")
	legacy.mustRefuse("tickets.acquire", ixArgs(stemGamma), "pass track explicitly")
	rootless.acquire(stemGamma, "track", "develop")
	if got := e.lease(stemGamma).Track; got != "develop" {
		t.Fatalf("D4 explicit track = %q", got)
	}

	detached := x.worktree("x-detached", "scratch", "develop")
	detached.git("checkout", "--quiet", "--detach")
	detached.mustRefuse("tickets.acquire", ixArgs(stemGamma), "detached")

	// Acquire validates the ticket file once the index is in use.
	x.mustRefuse("tickets.acquire", ixArgs("260101-feat-no-such"), "does not exist")
}

// ---- E: origin-closed refusal, close, landing -----------------------------------

// E4, E5 (acquire refusal) and I17 (offline hint).
func TestAcquireRefusesTicketClosedOnOrigin(t *testing.T) {
	e := newIxEnv(t)
	x := e.clone("x", "a@example.com")
	z := e.clone("z", "c@example.com")
	x.init()
	z.acquire(stemGamma) // before the landings: z's cache keeps alpha and beta registered
	e.landOnDevelop(stemAlpha, "ready", ".done")
	e.landOnDevelop(stemBeta, "ready", ".dropped")
	// x's checkout and remote-tracking ref are stale; acquire fetches.
	x.mustRefuse("tickets.acquire", ixArgs(stemAlpha), "already closed on origin", "pull")
	x.mustRefuse("tickets.acquire", ixArgs(stemBeta), "already closed on origin")

	// I17: z saw the index, then its tracking ref learned of the landing.
	z.git("fetch", "--quiet", "origin")
	cl, err := wsindex.Open(bgCtx(), z.root, wsindex.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if view, _ := cl.ReadCached(bgCtx()); view.Index == nil || view.Index.Registrations[stemAlpha] == nil {
		t.Fatal("I17 setup: z's cached index no longer registers alpha")
	}
	z.offline()
	z.mustRefuse("tickets.acquire", ixArgs(stemAlpha), "already closed on origin")
	z.mustRefuse("tickets.acquire", ixArgs(stemBeta), "already closed on origin")
	if n := z.pendingCount(); n != 0 {
		t.Fatalf("I17: refused offline acquire recorded %d pending entries", n)
	}
}

// E1 (through close and prune) and E2.
func TestCloseLeaseAndPruneOnLanding(t *testing.T) {
	e := newIxEnv(t)
	x := e.clone("x", "a@example.com")
	x.init()
	x.acquire(stemAlpha)
	worker := x.worktree("x-impl", "impl/develop/k1", "develop")
	worker.acquire(stemAlpha)
	worker.mustCall("tickets.close", map[string]any{"stem": stemAlpha, "status": "done"})
	lease := e.lease(stemAlpha)
	if lease.Phase != wsindex.PhaseClosed || lease.Email != "a@example.com" || lease.Impl == nil {
		t.Fatalf("E1 close lease = %+v", lease)
	}
	// E2: close on an unleased ticket creates a closed lease for the caller.
	x.mustCall("tickets.close", map[string]any{"stem": stemBeta, "status": "dropped"})
	if lease := e.lease(stemBeta); lease == nil || lease.Phase != wsindex.PhaseClosed || lease.Email != "a@example.com" {
		t.Fatalf("E2 lease = %+v", lease)
	}
	x.git("commit", "--quiet", "-m", "drop beta")
	worker.git("commit", "--quiet", "-m", "close alpha")
	x.git("merge", "--quiet", "--no-edit", "impl/develop/k1")
	x.git("push", "--quiet", "origin", "develop")
	// The next index write prunes both landed registrations.
	x.acquire(stemGamma)
	idx := e.index()
	if _, ok := idx.Registrations[stemAlpha]; ok {
		t.Fatal("E1: landed alpha was not pruned")
	}
	if _, ok := idx.Registrations[stemBeta]; ok {
		t.Fatal("landed beta was not pruned")
	}
}

// ---- I: offline acquire, pending log, flush --------------------------------------

// A4, A11, E10, I1, I5, I9, I11 at tool level.
func TestOfflineWritesPendAndFlush(t *testing.T) {
	e := newIxEnv(t)
	x := e.clone("x", "a@example.com")
	x.init()
	w := x.worktree("x-b", "track/b", "develop")
	worker := x.worktree("x-impl", "impl/develop/k1", "develop")
	x.offline() // the remote URL is clone-shared config

	start := time.Now()
	out := x.acquire(stemAlpha)
	if !strings.Contains(out, "status: pending") || !strings.Contains(out, "origin is unreachable") {
		t.Fatalf("offline acquire = %s", out)
	}
	w.mustRefuse("tickets.acquire", ixArgs(stemAlpha), "track develop")              // I11
	if out := worker.acquire(stemAlpha); !strings.Contains(out, "status: pending") { // I9 impl record
		t.Fatalf("offline impl record = %s", out)
	}
	created := w.mustCall("tickets.create_empty", map[string]any{"stem": "feat-offline-new", "initial_state": "idea"}) // E10, I5
	if !strings.Contains(created, "Created ") || !strings.Contains(created, "origin is unreachable") {
		t.Fatalf("offline create_empty = %s", created)
	}
	closed := x.mustCall("tickets.close", map[string]any{"stem": stemBeta, "status": "done"}) // I9 close
	if !strings.Contains(closed, "closed: "+stemBeta) || !strings.Contains(closed, "origin is unreachable") {
		t.Fatalf("offline close = %s", closed)
	}
	if elapsed := time.Since(start); elapsed > 30*time.Second {
		t.Fatalf("offline calls took %s", elapsed)
	}
	if n := x.pendingCount(); n != 4 {
		t.Fatalf("pending = %d, want 4", n)
	}

	x.online()
	x.acquire(stemGamma) // the next mutating tool flushes
	if n := x.pendingCount(); n != 0 {
		t.Fatalf("pending after flush = %d", n)
	}
	idx := e.index()
	alpha := idx.Registrations[stemAlpha].Lease
	if alpha == nil || alpha.Track != "develop" || alpha.Impl == nil || alpha.Impl.Branch != "impl/develop/k1" {
		t.Fatalf("alpha after flush = %+v", alpha)
	}
	if beta := idx.Registrations[stemBeta].Lease; beta == nil || beta.Phase != wsindex.PhaseClosed {
		t.Fatalf("beta after flush = %+v", beta)
	}
	registeredNew := false
	for _, s := range idx.Stems() {
		if strings.HasSuffix(s, "-feat-offline-new") {
			registeredNew = true
		}
	}
	if !registeredNew {
		t.Fatalf("the offline registration did not flush: %v", idx.Stems())
	}
}

// I3, I10: a conflicting pending acquire is dropped loudly; others apply.
func TestFlushDropsConflictsAndKeepsTheRest(t *testing.T) {
	e := newIxEnv(t)
	x := e.clone("x", "a@example.com")
	y := e.clone("y", "b@example.com")
	x.init()
	x.offline()
	x.acquire(stemAlpha)
	x.acquire(stemBeta)
	y.acquire(stemAlpha)
	x.online()
	out := x.acquire(stemGamma)
	for _, want := range []string{"report: ticket-index: dropped the offline acquire of " + stemAlpha, "track develop", "b@example.com"} {
		if !strings.Contains(out, want) {
			t.Fatalf("flush output lacks %q:\n%s", want, out)
		}
	}
	idx := e.index()
	if idx.Registrations[stemAlpha].Lease.Email != "b@example.com" {
		t.Fatal("the remote lost to a dropped pending entry")
	}
	if l := idx.Registrations[stemBeta].Lease; l == nil || l.Email != "a@example.com" {
		t.Fatalf("the non-conflicting entry did not apply: %+v", l)
	}
}

// I2, I12: an offline override records its holder and applies only against it.
func TestOfflineOverrideReplay(t *testing.T) {
	e := newIxEnv(t)
	x := e.clone("x", "a@example.com")
	y := e.clone("y", "b@example.com")
	z := e.clone("z", "c@example.com")
	x.init()
	y.acquire(stemAlpha)
	y.acquire(stemBeta)
	x.acquire(stemGamma) // refreshes x's cache: alpha and beta held by y
	x.offline()
	x.mustRefuse("tickets.acquire", ixArgs(stemAlpha), "b@example.com")
	x.acquire(stemAlpha, "dangerously_override_lease_status", true, "reason", "user said alpha")
	x.acquire(stemBeta, "dangerously_override_lease_status", true, "reason", "user said beta")
	// Meanwhile y releases beta and z takes it.
	y.mustCall("tickets.release", ixArgs(stemBeta))
	z.acquire(stemBeta)
	x.online()
	out := x.mustCall("tickets.release", ixArgs(stemGamma))
	if !strings.Contains(out, "dropped the offline acquire of "+stemBeta) || !strings.Contains(out, "c@example.com") {
		t.Fatalf("I12 flush output:\n%s", out)
	}
	idx := e.index()
	if idx.Registrations[stemAlpha].Lease.Email != "a@example.com" {
		t.Fatal("I2: the override against the recorded holder did not apply")
	}
	if idx.Registrations[stemBeta].Lease.Email != "c@example.com" {
		t.Fatal("I12: z lost the lease to a stale override")
	}
	if !strings.Contains(e.history(), "reason: user said alpha") {
		t.Fatal("I2: the replayed override lost its reason")
	}
}

// I4: a pending acquire of a ticket that landed meanwhile is dropped with
// the origin-closed report. I13: pending releases.
func TestFlushOriginClosedAndReleases(t *testing.T) {
	e := newIxEnv(t)
	x := e.clone("x", "a@example.com")
	y := e.clone("y", "b@example.com")
	x.init()
	x.acquire(stemBeta)
	x.acquire(stemGamma)
	x.offline()
	x.acquire(stemAlpha)
	x.mustCall("tickets.release", ixArgs(stemBeta))
	x.mustCall("tickets.release", ixArgs(stemGamma))
	y.acquire(stemGamma, "dangerously_override_lease_status", true, "reason", "handoff")
	e.landOnDevelop(stemAlpha, "ready", ".done")
	x.online()
	out := x.mustCall("tickets.create_empty", map[string]any{"stem": "feat-reconnect", "initial_state": "idea"})
	if !strings.Contains(out, "dropped the offline acquire of "+stemAlpha) || !strings.Contains(out, "closed on origin") {
		t.Fatalf("I4 flush output:\n%s", out)
	}
	if !strings.Contains(out, "dropped the offline release of "+stemGamma) {
		t.Fatalf("I13 flush output:\n%s", out)
	}
	idx := e.index()
	if idx.Registrations[stemBeta].Lease != nil {
		t.Fatal("I13: the pending release did not remove the lease")
	}
	if idx.Registrations[stemGamma].Lease.Email != "b@example.com" {
		t.Fatal("I13: a dropped release touched the new holder's lease")
	}
}

// ---- B: concurrency ----------------------------------------------------------------

// B1: simultaneous acquires of one unregistered stem: one winner.
func TestB1ConcurrentAcquireOneWinner(t *testing.T) {
	e := newIxEnv(t)
	clones := []*ixCheckout{e.clone("x", "a@example.com"), e.clone("y", "b@example.com"), e.clone("z", "c@example.com")}
	clones[0].init()
	const stem = "260924-feat-contended"
	for _, c := range clones {
		mustWrite(t, c.root, "ai-docs/tickets/ready/"+stem+".md", ixTicket("Contended"))
	}
	var wg sync.WaitGroup
	results := make([]struct {
		text  string
		isErr bool
	}, len(clones))
	for i, c := range clones {
		wg.Add(1)
		go func(i int, c *ixCheckout) {
			defer wg.Done()
			results[i].text, results[i].isErr = c.call("tickets.acquire", ixArgs(stem))
		}(i, c)
	}
	wg.Wait()
	winners := 0
	for _, r := range results {
		if !r.isErr {
			winners++
		}
	}
	if winners != 1 {
		t.Fatalf("winners = %d: %+v", winners, results)
	}
	holder := e.lease(stem).Email
	for _, r := range results {
		if r.isErr && !strings.Contains(r.text, holder) {
			t.Fatalf("loser refusal does not name the winner %s: %s", holder, r.text)
		}
	}
}

// B4, E8: piggyback registration racing acquire leaves one registration
// and the acquirer's lease; two clones registering one stem converge.
func TestB4RegistrationRacesAcquire(t *testing.T) {
	e := newIxEnv(t)
	x := e.clone("x", "a@example.com")
	y := e.clone("y", "b@example.com")
	x.init()
	const stem = "260924-feat-raced"
	for _, c := range []*ixCheckout{x, y} {
		mustWrite(t, c.root, "ai-docs/tickets/idea/"+stem+".md", ixTicket("Raced"))
		c.git("add", "-A")
		c.git("commit", "--quiet", "-m", "raced")
	}
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); x.mustCall("tickets.move", map[string]any{"stem": stem, "to": "todo"}) }()
	go func() { defer wg.Done(); y.acquire(stem) }()
	wg.Wait()
	y.mustCall("tickets.move", map[string]any{"stem": stem, "to": "todo"}) // E8
	idx := e.index()
	if reg := idx.Registrations[stem]; reg == nil || reg.Lease == nil || reg.Lease.Email != "b@example.com" {
		t.Fatalf("raced registration = %+v", reg)
	}
}

// B5, E3, E6, E7: GC runs once per period, never prunes a leased or
// closed-leased entry or one open on any origin branch, and a pruned
// local-only registration is re-registered by the next piggyback.
func TestB5GCRacesAcquire(t *testing.T) {
	e := newIxEnv(t)
	x := e.clone("x", "a@example.com")
	y := e.clone("y", "b@example.com")
	x.init()
	// A local-only ticket, and a ticket open only on an unmerged origin branch.
	const localOnly, branchOnly = "260924-feat-local-only", "260924-feat-branch-only"
	mustWrite(t, x.root, "ai-docs/tickets/idea/"+localOnly+".md", ixTicket("Local"))
	x.git("add", "-A")
	x.git("commit", "--quiet", "-m", "local")
	x.mustCall("tickets.move", map[string]any{"stem": localOnly, "to": "todo"})
	feature := x.worktree("x-feature", "feature-z", "origin/develop")
	mustWrite(t, feature.root, "ai-docs/tickets/idea/"+branchOnly+".md", ixTicket("Branch"))
	feature.git("add", "-A")
	feature.git("commit", "--quiet", "-m", "branch only")
	feature.git("push", "--quiet", "origin", "feature-z")
	feature.mustCall("tickets.move", map[string]any{"stem": branchOnly, "to": "todo"})
	x.mustCall("tickets.close", map[string]any{"stem": stemBeta, "status": "done"}) // closed lease, never merged

	e.clock.Advance(31 * 24 * time.Hour)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); x.mustCall("tickets.move", map[string]any{"stem": stemGamma, "to": "idea"}) }()
	go func() { defer wg.Done(); y.acquire(stemAlpha) }()
	wg.Wait()

	idx := e.index()
	if _, ok := idx.Registrations[localOnly]; ok {
		t.Fatal("E6: a local-only registration older than 30 days survived GC")
	}
	for _, s := range []string{stemAlpha, stemBeta, branchOnly} {
		if _, ok := idx.Registrations[s]; !ok {
			t.Fatalf("GC pruned %s", s)
		}
	}
	if l := idx.Registrations[stemAlpha].Lease; l == nil || l.Email != "b@example.com" {
		t.Fatalf("B5: the racing acquire was lost: %+v", l)
	}
	if runs := strings.Count(e.history(), "gc "+localOnly); runs != 1 {
		t.Fatalf("GC pruned %s in %d commits, want 1", localOnly, runs)
	}
	// E7.
	x.mustCall("tickets.move", map[string]any{"stem": localOnly, "to": "idea"})
	if _, ok := e.index().Registrations[localOnly]; !ok {
		t.Fatal("E7: the pruned registration was not re-registered")
	}
}

func bgCtx() context.Context { return context.Background() }

// The lease acquire bypasses a cached absence and discovers an index created
// meanwhile; the worker's impl-record acquire keeps the cache and makes no
// remote call.
func TestLeaseAcquireBypassesCachedAbsence(t *testing.T) {
	e := newIxEnv(t)
	x := e.clone("x", "a@example.com")
	worker := e.clone("w", "a@example.com")
	y := e.clone("y", "b@example.com")
	x.query() // no index yet: absence cached
	worker.query()
	y.init()

	worker.git("checkout", "--quiet", "-b", implTicketBranch("develop", stemBeta))
	worker.runner.reset()
	if out := worker.acquire(stemBeta); out != "ok" || worker.runner.total() != 0 {
		t.Fatalf("impl-record acquire within the absence = %q, remote calls %v", out, worker.runner.counts)
	}
	if out := x.acquire(stemAlpha); !strings.Contains(out, "status: acquired") {
		t.Fatalf("lease acquire within the absence = %s", out)
	}
	if l := e.lease(stemAlpha); l == nil || l.Email != "a@example.com" {
		t.Fatalf("lease = %+v", l)
	}
}

// C4: a takeover replayed from the pending log reports its warning in the
// flushing tool's output.
func TestReplayedTakeoverPrintsWarning(t *testing.T) {
	e := newIxEnv(t)
	x := e.clone("x", "a@example.com")
	x2 := e.clone("x2", "a@example.com")
	x.init()
	x.acquire(stemAlpha)
	x2.query() // x2 has seen the index
	x2.offline()
	x2.acquire(stemAlpha) // a pending takeover from the same email
	x2.online()
	out := x2.mustCall("tickets.move", map[string]any{"stem": stemGamma, "to": "idea"})
	if !strings.Contains(out, "ticket-index: replayed the offline acquire of "+stemAlpha) || !strings.Contains(out, "another clone") {
		t.Fatalf("flushing move = %s, want the replayed takeover warning", out)
	}
}

// C5: init and its check mode print the discard report of a pending log
// recorded against an index that was deleted meanwhile.
func TestIndexInitReportsDiscard(t *testing.T) {
	for _, check := range []bool{false, true} {
		e := newIxEnv(t)
		x := e.clone("x", "a@example.com")
		x.init()
		x.offline()
		x.acquire(stemBeta) // one pending entry
		x.online()
		runGit(t, e.origin, "update-ref", "-d", wsindex.RemoteRef)
		var out string
		if check {
			e.clone("y", "b@example.com").init() // a new index: x's pending log no longer continues it
			out = x.mustCall("tickets.index_init", map[string]any{"check": true})
		} else {
			out = x.init()
		}
		if !strings.Contains(out, "ticket-index: 1 offline entries were discarded") {
			t.Fatalf("check=%v: init output = %s", check, out)
		}
		if x.pendingCount() != 0 {
			t.Fatalf("check=%v: pending log not discarded", check)
		}
	}
}

// C6: a clone without a local refs/remotes/origin/HEAD resolves the
// review-track from origin's default branch on the online paths, so acquire
// refuses and pruning follows the same track init registered from.
func TestOnlineTrackWithoutLocalOriginHEAD(t *testing.T) {
	e := newIxEnv(t)
	x := e.clone("x", "a@example.com")
	x.git("remote", "set-head", "origin", "-d") // the local fallback would now say main
	if out := x.init(); !strings.Contains(out, "review_track: develop") {
		t.Fatalf("init = %s", out)
	}
	e.landOnDevelop(stemAlpha, "ready", ".done") // closed on develop only
	x.mustRefuse("tickets.acquire", ixArgs(stemAlpha), "already closed on origin")
	x.acquire(stemBeta)
	if _, ok := e.index().Registrations[stemAlpha]; ok {
		t.Fatal("pruning did not follow origin's review-track")
	}
}

// F1: the acquire lookup finds a ticket hidden by sparse checkout.
func TestAcquireFindsSparseHiddenTicket(t *testing.T) {
	e := newIxEnv(t)
	x := e.clone("x", "a@example.com")
	x.init()
	runGit(t, x.root, "sparse-checkout", "set", "--no-cone", "/*", "!/ai-docs/tickets/ready/"+stemAlpha+".md")
	if _, err := os.Stat(filepath.Join(x.root, "ai-docs", "tickets", "ready", stemAlpha+".md")); !os.IsNotExist(err) {
		t.Fatalf("the sparse scope did not hide the ticket: %v", err)
	}
	if out := x.acquire(stemAlpha); !strings.Contains(out, "status: acquired") {
		t.Fatalf("acquire of a sparse-hidden ticket = %s", out)
	}
}
