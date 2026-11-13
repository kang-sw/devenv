package wsdashboard

import (
	"net/http"
	"regexp"
	"strings"
	"testing"
)

// composerFunc returns dashboard.js's top-level page-script function name,
// from its declaration to its closing brace at the same indent.
func composerFunc(t *testing.T, src, name string) string {
	t.Helper()
	block := regexp.MustCompile(`(?s)\n  function ` + name + `\(.*?\n  \}\n`).FindString(src)
	if block == "" {
		t.Fatalf("dashboard.js holds no function %s", name)
	}
	return block
}

// composerToastPrelude stubs what the composer's send, cancel, steer and
// interrupt paths read: a recording toast stack, a post whose answer each
// case sets, and no-op composer helpers.
const composerToastPrelude = `
var bad = [], toasts = [];
function check(what, got, want) { if (JSON.stringify(got) !== JSON.stringify(want)) bad.push(what + ": got " + JSON.stringify(got) + ", want " + JSON.stringify(want)); }
var window = { WsToast: { show: function (t, o) { toasts.push([t, (o && o.kind) || "info"]); } } };
function wsT(k, a) { return a === undefined ? k : k + ":" + a; }
var answer = [true, {}];
function post(route, body, done) { done(answer[0], answer[1]); }
var ctext = { value: "", focus: function () {} }, imgs = [], imgSeq = 0, bmentions = [], chips = {}, interrupting = false, interruptedTurn = null, cmdSel = null;
function sendText() { return ctext.value; }
function chipList() { return []; }
function topicNotes() { return []; }
function grow() {} function clearMentions() {} function dropImg() {} function release() {} function clearTrash() {}
function renderImgs() {} function dropChip() {} function setItem() {} function closeTopicNotes() {} function renderChips() {}
function clearDraft() {} function restoreHeld() {} function syncBadge() {} function leadTurn() { return 1; }
function btn() { return { disabled: false, getAttribute: function () { return "h1"; } }; }
`

// TestComposerOutcomesToast runs the composer's outcome paths from
// dashboard.js under node: a failure or warning shows a "warn" toast, a
// success of cancel, steer, or interrupt shows a default (info) toast, and a
// successful send shows none. The cases execute under node when it is
// installed.
func TestComposerOutcomesToast(t *testing.T) {
	data, err := dashboardFS.ReadFile("assets/dashboard.js")
	if err != nil {
		t.Fatal(err)
	}
	src := string(data)
	notes := regexp.MustCompile(`(?s)\n  function noteWarn\(text\) \{[^\n]*\n  function noteOk\(text\) \{[^\n]*\n`).FindString(src)
	if notes == "" {
		t.Fatal("dashboard.js holds no noteWarn / noteOk pair")
	}
	var fns strings.Builder
	for _, name := range []string{"sendMessage", "cancelHeld", "steerHeld", "interruptTurn"} {
		fns.WriteString(composerFunc(t, src, name))
	}
	runNode(t, "composer toasts", composerToastPrelude+notes+"var sending = false;\n"+fns.String()+`
sendMessage();
check("an empty send warns", toasts, [["js.send.empty", "warn"]]);
toasts = []; ctext.value = "hi"; answer = [false, "busy"]; sendMessage();
check("a refused send warns", toasts, [["js.send.fail:busy", "warn"]]);
toasts = []; answer = [true, {}]; sendMessage();
check("a successful send shows no toast", toasts, []);
toasts = []; answer = [false, "gone"]; cancelHeld(btn()); steerHeld(btn()); interruptTurn();
check("cancel, steer and interrupt failures warn", toasts, [["js.cancel.fail:gone", "warn"], ["js.steer.fail:gone", "warn"], ["js.interrupt.fail:gone", "warn"]]);
toasts = []; interrupting = false; answer = [true, {}]; steerHeld(btn()); interruptTurn();
interrupting = false; answer = [true, { held: [{ id: "h1" }] }]; interruptTurn();
check("steer and interrupt successes are info toasts", toasts, [["js.steer.ok", "info"], ["js.interrupt.ok", "info"], ["js.interrupt.restored", "info"]]);
toasts = []; answer = [true, { id: "h1" }]; cancelHeld(btn());
check("a cancel success is an info toast", toasts, [["js.cancel.ok", "info"]]);
process.stdout.write(bad.join("\n"));
`)
}

// TestComposerHasNoNoteRow pins that the live composer reserves no row for
// notes: its outcomes are toasts.
func TestComposerHasNoNoteRow(t *testing.T) {
	f := startPageFixture(t)
	code, live := get(t, f.base+"/s/livesession?frame=1")
	if code != http.StatusOK {
		t.Fatalf("GET /s/livesession = %d", code)
	}
	if !strings.Contains(live, `id="ctext"`) {
		t.Fatal("live session page has no composer")
	}
	if strings.Contains(live, `id="cnote"`) || strings.Contains(live, `class="cnote"`) || strings.Contains(live, ".cnote {") {
		t.Error("live session page still carries the composer note row")
	}
}
