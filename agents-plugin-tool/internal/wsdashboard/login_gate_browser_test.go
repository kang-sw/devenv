package wsdashboard

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wslive"
)

// File-backed native clipboard/clock seams let Chromium exercise the real auth
// handlers without ever starting pbcopy or changing the daemon host's clipboard.
type loginGateBrowserClipboard struct{ path string }

func (w loginGateBrowserClipboard) Available() bool {
	_, err := os.Stat(w.path + ".unavailable")
	return os.IsNotExist(err)
}

func (w loginGateBrowserClipboard) WriteText(_ context.Context, text string) error {
	if _, err := os.Stat(w.path + ".fail"); err == nil {
		return errors.New("fixture clipboard unavailable")
	}
	return os.WriteFile(w.path, []byte(text), 0o600)
}

func TestLoginGateBrowser(t *testing.T) {
	// Resolve the module and browser cache before HOME becomes disposable.
	module := os.Getenv("WS_PLAYWRIGHT_MODULE")
	if module == "" {
		t.Skip("set WS_PLAYWRIGHT_MODULE to run Chromium acceptance")
	}
	module, err := filepath.Abs(module)
	if err != nil {
		t.Fatal(err)
	}
	if os.Getenv("PLAYWRIGHT_BROWSERS_PATH") == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			t.Fatal(err)
		}
		t.Setenv("PLAYWRIGHT_BROWSERS_PATH", filepath.Join(home, "Library", "Caches", "ms-playwright"))
	}
	t.Setenv("WS_PLAYWRIGHT_MODULE", module)
	t.Setenv("WS_CACHE_HOME", t.TempDir())
	t.Setenv("WS_CONFIG_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	t.Setenv("WS_MCP_DASHBOARD_DEV_SOURCE", "")

	dir, project, control := t.TempDir(), t.TempDir(), t.TempDir()
	clipboard := filepath.Join(control, "clipboard")
	clock := filepath.Join(control, "clock")
	if err := os.WriteFile(clock, []byte("0"), 0o600); err != nil {
		t.Fatal(err)
	}
	epoch := time.Now()
	now := func() time.Time {
		raw, err := os.ReadFile(clock)
		if err != nil {
			panic(err)
		}
		seconds, err := strconv.ParseInt(strings.TrimSpace(string(raw)), 10, 64)
		if err != nil {
			panic(err)
		}
		return epoch.Add(time.Duration(seconds) * time.Second)
	}
	src := newPageFake()
	register(t, dir, "livesession", project, epoch)
	src.setPage("livesession", PageContent{HasBoard: true, Seq: 1, Board: boardRegions(t, "Browser acceptance board")})
	writeSnapshot(t, dir, "livesession", &wslive.Snapshot{SchemaVersion: 1, Adapter: "pi", UpdatedAt: epoch.UnixMilli(), HeartbeatMs: 3600000})
	// Do NOT use startPageFixture/startIndexedDaemon: their automatic test cookie
	// wrappers would erase the cookieless boundary this acceptance test verifies.
	d, err := StartDaemon(DaemonOptions{Dir: dir, Home: t.TempDir(), Sessions: src,
		Version: "0.50.2-test", PollInterval: time.Hour, noGitBadge: true, authNow: now,
		clipboard: loginGateBrowserClipboard{path: clipboard}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(d.Close)
	waitFor(t, "login gate browser session indexed", func() bool { _, ok := d.Index().Session("livesession"); return ok })
	mockup, err := filepath.Abs("../../../ai-docs/assets/mockups/261116/localhost-token-gate/mockup.html")
	if err != nil {
		t.Fatal(err)
	}
	shots := os.Getenv("WS_LOGIN_GATE_SHOTS")
	if shots == "" {
		shots = "/tmp/ws-localhost-token-shots"
	}
	manifest, err := json.Marshal(map[string]string{
		"base": endpointURL(d.Endpoint(), ""), "token": d.Endpoint().Token,
		"clipboard": clipboard, "clock": clock,
		"secret": filepath.Join(dir, localhostSecretFile), "mockup": mockup, "shots": shots,
	})
	if err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(control, "fixture.json")
	if err := os.WriteFile(config, manifest, 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "node", "testdata/login-gate.browser.cjs", config)
	// Stream named stages so a stalled browser reports its last boundary.
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	err = cmd.Run()
	if err != nil {
		t.Fatalf("login gate Chromium acceptance: %v", err)
	}
}
