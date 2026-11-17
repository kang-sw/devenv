package wsdashboard

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wslive"
)

// The session's agents live in a popover over the conversation, opened by the
// running-agent badge on the composer border, in place of a panel above it.
// The script checks open and close (badge, Esc, outside click, the close
// button), the pin and its reload persistence, the group order (lead,
// running, ended, background, peers), a row's recent output and its full
// output action, the narrow fit, the board tab, the muted zero-running badge
// (including a non-live page retained for a draft), and that every row the
// agents region renders is inside the popover. WS_AGPOP_SHOTS=<dir> saves
// screenshots. Needs WS_PLAYWRIGHT_MODULE.
func TestAgentBadgePopoverBrowser(t *testing.T) {
	if os.Getenv("WS_PLAYWRIGHT_MODULE") == "" {
		t.Skip("set WS_PLAYWRIGHT_MODULE for disposable Chromium acceptance")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed")
	}
	f := startPageFixture(t)
	now := time.Now().UnixMilli()
	seq := int64(0)
	batches := func(n int, tools ...string) []wslive.BatchView {
		var out []wslive.BatchView
		for i := 0; i < n; i++ {
			seq++
			out = append(out, wslive.BatchView{Seq: seq, ToolNames: []string{tools[i%len(tools)]}, CallCount: 1 + i%3, StartedAt: now - int64(n-i)*20000, EndedAt: now - int64(n-i)*20000 + 4000})
		}
		return out
	}
	bg := 2
	agent := func(id, label, status, model string, ctx int64, b []wslive.BatchView, done, failed int, startedAgo int64) wslive.Agent {
		a := wslive.Agent{ID: id, Label: label, Status: status, Model: model, ContextTokens: ctx, StartedAt: now - startedAgo}
		a.Batches, a.Done, a.Failed = b, done, failed
		if status == wslive.StatusRunning {
			a.Phase, a.TurnStartedAt = wslive.PhaseThinking, now-startedAgo
		} else {
			a.EndedAt, a.ActiveMs = now-60000, startedAgo
		}
		return a
	}
	child := agent("a7", "probe-child", wslive.StatusRunning, "haiku-4.5", 9000, batches(1, "Read"), 1, 0, 30000)
	child.ParentID = "a1"
	snap := wslive.Snapshot{SchemaVersion: 1, Adapter: "pi", UpdatedAt: now, HeartbeatMs: 30000,
		Usage: &wslive.Usage{Background: &bg},
		Lead:  wslive.Lead{Activity: wslive.Activity{Phase: wslive.PhaseThinking, PhaseStartedAt: now - 5000, Batches: batches(3, "Read", "Grep")}},
		Agents: []wslive.Agent{
			agent("a1", "explore-ui", wslive.StatusRunning, "sonnet-5.5", 41000, batches(7, "Read", "Grep", "Bash"), 0, 0, 134000),
			agent("a2", "worker-css", wslive.StatusRunning, "sonnet-5.5", 67000, batches(3, "Edit"), 3, 0, 302000),
			agent("a3", "review-a", wslive.StatusDone, "opus-4.7", 88000, batches(5, "Read"), 5, 0, 580000),
			agent("a4", "review-b", wslive.StatusDone, "sonnet-5.5", 52000, batches(2, "Read"), 2, 0, 371000),
			agent("a5", "probe-go", wslive.StatusDone, "haiku-4.5", 12000, batches(1, "Bash"), 1, 0, 48000),
			agent("a6", "worker-test", wslive.StatusFailed, "sonnet-5.5", 30000, batches(2, "Bash"), 0, 2, 200000),
			child,
		}}
	// Keep the ordinary done class for old page CSS, but show Pi revival
	// labels in the row and hover. Native review-a must still say done.
	for i, id := range map[int]string{3: "sleepchild", 4: "deletedchild"} {
		snap.Agents[i].AgentKind, snap.Agents[i].ReplyID = wslive.AgentKindPi, id
		dir := SessionDir(f.dir, id)
		if err := os.MkdirAll(filepath.Join(dir, "pi"), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, AgentRecordFileName), []byte(`{"backend":"pi","closed_at":"2026-11-18"}`), 0o600); err != nil {
			t.Fatal(err)
		}
		if id == "sleepchild" {
			if err := os.WriteFile(filepath.Join(dir, "pi", "session.jsonl"), []byte("session\n"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
	}
	f.d.snapshots.SetLive(f.dir, "livesession", snap)
	f.d.broker.wake(liveDep("livesession"))
	// The zero-running fixture keeps a working lead and one ended agent;
	// removing its adapter then exercises the non-live draft-retained page.
	zero := wslive.Snapshot{SchemaVersion: 1, Adapter: "pi", HeartbeatMs: 30000,
		Lead:   wslive.Lead{Activity: wslive.Activity{Phase: wslive.PhaseThinking}},
		Agents: []wslive.Agent{agent("ended", "finished-worker", wslive.StatusDone, "sonnet-5.5", 12000, nil, 1, 0, 60000)}}
	// Each request swaps the agents region with a new caption or fixture state.
	updates := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next := snap
		switch r.URL.Query().Get("state") {
		case "zero":
			next = zero
		case "nonlive":
			next = zero
			next.Adapter = ""
		default:
			next.Agents[0].Caption = r.URL.Query().Get("caption")
		}
		next.UpdatedAt = time.Now().UnixMilli()
		f.d.snapshots.SetLive(f.dir, "livesession", next)
		f.d.broker.wake(liveDep("livesession"))
		w.WriteHeader(http.StatusNoContent)
	}))
	defer updates.Close()
	// Every row the agents region renders, by its stable key: the server's
	// own markup is the list the popover must keep whole.
	_, body := get(t, f.base+"/s/livesession?frame=1")
	rows := map[string]any{"peers": len(regexp.MustCompile(`class="peer `).FindAllString(body, -1))}
	var keys []string
	for _, m := range regexp.MustCompile(`data-key="(agent-[^"]+)"`).FindAllStringSubmatch(body, -1) {
		keys = append(keys, m[1])
	}
	rows["keys"] = keys
	if len(keys) != 8 || rows["peers"] == 0 {
		t.Fatalf("fixture rendered %v, want the lead, seven agents and a peer", rows)
	}
	want, _ := json.Marshal(rows)
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, node, "testdata/agent-popover.browser.cjs")
	cmd.Env = append(os.Environ(), "AGPOP_BASE="+f.base, "AGPOP_UPDATES="+updates.URL, "AGPOP_ROWS="+string(want), "AGPOP_SHOTS="+os.Getenv("WS_AGPOP_SHOTS"))
	cmd.WaitDelay = 5 * time.Second
	out, err := cmd.CombinedOutput()
	t.Logf("%s", out)
	if err != nil {
		t.Fatalf("agent badge popover: %v", err)
	}
}
