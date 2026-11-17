package wsagent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsdashboard"
)

// shortStartWait bounds a token's wait for its process's start for one
// test.
func shortStartWait(t *testing.T) {
	restore := mcpStartWait
	mcpStartWait = 100 * time.Millisecond
	t.Cleanup(func() { mcpStartWait = restore })
}

func isClosed(ch <-chan struct{}) bool {
	select {
	case <-ch:
		return true
	case <-time.After(2 * time.Second):
		return false
	}
}

// TestMCPTokenIssueReissueAndVoid: a start draws a token that resolves,
// once its process reports its start, to that process's hello; the next
// start, the process's end, or the conversation's close voids it; a token
// is one conversation's alone; a rejoin restores the recorded one.
func TestMCPTokenIssueReissueAndVoid(t *testing.T) {
	shortStartWait(t)
	h := newHarness(t, map[string]string{"chat": chatProfile})
	r := h.runner
	if s := r.attachMCP("c1", "", nil, "/w", 0); s != nil {
		t.Fatalf("with no endpoint there is no HTTP session to give: %+v", s)
	}
	r.SetEndpoint("http://127.0.0.1:7/")
	env := map[string]string{EnvAllowedTools: "tickets.query"}
	s1 := r.attachMCP("c1", "", env, "/w", 0)
	if s1 == nil || s1.URL != "http://127.0.0.1:7"+wsdashboard.ProxyHTTPPath || len(s1.Token) != 64 {
		t.Fatalf("a start gets the route and a fresh token: %+v", s1)
	}
	if _, _, ok := r.ResolveMCPToken(s1.Token); ok {
		t.Fatalf("a token whose process never reported its start does not resolve")
	}

	// A resolution waits for the start.
	resolved := make(chan wsdashboard.ProxyHello, 1)
	restore := mcpStartWait
	mcpStartWait = 5 * time.Second
	go func() {
		if hello, _, ok := r.ResolveMCPToken(s1.Token); ok {
			resolved <- hello
		}
		close(resolved)
	}()
	time.Sleep(20 * time.Millisecond)
	s1.Started([]string{"PATH=/bin", EnvMCPSessionURL + "=" + s1.URL, EnvMCPSessionToken + "=" + s1.Token}, 4242)
	var hello wsdashboard.ProxyHello
	select {
	case got, ok := <-resolved:
		if !ok {
			t.Fatal("the waiting resolution failed")
		}
		hello = got
	case <-time.After(5 * time.Second):
		t.Fatal("the waiting resolution never returned")
	}
	mcpStartWait = restore
	if hello.OwnerPID != 4242 || hello.Env["WS_MCP_HARNESS_PID"] != "4242" || hello.Cwd != "/w" || hello.Env[EnvAllowedTools] != "tickets.query" || hello.Env["PATH"] != "/bin" {
		t.Fatalf("the hello is the started process's: %+v", hello)
	}
	if _, has := hello.Env[EnvMCPSessionToken]; has {
		t.Fatalf("the session's credentials are not its server's: %v", hello.Env)
	}
	s1.Started([]string{"PATH=/other"}, 1)
	again, void1, ok := r.ResolveMCPToken(s1.Token)
	if !ok || again.OwnerPID != 4242 || again.Env["PATH"] != "/bin" {
		t.Fatalf("a second start report changes nothing: %+v %v", again, ok)
	}

	// The next start voids the last one's token.
	s2 := r.attachMCP("c1", "", env, "/w", 0)
	if s2 == nil || s2.Token == s1.Token {
		t.Fatalf("each start draws its own token: %+v", s2)
	}
	if !isClosed(void1) {
		t.Fatal("a re-issued conversation's old token is void")
	}
	if _, _, ok := r.ResolveMCPToken(s1.Token); ok {
		t.Fatal("a void token does not resolve")
	}
	s1.Started(nil, 9)
	if _, _, ok := r.ResolveMCPToken(s2.Token); ok {
		t.Fatal("the old session's start report is not the new one's")
	}
	s2.Started(nil, 0)
	_, void2, ok := r.ResolveMCPToken(s2.Token)
	if !ok {
		t.Fatal("the new token resolves")
	}

	// Another conversation's token is its own; the process's end voids only
	// its conversation's.
	s3 := r.attachMCP("c2", "", env, "/w2", 0)
	s3.Started(nil, 0)
	r.revokeToken("c1")
	if !isClosed(void2) {
		t.Fatal("the process's end voids its token")
	}
	if _, _, ok := r.ResolveMCPToken(s3.Token); !ok {
		t.Fatal("another conversation's token is untouched")
	}

	// A rejoin restores the recorded token, even before an endpoint is set;
	// the recorded PID stands in for one the backend cannot report.
	r.SetEndpoint("")
	s4 := r.attachMCP("c3", "recorded-token", env, "/w3", 99)
	if s4 == nil || s4.Token != "recorded-token" {
		t.Fatalf("a rejoin keeps its process's token: %+v", s4)
	}
	s4.Started([]string{"PATH=/bin"}, 0)
	if hello, _, ok := r.ResolveMCPToken("recorded-token"); !ok || hello.OwnerPID != 99 || hello.Env["WS_MCP_HARNESS_PID"] != "99" {
		t.Fatalf("the restored token resolves with the recorded PID: %+v %v", hello, ok)
	}
	for _, token := range []string{"", "unknown"} {
		if _, _, ok := r.ResolveMCPToken(token); ok {
			t.Fatalf("token %q resolves", token)
		}
	}
}

// TestMCPTokenVoidEndsAWaitingResolution: a token voided while a
// resolution waits for its start fails that resolution at once.
func TestMCPTokenVoidEndsAWaitingResolution(t *testing.T) {
	h := newHarness(t, map[string]string{"chat": chatProfile})
	h.runner.SetEndpoint("http://127.0.0.1:7")
	s := h.runner.attachMCP("c1", "", nil, "/w", 0)
	done := make(chan bool, 1)
	go func() {
		_, _, ok := h.runner.ResolveMCPToken(s.Token)
		done <- ok
	}()
	time.Sleep(20 * time.Millisecond)
	h.runner.revokeToken("c1")
	select {
	case ok := <-done:
		if ok {
			t.Fatal("a token voided before its start resolved")
		}
	case <-time.After(time.Second):
		t.Fatal("the void did not end the waiting resolution")
	}
}

// TestMCPHelloIsTheLaunchersHello: the hello a token resolves to is the
// one the stdio launcher would send from the same harness: the harness's
// environment under the entry's, the harness PID, the runtime binary, and
// the project root and harness the entry names.
func TestMCPHelloIsTheLaunchersHello(t *testing.T) {
	harness := []string{"PATH=/bin", "SHARED=harness", "WS_MCP_PROJECT_ROOT=/inherited", "NOEQUALS",
		EnvMCPSessionURL + "=http://x" + wsdashboard.ProxyHTTPPath, EnvMCPSessionToken + "=secret"}
	entry := map[string]string{"SHARED": "entry", "WS_MCP_PROJECT_ROOT": "/project", "WS_MCP_HARNESS": "claude", "BLANK": ""}
	hello := mcpHello(harness, entry, "/cwd", 321, "0.50.2")
	exe, _ := os.Executable()
	want := map[string]string{"PATH": "/bin", "SHARED": "entry", "WS_MCP_PROJECT_ROOT": "/project", "WS_MCP_HARNESS": "claude", "BLANK": "",
		"WS_MCP_HARNESS_PID": "321", "WS_MCP_RUNTIME_BINARY": exe}
	if len(hello.Env) != len(want) {
		t.Fatalf("env = %v, want %v", hello.Env, want)
	}
	for k, v := range want {
		if got, ok := hello.Env[k]; !ok || got != v {
			t.Fatalf("env[%s] = %q (%v), want %q", k, got, ok, v)
		}
	}
	if hello.Protocol != wsdashboard.ProxyProtocol || hello.Mode != wsdashboard.ProxyModeMCP || hello.Cwd != "/cwd" || hello.ProjectRoot != "/project" ||
		hello.Harness != "claude" || hello.OwnerPID != 321 || hello.Version != "0.50.2" || len(hello.Argv) != 0 {
		t.Fatalf("hello = %+v", hello)
	}
}

// TestMCPSessionEnv: a harness with a session gets its credentials; one
// without gets them blank, so none a parent left behind is inherited.
func TestMCPSessionEnv(t *testing.T) {
	parent := []string{"PATH=/bin", EnvMCPSessionURL + "=http://parent", EnvMCPSessionToken + "=parent-token"}
	got := mcpSessionEnv(parent, &MCPSession{URL: "http://x/mcp/session", Token: "mine"})
	slices.Sort(got)
	if want := []string{"PATH=/bin", EnvMCPSessionToken + "=mine", EnvMCPSessionURL + "=http://x/mcp/session"}; !slices.Equal(got, want) {
		t.Fatalf("with a session: %q, want %q", got, want)
	}
	got = mcpSessionEnv(parent, nil)
	slices.Sort(got)
	if want := []string{"PATH=/bin", EnvMCPSessionToken + "=", EnvMCPSessionURL + "="}; !slices.Equal(got, want) {
		t.Fatalf("without a session: %q, want %q", got, want)
	}
}

// TestClaudeWorkerMCPOverHTTP: a Claude worker with an HTTP session gets
// ws as an HTTP server whose bearer Claude Code expands from its
// environment; the token is in neither its arguments nor its config, and
// the backend reports the started process to the session.
func TestClaudeWorkerMCPOverHTTP(t *testing.T) {
	fake, out, outEnv := standin(t)
	spec := claudeSpec(t, `{"backend":"claude","tools":["Read"],"ws_tools":["tickets.query"],"mcp_servers":{"docs":{"command":"docs-mcp"}}}`)
	spec.Root, spec.Dir = t.TempDir(), t.TempDir()
	spec.WSMCP = &MCPCommand{Command: "/bin/ws-mcp", Env: map[string]string{EnvAllowedTools: "tickets.query"}}
	const token = "0123456789abcdef-session-token"
	type started struct {
		env []string
		pid int
	}
	reports := make(chan started, 2)
	spec.MCPSession = &MCPSession{URL: "http://127.0.0.1:7" + wsdashboard.ProxyHTTPPath, Token: token,
		Started: func(env []string, pid int) { reports <- started{env, pid} }}
	spec.Env = []string{"PATH=" + os.Getenv("PATH"), EnvMCPSessionToken + "=parent-token", outEnv}

	args, err := claudeArgs(spec)
	if err != nil {
		t.Fatal(err)
	}
	mcp, _ := flagValue(args, "--mcp-config")
	var cfg struct {
		MCPServers map[string]map[string]any `json:"mcpServers"`
	}
	if err := json.Unmarshal([]byte(mcp), &cfg); err != nil {
		t.Fatal(err)
	}
	ws := cfg.MCPServers[ReservedMCPServer]
	headers, _ := ws["headers"].(map[string]any)
	if ws["type"] != "http" || ws["url"] != spec.MCPSession.URL || headers["Authorization"] != "Bearer ${"+EnvMCPSessionToken+"}" || ws["command"] != nil {
		t.Fatalf("ws is the HTTP route with an expanded bearer: %v", ws)
	}
	if cfg.MCPServers["docs"]["command"] != "docs-mcp" || !slices.Contains(args, "--strict-mcp-config") {
		t.Fatalf("the profile's own servers stay, under the strict config: %v", args)
	}

	proc, err := ClaudeBackend{Command: fake}.Start(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	for deadline := time.Now().Add(30 * time.Second); ; time.Sleep(5 * time.Millisecond) {
		if _, err := os.Stat(filepath.Join(out, "ready")); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the stand-in claude never finished writing its records")
		}
	}
	proc.Close()
	read := func(name string) string {
		raw, err := os.ReadFile(filepath.Join(out, name))
		if err != nil {
			t.Fatal(err)
		}
		return string(raw)
	}
	if strings.Contains(read("args"), token) {
		t.Fatal("the token is not in the harness's arguments")
	}
	env := strings.Split(read("env"), "\n")
	if !slices.Contains(env, EnvMCPSessionToken+"="+token) || !slices.Contains(env, EnvMCPSessionURL+"="+spec.MCPSession.URL) || slices.Contains(env, EnvMCPSessionToken+"=parent-token") {
		t.Fatalf("the harness's environment carries its own session: %q", env)
	}
	select {
	case got := <-reports:
		if got.pid <= 0 || !slices.Contains(got.env, EnvMCPSessionToken+"="+token) {
			t.Fatalf("the start report carries the process's environment and PID: pid %d", got.pid)
		}
	default:
		t.Fatal("the backend never reported the start")
	}
	if len(reports) != 0 {
		t.Fatal("one start, one report")
	}
}

// TestClaudeWorkerRunsOverHTTP: a runner with an endpoint starts a Claude
// worker's ws-mcp over the HTTP route; its token resolves to the worker's
// allowlist and working directory, and its process's end voids it.
func TestClaudeWorkerRunsOverHTTP(t *testing.T) {
	if suiteBackend != BackendClaude {
		t.Skip("the HTTP session is Claude's")
	}
	h := newHarness(t, map[string]string{"chat": `{"backend":"claude","ws_tools":["tickets.query"]}`})
	h.endpoint = "http://127.0.0.1:1"
	h.runner.SetEndpoint(h.endpoint)
	c := h.create("chat")
	if _, err := h.runner.Send(c.ID, "hi"); err != nil {
		t.Fatal(err)
	}
	spec := h.backend.waitProcStart(t, 0)
	s := spec.MCPSession
	if s == nil || s.URL != h.endpoint+wsdashboard.ProxyHTTPPath || s.Token == "" || spec.WSMCP == nil {
		t.Fatalf("the worker's ws-mcp goes over HTTP: %+v", s)
	}
	s.Started([]string{"PATH=/bin"}, 777)
	hello, void, ok := h.runner.ResolveMCPToken(s.Token)
	if !ok || hello.Env[EnvAllowedTools] != "tickets.query" || hello.Env[EnvNoDashboard] != "1" || hello.OwnerPID != 777 || !sameDir(hello.Cwd, h.root) {
		t.Fatalf("the token resolves to the worker's ws-mcp context: %+v %v", hello, ok)
	}
	raw, err := os.ReadFile(recordPath(wsdashboard.SessionDir(h.dir, c.ID)))
	if err != nil || strings.Contains(string(raw), s.Token) {
		t.Fatalf("a worker's token is never persisted: %v", err)
	}
	h.backend.proc(0).exit()
	if !isClosed(void) {
		t.Fatal("the worker's end voids its token")
	}

	// Without an endpoint the worker keeps its stdio server.
	h.runner.SetEndpoint("")
	if _, err := h.runner.Send(c.ID, "again"); err != nil {
		t.Fatal(err)
	}
	if spec := h.backend.waitProcStart(t, 1); spec.MCPSession != nil || spec.WSMCP == nil || spec.WSMCP.Command != "/bin/ws-mcp" {
		t.Fatalf("no endpoint, the stdio server: %+v", spec)
	}
}

// startAt starts a runner whose daemon serves at endpoint, as a daemon
// start does (the endpoint is set before Start).
func (h *leadHarness) startAt(endpoint string) {
	h.runner = NewRunner(h.opts)
	h.runner.SetEndpoint(endpoint)
	h.runner.Start(func(string, string) { h.notes.Add(1) })
	h.t.Cleanup(h.runner.Close)
}

// TestClaudeLeadMCPOverHTTP: a hosted Claude lead loads the HTTP variant
// of its bundle, its token lives in its environment and its record, a
// later daemon that rejoins the process restores the token, and the
// process's end voids and forgets it.
func TestClaudeLeadMCPOverHTTP(t *testing.T) {
	shortStartWait(t)
	bundle := leadBundledFixture(t, "wsflow")
	h := newLeadHarness(t)
	h.opts.WSMCP = func(root string) (MCPCommand, error) {
		return MCPCommand{Command: "python3", Args: []string{"launcher.py", "serve", "--stdio"},
			Env: map[string]string{"WS_RSRC_ROOT": filepath.Join(bundle, "rsrc"), "WS_MCP_PROJECT_ROOT": root}}, nil
	}
	const endpoint = "http://127.0.0.1:1"
	h.startAt(endpoint)
	id, holder := h.runLead("one")
	spawn := h.holders.spawned(holder)
	dir, _ := flagValue(spawn.Argv, "--plugin-dir")
	if filepath.Dir(dir) != filepath.Join(h.dir, HostedVariantsDir) || !strings.HasPrefix(filepath.Base(dir), "wsflow-") {
		t.Fatalf("the lead loads its bundle's HTTP variant: --plugin-dir %q", dir)
	}
	if slices.Contains(spawn.Argv, "--mcp-config") || slices.Contains(spawn.Argv, "--strict-mcp-config") {
		t.Fatalf("a lead's own setup stays; the variant carries ws: %q", spawn.Argv)
	}
	token := spawn.Env[EnvMCPSessionToken]
	if len(token) != 64 || spawn.Env[EnvMCPSessionURL] != endpoint+wsdashboard.ProxyHTTPPath || strings.Contains(strings.Join(spawn.Argv, " "), token) {
		t.Fatalf("the token is in the lead's environment only: %v", spawn.Env)
	}
	rec := h.record(id)
	if rec.MCPToken != token {
		t.Fatalf("a held lead's token is recorded for its rejoin: %q", rec.MCPToken)
	}
	hello, _, ok := h.runner.ResolveMCPToken(token)
	if !ok || hello.OwnerPID != rec.HostPID || hello.Env["WS_MCP_HARNESS_PID"] != strconv.Itoa(rec.HostPID) ||
		hello.Env[EnvHostedSession] != "1" || hello.Env[EnvHostedSessionKey] != rec.WSKey || hello.Env[EnvAllowedTools] != "" ||
		hello.Env["WS_RSRC_ROOT"] != filepath.Join(bundle, "rsrc") || !sameDir(hello.ProjectRoot, h.root) {
		t.Fatalf("the token resolves to the hosted lead's ws-mcp context: %+v %v", hello, ok)
	}

	// The next daemon rejoins the process and restores its token.
	h.runner.Close()
	h.holders.restart()
	h.startAt(endpoint)
	eventually(t, "the token to be restored", func() bool {
		hello, _, ok := h.runner.ResolveMCPToken(token)
		return ok && hello.OwnerPID == rec.HostPID && hello.Env[EnvHostedSessionKey] == rec.WSKey
	})
	if h.holders.opCount("spawn") != 1 {
		t.Fatalf("a rejoin starts no process: %v", h.holders.opList())
	}
	_, void, _ := h.runner.ResolveMCPToken(token)

	// The process's end voids the token and forgets it.
	h.holders.exit(holder, 0)
	if !isClosed(void) {
		t.Fatal("the lead's end voids its token")
	}
	eventually(t, "the record to forget the token", func() bool { return h.record(id).MCPToken == "" })
}

// TestClaudeLeadMCPRouteMovedRestarts: a later daemon that serves another
// route (it listens on another port) does not restore the token the
// process sends to the old one; the process restarts at its first idle
// point with a new token on the new route.
func TestClaudeLeadMCPRouteMovedRestarts(t *testing.T) {
	shortStartWait(t)
	bundle := leadBundledFixture(t, "wsflow")
	h := newLeadHarness(t)
	h.opts.WSMCP = func(root string) (MCPCommand, error) {
		return MCPCommand{Command: "python3", Args: []string{"launcher.py", "serve", "--stdio"},
			Env: map[string]string{"WS_RSRC_ROOT": filepath.Join(bundle, "rsrc"), "WS_MCP_PROJECT_ROOT": root}}, nil
	}
	const before, after = "http://127.0.0.1:1", "http://127.0.0.1:2"
	h.startAt(before)
	id, holder := h.runLead("one")
	h.holders.out(holder, mcpTurn("u1", MCPConnected)...)
	h.idle(id)
	token := h.record(id).MCPToken
	if rec := h.record(id); token == "" || rec.MCPURL != before+wsdashboard.ProxyHTTPPath {
		t.Fatalf("the lead's route is recorded with its token: %+v", rec)
	}

	h.runner.Close()
	h.holders.restart()
	h.startAt(after)
	eventually(t, "the restart", func() bool { return h.holders.opCount("stop "+holder) == 1 && h.holders.opCount("launch") == 2 })
	eventually(t, "the new process", func() bool { st := h.state(id); return st.Process && h.holder(id) != holder })
	spawn := h.holders.spawned(h.holder(id))
	fresh := spawn.Env[EnvMCPSessionToken]
	if fresh == "" || fresh == token || spawn.Env[EnvMCPSessionURL] != after+wsdashboard.ProxyHTTPPath {
		t.Fatalf("the new process gets a new token on the new route: %v", spawn.Env)
	}
	if rec := h.record(id); rec.MCPToken != fresh || rec.MCPURL != after+wsdashboard.ProxyHTTPPath {
		t.Fatalf("the record holds the new process's: %+v", rec)
	}
	if _, _, ok := h.runner.ResolveMCPToken(token); ok {
		t.Fatal("the old route's token is not restored")
	}
	if _, _, ok := h.runner.ResolveMCPToken(fresh); !ok {
		t.Fatal("the new token resolves")
	}
}
