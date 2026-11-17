package wsdashboard

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsinbox"
	"github.com/kang-sw/devenv/internal/wslive"
)

// outboxDOM is the page the outbox reads in TestComposerOutboxLeavesOnDelivery:
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
