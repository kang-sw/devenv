package main

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/mcp"
	"github.com/kang-sw/devenv/internal/wsconfig"
	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wsmailbox"
	"github.com/kang-sw/devenv/internal/wsstate"
)

func cliHello(t *testing.T) wsdashboard.ProxyHello {
	t.Helper()
	root := t.TempDir()
	if out, err := exec.Command("git", "init", "-q", root).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	if err := os.MkdirAll(filepath.Join(root, "ai-docs", "tickets", "idea"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte("# Project\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	env := map[string]string{}
	for _, entry := range os.Environ() {
		if key, value, ok := strings.Cut(entry, "="); ok {
			env[key] = value
		}
	}
	rsrc, err := filepath.Abs(filepath.Join("..", "..", "..", "agents-plugin", "rsrc"))
	if err != nil {
		t.Fatal(err)
	}
	env["WS_RSRC_ROOT"] = rsrc
	env["WS_CACHE_HOME"], env["WS_CONFIG_HOME"] = t.TempDir(), t.TempDir()
	env["WS_MCP_CONFIG_MANIFESTS"], env["WS_MCP_ALLOWED_TOOLS"] = "", ""
	env["WS_MCP_NO_DASHBOARD"], env["WS_MCP_NO_AGENT"] = "1", ""
	env["WS_MCP_PROJECT_ROOT"] = root
	env["WS_MCP_CLI_NAME"] = "ws-cli"
	env["GIT_AUTHOR_NAME"], env["GIT_AUTHOR_EMAIL"] = "Test", "test@example.test"
	env["GIT_COMMITTER_NAME"], env["GIT_COMMITTER_EMAIL"] = "Test", "test@example.test"
	return wsdashboard.ProxyHello{Protocol: 1, Mode: "cli", Cwd: root, Env: env, OwnerPID: os.Getpid(), Version: version}
}

// Run the real host/session/frame path without starting any ws executable.
func hostedCLIResult(t *testing.T, hello wsdashboard.ProxyHello, argv ...string) (string, string, int) {
	t.Helper()
	return hostedCLIResultWithHost(t, &proxyHost{logw: io.Discard}, hello, argv...)
}

func hostedCLIResultWithHost(t *testing.T, host *proxyHost, hello wsdashboard.ProxyHello, argv ...string) (string, string, int) {
	t.Helper()
	hello.Argv = argv
	session, err := host.Accept(hello)
	if err != nil {
		t.Fatal(err)
	}
	client, server := net.Pipe()
	defer client.Close()
	_ = client.SetDeadline(time.Now().Add(15 * time.Second))
	go func() { defer server.Close(); session.Serve(context.Background(), server) }()
	var stderr strings.Builder
	scanner := bufio.NewScanner(client)
	scanner.Buffer(make([]byte, 4096), 4<<20)
	for scanner.Scan() {
		var frame wsdashboard.ProxyCLIFrame
		if err := json.Unmarshal(scanner.Bytes(), &frame); err != nil {
			t.Fatal(err)
		}
		if frame.Final {
			return frame.Data, stderr.String(), frame.ExitCode
		}
		if frame.Stream != "stderr" {
			t.Fatalf("unexpected frame: %+v", frame)
		}
		stderr.WriteString(frame.Data)
	}
	t.Fatalf("no final frame: %v", scanner.Err())
	return "", "", -1
}

// configStorePath decodes `config list --format json` rather than matching
// raw text: JSON escapes Windows path separators.
func configStorePath(out string) string {
	var payload struct {
		Path string `json:"path"`
	}
	if json.Unmarshal([]byte(out), &payload) != nil {
		return ""
	}
	return payload.Path
}

// pathWithin compares symlink-resolved paths: the store may report either form
// (macOS /var -> /private/var), and the store file need not exist yet.
func pathWithin(path, root string) bool {
	if path == "" || root == "" {
		return false
	}
	resolve := func(p string) string {
		// Resolve the longest existing prefix and re-append the rest.
		for dir, rest := filepath.Clean(p), ""; ; {
			if r, err := filepath.EvalSymlinks(dir); err == nil {
				return filepath.Join(r, rest)
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				return p
			}
			dir, rest = parent, filepath.Join(filepath.Base(dir), rest)
		}
	}
	rel, err := filepath.Rel(resolve(root), resolve(path))
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func TestHostedCLIExistingShapesAndOldHello(t *testing.T) {
	hello := cliHello(t) // No stdin or project_root: an older hello still works.
	for _, tc := range []struct {
		args     []string
		contains string
		code     int
	}{
		{[]string{"version"}, version, 0},
		{[]string{"runtime", "info"}, `"source_commit"`, 0},
		{[]string{"runtime", "capabilities"}, `"tickets.move"`, 0},
		{[]string{"doctor"}, "", 0},
		{[]string{"smoke"}, "ok stdio smoke:", 0},
		{[]string{"config", "list", "--format", "json"}, `"config"`, 0},
		{[]string{"git", "status", "--root", ".", "--format", "json"}, "", 0},
		{[]string{"git", "diff", "--format", "json"}, "", 0},
		{[]string{"path", "generate", "--kind", "review", "hosted-review"}, "hosted-review.md", 0},
		{[]string{"tickets", "list", "--format", "json"}, "[]", 0},
		{[]string{"tickets", "verify", "--format", "json"}, "", 1},
		{[]string{"tools", "runtime.read"}, `"properties"`, 0},
		{[]string{"call", "runtime.read", `{"format":"json"}`}, version, 0},
		{[]string{"call", "runtime.read", `{`}, "", 1},
		{[]string{"doctor", "--bad-flag"}, "", 2},
		{[]string{"unknown"}, "", 2},
	} {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			out, stderr, code := hostedCLIResult(t, hello, tc.args...)
			if code != tc.code || !strings.Contains(out, tc.contains) {
				t.Fatalf("out=%q stderr=%q code=%d; want %d containing %q", out, stderr, code, tc.code, tc.contains)
			}
		})
	}
}

func TestHostedCLIConcurrentCallerIsolation(t *testing.T) {
	a, b := cliHello(t), cliHello(t)
	a.Env["WS_MCP_NO_AGENT"], a.Env["WS_MCP_ALLOWED_TOOLS"] = "1", "runtime.read"
	b.Env["WS_MCP_ALLOWED_TOOLS"] = "project_tree"
	a.Env["WS_MCP_CONFIG_MANIFESTS"] = writeProxyTestManifest(t, false)
	a.Env["WS_MAILBOX"] = "first@machine"
	b.Env["WS_MAILBOX"] = "second@machine"
	cwd, _ := os.Getwd()
	processEnv := strings.Join(os.Environ(), "\n")
	processOut := os.Stdout
	var wg sync.WaitGroup
	for _, hello := range []wsdashboard.ProxyHello{a, b} {
		hello := hello
		wg.Add(1)
		go func() {
			defer wg.Done()
			out, stderr, code := hostedCLIResult(t, hello, "config", "list", "--format", "json")
			if code != 0 || !pathWithin(configStorePath(out), hello.Env["WS_CACHE_HOME"]) {
				t.Errorf("wrong config store: %s %s (%d)", out, stderr, code)
			}
			out, stderr, code = hostedCLIResult(t, hello, "tools")
			if code != 0 {
				t.Errorf("tools: %s (%d)", stderr, code)
			}
			allowed := hello.Env["WS_MCP_ALLOWED_TOOLS"]
			if !strings.Contains(out, "\n"+allowed+":") {
				t.Errorf("tools lost allowlist %s: %s", allowed, out)
			}
			other := "runtime.read"
			if allowed == other {
				other = "project_tree"
			}
			if strings.Contains(out, "\n"+other+":") {
				t.Errorf("tools leaked other profile: %s", out)
			}
		}()
	}
	wg.Wait()
	if got, _ := os.Getwd(); got != cwd || strings.Join(os.Environ(), "\n") != processEnv || os.Stdout != processOut {
		t.Fatal("hosted CLI mutated process context")
	}
	out, _, code := hostedCLIResult(t, a, "runtime", "capabilities")
	if code != 0 || strings.Contains(out, `"api.ask"`) {
		t.Fatalf("capabilities inherited daemon product mode: %s", out)
	}
}

func TestHostedCLIOmittedHomeOverridesDoNotInheritDaemon(t *testing.T) {
	hello := cliHello(t)
	home := t.TempDir()
	hello.Env["HOME"], hello.Env["USERPROFILE"] = home, home
	delete(hello.Env, "WS_CACHE_HOME")
	delete(hello.Env, "WS_CONFIG_HOME")
	t.Setenv("WS_CACHE_HOME", t.TempDir())
	t.Setenv("WS_CONFIG_HOME", t.TempDir())
	out, stderr, code := hostedCLIResult(t, hello, "config", "list", "--format", "json")
	if store := configStorePath(out); code != 0 || !pathWithin(store, home) || pathWithin(store, os.Getenv("WS_CACHE_HOME")) {
		t.Fatalf("omitted overrides inherited daemon: %s %s (%d)", out, stderr, code)
	}
	if _, added := hello.Env["WS_CACHE_HOME"]; added {
		t.Fatal("Accept mutated caller environment")
	}
}

func TestHostedCLIAuthenticatedUpgradeCarriesOptionalStdin(t *testing.T) {
	h := startHostedDaemon(t)
	hello := h.hello(wsdashboard.ProxyModeCLI, []string{"git", "commit", "--description-file", "-"}, os.Getpid(), nil)
	hello.Stdin = "one-shot input"
	conn := connectHosted(t, h.ep, hello)
	var final map[string]any
	for {
		frame, ok := conn.readLine(5 * time.Second)
		if !ok {
			t.Fatal("CLI stream ended without final frame")
		}
		if frame["final"] == true {
			final = frame
			break
		}
	}
	// Argument validation fails, but the hosted command returned a frame rather
	// than refusing the expanded argv or requiring a new hello protocol.
	if final["exit_code"] != float64(1) {
		t.Fatalf("commit frame: %v", final)
	}
	old := h.hello(wsdashboard.ProxyModeCLI, []string{"version"}, os.Getpid(), nil)
	frame, ok := connectHosted(t, h.ep, old).readLine(5 * time.Second)
	if !ok || frame["final"] != true || frame["exit_code"] != float64(0) || frame["data"] != version+"\n" {
		t.Fatalf("old hello: %v", frame)
	}
}

func TestHostedCLIStdinCommitAndRelativeRoots(t *testing.T) {
	hello := cliHello(t)
	root := hello.Cwd
	if err := os.WriteFile(filepath.Join(root, "change.txt"), []byte("change\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	branch, err := exec.Command("git", "-C", root, "symbolic-ref", "--short", "HEAD").Output()
	if err != nil {
		t.Fatal(err)
	}
	hello.Cwd = filepath.Dir(root)
	hello.ProjectRoot = "wrong-root"
	hello.Stdin = "Description from stdin: 한글\n"
	out, stderr, code := hostedCLIResult(t, hello, "git", "commit", "--root", filepath.Base(root), "--title", "test: hosted commit", "--description-file", "-", "--expected-branch", strings.TrimSpace(string(branch)), "--ai-context", "Verify caller input", "--path", "change.txt")
	if code != 0 {
		t.Fatalf("commit: %s %s (%d)", out, stderr, code)
	}
	cmd := exec.Command("git", "-C", root, "log", "-1", "--format=%B")
	body, err := cmd.Output()
	if err != nil || !strings.Contains(string(body), hello.Stdin) {
		t.Fatalf("commit body=%q err=%v", body, err)
	}
}

func TestHostedCLITicketWriteUsesExplicitRootWithoutMintingKeys(t *testing.T) {
	hello := cliHello(t)
	target := cliHello(t)
	hello.ProjectRoot = hello.Cwd
	out, stderr, code := hostedCLIResult(t, hello, "tickets", "create-empty", "--root", target.Cwd, "--stem", "bug-hosted-routing", "--initial-state", "idea")
	if code != 0 {
		t.Fatalf("ticket create: %s %s (%d)", out, stderr, code)
	}
	files, err := filepath.Glob(filepath.Join(target.Cwd, "ai-docs", "tickets", "idea", "*-bug-hosted-routing.md"))
	if err != nil || len(files) != 1 {
		t.Fatalf("ticket target: %v %v", files, err)
	}
	wrong, _ := filepath.Glob(filepath.Join(hello.Cwd, "ai-docs", "tickets", "idea", "*.md"))
	if len(wrong) != 0 {
		t.Fatalf("write used hello root instead of explicit target: %v", wrong)
	}
	if entries, _ := os.ReadDir(filepath.Join(hello.Env["WS_CACHE_HOME"], "sessions")); len(entries) != 0 {
		t.Fatalf("CLI write minted keys: %v", entries)
	}
}

func TestHostedCodexStopPayloadScopeAndFailOpen(t *testing.T) {
	for _, scope := range []wsmailbox.Scope{wsmailbox.ScopeMachine, wsmailbox.ScopeWorktree, wsmailbox.ScopeClone} {
		t.Run(string(scope), func(t *testing.T) {
			hello := cliHello(t)
			hello.ProjectRoot = t.TempDir() // Payload cwd has priority.
			hello.Env["WS_MAILBOX"] = "lead@" + string(scope)
			hello.Env["WS_MAILBOX_AUTO"] = "ignored@machine"
			paths := wsmailbox.Paths{Config: wsconfig.Options{ConfigHome: hello.Env["WS_CONFIG_HOME"]}, State: wsstate.Options{CacheHome: hello.Env["WS_CACHE_HOME"], Env: mcp.EnvFromMap(hello.Env)}}
			path, err := paths.ForScope(scope, hello.Cwd)
			if err != nil {
				t.Fatal(err)
			}
			if err := wsmailbox.WithLock(path, func(store *wsmailbox.StoreFile) error {
				store.Queues["lead"] = append(store.Queues["lead"], wsmailbox.Envelope{Content: "mail"})
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			for _, tc := range []struct {
				input   string
				block   bool
				warning bool
			}{
				{`{"stop_hook_active":true}`, false, false},
				{`{`, false, true},
				{`{"cwd":` + strconvQuote(hello.Cwd) + `}`, true, false},
				{`{"cwd":` + strconvQuote(hello.Cwd) + `}`, false, false},
			} {
				hello.Stdin = tc.input
				out, stderr, code := hostedCLIResult(t, hello, "mailbox", "codex-stop-hook")
				if code != 0 || strings.Contains(out, `"decision":"block"`) != tc.block || strings.Contains(stderr, "warning:") != tc.warning {
					t.Fatalf("payload=%s out=%q stderr=%q code=%d", tc.input, out, stderr, code)
				}
			}
			// Same slug and payload, different config/cache: no queue leaks.
			hello.Env["WS_CONFIG_HOME"], hello.Env["WS_CACHE_HOME"] = t.TempDir(), t.TempDir()
			out, stderr, code := hostedCLIResult(t, hello, "mailbox", "codex-stop-hook")
			if code != 0 || out != "" || stderr != "" {
				t.Fatalf("other caller: %q %q %d", out, stderr, code)
			}
		})
	}
}

func strconvQuote(s string) string { raw, _ := json.Marshal(s); return string(raw) }

func TestHostedCLIWaitDisconnectCancelsOwnerMarker(t *testing.T) {
	hello := cliHello(t)
	hello.Argv = []string{"mailbox", "wait", "--session-key", "amber-tide-fox"}
	session, err := (&proxyHost{logw: io.Discard}).Accept(hello)
	if err != nil {
		t.Fatal(err)
	}
	client, server := net.Pipe()
	done := make(chan struct{})
	go func() { defer close(done); defer server.Close(); session.Serve(context.Background(), server) }()
	paths := wsmailbox.Paths{State: wsstate.Options{CacheHome: hello.Env["WS_CACHE_HOME"]}}
	waitUntil(t, 3*time.Second, "listening marker", func() bool {
		marker, ok, err := paths.ReadListeningMarker("amber-tide-fox")
		return err == nil && ok && marker.PID == hello.OwnerPID
	})
	client.Close()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("disconnect did not cancel CLI wait")
	}
	if marker, ok, err := paths.ReadListeningMarker("amber-tide-fox"); err != nil || ok {
		t.Fatalf("marker remains: %+v %v", marker, err)
	}
}
