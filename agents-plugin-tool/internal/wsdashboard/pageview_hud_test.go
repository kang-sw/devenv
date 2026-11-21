// Session HUD tests cover handoff and agent projection.

package wsdashboard

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wslive"
)

// TestHandedOffSessionMark: the page of a lead session the daemon handed to
// a terminal carries the handed mark on <html> (the page script then reads
// the resume command), only while no terminal runs the session and the
// daemon does not host it again.
func TestHandedOffSessionMark(t *testing.T) {
	now := testNow
	ended := liveSession("handed", "/a", now)
	ended.EndedAt = stampFor(now)
	ended.Live = false
	resumed := liveSession("resumed", "/a", now)
	retaken := ended
	retaken.ReplyID, retaken.Hosted = "retaken", true
	mark := func(s Session, handedOff bool) bool {
		t.Helper()
		v := buildSessionView(pageInputs{Session: s, Now: now, HandedOff: handedOff, Lookup: lookupOf(nil, nil)})
		regions, err := renderSessionRegions(v, nil)
		if err != nil {
			t.Fatal(err)
		}
		html, err := renderSessionPage(v, regions, true, "")
		if err != nil {
			t.Fatal(err)
		}
		if got := strings.Contains(html, ` data-handed="1"`); got != v.HandedOff {
			t.Errorf("%s: page mark = %v, view = %v", s.ReplyID, got, v.HandedOff)
		}
		return v.HandedOff
	}
	if !mark(ended, true) {
		t.Error("a handed-off session no terminal runs lacks the mark")
	}
	if mark(ended, false) || mark(resumed, true) || mark(retaken, true) {
		t.Error("the mark shows without a handed-off record, on a session a terminal runs, or on a hosted one")
	}
}

func TestHUDProjectionStatePrecedenceVisibilityAndHarness(t *testing.T) {
	now := testNow
	leadSess := liveSession("self", "/p", now)
	leadSess.Harness = "claude"
	snap := liveSnap(now, wslive.Lead{},
		// Permission overrides failed/running.
		wslive.Agent{ID: "perm", Label: "perm-node", Status: wslive.StatusFailed, Harness: "agy",
			Activity: wslive.Activity{Permission: &wslive.Permission{ToolName: "bash", ArgsPreview: "rm -rf"}}},
		// Failed overrides holding/alive.
		wslive.Agent{ID: "fail", Status: wslive.StatusFailed, Holding: true, Alive: true, Harness: "codex"},
		// PhaseReplying and PhaseTool both map to tool.
		wslive.Agent{ID: "reply", Status: wslive.StatusRunning, Activity: wslive.Activity{Phase: wslive.PhaseReplying}},
		// Thinking inherits lead harness ("claude") when empty.
		wslive.Agent{ID: "think", Status: wslive.StatusRunning, Activity: wslive.Activity{Phase: wslive.PhaseThinking}},
		// Holding inherits parent harness from "fail" ("codex") instead of lead.
		wslive.Agent{ID: "hold", ParentID: "fail", Status: wslive.StatusDone, Holding: true, Alive: true},
		// Alive-idle stays visible as idle.
		wslive.Agent{ID: "alive", Status: wslive.StatusDone, Alive: true, AgentKind: wslive.AgentKindPi},
		// Asleep parent kept visible solely for connectivity to running child.
		wslive.Agent{ID: "asleep-anc", Status: wslive.StatusDone},
		wslive.Agent{ID: "grandchild", ParentID: "asleep-anc", Status: wslive.StatusRunning, Activity: wslive.Activity{Phase: wslive.PhaseTool}},
		// Asleep leaf is folded into Completed.
		wslive.Agent{ID: "done-leaf", Status: wslive.StatusDone},
	)
	v := buildSessionView(pageInputs{Session: leadSess, Snapshot: snap, Now: now, Lookup: lookupOf(nil, nil)})
	if v.Tree.HUD.Completed != 1 {
		t.Fatalf("HUD.Completed = %d, want 1", v.Tree.HUD.Completed)
	}
	byID := map[string]HUDNode{}
	var collect func([]HUDNode)
	collect = func(nodes []HUDNode) {
		for _, n := range nodes {
			byID[n.ID] = n
			collect(n.Children)
		}
	}
	collect(v.Tree.HUD.Roots)
	if _, ok := byID["done-leaf"]; ok {
		t.Fatal("asleep leaf should be excluded from visible HUD roots")
	}
	want := map[string][2]string{
		"perm":       {"permission", "agy"},
		"fail":       {"failed", "codex"},
		"reply":      {"tool", "claude"},
		"think":      {"thinking", "claude"},
		"hold":       {"holding", "codex"},
		"alive":      {"idle", "claude"},
		"asleep-anc": {"idle", "claude"},
		"grandchild": {"tool", "claude"},
	}
	for id, exp := range want {
		n, ok := byID[id]
		if !ok {
			t.Fatalf("missing visible HUD node %q", id)
		}
		if n.State != exp[0] || n.Harness != exp[1] {
			t.Fatalf("node %q = (state=%q, harness=%q), want (%q, %q)", id, n.State, n.Harness, exp[0], exp[1])
		}
	}
	for _, id := range []string{"perm", "fail", "reply", "think", "hold", "grandchild"} {
		if !byID[id].CloseActive {
			t.Errorf("node %q should require confirmation", id)
		}
	}
	for _, id := range []string{"alive", "asleep-anc"} {
		if byID[id].CloseActive {
			t.Errorf("node %q should close immediately", id)
		}
	}
	if byID["perm"].Task != "bash rm -rf" {
		t.Fatalf("perm Task = %q, want %q", byID["perm"].Task, "bash rm -rf")
	}
	// Pi fallback when neither parent nor lead specifies a harness.
	piLead := liveSession("self2", "/p", now)
	piLead.Harness = ""
	piView := buildSessionView(pageInputs{
		Session:  piLead,
		Snapshot: liveSnap(now, wslive.Lead{}, wslive.Agent{ID: "pi:1", AgentKind: wslive.AgentKindPi, Status: wslive.StatusRunning}),
		Now:      now,
	})
	if len(piView.Tree.HUD.Roots) != 1 || piView.Tree.HUD.Roots[0].Harness != "pi" {
		t.Fatalf("Pi fallback harness = %+v", piView.Tree.HUD.Roots)
	}
}

func TestHUDProjectionDepthCapOverflowAndLinkedDedup(t *testing.T) {
	now := testNow
	sessions := map[string]Session{
		"self": liveSession("self", "/p", now),
		"r1":   liveSession("r1", "/p", now),
		"r5":   liveSession("r5", "/p", now),
	}
	// Own snapshot has r1 (depth 1) -> n2 (depth 2) -> n3 (depth 3) -> n4 (depth 4).
	// Linked r1 snapshot duplicates n2 and also carries r5 (depth 3 under n2) -> n4b -> n5a (depth 5, running) + n5done (depth 5, done).
	ownSnap := liveSnap(now, wslive.Lead{},
		wslive.Agent{ID: "pi:r1", ReplyID: "r1", AgentKind: wslive.AgentKindPi, Status: wslive.StatusDone, Holding: true, Alive: true},
		wslive.Agent{ID: "pi:n2", ParentID: "pi:r1", AgentKind: wslive.AgentKindPi, Status: wslive.StatusDone, Holding: true, Alive: true},
	)
	r1Snap := liveSnap(now, wslive.Lead{},
		wslive.Agent{ID: "pi:n2", AgentKind: wslive.AgentKindPi, Status: wslive.StatusDone, Holding: true, Alive: true},
		wslive.Agent{ID: "n3", Status: wslive.StatusRunning},
		wslive.Agent{ID: "n4", ParentID: "n3", Status: wslive.StatusRunning},
		wslive.Agent{ID: "n5-vis", ParentID: "n4", Status: wslive.StatusRunning},
		wslive.Agent{ID: "n6-vis", ParentID: "n5-vis", Status: wslive.StatusDone, Alive: true},
		wslive.Agent{ID: "n5-done", ParentID: "n4", Status: wslive.StatusDone},
	)
	snaps := map[string]*wslive.Snapshot{"r1": r1Snap}
	v := buildSessionView(pageInputs{
		Session:  sessions["self"],
		Snapshot: ownSnap,
		Now:      now,
		Lookup:   lookupOf(sessions, snaps),
	})
	if len(v.Tree.HUD.Roots) != 1 || len(v.Tree.HUD.Roots[0].Children) != 2 {
		t.Fatalf("unexpected linked/dedup roots: %+v", v.Tree.HUD.Roots)
	}
	// Depth hierarchy: r1 (1) -> n3 (2) -> n4 (3) -> n5-vis (4, Overflow=1 for n6-vis).
	var n5 HUDNode
	for _, c := range v.Tree.HUD.Roots[0].Children {
		if c.ID == "n3" {
			n5 = c.Children[0].Children[0]
		}
	}
	if n5.ID != "n5-vis" || n5.Overflow != 1 || len(n5.Children) != 0 {
		t.Fatalf("depth-4 node n5-vis = %+v, want Overflow=1 and no Children", n5)
	}
	if v.Tree.HUD.Completed != 1 {
		t.Fatalf("HUD.Completed = %d, want 1 (n5-done only, no double count of pi:n2)", v.Tree.HUD.Completed)
	}
	// Verify SubagentCount and CompactAgents stay untouched by HUD holding/alive rules.
	if v.Tree.Subagents != (SubagentCount{}) || len(v.Tree.CompactAgents) != 0 {
		t.Fatalf("legacy Subagents/CompactAgents drifted: %+v / %+v", v.Tree.Subagents, v.Tree.CompactAgents)
	}
	// Verify rendered .ag-hud-data JSON inside .ag-source.
	set, err := renderSessionRegions(v, newEntryCache())
	if err != nil {
		t.Fatal(err)
	}
	treeHTML := string(set[regionTree].HTML)
	if !strings.Contains(treeHTML, `<script type="application/json" class="ag-hud-data">`) {
		t.Fatalf("tree HTML missing ag-hud-data carrier:\n%s", treeHTML)
	}
	start := strings.Index(treeHTML, `<script type="application/json" class="ag-hud-data">`) + len(`<script type="application/json" class="ag-hud-data">`)
	end := strings.Index(treeHTML[start:], `</script>`)
	var decoded HUDView
	if err := json.Unmarshal([]byte(treeHTML[start:start+end]), &decoded); err != nil {
		t.Fatalf("unmarshal ag-hud-data: %v", err)
	}
	if len(decoded.Roots) != 1 || decoded.Completed != 1 {
		t.Fatalf("decoded HUD = %+v", decoded)
	}
}
