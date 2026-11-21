package wsdashboard_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/mcp"
	"github.com/kang-sw/devenv/internal/wsconfig"
	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wsmailbox"
)

func TestSettingsInteractionBrowser(t *testing.T) {
	if os.Getenv("WS_PLAYWRIGHT_MODULE") == "" {
		t.Skip("set WS_PLAYWRIGHT_MODULE to run the settings Chromium acceptance")
	}
	t.Setenv("WS_RSRC_DEV_CHECKOUT", "")
	backend, err := mcp.DashboardConfigBackend("ws", wsconfig.Options{}, mcp.NewDashboardSessionSource(), nil)
	if err != nil {
		t.Fatal(err)
	}
	wsdashboard.RunSettingsVisualBrowser(t, backend)
}

func TestSettingsProductionBrowser(t *testing.T) {
	if os.Getenv("WS_PLAYWRIGHT_MODULE") == "" {
		t.Skip("set WS_PLAYWRIGHT_MODULE to run Chromium against the production settings backend")
	}
	cache, home := t.TempDir(), t.TempDir()
	t.Setenv("WS_CACHE_HOME", cache)
	t.Setenv("WS_CONFIG_HOME", home)
	t.Setenv("WS_RSRC_DEV_CHECKOUT", "")
	t.Setenv("WS_MCP_DASHBOARD_DEV_SOURCE", "")
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "-C", root, "init", "-q", "-b", "main").CombinedOutput(); err != nil {
		t.Fatalf("initialize session worktree: %v\n%s", err, out)
	}
	global := filepath.Join(home, "config.json")
	if err := os.WriteFile(global, []byte(`{"overrides":{"workflow.lang":"English"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	source := mcp.NewDashboardSessionSource()
	backend, err := mcp.DashboardConfigBackend("ws", wsconfig.Options{CacheHome: cache, ConfigHome: home}, source, nil)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(cache, "dashboard")
	keys := mcp.NewAgentKeyStore()
	var replyIDs []string
	for range 2 {
		key, err := keys.MintLead(root)
		if err != nil {
			t.Fatal(err)
		}
		id := wsmailbox.ReplyID(key)
		now := time.Now().UTC().Format(time.RFC3339)
		if err := wsdashboard.WriteRegistration(dir, wsdashboard.Registration{
			ReplyID: id, Project: root, Harness: "codex", GitCommonDir: filepath.Join(root, ".git"),
			RegisteredAt: now, HeartbeatAt: now,
		}); err != nil {
			t.Fatal(err)
		}
		replyIDs = append(replyIDs, id)
	}
	d, err := wsdashboard.StartDaemon(wsdashboard.DaemonOptions{
		Dir: dir, Home: t.TempDir(), Sessions: source, Worktrees: source, Config: backend,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	deadline := time.Now().Add(5 * time.Second)
	for !d.Index().Loaded() && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if len(d.Index().Sessions()) != 2 {
		t.Fatal("production dashboard index did not admit both live leads")
	}
	login, err := d.LoginLink("/settings")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "node", "testdata/settings-production.browser.cjs", login, replyIDs[0], replyIDs[1])
	out, err := cmd.CombinedOutput()
	t.Log(string(out))
	if err != nil {
		t.Fatalf("production settings Chromium acceptance: %v", err)
	}
}
