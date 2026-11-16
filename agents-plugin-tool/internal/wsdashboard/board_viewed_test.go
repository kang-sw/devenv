package wsdashboard

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wswhiteboard"
	"golang.org/x/net/html"
)

func viewedHarness(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile("testdata/viewed-panels.cjs")
	if err != nil {
		t.Fatal(err)
	}
	return string(data) + dashboardFuncs(t, "topicKey", "topicSeen", "boardExposed", "boardPanelNodes", "boardPanelKey", "boardRenderBlockers", "boardOwnerSnapshot", "boardStagePaged", "boardStageInput", "boardStageOrder", "boardRecordPending", "boardPendingBadge", "boardFlushViewed", "boardWithSnap", "boardScroller", "boardLivePanels", "boardPanelMap", "boardViewedPanels", "boardStageSnap", "boardExposureUnits", "boardReleaseDue", "boardPendingNames", "boardHome", "boardStageTop")
}

func TestBoardViewedCrossTopicInsertion(t *testing.T) {
	runNode(t, "cross-topic insertion", viewedHarness(t)+`
function must(what, ok) { if (!ok) throw new Error(what); }
var oldA=topic("A","a1",[panel("pA","unchanged A")],100);
var oldB=topic("B","b1",[panel("pB","unchanged B")],100);
var liveA=topic("A","a1",[panel("pA","unchanged A",true)],100);
var liveB=topic("B","b1",[panel("pB","unchanged B",true)],100);
liveA.exposed=liveB.exposed=true;
liveA.scroller=liveB.scroller=main;
main.panels=liveA.panels.concat(liveB.panels);
main.panels.forEach(function(p){p.scroller=main;});
setLive([liveA,liveB]);
var appended=panel("pNew","new 40px panel");appended.height=40;
var nextA=topic("A","a2",[panel("pA","unchanged A"),appended],140);
var previous=input([oldA,oldB]),incoming=input([nextA,oldB]);
boardRendered=previous;boardOffered={previous:previous,input:incoming};
// Literal model geometry: A spans [-99,1), B spans [1,101), and
// the reading line at 1 belongs to B. A still has one exposed pixel.
var scroll=99,before=geometry([oldA,oldB],scroll),readingLine=1;
must("fixture has one exposed pixel of A and reading reference B",
  Math.min(before.A+100,200)-Math.max(before.A,0)===1 &&
  before.B===readingLine && before.B+100>readingLine);
must("fixture appends only a new 40px panel without rewriting existing bytes/order",
  nextA.panels[0].outerHTML===oldA.panels[0].outerHTML &&
  nextA.panels[1]===appended && appended.height===40 &&
  nextA.height-oldA.height===40 && incoming.topics.B===oldB &&
  incoming.order.join()===previous.order.join());
var staged=boardStageInput(incoming);
// This must fail on the old product BEFORE any reference-B correction.
// Correcting B alone would hide its shift while displacing exposed A.
must("cross-topic insertion must be staged before reference-B correction",
  staged.topics.A===oldA && staged.topics.B===oldB && !!boardPending.A);
boardRecordPending();boardPendingBadge();
must("cross-topic insertion feeds the shared pill from pending data",
  window.wbPill.pending===1 && boardPendingNames().join()==="A");
var rendered=project([oldA,oldB],[staged.topics.A,staged.topics.B],[oldA,oldB]);
var uncorrected=geometry(rendered,scroll),delta=uncorrected.B-before.B;
var after=geometry(rendered,scroll+delta);
must("both exposed panels remain fixed with the sole correction anchored to B",
  after.A===-99 && after.B===1 && delta===0);
must("staging leaves the offered canonical insertion immutable",
  incoming.topics.A===nextA && nextA.panels.length===2 && topicSeen(nextA)==="a2");
`)
}

func TestBoardViewedRenderingAndRelease(t *testing.T) {
	runNode(t, "viewed rendering release", viewedHarness(t)+`
var oldA=topic("A","a1",[panel("p1","old1"),panel("p2","same2")]);
var oldB=topic("B","b1",[panel("p3","old3")]);
var oldC=topic("C","c1",[panel("p4","old4")]);
var liveA=topic("A","a1",[panel("p1","live1",true),panel("p2","live2",true)]);liveA.exposed=true;
var liveB=topic("B","b1",[panel("p3","live3",true)]);liveB.exposed=true;
var liveC=topic("C","c1",[panel("p4","live4",false)]);
setLive([liveA,liveB,liveC]);boardRendered=input([oldA,oldB,oldC]);
var nextA=topic("A","a2",[panel("p1","new1"),panel("p2","same2")]);
var nextB=topic("B","b2",[panel("p3","new3")]);
var nextC=topic("C","c2",[panel("p4","new4")]);
var next=input([nextB,nextA,nextC]);
boardOffered={previous:boardRendered,input:next};
var staged=boardStageInput(next);boardRecordPending();
check("all exposed changed panels hold their canonical bytes",staged.topics.A===oldA && staged.topics.B===oldB);
check("unrelated offscreen change is not buffered",staged.topics.C===nextC);
check("partial input never advances held hashes",topicSeen(staged.topics.A)==="a1" && topicSeen(staged.topics.C)==="c2");
check("incoming snapshot remains immutable",next.topics.A===nextA && next.order.join()==="B,A,C");
// p1 leaves completely; unchanged p2 remains visible in the SAME large topic.
liveA.panels[0].exposed=false;
staged=boardStageInput(next);
check("affected panel release is not topic-offscreen release",staged.topics.A===nextA && liveA.exposed && liveA.panels[1].exposed);
check("another changed exposed panel still holds",staged.topics.B===oldB);
// A newer snapshot changes the newly exposed p2 too: re-evaluate, do not
// release that rendering just because the earlier p1 blocker left.
var thirdA=topic("A","a3",[panel("p1","newer1"),panel("p2","changed2")]);
staged=boardStageInput(input([thirdA,nextB,nextC]));
check("newly affected exposed panel becomes a blocker",staged.topics.A===nextA && boardPending.A[0].id==="p2");
boardRelease=true;staged=boardStageInput(input([thirdA,nextB,nextC]));boardRelease=false;
check("explicit release admits every canonical rendering",staged.topics.A===thirdA && staged.topics.B===nextB);
// Owner input is narrowly scoped, not a global board exemption.
// Restore p1's exposure after the earlier offscreen-release case.
liveA.panels[0].exposed=true;
boardRendered=input([oldA,oldB,oldC]);boardOwn.A=true;
staged=boardStageInput(next);
check("external update cannot borrow a local send grant",staged.topics.A===oldA && !boardImmediate.A && boardOwn.A);
var receipt=panel('e99','owner receipt');receipt.owner=true;
var ownA=topic('A','own',[panel('p1','old1'),panel('p2','same2'),receipt]);
staged=boardStageInput(input([ownA,nextB,nextC]));
check("owner receipt applies immediately, unrelated topic held",staged.topics.A===ownA && staged.topics.B===oldB && boardImmediate.A);
// A concurrent panel rewrite is retained while the owner's new receipt
// lands from the same server snapshot, without attributing the rewrite.
boardRendered=input([oldA,oldB,oldC]);boardOwn.A=true;
var mixed=topic('A','mixed',[panel('p1','external rewrite'),panel('p2','same2'),receipt]);
staged=boardStageInput(input([mixed,nextB,nextC]));
check("mixed update admits only the owner receipt",staged.topics.A.panels[0].outerHTML==='old1' && staged.topics.A.panels.at(-1).id==='e99' && boardPending.A && boardImmediate.A);
// One exposed pixel is enough: the owner inspection, not reading-line
// selection or a topic size threshold, is the eligibility authority.
check("dashboard delegates exact exposure to owner",boardExposed(liveA.panels[1]));
process.stdout.write(bad.join("\n"));`)
}

func TestBoardViewedPlacementAndQueuedCounts(t *testing.T) {
	runNode(t, "viewed placement and counts", viewedHarness(t)+`
var A=topic("A","a1"),B=topic("B","b1"),C=topic("C","c1"),P=topic("P","p1"),Q=topic("Q","q1"),R=topic("R","r1");
var current=[A,B,C],pinCurrent=[P,Q,R];
check("visible A/B relative order stays",keys(freezeCheck(current,[B,A,C],[A,B],"main reorder"))==="A,B,C");
check("second queued reorder does not displace A/B",keys(freezeCheck(current,[C,B,A],[A,B],"queued main"))==="C,A,B");
check("pinned owner has same interval guarantee",keys(freezeCheck(pinCurrent,[Q,P,R],[P,Q],"pin reorder"))==="P,Q,R");
freezeCheck(pinCurrent,[R,Q,P],[P,Q],"queued pins");
// A completely offscreen topic above the interval may grow; a single
// owner correction then preserves EACH visible unit, not just the first.
var bigC=topic("C","c2",[],450);
freezeCheck([C,A,B],[bigC,A,B],[A,B],"offscreen growth above");
freezeCheck([A,B,C],[A,B,bigC],[A,B],"offscreen growth below");
check("only remaining visible B: canonical reorder releases",keys(project(current,[B,A,C],[B]))==="B,A,C");
check("no blockers: latest canonical order releases",keys(project(current,[C,B,A],[]))==="C,B,A");
// Content morphs preserve interval placement by stable topic key, not old
// live object identity (which would falsely advance boardSeen).
var newA=topic("A","a2");
var mapped=project(current,[B,newA,C],[A,B]);
check("placement staging admits an eligible new rendering",mapped[0]===newA && mapped[1]===B);
// Revisions retain their ledger evidence, but the pill counts distinct
// pending topics. Replaying the latest snapshot must not increment again.
boardChanges=[];boardPending={A:true};
var one=input([A]),two=input([newA]),newerA=topic("A","a3"),three=input([newerA]);
boardOffered={previous:one,input:two};boardRecordPending();
boardOffered={previous:two,input:three};boardRecordPending();
boardRecordPending();boardPendingBadge();
check("two revisions of same topic count once",window.wbPill.pending===1 && boardChanges.length===2);
boardPending={};boardRecordPending();boardPendingBadge();
check("release clears pending ledger and shared control",boardChanges.length===0 && window.wbPill.pending===0);
process.stdout.write(bad.join("\n"));`)
}

func TestBoardViewedTopOriginClickOnly(t *testing.T) {
	runNode(t, "top-origin click-only", viewedHarness(t)+`
main.scrollTop=0;
var A=topic('A','a',[panel('a','visible')]),B=topic('B','b',[panel('b','far')]);
var liveA=topic('A','a',[panel('a','visible',true)]),liveB=topic('B','b',[panel('b','far')]);
liveA.exposed=true;setLive([liveA,liveB]);
boardRendered=input([A,B]);var changedB=topic('B','b2',[panel('b','new far')]);
var rise=input([changedB,A]);boardOffered={previous:boardRendered,input:rise};
var staged=boardStageInput(rise);boardRecordPending();
check('top rise preserves old order and bytes',staged.order.join()==='A,B' && staged.topics.B===B && boardPendingNames().join()==='B');
var fresh=topic('NEW','n',[panel('n','new topic')]),incoming=input([fresh,changedB,A]);
boardOffered={previous:rise,input:incoming};staged=boardStageInput(incoming);boardRecordPending();
check('unmounted new topic is counted but not rendered',staged.order.join()==='A,B' && !staged.topics.NEW && boardPendingNames().length===2);
main.scrollTop=500;liveA.exposed=false;boardOffered=null;
staged=boardStageInput(incoming);boardRecordPending();
check('scroll-away restaging cannot release top-origin changes',staged.order.join()==='A,B' && staged.topics.B===B && !staged.topics.NEW && boardPendingNames().length===2);
boardRelease=true;staged=boardStageInput(incoming);boardRecordPending();boardRelease=false;
check('explicit release admits canonical order/content and clears top latch',staged.order.join()==='NEW,B,A' && staged.topics.B===changedB && staged.topics.NEW===fresh && Object.keys(boardTopHeld).length===0 && boardPendingNames().length===0);
process.stdout.write(bad.join('\n'));`)
}

func TestBoardViewedInternalGeometry(t *testing.T) {
	runNode(t, "viewed internal geometry", viewedHarness(t)+`
var old=topic("large","old",[panel("p1","one"),panel("p2","two")]);
var live=topic("large","old",[panel("p1","one",true),panel("p2","two",true)]);live.exposed=true;
var next=topic("large","new",[panel("p1","one"),panel("inserted","new"),panel("p2","two")]);
check("insertion between visible unchanged panels is incompatible",boardRenderBlockers(live,old,next).length===2);
live.panels[0].exposed=false;
check("after earlier panel leaves, remaining panel can be owner-compensated",boardRenderBlockers(live,old,next).length===0);
// Initial inert canonical history is consulted, never sandbox child DOM.
var history=topic("large","history",[]);history.templates=[{content:{panels:[panel("p9","old9")]}}];
var nextHistory=topic("large","newHistory",[]);nextHistory.templates=[{content:{panels:[panel("p9","new9")]}}];
var liveHistory=topic("large","history",[panel("p9","live9",true)]);
check("materialized history panel uses immutable template source",boardRenderBlockers(liveHistory,history,nextHistory).length===1);
process.stdout.write(bad.join("\n"));`)
}

// Use production article identities: only the second question is exposed.
func TestBoardViewedTimelineQuestionIdentity(t *testing.T) {
	type article struct {
		Attrs map[string]string `json:"attrs"`
		HTML  string            `json:"html"`
	}
	render := func(st *wswhiteboard.State) []article {
		t.Helper()
		regions, err := wswhiteboard.RenderRegions(wswhiteboard.BuildView(st, wswhiteboard.ViewInput{Layout: wswhiteboard.LayoutFull, Location: time.UTC}))
		if err != nil {
			t.Fatal(err)
		}
		root, err := html.Parse(strings.NewReader(string(regions.Board)))
		if err != nil {
			t.Fatal(err)
		}
		var articles []article
		var walk func(*html.Node)
		walk = func(n *html.Node) {
			if n.Type == html.ElementNode && n.Data == "article" {
				attrs := map[string]string{}
				for _, attr := range n.Attr {
					attrs[attr.Key] = attr.Val
				}
				if strings.Contains(attrs["class"], "timeline-ask") {
					var out strings.Builder
					if err := html.Render(&out, n); err != nil {
						t.Fatal(err)
					}
					articles = append(articles, article{attrs, out.String()})
				}
			}
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				walk(c)
			}
		}
		walk(root)
		if len(articles) != 2 {
			t.Fatalf("want two question articles, got %d", len(articles))
		}
		return articles
	}
	for _, eventless := range []bool{false, true} {
		name := "coalesced"
		if eventless {
			name = "eventless"
		}
		t.Run(name, func(t *testing.T) {
			st := &wswhiteboard.State{}
			env := wswhiteboard.Env{Now: time.Date(2026, 11, 15, 10, 0, 0, 0, time.UTC)}
			_, problems := wswhiteboard.Apply(st, `<wb:topic name="t"><wb:log>context</wb:log></wb:topic>`, env)
			if len(problems) > 0 {
				t.Fatal(problems)
			}
			_, problems = wswhiteboard.Apply(st, `<wb:topic name="t"><wb:add class="technical"><wb:title>first</wb:title></wb:add><wb:add class="policy"><wb:title>second</wb:title><wb:option>A</wb:option><wb:option>B</wb:option></wb:add></wb:topic>`, env)
			if len(problems) > 0 {
				t.Fatal(problems)
			}
			if eventless {
				// An open question without a creation event has no history
				// line (its card stacks at the topic bottom); settled ones
				// keep their eventless rows at the history head.
				if _, problems = wswhiteboard.Apply(st, `<wb:answer id="1" value="ok">ok</wb:answer><wb:answer id="2" value="a">a</wb:answer>`, env); len(problems) > 0 {
					t.Fatal(problems)
				}
				st.Topics[0].Entries = nil
			} else if len(st.Topics[0].Entries) == 0 || !strings.HasPrefix(st.Topics[0].Entries[len(st.Topics[0].Entries)-1].Text, "Items added: ") {
				t.Fatal("fixture must coalesce the two question creation events")
			}
			before := render(st)
			st.Items[1].Title = "second changed"
			after := render(st)
			data, err := json.Marshal([][]article{before, after, render(st)})
			if err != nil {
				t.Fatal(err)
			}
			runNode(t, name+" question identity", viewedHarness(t)+`var snapshots = `+string(data)+`;
function questions(rows, exposed) {
  return rows.map(function(r, i) {
    var n=panel(r.attrs.id || '', r.html, exposed && i===1);
    n.getAttribute=function(k){return r.attrs[k] || null;};return n;
  });
}
var old=topic('t','old',questions(snapshots[0]));
var next=topic('t','next',questions(snapshots[1]));
var live=topic('t','old',questions(snapshots[0],true));
check('fixture changes only the second question',old.panels[0].outerHTML===next.panels[0].outerHTML && old.panels[1].outerHTML!==next.panels[1].outerHTML);
check('secondary question key cannot alias a durable entry',old.panels[1].getAttribute('data-entry')==='ask:2');
check('every question has a nonempty distinct data-entry',old.panels.every(function(p){return !!p.getAttribute('data-entry');}) && new Set(old.panels.map(function(p){return p.getAttribute('data-entry');})).size===2);
check('rerenders retain question keys',JSON.stringify(old.panels.map(boardPanelKey))===JSON.stringify(next.panels.map(boardPanelKey)) && JSON.stringify(next.panels.map(boardPanelKey))===JSON.stringify(questions(snapshots[2]).map(boardPanelKey)));
check('unchanged first question is not a substitute for the exposed second',boardRenderBlockers(live,old,next)[0]===live.panels[1]);
live.panels[1].exposed=false;
check('second question leaving releases its update',boardRenderBlockers(live,old,next).length===0);
process.stdout.write(bad.join("\n"));`)
		})
	}
}

// A scroll re-stages only when a release can follow, never per frame.
func TestBoardViewedScrollReleaseGate(t *testing.T) {
	runNode(t, "viewed scroll release gate", viewedHarness(t)+`
var A=topic("A","a1",[panel("p1","one",true),panel("p2","two",true)]),B=topic("B","b1",[panel("p3","three")]);
A.exposed=true;setLive([A,B]);
boardPending={A:[A.panels[0]]};boardExposedUnits=boardExposureUnits();
check("exposed units are the visible topics and panels",boardExposedUnits.length===3 && boardExposedUnits[0]===A && boardExposedUnits[1]===A.panels[0] && boardExposedUnits[2]===A.panels[1]);
check("an unchanged view does not re-stage",!boardReleaseDue());
A.panels[0].exposed=false;
check("a pending panel leaving re-stages",boardReleaseDue());
// Another topic's panel bounds the frozen interval: it leaving while its
// topic and the blocker stay exposed must still re-stage.
A.panels[0].exposed=true;B.exposed=true;B.panels[0].exposed=true;boardExposedUnits=boardExposureUnits();
check("an unchanged cross-topic view does not re-stage",!boardReleaseDue());
B.panels[0].exposed=false;
check("another topic's exposed panel leaving re-stages",boardReleaseDue());
B.exposed=false;boardExposedUnits=boardExposureUnits();
boardPending={A:true};B.exposed=true;
check("a changed exposed unit set re-stages staged placement",boardReleaseDue());
B.exposed=false;
check("placement stays staged while the interval is unchanged",!boardReleaseDue());
boardExposedUnits=null;
check("no recorded view re-stages",boardReleaseDue());
process.stdout.write(bad.join("\n"));`)
}

func TestBoardViewedFetchedPageTransfer(t *testing.T) {
	runNode(t, "viewed fetched page transfer", viewedHarness(t)+`
var old=topic('past','old',[panel('p1','old1'),panel('p2','same2')]);
var live=topic('past','old',[panel('p1','old1',true),panel('p2','same2',true)]);live.exposed=true;
var next=topic('past','next',[panel('p1','new1'),panel('p2','same2')]);
setLive([]);board.pagedTopics=[live];boardPaged.past=old;boardRendered=input([]);
var offered=input([next]);var staged=boardStageInput(offered);
check('visible fetched card does not transfer out of its page',!staged.topics.past && staged.order.length===0 && boardPending.past && !boardPagedEdits.length);
live.panels[0].exposed=false;staged=boardStageInput(offered);
check('offscreen panel rendering admits in page while sibling stays exposed',boardPagedEdits.length===1 && boardPagedEdits[0].source===next && boardPaged.past===next && !staged.topics.past);
live.exposed=false;staged=boardStageInput(offered);
check('card transfer releases only after card fully leaves',staged.topics.past===next && !boardPending.past);
board.pagedTopics=[];
process.stdout.write(bad.join("\n"));`)
}

func TestBoardViewedBoundaryAndBatchWiring(t *testing.T) {
	js, err := dashboardTestAsset("assets/dashboard.js")
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{
		`var f = docFrame(e.source), m = e.data;`,
		`if (!f || !m || typeof m !== "object") return;`,
		`if (!key || m.docKey !== key || f.getAttribute("data-doc") !== key) return;`,
		`if (f.wbNext) { postTheme(f); return; }`,
		`if (h >= 0 && h < 100000) queueFrameHeight(f, h);`,
	} {
		if !strings.Contains(string(js), s) {
			t.Errorf("trusted frame boundary changed: %s", s)
		}
	}
	batch := dashboardFuncs(t, "applyFrameHeights")
	capture, write := strings.Index(batch, "boardKeep(Array.from(batch.keys()))"), strings.Index(batch, `f.style.height = to + "px";`)
	if capture < 0 || write < capture || strings.Count(batch, "kept.h.apply()") != 1 {
		t.Error("height batch must pre-capture all actual scrollers and apply once after all writes")
	}
}
