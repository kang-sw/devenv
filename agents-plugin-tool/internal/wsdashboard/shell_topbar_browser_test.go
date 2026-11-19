package wsdashboard

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// The shell's header strip and rail on real pages, in Chromium: the 34px
// handles hang from the top edge of the stage column beside the
// full-height docked rail, the overlay rail covers them, and the strip
// steps down center-first down to the desktop shell's 760px minimum.
// WS_SHELL_TOPBAR_SHOTS (absolute) keeps the screenshots, with the approved
// mockup's for a side-by-side look.
func TestShellTopbarBrowser(t *testing.T) {
	if os.Getenv("WS_PLAYWRIGHT_MODULE") == "" {
		t.Skip("set WS_PLAYWRIGHT_MODULE to use cached Chromium")
	}
	shots := os.Getenv("WS_SHELL_TOPBAR_SHOTS")
	if shots != "" && !filepath.IsAbs(shots) {
		t.Fatal("WS_SHELL_TOPBAR_SHOTS must be absolute")
	}
	mockup, err := filepath.Abs(filepath.Join("..", "..", "..", "ai-docs", "assets", "mockups", "261119", "shell-redesign", "shell-redesign-v4.html"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(mockup); err != nil {
		mockup = ""
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CODEX_HOME", filepath.Join(home, ".codex"))
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(home, ".claude"))
	t.Setenv("WS_CACHE_HOME", t.TempDir())
	t.Setenv("WS_CONFIG_HOME", t.TempDir())
	SetDevAssets(nil)
	f := startPageFixture(t, func(o *DaemonOptions) {
		o.Port = 0
		o.LAN = false
		o.bindHost = "127.0.0.1"
	})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	out, err := exec.CommandContext(ctx, "node", "testdata/shell-topbar.browser.cjs", f.base, shots, mockup).CombinedOutput()
	t.Log(string(out))
	if err != nil {
		t.Fatalf("Chromium shell header strip: %v", err)
	}
}
