package wsdashboard

import (
	"net/http"
	"regexp"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wslive"
)

// slashDOM is a minimal DOM, a virtual clock, and a composer textarea for
// running mention.js and dashboard.js's slash command section under node.
const slashDOM = `
var bad = [];
function check(what, got, want) { if (JSON.stringify(got) !== JSON.stringify(want)) bad.push(what + ": got " + JSON.stringify(got) + ", want " + JSON.stringify(want)); }
var now = 0, timers = [], tid = 0;
function setTimeout(fn, ms) { var id = ++tid; timers.push({ id: id, at: now + (ms || 0), fn: fn }); return id; }
function clearTimeout(id) { timers = timers.filter(function (t) { return t.id !== id; }); }
function advance(ms) { now += ms; for (;;) { var due = timers.filter(function (t) { return t.at <= now; }); if (!due.length) break; timers = timers.filter(function (t) { return t.at > now; }); due.forEach(function (t) { t.fn(); }); } }
function El(tag) {
  this.tag = tag; this.children = []; this.attrs = {}; this.style = {}; this.hidden = false; this.className = ""; this.parentNode = null; this._text = ""; this.listeners = {};
  this.offsetWidth = 100; this.offsetHeight = 20; this.offsetLeft = 0; this.offsetTop = 0; this.scrollLeft = 0; this.scrollTop = 0;
  var self = this;
  this.classList = { toggle: function (c, on) { var cs = self.className.split(" ").filter(function (x) { return x && x !== c; }); if (on) cs.push(c); self.className = cs.join(" "); } };
}
El.prototype.setAttribute = function (k, v) { this.attrs[k] = String(v); };
El.prototype.getAttribute = function (k) { return Object.prototype.hasOwnProperty.call(this.attrs, k) ? this.attrs[k] : null; };
El.prototype.removeAttribute = function (k) { delete this.attrs[k]; };
El.prototype.appendChild = function (c) { c.parentNode = this; this.children.push(c); return c; };
El.prototype.removeChild = function (c) { this.children = this.children.filter(function (x) { return x !== c; }); c.parentNode = null; };
Object.defineProperty(El.prototype, "textContent", {
  get: function () { return this.children.length ? this.children.map(function (c) { return c.textContent; }).join("") : this._text; },
  set: function (v) { this.children = []; this._text = String(v); }
});
El.prototype.querySelector = function (sel) {
  var want = sel.split(".").filter(Boolean), found = null;
  (function walk(n) { n.children.forEach(function (c) { if (found) return; var have = (c.className || "").split(" "); if (want.every(function (k) { return have.indexOf(k) >= 0; })) found = c; else walk(c); }); })(this);
  return found;
};
El.prototype.addEventListener = function (t, fn) { (this.listeners[t] = this.listeners[t] || []).push(fn); };
El.prototype.dispatchEvent = function (e) { (this.listeners[e.type] || []).forEach(function (fn) { fn(e); }); };
El.prototype.getBoundingClientRect = function () { return { left: 0, top: 500 }; };
function Event(type) { this.type = type; }
var innerWidth = 1000, innerHeight = 800;
var window = globalThis;
window.addEventListener = function () {};
function getComputedStyle() { return {}; }
var document = { body: new El("body"), activeElement: null, createElement: function (tag) { return new El(tag); } };
var ctext = new El("textarea");
ctext.value = ""; ctext.selectionStart = ctext.selectionEnd = 0;
ctext.setAttribute("placeholder", "hint");
ctext.setRangeText = function (s, a, b) { this.value = this.value.slice(0, a) + s + this.value.slice(b); this.selectionStart = this.selectionEnd = a + s.length; };
ctext.setSelectionRange = function (a, b) { this.selectionStart = a; this.selectionEnd = b; };
ctext.focus = function () { document.activeElement = ctext; };
document.activeElement = ctext;
// type puts s in the box with the caret at its end, as typing it would.
function type(s) { ctext.value = s; ctext.selectionStart = ctext.selectionEnd = s.length; ctext.dispatchEvent(new Event("input")); advance(100); }
function key(popup, k) { var e = { key: k, prevented: false, preventDefault: function () { this.prevented = true; } }; return popup.key(e) && e.prevented; }
// popRows reads a popup's list: "# heading" for a heading, else each row's
// spans' text joined by "|".
function popRows(pop) {
  return pop.children.map(function (li) { return li.className === "wsm-head" ? "# " + li.textContent : li.children.map(function (c) { return c.textContent; }).join("|"); });
}
function pops() { return document.body.children.filter(function (c) { return c.className === "wsm-pop"; }); }
`

// slashPrelude stubs what the slash command section reads from the rest of
// dashboard.js: the cmds region, the capsule, the post, the toasts, and the
// composer helpers.
const slashPrelude = `
var doc = document, rid = "sess1", sending = false, imgs = [], toasts = [], posted = [], answer = [true, { id: "h1", merged: false }];
var cmds = new El("div"), crow = new El("div"), ccmd = new El("span"), cname = new El("b"), cargs = new El("span");
cname.className = "ccmd-name"; cargs.className = "ccmd-args"; ccmd.hidden = true;
ccmd.appendChild(cname); ccmd.appendChild(cargs); crow.appendChild(ccmd);
function $(id) { return id === "ccmd" ? ccmd : null; }
function region(name) { return name === "cmds" ? cmds : null; }
function q(sel, el) { return el.children.filter(function (c) { return c.tag === "i"; }); }
function setCommands(list) {
  cmds.children = [];
  list.forEach(function (c) { var i = new El("i"); i.setAttribute("data-name", c[0]); if (c[1]) i.setAttribute("data-desc", c[1]); if (c[2]) i.setAttribute("data-src", c[2]); if (c[3]) i.setAttribute("data-layer", c[3]); cmds.appendChild(i); });
}
function wsT(k, a) { return a === undefined ? k : k + ":" + a; }
function noteWarn(t) { toasts.push(t); }
function post(route, body, done) { posted.push([route, body]); done(answer[0], answer[1]); }
function editText(a, b, s) { ctext.setRangeText(s, a, b); ctext.dispatchEvent(new Event("input")); }
function sendText() { return ctext.value; }
function grow() {} function clearMentions() {} function queueDraft() {} function clearDraft() {}
`

// slashSection is dashboard.js's slash command section, from its banner to
// the next section's.
func slashSection(t *testing.T, src string) string {
	t.Helper()
	block := regexp.MustCompile(`(?s)\n  // --- composer slash commands -.*?\n  // --- board mentions`).FindString(src)
	if block == "" {
		t.Fatal("dashboard.js holds no composer slash commands section before the board mentions one")
	}
	return block
}

func pageScripts(t *testing.T) (mention, dashboard string) {
	t.Helper()
	m, err := dashboardFS.ReadFile("assets/mention.js")
	if err != nil {
		t.Fatal(err)
	}
	d, err := dashboardFS.ReadFile("assets/dashboard.js")
	if err != nil {
		t.Fatal(err)
	}
	return string(m), string(d)
}

// TestSlashPickerListsAndPicks runs mention.js and the composer's slash
// command section under node: / in an empty box opens the session's
// commands (name, description, source), grouped under user-layer and
// agent-layer headings where the listing tells and as one list where it
// cannot, filtered by the typed prefix; a session with no list gets no
// picker and / stays text; a pick, by Enter or by a space after an exact
// name, makes the command the selection and empties the box; a send posts
// {command, args}; Backspace in an empty box removes the selection; a send
// with images attached is refused; the @ file mention still inserts paths.
func TestSlashPickerListsAndPicks(t *testing.T) {
	mention, dash := pageScripts(t)
	if !strings.Contains(dash, "(mention && mention.key(e)) || (slash && slash.key(e))") {
		t.Error("the composer's keydown does not hand keys to the slash picker")
	}
	runNode(t, "slash picker", slashDOM+mention+slashPrelude+slashSection(t, dash)+composerFunc(t, dash, "sendMessage")+`
var pop = pops()[0];
check("the picker is attached", !!slash, true);

// no list: no picker, / is text
setCommands([]);
type("/");
check("no list opens no picker", pop.hidden, true);
check("no list keeps / as text", [ctext.value, cmdSel], ["/", null]);

// a list without layers is one list, in the host's order
setCommands([["compact", "Free up context", "builtin"], ["review", "Review a branch", "plugin"], ["context", "", ""]]);
type("/");
check("one list", popRows(pop), ["/compact|Free up context|builtin", "/review|Review a branch|plugin", "/context|"]);
type("/co");
check("filtered by prefix", popRows(pop), ["/compact|Free up context|builtin", "/context|"]);
type("/co x");
check("whitespace closes the picker", pop.hidden, true);

// layers group the rows
setCommands([["deploy", "Ship it", "skill", "agent"], ["compact", "Free up context", "builtin", "user"], ["misc", "", "mcp"], ["clear", "", "builtin", "user"]]);
type("/");
check("grouped by layer", popRows(pop), ["# js.cmd.group.user", "/compact|Free up context|builtin", "/clear||builtin", "# js.cmd.group.agent", "/deploy|Ship it|skill", "# js.cmd.group.rest", "/misc||mcp"]);
type("/c");
check("a filtered group keeps its heading", popRows(pop), ["# js.cmd.group.user", "/compact|Free up context|builtin", "/clear||builtin"]);

// Enter picks the active row
check("Enter is taken", key(slash, "Enter"), true);
check("the pick is the selection", [cmdSel, ctext.value, ccmd.hidden, cname.textContent, crow.className, ctext.getAttribute("placeholder")],
  [{ name: "compact", args: null }, "", false, "/compact", "hascmd", "js.cmd.args:compact"]);
check("the picker closed", pop.hidden, true);
type("/");
check("no picker while a command is selected", pop.hidden, true);
type("keep the plan");
sendMessage();
check("the send posts the command and its arguments", posted, [["message", { command: "compact", args: "keep the plan" }]]);
check("the sent selection and box clear", [cmdSel, ctext.value, ccmd.hidden, ctext.getAttribute("placeholder")], [null, "", true, "hint"]);

// a space after an exact name picks it
type("/clear");
check("Space picks the exact name", key(slash, " "), true);
check("picked by space", [cmdSel && cmdSel.name, ctext.value], ["clear", ""]);
type("/cl");
uncommand();
check("Backspace in an empty box removes the selection", [cmdSel, ccmd.hidden], [null, true]);
type("/cl");
check("a space after a partial name is text", key(slash, " "), false);

// images refuse a command send and stay
type("/");
key(slash, "Enter");
imgs = [{ n: 1 }]; posted = [];
type("[Image #1]");
sendMessage();
check("a send with images is refused", [posted, toasts, cmdSel && cmdSel.name], [[], ["js.cmd.images"], "compact"]);
imgs = []; clearCommand(); ctext.value = "";

// the @ file mention is unchanged
var picked = [], files = window.wsMention.attach(ctext, {
  source: function (q, done) { done(q === "sr" ? [{ path: "src/a.go" }, { path: "src", dir: true }] : []); },
  onPick: function (p) { picked.push(p.text); }
});
var fpop = pops()[1];
type("see @sr");
check("the file rows", popRows(fpop), ["a.go|src/", "src/|"]);
check("the slash picker stays shut", pop.hidden, true);
key(files, "Enter");
check("a file pick inserts the mention", [ctext.value, picked], ["see @src/a.go ", ["@src/a.go"]]);
process.stdout.write(bad.join("\n"));
`)
}

// TestSlashCommandRestoresAsSelection runs restoreHeld under node: a
// returned command becomes the selection with its arguments in the box;
// with other text in the composer its arguments stay on the capsule, a send
// posts them and keeps the text; a second command comes back as its typed
// line with a warning.
func TestSlashCommandRestoresAsSelection(t *testing.T) {
	mention, dash := pageScripts(t)
	restore := composerFunc(t, dash, "restoreHeld") + composerFunc(t, dash, "restoreHeldNow") + composerFunc(t, dash, "validPicks") + composerFunc(t, dash, "validConsoles")
	send := composerFunc(t, dash, "sendMessage")
	runNode(t, "slash restore", slashDOM+mention+slashPrelude+`
var imgSeq = 0, chips = {}, bmentions = [], wpicks = [], wconsoles = [], holdSync = false, MARKER = "\\[Image #([1-9][0-9]{0,5})\\]";
function allNums() { return markerNums(ctext.value); } function syncMarkers() {}
function markerNums(text) { var out = {}, re = new RegExp(MARKER, "g"), m; while ((m = re.exec(text))) out[+m[1]] = true; return out; }
function marker(n) { return "[Image #" + n + "]"; }
function attachSrc(n) { return n; } function restoreTopics() { return []; } function shiftMentions() {}
function putChip() {} function setItem() {} function validMentions() { return []; } function renderChips() {} function renderImgs() {}
`+slashSection(t, dash)+restore+send+`
setCommands([["compact", "", "builtin"]]);
restoreHeld([{ text: "", command: { name: "compact", args: "focus" } }]);
check("a lone command restores as the selection", [cmdSel, ctext.value, ccmd.hidden], [{ name: "compact", args: null }, "focus", false]);
sendMessage();
check("its resend is a command", posted, [["message", { command: "compact", args: "focus" }]]);

posted = [];
ctext.value = "draft";
restoreHeld([{ text: "plain" }, { text: "", command: { name: "compact", args: "focus" } }, { text: "", command: { name: "review" } }]);
check("with other text the arguments stay on the capsule", [cmdSel, cargs.textContent, cargs.hidden], [{ name: "compact", args: "focus" }, "focus", false]);
check("the other text and the second command's line", ctext.value, "plain\n\n/review\n\ndraft");
check("the second command warns", toasts, ["js.cmd.astext"]);
sendMessage();
check("a capsule's arguments send as they are", posted, [["message", { command: "compact", args: "focus" }]]);
check("the box keeps its text for a later send", [cmdSel, ctext.value], [null, "plain\n\n/review\n\ndraft"]);

ctext.value = "";
setCommand("compact", "focus");
uncommand();
check("Backspace on a capsule with arguments puts them in the box", [cmdSel, ctext.value], [{ name: "compact", args: null }, "focus"]);

// A cancel's picked elements and console blocks go back ahead of the
// draft's; a malformed one is dropped.
cmdSel = null; ctext.value = ""; toasts = [];
wpicks = [{ url: "http://localhost:5173/", selector: "#d", text: "draft" }];
restoreHeld([{ text: "", picks: [{ url: "http://localhost:5173/a", selector: "#a", text: "A" }, { url: "", selector: "#bad", text: "" }],
  consoles: [{ url: "http://localhost:5173/a", lines: ["e1", 7, " "] }, { url: "http://localhost:5173/a", lines: [] }] }]);
check("returned picks go ahead of the draft's", wpicks, [{ url: "http://localhost:5173/a", selector: "#a", text: "A" }, { url: "http://localhost:5173/", selector: "#d", text: "draft" }]);
check("returned console blocks keep their lines", wconsoles, [{ url: "http://localhost:5173/a", lines: ["e1"] }]);
process.stdout.write(bad.join("\n"));
`)
}

// TestHeldCommandBubbleHasNoSteer pins the held command bubble: its typed
// line and the cancel control, and no steer control, which the daemon
// refuses for a command.
func TestHeldCommandBubbleHasNoSteer(t *testing.T) {
	f := startPageFixture(t)
	f.withCommands(t, wslive.SlashCommand{Name: "compact", Source: "builtin"})
	text, _ := f.send(t, `{"text":"plain"}`)
	cmd, _ := f.send(t, `{"command":"compact","args":"keep <the> plan"}`)
	code, page := get(t, f.base+"/s/livesession?frame=1")
	if code != http.StatusOK {
		t.Fatalf("GET /s/livesession = %d", code)
	}
	bubble := regexp.MustCompile(`(?s)<div class="msg user held hcmd" data-entry="` + cmd + `">.*?</div></div>`).FindString(page)
	if bubble == "" {
		t.Fatalf("no held command bubble for %s:\n%s", cmd, page)
	}
	contains(t, "held command bubble", bubble, `<code class="cmdtyped">/compact keep &lt;the&gt; plan</code>`, `data-cancel="`+cmd+`"`)
	lacks(t, "held command bubble", bubble, `data-steer=`)
	if !strings.Contains(page, `data-steer="`+text+`"`) {
		t.Error("the held text message lost its steer control")
	}
}

// TestCommandEntryShowsItsOutput pins the command ring entry: the typed
// line and the output with its whitespace, or the terminal note when the
// command showed no text.
func TestCommandEntryShowsItsOutput(t *testing.T) {
	ring := []wslive.RingRecord{
		ringRecord(1, wslive.RingEntry{Kind: wslive.RingCommand, Command: "/context", Text: "  used <40%>\n  free 60%"}),
		ringRecord(2, wslive.RingEntry{Kind: wslive.RingCommand, Command: "/config"}),
	}
	set, err := renderSessionRegions(historyView(ring, "", PageContent{}, sessionLookup{}), newEntryCache())
	if err != nil {
		t.Fatal(err)
	}
	html := string(set[regionEntries].HTML)
	out := regexp.MustCompile(`(?s)<div class="cmdrun" data-entry="` + ring[0].Name + `">.*?</pre></div>`).FindString(html)
	contains(t, "command with output", out, `<code>/context</code>`, `<pre class="cmdout">  used &lt;40%&gt;`+"\n"+`  free 60%</pre>`)
	lacks(t, "command with output", out, "cmdnote")
	none := regexp.MustCompile(`(?s)<div class="cmdrun" data-entry="` + ring[1].Name + `">.*?</div></div>`).FindString(html)
	contains(t, "command with no output", none, `<code>/config</code>`, `<div class="cmdnote">no text output: it opens in the terminal</div>`)
	lacks(t, "command with no output", none, "cmdout")
}

// TestCommandListReachesThePageOnlyWhenListed pins the cmds region: the
// snapshot's commands as data the picker reads (name, description, source,
// and layer when set), and nothing for a snapshot with no list.
func TestCommandListReachesThePageOnlyWhenListed(t *testing.T) {
	now := testNow
	view := func(cmds ...wslive.SlashCommand) string {
		snap := liveSnap(now, wslive.Lead{})
		snap.Commands = cmds
		set, err := renderSessionRegions(buildSessionView(pageInputs{Session: liveSession("self", "/p", now), Snapshot: snap, Now: now}), nil)
		if err != nil {
			t.Fatal(err)
		}
		r, ok := set[regionCmds]
		if !ok {
			t.Fatal("the live page has no cmds region")
		}
		return string(r.HTML)
	}
	if got := view(); got != "" {
		t.Errorf("a snapshot with no commands rendered %q", got)
	}
	got := view(wslive.SlashCommand{Name: "compact", Description: `Free "context"`, Source: "builtin"}, wslive.SlashCommand{Name: "deploy", Layer: wslive.LayerAgent})
	want := `<i data-name="compact" data-desc="Free &#34;context&#34;" data-src="builtin"></i><i data-name="deploy" data-layer="agent"></i>`
	if got != want {
		t.Errorf("cmds region = %s\nwant %s", got, want)
	}

	f := startPageFixture(t)
	_, page := get(t, f.base+"/s/livesession?frame=1")
	if !strings.Contains(page, `<div id="r-cmds" class="region r-cmds" data-region="cmds"`) || strings.Contains(page, `data-name="compact"`) {
		t.Error("the live page without a command list has data in its cmds region, or no region")
	}
	f.withCommands(t, wslive.SlashCommand{Name: "compact", Source: "builtin"})
	_, page = get(t, f.base+"/s/livesession?frame=1")
	if !strings.Contains(page, `<i data-name="compact" data-src="builtin"></i>`) {
		t.Error("the live page does not carry the session's command list")
	}
}

// TestSwapReopensThePickerWhenTheListArrives: a session whose command list
// arrives after the page (a just-created hosted session) opens the picker
// for a / already waiting in the box, with no reload and no further key.
func TestSwapReopensThePickerWhenTheListArrives(t *testing.T) {
	_, dash := pageScripts(t)
	if !strings.Contains(dash, `if (name === "cmds" && slash) slash.sync();`) {
		t.Error("swap does not hand a new command list to the slash picker")
	}
}
