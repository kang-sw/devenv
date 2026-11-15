package wsdashboard

import (
	"regexp"
	"strings"
	"testing"
)

// streamPageHarness is a fake WebSocket, a fake connection indicator, and a
// virtual clock for running dashboard.js's stream code under node.
const streamPageHarness = `
var now = 0, timers = [], tid = 0;
function setTimeout(fn, ms) { var id = ++tid; timers.push({ id: id, at: now + (ms || 0), fn: fn }); return id; }
function clearTimeout(id) { timers = timers.filter(function (t) { return t.id !== id; }); }
function advance(ms) {
  var end = now + ms;
  for (;;) {
    timers.sort(function (a, b) { return a.at - b.at || a.id - b.id; });
    var t = timers[0];
    if (!t || t.at > end) break;
    timers.shift(); now = t.at; t.fn();
  }
  now = end;
}
var sockets = [];
function WebSocket(url) { this.url = url; this.sent = []; this.closed = false; sockets.push(this); }
WebSocket.prototype.send = function (d) { this.sent.push(d); };
WebSocket.prototype.close = function () { this.closed = true; };
WebSocket.prototype.msg = function (event, data) { this.onmessage({ data: JSON.stringify(data === undefined ? { event: event } : { event: event, data: data }) }); };
WebSocket.prototype.fail = function () { this.onerror({}); this.onclose({}); };
var window = { WebSocket: WebSocket };
var location = { protocol: "http:", host: "localhost:1234" };
var bad = [], log = [];
function check(what, got, want) { if (JSON.stringify(got) !== JSON.stringify(want)) bad.push(what + ": got " + JSON.stringify(got) + ", want " + JSON.stringify(want)); }
function wsT(k) { return k; }
var indicator = { title: "", off: false, classList: { toggle: function (c, on) { indicator.off = on; } } };
function q() { return [indicator]; }
var doc = { hidden: false };
// The page stands alone: no shell around it, no shell above it.
var shell = false, framed = false, shHidden = false, shTrackerRev = null;
function shDrafts() { return false; }
`

// TestPageStreamsRideWebSocket pins the page side of the stream transport
// (dashboard.js openStream, connect, supOpen): the page opens no
// EventSource; the page stream opens a WebSocket with the held hashes,
// answers pings, applies regions (holding them on a hidden tab while the
// tracker event still acts), shows a drop and a recovery on the connection
// indicator, forgets the tracker revision and reconnects once per drop with
// the hashes held then, and confirms "gone" by a delayed HTTP probe. The
// support chat's stream treats its first entries region after each connect
// as history and acts on the ui lines of later ones, a hidden tab included, retries a drop,
// and closes on "gone". The cases execute under node when it is installed.
func TestPageStreamsRideWebSocket(t *testing.T) {
	data, err := dashboardTestAsset("assets/dashboard.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	if strings.Contains(js, "EventSource") {
		t.Fatal("dashboard.js still opens an EventSource")
	}
	block := func(what, pattern string) string {
		t.Helper()
		got := regexp.MustCompile(pattern).FindString(js)
		if got == "" {
			t.Fatalf("dashboard.js holds no %s", what)
		}
		return got
	}
	open := block("openStream", `(?s)\n  function openStream\(path, on\) \{.*?\n  \}\n`)
	connect := block("connected ... connect", `(?s)\n  var sseOn = true;\n.*?\n  // showRegions applies`)
	sup := block("supOpen ... supClose", `(?s)\n  function supOpen\(c\) \{.*?\n  function supClose\(c\) \{.*?\n  \}\n`)
	act := block("supActUI", `(?s)\n  function supActUI\(c, added, st\) \{.*?\n  \}\n`)

	runNode(t, "page stream", "(async function () {\n"+streamPageHarness+`
function fetch() { return Promise.resolve({ status: 404 }); }
var mode = "session", rid = "rid1", worktree = "", heldNow = "board.1,head.2", hiddenRegions = {}, modeHash = "", liveMode = false;
function held() { return heldNow; }
function boardInputFull() { return null; }
function pageHidden() { return doc.hidden; }
function trackerLost() { log.push("lost"); }
function trackerNudge(rev) { log.push("nudge:" + rev); }
function swap(name, hash, html) { log.push("swap:" + name + "." + hash); }
function tlHold(name) { log.push("hold:" + name); }
function modeChanged(m) { log.push("mode:" + m); }
function syncBadge() {}
function banner(text) { log.push("banner:" + text); }
`+open+connect+`
connect();
check("one socket with the held hashes", sockets.map(function (s) { return s.url; }), ["ws://localhost:1234/s/rid1/events?h=" + encodeURIComponent("board.1,head.2") + "&topics=1"]);
var s = sockets[0];
s.onopen({});
check("open lights the indicator", indicator.off, false);
s.msg("region", { name: "board", hash: "3", html: "<p>b</p>" });
s.msg("tracker", { rev: 0 });
check("region and tracker act", log, ["swap:board.3", "nudge:0"]);
s.msg("ping");
check("ping answered", s.sent, ['{"event":"pong"}']);
log = [];
doc.hidden = true;
s.msg("region", { name: "head", hash: "4", html: "<p>h</p>" });
s.msg("tracker", { rev: 1 });
check("a hidden tab holds regions, the tracker still acts", [log, Object.keys(hiddenRegions)], [["hold:head", "nudge:1"], ["head"]]);
doc.hidden = false;
log = [];
heldNow = "board.3,head.4";
s.fail();
check("a drop shows on the indicator and forgets the revision", [indicator.off, log], [true, ["lost"]]);
advance(2999);
check("no reconnect before 3 s", sockets.length, 1);
advance(1);
check("one reconnect per drop, with the hashes held now", sockets.map(function (x) { return x.url; }).slice(1), ["ws://localhost:1234/s/rid1/events?h=" + encodeURIComponent("board.3,head.4") + "&topics=1"]);
var r = sockets[1];
r.onopen({});
check("recovery shows on the indicator", indicator.off, false);
s.msg("region", { name: "board", hash: "9", html: "" });
check("the dropped socket is ignored", log, ["lost"]);
log = [];
r.msg("gone", {});
r.onclose({});
check("gone does not banner immediately", log, ["lost"]);
advance(3000);
await Promise.resolve();
advance(10000);
check("confirmed gone is final", [log, indicator.off, r.closed, sockets.length], [["lost", "banner:js.banner.gone"], true, true, 2]);
mode = "index";
connect();
check("the index stream", sockets[2].url, "ws://localhost:1234/events?h=" + encodeURIComponent("board.3,head.4"));
process.stdout.write(bad.join("\n"));
})();
`)

	runNode(t, "support chat stream", "(function () {\n"+streamPageHarness+`
var regions = [], acted = [];
function supShown() { return true; }
var supH = { uiCommand: function (id, cmd) { acted.push(id + ":" + cmd.op + ":" + cmd.arg); } };
function uiLine(op, arg) { return { matches: function () { return true; }, getAttribute: function (k) { return { "data-ui-op": op, "data-ui-arg": arg }[k] || null; } }; }
// supRegion stands in for the DOM merge: an entries region adds one ui line.
function supRegion(c, name, html, st) {
  regions.push(name + (st ? ":live" : ":history"));
  if (st && name === "entries") supActUI(c, [uiLine("view", html)], st);
}
`+open+act+sup+`
var c = { id: "store1", conv: { id: "conv1", page: "/s/conv1" }, root: { parentNode: {} }, st: null, stFor: null, retry: null };
supOpen(c);
supOpen(c);
check("one socket for the conversation", sockets.map(function (s) { return s.url; }), ["ws://localhost:1234/s/conv1/events?regions=entries,convlive"]);
var s = sockets[0];
s.onopen({});
s.msg("region", { name: "convlive", html: "" });
s.msg("region", { name: "entries", html: "kanban" });
doc.hidden = true;
s.msg("region", { name: "entries", html: "list" });
s.msg("ping");
check("the first entries region is history, later ones act, hidden or not", [regions, acted, s.sent], [["convlive:history", "entries:history", "entries:live"], ["store1:view:list"], ['{"event":"pong"}']]);
regions = []; acted = [];
s.fail();
check("a drop closes the stream", [c.st, s.closed], [null, true]);
advance(2000);
check("one retry after 2 s", sockets.length, 2);
var r = sockets[1];
s.msg("region", { name: "entries", html: "late" });
r.msg("region", { name: "entries", html: "again" });
r.msg("region", { name: "entries", html: "table" });
check("a reconnect's first entries region is history again; the old socket is ignored", [regions, acted], [["entries:history", "entries:live"], ["store1:view:table"]]);
r.msg("gone", {});
r.onclose({});
advance(10000);
check("gone closes for good", [c.st, r.closed, sockets.length], [null, true, 2]);
process.stdout.write(bad.join("\n"));
})();
`)
}

// TestPageReloadsOnOtherAssets pins the page side of the assets identity
// (dashboard.js connect's onAssets): the same identity, none from the
// daemon, or none on the page does nothing; another one reloads, or banners
// while a draft is held; and a second reload for the same identity in one
// tab banners instead, so a page never reloads in a loop. The cases execute
// under node when it is installed.
func TestPageReloadsOnOtherAssets(t *testing.T) {
	data, err := dashboardTestAsset("assets/dashboard.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	block := func(what, pattern string) string {
		t.Helper()
		got := regexp.MustCompile(pattern).FindString(js)
		if got == "" {
			t.Fatalf("dashboard.js holds no %s", what)
		}
		return got
	}
	open := block("openStream", `(?s)\n  function openStream\(path, on\) \{.*?\n  \}\n`)
	connect := block("connected ... connect", `(?s)\n  var sseOn = true;\n.*?\n  // showRegions applies`)
	draft := block("holdsDraft", `\n  function holdsDraft\(\) \{[^\n]+\n`)
	modeChanged := block("modeChanged", `(?s)\n  function modeChanged\(next\) \{.*?\n  \}\n`)

	runNode(t, "assets identity", "(function () {\n"+streamPageHarness+`
var mode = "session", rid = "rid1", worktree = "", hiddenRegions = {}, modeHash = "", liveMode = false;
function held() { return ""; }
function trackerLost() {}
function trackerNudge() {}
function boardSaveReload() { log.push("save-board"); }
function swap() {}
function tlHold() {}
function syncBadge() {}
function banner(text) { log.push("banner:" + text); }
var ctext = { value: "" }, cmdSel = null, imgs = [], closeRequests = [];
function boardAnswers() { return false; }
function topicNotes() { return []; }
var mine = "aaa";
var root = { getAttribute: function (k) { return k === "data-assets" ? mine : null; } };
var stored = {};
window.sessionStorage = {
  getItem: function (k) { return Object.prototype.hasOwnProperty.call(stored, k) ? stored[k] : null; },
  setItem: function (k, v) { stored[k] = String(v); }
};
location.reload = function () { log.push("reload"); };
`+open+draft+modeChanged+connect+`
connect();
var s = sockets[0];
s.msg("assets", { id: "aaa" });
check("the same assets do nothing", log, []);
s.msg("assets", {});
check("a daemon naming none does nothing", log, []);
s.msg("assets", { id: "bbb" });
check("other assets save before reloading", log, ["save-board", "reload"]);
log = [];
s.msg("assets", { id: "bbb" });
check("a second reload for the same assets banners instead", log, ["banner:js.banner.updated"]);
log = [];
s.msg("assets", { id: "ccc" });
check("newer assets reload again", log, ["save-board", "reload"]);
log = [];
ctext.value = "draft";
s.msg("assets", { id: "ddd" });
check("a held draft banners", log, ["banner:js.banner.updated"]);
log = [];
ctext.value = "";
closeRequests = [{ topic: "close-only", reason: "" }];
check("an empty-reason close-only draft is held", holdsDraft(), true);
s.msg("assets", { id: "close-assets" });
check("a close-only draft prevents an assets reload", log, ["banner:js.banner.updated"]);
log = [];
modeChanged("live");
modeChanged("session");
check("a close-only draft prevents mode reloads", log, ["banner:js.banner.appeared", "banner:js.banner.wentaway"]);
log = [];
closeRequests = [];
check("removing the last close request releases the draft guard", holdsDraft(), false);
s.msg("assets", { id: "close-assets" });
modeChanged("live");
check("assets and modes can reload after removing the request", log, ["save-board", "reload", "reload"]);
log = [];
mine = null;
s.msg("assets", { id: "eee" });
check("a page rendered without an identity does nothing", log, []);
log = [];
mine = "aaa";
shell = true;
var shFrames = [{ el: { contentWindow: { wsSaveBoardReload: function () { log.push("save-frame"); } } } }, { el: { contentWindow: { wsSaveBoardReload: function () { log.push("save-inactive"); } } } }];
var shOn = shFrames[0];
s.msg("assets", { id: "fff" });
check("shell saves only its active frame before reload", log, ["save-board", "save-frame", "reload"]);
log = [];
shOn = { el: {} };
s.msg("assets", { id: "ggg" });
check("shell tolerates an unavailable active frame", log, ["save-board", "reload"]);
log = [];
shOn = null;
s.msg("assets", { id: "hhh" });
check("shell tolerates no active frame", log, ["save-board", "reload"]);
process.stdout.write(bad.join("\n"));
})();
`)
}
