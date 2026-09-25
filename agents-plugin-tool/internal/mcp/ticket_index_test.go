package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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
// subcommand, records each remote fetch's arguments, and can fail index-ref
// fetches to simulate a transport failure after a successful discovery.
// beforeRemote, when set before the call under test, runs ahead of each
// remote command with its subcommand and arguments, so a test can change the
// origin between two remote steps of one call.
type ixRunner struct {
	mu           sync.Mutex
	counts       map[string]int
	fetches      []string
	failFetch    atomic.Bool
	beforeRemote func(sub string, args []string)
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
		if sub == "fetch" {
			r.fetches = append(r.fetches, strings.Join(cmd.Args, " "))
		}
		r.mu.Unlock()
		if r.beforeRemote != nil {
			r.beforeRemote(sub, cmd.Args)
		}
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

// fetchArgs returns the arguments of each remote fetch since the last reset.
func (r *ixRunner) fetchArgs() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.fetches...)
}

func (r *ixRunner) reset() {
	r.mu.Lock()
	r.counts = map[string]int{}
	r.fetches = nil
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

// hang points origin at an ssh remote whose transport never answers, so
// every remote command runs into its timeout. The script's path, like the
// live and stop paths inside it, is shell-quoted with forward slashes because
// git runs it through sh, which would eat a Windows path's backslashes and
// fail fast instead of hanging.
//
// The transport is a copy of internal/wsindex's hangTransport, whose comment
// explains its shape; keep the two in step. In short: it blocks on git's
// stdin, which ends it on POSIX when the timeout kills git's process group,
// and it also ends on a stop file the cleanup raises, because on Windows the
// timeout can leave the real git.exe (behind Git for Windows' redirector) and
// the transport alive inside the test tree, failing TempDir's cleanup. On
// POSIX the stop file is not waited on (see stopHangTransports).
func (c *ixCheckout) hang() {
	t := c.env.t
	t.Helper()
	dir := c.env.dir
	live := filepath.Join(dir, "hang-live")
	stop := filepath.Join(dir, "hang-stop")
	if err := os.MkdirAll(live, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { stopHangTransports(t, live, stop) })
	script := filepath.Join(dir, "hang-ssh.sh")
	mustWrite(t, dir, "hang-ssh.sh", fmt.Sprintf(`#!/bin/sh
live=%s
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
`, shellQuote(filepath.ToSlash(live)), shellQuote(filepath.ToSlash(stop))))
	if err := os.Chmod(script, 0o755); err != nil {
		t.Fatal(err)
	}
	c.git("config", "core.sshCommand", shellQuote(filepath.ToSlash(script)))
	c.git("remote", "set-url", "origin", "ssh://git@ticket-index.invalid/repo.git")
}

// stopHangTransports raises the stop file and, on Windows, waits for every
// hang transport to clear its live marker. POSIX has nothing to wait for: the
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

// shellQuote single-quotes s for sh. It is a copy of internal/wsindex's
// unexported shellQuote (git.go), kept local like the hang transport itself;
// keep the two in step.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// call runs one tool and returns its text and whether it was an error.
func (c *ixCheckout) call(tool string, args map[string]any) (string, bool) {
	c.env.t.Helper()
	text, isErr, err := c.callRaw(tool, args)
	if err != nil {
		c.env.t.Fatal(err)
	}
	return text, isErr
}

// callRaw is call without the testing.T, so a goroutine can run it and
// hand its error back to the test goroutine (FailNow must not run off it).
func (c *ixCheckout) callRaw(tool string, args map[string]any) (string, bool, error) {
	if args == nil {
		args = map[string]any{}
	}
	args["session_key"] = c.key
	id := ixCallID.Add(1)
	raw, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": id, "method": "tools/call",
		"params": map[string]any{"name": tool, "arguments": args},
	})
	if err != nil {
		return "", false, err
	}
	var out bytes.Buffer
	if err := c.server.ServeStdio(context.Background(), strings.NewReader(string(raw)+"\n"), &out); err != nil {
		return "", false, fmt.Errorf("%s: ServeStdio: %v", tool, err)
	}
	for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		var resp struct {
			ID     json.RawMessage `json:"id"`
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
			return "", false, fmt.Errorf("%s: %v\n%s", tool, err, line)
		}
		if string(resp.ID) != fmt.Sprint(id) {
			continue
		}
		if resp.Error != nil {
			return resp.Error.Message, true, nil
		}
		if len(resp.Result.Content) != 1 {
			return "", false, fmt.Errorf("%s: unexpected response %s", tool, line)
		}
		return resp.Result.Content[0].Text, resp.Result.IsError, nil
	}
	return "", false, fmt.Errorf("%s: no response with id %d in %s", tool, id, out.String())
}

// mustCallErr is mustCall for a goroutine: it returns the failure instead
// of failing the test.
func (c *ixCheckout) mustCallErr(tool string, args map[string]any) error {
	text, isErr, err := c.callRaw(tool, args)
	if err != nil {
		return err
	}
	if isErr {
		return fmt.Errorf("%s(%v) failed: %s", tool, args, text)
	}
	return nil
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

// gcCommits counts the index commits that carry a GC audit line.
func (e *ixEnv) gcCommits() int {
	n := 0
	for _, msg := range strings.Split(string(runGitOutput(e.t, e.origin, "log", "--format=%B%x00", wsindex.RemoteRef)), "\x00") {
		for _, line := range strings.Split(msg, "\n") {
			if strings.HasPrefix(line, "gc ") {
				n++
				break
			}
		}
	}
	return n
}

// remoteBlobsAndHistory is every index.json version on origin plus the
// index commit messages: everything the index ever published.
func (e *ixEnv) remoteBlobsAndHistory() string {
	var b strings.Builder
	for _, commit := range strings.Fields(string(runGitOutput(e.t, e.origin, "rev-list", wsindex.RemoteRef))) {
		b.Write(runGitOutput(e.t, e.origin, "cat-file", "blob", commit+":index.json"))
	}
	return b.String() + e.history()
}

// pathForms returns p and its symlink-resolved form (macOS temp dirs live
// under a /var -> /private/var link).
func pathForms(t *testing.T, p string) []string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(p)
	if err != nil {
		t.Fatal(err)
	}
	return []string{p, resolved}
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

// tickets.index_init is lead-only: a delegate or leaf session key is refused
// at the capability gate and creates no index ref.
func TestIndexInitRejectsNonLeadKeys(t *testing.T) {
	e := newIxEnv(t)
	x := e.clone("x", "x@example.com")
	for _, role := range []toolRole{roleDelegate, roleLeaf} {
		key, err := x.server.sessions.mint(x.root, role, "")
		if err != nil {
			t.Fatalf("mint %s key: %v", role, err)
		}
		for _, args := range []map[string]any{{"session_key": key}, {"session_key": key, "check": true}} {
			resp := callToolOnce(t, x.server, int(ixCallID.Add(1)), "tickets.index_init", args)
			if !strings.Contains(resp, "tool not available in current") || !strings.Contains(resp, "-32601") {
				t.Fatalf("%s key: want the lead-only rejection, got:\n%s", role, resp)
			}
		}
	}
	if e.remoteTip() != "" || x.hasRef(ixCacheRef) || x.runner.total() != 0 {
		t.Fatalf("a rejected init touched the index (remote calls %v)", x.runner.counts)
	}
}

// An origin with no branches (a clone whose work was never pushed) inits an
// index with zero registrations: the local tickets are not origin's.
func TestInitOnUnpushedOriginRegistersNothing(t *testing.T) {
	e := newIxEnv(t)
	empty := filepath.Join(e.dir, "empty.git")
	runGit(t, e.dir, "init", "--quiet", "--bare", "-b", "main", empty)
	local := filepath.Join(e.dir, "local")
	runGit(t, e.dir, "init", "--quiet", "-b", "main", local)
	runGit(t, local, "config", "user.email", "l@example.com")
	runGit(t, local, "config", "user.name", "l")
	runGit(t, local, "config", "commit.gpgsign", "false")
	mustWrite(t, local, "ai-docs/tickets/ready/"+stemAlpha+".md", ixTicket("Alpha"))
	runGit(t, local, "add", "-A")
	runGit(t, local, "commit", "--quiet", "-m", "unpushed")
	runGit(t, local, "remote", "add", "origin", empty)
	e.origin = empty // e.index and e.remoteTip read the empty origin
	x := e.login(local)
	if out := x.init(); !strings.Contains(out, "status: created") || !strings.Contains(out, "registered: 0 open tickets") {
		t.Fatalf("init on an unpushed origin = %s", out)
	}
	if idx := e.index(); len(idx.Registrations) != 0 {
		t.Fatalf("registrations = %v, want none", idx.Stems())
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
	runGit(t, scratch, "config", "user.name", "s")
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
	errs := make([]error, 2)
	for i, c := range []*ixCheckout{x, y} {
		wg.Add(1)
		go func(i int, c *ixCheckout) {
			defer wg.Done()
			outs[i], _, errs[i] = c.callRaw("tickets.index_init", nil)
		}(i, c)
	}
	wg.Wait()
	if err := errors.Join(errs...); err != nil {
		t.Fatal(err)
	}
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

// Landed-closure pruning on a non-acquire write reads the local tracking ref
// of the review-track and never fetches it: a landing the clone has not
// fetched is not pruned, and once a plain git fetch brings it in, the next
// write prunes it with no fetch of the review-track.
func TestNonAcquireWritePrunesFromLocalTrackingRef(t *testing.T) {
	e := newIxEnv(t)
	x := e.clone("x", "a@example.com")
	x.init()
	e.landOnDevelop(stemAlpha, "ready", ".done")
	fetchedTrack := func() bool {
		for _, args := range x.runner.fetchArgs() {
			if strings.Contains(args, "refs/heads/") {
				return true
			}
		}
		return false
	}

	x.runner.reset()
	x.mustCall("tickets.move", map[string]any{"stem": stemGamma, "to": "idea"})
	if _, ok := e.index().Registrations[stemAlpha]; !ok {
		t.Fatal("a landing the clone has not fetched was pruned")
	}
	if fetchedTrack() {
		t.Fatalf("a non-acquire write fetched a branch: %v", x.runner.fetchArgs())
	}

	x.git("fetch", "--quiet", "origin") // outside the tool: the runner sees nothing
	x.runner.reset()
	x.mustCall("tickets.move", map[string]any{"stem": stemGamma, "to": "todo"})
	if _, ok := e.index().Registrations[stemAlpha]; ok {
		t.Fatal("the landed closure in the local tracking ref was not pruned")
	}
	if fetchedTrack() {
		t.Fatalf("a non-acquire write fetched a branch: %v", x.runner.fetchArgs())
	}
}

// ---- I: offline acquire, pending log, flush --------------------------------------

// A4, A11, E10, I1, I5, I9, I11 at tool level. The worktrees record their
// entries one after another; the concurrent-append cases (I5-I7) are owned
// by the library-level wsindex tests. A flushed entry's worktree path stays
// local: it reaches neither index.json nor the index history.
func TestOfflineWritesRecordSequentiallyAndFlush(t *testing.T) {
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
	published := e.remoteBlobsAndHistory()
	for _, p := range pathForms(t, e.dir) {
		if strings.Contains(published, p) {
			t.Fatalf("a worktree path (%s) reached the remote index:\n%s", p, published)
		}
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
	y.git("checkout", "--quiet", "-b", "track/y")
	y.acquire(stemAlpha)
	acquiredAt := e.lease(stemAlpha).TouchedAt.UTC().Format(time.RFC3339)
	x.online()
	out := x.acquire(stemGamma)
	// The report names the dropped entry's own track and local worktree path,
	// the winner, and when the winner acquired.
	recordedOn := "recorded on track develop (" + x.root + ")"
	if resolved := pathForms(t, x.root)[1]; !strings.Contains(out, recordedOn) {
		recordedOn = "recorded on track develop (" + resolved + ")"
	}
	for _, want := range []string{"report: ticket-index: dropped the offline acquire of " + stemAlpha, recordedOn, "held by b@example.com (track track/y", "since " + acquiredAt} {
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
	errs := make([]error, len(clones))
	for i, c := range clones {
		wg.Add(1)
		go func(i int, c *ixCheckout) {
			defer wg.Done()
			results[i].text, results[i].isErr, errs[i] = c.callRaw("tickets.acquire", ixArgs(stem))
		}(i, c)
	}
	wg.Wait()
	if err := errors.Join(errs...); err != nil {
		t.Fatal(err)
	}
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
	errs := make([]error, 2)
	wg.Add(2)
	go func() {
		defer wg.Done()
		errs[0] = x.mustCallErr("tickets.move", map[string]any{"stem": stem, "to": "todo"})
	}()
	go func() { defer wg.Done(); errs[1] = y.mustCallErr("tickets.acquire", ixArgs(stem)) }()
	wg.Wait()
	if err := errors.Join(errs...); err != nil {
		t.Fatal(err)
	}
	y.mustCall("tickets.move", map[string]any{"stem": stem, "to": "todo"}) // E8
	idx := e.index()
	if reg := idx.Registrations[stem]; reg == nil || reg.Lease == nil || reg.Lease.Email != "b@example.com" {
		t.Fatalf("raced registration = %+v", reg)
	}
}

// B5, E3, E6, E7: GC runs once per period, never prunes a leased or
// closed-leased entry (even one open on no origin branch) or one open on any
// origin branch, and a pruned local-only registration is re-registered by the
// next piggyback.
func TestB5GCRacesAcquire(t *testing.T) {
	e := newIxEnv(t)
	x := e.clone("x", "a@example.com")
	y := e.clone("y", "b@example.com")
	x.init()
	// Local-only tickets (one unleased, one kept only by an active lease, one
	// only by a closed lease), and a ticket open only on an unmerged origin
	// branch.
	const localOnly, branchOnly = "260924-feat-local-only", "260924-feat-branch-only"
	const leasedOnly, closedOnly = "260924-feat-leased-only", "260924-feat-closed-only"
	for _, stem := range []string{localOnly, leasedOnly, closedOnly} {
		mustWrite(t, x.root, "ai-docs/tickets/idea/"+stem+".md", ixTicket(stem))
	}
	x.git("add", "-A")
	x.git("commit", "--quiet", "-m", "local")
	x.mustCall("tickets.move", map[string]any{"stem": localOnly, "to": "todo"})
	x.acquire(leasedOnly)
	x.mustCall("tickets.close", map[string]any{"stem": closedOnly, "status": "done"})
	feature := x.worktree("x-feature", "feature-z", "origin/develop")
	mustWrite(t, feature.root, "ai-docs/tickets/idea/"+branchOnly+".md", ixTicket("Branch"))
	feature.git("add", "-A")
	feature.git("commit", "--quiet", "-m", "branch only")
	feature.git("push", "--quiet", "origin", "feature-z")
	feature.mustCall("tickets.move", map[string]any{"stem": branchOnly, "to": "todo"})
	x.mustCall("tickets.close", map[string]any{"stem": stemBeta, "status": "done"}) // closed lease, never merged

	e.clock.Advance(31 * 24 * time.Hour)
	gcAt := e.clock.Now().UTC().Truncate(time.Second)
	var wg sync.WaitGroup
	errs := make([]error, 2)
	wg.Add(2)
	go func() {
		defer wg.Done()
		errs[0] = x.mustCallErr("tickets.move", map[string]any{"stem": stemGamma, "to": "idea"})
	}()
	go func() { defer wg.Done(); errs[1] = y.mustCallErr("tickets.acquire", ixArgs(stemAlpha)) }()
	wg.Wait()
	if err := errors.Join(errs...); err != nil {
		t.Fatal(err)
	}

	idx := e.index()
	if _, ok := idx.Registrations[localOnly]; ok {
		t.Fatal("E6: a local-only registration older than 30 days survived GC")
	}
	for _, s := range []string{stemAlpha, stemBeta, branchOnly, leasedOnly, closedOnly} {
		if _, ok := idx.Registrations[s]; !ok {
			t.Fatalf("GC pruned %s", s)
		}
	}
	if l := idx.Registrations[leasedOnly].Lease; l == nil || l.Phase != wsindex.PhaseActive {
		t.Fatalf("the lease-only entry lost its lease: %+v", l)
	}
	if l := idx.Registrations[closedOnly].Lease; l == nil || l.Phase != wsindex.PhaseClosed {
		t.Fatalf("the closed-lease-only entry lost its lease: %+v", l)
	}
	if l := idx.Registrations[stemAlpha].Lease; l == nil || l.Email != "b@example.com" {
		t.Fatalf("B5: the racing acquire was lost: %+v", l)
	}
	// The race leaves one GC commit and last_gc at the race's time. That
	// holds even if both racers ran GC, so it does not show GC ran once;
	// the within-period follow-up below carries the once-per-period rule.
	if idx.Meta.LastGC == nil || !idx.Meta.LastGC.Equal(gcAt) {
		t.Fatalf("last_gc = %v, want %v", idx.Meta.LastGC, gcAt)
	}
	if n := e.gcCommits(); n != 1 {
		t.Fatalf("GC commits = %d, want 1:\n%s", n, e.history())
	}
	// Within the period GC does not run again: the released lease-only
	// entry, now old, unleased, and open nowhere on origin, survives the next
	// write.
	x.mustCall("tickets.release", ixArgs(leasedOnly))
	e.clock.Advance(24 * time.Hour)
	x.mustCall("tickets.move", map[string]any{"stem": stemGamma, "to": "todo"})
	if _, ok := e.index().Registrations[leasedOnly]; !ok {
		t.Fatal("GC ran again within its period")
	}
	if n := e.gcCommits(); n != 1 {
		t.Fatalf("GC commits after a write within the period = %d, want 1", n)
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

	// With no index on origin the bypassing acquire asks once, stays the
	// plain ok, and re-caches the absence for the reads after it.
	x.runner.reset()
	if out := x.acquire(stemAlpha); out != "ok" || x.runner.count("ls-remote") != 1 || x.runner.total() != 1 {
		t.Fatalf("lease acquire with no index = %q, remote calls %v", out, x.runner.counts)
	}
	x.runner.reset()
	if x.query(); x.runner.total() != 0 {
		t.Fatalf("the acquire did not re-cache absence: query made %v", x.runner.counts)
	}
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

// A lead's lease acquire carries the relay direction on its own warning
// lines; the worker's impl record, release, JSON output, and replayed-entry
// reports do not.
func TestAcquireWarningRelaySuffix(t *testing.T) {
	e := newIxEnv(t)
	x := e.clone("x", "a@example.com")
	x2 := e.clone("x2", "a@example.com")
	x.init()
	x.acquire(stemAlpha)
	x.acquire(stemBeta)
	x.acquire(stemGamma)

	warnings := func(label, out string, relayed bool) {
		t.Helper()
		n := 0
		for _, line := range strings.Split(out, "\n") {
			if !strings.HasPrefix(line, "warning: ") {
				if strings.Contains(line, relayWarningSuffix) {
					t.Errorf("%s: non-warning line carries the relay suffix: %q", label, line)
				}
				continue
			}
			n++
			if got := strings.HasSuffix(line, relayWarningSuffix); got != relayed {
				t.Errorf("%s: relay suffix on %q = %v, want %v", label, line, got, relayed)
			}
		}
		if n == 0 {
			t.Fatalf("%s: no warning line in:\n%s", label, out)
		}
	}

	// Lease path, online: a same-email takeover from another clone.
	warnings("lease takeover", x2.acquire(stemAlpha), true)
	// Impl record, online: the same takeover warning, from a worker.
	worker := x2.worktree("x2-impl", "impl/develop/k1", "develop")
	out := worker.acquire(stemBeta)
	if !strings.Contains(out, "status: takeover") || !strings.Contains(out, "impl_branch: impl/develop/k1") {
		t.Fatalf("online impl record = %s", out)
	}
	warnings("impl takeover", out, false)

	x2.offline()
	// Lease path, offline: a pending takeover with both warnings relayed.
	out = x2.acquire(stemGamma)
	if !strings.Contains(out, "status: pending") || !strings.Contains(out, "another clone") {
		t.Fatalf("offline lease acquire = %s", out)
	}
	warnings("offline lease", out, true)
	warnings("offline impl record", worker.acquire(stemAlpha), false)
	warnings("offline release", x2.mustCall("tickets.release", ixArgs(stemBeta)), false)

	// The flush replays gamma's takeover as a report line, never relayed.
	x2.online()
	out = x2.acquire(stemAlpha)
	if !strings.Contains(out, "report: ticket-index: replayed the offline acquire of "+stemGamma) {
		t.Fatalf("flushing acquire = %s, want the replayed takeover report", out)
	}
	if strings.Contains(out, relayWarningSuffix) {
		t.Fatalf("flushing acquire relays a replayed entry: %s", out)
	}

	// JSON output is unchanged on the lease path.
	out = x.acquire(stemAlpha, "format", "json")
	if !strings.Contains(out, `"warnings"`) || strings.Contains(out, relayWarningSuffix) {
		t.Fatalf("JSON lease takeover = %s", out)
	}
}

// C5: init and its check mode print the discard report of a pending log
// recorded against an index that was deleted meanwhile.
func TestIndexInitReportsDiscard(t *testing.T) {
	const report = "ticket-index: 1 offline entries were discarded"
	for _, mode := range []string{"create", "adopt", "check", "create json", "check json"} {
		e := newIxEnv(t)
		x := e.clone("x", "a@example.com")
		x.init()
		x.offline()
		x.acquire(stemBeta) // one pending entry
		x.online()
		runGit(t, e.origin, "update-ref", "-d", wsindex.RemoteRef)
		if mode != "create" && mode != "create json" {
			e.clone("y", "b@example.com").init() // a new index: x's pending log no longer continues it
		}
		args := map[string]any{}
		if strings.HasPrefix(mode, "check") {
			args["check"] = true
		}
		if strings.HasSuffix(mode, "json") {
			args["format"] = "json"
		}
		out := x.mustCall("tickets.index_init", args)
		if mode == "adopt" && !strings.Contains(out, "status: adopted") {
			t.Fatalf("%s: init output = %s", mode, out)
		}
		if strings.HasSuffix(mode, "json") {
			var value struct {
				Reports []string `json:"reports"`
			}
			if err := json.Unmarshal([]byte(out), &value); err != nil || len(value.Reports) != 1 || !strings.HasPrefix(value.Reports[0], report) {
				t.Fatalf("%s: json = %s (%v)", mode, out, err)
			}
		} else if strings.Count(out, report) != 1 {
			t.Fatalf("%s: init output = %s", mode, out)
		}
		if x.pendingCount() != 0 {
			t.Fatalf("%s: pending log not discarded", mode)
		}
	}
}

// C5, adopt error: when the index disappears between Create's ls-remote and
// the adopt fetch, init fails and still prints the discard report of the
// pending log that failed adopt already dropped.
func TestIndexInitAdoptErrorPrintsDiscardReport(t *testing.T) {
	e := newIxEnv(t)
	x := e.clone("x", "a@example.com")
	x.init()
	x.offline()
	x.acquire(stemBeta) // one pending entry
	x.online()
	listed, deleted := false, false
	x.runner.beforeRemote = func(sub string, args []string) {
		switch {
		case sub == "ls-remote":
			listed = true
		case sub == "fetch" && listed && !deleted && strings.Contains(strings.Join(args, " "), wsindex.RemoteRef):
			deleted = true
			runGit(t, e.origin, "update-ref", "-d", wsindex.RemoteRef)
		}
	}
	out := x.mustRefuse("tickets.index_init", nil, "tickets.index_init: remote index is absent")
	x.runner.beforeRemote = nil
	if !deleted {
		t.Fatalf("the index ref was never deleted before an adopt fetch: %s", out)
	}
	const report = "\nreport: ticket-index: 1 offline entries were discarded"
	if strings.Count(out, report) != 1 {
		t.Fatalf("adopt error output lacks the discard report: %q", out)
	}
	if x.pendingCount() != 0 {
		t.Fatal("pending log not discarded")
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
	// The default branch's tracking ref is missing too: the online path
	// must fetch it to read the declaration.
	x.git("update-ref", "-d", "refs/remotes/origin/main")
	e.landOnDevelop(stemAlpha, "ready", ".done") // closed on develop only
	x.mustRefuse("tickets.acquire", ixArgs(stemAlpha), "already closed on origin")
	e.landOnDevelop(stemBeta, "ready", ".done")
	x.git("fetch", "--quiet", "origin")
	x.mustCall("tickets.move", map[string]any{"stem": stemGamma, "to": "idea"}) // a non-acquire write prunes
	idx := e.index()
	for _, stem := range []string{stemAlpha, stemBeta} {
		if _, ok := idx.Registrations[stem]; ok {
			t.Fatalf("pruning of %s did not follow origin's review-track", stem)
		}
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
	x.mustRefuse("tickets.acquire", ixArgs("260101-feat-no-such-ticket"), "does not exist in this checkout")
}
