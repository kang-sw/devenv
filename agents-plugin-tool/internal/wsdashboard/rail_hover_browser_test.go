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

func TestAgyOutputRateHoverAndRowBrowser(t *testing.T) {
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
	now := time.Now()
	turn1At := now.Add(-15 * time.Second).UnixMilli()
	turn2Start := now.Add(-4 * time.Second).UnixMilli()
	ctxTokens := int64(4771)
	ctxPct := float64(ctxTokens) * 100 / 1048576

	// Register an agy subagent session in its 2nd turn. Persist output-rate.json
	// separately from snapshot.json (no lead.outputRate in snapshot.json) so the
	// render exercises SnapshotCache's T2 recovery overlay end-to-end.
	if err := WriteRegistration(f.dir, Registration{
		ReplyID:      "agychild",
		Project:      f.project,
		Harness:      "agy",
		RegisteredAt: now.Add(-2 * time.Minute).UTC().Format(time.RFC3339Nano),
		HeartbeatAt:  now.UTC().Format(time.RFC3339),
		Hosted:       true,
	}); err != nil {
		t.Fatal(err)
	}
	if err := writeSessionLabel(SessionDir(f.dir, "agychild"), "agy worker"); err != nil {
		t.Fatal(err)
	}
	if err := wslive.WriteOutputRate(SessionDir(f.dir, "agychild"), &wslive.OutputRate{OutputTps: 70.0, At: turn1At}); err != nil {
		t.Fatal(err)
	}
	writeSnapshot(t, f.dir, "agychild", &wslive.Snapshot{
		SchemaVersion: 1,
		ReplyID:       "agychild",
		Adapter:       "agy",
		UpdatedAt:     now.UnixMilli(),
		HeartbeatMs:   30000,
		Lead: wslive.Lead{
			Activity:       wslive.Activity{Phase: wslive.PhaseReplying},
			TurnStartedAt:  turn2Start,
			PreviousTurnMs: 12000,
		},
		Usage: &wslive.Usage{Context: &wslive.UsageContext{Tokens: &ctxTokens, Window: 1048576, Percent: &ctxPct}},
	})
	// Update parent session snapshot with the running 2nd-turn agy subagent row.
	writeSnapshot(t, f.dir, "livesession", &wslive.Snapshot{
		SchemaVersion: 1,
		ReplyID:       "livesession",
		Adapter:       "claude",
		UpdatedAt:     now.UnixMilli(),
		HeartbeatMs:   30000,
		Lead:          wslive.Lead{Activity: wslive.Activity{Phase: wslive.PhaseThinking}, TurnStartedAt: now.Add(-30 * time.Second).UnixMilli()},
		Agents: []wslive.Agent{{
			Activity:      wslive.Activity{Phase: wslive.PhaseReplying},
			ID:            "pi:agychild",
			ReplyID:       "agychild",
			AgentKind:     wslive.AgentKindPi,
			Harness:       "agy",
			Label:         "agy worker",
			Status:        wslive.StatusRunning,
			StartedAt:     now.Add(-2 * time.Minute).UnixMilli(),
			ActiveMs:      12000,
			TurnStartedAt: turn2Start,
			Model:         "gemini-3.8-flash",
			Effort:        "high",
			ContextTokens: ctxTokens,
			OutputTps:     70.0,
		}},
	})
	f.src.set("agychild", SessionData{HasBoard: false, Fingerprint: now.String(), SnapshotUpdatedAt: now})
	f.src.set("livesession", SessionData{HasBoard: true, Fingerprint: now.String(), SnapshotUpdatedAt: now})
	f.d.Index().Refresh()
	waitFor(t, "agychild indexed", func() bool {
		_, ok := f.d.Index().Session("agychild")
		return ok
	})

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	args := []string{"testdata/agy-output-rate.browser.cjs", f.base}
	if capture := os.Getenv("WS_CAPTURE_PATH"); capture != "" {
		args = append(args, capture)
	}
	cmd := exec.CommandContext(ctx, "node", args...)
	out, err := cmd.CombinedOutput()
	t.Log(string(out))
	if err != nil {
		t.Fatalf("Chromium agy output rate acceptance: %v", err)
	}
}
