package wsdashboard

import (
	"context"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wslive"
)

func TestDashboardDraftRecoveryBrowser(t *testing.T) {
	if os.Getenv("WS_PLAYWRIGHT_MODULE") == "" {
		t.Skip("set WS_PLAYWRIGHT_MODULE for Chromium acceptance")
	}
	t.Setenv("HOME", t.TempDir())
	t.Setenv("WS_CACHE_HOME", t.TempDir())
	t.Setenv("WS_CONFIG_HOME", t.TempDir())
	for _, key := range []string{"CLAUDE_CONFIG_DIR", "CODEX_HOME", "PI_CODING_AGENT_DIR"} {
		t.Setenv(key, "")
		if err := os.Unsetenv(key); err != nil {
			t.Fatal(err)
		}
	}
	f := startPageFixture(t)
	f.withCommands(t, wslive.SlashCommand{Name: "compact", Source: "builtin"})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "node", "testdata/draft-recovery.browser.cjs", f.base)
	out, err := cmd.CombinedOutput()
	t.Log(string(out))
	if err != nil {
		t.Fatalf("dashboard draft recovery in Chromium: %v", err)
	}
	if os.Getenv("WS_DRAFT_CAPTURE_BEFORE") == "" {
		held := f.held(t)
		if len(held) != 2 {
			t.Fatalf("the two accepted but unacknowledged POSTs should each be held once, got %d: %+v", len(held), held)
		}
	}
}
