package wsdashboard

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsinbox"
	"github.com/kang-sw/devenv/internal/wslive"
)

// outboxDOM is the page the outbox reads in TestComposerOutboxLeavesOnDelivery
// and TestComposerOutboxCancel:
// a convlive region of fake elements the client bubbles go into, and a q
// that answers "#r-entries <classes>[data-entry]" selectors (comma lists
// too) from the server's rendered entries markup in entriesHTML.
const outboxDOM = `
function fakeEl() {
  var el = { className: "", children: [], parentNode: null, attrs: {}, textContent: "",
    setAttribute: function (k, v) { el.attrs[k] = String(v); },
    getAttribute: function (k) { return k in el.attrs ? el.attrs[k] : null; },
    appendChild: function (c) { return el.insertBefore(c, null); },
    insertBefore: function (c, ref) {
      if (c.parentNode) c.parentNode.removeChild(c);
      var i = ref ? el.children.indexOf(ref) : -1;
      if (i < 0) el.children.push(c); else el.children.splice(i, 0, c);
      c.parentNode = el;
      return c;
    },
    removeChild: function (c) { var i = el.children.indexOf(c); if (i >= 0) el.children.splice(i, 1); c.parentNode = null; return c; },
    classList: { contains: function (c) { return el.className.split(/\s+/).indexOf(c) >= 0; } }
  };
  Object.defineProperty(el, "firstChild", { get: function () { return el.children[0] || null; } });
  return el;
}
var clive = fakeEl(), sentdata = fakeEl();
clive.className = "conv clive";
sentdata.className = "sentdata";
clive.appendChild(sentdata);
var doc = { querySelector: function (sel) { return sel === "#r-convlive .clive" ? clive : null; }, createElement: fakeEl };
var entriesHTML = "";
var q = function (sel) {
  var wants = sel.split(",").map(function (s) {
    var m = /^\s*#r-entries ((?:\.[\w-]+)+)\[data-entry\]\s*$/.exec(s);
    if (!m) throw new Error("unexpected selector " + s);
    return m[1].slice(1).split(".");
  });
  var out = [], re = /<\w+ class="([^"]*)"[^>]*?\sdata-entry="([^"]*)"/g, m;
  while ((m = re.exec(entriesHTML))) {
    var cls = m[1].split(/\s+/), key = m[2];
    if (wants.some(function (w) { return w.every(function (c) { return cls.indexOf(c) >= 0; }); })) {
      out.push({ getAttribute: (function (k) { return function (a) { return a === "data-entry" ? k : null; }; })(key) });
    }
  }
  return out;
};
var posted = 0;
post = function (route, body, done) { done(true, { id: "h" + (++posted) }); };
function bubbles() { return clive.children.filter(function (c) { return c.classList.contains("held"); }).map(function (c) { return c.getAttribute("data-entry"); }); }
`

// TestComposerOutboxLeavesOnDelivery pins that a sent message shows once:
// the client bubble the daemon's OK puts in the conversation leaves when the
// entries region shows the session received it, whether the session read it
// alone or in a batch with another send. A batch keys its .user-batch
// wrapper, not its bubbles, so the outbox must read that key too. The
// markup is the server's own render of whiteboard-origin entries; the cases
// execute under node when it is installed.
func TestComposerOutboxLeavesOnDelivery(t *testing.T) {
	offset := 540
	user := func(i int, parts ...string) wslive.RingRecord {
		var texts []string
		for j, p := range parts {
			texts = append(texts, wsinbox.Entry{SentAt: testNow.Add(time.Duration(i*10+j) * time.Second).UnixMilli(), UTCOffsetMinutes: &offset, Parts: []string{p}}.Text())
		}
		return ringRecord(i, wslive.RingEntry{Kind: wslive.RingUser, Origin: wslive.OriginWhiteboard, Text: strings.Join(texts, "\n\n")})
	}
	ring := []wslive.RingRecord{
		user(1, "earlier"),
		user(2, "first of the batch", "second of the batch"),
		user(3, "alone"),
	}
	entries := make([]string, len(ring))
	for n := range ring {
		v := buildSessionView(pageInputs{Session: liveSession("self", "/proj", testNow), Snapshot: liveSnap(testNow, wslive.Lead{}), Ring: ring[:n+1], Now: testNow})
		set, err := renderSessionRegions(v, newEntryCache())
		if err != nil {
			t.Fatal(err)
		}
		entries[n] = string(set[regionEntries].HTML)
	}
	if !strings.Contains(entries[1], `<div class="user-batch" data-entry="`+ring[1].Name+`">`) || strings.Count(entries[1], `class="msg user"`) != 3 {
		t.Fatalf("the batch render changed shape; the outbox reads its key from .user-batch:\n%s", entries[1])
	}
	if regexp.MustCompile(`class="msg user" data-entry="` + regexp.QuoteMeta(ring[1].Name) + `"`).MatchString(entries[1]) {
		t.Fatal("a batch bubble carries the batch key")
	}
	js, err := json.Marshal(entries)
	if err != nil {
		t.Fatal(err)
	}
	src := mustAsset(t, "dashboard.js")
	notes := regexp.MustCompile(`(?s)\n  function noteWarn\(text\) \{[^\n]*\n  function noteOk\(text\) \{[^\n]*\n`).FindString(src)
	if notes == "" {
		t.Fatal("dashboard.js holds no noteWarn / noteOk pair")
	}
	runNode(t, "composer outbox delivery", selectionCodecSource(t)+composerToastPrelude+notes+"var sending = false;\n"+composerSendSource(t, src)+outboxDOM+`
var rendered = `+string(js)+`;
entriesHTML = rendered[0];
ctext.value = "first of the batch"; sendMessage();
ctext.value = "second of the batch"; sendMessage();
check("both acknowledged sends show as bubbles", bubbles(), ["h1", "h2"]);
outboxSync();
check("a render before delivery keeps them", bubbles(), ["h1", "h2"]);
entriesHTML = rendered[1];
outboxSync();
check("the delivered batch replaces both bubbles", bubbles(), []);
ctext.value = "alone"; sendMessage();
check("a single acknowledged send shows as a bubble", bubbles(), ["h3"]);
outboxSync();
check("the earlier batch does not take the new bubble", bubbles(), ["h3"]);
entriesHTML = rendered[2];
outboxSync();
check("the delivered message replaces its bubble", bubbles(), []);
process.stdout.write(bad.join("\n"));
`)
}

// TestComposerOutboxStructuredContent pins acknowledged sends with no free text:
// their structured content must remain visible until the server bubble arrives.
func TestComposerOutboxStructuredContent(t *testing.T) {
	src := mustAsset(t, "dashboard.js")
	runNode(t, "composer structured outbox", composerToastPrelude+outboxDOM+composerFunc(t, src, "heldAct")+composerFunc(t, src, "outboxBubble")+`
function find(el, cls) {
  if (el.className === cls) return el;
  for (var i = 0; i < el.children.length; i++) { var found = find(el.children[i], cls); if (found) return found; }
  return null;
}
function content(el) { return el.textContent + el.children.map(content).join(""); }
var cases = [
  [{ chips: [{ id: "03", answer: "ok", note: "<answer & note>" }] }, "achip", "(03) ok<answer & note>"],
  [{ chips: [{ id: "04", note: "note only" }] }, "achip", "(04) notenote only"],
  [{ topics: [{ topic: "<topic>", note: "topic note" }] }, "achip tchip", "[<topic>]topic note"],
  [{ closeRequests: [{ topic: "ship", reason: "<reason>" }] }, "achip close-request-chip", "js.closerequest.label:ship<reason>"],
  [{ mentions: [{ topic: "ship", id: "03", text: "quote" }] }, "achip mchip", "[ship] (03)quote"],
  [{ picks: [{ selector: "#target", text: "picked", url: "https://example.invalid/" }] }, "achip mchip pchip", "#targetpicked"],
  [{ consoles: [{ url: "https://example.invalid/", lines: ["one", "two"] }] }, "achip mchip cchip", "console ×2https://example.invalid/"]
];
cases.forEach(function (row) {
  var bubble = outboxBubble({ id: "held", back: row[0], pics: [] });
  var chips = find(bubble, "hchips"), chip = chips && find(chips, row[1]);
  check("structured send is visible: " + row[1], chip && content(chip), row[2]);
  check("structured send keeps a bubble", !!find(bubble, "bub"), true);
});
var mixed = outboxBubble({ id: "held", back: { text: "words", chips: [{ id: "03", answer: "ok" }], topics: [{ topic: "ship", note: "note" }] }, pics: [] });
check("mixed message retains text and both chip kinds", content(mixed), "(03) ok[ship]notewords");
var mixedChips = find(mixed, "hchips");
check("chip content is escaped DOM text, not HTML", !!mixedChips && mixedChips.children.every(function (c) { return !c.innerHTML; }), true);
process.stdout.write(bad.join("\n"));
`)
}

// The request boundary only runs after the recovery write succeeds.
func TestComposerOutboxPersistsImmediateSend(t *testing.T) {
	src := mustAsset(t, "dashboard.js")
	runNode(t, "composer immediate send draft", selectionCodecSource(t)+composerToastPrelude+composerSendSource(t, src)+outboxDOM+`
var commit, pendingPost, requests = 0, returned = [];
function noteOk() {}
restoreHeld = function (list) { returned.push(list[0].text); };
persistOutbox = function (done) { commit = done; };
post = function (route, body, done) {
  pendingPost = done;
  requests++;
  check("the persisted payload is what posts", body.text, "typed and sent immediately");
};
ctext.value = "typed and sent immediately";
sendMessage();
check("composer clears immediately", ctext.value, "");
check("request awaits its recovery record", typeof pendingPost, "undefined");
check("the recovery row records posting", outbox[0].recoveryState, "posting");
commit(true);
check("the POST starts after commit", typeof pendingPost, "function");
pendingPost(true, { id: "h1" });
check("acknowledged send leaves the recovery queue", outboxUnsent(), false);
ctext.value = "cancel during the write";
sendMessage();
var cancelCommit = commit;
outboxCancel(outbox.find(function (m) { return !m.acked; }));
cancelCommit(true);
check("Cancel before the POST preserves its contents", returned, ["cancel during the write"]);
check("an eventual durable write cannot send after Cancel", requests, 1);
process.stdout.write(bad.join("\n"));
`)
}

// outboxActsDOM extends outboxDOM with the composer's heldacts template: a
// steer and a cancel button prototype whose clones record click listeners
// (click runs them), and a post whose message answers a case can hold.
const outboxActsDOM = `
function protoBtn(kind) {
  return { cloneNode: function () {
    var b = fakeEl(), on = [];
    b.className = "btn mini h" + kind;
    b.setAttribute("data-" + kind, "");
    b.addEventListener = function (ev, fn) { if (ev === "click") on.push(fn); };
    b.click = function () { on.forEach(function (fn) { fn(); }); };
    return b;
  } };
}
var protos = { steer: protoBtn("steer"), cancel: protoBtn("cancel") };
var heldacts = { content: { querySelector: function (sel) { var m = /^\[data-(\w+)\]$/.exec(sel); return m ? protos[m[1]] || null : null; } } };
var pageQuery = doc.querySelector;
doc.querySelector = function (sel) { return sel === "#heldacts" ? heldacts : pageQuery(sel); };
var calls = [], holding = false, held = [], cancelAnswer = [true, {}], restored = [], holdCancel = false, cancels = [];
post = function (route, body, done) {
  calls.push(route === "cancel" ? [route, body.id] : [route, body.text]);
  if (route !== "message") { if (holdCancel) cancels.push(done); else done(cancelAnswer[0], cancelAnswer[1]); return; }
  if (holding) held.push(done); else done(true, { id: "h" + (++posted) });
};
restoreHeld = function (list) { restored.push(list.map(function (r) { return r.text; })); };
var crow = fakeEl(), composer = fakeEl();
composer.appendChild(crow);
ctext.closest = function (sel) { return sel === ".crow" ? crow : null; };
function rows() { return composer.children[0].children.map(function (r) { return r.children[1].textContent; }); }
function bubbleOf(id) { return clive.children.filter(function (c) { return c.getAttribute("data-entry") === id; })[0]; }
function actsOf(el) { return el.children.filter(function (c) { return /\bh(steer|cancel)\b/.test(c.className); }).map(function (c) { return [c.className, c.getAttribute("data-steer") || c.getAttribute("data-cancel")]; }); }
function cancelOf(el) { return el.children.filter(function (c) { return c.classList.contains("hcancel"); })[0]; }
`

// TestComposerOutboxCancel pins the outbox's cancel: the bubble an
// acknowledged send shows carries the server held bubble's steer and cancel
// buttons for its id, and its cancel takes the message back and drops the
// bubble so no sync puts it back; a refused cancel leaves the bubble. A row
// still waiting for its post comes back without a post; a row whose post is
// out cancels on the daemon once the OK names its id. The cases execute
// under node when it is installed.
func TestComposerOutboxCancel(t *testing.T) {
	src := mustAsset(t, "dashboard.js")
	notes := regexp.MustCompile(`(?s)\n  function noteWarn\(text\) \{[^\n]*\n  function noteOk\(text\) \{[^\n]*\n`).FindString(src)
	if notes == "" {
		t.Fatal("dashboard.js holds no noteWarn / noteOk pair")
	}
	runNode(t, "composer outbox cancel", selectionCodecSource(t)+composerToastPrelude+notes+"var sending = false;\n"+composerSendSource(t, src)+composerFunc(t, src, "cancelHeld")+outboxDOM+outboxActsDOM+`
ctext.value = "one"; sendMessage();
check("an acknowledged send's bubble carries the held buttons", actsOf(bubbleOf("h1").children[0]), [["btn mini hsteer", "h1"], ["btn mini hcancel", "h1"]]);
cancelAnswer = [true, { id: "h1", text: "one" }];
cancelHeld(cancelOf(bubbleOf("h1").children[0]));
check("the bubble's cancel posts its id", calls.slice(-1), [["cancel", "h1"]]);
check("a cancelled bubble leaves", bubbles(), []);
check("a cancelled bubble leaves the outbox", outbox.length, 0);
check("a cancel restores the daemon's copy", restored.slice(-1), [["one"]]);
outboxSync();
check("no sync puts a cancelled bubble back", bubbles(), []);

holding = true; calls = []; restored = []; toasts = [];
ctext.value = "first"; sendMessage();
ctext.value = "second"; sendMessage();
check("only the first send posts", calls, [["message", "first"]]);
check("both unanswered sends show as rows", rows(), ["first", "second"]);
cancelOf(outbox[1].row).click();
check("a queued row's cancel posts nothing", calls, [["message", "first"]]);
check("a queued row's cancel restores its text", restored, [["second"]]);
check("a queued row's cancel leaves the outbox", outbox.map(function (m) { return m.back.text; }), ["first"]);
check("a queued row's cancel confirms", toasts, [["js.cancel.ok", "info"]]);
check("a queued row's cancel removes its row", rows(), ["first"]);
cancelOf(outbox[0].row).click();
check("an in-flight row's cancel waits for the OK", calls, [["message", "first"]]);
check("an in-flight row's cancel removes its row", rows(), []);
holdCancel = true;
held.shift()(true, { id: "h9" });
check("the OK cancels the in-flight send by its id", calls, [["message", "first"], ["cancel", "h9"]]);
check("an in-flight send whose cancel is out shows no bubble", bubbles(), []);
outboxSync();
check("no sync shows a bubble whose cancel is out", bubbles(), []);
holdCancel = false;
cancels.shift()(true, { id: "h9", text: "first" });
check("the cancelled in-flight send shows no bubble", bubbles(), []);
check("the cancelled in-flight send leaves the outbox", outbox.length, 0);
check("the cancelled in-flight send comes back", restored, [["second"], ["first"]]);
check("the queued cancelled send never posts", held.length, 0);

holding = false; toasts = [];
ctext.value = "two"; sendMessage();
var two = bubbles()[0], btn2 = cancelOf(bubbleOf(two).children[0]);
cancelAnswer = [false, "delivered"];
cancelHeld(btn2);
check("a refused cancel keeps the bubble", bubbles(), [two]);
check("a refused cancel re-enables its button", btn2.disabled, false);
check("a refused cancel warns", toasts, [["js.cancel.fail:delivered", "warn"]]);

holding = true; toasts = [];
ctext.value = "three"; sendMessage();
cancelOf(outbox[outbox.length - 1].row).click();
held.shift()(true, { id: "h8" });
check("an in-flight send whose cancel is refused shows its bubble", bubbles(), [two, "h8"]);
check("an in-flight send whose cancel is refused warns", toasts, [["js.cancel.fail:delivered", "warn"]]);
process.stdout.write(bad.join("\n"));
`)
}

// TestComposerHeldActsTemplate pins the live composer's heldacts template
// the outbox clones its buttons from: the held bubble's own steer and cancel
// markup, with an empty id for the client to fill.
func TestComposerHeldActsTemplate(t *testing.T) {
	f := startPageFixture(t)
	code, live := get(t, f.base+"/s/livesession?frame=1")
	if code != http.StatusOK {
		t.Fatalf("GET /s/livesession = %d", code)
	}
	m := regexp.MustCompile(`<template id="heldacts">(.*?)</template>`).FindStringSubmatch(live)
	if m == nil {
		t.Fatal("live composer has no heldacts template")
	}
	for _, want := range []string{`<button class="btn mini hsteer" data-steer=""`, `<button class="btn mini hcancel" data-cancel=""`} {
		if !strings.Contains(m[1], want) {
			t.Errorf("heldacts template lacks %s:\n%s", want, m[1])
		}
	}
}
