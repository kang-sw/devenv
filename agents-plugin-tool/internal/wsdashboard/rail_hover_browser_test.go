package wsdashboard

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wslive"
)

// Uses disposable fixture sessions, never an installed/shared daemon.
// With the isolated HOME, PLAYWRIGHT_BROWSERS_PATH must name cached browsers.
func TestRailHoverUpdatesBrowser(t *testing.T) {
	if os.Getenv("WS_PLAYWRIGHT_MODULE") == "" {
		t.Skip("set WS_PLAYWRIGHT_MODULE to run Chromium acceptance")
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
	updates := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		phase := wslive.PhaseThinking
		if r.URL.Query().Get("idle") == "1" {
			phase = wslive.PhaseIdle
		}
		now := time.Now()
		writeSnapshot(t, f.dir, "livesession", &wslive.Snapshot{
			SchemaVersion: 1, Adapter: "pi", UpdatedAt: now.UnixMilli(), HeartbeatMs: 30000,
			Lead: wslive.Lead{Activity: wslive.Activity{Phase: phase}},
		})
		// The fake source does not fingerprint snapshots itself; model the
		// production source's changed fingerprint to wake the rail stream.
		f.src.set("livesession", SessionData{HasBoard: true, Fingerprint: now.String(), SnapshotUpdatedAt: now})
		f.d.Index().Refresh()
		json.NewEncoder(w).Encode(map[string]int64{"at": now.UnixMilli()})
	}))
	defer updates.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "node", "testdata/rail-hover.browser.cjs", f.base, updates.URL)
	out, err := cmd.CombinedOutput()
	t.Log(string(out))
	if err != nil {
		t.Fatalf("Chromium rail hover acceptance: %v", err)
	}
}
