package wsdashboard

import (
	"regexp"
	"testing"
)

// TestComposerNoteTTL runs dashboard.js's syncNote under node: a note given
// a ttl (a success) clears itself after it, a later note cancels that clear,
// and a note with no ttl (a failure) stays.
func TestComposerNoteTTL(t *testing.T) {
	data, err := dashboardFS.ReadFile("assets/dashboard.js")
	if err != nil {
		t.Fatal(err)
	}
	block := regexp.MustCompile(`(?s)  var noteTimer = 0;\n  function syncNote\(text, ttl\) \{.*?\n  \}\n  var NOTE_OK = \d+;`).FindString(string(data))
	if block == "" {
		t.Fatal("dashboard.js holds no syncNote(text, ttl) block followed by NOTE_OK")
	}
	runNode(t, "composer note ttl", `
var timers = [], now = 0;
function setTimeout(fn, ms) { timers.push({ fn: fn, at: now + ms, id: timers.length + 1 }); return timers.length; }
function clearTimeout(id) { if (id) timers[id - 1].fn = null; }
function tick(ms) { now += ms; timers.forEach(function (t) { if (t.fn && t.at <= now) { var f = t.fn; t.fn = null; f(); } }); }
var cnote = { textContent: "" };
function check(want, msg) { if (cnote.textContent !== want) throw new Error(msg + ": note = " + JSON.stringify(cnote.textContent)); }
`+block+`
syncNote("sent", NOTE_OK); check("sent", "shown");
tick(NOTE_OK - 1); check("sent", "still shown before the ttl");
tick(1); check("", "cleared at the ttl");
syncNote("sent", NOTE_OK); tick(1000); syncNote("failed");
tick(NOTE_OK * 3); check("failed", "a failure stays and the earlier clear is cancelled");
syncNote("sent", NOTE_OK); syncNote(); check("", "cleared at once");
tick(NOTE_OK); check("", "nothing comes back");
`)
}
