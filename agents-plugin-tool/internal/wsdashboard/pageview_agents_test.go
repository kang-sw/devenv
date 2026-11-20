// Agent view tests cover rows, telemetry, and activity.

package wsdashboard

import (
	"fmt"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wslive"
)

func TestRunningAgentHasOneLiveSlot(t *testing.T) {
	now := testNow
	ms := now.UnixMilli()
	fin := func(tool string) wslive.BatchView {
		return wslive.BatchView{ToolNames: []string{tool}, CallCount: 1, EndedAt: ms}
	}
	running := wslive.BatchView{Running: &wslive.RunningView{ToolName: "RunTool", ArgsPreview: "run args", StartedAt: ms}}
	generating := wslive.BatchView{Generating: &wslive.GeneratingView{Kind: wslive.PhaseTool, ToolName: "GenTool", Chars: 12, PhaseStartedAt: ms}}
	thinking := wslive.BatchView{Generating: &wslive.GeneratingView{Kind: wslive.PhaseThinking, PhaseStartedAt: ms}}
	perm := &wslive.Permission{ToolName: "PermTool", StartedAt: ms}
	const ph = "placeholder"
	cases := []struct {
		name    string
		batches []wslive.BatchView
		perm    *wslive.Permission
		want    [1]string // the tool name the row shows, or ph
	}{
		{"no batches", nil, nil, [1]string{ph}},
		{"thinking only", []wslive.BatchView{thinking}, nil, [1]string{ph}},
		{"one finished", []wslive.BatchView{fin("F1")}, nil, [1]string{"F1"}},
		{"three finished", []wslive.BatchView{fin("F1"), fin("F2"), fin("F3")}, nil, [1]string{"F3"}},
		{"finished then thinking", []wslive.BatchView{fin("F1"), fin("F2"), thinking}, nil, [1]string{"F2"}},
		{"running and finished", []wslive.BatchView{fin("F1"), fin("F2"), running}, nil, [1]string{"RunTool"}},
		{"generating and running", []wslive.BatchView{fin("F1"), running, generating}, nil, [1]string{"GenTool"}},
		{"permission, running, generating", []wslive.BatchView{fin("F1"), running, generating}, perm, [1]string{"PermTool"}},
		{"permission, finished", []wslive.BatchView{fin("F1"), fin("F2")}, perm, [1]string{"PermTool"}},
		{"permission alone", nil, perm, [1]string{"PermTool"}},
	}
	for _, c := range cases {
		agent := wslive.Agent{ID: "a1", Status: wslive.StatusRunning, StartedAt: ms,
			Activity: wslive.Activity{Phase: wslive.PhaseTool, Batches: c.batches, Permission: c.perm}}
		v := buildSessionView(pageInputs{Session: liveSession("self", "/p", now), Snapshot: liveSnap(now, wslive.Lead{}, agent), Now: now})
		if got := len(v.Tree.Agents[0].Slots); got != liveSlotCount {
			t.Errorf("%s: Slots = %d, want %d", c.name, got, liveSlotCount)
		}
		_, rows := liveList(t, renderTree(t, v))
		if len(rows) != 1 {
			t.Errorf("%s: live rows = %d, want 1: %v", c.name, len(rows), rows)
			continue
		}
		for i, want := range c.want {
			row := rows[i]
			switch {
			case want == ph:
				if row != `<li class="brow ph"><span class="mk">-</span></li>` {
					t.Errorf("%s: row %d is no placeholder: %s", c.name, i, row)
				}
			case !strings.Contains(row, `title="`+want+`"`):
				t.Errorf("%s: row %d lacks %s: %s", c.name, i, want, row)
			case want == "PermTool" && !strings.HasPrefix(row, `<li class="brow live perm">`),
				want == "RunTool" && !strings.Contains(row, `run args`),
				want == "GenTool" && !strings.Contains(row, `writing arguments`),
				strings.HasPrefix(want, "F") && !strings.HasPrefix(row, `<li class="brow done"><span class="mk">✓</span>`):
				t.Errorf("%s: row %d has the wrong look for %s: %s", c.name, i, want, row)
			}
			if (want == "RunTool" || want == "GenTool") && !strings.Contains(row, `<span class="mk"><span class="spin"></span></span>`) {
				t.Errorf("%s: live tool row %d lacks its spinner: %s", c.name, i, row)
			}
		}
	}
	// A finished or failed agent has no live list.
	for _, status := range []string{wslive.StatusDone, wslive.StatusFailed} {
		agent := wslive.Agent{ID: "a1", Status: status, Activity: wslive.Activity{Batches: []wslive.BatchView{fin("F1")}}}
		v := buildSessionView(pageInputs{Session: liveSession("self", "/p", now), Snapshot: liveSnap(now, wslive.Lead{}, agent), Now: now})
		if v.Tree.Agents[0].Slots != nil {
			t.Errorf("%s agent has slots: %+v", status, v.Tree.Agents[0].Slots)
		}
		if html := renderTree(t, v); strings.Contains(html, "brows live") {
			t.Errorf("%s agent renders a live list: %s", status, html)
		}
	}
}

// TestAgentsPanelSpinsOnlyOnToolRows pins Decision "agent marks are a state
// dot": in the agents panel only live tool rows carry the rotating spinner;
// agent, lead, and peer marks pulse, phase pills and the running-subagent
// label have none. The session index row and the subagent badge keep theirs.
func TestAgentsPanelSpinsOnlyOnToolRows(t *testing.T) {
	now := testNow
	ms := now.UnixMilli()
	run := func(tool string) wslive.BatchView {
		return wslive.BatchView{Running: &wslive.RunningView{ToolName: tool, StartedAt: ms}}
	}
	busy := func(phase string) wslive.Lead {
		return wslive.Lead{Activity: wslive.Activity{Phase: phase, PhaseStartedAt: ms, Batches: []wslive.BatchView{{ToolNames: []string{"Read"}, CallCount: 1, EndedAt: ms}, run("Bash")}}}
	}
	agents := []wslive.Agent{
		{ID: "a", Status: wslive.StatusRunning, Activity: wslive.Activity{Phase: wslive.PhaseTool, Batches: []wslive.BatchView{run("Edit")}}},
		{ID: "b", ParentID: "a", Status: wslive.StatusRunning, Activity: wslive.Activity{Phase: wslive.PhaseThinking}},
	}
	sessions := map[string]Session{"samewt": liveSession("samewt", "/p", now), "otherwt": liveSession("otherwt", "/q", now)}
	snaps := map[string]*wslive.Snapshot{
		"samewt":  liveSnap(now, busy(wslive.PhaseReplying), agents...),
		"otherwt": liveSnap(now, busy(wslive.PhaseCompacting), agents...),
	}
	contacts := []Contact{{Address: "id:samewt", ReplyID: "samewt", LastExchange: now}, {Address: "id:otherwt", ReplyID: "otherwt", LastExchange: now}}
	v := buildSessionView(pageInputs{Session: liveSession("self", "/p", now), Snapshot: liveSnap(now, busy(wslive.PhaseTool), agents...), Now: now,
		Page: PageContent{Contacts: contacts}, Lookup: lookupOf(sessions, snaps)})
	html := renderTree(t, v)
	for _, want := range []string{`class="prun"`, `<span class="phase tool">`, `<span class="phase thinking">`, `<span class="phase replying">`, `<ol class="brows live bguide">`} {
		if !strings.Contains(html, want) {
			t.Fatalf("tree lacks %q: %s", want, html)
		}
	}
	// Lead, two peers (one compact), and every running agent row pulse.
	if n := strings.Count(html, `<span class="pulse"></span>`); n < 6 {
		t.Errorf("pulsing marks = %d, want at least 6: %s", n, html)
	}
	// Remove every live tool row; no spinner may remain.
	rest := html
	spinRows := 0
	for {
		i := strings.Index(rest, `<li class="brow live`)
		if i < 0 {
			break
		}
		j := strings.Index(rest[i:], "</li>")
		if strings.Contains(rest[i:i+j], `class="spin"`) {
			spinRows++
		}
		rest = rest[:i] + rest[i+j+len("</li>"):]
	}
	if spinRows == 0 {
		t.Errorf("no live tool row spins: %s", html)
	}
	if strings.Contains(rest, "spin") {
		t.Errorf("a spinner outside the live tool rows: %s", rest)
	}
	// Outside the agents panel the spinner stays.
	if srow := renderNamed(t, "srow", IndexRow{ReplyID: "s", Live: true, Adapter: true, Phase: wslive.PhaseTool}); !strings.Contains(srow, `<span class="phase tool"><span class="spin"></span>`) {
		t.Errorf("session index row lost its spinner: %s", srow)
	}
	if badge := renderNamed(t, "subbadge", nil); !strings.Contains(badge, `<span class="spin"></span>`) {
		t.Errorf("subagent badge lost its spinner: %s", badge)
	}
	// Reduced motion stops the pulse as it stops the spinner.
	css := mustAsset(t, "dashboard.css")
	if !strings.Contains(css, "@media (prefers-reduced-motion: reduce) { .spin, .caret, .pulse, .pibadge { animation: none; } }") {
		t.Errorf("reduced-motion rule does not name .pulse and .pibadge")
	}
}

// TestPiAgentRowBlinksAPiBadgeAndLinks pins a Pi subagent's row: while it
// runs, a blinking "pi" badge stands where another running row pulses; a
// finished one shows its outcome as any row does; and its reply-id links to
// the conversation the agent runner hosts, which has a page and no board.
func TestPiAgentRowBlinksAPiBadgeAndLinks(t *testing.T) {
	now := testNow
	snap := liveSnap(now, wslive.Lead{},
		wslive.Agent{ID: "pi:childone", Label: "pi worker", Status: wslive.StatusRunning, AgentKind: wslive.AgentKindPi, ReplyID: "childone"},
		wslive.Agent{ID: "pi:childtwo", Label: "pi done", Status: wslive.StatusDone, AgentKind: wslive.AgentKindPi, ReplyID: "childtwo"},
		wslive.Agent{ID: "native", Label: "native", Status: wslive.StatusRunning},
	)
	runner := liveSession("childone", "/p", now)
	runner.HasBoard, runner.Agent, runner.Unlisted = false, true, true
	v := buildSessionView(pageInputs{Session: liveSession("self", "/p", now), Snapshot: snap, Now: now, Lookup: lookupOf(map[string]Session{"childone": runner}, nil)})
	html := renderTree(t, v)
	// The compact source repeats presentation marks, not full-tree rows.
	html = html[strings.Index(html, `<div class="thead">`):]
	if n := strings.Count(html, `<span class="pibadge"`); n != 1 {
		t.Fatalf("pi badges = %d, want the running pi row's alone: %s", n, html)
	}
	if n := strings.Count(html, `<span class="pulse"></span>`); n != 1 {
		t.Fatalf("pulses = %d, want the native running row's alone: %s", n, html)
	}
	if !strings.Contains(html, `href="/s/childone"`) {
		t.Fatalf("the pi row should link its runner conversation: %s", html)
	}
	if strings.Contains(html, `href="/s/childtwo"`) {
		t.Fatalf("a reply-id this daemon does not serve must not link: %s", html)
	}
	css := mustAsset(t, "dashboard.css")
	if !strings.Contains(css, ".pibadge {") || !strings.Contains(css, "animation: dash-pulse") {
		t.Fatalf("the pi badge should blink as the pulse does")
	}
}

// TestAgentRowRendersHarnessBadgeWhenPresent pins that a running agent.spawn
// child with Harness renders the 2-letter harness badge (.hb.sm) in both the
// compact roster and full tree instead of the legacy pibadge, while an old
// record without Harness still falls back to pibadge.
func TestAgentRowRendersHarnessBadgeWhenPresent(t *testing.T) {
	now := testNow
	snap := liveSnap(now, wslive.Lead{},
		wslive.Agent{ID: "pi:agy1", Label: "agy worker", Status: wslive.StatusRunning, AgentKind: wslive.AgentKindPi, Harness: "agy", ReplyID: "agy1"},
		wslive.Agent{ID: "pi:claude1", Label: "claude worker", Status: wslive.StatusRunning, AgentKind: wslive.AgentKindPi, Harness: "claude", ReplyID: "claude1"},
		wslive.Agent{ID: "pi:claude2", Label: "claude done", Status: wslive.StatusDone, AgentKind: wslive.AgentKindPi, Harness: "claude", ReplyID: "claude2"},
		wslive.Agent{ID: "pi:legacy1", Label: "legacy pi", Status: wslive.StatusRunning, AgentKind: wslive.AgentKindPi, ReplyID: "legacy1"},
	)
	v := buildSessionView(pageInputs{Session: liveSession("self", "/p", now), Snapshot: snap, Now: now})
	fullHTML := renderTree(t, v)
	split := strings.Index(fullHTML, `<div class="thead">`)
	compactHTML, treeHTML := fullHTML[:split], fullHTML[split:]

	for label, html := range map[string]string{"compact": compactHTML, "tree": treeHTML} {
		if n := strings.Count(html, `<span class="hb sm" data-h="agy"></span>`); n != 1 {
			t.Fatalf("%s: agy hb badges = %d, want 1: %s", label, n, html)
		}
		if n := strings.Count(html, `<span class="hb sm" data-h="claude"></span>`); n != 1 {
			t.Fatalf("%s: claude hb badges = %d, want 1 (running only): %s", label, n, html)
		}
		if n := strings.Count(html, `<span class="pibadge"`); n != 1 {
			t.Fatalf("%s: legacy pibadge fallback = %d, want 1 (legacy row only): %s", label, n, html)
		}
	}
	if !strings.Contains(treeHTML, `class="chip astatus">sleep</span>`) {
		t.Fatalf("ended claude child should keep sleep presentation chip: %s", treeHTML)
	}
}

func TestSpawnTreeUsesOrdinaryNestingAndCountsWithoutDuplicateRows(t *testing.T) {
	now := testNow
	st := wslive.NewState("lead-key")
	st.SetAdapter("claude")
	for _, ev := range []wslive.Event{
		{Kind: wslive.KindAgent, AgentID: "pi:child", AgentKind: wslive.AgentKindPi, SessionKey: "child-key", Label: "worker", Status: wslive.StatusRunning},
		{Kind: wslive.KindAgent, AgentID: "pi:leaf", AgentKind: wslive.AgentKindPi, ParentID: "pi:child", Label: "leaf", Status: wslive.StatusDone},
		{Kind: wslive.KindAgent, AgentID: "harness-child", SessionKey: "child-key", Label: "duplicate", Status: wslive.StatusRunning},
	} {
		if _, err := st.Apply(ev, now.UnixMilli()); err != nil {
			t.Fatal(err)
		}
	}
	snap := st.Snapshot(now.UnixMilli())
	v := buildSessionView(pageInputs{Session: liveSession("self", "/p", now), Snapshot: &snap, Now: now})
	if len(v.Tree.Agents) != 1 || v.Tree.Agents[0].ID != "pi:child" || len(v.Tree.Agents[0].Children) != 1 || v.Tree.Agents[0].Children[0].ID != "pi:leaf" {
		t.Fatalf("spawn tree = %+v", v.Tree.Agents)
	}
	if v.Tree.Running != 1 || v.Tree.Finished != 1 || v.Tree.Failed != 0 {
		t.Fatalf("counts = %d/%d/%d", v.Tree.Running, v.Tree.Finished, v.Tree.Failed)
	}
	html := renderTree(t, v)
	if !strings.Contains(html, "worker") || !strings.Contains(html, "leaf") || strings.Contains(html, "duplicate") {
		t.Fatalf("ordinary agents pane lost/duplicated a row: %s", html)
	}
}

// TestAgentRowStats pins a subagent row's telemetry: model and effort,
// compact context size with the exact count in its title, and the rounded
// output rate; a value the adapter did not send leaves its stat out (no
// placeholder), a row with none has no stats box, and an ended agent keeps
// its model, effort, and context but shows no rate. The leading items sit in
// an aline wrapper, and the CSS moves the stats to a second line under the
// name through a container query on the agents panel.
func TestAgentRowStats(t *testing.T) {
	full := wslive.Agent{ID: "a", Status: wslive.StatusRunning, Model: "claude-opus-5-5", Effort: "high", ContextTokens: 48_211, OutputTps: 61.6}
	html := renderNamed(t, "agent", AgentNode{Agent: full})
	// effort is its own stat after the model, outside the model's ellipsis
	for _, want := range []string{`<span class="astat"><span class="amodel" title="model: claude-opus-5-5">claude-opus-5-5</span><span class="aeff" title="effort: high">high</span><span class="actx"`,
		`<span class="actx" title="context: 48211 tokens">48.2k</span>`, `<span class="atps" title="output: 62 tokens/s">62 t/s</span>`} {
		if !strings.Contains(html, want) {
			t.Errorf("full row lacks %s: %s", want, html)
		}
	}

	partial := wslive.Agent{ID: "b", Status: wslive.StatusRunning, Effort: "low"}
	html = renderNamed(t, "agent", AgentNode{Agent: partial})
	if !strings.Contains(html, `<span class="astat"><span class="aeff" title="effort: low">low</span></span>`) || strings.Contains(html, "amodel") {
		t.Errorf("effort-only row = %s", html)
	}
	noEffort := wslive.Agent{ID: "d", Status: wslive.StatusRunning, Model: "claude-sonnet-4-5", ContextTokens: 41_000}
	html = renderNamed(t, "agent", AgentNode{Agent: noEffort})
	if !strings.Contains(html, `<span class="amodel" title="model: claude-sonnet-4-5">claude-sonnet-4-5</span><span class="actx"`) || strings.Contains(html, "aeff") {
		t.Errorf("a row without a recorded effort renders one: %s", html)
	}
	if html = renderNamed(t, "agent", AgentNode{Agent: wslive.Agent{ID: "c", Status: wslive.StatusRunning}}); strings.Contains(html, "astat") {
		t.Errorf("a row without telemetry has a stats box: %s", html)
	}

	ended := full
	ended.Status, ended.EndedAt = wslive.StatusDone, testNow.UnixMilli()
	html = renderNamed(t, "agent", AgentNode{Agent: ended})
	if !strings.Contains(html, `class="amodel"`) || !strings.Contains(html, `class="aeff"`) || !strings.Contains(html, `class="actx"`) || strings.Contains(html, "atps") {
		t.Errorf("ended row = %s", html)
	}

	if !strings.Contains(html, `<summary class="ahead"><span class="aline">`) || !strings.Contains(html, `</span><span class="astat">`) {
		t.Errorf("the stats are not a sibling after the aline wrapper: %s", html)
	}

	css := mustAsset(t, "dashboard.css")
	for _, want := range []string{".r-tree { container: atree / inline-size; }", ".aline { display: contents; }", "@container atree (max-width: 439px) {",
		".ahead:has(> .aline) { flex-wrap: wrap;", ".ahead > .astat { order: 1; flex: 0 0 100%;", ".ahead > .astat .aeff, .ahead > .astat .actx, .ahead > .astat .atps { flex: none; }"} {
		if !strings.Contains(css, want) {
			t.Errorf("dashboard.css lacks %q", want)
		}
	}
	if strings.Contains(css, ".ahead .astat { display: none; }") {
		t.Errorf("dashboard.css still hides the stats on a narrow panel")
	}
	if js := mustAsset(t, "dashboard.js"); strings.Contains(js, "agentTip") {
		t.Errorf("dashboard.js still carries the stats hover card")
	}
}

// TestAgentActiveTime keeps elapsed working time separate from the time since
// completion, including a held Pi row that is marked running but is idle.
func TestAgentActiveTime(t *testing.T) {
	start := testNow.UnixMilli()
	running := AgentNode{Agent: wslive.Agent{ID: "pi:1", Status: wslive.StatusRunning, ActiveMs: 12_000, TurnStartedAt: start, EndedAt: start - 60_000}}
	html := renderNamed(t, "agent", running)
	want := fmt.Sprintf(`data-ms="%d" data-f="since"`, start-12_000)
	if !strings.Contains(html, want) || strings.Contains(html, `data-f="ago"`) {
		t.Errorf("running active time = %s; want %s and no ago", html, want)
	}

	running.TurnStartedAt = 0 // held but no turn is working
	html = renderNamed(t, "agent", running)
	if !strings.Contains(html, `data-span="12000">12s`) || strings.Contains(html, `data-f="since"`) {
		t.Errorf("held idle active time = %s", html)
	}

	ended := AgentNode{Agent: wslive.Agent{ID: "done", Status: wslive.StatusDone, ActiveMs: 72_000, EndedAt: start, ContextTokens: 1000}}
	html = renderNamed(t, "agent", ended)
	stats := strings.Index(html, `<span class="astat">`)
	ctx := strings.Index(html, `class="actx"`)
	ago := strings.Index(html, `data-f="ago"`)
	am := strings.Index(html, `<span class="am">`)
	if stats < 0 || ctx < stats || ago < ctx || am < ago || !strings.Contains(html, `data-span="72000">1m 12s`) || strings.Contains(html, `data-f="since"`) {
		t.Errorf("finished active time and stats = %s", html)
	}

	ended.ActiveMs = 0 // older snapshots have no accumulated timing
	html = renderNamed(t, "agent", ended)
	if strings.Contains(html, `data-span="0"`) {
		t.Errorf("finished row with unknown active time has a duration: %s", html)
	}

	ended.ContextTokens = 0
	html = renderNamed(t, "agent", ended)
	if !strings.Contains(html, `<span class="astat"><span><time class="t"`) || !strings.Contains(html, `data-f="ago"`) {
		t.Errorf("ago-only stats missing: %s", html)
	}
}
