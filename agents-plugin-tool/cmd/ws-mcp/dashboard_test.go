package main

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/mcp"
	"github.com/kang-sw/devenv/internal/wsdashboard"
)

var dashboardTestClient = &http.Client{Timeout: 2 * time.Second}

func TestStartDashboardWithTheKeyOffSpawnsNothing(t *testing.T) {
	dir := t.TempDir()
	calls := 0
	outcome := startDashboard(context.Background(), mcp.DashboardSettings{Enabled: false, Port: 12345}, dir, func([]string) error { calls++; return nil }, io.Discard)
	if calls != 0 || outcome != "" {
		t.Fatalf("key off: %d spawns, outcome %q", calls, outcome)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Fatalf("key off wrote %v", entries)
	}
}

func TestStartDashboardPassesThePortKeyAsTheDaemonFlag(t *testing.T) {
	var got []string
	var log bytes.Buffer
	outcome := startDashboard(context.Background(), mcp.DashboardSettings{Enabled: true, Port: 12345}, t.TempDir(), func(args []string) error {
		got = args
		return errors.New("spawn refused in test")
	}, &log)
	if want := []string{"dashboard", "--port", "12345"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("spawn args = %v, want %v", got, want)
	}
	if outcome != wsdashboard.OutcomeUnavailable || !strings.Contains(log.String(), "dashboard unavailable") {
		t.Fatalf("outcome %q, log %q", outcome, log.String())
	}
}

// buildDashboardTestBin builds ws-mcp, optionally stamping another version.
func buildDashboardTestBin(t *testing.T, stampVersion string) string {
	t.Helper()
	bin := wsMCPTestBin(t)
	args := []string{"build", "-o", bin}
	if stampVersion != "" {
		args = append(args, "-ldflags", "-X main.version="+stampVersion)
	}
	build := exec.Command("go", append(args, ".")...)
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build failed: %v\n%s", err, out)
	}
	return bin
}

// dashboardTestEnv isolates the cache (and so the daemon directory) and the
// config home, and stops whatever daemon is left there when the test ends.
func dashboardTestEnv(t *testing.T) (env []string, dir string) {
	t.Helper()
	cache := filepath.Join(t.TempDir(), "cache")
	env = append(os.Environ(), "WS_CACHE_HOME="+cache, "WS_CONFIG_HOME="+t.TempDir())
	dir, err := wsdashboard.Dir(cache)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if ep, err := wsdashboard.ReadEndpoint(dir); err == nil {
			_ = wsdashboard.RequestShutdown(context.Background(), dashboardTestClient, ep)
		}
	})
	return env, dir
}

// waitForEndpoint polls dir until a healthy daemon of the wanted version (any
// version when empty) answers, failing the test after the deadline.
func waitForEndpoint(t *testing.T, dir, wantVersion string, deadline time.Duration) wsdashboard.Endpoint {
	t.Helper()
	end := time.Now().Add(deadline)
	for time.Now().Before(end) {
		if ep, err := wsdashboard.ReadEndpoint(dir); err == nil {
			if info, err := wsdashboard.Health(context.Background(), dashboardTestClient, ep); err == nil && (wantVersion == "" || info.Version == wantVersion) {
				return ep
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("no healthy daemon of version %q in %s", wantVersion, deadline)
	return wsdashboard.Endpoint{}
}

func waitExit(t *testing.T, cmd *exec.Cmd, deadline time.Duration) error {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		return err
	case <-time.After(deadline):
		_ = cmd.Process.Kill()
		t.Fatalf("%s did not exit in %s", cmd.Path, deadline)
		return nil
	}
}

func TestDashboardSubcommandHoldsTheLockUntilAnAuthenticatedShutdown(t *testing.T) {
	bin := buildDashboardTestBin(t, "")
	env, dir := dashboardTestEnv(t)

	first := exec.Command(bin, "dashboard", "--port", "0")
	first.Env = env
	if err := first.Start(); err != nil {
		t.Fatal(err)
	}
	ep := waitForEndpoint(t, dir, version, 10*time.Second)
	if ep.PID != first.Process.Pid {
		t.Fatalf("endpoint pid %d, daemon pid %d", ep.PID, first.Process.Pid)
	}

	second := exec.Command(bin, "dashboard", "--port", "0", "--lock-wait", "200ms")
	second.Env = env
	out, err := second.CombinedOutput()
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 3 || !strings.Contains(string(out), "another dashboard daemon is running") {
		t.Fatalf("second daemon: err %v, output %q; want exit 3 refused by the lock", err, out)
	}
	if again, _ := wsdashboard.ReadEndpoint(dir); again != ep {
		t.Fatalf("refused daemon touched the endpoint file: %+v", again)
	}

	if err := wsdashboard.RequestShutdown(context.Background(), dashboardTestClient, ep); err != nil {
		t.Fatal(err)
	}
	if err := waitExit(t, first, 10*time.Second); err != nil {
		t.Fatalf("daemon exit: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "endpoint.json")); !os.IsNotExist(err) {
		t.Fatalf("endpoint file after shutdown: %v", err)
	}
}

// writeDashboardTestManifest writes a dashboard manifest like the shipped
// ones, but with port 0, so a test never binds the real default port.
func writeDashboardTestManifest(t *testing.T, enabled bool) string {
	t.Helper()
	value := "false"
	if enabled {
		value = "true"
	}
	path := filepath.Join(t.TempDir(), "dashboard-config-manifest.json")
	body := `{"schema_version": 1, "namespace": "dashboard.", "keys": [
  {"key": "dashboard.enabled", "type": "boolean", "default": ` + value + `, "description": "test"},
  {"key": "dashboard.port", "type": "integer", "minimum": 0, "maximum": 65535, "default": 0, "description": "test"}]}`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// startServe starts `ws-mcp serve --stdio`, sends initialize and tools/list,
// and returns once tools/list has answered. Closing the returned stdin ends
// the serve.
func startServe(t *testing.T, bin string, env []string) (*exec.Cmd, io.WriteCloser) {
	t.Helper()
	serve := exec.Command(bin, "serve", "--stdio", "--root", t.TempDir())
	serve.Env = env
	stdin, err := serve.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := serve.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := serve.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = serve.Process.Kill() })
	_, _ = io.WriteString(stdin, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`+"\n"+`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`+"\n")
	answered := make(chan string, 1)
	go func() {
		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 0, 1<<20), 16<<20)
		for scanner.Scan() {
			if line := scanner.Text(); strings.Contains(line, `"id":2`) {
				answered <- line
				break
			}
		}
		_, _ = io.Copy(io.Discard, stdout)
	}()
	select {
	case line := <-answered:
		if !strings.Contains(line, `"project_tree"`) {
			t.Fatalf("tools/list answer lacks tools: %.300s", line)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("serve did not answer tools/list")
	}
	return serve, stdin
}

// TestServeReplacesAnOlderDaemonThatThenOutlivesServe: the handoff end to end
// with real processes. An older daemon runs; a newer ws-mcp serve answers its
// tools at once, asks the older daemon to stop, and starts a detached daemon
// that writes its endpoint file and keeps serving after the serve exits.
func TestServeReplacesAnOlderDaemonThatThenOutlivesServe(t *testing.T) {
	oldBin := buildDashboardTestBin(t, "0.0.1")
	newBin := buildDashboardTestBin(t, "")
	env, dir := dashboardTestEnv(t)

	old := exec.Command(oldBin, "dashboard", "--port", "0")
	old.Env = env
	if err := old.Start(); err != nil {
		t.Fatal(err)
	}
	oldEp := waitForEndpoint(t, dir, "0.0.1", 10*time.Second)

	serveEnv := append(env, "WS_MCP_CONFIG_MANIFESTS="+writeDashboardTestManifest(t, true))
	serve, stdin := startServe(t, newBin, serveEnv)
	newEp := waitForEndpoint(t, dir, version, 20*time.Second)
	if newEp.Token == oldEp.Token || newEp.PID == oldEp.PID {
		t.Fatalf("endpoint not replaced: %+v", newEp)
	}
	if err := waitExit(t, old, 10*time.Second); err != nil {
		t.Fatalf("older daemon exit: %v", err)
	}

	_ = stdin.Close()
	if err := waitExit(t, serve, 10*time.Second); err != nil {
		t.Fatalf("serve exit: %v", err)
	}
	if info, err := wsdashboard.Health(context.Background(), dashboardTestClient, newEp); err != nil || info.Version != version {
		t.Fatalf("daemon after serve exited: %+v, %v", info, err)
	}
}

// TestServeAnswersToolsWithoutADaemon: serve's MCP tools answer normally when
// the dashboard is off, its manifest is missing, or the daemon cannot start.
func TestServeAnswersToolsWithoutADaemon(t *testing.T) {
	bin := buildDashboardTestBin(t, "")
	claudeManifest, err := filepath.Abs(filepath.Join("..", "..", "..", "agents-plugin", "config-manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	notADir := filepath.Join(t.TempDir(), "cache-is-a-file")
	if err := os.WriteFile(notADir, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name      string
		manifests string
		cache     string // overrides the isolated cache home
	}{
		{"key off", writeDashboardTestManifest(t, false), ""},
		{"manifest missing", claudeManifest + ";" + filepath.Join(t.TempDir(), "absent.json"), ""},
		{"daemon cannot start", writeDashboardTestManifest(t, true), notADir},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env, dir := dashboardTestEnv(t)
			env = append(env, "WS_MCP_CONFIG_MANIFESTS="+tc.manifests)
			if tc.cache != "" {
				env = append(env, "WS_CACHE_HOME="+tc.cache)
			}
			serve, stdin := startServe(t, bin, env)
			_ = stdin.Close()
			if err := waitExit(t, serve, 10*time.Second); err != nil {
				t.Fatalf("serve exit: %v", err)
			}
			if tc.cache == "" {
				if _, err := os.Stat(dir); !os.IsNotExist(err) {
					t.Fatalf("dashboard directory exists with the dashboard off: %v", err)
				}
			}
		})
	}
}
