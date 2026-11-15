package wsdashboard

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// TestDesktopUpdateBrowser pins the desktop shell's update UI: a browser tab
// (no window.werkschaftDesktop) shows no settings row and no toast; a page
// with a stub of the shell's preload bridge shows the row, its phase line,
// and the bottom-right available / ready / up-to-date toasts, each calling
// back into the bridge. Uses a disposable fixture daemon, never the shared
// one; SHOTS, when set, names a directory for review screenshots.
func TestDesktopUpdateBrowser(t *testing.T) {
	if os.Getenv("WS_PLAYWRIGHT_MODULE") == "" {
		t.Skip("set WS_PLAYWRIGHT_MODULE to run Chromium acceptance")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CODEX_HOME", filepath.Join(home, ".codex"))
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(home, ".claude"))
	SetDevAssets(nil)
	f := startPageFixture(t, func(o *DaemonOptions) {
		o.Port = 0
		o.LAN = false
		o.bindHost = "127.0.0.1"
	})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	out, err := exec.CommandContext(ctx, "node", "testdata/desktop-update.browser.cjs", f.base).CombinedOutput()
	t.Log(string(out))
	if err != nil {
		t.Fatalf("Chromium desktop update acceptance: %v", err)
	}
}
