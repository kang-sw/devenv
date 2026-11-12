package wsdashboard

import (
	"regexp"
	"strings"
	"testing"
)

// TestComposerShowsTopicNoteChips pins the composer's topic-note chips
// (dashboard.js renderChips with topicNotes): each non-empty topic note on
// the board renders as a "[topic] text" chip after the answer chips, read
// from the board's current DOM on every render; its × empties the note
// through an input event that bubbles (the path a typed edit takes); its
// body opens a closed note through the topic's pen and focuses an open one.
// topicNotes gives each note's topic and text, which a send carries as its
// topics; restoreTopics puts a cancelled message's notes back on their
// topics through the same input path and returns a "[topic] note" line for a
// topic no longer on the board. The cases execute under node when it is
// installed.
func TestComposerShowsTopicNoteChips(t *testing.T) {
	data, err := dashboardFS.ReadFile("assets/dashboard.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	chips := regexp.MustCompile(`(?s)\n  function renderChips\(\) \{.*?\n  function boardAnswers\(\)`).FindString(js)
	if chips == "" {
		t.Fatal("dashboard.js holds no renderChips ... boardAnswers block")
	}
	chips = strings.TrimSuffix(chips, "function boardAnswers()")
	notes := regexp.MustCompile(`(?s)\n  function topicNotes\(\) \{.*?\n  \}\n`).FindString(js)
	if notes == "" {
		t.Fatal("dashboard.js holds no topicNotes")
	}
	restore := regexp.MustCompile(`(?s)\n  function restoreTopics\(list\) \{.*?\n  \}\n`).FindString(js)
	if restore == "" {
		t.Fatal("dashboard.js holds no restoreTopics")
	}
	open := regexp.MustCompile(`(?s)\n  function openNote\(note, s\) \{.*?\n  \}\n`).FindString(js)
	if open == "" {
		t.Fatal("dashboard.js holds no openNote")
	}
	runNode(t, "topic-note chip", `
function El(tag) { this.tagName = tag.toUpperCase(); this.children = []; this.parentNode = null; this.attrs = {}; this.className = ""; this.textContent = ""; this.title = ""; this.hidden = false; this.value = ""; this.events = []; this.style = {}; }
El.prototype.setAttribute = function (k, v) { this.attrs[k] = String(v); };
El.prototype.getAttribute = function (k) { return k in this.attrs ? this.attrs[k] : null; };
El.prototype.appendChild = function (c) { c.parentNode = this; this.children.push(c); return c; };
El.prototype.has = function (c) { return (" " + this.className + " ").indexOf(" " + c + " ") >= 0; };
El.prototype.all = function () { var out = []; this.children.forEach(function (c) { out.push(c); out = out.concat(c.all()); }); return out; };
El.prototype.closest = function (sel) { for (var e = this; e; e = e.parentNode) if (e.has(sel.slice(1))) return e; return null; };
El.prototype.querySelector = function (sel) { return this.all().filter(function (e) { return e.has(sel.slice(1)); })[0] || null; };
El.prototype.dispatchEvent = function (e) { this.events.push(e.type + (e.bubbles ? "/bubbles" : "")); };
El.prototype.scrollIntoView = function () { log.push("scroll:" + this.getAttribute("data-topic")); };
El.prototype.focus = function () { log.push("focus:" + this.getAttribute("data-topic")); };
El.prototype.click = function () { var n = this.parentNode.querySelector(".tnote"); n.hidden = false; log.push("pen:" + n.getAttribute("data-topic")); n.focus(); };
Object.defineProperty(El.prototype, "textContent", { get: function () { return this._t; }, set: function (v) { this._t = v; if (v === "") this.children = []; } });
function Event(type, o) { this.type = type; this.bubbles = !!(o && o.bubbles); }
var doc = { createElement: function (t) { return new El(t); }, createTextNode: function (s) { var e = new El("#text"); e.textContent = s; return e; } };
var board = new El("div"), cchips = new El("div"), log = [], bad = [], live = false;
function region(name) { return name === "board" ? board : null; }
function q(sel, root) { return root.all().filter(function (e) { return e.has("tnote") && e.getAttribute("data-topic") !== null; }); }
function wsT(k) { return k; }
function tabShown() { return true; }
function setTab() {}
var answer = [{ id: "03", answer: "A", note: "" }];
function chipList() { return answer; }
function check(what, got, want) { if (JSON.stringify(got) !== JSON.stringify(want)) bad.push(what + ": got " + JSON.stringify(got) + ", want " + JSON.stringify(want)); }
function topic(name, value, hidden) {
  var s = new El("section"), pen = new El("button"), n = new El("textarea");
  s.className = "topic"; s.setAttribute("data-topic", name);
  pen.className = "btn mini tnote-toggle";
  n.className = "note tnote"; n.setAttribute("data-topic", name); n.value = value; n.hidden = hidden;
  s.appendChild(pen); s.appendChild(n); board.appendChild(s);
  return n;
}
`+notes+chips+restore+open+`
var a = topic("Plan \"x\"", "  first note  ", false), b = topic("Empty", "   ", true), c = topic("Later", "closed text", true);
renderChips();
function row() { return cchips.children.map(function (el) { return el.className + "|" + el.children.map(function (k) { return k.textContent; }).join("|"); }); }
check("answer chip first, then one chip per non-empty note", row(), ["achip|(03)| A|×", "achip tchip|[Plan \"x\"]|first note|×", "achip tchip|[Later]|closed text|×"]);
var t1 = cchips.children[1];
check("chip carries its topic", [t1.getAttribute("data-tchip"), t1.children[2].getAttribute("data-untchip"), t1.children[2].title, t1.title], ["Plan \"x\"", "Plan \"x\"", "js.chip.tnote.remove", "js.chip.tnote.open"]);
check("a send's topics", topicNotes().map(function (n) { return [n.topic, n.value]; }), [["Plan \"x\"", "  first note  "], ["Later", "closed text"]]);
a.value = "edited";
renderChips();
check("a render reads the current note", row()[1], "achip tchip|[Plan \"x\"]|edited|×");
clearTopicNote("Plan \"x\"");
check("× empties the note through a bubbling input", [a.value, a.events], ["", ["input/bubbles"]]);
renderChips();
check("an emptied note drops its chip", row(), ["achip|(03)| A|×", "achip tchip|[Later]|closed text|×"]);
focusTopicNote("Later");
check("a closed note opens through its pen", [c.hidden, log], [false, ["scroll:Later", "pen:Later", "focus:Later"]]);
log = [];
focusTopicNote("Later");
check("an open note is focused", log, ["scroll:Later", "focus:Later"]);
log = [];
focusTopicNote("Gone"); clearTopicNote("Gone");
check("a topic no longer on the board is a no-op", log, []);
var e = topic("Empty2", "", true), l = topic("Keep", "mine", false);
var stray = restoreTopics([{ topic: "Empty2", note: "back" }, { topic: "Keep", note: "theirs" }, { topic: "Keep", note: "mine" }, { topic: "Gone", note: "lost?" }, { topic: "Empty2", note: " " }]);
check("restored notes open on their topics", [e.value, e.hidden, e.events, l.value, l.events], ["back", false, ["input/bubbles"], "mine\ntheirs", ["input/bubbles"]]);
check("a topic off the board comes back as a line", stray, ["[Gone] lost?"]);
process.stdout.write(bad.join("\n"));
`)
}
