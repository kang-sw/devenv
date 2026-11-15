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

// TestRailKidBusyBrowser drives the parent row's working count in Chromium over
// disposable fixture sessions, never an installed/shared daemon: five
// children under livesession, three at work (thinking; idle with a
// subagent; replying with a subagent), then live state changes through the
// rail stream. WS_RAIL_KID_BUSY_SHOTS, when set to an absolute directory,
// also takes dark and light screenshots there.
func TestRailKidBusyBrowser(t *testing.T) {
	if os.Getenv("WS_PLAYWRIGHT_MODULE") == "" {
		t.Skip("set WS_PLAYWRIGHT_MODULE to run Chromium acceptance")
	}
	shots := os.Getenv("WS_RAIL_KID_BUSY_SHOTS")
	if shots != "" && !filepath.IsAbs(shots) {
		t.Fatal("WS_RAIL_KID_BUSY_SHOTS must be absolute")
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
	kids := []string{"kidone", "kidtwo", "kidthree", "kidfour", "kidfive"}
	state := func(id, s string) {
		now := time.Now()
		snap := &wslive.Snapshot{SchemaVersion: 1, Adapter: "pi", UpdatedAt: now.UnixMilli(), HeartbeatMs: 30000,
			Lead: wslive.Lead{Activity: wslive.Activity{Phase: wslive.PhaseIdle}}}
		switch s {
		case "think":
			snap.Lead.Phase = wslive.PhaseThinking
		case "agents":
			snap.Agents = []wslive.Agent{{ID: "sub-" + id, Status: wslive.StatusRunning, StartedAt: now.UnixMilli()}}
		case "both":
			snap.Lead.Phase = wslive.PhaseReplying
			snap.Agents = []wslive.Agent{{ID: "sub-" + id, Status: wslive.StatusRunning, StartedAt: now.UnixMilli()}}
		}
		writeSnapshot(t, f.dir, id, snap)
		// The fake source does not fingerprint snapshots itself; model the
		// production source's changed fingerprint to wake the rail stream.
		f.src.set(id, SessionData{HasBoard: true, Fingerprint: now.String(), SnapshotUpdatedAt: now})
	}
	now := time.Now()
	for i, id := range kids {
		register(t, f.dir, id, f.project, now)
		state(id, []string{"think", "agents", "both", "idle", "idle"}[i])
	}
	f.d.Index().Refresh()
	waitFor(t, "the children to be indexed", func() bool {
		for _, id := range kids {
			if _, ok := f.d.Index().Session(id); !ok {
				return false
			}
		}
		return true
	})
	layout := func(body string) {
		t.Helper()
		if code, _, raw := postJSON(t, f.base+railLayoutRoute, f.base, body); code != http.StatusOK {
			t.Fatalf("POST %s = %d %s", body, code, raw)
		}
	}
	layout(`{"op":"import","order":["livesession","boardsession","kidone","kidtwo","kidthree","kidfour","kidfive"]}`)
	for _, id := range kids {
		layout(`{"op":"attach","rid":"` + id + `","parent":"livesession"}`)
	}
	updates := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		for _, id := range kids {
			if s := q.Get(id); s != "" {
				state(id, s)
			}
		}
		f.d.Index().Refresh()
		json.NewEncoder(w).Encode(map[string]bool{"ok": true})
	}))
	defer updates.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "node", "testdata/rail-kid-busy.browser.cjs", f.base, updates.URL, shots)
	out, err := cmd.CombinedOutput()
	t.Log(string(out))
	if err != nil {
		t.Fatalf("Chromium rail working-children acceptance: %v", err)
	}
}
