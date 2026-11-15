package wsdashboard

import (
	"context"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wslive"
)

// TestRailRowMetaBrowser drives a rail row's badges and meta line in
// Chromium over disposable fixture sessions, never an installed/shared
// daemon: the own-subagent count and the question dot on the harness
// icon, which keep the icon's and the name's x; a parent's toggle and
// working count opening its second line; child rows staying one line; no
// robot badge or policy capsule in a row; no pulse under reduced motion.
// WS_RAIL_ROW_META_SHOTS, when set to an absolute directory, also takes
// dark and light screenshots there (expanded and collapsed).
func TestRailRowMetaBrowser(t *testing.T) {
	if os.Getenv("WS_PLAYWRIGHT_MODULE") == "" {
		t.Skip("set WS_PLAYWRIGHT_MODULE to run Chromium acceptance")
	}
	shots := os.Getenv("WS_RAIL_ROW_META_SHOTS")
	if shots != "" && !filepath.IsAbs(shots) {
		t.Fatal("WS_RAIL_ROW_META_SHOTS must be absolute")
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
	type spec struct {
		id, harness, label, msg string
		user, working           bool
		agents, questions       int
	}
	const wb = "Whiteboard: http://127.0.0.1:47213/wb/devenv/rail-meta#topic-3"
	specs := []spec{
		{id: "metaparent", harness: "claude", label: "메타 라인 목업 정리", msg: wb, working: true, agents: 1, questions: 2},
		{id: "kidone", harness: "claude", label: "worker: rail drag reorder", working: true, agents: 2, questions: 1},
		{id: "kidtwo", harness: "codex", label: "review: whiteboard relay", working: true},
		{id: "kidthree", harness: "pi", label: "pi: lockup svg pass", working: true},
		{id: "kidfour", harness: "claude", label: "ticket draft: center scroll"},
		{id: "kidfive", harness: "codex", label: "audit-doc fresh read"},
		{id: "idleparent", harness: "claude", label: "release gate sweep", msg: "Merged develop into main; waiting for the canary ledger."},
		{id: "idleone", harness: "claude", label: "idle child one"},
		{id: "idletwo", harness: "codex", label: "idle child two"},
		{id: "idlethree", harness: "pi", label: "idle child three"},
		{id: "idlefour", harness: "claude", label: "idle child four"},
		{id: "idlefive", harness: "codex", label: "idle child five"},
		{id: "agentsrow", harness: "codex", label: "dashboard i18n pass", msg: "Updating ko strings for the rail fold and the usage popover.", working: true, agents: 2},
		{id: "plainrow", harness: "claude", label: "mailbox peer chat", msg: "ping me when the whiteboard topic settles.", user: true},
		{id: "dotrow", harness: "claude", label: "ship notes draft", msg: "Release notes for the next patch: rail drag reorder, working-children count.", working: true, agents: 12, questions: 1},
	}
	now := time.Now()
	for _, s := range specs {
		reg := Registration{ReplyID: s.id, Project: f.project, Harness: s.harness, RegisteredAt: stampFor(now), HeartbeatAt: stampFor(now)}
		if err := WriteRegistration(f.dir, reg); err != nil {
			t.Fatal(err)
		}
		dir := SessionDir(f.dir, s.id)
		if err := writeSessionLabel(dir, s.label); err != nil {
			t.Fatal(err)
		}
		snap := &wslive.Snapshot{SchemaVersion: 1, Adapter: s.harness, UpdatedAt: now.UnixMilli(), HeartbeatMs: 30000,
			Lead: wslive.Lead{Activity: wslive.Activity{Phase: wslive.PhaseIdle}}}
		if s.working {
			snap.Lead.Phase = wslive.PhaseReplying
		}
		for i := 0; i < s.agents; i++ {
			snap.Agents = append(snap.Agents, wslive.Agent{ID: s.id + "-sub-" + strconv.Itoa(i), Status: wslive.StatusRunning, StartedAt: now.UnixMilli()})
		}
		writeSnapshot(t, f.dir, s.id, snap)
		if s.msg != "" {
			kind := wslive.RingReply
			if s.user {
				kind = wslive.RingUser
			}
			if err := wslive.AppendRing(wslive.RingDir(dir), []wslive.RingEntry{{Kind: kind, Text: s.msg}}); err != nil {
				t.Fatal(err)
			}
		}
		f.src.set(s.id, SessionData{HasBoard: true, CountPolicy: s.questions, Fingerprint: now.String(), SnapshotUpdatedAt: now})
	}
	f.d.Index().Refresh()
	waitFor(t, "the rows to be indexed", func() bool {
		for _, s := range specs {
			if _, ok := f.d.Index().Session(s.id); !ok {
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
	layout(`{"op":"import","order":["metaparent","kidone","kidtwo","kidthree","kidfour","kidfive","idleparent","idleone","idletwo","idlethree","idlefour","idlefive","agentsrow","plainrow","dotrow","livesession","boardsession"]}`)
	for _, k := range []string{"kidone", "kidtwo", "kidthree", "kidfour", "kidfive"} {
		layout(`{"op":"attach","rid":"` + k + `","parent":"metaparent"}`)
	}
	for _, k := range []string{"idleone", "idletwo", "idlethree", "idlefour", "idlefive"} {
		layout(`{"op":"attach","rid":"` + k + `","parent":"idleparent"}`)
	}
	layout(`{"op":"star","rid":"metaparent"}`)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "node", "testdata/rail-row-meta.browser.cjs", f.base, shots)
	out, err := cmd.CombinedOutput()
	t.Log(string(out))
	if err != nil {
		t.Fatalf("Chromium rail row meta acceptance: %v", err)
	}
}
