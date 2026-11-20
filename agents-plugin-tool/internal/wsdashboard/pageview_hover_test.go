package wsdashboard

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wslive"
)

func TestSessionHoverExistingEvidence(t *testing.T) {
	now := testNow
	reg := Registration{ReplyID: "lead1", Harness: "pi", RegisteredAt: stampFor(now.Add(-time.Hour))}
	pct, cache, tokens := 62.0, 94.0, int64(123400)
	snap := &wslive.Snapshot{Adapter: "pi", HeartbeatMs: 30000, UpdatedAt: now.UnixMilli(), Lead: wslive.Lead{
		Activity: wslive.Activity{Phase: wslive.PhaseTool}, TurnStartedAt: now.Add(-time.Minute).UnixMilli(), PreviousTurnMs: 23000,
	}, Usage: &wslive.Usage{Context: &wslive.UsageContext{Tokens: &tokens, Window: 200000, Percent: &pct, CacheHit: &cache},
		Cost:   &wslive.UsageCost{USD: 1.84, Estimate: true, Incomplete: true},
		Limits: []wslive.UsageLimit{{Kind: wslive.LimitFiveHour, Percent: 50}}},
		Agents: []wslive.Agent{{ID: "child", Model: "must-not-inherit", Effort: "high", OutputTps: 38, Status: wslive.StatusRunning}},
	}
	row := railRow(railInput{Session: Session{Registration: reg, Label: `<unsafe>`}, Snapshot: snap}, true, now)
	if row.Hover.Stale || row.Hover.TurnStartedAt != snap.Lead.TurnStartedAt || row.Hover.LastTurnMs != 23000 || row.Hover.RegisteredAt != now.Add(-time.Hour).UnixMilli() {
		t.Fatalf("working hover = %+v", row.Hover)
	}
	html := renderNamed(t, "session-hover", row)
	contains(t, "working hover", html, "&lt;unsafe&gt;", "Observed output model", "Unknown", "123,400 / 200K", "CH 94%", "~$1.84&#43;", "Current turn", "Last snapshot", `data-f="since"`)
	for _, forbidden := range []string{"must-not-inherit", "high", "38 t/s", "five_hour", "sb-lim", "5-hour", "weekly", "onclick", "<button"} {
		if strings.Contains(html, forbidden) {
			t.Errorf("hover invents evidence or controls %q: %s", forbidden, html)
		}
	}

	snap.Lead.Phase, snap.Lead.TurnStartedAt = wslive.PhaseIdle, 0
	row = railRow(railInput{Session: Session{Registration: reg}, Snapshot: snap}, true, now)
	html = renderNamed(t, "session-hover", row)
	contains(t, "idle hover", html, "Last turn", "Last-turn output rate", `data-span="23000"`, `class="na">—`)
	if strings.Contains(html, `data-f="since"`) {
		t.Error("idle hover ticks the last turn")
	}

	for _, tc := range []struct {
		name string
		reg  Registration
		snap *wslive.Snapshot
		live bool
	}{
		{"absent", reg, nil, true},
		{"stale", reg, &wslive.Snapshot{Adapter: "pi", HeartbeatMs: 30000, UpdatedAt: now.Add(-time.Hour).UnixMilli(), Lead: snap.Lead, Usage: snap.Usage}, true},
		{"ended", reg, snap, false},
		{"stopped", Registration{ReplyID: "lead1", Hosted: true, HostStopped: true}, snap, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := railRow(railInput{Session: Session{Registration: tc.reg}, Snapshot: tc.snap}, tc.live, now)
			if !r.Hover.Stale || r.Hover.TurnStartedAt != 0 || r.Hover.Phase != "" {
				t.Fatalf("stale/absent hover claims live activity: %+v", r.Hover)
			}
			out := renderNamed(t, "session-hover", r)
			contains(t, "stale source", out, `data-stale="1"`, "Unknown")
		})
	}
}

func TestSessionHoverObservedIdentityAndLastTurnRate(t *testing.T) {
	now := testNow
	at := now.Add(-time.Second).UnixMilli()
	rate := 41.6
	observed := &wslive.ObservedModel{Current: wslive.ModelObservation{ID: "answer", Model: `<physical>`, Source: "pi.assistant", At: at, Effort: "low", EffortSource: "message", OutputTps: &rate}, History: []wslive.ModelChange{{Model: "first-model", At: at - 2000}, {Previous: "first-model", Model: "previous-model", At: at - 1000}, {Previous: "previous-model", Model: `<physical>`, At: at}}}
	snap := &wslive.Snapshot{Adapter: "pi", UpdatedAt: now.UnixMilli(), Lead: wslive.Lead{Activity: wslive.Activity{Phase: wslive.PhaseIdle}, Observed: observed, LastTurnOutput: &observed.Current}}
	reg := Registration{ReplyID: "lead1", Harness: "pi"}
	row := railRow(railInput{Session: Session{Registration: reg}, Snapshot: snap}, true, now)
	html := renderNamed(t, "session-hover", row)
	contains(t, "observed hover", html, "&lt;physical&gt;", "Last observed", "pi.assistant", `data-ms="`+fmt.Sprint(at)+`"`, "previous-model → &lt;physical&gt;", `class="tps">42`, "Last-turn output rate", "message")
	if strings.Contains(html, "first-model") || strings.Contains(html, "Unknown") {
		t.Fatalf("popup showed full history or unknown: %s", html)
	}
	history := renderNamed(t, "r-entries", SessionView{ModelHistory: []ModelHistoryRow{{Observed: observed}}})
	contains(t, "detail history", history, "Observed model history", "first-model", "previous-model", "&lt;physical&gt;")
	snap.Lead.Phase, snap.Lead.TurnStartedAt = wslive.PhaseThinking, at+1
	row = railRow(railInput{Session: Session{Registration: reg}, Snapshot: snap}, true, now)
	if row.Hover.Rate != 0 {
		t.Fatal("new working turn borrowed last-turn rate")
	}
	reg.HostStopped = true
	row = railRow(railInput{Session: Session{Registration: reg}, Snapshot: snap}, true, now)
	if row.Hover.Rate != 0 || row.Hover.Observed.Current.At != at {
		t.Fatalf("stopped rate/evidence: %+v", row.Hover)
	}
	contains(t, "stopped provenance", renderNamed(t, "session-hover", row), "Last observed", "&lt;physical&gt;")
}

func TestAgentModelHoverOwnEvidenceOverridesReportedModel(t *testing.T) {
	observed := &wslive.ObservedModel{Current: wslive.ModelObservation{ID: "answer", Model: "child-output", Source: "pi.assistant", At: 1000}}
	child := AgentNode{Agent: wslive.Agent{ID: "child", Status: wslive.StatusRunning, Model: "state-only", Effort: "high", Observed: observed}}
	if child.DisplayModel() != "child-output" {
		t.Fatal("row still shows requested/state identity")
	}
	html := renderNamed(t, "agent", child)
	contains(t, "observed agent row", html, `class="amodel" title="Observed output model">child-output`, `class="aeff" title="effort: high"`, "Last observed", "Adapter-reported model · state-only")
	if strings.Contains(html, "Unknown") || strings.Contains(html, "latest output identity unverified") {
		t.Fatalf("own response left unverified: %s", html)
	}
}

func TestObservedHistoryIsReadableWithoutLiveActivity(t *testing.T) {
	o := &wslive.ObservedModel{Current: wslive.ModelObservation{ID: "answer", Model: "physical", Source: "pi.assistant", At: 1000}, History: []wslive.ModelChange{{Model: "physical", Source: "pi.assistant", At: 1000}}}
	v := buildSessionView(pageInputs{Now: testNow, Session: Session{Registration: Registration{ReplyID: "saved", Harness: "pi"}}, Snapshot: &wslive.Snapshot{Lead: wslive.Lead{Observed: o}, ObservedAgents: map[string]*wslive.ObservedModel{"retired-child": o}}})
	if !v.SavedConversation || v.Adapter || len(v.ModelHistory) != 2 || len(v.Tree.Agents) != 0 || v.Lead.Phase != "" {
		t.Fatalf("history became activity: %+v", v)
	}
	contains(t, "saved model history", renderNamed(t, "r-entries", v), "Observed model history", "retired-child", "physical")
	if html := renderNamed(t, "r-entries", v); strings.Contains(html, `<details class="modelhistory"`) || !strings.Contains(html, `class="chip modelhistory-trigger"`) {
		t.Fatalf("model history is not a title-bar popover source: %s", html)
	}
}

func TestSessionHoverTooltipBoundary(t *testing.T) {
	row := RailRow{ReplyID: "reply123", Label: "label", Harness: "pi", Live: true, Message: "last message", Child: true, LastTool: testNow.UnixMilli()}
	html := renderNamed(t, "rrow", row)
	contains(t, "row tooltips", html, `class="rid lbl">label</span>`, `title="pi · lead waiting"`, `title="last message"`, `data-star="0"`, `class="shp-source"`, `data-tl="last tool call"`)
	if strings.Contains(html, `class="star"`) || strings.Contains(html, `class="hstar"`) {
		t.Error("unstarred row has an inline favorite button or badge")
	}
	row.Starred = true
	favorite := renderNamed(t, "rrow", row)
	contains(t, "favorite badge tooltip", favorite, `data-star="1"`, `class="hstar" role="img" aria-label="Favorite" title="Favorite"`)
	if strings.Contains(html, `class="rid lbl" title=`) {
		t.Error("rail reply-ID tooltip remains")
	}
	css := mustAsset(t, "dashboard.css")
	contains(t, "read-only geometry", css, `.rpop.shp { position: fixed;`, `width: 320px;`, `pointer-events: none;`, `.rpop.shp * { pointer-events: none; }`, `.rpop.shp[hidden] { display: none; }`)
}

func TestAgentModelHoverProvenance(t *testing.T) {
	parent := AgentNode{Agent: wslive.Agent{ID: "parent", Status: wslive.StatusRunning, Model: "parent-model", Effort: "high"},
		Children: []AgentNode{{Agent: wslive.Agent{ID: "child", Status: wslive.StatusRunning}}}}
	html := renderNamed(t, "agent-model-hover", parent)
	contains(t, "reported model", html, "Observed output model", "Unknown", "Adapter-reported model", "parent-model", "Recorded effort: high", "latest output identity unverified")
	child := renderNamed(t, "agent-model-hover", parent.Children[0])
	contains(t, "independent unknown", child, "child", "Unknown")
	for _, absent := range []string{"parent-model", "high", "shp-eff", "Adapter-reported model"} {
		if strings.Contains(child, absent) {
			t.Errorf("child inherited %q: %s", absent, child)
		}
	}
	parent.Model = `<script>alert("x")</script>`
	parent.Status = wslive.StatusDone
	html = renderNamed(t, "agent-model-hover", parent)
	contains(t, "escaped/stopped model", html, "&lt;script&gt;", `data-stale="1"`)
	if strings.Contains(html, "<script>") {
		t.Error("model inserted executable markup")
	}
	// Original stats and effort remain separate; native titles are retained.
	html = renderNamed(t, "agent", parent)
	contains(t, "retained stats", html, `class="amodel" title="model:`, `class="aeff" title="effort: high"`, `class="shp-source"`)
}

func TestSessionHoverScriptLifecycle(t *testing.T) {
	runNode(t, "session hover lifecycle", railJSHarness+mustAsset(t, "rail.js")+`
var R = window.WsRail;
check("normal placement", R.hoverPlace({left:30,top:80},340,320,200,{width:1280,height:800}), {left:348,top:80});
check("viewport clamp", R.hoverPlace({left:900,top:780},980,320,200,{width:1000,height:800}), {left:572,top:592});
var events = {}, wins = {}, mutate, hit, popup, owner;
function src(text) { var s = new El("shp-source"); s.tagName = "TEMPLATE"; s.innerHTML = text; return s; }
function anchor(id, text) {
 var n = row(id); n.appendChild(src(text)); n.isConnected = true;
 n.getClientRects = function () { return [1]; };
 n.getBoundingClientRect = function () { return {left:10,right:330,top:50}; };
 n.closest = function (sel) { return sel === ".rail" ? null : n; }; return n;
}
var doc = {documentElement: {className: ""}, body: {appendChild:function (p) { popup=p; }}, addEventListener:function (k,f) { events[k]=f; },
 elementFromPoint:function () { return hit; }, createElement:function () { var p=new El(); p.style={}; p.offsetWidth=320;p.offsetHeight=200;p.contains=function(n){return n===p;};return p; }};
var win = {innerWidth:1280,innerHeight:800,addEventListener:function(k,f){wins[k]=f;},MutationObserver:function(f){mutate=f;this.observe=function(){};}};
var ctl=R.hover(doc,win), a=anchor("a","own A"), b=anchor("b","own B");
hit=a; events.mouseover({target:a,clientX:40,clientY:60});
check("row A opens own source", [popup.hidden,popup.innerHTML], [false,"own A"]);
a.isConnected=false; hit=b; mutate([{target:{}}]);
check("SSE refinds current pointer instead of detached evidence", popup.innerHTML,"own B");
b.children[b.children.length-1].innerHTML="updated B"; mutate([{target:{}}]);
check("SSE refreshes source",popup.innerHTML,"updated B");
mutate([{target:popup}]); check("own popup mutation ignored",popup.innerHTML,"updated B");
hit=null; mutate([{target:{}}]);check("removed row hides",popup.hidden,true);
hit=a; a.isConnected=true; events.mouseover({target:a,clientX:40,clientY:60}); events.pointerdown({});check("pointer down hides without capturing navigation",popup.hidden,true);
events.mouseover({target:a,clientX:40,clientY:60}); events.keydown({key:"Escape"});check("Escape hides",popup.hidden,true);
events.mouseover({target:a,clientX:40,clientY:60}); events.scroll({});check("scroll hides stale position",popup.hidden,true);
events.mouseover({target:a,clientX:40,clientY:60}); wins.resize();check("resize hides",popup.hidden,true);
doc.activeElement=b; events.focusin({target:b});check("focus opens",popup.innerHTML,"updated B");
doc.activeElement=null;mutate([{target:{}}]);check("lost focus after SSE hides",popup.hidden,true);
	// An agent with no direct source must not borrow its child's/parent's template.
	var orphan=anchor("orphan","nested");orphan.children=[];hit=orphan;events.mouseover({target:orphan,clientX:40,clientY:60});check("no direct source stays closed",popup.hidden,true);
	if(bad.length) console.log(bad.join("\n"));
	`)
}

func TestSessionHoverAgyOutputRateAndUnknownModel(t *testing.T) {
	now := testNow
	at := now.Add(-2 * time.Second).UnixMilli()
	reg := Registration{ReplyID: "agychild", Harness: "agy", RegisteredAt: stampFor(now.Add(-time.Minute))}
	snap := &wslive.Snapshot{
		Adapter: "agy", HeartbeatMs: 30000, UpdatedAt: now.UnixMilli(),
		Lead: wslive.Lead{
			Activity:   wslive.Activity{Phase: wslive.PhaseIdle},
			OutputRate: &wslive.OutputRate{OutputTps: 69.6, At: at},
		},
	}

	// Idle after eligible turn: hover shows rounded rate and Unknown observed model.
	row := railRow(railInput{Session: Session{Registration: reg, Label: "agy worker"}, Snapshot: snap}, true, now)
	if row.Hover.Rate != 70 || row.Hover.RateAt != at || row.Hover.Observed != nil {
		t.Fatalf("idle agy hover = %+v", row.Hover)
	}
	html := renderNamed(t, "session-hover", row)
	contains(t, "idle agy hover", html, "Observed output model", "Unknown", `class="tps">70`, "Last-turn output rate")

	// Second turn generating: hover retains prior rate and keeps Unknown observed model.
	snap.Lead.Phase, snap.Lead.TurnStartedAt = wslive.PhaseReplying, at+1000
	row = railRow(railInput{Session: Session{Registration: reg, Label: "agy worker"}, Snapshot: snap}, true, now)
	if row.Hover.Rate != 70 || row.Hover.RateAt != at || row.Hover.Observed != nil {
		t.Fatalf("working agy hover = %+v", row.Hover)
	}
	html = renderNamed(t, "session-hover", row)
	contains(t, "working agy hover", html, "Observed output model", "Unknown", `class="tps">70`, "Output rate")

	// Stopped/stale agy session hides live rate.
	stopped := reg
	stopped.HostStopped = true
	row = railRow(railInput{Session: Session{Registration: stopped}, Snapshot: snap}, true, now)
	if row.Hover.Rate != 0 {
		t.Fatalf("stopped agy hover kept rate: %+v", row.Hover)
	}

	// Parent subagent row shows rate while running and Unknown observed model in hover.
	child := AgentNode{Agent: wslive.Agent{
		ID: "pi:agychild", ReplyID: "agychild", AgentKind: wslive.AgentKindPi, Harness: "agy",
		Label: "agy worker", Status: wslive.StatusRunning, Model: "gemini-3.8-flash", OutputTps: 69.6,
	}}
	rowHTML := renderNamed(t, "agent", child)
	contains(t, "running agy subagent row", rowHTML, `class="atps" title="output: 70 tokens/s">70 t/s`, "Observed output model", "Unknown")
	child.Status = wslive.StatusDone
	if doneHTML := renderNamed(t, "agent", child); strings.Contains(doneHTML, "atps") {
		t.Fatalf("ended agy subagent row kept rate: %s", doneHTML)
	}

	// Authoritative session reopen warms the sidecar once; repeated rail renders
	// after snapshot invalidations use only that recovered in-memory value.
	stateDir := t.TempDir()
	sessionDir := SessionDir(stateDir, reg.ReplyID)
	if err := wslive.WriteOutputRate(sessionDir, &wslive.OutputRate{OutputTps: 69.6, At: at}); err != nil {
		t.Fatal(err)
	}
	writeSnapshot(t, stateDir, reg.ReplyID, &wslive.Snapshot{
		SchemaVersion: 1, ReplyID: reg.ReplyID, Adapter: "agy", HeartbeatMs: 30000, UpdatedAt: now.UnixMilli(),
		Lead: wslive.Lead{Activity: wslive.Activity{Phase: wslive.PhaseReplying}, TurnStartedAt: at + 1000},
	})
	var cache SnapshotCache
	beforeWarm := cache.outputRates.Reads()
	source := agentSessions{inner: newFakeSource(), dir: stateDir, now: func() time.Time { return now }, snaps: &cache}
	if _, ok, err := source.ReadSession(reg); err != nil || !ok {
		t.Fatalf("authoritative session warm: ok=%v err=%v", ok, err)
	}
	if got := cache.outputRates.Reads() - beforeWarm; got != 2 {
		t.Fatalf("authoritative recovery warm sidecar operations = %d, want 2 bounded reads", got)
	}
	beforeRenders := cache.outputRates.Reads()
	var reopened *wslive.Snapshot
	for range 3 {
		cache.Invalidate(stateDir, reg.ReplyID)
		reopened = cache.Snapshot(stateDir, reg.ReplyID)
		if reopened == nil || reopened.Lead.OutputRate == nil || reopened.Lead.OutputRate.OutputTps != 69.6 || reopened.Lead.Observed != nil {
			t.Fatalf("reopened snapshot cache overlay = %+v", reopened)
		}
	}
	if got := cache.outputRates.Reads() - beforeRenders; got != 0 {
		t.Fatalf("three invalidated rail renders read output-rate sidecar %d times, want 0", got)
	}
	reopenedRow := railRow(railInput{Session: Session{Registration: reg}, Snapshot: reopened}, true, now)
	if reopenedRow.Hover.Rate != 70 || reopenedRow.Hover.RateAt != at {
		t.Fatalf("reopened hover = %+v", reopenedRow.Hover)
	}

	// A current snapshot already carries the folded rate, so a cold render does
	// not touch the recovery sidecar.
	currentStateDir := t.TempDir()
	if err := os.MkdirAll(SessionDir(currentStateDir, "agycurrent"), 0o700); err != nil {
		t.Fatal(err)
	}
	writeSnapshot(t, currentStateDir, "agycurrent", &wslive.Snapshot{
		SchemaVersion: 1, ReplyID: "agycurrent", Adapter: "agy", HeartbeatMs: 30000, UpdatedAt: now.UnixMilli(),
		Lead: wslive.Lead{
			Activity:   wslive.Activity{Phase: wslive.PhaseIdle},
			OutputRate: &wslive.OutputRate{OutputTps: 69.6, At: at},
		},
	})
	var currentCache SnapshotCache
	beforeReads := currentCache.outputRates.Reads()
	if got := currentCache.Snapshot(currentStateDir, "agycurrent"); got == nil || got.Lead.OutputRate == nil {
		t.Fatalf("current snapshot rate = %+v", got)
	}
	if got := currentCache.outputRates.Reads() - beforeReads; got != 0 {
		t.Fatalf("current snapshot render read output-rate sidecar %d times, want 0", got)
	}
}
