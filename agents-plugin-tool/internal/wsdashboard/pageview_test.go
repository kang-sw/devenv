package wsdashboard

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wslive"
)

// liveSnap is a fresh snapshot with an adapter at now.
func liveSnap(now time.Time, lead wslive.Lead, agents ...wslive.Agent) *wslive.Snapshot {
	return &wslive.Snapshot{SchemaVersion: 1, Adapter: "pi", UpdatedAt: now.UnixMilli(), HeartbeatMs: 30000, Lead: lead, Agents: agents}
}

func liveSession(id, project string, now time.Time) Session {
	reg := Registration{ReplyID: id, Project: project, Harness: "pi", RegisteredAt: stampFor(now), HeartbeatAt: stampFor(now)}
	return Session{Registration: reg, Live: true, HasBoard: true}
}

func endedSession(id, project string, now time.Time) Session {
	s := liveSession(id, project, now.Add(-time.Hour))
	s.EndedAt = stampFor(now.Add(-time.Hour))
	s.Live = false
	return s
}

func TestHarnessProcessHead(t *testing.T) {
	now := time.Now()
	prev := processAlive
	processAlive = func(pid int) bool { return pid == 123 }
	t.Cleanup(func() { processAlive = prev })
	for _, tc := range []struct {
		name string
		reg  Registration
		want string
	}{
		{"host running", Registration{Hosted: true, HostPID: 123}, `class="host sse" title="harness process running"`},
		{"host exited", Registration{Hosted: true, HostPID: 456}, `class="host sse stopped" title="harness process stopped"`},
		{"host stopped", Registration{Hosted: true, HostStopped: true}, `class="host sse stopped" title="harness process stopped"`},
		{"host starting", Registration{Hosted: true}, `class="host sse stopped" title="harness process stopped"`},
		{"terminal live", Registration{HeartbeatAt: stampFor(now)}, `class="host sse" title="harness process running"`},
		{"terminal stale", Registration{HeartbeatAt: stampFor(now.Add(-time.Hour))}, `class="host sse stopped" title="harness process stopped"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.reg.Harness = "claude"
			view := buildSessionView(pageInputs{Session: Session{Registration: tc.reg}, Now: now})
			regions, err := renderSessionRegions(view, nil)
			if err != nil {
				t.Fatal(err)
			}
			if head := string(regions[regionHead].HTML); !strings.Contains(head, tc.want) {
				t.Errorf("head %q lacks %q", head, tc.want)
			}
		})
	}
}

// lookupOf serves sessions and snapshots from maps.
func lookupOf(sessions map[string]Session, snaps map[string]*wslive.Snapshot) sessionLookup {
	return sessionLookup{
		session: func(id string) (Session, bool) {
			s, ok := sessions[id]
			return s, ok
		},
		snapshot: func(id string) *wslive.Snapshot { return snaps[id] },
	}
}

func TestAdapterPresenceChoosesTheMode(t *testing.T) {
	now := testNow
	base := pageInputs{Session: liveSession("self", "/p", now), Now: now}
	cases := map[string]struct {
		snap *wslive.Snapshot
		want string
	}{
		"no snapshot":     {nil, modeBoard},
		"no adapter":      {&wslive.Snapshot{UpdatedAt: now.UnixMilli()}, modeBoard},
		"stale adapter":   {&wslive.Snapshot{Adapter: "pi", UpdatedAt: now.Add(-2 * time.Minute).UnixMilli(), HeartbeatMs: 30000}, modeBoard},
		"fresh adapter":   {liveSnap(now, wslive.Lead{}), modeLive},
		"default beat ok": {&wslive.Snapshot{Adapter: "claude", UpdatedAt: now.Add(-60 * time.Second).UnixMilli()}, modeLive},
	}
	for name, c := range cases {
		in := base
		in.Snapshot = c.snap
		if got := buildSessionView(in).Mode(); got != c.want {
			t.Errorf("%s: mode = %s, want %s", name, got, c.want)
		}
	}
}

// TestLookupReadsNoSnapshotForPresenceOnly: a view that uses a looked-up
// session only for whether it is served (a mail line's peer link) or has a
// board (a linked agent row) reads no snapshot of it.
func TestLookupReadsNoSnapshotForPresenceOnly(t *testing.T) {
	now := testNow
	snap := liveSnap(now, wslive.Lead{}, wslive.Agent{ID: "a1", Label: "worker", Status: wslive.StatusDone, ReplyID: "linked"})
	sessions := map[string]Session{
		"linked":   liveSession("linked", "/p", now),
		"mailpeer": liveSession("mailpeer", "/p", now),
	}
	var read []string
	lookup := lookupOf(sessions, nil)
	lookup.snapshot = func(id string) *wslive.Snapshot {
		read = append(read, id)
		return nil
	}
	ring := []wslive.RingRecord{{Name: "m1", At: now.Add(-time.Minute), RingEntry: wslive.RingEntry{Kind: wslive.RingMail, Direction: wslive.MailRecv, Peer: "id:mailpeer", Title: "hi"}}}
	v := buildSessionView(pageInputs{Session: liveSession("self", "/p", now), Snapshot: snap, Ring: ring, Now: now, Lookup: lookup})
	if len(v.Tree.Agents) != 1 || !v.Tree.Agents[0].Linked {
		t.Fatalf("agents = %+v, want a1 linked", v.Tree.Agents)
	}
	if len(v.Conversation.Entries) != 1 || !v.Conversation.Entries[0].PeerLinked {
		t.Fatalf("entries = %+v, want the mail line linked", v.Conversation.Entries)
	}
	if len(read) != 0 {
		t.Fatalf("a presence-only lookup read snapshots %v", read)
	}
}

func TestAgentTreeNestsAndLinksServedDelegates(t *testing.T) {
	now := testNow
	running := wslive.BatchView{Seq: 2, ToolNames: []string{"Bash"}, Running: &wslive.RunningView{ToolName: "Bash", ArgsPreview: "go test"}}
	done := wslive.BatchView{Seq: 1, ToolNames: []string{"Read"}, CallCount: 2, EndedAt: now.UnixMilli()}
	snap := liveSnap(now, wslive.Lead{},
		wslive.Agent{ID: "a1", Label: "worker", Status: wslive.StatusRunning, ReplyID: "delegateone", Activity: wslive.Activity{Batches: []wslive.BatchView{done, running}}},
		wslive.Agent{ID: "a2", ParentID: "a1", Label: "child", Status: wslive.StatusDone},
		wslive.Agent{ID: "a3", ParentID: "gone", Label: "orphan", Status: wslive.StatusFailed, ReplyID: "noboard"},
	)
	sessions := map[string]Session{"delegateone": liveSession("delegateone", "/p", now)}
	noBoard := liveSession("noboard", "/p", now)
	noBoard.HasBoard = false
	sessions["noboard"] = noBoard
	v := buildSessionView(pageInputs{Session: liveSession("self", "/p", now), Snapshot: snap, Now: now, Lookup: lookupOf(sessions, nil)})
	if len(v.Tree.Agents) != 2 || v.Tree.Agents[0].ID != "a1" || v.Tree.Agents[1].ID != "a3" {
		t.Fatalf("roots = %+v", v.Tree.Agents)
	}
	a1 := v.Tree.Agents[0]
	if !a1.Linked || a1.Running == nil || a1.Running.ToolName != "Bash" || len(a1.Finished) != 1 {
		t.Errorf("a1 = %+v", a1)
	}
	if len(a1.Children) != 1 || a1.Children[0].ID != "a2" {
		t.Errorf("a1 children = %+v", a1.Children)
	}
	if v.Tree.Agents[1].Linked {
		t.Errorf("a delegate whose session has no board must not link")
	}
	if v.Tree.Running != 1 || v.Tree.Finished != 1 || v.Tree.Failed != 1 {
		t.Errorf("counts = %d/%d/%d", v.Tree.Running, v.Tree.Finished, v.Tree.Failed)
	}
}

func TestSessionPageAgentPopoverGroupsAndTasks(t *testing.T) {
	running := AgentNode{Agent: wslive.Agent{ID: "running", Status: wslive.StatusRunning},
		Children: []AgentNode{{Agent: wslive.Agent{ID: "done-child", Status: wslive.StatusDone}}}}
	ended := AgentNode{Agent: wslive.Agent{ID: "ended", Status: wslive.StatusDone},
		Children: []AgentNode{{Agent: wslive.Agent{ID: "running-child", Status: wslive.StatusRunning}}}}
	failed := AgentNode{Agent: wslive.Agent{ID: "failed", Status: wslive.StatusFailed}}
	other := AgentNode{Agent: wslive.Agent{ID: "other-running", Status: wslive.StatusRunning}}
	tree := TreeView{Agents: []AgentNode{ended, running, failed, other}}
	// Only top-level status chooses the group; preserve tree order and
	// children even when a child's status differs from its parent's.
	if got, want := tree.RunningAgents(), []AgentNode{running, other}; !reflect.DeepEqual(got, want) {
		t.Errorf("RunningAgents = %+v, want %+v", got, want)
	}
	if got, want := tree.EndedAgents(), []AgentNode{ended, failed}; !reflect.DeepEqual(got, want) {
		t.Errorf("EndedAgents = %+v, want %+v", got, want)
	}
	if empty := (TreeView{}); len(empty.RunningAgents()) != 0 || len(empty.EndedAgents()) != 0 {
		t.Error("an empty tree has popover agents")
	}
	// wslive's supported statuses, crossed so every parent's group is
	// independent of each supported child status (and keeps the child).
	statuses := []string{wslive.StatusRunning, wslive.StatusDone, wslive.StatusFailed}
	for _, parentStatus := range statuses {
		for _, childStatus := range statuses {
			t.Run(parentStatus+"/"+childStatus, func(t *testing.T) {
				node := AgentNode{Agent: wslive.Agent{ID: "parent", Status: parentStatus},
					Children: []AgentNode{{Agent: wslive.Agent{ID: "child", Status: childStatus}}}}
				tree := TreeView{Agents: []AgentNode{node}}
				wantRunning := parentStatus == wslive.StatusRunning
				if node.IsRunning() != wantRunning {
					t.Errorf("IsRunning = %t, want %t", node.IsRunning(), wantRunning)
				}
				selected, excluded := tree.RunningAgents(), tree.EndedAgents()
				if !wantRunning {
					selected, excluded = excluded, selected
				}
				if !reflect.DeepEqual(selected, []AgentNode{node}) || len(excluded) != 0 {
					t.Errorf("status groups = %+v / %+v, want intact parent in one group only", selected, excluded)
				}
			})
		}
	}
	for _, tc := range []struct {
		name                 string
		background, monitors int
		want                 int
	}{
		{"none", 0, 0, 0},
		{"background", 2, 0, 2},
		{"monitors", 0, 3, 3},
		{"both", 2, 3, 5},
	} {
		t.Run(tc.name, func(t *testing.T) {
			view := SessionView{Background: tc.background, Monitors: tc.monitors}
			if got := view.Tasks(); got != tc.want {
				t.Errorf("Tasks = %d, want %d", got, tc.want)
			}
		})
	}
}

// TestSubagentCounts pins the composer badge's N(+M): Direct counts the
// running agents with no known parent (an evicted parent's running child
// included), Deep the running agents below them plus the running agents of
// a running delegate that holds its own served session, recursively, each
// session counted once.
func TestSubagentCounts(t *testing.T) {
	now := testNow
	run := func(id, parent, reply string) wslive.Agent {
		return wslive.Agent{ID: id, ParentID: parent, ReplyID: reply, Status: wslive.StatusRunning}
	}
	done := func(id, parent string) wslive.Agent {
		return wslive.Agent{ID: id, ParentID: parent, Status: wslive.StatusDone}
	}
	stale := *liveSnap(now, wslive.Lead{}, run("s1", "", ""))
	stale.UpdatedAt = now.Add(-10 * time.Minute).UnixMilli()
	sessions := map[string]Session{}
	for _, id := range []string{"self", "dlg", "dlg2", "loop", "stale", "nosnap"} {
		sessions[id] = liveSession(id, "/p", now)
	}
	snaps := map[string]*wslive.Snapshot{
		// dlg runs two agents, one nested, and a delegate of its own (dlg2).
		"dlg":  liveSnap(now, wslive.Lead{}, run("d1", "", "dlg2"), run("d2", "d1", ""), done("d3", "")),
		"dlg2": liveSnap(now, wslive.Lead{}, run("e1", "", "")),
		// loop's agent points back at the viewed session and at loop itself.
		"loop":  liveSnap(now, wslive.Lead{}, run("l1", "", "self"), run("l2", "", "loop")),
		"stale": &stale,
	}
	lookup := lookupOf(sessions, snaps)
	cases := map[string]struct {
		agents []wslive.Agent
		want   SubagentCount
	}{
		"none":                {nil, SubagentCount{}},
		"direct children":     {[]wslive.Agent{run("a", "", ""), run("b", "", "")}, SubagentCount{Direct: 2}},
		"grandchildren":       {[]wslive.Agent{run("a", "", ""), run("b", "a", ""), run("c", "b", "")}, SubagentCount{Direct: 1, Deep: 2}},
		"under finished":      {[]wslive.Agent{done("a", ""), run("b", "a", "")}, SubagentCount{Deep: 1}},
		"evicted parent":      {[]wslive.Agent{run("b", "gone", ""), run("c", "b", "")}, SubagentCount{Direct: 1, Deep: 1}},
		"finished ignored":    {[]wslive.Agent{done("a", ""), done("b", "a"), {ID: "c", Status: wslive.StatusFailed}}, SubagentCount{}},
		"linked delegate":     {[]wslive.Agent{run("a", "", "dlg")}, SubagentCount{Direct: 1, Deep: 3}},
		"finished delegate":   {[]wslive.Agent{{ID: "a", ReplyID: "dlg", Status: wslive.StatusDone}}, SubagentCount{}},
		"delegate twice":      {[]wslive.Agent{run("a", "", "dlg"), run("b", "", "dlg")}, SubagentCount{Direct: 2, Deep: 3}},
		"lookup cycle":        {[]wslive.Agent{run("a", "", "loop")}, SubagentCount{Direct: 1, Deep: 2}},
		"stale delegate":      {[]wslive.Agent{run("a", "", "stale")}, SubagentCount{Direct: 1}},
		"unserved delegate":   {[]wslive.Agent{run("a", "", "unknown"), run("b", "", "nosnap")}, SubagentCount{Direct: 2}},
		"self parent ignored": {[]wslive.Agent{run("a", "a", "")}, SubagentCount{Direct: 1}},
	}
	for name, c := range cases {
		got, rows := subagentProjection("self", c.agents, lookup, now)
		if got != c.want {
			t.Errorf("%s: counts = %+v, want %+v", name, got, c.want)
		}
		if len(rows) != got.Direct+got.Deep {
			t.Errorf("%s: %d compact rows for %+v", name, len(rows), got)
		}
		seen := map[string]bool{}
		for _, row := range rows {
			if row.Status != wslive.StatusRunning || seen[row.ID] {
				t.Errorf("%s: unexpected or duplicate row %q", name, row.ID)
			}
			seen[row.ID] = true
		}
	}
}

// TestConvLiveCarriesSubagentCounts: the convlive region carries the counts
// for the composer's subagent badge beside the lead's phase.
func TestConvLiveCarriesSubagentCounts(t *testing.T) {
	snap := liveSnap(testNow, wslive.Lead{},
		wslive.Agent{ID: "a", Status: wslive.StatusRunning},
		wslive.Agent{ID: "b", ParentID: "a", Status: wslive.StatusRunning},
		wslive.Agent{ID: "c", ParentID: "a", Status: wslive.StatusRunning},
		wslive.Agent{ID: "d", Status: wslive.StatusDone})
	v := buildSessionView(pageInputs{Session: liveSession("self", "/p", testNow), Snapshot: snap, Now: testNow})
	if v.Conversation.Subagents != (SubagentCount{Direct: 1, Deep: 2}) {
		t.Fatalf("Subagents = %+v", v.Conversation.Subagents)
	}
	set, err := renderSessionRegions(v, newEntryCache())
	if err != nil {
		t.Fatal(err)
	}
	if html := string(set[regionTree].HTML); !strings.Contains(html, `class="ag-source" hidden data-sub="1" data-subdeep="2"`) || strings.Count(html, `class="compact-agent"`) != 3 {
		t.Errorf("tree lacks matching counts and compact rows:\n%s", html)
	}
	if html := string(set[regionConvLive].HTML); !strings.Contains(html, `<div class="conv clive" data-phase="" data-sub="1" data-subdeep="2" data-bg="0" data-monitors="0">`) {
		t.Errorf("convlive lacks the subagent counts:\n%s", html)
	}
}

// TestConvLiveCarriesTurnStamp: while a turn runs, the convlive region
// carries its start stamp as data-turn, the boundary the interrupt badge
// resets on when the next turn starts with no idle phase between; an idle
// lead carries none.
func TestConvLiveCarriesTurnStamp(t *testing.T) {
	render := func(lead wslive.Lead) string {
		v := buildSessionView(pageInputs{Session: liveSession("self", "/p", testNow), Snapshot: liveSnap(testNow, lead), Now: testNow})
		set, err := renderSessionRegions(v, newEntryCache())
		if err != nil {
			t.Fatal(err)
		}
		return string(set[regionConvLive].HTML)
	}
	started := testNow.Add(-2 * time.Second).UnixMilli()
	working := wslive.Lead{Activity: wslive.Activity{Phase: wslive.PhaseThinking, PhaseStartedAt: started}, TurnStartedAt: started}
	if html, want := render(working), fmt.Sprintf(`data-subdeep="0" data-turn="%d"`, started); !strings.Contains(html, want) {
		t.Errorf("a working lead's convlive lacks %s:\n%s", want, html)
	}
	if html := render(wslive.Lead{}); strings.Contains(html, "data-turn") {
		t.Errorf("an idle lead's convlive carries a turn stamp:\n%s", html)
	}
}
