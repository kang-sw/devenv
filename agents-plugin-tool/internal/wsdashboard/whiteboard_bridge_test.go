package wsdashboard

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

func whiteboardBridgeFunctions(t *testing.T, js string, names ...string) string {
	t.Helper()
	var out strings.Builder
	for _, name := range names {
		block := regexp.MustCompile(`\n  function ` + name + `\([^)]*\) \{[^\n]*\}\n`).FindString(js)
		if block == "" {
			block = regexp.MustCompile(`(?s)\n  function ` + name + `\([^)]*\) \{\n.*?\n  \}\n`).FindString(js)
		}
		if block == "" {
			t.Fatalf("dashboard.js holds no %s", name)
		}
		out.WriteString(block)
	}
	return out.String()
}

func TestBoardEntryFrameBridgeStaysNarrow(t *testing.T) {
	data, err := dashboardFS.ReadFile("assets/dashboard.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	src := whiteboardBridgeFunctions(t, js, "boardJump", "docFrame", "postDoc", "str", "num", "mentionOf", "selEl", "entryAt", "frameMention")
	listener := regexp.MustCompile(`window\.addEventListener\("message", function \(e\) \{\n    var f = docFrame\(e\.source\), m = e\.data;\n    if \(!f \|\| !m \|\| typeof m !== "object"\) return;[\s\S]*?\n  \}\);\n`).FindString(js)
	if listener == "" {
		t.Fatal("no source-gated frame listener")
	}
	runNode(t, "entry frame bridge", `
var bad = [];
function check(what, got, want) { if (JSON.stringify(got) !== JSON.stringify(want)) bad.push(what + ": got " + JSON.stringify(got) + ", want " + JSON.stringify(want)); }
var calls = [], jumps = [], pending = null, mpend = null, cchips = {}, handler;
var board = { name: "board" }, pins = { name: "pins" }, targets = {};
function target(id, owned) { return targets[id] = { id: id, owned: owned, tagName: "DIV", parentNode: null, scrollIntoView: function () { jumps.push(id); } }; }
var fold = { tagName: "DETAILS", owned: true, open: false, parentNode: null };
target("item-3", true).parentNode = fold;
target("p1", true).parentNode = fold;
target("item-4", false); target("p404", false);
function boardBoxes() { return [board, pins]; }
function inBoard(n) { return !!n && n.owned === true; }
function $(id) { return targets[id] || null; }
var window = { wbHistory: { loadTarget: function (root, id) {
  calls.push(root.name + ":" + id);
  if (id === "p9" && root === pins) return target("p9", true);
  return root === board && targets[id] || null;
} }, addEventListener: function (kind, fn) { if (kind === "message") handler = fn; } };
var topic = { getAttribute: function (k) { return k === "data-topic" ? "trusted-topic" : null; } };
var wrapper = { nodeType: 1, getAttribute: function (k) { return k === "data-entry" ? "e7" : k === "data-entry-item" ? "3" : null; }, closest: function (s) { return s === "[data-entry]" ? this : topic; } };
function frame(owned, entry) {
  return { nodeType: 1, owned: owned, parentNode: entry ? wrapper : topic, contentWindow: {},
    getAttribute: function (k) { return k === "data-entry" ? entry : k === "data-doc-key" || k === "data-doc" ? "entry:" + entry : null; },
    closest: function (s) { return s === "[data-entry]" ? (entry ? this : null) : topic; } };
}
var own = frame(true, "e7"), foreign = frame(false, "e999"), frames = [own, foreign];
function q() { return frames; }
function placeMention(p) { pending = p; mpend = p; }
function hideMention() { pending = null; mpend = null; }
`+src+listener+`
boardJump("p1"); boardJump("p9"); boardJump("item-3");
check("existing panel/item targets scroll, including deferred pinned history", jumps, ["p1", "p9", "item-3"]);
check("containing disclosures open before navigation", fold.open, true);
var count = calls.length;
["#p1", "e7", "p0", "p01", "p1 div", "item-3,body", "item-3[x]", "https://evil/p1", "../p1", "__proto__", null, 7].forEach(boardJump);
check("non-allowlisted IDs never enter the history loader", calls.length, count);
boardJump("item-4"); boardJump("p404"); boardJump("p777");
check("off-board and missing targets never scroll", jumps, ["p1", "p9", "item-3"]);
handler({ source: own.contentWindow, data: { wbDoc: "sel", docKey: "entry:e7", text: "selected HTML", entry: "e999", id: "99", topic: "forged", x: 4, y: 5 } });
check("claimed locator/topic/id are ignored", pending.m, { topic: "trusted-topic", text: "selected HTML", id: "3", entry: "e7" });
check("selection coordinates remain data", [pending.x, pending.y], [4, 5]);
pending = null;
handler({ source: foreign.contentWindow, data: { wbDoc: "sel", docKey: "entry:e7", text: "foreign" } });
handler({ source: {}, data: { wbDoc: "sel", docKey: "entry:e7", text: "unknown source" } });
handler({ source: own.contentWindow, data: { wbDoc: "sel", docKey: "entry:e999", text: "wrong frame identity" } });
handler({ source: own.contentWindow, data: { wbDoc: "sel", text: "missing frame identity" } });
check("only this board's frame with matching identity may mention", pending, null);
handler({ source: foreign.contentWindow, data: { wbDoc: "jump", docKey: "entry:e7", id: "p1" } });
handler({ source: own.contentWindow, data: { wbDoc: "jump", docKey: "entry:e999", id: "p1" } });
check("foreign source or wrong identity cannot jump", jumps, ["p1", "p9", "item-3"]);
handler({ source: own.contentWindow, data: { wbDoc: "jump", docKey: "entry:e7", id: "p1" } });
check("own matching frame can jump to a validated target", jumps, ["p1", "p9", "item-3", "p1"]);
var posted = [];
own.contentWindow.postMessage = function (m) { posted.push(m); };
postDoc(own, { wbDoc: "size" });
check("host replies carry the same frame identity", posted, [{ wbDoc: "size", docKey: "entry:e7" }]);
process.stdout.write(bad.join("\n"));`)
}

func TestBoardEntryDocFrameKeyEnvelope(t *testing.T) {
	data, err := os.ReadFile("../wswhiteboard/assets/doc-frame.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	if !strings.Contains(js, `d.querySelector('meta[name="wb-doc-key"]')`) {
		t.Fatal("frame identity is not read from its rendered metadata")
	}
	post := regexp.MustCompile(`function post\(m\) \{[^\n]*\}`).FindString(js)
	listener := regexp.MustCompile(`(?s)window.addEventListener\("message", function \(e\) \{.*?\n  \}\);`).FindString(js)
	if post == "" || listener == "" {
		t.Fatal("missing frame bridge")
	}
	runNode(t, "frame identity envelope", `
var bad = [], posted = [], handler, sizes = 0, relays = 0, last = 0;
var docKey = "entry:e7", parent = { postMessage: function (m) { posted.push(m); } };
var window = { parent: parent, addEventListener: function (type, fn) { handler = fn; } };
function size() { sizes++; } function askRelays() { relays++; }
function theme() {} function relayed() {}
`+post+listener+`
post({ wbDoc: "sel", docKey: "forged", text: "selected" });
if (posted[0].docKey !== "entry:e7") bad.push("post failed to bind rendered document identity");
handler({ source: {}, data: { wbDoc: "size", docKey: "entry:e7" } });
handler({ source: parent, data: { wbDoc: "size", docKey: "entry:e8" } });
handler({ source: parent, data: { wbDoc: "size" } });
if (sizes || relays) bad.push("foreign/mismatched host message accepted");
handler({ source: parent, data: { wbDoc: "size", docKey: "entry:e7" } });
if (sizes !== 1 || relays !== 1) bad.push("matching parent message rejected");
process.stdout.write(bad.join("\n"));`)
}

func TestBoardEntryFrameHeightsAreIndependent(t *testing.T) {
	data, err := dashboardFS.ReadFile("assets/dashboard.js")
	if err != nil {
		t.Fatal(err)
	}
	src := whiteboardBridgeFunctions(t, string(data), "frameHeights", "setFrameHeights")
	runNode(t, "entry frame heights", `
var bad = [];
function check(what, got, want) { if (JSON.stringify(got) !== JSON.stringify(want)) bad.push(what + ": got " + JSON.stringify(got) + ", want " + JSON.stringify(want)); }
function frame(key, height) { return { getAttribute: function () { return key; }, style: { height: height } }; }
var old = [frame("topic:topic/e7", "100px"), frame("entry:e7", "200px"), frame("entry:e9", "300px")];
var fresh = [frame("entry:e9", ""), frame("topic:topic/e7", ""), frame("entry:e7", ""), frame("entry:e10", "")];
function q(sel, root) { return root; }
`+src+`
setFrameHeights(fresh, frameHeights(old));
check("body and panel versions keep distinct heights across a swap", fresh.map(function (f) { return f.style.height; }), ["300px", "100px", "200px", ""]);
process.stdout.write(bad.join("\n"));`)
}

func TestBoardHistorySnapshotIncludesPinnedTopics(t *testing.T) {
	data, err := dashboardFS.ReadFile("assets/dashboard.js")
	if err != nil {
		t.Fatal(err)
	}
	src := whiteboardBridgeFunctions(t, string(data), "captureBoardHistory")
	runNode(t, "history snapshots", `
var bad = [];
var window = { wbHistory: { capture: function (root) { return [{ key: root, first: "e7" }]; } } };
function boardBoxes() { return ["board", "pins"]; }
`+src+`
var got = captureBoardHistory();
if (!Array.isArray(got) || got.length !== 2 || got[0].key !== "board" || got[1].key !== "pins") bad.push("history snapshots must stay arrays across all board boxes: " + JSON.stringify(got));
process.stdout.write(bad.join("\n"));`)
}

func TestBoardHistoryClosedPagingSkipsMaterializedTopics(t *testing.T) {
	data, err := dashboardFS.ReadFile("assets/dashboard.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	src := whiteboardBridgeFunctions(t, js, "boardTopicPresent")
	loader := whiteboardBridgeFunctions(t, js, "loadClosed")
	if !strings.Contains(loader, "if (!boardTopicPresent(name)) return;") || !strings.Contains(loader, `s.setAttribute("data-slot", name)`) {
		t.Fatal("closed paging does not replace already materialized topics with count-preserving slots")
	}
	runNode(t, "closed topic deduplication", `
var bad = [];
var window = { WsCards: { pinnedTopic: function (name) { return name === "pinned"; } } };
function boardQ() { return [{ getAttribute: function () { return "jump-loaded"; } }]; }
`+src+`
if (!boardTopicPresent("jump-loaded") || !boardTopicPresent("pinned") || boardTopicPresent("not-loaded")) bad.push("closed paging would duplicate a navigation-loaded or pinned topic");
process.stdout.write(bad.join("\n"));`)
}

func TestBoardHistoryHooksSurroundSSESwap(t *testing.T) {
	data, err := dashboardFS.ReadFile("assets/dashboard.js")
	if err != nil {
		t.Fatal(err)
	}
	js := whiteboardBridgeFunctions(t, string(data), "swap")
	capture, morph := strings.Index(js, "captureBoardHistory()"), strings.Index(js, "morphBoard(el, html)")
	materialize, disclosure := strings.Index(js, "window.wbHistory.restore(n, history)"), strings.Index(js, "restoreOpen(n)")
	lastRestore, outerScroll := strings.LastIndex(js, "window.wbHistory.restore(b, history)"), strings.Index(js, "restoreScroll(scroll)")
	if capture < 0 || capture >= morph || materialize < morph || materialize >= disclosure || lastRestore <= outerScroll {
		t.Fatal("SSE swaps must capture before morph, load history before disclosure restoration, and restore anchors after outer scrolling")
	}
}
