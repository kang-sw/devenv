package wsdashboard

import (
	"regexp"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wswhiteboard"
)

// jsBlock returns the dashboard.js source a pattern matches, failing the
// test when it matches nothing.
func jsBlock(t *testing.T, js, what, pattern string) string {
	t.Helper()
	src := regexp.MustCompile(pattern).FindString(js)
	if src == "" {
		t.Fatalf("dashboard.js holds no %s", what)
	}
	return src
}

// tlAskSrc is dashboard.js's TL_ASK declaration, the tab label's selector
// for the board's open asks.
func tlAskSrc(t *testing.T, js string) string {
	t.Helper()
	return jsBlock(t, js, "TL_ASK", `var TL_ASK = '[^'\n]*';`) + "\n"
}

// checkClientDOM is a node per element answering fixed selectors (sel), with
// attributes, a class set, children for inserted marks, and the stubs the
// extracted dashboard.js blocks read. A check row (checkRow) holds its tick
// box, its x (button.no-toggle), its note, and a head with an ID chip.
const checkClientDOM = `
var bad = [];
function check(what, got, want) { if (JSON.stringify(got) !== JSON.stringify(want)) bad.push(what + ": got " + JSON.stringify(got) + ", want " + JSON.stringify(want)); }
function N(attrs, sel) { this.attrs = attrs || {}; this.sel = sel || {}; this.cls = {}; this.kids = []; this.checked = false; this.hidden = true; this.value = ""; this.fired = 0; this.textContent = ""; var me = this;
  this.classList = { add: function () { for (var i = 0; i < arguments.length; i++) me.cls[arguments[i]] = true; }, remove: function () { for (var i = 0; i < arguments.length; i++) delete me.cls[arguments[i]]; }, contains: function (c) { return !!me.cls[c]; } }; }
N.prototype.querySelectorAll = function (s) { return this.sel[s] || []; };
N.prototype.querySelector = function (s) { return (this.sel[s] || [])[0] || null; };
N.prototype.getAttribute = function (k) { return k in this.attrs ? this.attrs[k] : null; };
N.prototype.setAttribute = function (k, v) { this.attrs[k] = String(v); };
N.prototype.removeAttribute = function (k) { delete this.attrs[k]; };
N.prototype.hasAttribute = function (k) { return k in this.attrs; };
N.prototype.dispatchEvent = function () { this.fired++; };
N.prototype.insertBefore = function (c, ref) { var i = this.kids.indexOf(ref); c.parentNode = this; if (i < 0) this.kids.push(c); else this.kids.splice(i, 0, c); };
N.prototype.removeChild = function (c) { this.kids.splice(this.kids.indexOf(c), 1); };
Object.defineProperty(N.prototype, "firstChild", { get: function () { return this.kids[0] || null; } });
Object.defineProperty(N.prototype, "nextSibling", { get: function () { var p = this.parentNode; return p ? p.kids[p.kids.indexOf(this) + 1] || null : null; } });
function Event() {}
var BOX = "label.ack > input, label.ck > input", PICK = '.opt[data-opt][aria-pressed="true"], .btn[data-v][aria-pressed="true"]';
function checkRow(id) {
  var box = new N(), x = new N({ "aria-pressed": "false" }), note = new N(), idc = new N(), head = new N({}, { ".id": [idc] }), s = {};
  head.insertBefore(idc, null);
  s[BOX] = [box]; s["button.no-toggle"] = [x]; s["textarea.note"] = [note]; s[".head, .stext"] = [head];
  var row = new N({ "data-id": id }, s); row.box = box; row.x = x; row.note = note; row.head = head; return row;
}
// boardQ answers a board item lookup only through a .ckrow selector, so a
// lookup that leaves check rows out finds nothing.
var ROWS = {}, MARKS = [];
function boardQ(sel) {
  if (sel === ".sentmark") return MARKS.slice();
  if (sel === ".sent-sent, .sent-held") return Object.keys(ROWS).map(function (k) { return ROWS[k]; }).filter(function (r) { return r.cls["sent-sent"] || r.cls["sent-held"]; });
  var m = sel.match(/\.ckrow\[data-id="([^"]+)"\]/);
  return m && ROWS[m[1]] ? [ROWS[m[1]]] : [];
}
function q(sel, el) { return (el || PAGE).querySelectorAll(sel); }
`

// TestCheckRowClientPaths pins dashboard.js's check-row paths under node:
// readItem reads a staged "no" (the row's data-no) as the answer "no" and
// setItem puts it back on the row (its x pressed, its box clear), then
// clears it; tlDrafted counts a staged "no" or a ticked check box as
// drafted, and a blank note as not; applySent finds a check row by its ID
// and marks it sent or held. The cases execute under node when it is
// installed.
func TestCheckRowClientPaths(t *testing.T) {
	data, err := dashboardFS.ReadFile("assets/dashboard.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	src := jsBlock(t, js, "setCheckNo", `(?s)\n  function setCheckNo\(item, on\) \{.*?\n  \}\n`) +
		jsBlock(t, js, "boardItem ... setItem block", `(?s)\n  function boardItem\(id\) \{.*?\n  // syncChips takes`) +
		jsBlock(t, js, "tlDrafted", `(?s)\n  function tlDrafted\(el\) \{.*?\n  \}\n`) +
		jsBlock(t, js, "applySent", `(?s)\n  function applySent\(force\) \{.*?\n  \}\n`)
	runNode(t, "check-row client", checkClientDOM+`
var PAGE = new N(), BOARD = new N(), sentSig = "";
var doc = { createElement: function () { return new N(); } };
function region(name) { return name === "board" ? BOARD : null; }
function wsT(k) { return k; }
`+src+`
var a = checkRow("03"); ROWS["03"] = a;
a.setAttribute("data-no", ""); a.x.setAttribute("aria-pressed", "true");
var chip = readItem(a);
check("readItem reads a staged no", chip, { id: "03", answer: "no", note: "" });
var b = checkRow("03"); ROWS["03"] = b;
setItem("03", chip);
check("setItem stages the no", [b.hasAttribute("data-no"), b.x.getAttribute("aria-pressed"), b.box.checked], [true, "true", false]);
check("setItem lets the form rebuild its reply", b.note.fired, 1);
check("the round trip reads back the same chip", readItem(b), chip);
setItem("03", { id: "03", answer: "ok", note: "" });
check("an ok clears the no and ticks the box", [b.hasAttribute("data-no"), b.x.getAttribute("aria-pressed"), b.box.checked], [false, "false", true]);
check("a ticked check row reads ok", readItem(b).answer, "ok");
setItem("03", null);
check("no chip clears the row", [b.hasAttribute("data-no"), b.box.checked, readItem(b)], [false, false, null]);

var c = checkRow("04");
check("an untouched check row is not drafted", tlDrafted(c), false);
c.note.value = "   ";
check("a blank note is not a draft", tlDrafted(c), false);
c.setAttribute("data-no", "");
check("a staged no is drafted", tlDrafted(c), true);
c.removeAttribute("data-no"); c.box.checked = true;
check("a ticked check box is drafted", tlDrafted(c), true);

ROWS = { "03": b, "05": checkRow("05") };
var held = new N({ "data-id": "03", "data-state": "held", "data-answer": "no", "data-note": "" });
var sent = new N({ "data-id": "05", "data-state": "sent", "data-answer": "ok", "data-note": "" });
PAGE.sel[".sentdata i[data-id]"] = [held, sent];
applySent(true);
check("a held check row is marked held", [b.cls["sent-held"], b.cls["sent-sent"]], [true, undefined]);
check("a sent check row is marked sent", ROWS["05"].cls["sent-sent"], true);
var mark = b.head.kids[1];
check("the mark follows the ID chip", [b.head.kids.length, mark && mark.className, mark && mark.textContent], [2, "sentmark held", "held · no"]);

// A note-only sent chip marks its row "sent · note"; once the sent data
// drops the item's entry (the note's turn settled), the next run clears the
// row's sent class and its mark, and the other rows keep theirs.
ROWS["06"] = checkRow("06");
var noted = new N({ "data-id": "06", "data-state": "sent", "data-answer": "", "data-note": "why" });
PAGE.sel[".sentdata i[data-id]"] = [held, sent, noted];
function marks() { var out = []; Object.keys(ROWS).forEach(function (k) { ROWS[k].head.kids.forEach(function (m) { if (/^sentmark/.test(m.className || "")) out.push(m); }); }); return out; }
MARKS = marks();
applySent(false);
var nm = ROWS["06"].head.kids[1];
check("a note-only chip marks its row sent", [ROWS["06"].cls["sent-sent"], nm && nm.textContent], [true, "sent · note"]);
PAGE.sel[".sentdata i[data-id]"] = [held, sent];
MARKS = marks();
applySent(false);
check("a dropped entry clears the row's sent class", [ROWS["06"].cls["sent-sent"], ROWS["06"].cls["sent-held"]], [undefined, undefined]);
check("a dropped entry removes the row's mark", ROWS["06"].head.kids.length, 1);
check("the other rows keep their marks", [b.cls["sent-held"], ROWS["05"].cls["sent-sent"], b.head.kids.length, ROWS["05"].head.kids.length], [true, true, 2, 2]);
process.stdout.write(bad.join("\n"));`)
}

// TestTabLabelAsksAreThePillsItems pins that the tab label's open asks
// (TL_ASK) are the items the board's open-question pill counts: on the
// dashboard the pill hands its count to the tab label, and a hidden tab
// counts a held board with TL_ASK in its place, so the two must select the
// same items, check rows included and assumed ones left out.
func TestTabLabelAsksAreThePillsItems(t *testing.T) {
	data, err := dashboardFS.ReadFile("assets/dashboard.js")
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`var TL_ASK = '([^'\n]*)';`).FindStringSubmatch(string(data))
	if m == nil {
		t.Fatal("dashboard.js holds no TL_ASK")
	}
	if !strings.Contains(m[1], ".ckrow[data-id]:not([data-assumed])") {
		t.Errorf("TL_ASK %q leaves out open check rows", m[1])
	}
	wb, err := wswhiteboard.PageAssets()
	if err != nil {
		t.Fatal(err)
	}
	if want := "d.querySelectorAll('" + m[1] + "')"; !strings.Contains(string(wb.FormScript), want) {
		t.Errorf("the pill's items are not TL_ASK: the board script lacks %s", want)
	}
}
