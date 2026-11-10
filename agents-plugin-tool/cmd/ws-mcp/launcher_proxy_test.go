package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wsmailbox"
)

// The end-to-end smoke of the daemon-hosted path: the real Python launcher,
// the real binary, a scratch cache root. The launcher must stage the binary,
// start the daemon from the staged copy, and serve `initialize`,
// `tools/list`, and a tool call through it; `mailbox wait` through the
// launcher must answer as the direct CLI does. (The server sends no
// server-initiated notification of its own, so pass-through of one is
// covered by the launcher's fake-daemon tests in agents-plugin/tests.)

func launcherTestPlugin(t *testing.T, bin string) (launcher string, env []string, cache, cwd string) {
	t.Helper()
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 is not on PATH")
	}
	versionOut, err := exec.Command(bin, "version").Output()
	if err != nil {
		t.Fatal(err)
	}
	version := strings.TrimSpace(string(versionOut))

	repo := filepath.Join("..", "..", "..", "agents-plugin")
	raw, err := os.ReadFile(filepath.Join(repo, "runtime.json"))
	if err != nil {
		t.Fatal(err)
	}
	var contract map[string]any
	if err := json.Unmarshal(raw, &contract); err != nil {
		t.Fatal(err)
	}
	// The launcher accepts the runtime only at the contract's exact version;
	// this contract stands for the built binary.
	contract["plugin_version"] = version
	plugin := t.TempDir()
	contractJSON, _ := json.MarshalIndent(contract, "", "  ")
	if err := os.WriteFile(filepath.Join(plugin, "runtime.json"), contractJSON, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(plugin, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	script, err := os.ReadFile(filepath.Join(repo, "bin", "ws-mcp-launcher.py"))
	if err != nil {
		t.Fatal(err)
	}
	launcher = filepath.Join(plugin, "bin", "ws-mcp-launcher.py")
	if err := os.WriteFile(launcher, script, 0o755); err != nil {
		t.Fatal(err)
	}
	rsrc, err := filepath.Abs(filepath.Join(repo, "rsrc"))
	if err != nil {
		t.Fatal(err)
	}

	env, _ = dashboardTestEnv(t)
	cache = cacheHomeFromEnv(t, env)
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
	// The manifest list is relative to the launcher's cwd, as Codex's
	// .mcp.json writes it (cwd "." and "./config-manifest.json"); the hello
	// must carry it absolute or the daemon reads it against its own cwd.
	manifest := writeDashboardTestManifest(t, true)
	cwd = filepath.Dir(manifest)
	env = append(env,
		"WS_MCP_BOOTSTRAP_BINARY="+bin,
		"WS_MCP_RUNTIME_DIR="+filepath.Join(t.TempDir(), "runtime"),
		"WS_RSRC_ROOT="+rsrc,
		"WS_MCP_PROJECT_ROOT="+project,
		"WS_MCP_CONFIG_MANIFESTS=./"+filepath.Base(manifest),
		"WS_MCP_LAUNCHER_DEBUG=1",
	)
	_ = python
	return launcher, env, cache, cwd
}

type launcherClient struct {
	t      *testing.T
	cmd    *exec.Cmd
	stdin  *os.File
	out    *bufio.Reader
	stderr *bytes.Buffer
	nextID int
}

func startLauncher(t *testing.T, launcher, dir string, env []string, args ...string) *launcherClient {
	t.Helper()
	cmd := exec.Command("python3", append([]string{launcher}, args...)...)
	cmd.Env = env
	cmd.Dir = dir
	stdinR, stdinW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stdin = stdinR
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	stdinR.Close()
	t.Cleanup(func() {
		stdinW.Close()
		if cmd.ProcessState == nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	})
	return &launcherClient{t: t, cmd: cmd, stdin: stdinW, out: bufio.NewReader(stdout), stderr: &stderr}
}

func (c *launcherClient) call(method string, params any) map[string]any {
	c.t.Helper()
	c.nextID++
	msg := map[string]any{"jsonrpc": "2.0", "id": c.nextID, "method": method}
	if params != nil {
		msg["params"] = params
	}
	raw, _ := json.Marshal(msg)
	if _, err := c.stdin.Write(append(raw, '\n')); err != nil {
		c.t.Fatalf("write %s: %v", method, err)
	}
	type lineResult struct {
		line []byte
		err  error
	}
	got := make(chan lineResult, 1)
	go func() {
		line, err := c.out.ReadBytes('\n')
		got <- lineResult{line, err}
	}()
	select {
	case r := <-got:
		if r.err != nil {
			c.t.Fatalf("%s: no answer: %v\nstderr:\n%s", method, r.err, c.stderr.String())
		}
		var resp map[string]any
		if err := json.Unmarshal(r.line, &resp); err != nil {
			c.t.Fatalf("%s: answer %q: %v", method, r.line, err)
		}
		if id, _ := resp["id"].(float64); int(id) != c.nextID {
			c.t.Fatalf("%s: answer id %v, want %d: %s", method, resp["id"], c.nextID, r.line)
		}
		return resp
	case <-time.After(60 * time.Second):
		c.t.Fatalf("%s: no answer in 60s\nstderr:\n%s", method, c.stderr.String())
		return nil
	}
}

func (c *launcherClient) notify(method string) {
	c.t.Helper()
	raw, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "method": method})
	if _, err := c.stdin.Write(append(raw, '\n')); err != nil {
		c.t.Fatal(err)
	}
}

func TestLauncherServesThroughTheDaemonItStartsFromTheStagedCopy(t *testing.T) {
	bin := buildDashboardTestBin(t, "")
	launcher, env, cache, cwd := launcherTestPlugin(t, bin)
	versionOut, _ := exec.Command(bin, "version").Output()
	version := strings.TrimSpace(string(versionOut))

	client := startLauncher(t, launcher, cwd, env, "serve", "--stdio")
	init := client.call("initialize", map[string]any{"protocolVersion": "2025-03-26", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "smoke", "version": "0"}})
	result, _ := init["result"].(map[string]any)
	serverInfo, _ := result["serverInfo"].(map[string]any)
	if serverInfo["version"] != version {
		t.Fatalf("initialize result = %v, want serverInfo.version %q\nstderr:\n%s", init, version, client.stderr.String())
	}
	client.notify("notifications/initialized")
	list := client.call("tools/list", nil)
	tools, _ := list["result"].(map[string]any)["tools"].([]any)
	names := map[string]bool{}
	for _, tool := range tools {
		names[tool.(map[string]any)["name"].(string)] = true
	}
	if !names["runtime.read"] || !names["todo.list"] {
		t.Fatalf("tools/list = %v", list)
	}
	read := client.call("tools/call", map[string]any{"name": "runtime.read", "arguments": map[string]any{"format": "json"}})
	if _, ok := read["result"]; !ok || read["error"] != nil {
		t.Fatalf("runtime.read = %v", read)
	}
	if !strings.Contains(toolText(t, read), version) {
		t.Fatalf("runtime.read answer %q does not name the version %q", toolText(t, read), version)
	}

	// The relative manifest reached the hosted server: its dashboard is
	// declared, so the lead session this login opens registers.
	ferrule := client.call("tools/call", map[string]any{"name": "ferrule", "arguments": map[string]any{"root": envValue(t, env, "WS_MCP_PROJECT_ROOT")}})
	var key string
	for _, segment := range strings.Split(toolText(t, ferrule), "\n") {
		if k, ok := strings.CutPrefix(strings.TrimSpace(segment), "session_key:"); ok {
			key = strings.TrimSpace(k)
		}
	}
	if key == "" {
		t.Fatalf("ferrule answered no session key: %v", ferrule)
	}
	if list := client.call("tools/call", map[string]any{"name": "todo.list", "arguments": map[string]any{"session_key": key}}); list["error"] != nil {
		t.Fatalf("todo.list: %v", list["error"])
	}
	stateDir, _ := wsdashboard.Dir(cache)
	if reg, err := wsdashboard.ReadRegistration(stateDir, wsmailbox.ReplyID(key)); err != nil || !reg.Live(time.Now()) {
		t.Fatalf("hosted lead session not registered (the relative manifest was lost?): %+v, %v\nstderr:\n%s", reg, err, client.stderr.String())
	}

	// The daemon runs from the staged copy, and it alone.
	dir, _ := wsdashboard.Dir(cache)
	ep := waitForEndpoint(t, dir, version, 5*time.Second)
	// Its name follows Go's StageExecutable rule for the runtime binary the
	// launcher staged (a copy of bin under the runtime dir, named
	// ws-mcp-<version>-<hash>): sha256[:16] of the content plus
	// filepath.Ext of that name, lower-cased.
	runtimeEntries, err := os.ReadDir(envValue(t, env, "WS_MCP_RUNTIME_DIR"))
	if err != nil {
		t.Fatal(err)
	}
	runtimeName := ""
	for _, e := range runtimeEntries {
		if strings.HasPrefix(e.Name(), "ws-mcp-") {
			runtimeName = e.Name()
		}
	}
	if runtimeName == "" {
		t.Fatalf("runtime dir entries = %v, want the staged runtime binary", runtimeEntries)
	}
	wantStaged := "ws-mcp-dashboard-" + wsdashboard.ExecutableContentHash(bin)[:16] + strings.ToLower(filepath.Ext(runtimeName))
	entries, err := os.ReadDir(filepath.Join(dir, "bin"))
	if err != nil || len(entries) != 1 || entries[0].Name() != wantStaged {
		t.Fatalf("staged binaries = %v (%v), want exactly %s (the Python staging must name the copy as Go's StageExecutable does)", entries, err, wantStaged)
	}
	if ep.PID == client.cmd.Process.Pid {
		t.Fatal("the daemon is the launcher process")
	}
	if strings.Contains(client.stderr.String(), "direct path") {
		t.Fatalf("the launcher fell back to the direct path:\n%s", client.stderr.String())
	}

	client.stdin.Close()
	if err := waitExit(t, client.cmd, 10*time.Second); err != nil {
		t.Fatalf("launcher exit: %v\nstderr:\n%s", err, client.stderr.String())
	}
	// The daemon outlives the session.
	if _, err := wsdashboard.Health(context.Background(), dashboardTestClient, ep); err != nil {
		t.Fatalf("daemon health after the session closed: %v", err)
	}

	// A second session connects to the running daemon without another start.
	second := startLauncher(t, launcher, cwd, env, "serve", "--stdio")
	second.call("initialize", map[string]any{"protocolVersion": "2025-03-26", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "smoke2", "version": "0"}})
	if strings.Contains(second.stderr.String(), "starting the daemon") {
		t.Fatalf("the second session started a daemon:\n%s", second.stderr.String())
	}
	second.stdin.Close()
	if err := waitExit(t, second.cmd, 10*time.Second); err != nil {
		t.Fatalf("second launcher exit: %v", err)
	}
	if ep2, err := wsdashboard.ReadEndpoint(dir); err != nil || ep2.PID != ep.PID {
		t.Fatalf("endpoint after the second session = %+v (%v), want the same daemon %d", ep2, err, ep.PID)
	}
}

func toolText(t *testing.T, resp map[string]any) string {
	t.Helper()
	content, _ := resp["result"].(map[string]any)["content"].([]any)
	var b strings.Builder
	for _, item := range content {
		if text, ok := item.(map[string]any)["text"].(string); ok {
			b.WriteString(text)
		}
	}
	return b.String()
}

func TestLauncherRunsMailboxWaitThroughTheDaemonLikeTheDirectCLI(t *testing.T) {
	bin := buildDashboardTestBin(t, "")
	launcher, env, _, cwd := launcherTestPlugin(t, bin)

	run := func(name string, argv ...string) (string, string, int) {
		t.Helper()
		cmd := exec.Command(name, argv...)
		cmd.Env = env
		cmd.Dir = cwd
		var stdout, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		err := cmd.Run()
		code := 0
		if exitErr, ok := err.(*exec.ExitError); ok {
			code = exitErr.ExitCode()
		} else if err != nil {
			t.Fatalf("%s %v: %v", name, argv, err)
		}
		return stdout.String(), stderr.String(), code
	}
	waitArgs := []string{"mailbox", "wait", "--session-key", "amber-tide-fox", "--timeout", "1s", "--format", "json"}
	hostedOut, hostedErr, hostedCode := run("python3", append([]string{launcher}, waitArgs...)...)
	directOut, _, directCode := run(bin, waitArgs...)
	if hostedCode != mailboxWaitExitTimeout || directCode != mailboxWaitExitTimeout {
		t.Fatalf("exit codes: hosted %d, direct %d, want %d\nhosted stderr:\n%s", hostedCode, directCode, mailboxWaitExitTimeout, hostedErr)
	}
	if strings.Contains(hostedErr, "direct path") {
		t.Fatalf("mailbox wait fell back to the direct path:\n%s", hostedErr)
	}
	var hosted, direct map[string]any
	if err := json.Unmarshal([]byte(hostedOut), &hosted); err != nil {
		t.Fatalf("hosted stdout %q: %v\nstderr:\n%s", hostedOut, err, hostedErr)
	}
	if err := json.Unmarshal([]byte(directOut), &direct); err != nil {
		t.Fatalf("direct stdout %q: %v", directOut, err)
	}
	// The re-arm command starts with the CLI command name: the launcher's
	// recorded runtime binary for the hosted run, argv[0] for the direct
	// one. Its arguments must agree.
	const rearmTail = " mailbox wait --session-key amber-tide-fox --timeout 1s"
	for name, answer := range map[string]map[string]any{"hosted": hosted, "direct": direct} {
		rearm, _ := answer["rearm"].(map[string]any)
		command, _ := rearm["command"].(string)
		if !strings.HasSuffix(command, rearmTail) {
			t.Fatalf("%s rearm command = %q, want the suffix %q", name, command, rearmTail)
		}
		delete(rearm, "command")
	}
	for _, key := range []string{"elapsed", "elapsed_ms", "waited", "waited_ms", "deadline", "started_at", "ended_at"} {
		delete(hosted, key)
		delete(direct, key)
	}
	hostedJSON, _ := json.Marshal(hosted)
	directJSON, _ := json.Marshal(direct)
	if string(hostedJSON) != string(directJSON) {
		t.Fatalf("mailbox wait answers differ:\nhosted %s\ndirect %s", hostedJSON, directJSON)
	}
}
