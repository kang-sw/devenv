package wsdashboard

import (
	"net/http"
	"os/exec"
	"regexp"
	"strings"
	"testing"
)

// nodeScript runs script under node, fed on stdin rather than with -e:
// Windows caps a command line at 32767 characters, and the scripts embed
// whole assets.
func nodeScript(node, script string) *exec.Cmd {
	cmd := exec.Command(node, "-")
	cmd.Stdin = strings.NewReader(script)
	return cmd
}

// runNode runs script under node and fails on any output (each case
// prints its failed checks); it skips when node is not installed.
func runNode(t *testing.T, what, script string) {
	t.Helper()
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skipf("node not installed; %s cases not executed", what)
	}
	out, err := nodeScript(node, script).CombinedOutput()
	if err != nil {
		t.Fatalf("node: %v\n%s", err, out)
	}
	if s := strings.TrimSpace(string(out)); s != "" {
		t.Errorf("%s cases failed:\n%s", what, s)
	}
}

// toastDOM is a minimal DOM and a virtual clock (setTimeout, clearTimeout,
// Date.now, advance) for running toast.js under node.
const toastDOM = `
// fake DOM + virtual clock shared by the harnesses
var now = 0, timers = [], tid = 0;
Date.now = function () { return now; };
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
function El(tag) { this.tagName = tag.toUpperCase(); this.children = []; this.parentNode = null; this.attrs = {}; this.on = {}; this.className = ""; this.textContent = ""; this.innerHTML = ""; var self = this;
  this.classList = { add: function (c) { if (!self.classList.contains(c)) self.className = (self.className + " " + c).trim(); }, contains: function (c) { return (" " + self.className + " ").indexOf(" " + c + " ") >= 0; } }; }
El.prototype.setAttribute = function (k, v) { this.attrs[k] = String(v); };
El.prototype.getAttribute = function (k) { return k in this.attrs ? this.attrs[k] : null; };
El.prototype.hasAttribute = function (k) { return k in this.attrs; };
Object.defineProperty(El.prototype, "firstChild", { get: function () { return this.children[0] || null; } });
Object.defineProperty(El.prototype, "lastChild", { get: function () { return this.children[this.children.length - 1] || null; } });
El.prototype.appendChild = function (c) { return this.insertBefore(c, null); };
El.prototype.insertBefore = function (c, ref) { if (c.parentNode) c.parentNode.removeChild(c); var i = ref ? this.children.indexOf(ref) : -1; if (i < 0) this.children.push(c); else this.children.splice(i, 0, c); c.parentNode = this; return c; };
El.prototype.removeChild = function (c) { var i = this.children.indexOf(c); if (i >= 0) this.children.splice(i, 1); c.parentNode = null; return c; };
El.prototype.addEventListener = function (t, fn) { (this.on[t] = this.on[t] || []).push(fn); };
El.prototype.fire = function (t, e) { e = e || {}; e.type = t; (this.on[t] || []).slice().forEach(function (fn) { fn(e); }); };
var document = { body: new El("body"), createElement: function (t) { return new El(t); } };
var window = {};
var bad = [];
function check(what, got, want) { if (JSON.stringify(got) !== JSON.stringify(want)) bad.push(what + ": got " + JSON.stringify(got) + ", want " + JSON.stringify(want)); }
`

// TestToastStack pins the shared toast stack (toast.js, window.WsToast): at
// most three toasts, newest on top, a fourth dropping the oldest; an info
// toast leaves after 4 s, a warn toast after 6 s, and a notice after 8 s; a
// hover pauses the timer and leaving resumes it with the time left; a
// clickable toast is a button whose click runs its action and closes it;
// content is text unless the caller asks for HTML. The cases execute under
// node when it is installed.
func TestToastStack(t *testing.T) {
	data, err := dashboardFS.ReadFile("assets/toast.js")
	if err != nil {
		t.Fatal(err)
	}
	runNode(t, "toast stack", toastDOM+string(data)+`
var W = window.WsToast;
function texts() { var s = document.body.children[0]; return s ? s.children.map(function (c) { return c.children[0].textContent || c.children[0].innerHTML; }) : []; }
W.show("a"); W.show("b"); W.show("c");
check("three stacked, newest on top", texts(), ["c", "b", "a"]);
W.show("d");
check("a fourth drops the oldest", texts(), ["d", "c", "b"]);
advance(3999);
check("info toasts stay until 4 s", texts().length, 3);
advance(1);
check("info toasts leave at 4 s", texts(), []);
check("the stack box stays with its role", document.body.children[0].getAttribute("role"), "status");
var clicked = 0;
W.show("n", { kind: "notice", onClick: function () { clicked++; } });
var n = document.body.children[0].children[0];
check("a notice is a button", [n.className, n.getAttribute("role"), n.tabIndex], ["wstoast notice click", "button", 0]);
advance(7999);
check("a notice stays until 8 s", texts(), ["n"]);
advance(1);
check("a notice leaves at 8 s", texts(), []);
W.show("h");
var h = document.body.children[0].children[0];
advance(1000); h.fire("mouseenter"); advance(10000);
check("hover pauses the timer", texts(), ["h"]);
h.fire("mouseleave"); advance(2999);
check("leaving resumes with the time left", texts(), ["h"]);
advance(1);
check("and it leaves when that runs out", texts(), []);
W.show("k", { onClick: function () { clicked++; } });
document.body.children[0].children[0].fire("click", { preventDefault: function () {} });
check("a click runs onClick and closes", [clicked, texts()], [1, []]);
W.show("<b>x</b>");
check("text by default", document.body.children[0].children[0].children[0].textContent, "<b>x</b>");
advance(4000);
W.show("<b>x</b>", { html: true });
check("html on request", [document.body.children[0].children[0].children[0].innerHTML, document.body.children[0].children[0].children[0].textContent], ["<b>x</b>", ""]);
W.show("s", { kind: "summary" });
check("summary kind", document.body.children[0].children[0].className, "wstoast summary");
advance(100000);
W.show("w", { kind: "warn" });
check("warn kind", document.body.children[0].children[0].className, "wstoast warn");
advance(5999);
check("a warn toast stays until 6 s", texts(), ["w"]);
advance(1);
check("a warn toast leaves at 6 s", texts(), []);
check("no timers left", timers.length, 0);
process.stdout.write(bad.join("\n"));
`)
}

// tabLabelPrelude stubs what dashboard.js's tab-label block reads: regions
// as nodes answering fixed selectors, a detached parse (frag) by HTML key,
// the toast stack, wsTn, the mounted tracker's open calls, and its listed
// stores (tkOrder, which the tracker term prunes its baselines by).
const tabLabelPrelude = `
var bad = [], toasts = [], opened = [], fragCalls = 0, writes = 0;
function check(what, got, want) { if (JSON.stringify(got) !== JSON.stringify(want)) bad.push(what + ": got " + JSON.stringify(got) + ", want " + JSON.stringify(want)); }
function N(p) { p = p || {}; this.sel = p.sel || {}; this.attrs = p.attrs || {}; this.up = p.up || null; this.textContent = p.text || ""; this.value = p.value || ""; this.checked = !!p.checked; }
N.prototype.querySelectorAll = function (s) { return this.sel[s] || []; };
N.prototype.querySelector = function (s) { return (this.sel[s] || [])[0] || null; };
N.prototype.getAttribute = function (k) { return k in this.attrs ? this.attrs[k] : null; };
N.prototype.hasAttribute = function (k) { return k in this.attrs; };
N.prototype.setAttribute = function (k, v) { this.attrs[k] = String(v); };
N.prototype.closest = function () { return this.up; };
var title = "sess - ws dashboard";
var doc = { hidden: false };
Object.defineProperty(doc, "title", { get: function () { return title; }, set: function (v) { writes++; title = v; } });
var root = new N(), REG = {}, FR = {};
function region(name) { return REG[name] || null; }
function q(sel, el) { return el.querySelectorAll(sel); }
function frag(html) { fragCalls++; return FR[html]; }
var window = { WsToast: { show: function (t, o) { toasts.push({ text: t, kind: (o && o.kind) || "info", click: o && o.onClick }); } } };
function wsTn(k, n) { return k + ":" + n; }
var tkApi = { openNotice: function (x) { opened.push("notice:" + x.key); }, openNotify: function (s) { opened.push("notify:" + s); } };
var tkOrder = ["s"];
var PERM = ".phase.permission, .brow.perm", ASK = '.card.policy[id^="item-"], .arow[id^="item-"], .ckrow[data-id]:not([data-assumed])', SENT = ".sentdata i[data-id]";
function item(id, draft) { return new N({ attrs: { "data-id": id }, sel: draft ? { '.opt[data-opt][aria-pressed="true"]': [new N()] } : {} }); }
function sent(id) { return new N({ attrs: { "data-id": id } }); }
`

// TestTabLabelCount pins dashboard.js's tab label: the title prefix sums the
// pill's board asks, the permission prompts (one per agent, the
// conversation's wait folded into the lead), and the rendered tracker store's
// unread notifications; it writes only when the total changes and clears at
// 0. While hidden, a held update of a counted region is parsed detached once
// per new hash, drafted and sent asks left out. A store's first loaded render
// sets its baseline silently; a new key then toasts while shown and, while
// hidden, joins one summary toast on return that counts the arrivals still
// unread. The index page sums its sessions' policy chips and approvals. The
// cases execute under node when it is installed.
func TestTabLabelCount(t *testing.T) {
	data, err := dashboardFS.ReadFile("assets/dashboard.js")
	if err != nil {
		t.Fatal(err)
	}
	block := regexp.MustCompile(`(?s)\n  // --- tab label.*?\n  var sseOn = true;`).FindString(string(data))
	if block == "" {
		t.Fatal("dashboard.js holds no tab label ... sseOn block")
	}
	block = strings.TrimSuffix(block, "var sseOn = true;")
	runNode(t, "session tab label", tabLabelPrelude+boardBoxSrc(t, string(data))+`
var mode = "session";
var R = new N(), A = new N(), B = new N();
var treeSel = {}; treeSel[PERM] = [new N({ up: A }), new N({ up: A }), new N({ up: B })]; treeSel[".agent.root"] = [R];
REG.tree = new N({ sel: treeSel, attrs: { "data-h": "t1" } });
var cl = {}; cl[".permwait"] = [new N()]; cl[SENT] = [sent("2")];
REG.convlive = new N({ sel: cl, attrs: { "data-h": "c1" } });
var bs = {}; bs[ASK] = [item("1", true), item("2"), item("9")];
REG.board = new N({ sel: bs, attrs: { "data-h": "b1" } });
`+block+`
window.wbTitleCount(2);
check("pill count alone", title, "(2) sess - ws dashboard");
tlCount();
check("tree agents A, B and the lead's conversation wait", title, "(5) sess - ws dashboard");
var w = writes; window.wbTitleCount(2); tlCount();
check("an unchanged count writes nothing", writes, w);
check("the base is recorded once", root.getAttribute("data-wb-title"), "sess - ws dashboard");
// the wait shown in the lead's tree row folds into the lead
treeSel[PERM].push(new N({ up: R })); tlCount();
check("a lead tree row and the conversation wait count once", title, "(5) sess - ws dashboard");
// hidden: a held board parsed detached, drafted and sent asks left out
doc.hidden = true;
var hb = {}; hb[ASK] = [item("1"), item("2"), item("3"), item("4")]; FR.B2 = new N({ sel: hb });
tlHold("board", "b2", "B2");
check("held board: 3, 4 open (1 drafted live, 2 sent)", title, "(5) sess - ws dashboard");
var hb5 = {}; hb5[ASK] = hb[ASK].concat([item("5")]); FR.B2 = new N({ sel: hb5 }); tlHold("board", "b2", "B2");
check("the same hash is not parsed again", [fragCalls, title], [1, "(5) sess - ws dashboard"]);
tlHold("tree", "t1", "T1");
check("a held update equal to the live region is not parsed", fragCalls, 1);
var hc = {}; hc[SENT] = [sent("2"), sent("3")]; FR.C2 = new N({ sel: hc });
tlHold("convlive", "c2", "C2");
check("held conversation: 3 sent too; its wait gone, the lead's tree row still waits", title, "(4) sess - ws dashboard");
var ht = {}; ht[PERM] = []; FR.T2 = new N({ sel: ht });
tlHold("tree", "t2", "T2");
check("held tree without prompts", title, "(1) sess - ws dashboard");
// showing the tab: the swaps land and the held parse is dropped
doc.hidden = false; tlHeld = {};
treeSel[PERM] = []; cl[".permwait"] = []; window.wbTitleCount(0); tlCount();
check("nothing left: no prefix", title, "sess - ws dashboard");
// tracker: baseline silently, toast new while shown, summarize hidden ones
function U(keys, ready) { return { store: "s", ready: ready !== false, entries: keys.map(function (k) { return { store: "s", item: "i" + k, ev: null, key: k, kind: "mention", label: "Mention", title: "T" + k, author: k === "k3" ? "Ann" : "" }; }) }; }
tlTracker(U([], false));
check("an unloaded store counts 0", title, "sess - ws dashboard");
tlTracker(U(["k1", "k2"]));
check("first loaded render: counted, no toast", [title, toasts.length], ["(2) sess - ws dashboard", 0]);
tlTracker(U(["k1", "k2", "k3"]));
check("a new key toasts once as a notice", toasts.map(function (t) { return [t.text, t.kind]; }), [["Mention · Tk3 · Ann", "notice"]]);
toasts[0].click();
check("its click opens the entry", opened, ["notice:k3"]);
tlTracker(U(["k1", "k2", "k3"]));
check("a known key does not toast again", toasts.length, 1);
doc.hidden = true;
tlTracker(U(["k1", "k2", "k3", "k4", "k5", "k6"]));
check("hidden: counted, no toast", [title, toasts.length], ["(6) sess - ws dashboard", 1]);
tlTracker(U(["k1", "k2", "k3", "k4", "k5"]));
check("reading one drops the count", title, "(5) sess - ws dashboard");
tlShown();
check("still hidden: no summary", toasts.length, 1);
doc.hidden = false; tlShown();
check("one summary for the hidden arrivals still unread", toasts.slice(1).map(function (t) { return [t.text, t.kind]; }), [["js.toast.newnotes:2", "summary"]]);
toasts[1].click();
check("the summary opens the Notify tab", opened, ["notice:k3", "notify:s"]);
tlShown();
check("no second summary", toasts.length, 2);
process.stdout.write(bad.join("\n"));
`)
	runNode(t, "index tab label", tabLabelPrelude+boardBoxSrc(t, string(data))+`
var mode = "index";
var ss = {}; ss[".sess .chip.policy"] = [new N({ text: "3" }), new N({ text: "1" })]; ss[".sess .phase.permission"] = [new N()];
REG.sessions = new N({ sel: ss, attrs: { "data-h": "s1" } });
`+block+`
tlCount();
check("index: policy chips plus approvals", title, "(5) sess - ws dashboard");
doc.hidden = true;
var hs = {}; hs[".sess .chip.policy"] = [new N({ text: "1" })]; FR.S2 = new N({ sel: hs });
tlHold("sessions", "s2", "S2");
check("index: a held sessions region counts while hidden", title, "(1) sess - ws dashboard");
tlHold("board", "b9", "B9");
check("index: an uncounted region is not parsed", fragCalls, 1);
process.stdout.write(bad.join("\n"));
`)
}

// TestPagesCarryTheToastStackAndTitleHost pins the shared toast stack on
// every dashboard page, after the string table and ahead of the tracker and
// page scripts that use it, and the html marker that makes the board's
// open-question pill leave the title to the page.
func TestPagesCarryTheToastStackAndTitleHost(t *testing.T) {
	f := startPageFixture(t)
	for name, path := range map[string]string{"live": "/s/livesession", "board-first": "/s/boardsession", "index": "/"} {
		code, page := get(t, f.base+path)
		if code != http.StatusOK {
			t.Fatalf("GET %s = %d", path, code)
		}
		i18n, toast, tracker := strings.Index(page, "window.wsT = function"), strings.Index(page, "window.WsToast = { show: show }"), strings.Index(page, "window.WsTracker = { mount: mount }")
		if i18n < 0 || toast < i18n || tracker < toast {
			t.Errorf("%s page: the toast stack is missing or not between the string table and the tracker script", name)
		}
		if strings.Count(page, "window.WsToast = { show: show }") != 1 {
			t.Errorf("%s page carries the toast stack more than once", name)
		}
		if !regexp.MustCompile(`<html [^>]*data-wb-titlehost`).MatchString(page) {
			t.Errorf("%s page lacks the data-wb-titlehost marker", name)
		}
		for _, hook := range []string{".wstoasts {", ".wstoast.notice::before", "onRender: tlTracker", "if (O.onRender) O.onRender(UNREADS);", "openNotify: openNotify", `"js.toast.newnotes.other"`} {
			if !strings.Contains(page, hook) {
				t.Errorf("%s page lacks the toast/title hook %q", name, hook)
			}
		}
		if strings.Contains(page, "ltoast") {
			t.Errorf("%s page still carries the old link toast", name)
		}
	}
}
