package wswhiteboard

import (
	"encoding/json"
	"os/exec"
	"strings"
	"testing"
)

// formPrelude is a page for answers.js and form.js under node: a mini DOM
// built by parsing rendered board HTML, whose elements answer the selectors
// the form uses (tag, classes, attributes with = or ^=, :not() of one such
// selector, descendant and child combinators, selector lists), with
// closest(), listeners, and a fire() that runs them, under a root element and
// a body whose class is BODY_CLASS ("full" unless the script sets it first).
// It is enough for the answer form, not a browser: no layout, no default
// actions; requestAnimationFrame callbacks run on frame().
const formPrelude = `
var bad = [];
function check(what, got, want) { if (JSON.stringify(got) !== JSON.stringify(want)) bad.push(what + ": got " + JSON.stringify(got) + ", want " + JSON.stringify(want)); }
function E(tag, attrs) { var me = this; this.tag = tag; this.children = []; this.parent = null; this.attrs = attrs || {}; this.on = {};
  this.hidden = "hidden" in this.attrs; this.checked = "checked" in this.attrs; this.value = ""; this.textContent = ""; this.style = {}; this.scrollHeight = 20;
  this.classList = { contains: function (c) { return me.cls().indexOf(c) >= 0; },
    toggle: function (c, on) { var l = me.cls().filter(function (x) { return x !== c; }); if (on) l.push(c); me.attrs["class"] = l.join(" "); } }; }
E.prototype.cls = function () { return (this.attrs["class"] || "").split(/\s+/).filter(Boolean); };
E.prototype.getAttribute = function (k) { return k in this.attrs ? this.attrs[k] : null; };
E.prototype.setAttribute = function (k, v) { this.attrs[k] = String(v); };
E.prototype.removeAttribute = function (k) { delete this.attrs[k]; };
E.prototype.hasAttribute = function (k) { return k in this.attrs; };
E.prototype.addEventListener = function (type, fn) { (this.on[type] = this.on[type] || []).push(fn); };
E.prototype.focus = function () {}; E.prototype.blur = function () {}; E.prototype.select = function () {};
E.prototype.contains = function (n) { for (; n; n = n.parent) if (n === this) return true; return false; };
var rafs = {}, rafSeq = 0;
function requestAnimationFrame(fn) { rafs[++rafSeq] = fn; return rafSeq; }
function cancelAnimationFrame(id) { delete rafs[id]; }
function frame() { var f = rafs; rafs = {}; Object.keys(f).forEach(function (k) { f[k](); }); }
function fire(e, type) { (e.on[type] || []).forEach(function (fn) { fn.call(e, { target: e, key: "" }); }); }
function one(e, p) { var not = /^(.*):not\((.*)\)$/.exec(p); if (not) return one(e, not[1]) && !one(e, not[2]);
  var m = /^([a-z]*)((?:\.[\w-]+|\[[\w-]+(?:\^?="[^"]*")?\])*)$/.exec(p); if (!m) throw new Error("selector " + p);
  if (m[1] && e.tag !== m[1]) return false; var ok = true, cls = e.cls();
  m[2].replace(/\.([\w-]+)|\[([\w-]+)(?:(\^?)="([^"]*)")?\]/g, function (_, c, a, pre, v) {
    if (c && cls.indexOf(c) < 0) ok = false;
    if (a && (!(a in e.attrs) || (v !== undefined && (pre ? e.attrs[a].indexOf(v) !== 0 : e.attrs[a] !== v)))) ok = false; return ""; });
  return ok; }
function desc(e, out) { e.children.forEach(function (c) { out.push(c); desc(c, out); }); return out; }
function sel(root, s) { var out = [];
  s.split(",").forEach(function (alt) { var parts = alt.trim().split(/\s+/), set = [root];
    for (var i = 0; i < parts.length; i++) { var child = parts[i] === ">"; if (child) i++; var p = parts[i], next = [];
      set.forEach(function (e) { (child ? e.children : desc(e, [])).forEach(function (c) { if (one(c, p) && next.indexOf(c) < 0) next.push(c); }); }); set = next; }
    set.forEach(function (e) { if (e !== root && out.indexOf(e) < 0) out.push(e); }); });
  var all = desc(root, []); return out.sort(function (a, b) { return all.indexOf(a) - all.indexOf(b); }); }
E.prototype.querySelectorAll = function (s) { return sel(this, s); };
E.prototype.querySelector = function (s) { return sel(this, s)[0] || null; };
E.prototype.closest = function (s) { for (var e = this; e; e = e.parent) if (s.split(",").some(function (p) { return one(e, p.trim()); })) return e; return null; };
var VOID = { input: 1, br: 1, img: 1, meta: 1, hr: 1, link: 1 };
function parse(html) { var root = new E("#root"), cur = root;
  html.replace(/<!--[\s\S]*?-->|<(\/?)([a-zA-Z][\w-]*)((?:[^>"]|"[^"]*")*?)(\/?)>|[^<]+/g, function (m, close, tag, rest, self) {
    if (!tag) { if (m[0] !== "<") cur.textContent += m; return ""; }
    tag = tag.toLowerCase();
    if (close) { for (var e = cur; e && e !== root; e = e.parent) if (e.tag === tag) { cur = e.parent; break; } return ""; }
    var attrs = {}; rest.replace(/([\w:-]+)(?:="([^"]*)")?/g, function (_, k, v) { attrs[k] = v === undefined ? "" : v.replace(/&#34;/g, '"').replace(/&amp;/g, "&"); return ""; });
    var el = new E(tag, attrs); el.parent = cur; cur.children.push(el);
    if (!self && !VOID[tag]) cur = el; return ""; });
  return root; }
if (typeof BODY_CLASS === "undefined") var BODY_CLASS = "full";
var root = parse(BOARD);
var document = { getElementById: function (id) { return sel(root, "[id]").filter(function (e) { return e.attrs.id === id; })[0] || null; },
  querySelectorAll: function (s) { return sel(root, s); }, querySelector: function (s) { return sel(root, s)[0] || null; },
  addEventListener: function () {}, execCommand: function () { return true; },
  documentElement: new E("html"), body: new E("body", BODY_CLASS ? { "class": BODY_CLASS } : {}) };
var window = {};
function byId(id) { return document.getElementById(id); }
function reply() { return byId("reply-text").value.replace(/^<wb:answers>\n|\n<\/wb:answers>$/g, ""); }
function click(e) { fire(e, "click"); }
function tick(box, on) { box.checked = on; fire(box, "input"); fire(box, "change"); }
function type(note, text) { note.value = text; fire(note, "input"); fire(note, "change"); }
function key(note, text) { note.value = text; fire(note, "input"); }
`

// TestCheckRowsStageAnswersInTheReplyBar runs answers.js and form.js over a
// rendered dashboard board (bulk chips on) holding a policy item, an ack, an
// open check (03), and an assumed check (06), and pins the check staging:
// the box stages ok, the x stages no (clearing the box and opening the
// note), the two release each other, an assumed check sends a note alone,
// and neither the all-ok box nor the topic bar's bulk buttons tick a check.
// The cases execute under node when it is installed.
func TestCheckRowsStageAnswersInTheReplyBar(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed; check staging cases not executed")
	}
	regions, err := RenderRegions(BuildView(checkBoard(t), ViewInput{Layout: LayoutFull, ReplyID: "r"}))
	if err != nil {
		t.Fatal(err)
	}
	board, _ := json.Marshal(string(regions.Board))
	answers, err := embeddedAsset("answers.js")
	if err != nil {
		t.Fatal(err)
	}
	form, err := embeddedAsset("form.js")
	if err != nil {
		t.Fatal(err)
	}
	script := "var BOARD = " + string(board) + ";\n" + formPrelude + "(function () {\n" + string(answers) + string(form) + "})();\n" + `
var row = byId("item-3"), box = row.querySelector("label.ck > input"), x = row.querySelector("button.no-toggle"), note = row.querySelector("textarea.note");
check("nothing staged", reply(), "");
tick(box, true);
check("box stages ok", reply(), "03=ok");
check("a staged row is marked", row.classList.contains("staged"), true);
click(x);
check("x stages no", [reply(), box.checked, x.getAttribute("aria-pressed"), row.hasAttribute("data-no"), note.hidden], ["03=no", false, "true", true, false]);
type(note, "not yet");
check("no with a note", reply(), "03=no^not yet");
tick(box, true);
check("the box releases the x", [reply(), x.getAttribute("aria-pressed"), row.hasAttribute("data-no")], ["03=ok^not yet", "false", false]);
click(x); click(x);
check("a second x press releases it", [reply(), row.hasAttribute("data-no")], ["03=^not yet", false]);
type(note, "");
check("nothing left staged", [reply(), row.classList.contains("staged")], ["", false]);
var all = byId("ack-all");
tick(all, true);
check("the all-ok box ticks the ack alone", [reply(), box.checked], ["02=ok", false]);
tick(all, false);
document.querySelectorAll("button[data-bulk]").forEach(click);
check("the bulk buttons answer the policy pick and the ack, never the check", [reply(), box.checked], ["01=a\n02=ok", false]);
var as = byId("item-6");
check("the assumed check has a pen and no box", [!!as.querySelector("button.note-toggle"), as.querySelector("label.ck > input")], [true, null]);
click(as.querySelector("button.note-toggle"));
type(as.querySelector("textarea.note"), "wait, no");
check("the assumed check sends a note alone", reply(), "01=a\n06=^wait, no\n02=ok");
check("answered checks carry no answer control", [byId("item-4").getAttribute("data-id"), byId("item-5").getAttribute("data-id")], [null, null]);
process.stdout.write(bad.join("\n"));`
	out, err := nodeScript(node, script).CombinedOutput()
	if err != nil {
		t.Fatalf("node: %v\n%s", err, out)
	}
	if s := strings.TrimSpace(string(out)); s != "" {
		t.Errorf("check staging cases failed:\n%s", s)
	}
}

// TestCheckHideToggleKeepsItsStateOnTheRoot runs answers.js and form.js over
// the check board and pins the hide toggle: on by default (data-ck-hide="1"
// on <html>, the button pressed), which the stylesheet turns into hiding the
// rows without data-id, the answered 04 and 05 and never the open 03 or the
// assumed 06; a press turns it off and stores "0" in localStorage; a rerun of
// the script (a dashboard board swap) keeps the stored state, syncs a freshly
// rendered button, and leaves a button an earlier run wired toggling the
// shared state; and the embed never touches storage. The cases execute under
// node when it is installed.
func TestCheckHideToggleKeepsItsStateOnTheRoot(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed; check hide toggle cases not executed")
	}
	regions, err := RenderRegions(BuildView(checkBoard(t), ViewInput{Layout: LayoutFull, ReplyID: "r"}))
	if err != nil {
		t.Fatal(err)
	}
	board, _ := json.Marshal(string(regions.Board))
	answers, err := embeddedAsset("answers.js")
	if err != nil {
		t.Fatal(err)
	}
	form, err := embeddedAsset("form.js")
	if err != nil {
		t.Fatal(err)
	}
	cases := func(bodyClass, body string) string {
		return "var BOARD = " + string(board) + ";\nvar BODY_CLASS = " + bodyClass + ";\n" + formPrelude + `
var stored = {}, touched = 0;
window.localStorage = { getItem: function (k) { touched++; return k in stored ? stored[k] : null; }, setItem: function (k, v) { touched++; stored[k] = String(v); } };
function run() {
` + string(answers) + string(form) + `
}
function state() { return document.documentElement.getAttribute("data-ck-hide"); }
function pressed() { return document.querySelectorAll("button.ckhide").map(function (b) { return b.getAttribute("aria-pressed"); }); }
function ids(sel) { return document.querySelectorAll(sel).map(function (e) { return e.attrs.id; }); }
` + body + `
process.stdout.write(bad.join("\n"));`
	}
	full := cases(`"full"`, `
check("before the script runs nothing is set", state(), null);
run();
check("on by default", [state(), pressed(), stored], ["1", ["true"], {}]);
check("the rows the stylesheet hides are the answered ones", ids(".topic .checks .ckrow:not([data-id])"), ["item-4", "item-5"]);
check("open and assumed rows stay", ids(".topic .checks .ckrow[data-id]"), ["item-3", "item-6"]);
var first = document.querySelector("button.ckhide");
click(first);
check("a press turns it off and stores it", [state(), pressed(), stored["ws.wb.checks.hideAnswered"]], ["0", ["false"], "0"]);
run();
check("a rerun over the kept board keeps the stored state", [state(), pressed()], ["0", ["false"]]);
click(first);
check("the button an earlier run wired toggles the shared state once", [state(), pressed(), stored["ws.wb.checks.hideAnswered"]], ["1", ["true"], "1"]);
click(first);
root = parse(BOARD);
check("a swapped-in button renders pressed", pressed(), ["true"]);
run();
check("a rerun syncs the swapped-in button to the stored state", [state(), pressed()], ["0", ["false"]]);
document.documentElement.removeAttribute("data-ck-hide");
run();
check("a fresh page reads the stored state", state(), "0");`)
	embed := cases(`"embed"`, `
stored["ws.wb.checks.hideAnswered"] = "0";
run();
check("the embed starts on whatever another page stored", [state(), pressed()], ["1", ["true"]]);
click(document.querySelector("button.ckhide"));
check("the embed toggles for the page alone", [state(), pressed(), stored["ws.wb.checks.hideAnswered"], touched], ["0", ["false"], "0", 0]);`)
	for name, script := range map[string]string{"full": full, "embed": embed} {
		out, err := nodeScript(node, script).CombinedOutput()
		if err != nil {
			t.Fatalf("%s: node: %v\n%s", name, err, out)
		}
		if s := strings.TrimSpace(string(out)); s != "" {
			t.Errorf("%s: check hide toggle cases failed:\n%s", name, s)
		}
	}
}

// TestTypingBuildsTheReplyOnceAFrame pins the typing path of form.js: an
// input event (a keystroke) builds the reply in the next frame, once however
// many keystrokes land before it, and re-marks only the edited control's
// topic's check rows; a click or change builds the whole board at once and
// covers a waiting build; and a reader of the reply sees a keystroke typed
// just before it, since the copy button and window.wbFlushReply (the
// dashboard's boardAnswers) flush the waiting build first. The cases execute
// under node when it is installed.
func TestTypingBuildsTheReplyOnceAFrame(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed; typing cases not executed")
	}
	st := &State{}
	mustApply(t, st, `<wb:topic name="a"><wb:add class="check"><wb:title>first</wb:title></wb:add></wb:topic>
<wb:topic name="b"><wb:add class="check"><wb:title>second</wb:title></wb:add></wb:topic>`)
	regions, err := RenderRegions(BuildView(st, ViewInput{Layout: LayoutFull, ReplyID: "r"}))
	if err != nil {
		t.Fatal(err)
	}
	board, _ := json.Marshal(string(regions.Board))
	answers, err := embeddedAsset("answers.js")
	if err != nil {
		t.Fatal(err)
	}
	form, err := embeddedAsset("form.js")
	if err != nil {
		t.Fatal(err)
	}
	script := "var BOARD = " + string(board) + ";\n" + formPrelude + "(function () {\n" + string(answers) + string(form) + "})();\n" + `
var builds = 0, block = window.wbAnswers.block;
window.wbAnswers.block = function (a, b) { builds++; return block(a, b); };
var ra = byId("item-1"), rb = byId("item-2"), na = ra.querySelector("textarea.note"), boxA = ra.querySelector("label.ck > input");
var tb = document.querySelector('section.topic[data-topic="b"] textarea.tnote');
key(na, "h"); key(na, "hi");
check("a keystroke waits for its frame", [reply(), builds], ["", 0]);
frame();
check("the frame builds once for both keystrokes", [reply(), builds, ra.classList.contains("staged")], ["01=^hi", 1, true]);
// A typed build re-marks the edited topic's rows only: topic b's stale mark
// stays until a whole-board build.
rb.classList.toggle("staged", true);
key(na, "hi!"); frame();
check("a typed build leaves other topics' rows", [reply(), rb.classList.contains("staged")], ["01=^hi!", true]);
tick(boxA, true);
check("a change builds the whole board at once", [reply(), rb.classList.contains("staged")], ["01=ok^hi!", false]);
// A change covers a waiting build: the frame then builds nothing more.
key(na, "x"); var n = builds; tick(boxA, false); frame();
check("a change covers the waiting build", [reply(), builds - n], ["01=^x", 1]);
// Readers flush a keystroke typed just before them.
key(tb, "note on b");
click(byId("reply-copy"));
check("the copy button sees the keystroke", reply().indexOf("note on b") >= 0, true);
key(tb, "note on b, more");
check("wbFlushReply is the page's flush", typeof window.wbFlushReply, "function");
window.wbFlushReply();
check("boardAnswers' flush sees the keystroke", reply().indexOf("note on b, more") >= 0, true);
n = builds; frame();
check("a flushed build does not run again", builds, n);
process.stdout.write(bad.join("\n"));`
	out, err := nodeScript(node, script).CombinedOutput()
	if err != nil {
		t.Fatalf("node: %v\n%s", err, out)
	}
	if s := strings.TrimSpace(string(out)); s != "" {
		t.Errorf("typing cases failed:\n%s", s)
	}
}

// TestPillRedrawsTypingOnceAFrame pins that pill.js redraws on typing in
// the next frame, not inside the input event right after form.js grew the
// note, and drops a waiting redraw when torn down.
func TestPillRedrawsTypingOnceAFrame(t *testing.T) {
	data, err := assetFS.ReadFile("assets/pill.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{`host.addEventListener("input", redrawSoon);`, `host.removeEventListener("input", redrawSoon);`, `requestAnimationFrame(function () { typed = 0; update(); })`, `if (typed) cancelAnimationFrame(typed);`} {
		if !strings.Contains(js, want) {
			t.Errorf("pill.js lacks %q", want)
		}
	}
}
