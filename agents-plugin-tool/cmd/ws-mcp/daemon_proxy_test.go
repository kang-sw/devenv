package main

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wsmailbox"
	"github.com/kang-sw/devenv/internal/wsstate"
)

// Daemon-hosted sessions through the real binary, driven by a Go-side
// proxy client (the launcher's wire protocol without the launcher): what a
// session's end releases, how a CLI wait is cancelled, and how a LAN
// change replaces the daemon.

// writeProxyTestManifest is writeDashboardTestManifest with the LAN key.
func writeProxyTestManifest(t *testing.T, enabled bool) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "dashboard-config-manifest.json")
	body := `{"schema_version": 1, "namespace": "dashboard.", "keys": [
  {"key": "dashboard.enabled", "type": "boolean", "default": ` + strconv.FormatBool(enabled) + `, "description": "test"},
  {"key": "dashboard.port", "type": "integer", "minimum": 0, "maximum": 65535, "default": 0, "description": "test"},
  {"key": "dashboard.lan_enabled", "type": "boolean", "default": false, "description": "test"}]}`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

type hostedDaemon struct {
	bin, version, dir, cache, configHome, project string
	env                                           []string
	ep                                            wsdashboard.Endpoint
}

// startHostedDaemon builds the binary and runs its daemon-start on a scratch
// cache root, with a port-0 dashboard manifest declared in the environment.
func startHostedDaemon(t *testing.T) *hostedDaemon {
	t.Helper()
	bin := buildDashboardTestBin(t, "")
	versionOut, err := exec.Command(bin, "version").Output()
	if err != nil {
		t.Fatal(err)
	}
	env, dir := dashboardTestEnv(t)
	rsrc, err := filepath.Abs(filepath.Join("..", "..", "..", "agents-plugin", "rsrc"))
	if err != nil {
		t.Fatal(err)
	}
	project := t.TempDir()
	if err := os.MkdirAll(filepath.Join(project, "ai-docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "ai-docs", "_index.md"), []byte("# Index\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "init", "-q", project).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	env = append(env,
		"WS_MCP_CONFIG_MANIFESTS="+writeProxyTestManifest(t, true),
		"WS_RSRC_ROOT="+rsrc,
		"WS_SKILLS_ROOT="+filepath.Join(t.TempDir(), "missing-skills"),
		"WS_MCP_ALLOWED_TOOLS=",
	)
	h := &hostedDaemon{bin: bin, version: strings.TrimSpace(string(versionOut)), dir: dir, cache: cacheHomeFromEnv(t, env), configHome: envValue(t, env, "WS_CONFIG_HOME"), project: project, env: env}
	start := exec.Command(bin, "daemon-start")
	start.Env = env
	if out, err := start.CombinedOutput(); err != nil {
		t.Fatalf("daemon-start: %v\n%s", err, out)
	}
	h.ep = waitForEndpoint(t, dir, h.version, 10*time.Second)
	return h
}

// helloEnv is the daemon's test environment as a hello env map, with
// overrides on top.
func (h *hostedDaemon) helloEnv(overrides map[string]string) map[string]string {
	out := map[string]string{}
	for _, kv := range h.env {
		if k, v, ok := strings.Cut(kv, "="); ok {
			out[k] = v
		}
	}
	for k, v := range overrides {
		out[k] = v
	}
	return out
}

func (h *hostedDaemon) hello(mode string, argv []string, ownerPID int, overrides map[string]string) wsdashboard.ProxyHello {
	return wsdashboard.ProxyHello{Protocol: wsdashboard.ProxyProtocol, Mode: mode, Argv: argv, Cwd: h.project, ProjectRoot: h.project, Env: h.helloEnv(overrides), OwnerPID: ownerPID, Version: h.version}
}

type hostedConn struct {
	t      *testing.T
	conn   net.Conn
	r      *bufio.Reader
	nextID int
}

// connect upgrades to the daemon at ep and sends hello; the accept line
// must be ok.
func connectHosted(t *testing.T, ep wsdashboard.Endpoint, hello wsdashboard.ProxyHello) *hostedConn {
	t.Helper()
	conn, err := net.Dial("tcp", "127.0.0.1:"+strconv.Itoa(ep.Port))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	req := "GET " + wsdashboard.ProxyPath + " HTTP/1.1\r\nHost: 127.0.0.1:" + strconv.Itoa(ep.Port) + "\r\nUpgrade: " + wsdashboard.ProxyUpgrade + "\r\nConnection: Upgrade\r\nAuthorization: Bearer " + ep.Token + "\r\n\r\n"
	if _, err := conn.Write([]byte(req)); err != nil {
		t.Fatal(err)
	}
	r := bufio.NewReader(conn)
	status, err := r.ReadString('\n')
	if err != nil || !strings.Contains(status, " 101 ") {
		t.Fatalf("upgrade status %q: %v", status, err)
	}
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			t.Fatal(err)
		}
		if strings.TrimSpace(line) == "" {
			break
		}
	}
	raw, _ := json.Marshal(hello)
	if _, err := conn.Write(append(raw, '\n')); err != nil {
		t.Fatal(err)
	}
	line, err := r.ReadBytes('\n')
	if err != nil {
		t.Fatalf("accept line: %v", err)
	}
	var accept wsdashboard.ProxyAccept
	if err := json.Unmarshal(line, &accept); err != nil || !accept.OK {
		t.Fatalf("hello refused: %s (%v)", line, err)
	}
	return &hostedConn{t: t, conn: conn, r: r}
}

func (c *hostedConn) send(msg map[string]any) {
	c.t.Helper()
	raw, _ := json.Marshal(msg)
	if _, err := c.conn.Write(append(raw, '\n')); err != nil {
		c.t.Fatal(err)
	}
}

// readLine reads one line within the deadline; ok is false at EOF.
func (c *hostedConn) readLine(deadline time.Duration) (map[string]any, bool) {
	c.t.Helper()
	_ = c.conn.SetReadDeadline(time.Now().Add(deadline))
	defer c.conn.SetReadDeadline(time.Time{})
	line, err := c.r.ReadBytes('\n')
	if err != nil {
		if ne, ok := err.(net.Error); ok && ne.Timeout() {
			c.t.Fatalf("no line in %s", deadline)
		}
		return nil, false
	}
	var msg map[string]any
	if err := json.Unmarshal(line, &msg); err != nil {
		c.t.Fatalf("line %q: %v", line, err)
	}
	return msg, true
}

func (c *hostedConn) call(method string, params any) map[string]any {
	c.t.Helper()
	c.nextID++
	msg := map[string]any{"jsonrpc": "2.0", "id": c.nextID, "method": method}
	if params != nil {
		msg["params"] = params
	}
	c.send(msg)
	for {
		resp, ok := c.readLine(30 * time.Second)
		if !ok {
			c.t.Fatalf("%s: stream ended", method)
		}
		if id, _ := resp["id"].(float64); int(id) == c.nextID {
			return resp
		}
	}
}

// login runs initialize as a Claude client and ferrule at the project
// root; it returns the top-level lead session key.
func (c *hostedConn) login(root string) string {
	c.t.Helper()
	c.call("initialize", map[string]any{"protocolVersion": "2025-03-26", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "claude-code", "version": "0"}})
	c.send(map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"})
	resp := c.call("tools/call", map[string]any{"name": "ferrule", "arguments": map[string]any{"root": root}})
	for _, segment := range strings.Split(toolText(c.t, resp), "\n") {
		if key, ok := strings.CutPrefix(strings.TrimSpace(segment), "session_key:"); ok {
			return strings.TrimSpace(key)
		}
	}
	c.t.Fatalf("ferrule answered no session key: %v", resp)
	return ""
}

func (c *hostedConn) tool(name string, args map[string]any) map[string]any {
	c.t.Helper()
	resp := c.call("tools/call", map[string]any{"name": name, "arguments": args})
	if resp["error"] != nil {
		c.t.Fatalf("%s: %v", name, resp["error"])
	}
	return resp
}

// ownerProcess is a stand-in launcher: a process whose PID the hello
// names as owner, and which the test ends when that launcher would exit.
func ownerProcess(t *testing.T) *exec.Cmd {
	t.Helper()
	if goruntime.GOOS == "windows" {
		t.Skip("the stand-in owner process is sleep(1)")
	}
	owner := exec.Command("sleep", "120")
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Process.Kill(); _ = owner.Wait() })
	return owner
}

func waitUntil(t *testing.T, deadline time.Duration, what string, cond func() bool) {
	t.Helper()
	end := time.Now().Add(deadline)
	for time.Now().Before(end) {
		if cond() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("not within %s: %s", deadline, what)
}

func TestHostedSessionReleasesItsDashboardSessionAndPresenceWhenItCloses(t *testing.T) {
	h := startHostedDaemon(t)
	owner := ownerProcess(t)

	first := connectHosted(t, h.ep, h.hello(wsdashboard.ProxyModeMCP, []string{"serve", "--stdio"}, owner.Process.Pid, map[string]string{"WS_MAILBOX": "amber@machine"}))
	key := first.login(h.project)
	first.tool("todo.list", map[string]any{"session_key": key})
	reg, err := wsdashboard.ReadRegistration(h.dir, wsmailbox.ReplyID(key))
	if err != nil || !reg.Live(time.Now()) || reg.Harness != "claude" {
		t.Fatalf("registration after the keyed call = %+v, %v", reg, err)
	}

	// Presence names the owner (the launcher), not the daemon or this test.
	t.Setenv("WS_CONFIG_HOME", h.configHome)
	storePath, err := wsmailbox.MachinePath()
	if err != nil {
		t.Fatal(err)
	}
	presence := func() (wsmailbox.Presence, bool) {
		store, err := wsmailbox.Load(storePath)
		if err != nil {
			return wsmailbox.Presence{}, false
		}
		p, ok := store.Presence["amber"]
		return p, ok
	}
	p, ok := presence()
	if !ok || p.PID != owner.Process.Pid {
		t.Fatalf("presence = %+v (%v), want the owner pid %d (daemon %d, test %d)", p, ok, owner.Process.Pid, h.ep.PID, os.Getpid())
	}

	// A second connection claiming the name while the first is live is a
	// conflict, not a takeover.
	second := connectHosted(t, h.ep, h.hello(wsdashboard.ProxyModeMCP, []string{"serve", "--stdio"}, os.Getpid(), map[string]string{"WS_MAILBOX": "amber@machine"}))
	second.login(h.project)
	p, _ = presence()
	if p.PID != owner.Process.Pid || !p.Conflict {
		t.Fatalf("presence after the second claim = %+v, want the first owner flagged conflict", p)
	}

	// A session whose dashboard is off registers nothing.
	off := connectHosted(t, h.ep, h.hello(wsdashboard.ProxyModeMCP, []string{"serve", "--stdio"}, os.Getpid(), map[string]string{"WS_MCP_CONFIG_MANIFESTS": writeProxyTestManifest(t, false)}))
	offKey := off.login(h.project)
	off.tool("todo.list", map[string]any{"session_key": offKey})
	if _, err := wsdashboard.ReadRegistration(h.dir, wsmailbox.ReplyID(offKey)); !os.IsNotExist(err) {
		t.Fatalf("a dashboard-off session was registered: %v", err)
	}
	if info, err := wsdashboard.Health(context.Background(), dashboardTestClient, h.ep); err != nil || info.PID != h.ep.PID {
		t.Fatalf("the settings difference replaced the daemon: %+v, %v", info, err)
	}

	// The first launcher goes: its session ends and its presence is stale.
	first.conn.Close()
	_ = owner.Process.Kill()
	_ = owner.Wait()
	waitUntil(t, 5*time.Second, "registration marked ended", func() bool {
		reg, err := wsdashboard.ReadRegistration(h.dir, wsmailbox.ReplyID(key))
		return err == nil && reg.EndedAt != ""
	})
	if p, ok := presence(); !ok || wsstate.ProcessAlive(p.PID) {
		t.Fatalf("presence after the owner went = %+v (%v), want a stale record", p, ok)
	}
	// The daemon is untouched by the session's end.
	second.tool("todo.list", map[string]any{"session_key": key})
}

func TestHostedMailboxWaitEndsWithItsConnection(t *testing.T) {
	h := startHostedDaemon(t)
	root, err := wsstate.CacheRoot(wsstate.Options{CacheHome: h.cache})
	if err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(root, "mailbox-listening", "amber-tide-fox.json")
	exists := func() bool { _, err := os.Stat(marker); return err == nil }
	argv := []string{"mailbox", "wait", "--session-key", "amber-tide-fox", "--timeout", "60s", "--format", "json"}

	first := connectHosted(t, h.ep, h.hello(wsdashboard.ProxyModeCLI, argv, os.Getpid(), nil))
	waitUntil(t, 5*time.Second, "listening marker written", exists)

	// A replacement wait on the same key (another launcher, so another
	// owner pid: the daemon's stands in) takes the marker over; the old
	// wait's end must not remove it.
	second := connectHosted(t, h.ep, h.hello(wsdashboard.ProxyModeCLI, argv, h.ep.PID, nil))
	markerPID := func() int {
		raw, err := os.ReadFile(marker)
		if err != nil {
			return 0
		}
		var m wsmailbox.ListeningMarker
		if json.Unmarshal(raw, &m) != nil {
			return 0
		}
		return m.PID
	}
	waitUntil(t, 5*time.Second, "the replacement wait owns the marker", func() bool { return markerPID() == h.ep.PID })
	first.conn.Close()
	// The first wait's end (its connection is gone) must leave the
	// replacement's marker alone, for as long as that end can take.
	for end := time.Now().Add(time.Second); time.Now().Before(end); time.Sleep(50 * time.Millisecond) {
		if markerPID() != h.ep.PID {
			t.Fatal("the old wait's cleanup removed the replacement wait's marker")
		}
	}

	// The remaining wait ends with its connection, clearing its marker; no
	// final frame reaches a launcher that is gone, so the stream just ends.
	second.conn.Close()
	waitUntil(t, 3*time.Second, "listening marker cleared", func() bool { return !exists() })

	// A wait that times out answers through frames with the CLI's code.
	timed := connectHosted(t, h.ep, h.hello(wsdashboard.ProxyModeCLI, []string{"mailbox", "wait", "--session-key", "amber-tide-fox", "--timeout", "1s", "--format", "json"}, os.Getpid(), nil))
	var final map[string]any
	for {
		frame, ok := timed.readLine(10 * time.Second)
		if !ok {
			t.Fatal("stream ended before the final frame")
		}
		if frame["final"] == true {
			final = frame
			break
		}
	}
	if code, _ := final["exit_code"].(float64); int(code) != mailboxWaitExitTimeout || !strings.Contains(final["data"].(string), `"timed_out":true`) {
		t.Fatalf("final frame = %v", final)
	}
	if _, ok := timed.readLine(5 * time.Second); ok {
		t.Fatal("a line after the final frame")
	}
}

func TestHostedConnectionSeeingALANChangeReplacesTheDaemon(t *testing.T) {
	h := startHostedDaemon(t)
	if info, err := wsdashboard.Health(context.Background(), dashboardTestClient, h.ep); err != nil || info.LAN {
		t.Fatalf("health before the change = %+v, %v", info, err)
	}
	before := connectHosted(t, h.ep, h.hello(wsdashboard.ProxyModeMCP, []string{"serve", "--stdio"}, os.Getpid(), nil))
	before.call("initialize", map[string]any{"protocolVersion": "2025-03-26", "capabilities": map[string]any{}})

	// A CLI wait blocked on mail, and a tool call that outlives the drain
	// budget: the fake git holds `status` for longer than the budget, then
	// until the test releases it. On Windows the fake git is unavailable
	// (a POSIX script) and only the slow-call assertions are skipped.
	waiting := connectHosted(t, h.ep, h.hello(wsdashboard.ProxyModeCLI, []string{"mailbox", "wait", "--session-key", "drain-wait-key", "--timeout", "60s", "--format", "json"}, os.Getpid(), nil))
	slowGit := slowGitDir(t, "status", 3*wsdashboard.DefaultLockWait/2)
	var slow *hostedConn
	var slowKey string
	if slowGit != "" {
		slow = connectHosted(t, h.ep, h.hello(wsdashboard.ProxyModeMCP, []string{"serve", "--stdio"}, os.Getpid(), map[string]string{"PATH": slowGit + string(os.PathListSeparator) + os.Getenv("PATH")}))
		slowKey = slow.login(h.project)
	}

	config := `{"schema_version": 1, "overrides": {"dashboard.lan_enabled": "true"}}`
	if err := os.WriteFile(filepath.Join(h.configHome, "config.json"), []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}

	// The slow call starts before the trigger; its answer must still come.
	var slowCallID int
	var slowSentAt time.Time
	if slow != nil {
		slow.nextID++
		slowCallID = slow.nextID
		slow.send(map[string]any{"jsonrpc": "2.0", "id": slowCallID, "method": "tools/call", "params": map[string]any{"name": "git.status", "arguments": map[string]any{"session_key": slowKey}}})
		slowSentAt = time.Now()
		waitUntil(t, 10*time.Second, "the slow git to start", func() bool {
			_, err := os.Stat(filepath.Join(slowGit, "started"))
			return err == nil
		})
	}

	// The next session sees the new global value; the daemon replaces
	// itself and drains the sessions it hosts.
	trigger := connectHosted(t, h.ep, h.hello(wsdashboard.ProxyModeMCP, []string{"serve", "--stdio"}, os.Getpid(), nil))
	trigger.call("initialize", map[string]any{"protocolVersion": "2025-03-26", "capabilities": map[string]any{}})
	triggeredAt := time.Now()

	// The blocked CLI wait is cancelled at once: its final frame carries
	// the interrupted code well inside the drain budget.
	var final map[string]any
	for final == nil {
		frame, ok := waiting.readLine(wsdashboard.DefaultLockWait)
		if !ok {
			t.Fatal("the waiting stream ended without a final frame")
		}
		if frame["final"] == true {
			final = frame
		}
	}
	if code, _ := final["exit_code"].(float64); int(code) != mailboxWaitExitInterrupted {
		t.Fatalf("drained wait's final frame = %v", final)
	}
	if time.Since(triggeredAt) > wsdashboard.DefaultLockWait {
		t.Fatalf("the drained wait ended %v after the trigger", time.Since(triggeredAt))
	}

	var successor wsdashboard.Endpoint
	waitUntil(t, 30*time.Second, "a LAN daemon with a new pid", func() bool {
		ep, err := wsdashboard.ReadEndpoint(h.dir)
		if err != nil || ep.PID == h.ep.PID {
			return false
		}
		info, err := wsdashboard.Health(context.Background(), dashboardTestClient, ep)
		if err != nil || !info.LAN || info.PID != ep.PID {
			return false
		}
		successor = ep
		return true
	})
	for name, c := range map[string]*hostedConn{"before": before, "trigger": trigger} {
		if _, ok := c.readLine(wsdashboard.DefaultLockWait); ok {
			t.Fatalf("%s: a line instead of the drained stream's end", name)
		}
	}
	// The successor serves while the predecessor still runs the slow call:
	// the lock and endpoint were released before that call ended. The fake
	// git holds the call until the release file exists, so a slow runner
	// cannot end the call before this check.
	if slow != nil {
		if !wsstate.ProcessAlive(h.ep.PID) {
			t.Fatal("the predecessor exited before its slow call ended")
		}
		if err := os.WriteFile(filepath.Join(slowGit, "release"), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	after := connectHosted(t, successor, h.hello(wsdashboard.ProxyModeMCP, []string{"serve", "--stdio"}, os.Getpid(), nil))
	init := after.call("initialize", map[string]any{"protocolVersion": "2025-03-26", "capabilities": map[string]any{}})
	if init["result"] == nil {
		t.Fatalf("initialize on the successor = %v", init)
	}
	// ... and the slow call's answer is delivered before its stream ends.
	if slow != nil {
		for {
			resp, ok := slow.readLine(3 * wsdashboard.DefaultLockWait)
			if !ok {
				t.Fatal("the slow session's stream ended before its call was answered")
			}
			if id, _ := resp["id"].(float64); int(id) == slowCallID {
				if resp["error"] != nil || !strings.Contains(toolText(t, resp), "master") {
					t.Fatalf("slow git.status answer = %v", resp)
				}
				if held := time.Since(slowSentAt); held < 3*wsdashboard.DefaultLockWait/2-500*time.Millisecond {
					t.Fatalf("the slow call answered after %v; the fake git did not hold it", held)
				}
				break
			}
		}
		if _, ok := slow.readLine(wsdashboard.DefaultLockWait); ok {
			t.Fatal("a line after the slow session's answer")
		}
	}
	waitUntil(t, 10*time.Second, "the predecessor to exit", func() bool { return !wsstate.ProcessAlive(h.ep.PID) })

	// A hello whose session the dashboard does not apply to, or whose
	// global config is another one, never triggers a replacement: it
	// declares no dashboard manifest, sets WS_MCP_NO_DASHBOARD=1, or
	// carries a WS_CONFIG_HOME other than the daemon's. Each resolves
	// lan_enabled=false against the LAN successor (the global file now says
	// false too), so only the guards keep the successor in place.
	config = `{"schema_version": 1, "overrides": {"dashboard.lan_enabled": "false"}}`
	if err := os.WriteFile(filepath.Join(h.configHome, "config.json"), []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}
	for name, overrides := range map[string]map[string]string{
		"no manifest":       {"WS_MCP_CONFIG_MANIFESTS": ""},
		"dashboard off":     {"WS_MCP_NO_DASHBOARD": "1"},
		"other config home": {"WS_CONFIG_HOME": t.TempDir()},
	} {
		c := connectHosted(t, successor, h.hello(wsdashboard.ProxyModeMCP, []string{"serve", "--stdio"}, os.Getpid(), overrides))
		if init := c.call("initialize", map[string]any{"protocolVersion": "2025-03-26", "capabilities": map[string]any{}}); init["result"] == nil {
			t.Fatalf("%s: initialize = %v", name, init)
		}
		c.conn.Close()
	}
	for end := time.Now().Add(2 * time.Second); time.Now().Before(end); time.Sleep(100 * time.Millisecond) {
		ep, err := wsdashboard.ReadEndpoint(h.dir)
		if err != nil || ep.PID != successor.PID {
			t.Fatalf("a session the LAN change does not apply to replaced the daemon: %+v, %v", ep, err)
		}
	}
	if info, err := wsdashboard.Health(context.Background(), dashboardTestClient, successor); err != nil || !info.LAN {
		t.Fatalf("health after the ignored hellos = %+v, %v", info, err)
	}
}

// slowGitDir is a directory with a git that, for the given subcommand,
// creates the directory's "started" file, sleeps delay, then waits (up to
// 30 s) for its "release" file before running the real git with the same
// arguments, marking its stderr so a caller can tell it ran. On Windows the
// script cannot run and the result is "", which a caller treats as no slow
// git (the POSIX script is the only shim; a .cmd shim has no portable
// sleep or file wait).
func slowGitDir(t *testing.T, subcommand string, delay time.Duration) string {
	t.Helper()
	if goruntime.GOOS == "windows" {
		return ""
	}
	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git not on PATH")
	}
	dir := t.TempDir()
	script := "#!/bin/sh\n" +
		"for a in \"$@\"; do if [ \"$a\" = " + subcommand + " ]; then\n" +
		"  : > \"" + filepath.Join(dir, "started") + "\"\n" +
		"  sleep " + strconv.FormatFloat(delay.Seconds(), 'f', 1, 64) + "\n" +
		"  i=0; while [ ! -e \"" + filepath.Join(dir, "release") + "\" ] && [ $i -lt 300 ]; do sleep 0.1; i=$((i+1)); done\n" +
		"  echo SLOW-GIT-DONE >&2; break\n" +
		"fi; done\n" +
		"exec " + realGit + " \"$@\"\n"
	if err := os.WriteFile(filepath.Join(dir, "git"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

// TestHostedMailboxWaitResolvesARelativeRootAgainstTheHelloCwd: a relative
// --root names a directory under the launcher's cwd, not the daemon's. The
// worktree-scope slug makes the root matter: the wait resolves that store
// under the root, which exists only under the project.
func TestHostedMailboxWaitResolvesARelativeRootAgainstTheHelloCwd(t *testing.T) {
	h := startHostedDaemon(t)
	if err := os.MkdirAll(filepath.Join(h.project, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat("sub"); err == nil {
		t.Fatal("the daemon's cwd has a sub directory too; the test cannot tell the roots apart")
	}
	argv := []string{"mailbox", "wait", "--session-key", "relative-root-key", "--root", "sub", "--slug", "amber@worktree", "--timeout", "1s", "--format", "json"}
	c := connectHosted(t, h.ep, h.hello(wsdashboard.ProxyModeCLI, argv, os.Getpid(), nil))
	var final map[string]any
	var stderr strings.Builder
	for final == nil {
		frame, ok := c.readLine(10 * time.Second)
		if !ok {
			t.Fatalf("stream ended before the final frame; stderr: %s", stderr.String())
		}
		if frame["final"] == true {
			final = frame
		} else if data, _ := frame["data"].(string); frame["stream"] == "stderr" {
			stderr.WriteString(data)
		}
	}
	if code, _ := final["exit_code"].(float64); int(code) != mailboxWaitExitTimeout {
		t.Fatalf("final frame = %v; stderr: %s", final, stderr.String())
	}
}

// TestHostedMailboxWaitWokenByMailExitsZeroOnTheStream pins the final frame
// of a wait a mail wakes: exit_code is present and zero. The field used to
// be omitted when zero, and the launcher read a missing code as 1, so a
// delivered mail looked like a failed wait to the mod and the Pi waiter and
// the mail push never fired.
func TestHostedMailboxWaitWokenByMailExitsZeroOnTheStream(t *testing.T) {
	h := startHostedDaemon(t)
	const waitKey = "amber-tide-fox"
	root, err := wsstate.CacheRoot(wsstate.Options{CacheHome: h.cache})
	if err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(root, "mailbox-listening", waitKey+".json")
	wait := connectHosted(t, h.ep, h.hello(wsdashboard.ProxyModeCLI, []string{"mailbox", "wait", "--session-key", waitKey, "--timeout", "30s", "--format", "json"}, os.Getpid(), nil))
	waitUntil(t, 5*time.Second, "listening marker written", func() bool { _, err := os.Stat(marker); return err == nil })

	sender := connectHosted(t, h.ep, h.hello(wsdashboard.ProxyModeMCP, []string{"serve", "--stdio"}, os.Getpid(), nil))
	key := sender.login(h.project)
	sender.tool("mailbox.send", map[string]any{"session_key": key, "to": wsmailbox.ReplyIDPrefix + wsmailbox.ReplyID(waitKey), "content": "wake up"})

	var final map[string]any
	for {
		frame, ok := wait.readLine(15 * time.Second)
		if !ok {
			t.Fatal("stream ended before the final frame")
		}
		if frame["final"] == true {
			final = frame
			break
		}
	}
	code, present := final["exit_code"]
	if !present {
		t.Fatalf("the final frame carries no exit_code (the launcher would read 1): %v", final)
	}
	if code.(float64) != 0 || !strings.Contains(final["data"].(string), `"timed_out":false`) {
		t.Fatalf("final frame = %v, want exit_code 0 with the mail", final)
	}
}
