package wsdashboard

import (
	"errors"
	"html"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wslive"
)

// labelPost posts a label body to session id's label route with the
// daemon's own Origin.
func (f *pageFixture) labelPost(t *testing.T, id, body string) (int, string) {
	t.Helper()
	code, _, raw := postJSON(t, f.base+"/s/"+id+"/label", f.base, body)
	return code, raw
}

// waitLabel waits until the index holds label for session id.
func (f *pageFixture) waitLabel(t *testing.T, id, label string) {
	t.Helper()
	waitFor(t, "the index to read label "+label, func() bool {
		s, ok := f.d.Index().Session(id)
		return ok && s.Label == label
	})
}

// TestLabelRouteSavesShowsAndResets pins the display label: a POST stores
// the trimmed label in the session directory and answers it; the session
// page's head, tab title, and rail row, and the index list, then show it in
// place of the reply-id, with the reply-id in the tooltip and the mailbox
// address naming the label as the name's copy text. A session without a live adapter takes
// a label too. An empty label removes the file and shows the reply-id
// again.
func TestLabelRouteSavesShowsAndResets(t *testing.T) {
	f := startPageFixture(t)

	// No label file: the head, title, and rail show the reply-id.
	_, page := get(t, f.base+"/s/livesession")
	for _, want := range []string{
		`<title>livesession - ws dashboard</title>`,
		`data-label="" data-max="64" data-title="livesession - ws dashboard"`,
		`data-copy="` + html.EscapeString((SessionView{ReplyID: "livesession", Project: f.project, Harness: "claude"}).MailboxRef()) + `"`,
		`class="sren" title="Rename (shown on this dashboard only)"`,
		`<span class="rid">livesession</span>`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("unlabelled page lacks %q", want)
		}
	}

	rev := func() uint64 { s, _ := f.d.Index().Session("livesession"); return s.Rev }
	before := rev()
	code, raw := f.labelPost(t, "livesession", `{"label":"  Build bot  "}`)
	if code != http.StatusOK || strings.TrimSpace(raw) != `{"label":"Build bot"}` {
		t.Fatalf("label = %d %s", code, raw)
	}
	if got := readSessionLabel(SessionDir(f.dir, "livesession")); got != "Build bot" {
		t.Fatalf("stored label = %q", got)
	}
	f.waitLabel(t, "livesession", "Build bot")
	if rev() <= before {
		t.Errorf("a label change did not raise the session's revision")
	}

	_, page = get(t, f.base+"/s/livesession")
	for _, want := range []string{
		`<title>Build bot - ws dashboard</title>`,
		`data-label="Build bot" data-max="64" data-title="Build bot - ws dashboard"`,
		`data-copy="` + html.EscapeString((SessionView{ReplyID: "livesession", Project: f.project, Harness: "claude", Label: "Build bot"}).MailboxRef()) + `"`,
		`<span class="rid lbl" title="livesession">Build bot</span>`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("labelled page lacks %q", want)
		}
	}
	// Display only: every address and link keeps the reply-id.
	if !strings.Contains(page, `data-rid="livesession"`) || !strings.Contains(page, `href="/s/livesession"`) {
		t.Errorf("the label replaced the reply-id outside the display")
	}
	_, index := get(t, f.base+"/?frame=1")
	if !strings.Contains(index, `<span class="rid lbl" title="livesession">Build bot</span>`) {
		t.Errorf("index list lacks the label")
	}

	// A board-first session (no live adapter) is renamed all the same.
	if code, raw := f.labelPost(t, "boardsession", `{"label":"보드 세션"}`); code != http.StatusOK {
		t.Fatalf("board session label = %d %s", code, raw)
	}
	f.waitLabel(t, "boardsession", "보드 세션")

	// An empty label resets to the reply-id and removes the file.
	if code, raw := f.labelPost(t, "livesession", `{"label":"   "}`); code != http.StatusOK || strings.TrimSpace(raw) != `{"label":""}` {
		t.Fatalf("reset = %d %s", code, raw)
	}
	if _, err := os.Stat(filepath.Join(SessionDir(f.dir, "livesession"), labelFileName)); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("reset left the label file: %v", err)
	}
	f.waitLabel(t, "livesession", "")
	_, page = get(t, f.base+"/s/livesession")
	if !strings.Contains(page, `<title>livesession - ws dashboard</title>`) || !strings.Contains(page, `title="livesession&#10;click to copy">livesession</button>`) {
		t.Errorf("reset page does not show the reply-id")
	}
	// Resetting again is no error.
	if code, raw := f.labelPost(t, "livesession", `{"label":""}`); code != http.StatusOK {
		t.Fatalf("second reset = %d %s", code, raw)
	}
}

// TestLabelRouteValidates pins the label's limits: at most 64 characters
// (counted as characters, not bytes), one line without control characters,
// and a JSON body. A refused label leaves the stored one alone.
func TestLabelRouteValidates(t *testing.T) {
	f := startPageFixture(t)
	if code, raw := f.labelPost(t, "livesession", `{"label":"kept"}`); code != http.StatusOK {
		t.Fatalf("label = %d %s", code, raw)
	}
	for _, body := range []string{
		`{"label":"` + strings.Repeat("가", 65) + `"}`,
		`{"label":"two\nlines"}`,
		`{"label":"tab\there"}`,
		`{"label":"nul\u0000"}`,
		`{"label":"line\u2028sep"}`,
		`{"label":`,
		`{"label":7}`,
	} {
		if code, raw := f.labelPost(t, "livesession", body); code != http.StatusBadRequest {
			t.Errorf("label %s = %d %s, want 400", body, code, raw)
		}
	}
	if code, _, _ := postJSON(t, f.base+"/s/livesession/label", f.base, `{"label":"`+strings.Repeat("x", labelBodyLimit)+`"}`); code != http.StatusRequestEntityTooLarge {
		t.Errorf("oversized body = %d, want 413", code)
	}
	if got := readSessionLabel(SessionDir(f.dir, "livesession")); got != "kept" {
		t.Fatalf("a refused label changed the stored one to %q", got)
	}
	full := strings.Repeat("가", 64)
	if code, raw := f.labelPost(t, "livesession", `{"label":"`+full+`"}`); code != http.StatusOK {
		t.Fatalf("64-character label = %d %s", code, raw)
	}
	if got := readSessionLabel(SessionDir(f.dir, "livesession")); got != full {
		t.Fatalf("stored label = %q", got)
	}

	// A file no route wrote (hand-edited, or invalid) reads as no label.
	dir := SessionDir(f.dir, "livesession")
	for _, raw := range []string{"not json", `{"label":"a\nb"}`, `{"label":"` + strings.Repeat("x", 65) + `"}`} {
		if err := os.WriteFile(filepath.Join(dir, labelFileName), []byte(raw), 0o600); err != nil {
			t.Fatal(err)
		}
		if got := readSessionLabel(dir); got != "" {
			t.Errorf("label file %q reads as %q", raw, got)
		}
	}
}

// TestLabelRouteGate pins the route's gate, the board-answer write routes'
// own: a POST, the daemon's own Origin (403 without it), a session the index
// holds (404 otherwise). No refused request writes a file, and no answer
// carries a CORS header.
func TestLabelRouteGate(t *testing.T) {
	f := startPageFixture(t)
	for _, origin := range []string{"", "null", "http://evil.example", "https://" + strings.TrimPrefix(f.base, "http://"), "http://localhost:1"} {
		code, header, _ := postJSON(t, f.base+"/s/livesession/label", origin, `{"label":"x"}`)
		if code != http.StatusForbidden {
			t.Errorf("label with Origin %q = %d, want 403", origin, code)
		}
		assertNoCORS(t, header)
	}
	for _, id := range []string{"nosuchsession", "Bad-ID"} {
		if code, _ := f.labelPost(t, id, `{"label":"x"}`); code != http.StatusNotFound {
			t.Errorf("label of %q = %d, want 404", id, code)
		}
	}
	if code, _ := get(t, f.base+"/s/livesession/label"); code != http.StatusMethodNotAllowed {
		t.Errorf("GET label = %d, want 405", code)
	}
	if _, err := os.Stat(filepath.Join(SessionDir(f.dir, "livesession"), labelFileName)); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("a refused request wrote the label file: %v", err)
	}
	if _, err := os.Stat(SessionDir(f.dir, "nosuchsession")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("a refused request made a session directory: %v", err)
	}
}

// labelJSPrelude is a minimal DOM (class selectors, closest, insertBefore,
// removeChild, hidden, listeners) and the page helpers the head's rename
// block calls, recorded: copyText, post (held until the test answers it),
// toast, wsT, swap, and tlRebase.
const labelJSPrelude = `
var bad = [];
function check(what, got, want) { if (JSON.stringify(got) !== JSON.stringify(want)) bad.push(what + ": got " + JSON.stringify(got) + ", want " + JSON.stringify(want)); }
function El(tag, cls, attrs) {
  this.tagName = tag.toUpperCase(); this.className = cls || ""; this.attrs = attrs || {}; this.children = []; this.parentNode = null;
  this.hidden = false; this.textContent = ""; this.value = ""; this.on = {}; this.focused = 0;
  var self = this;
  this.classList = { toggle: function (c, on) { var l = self.className.split(" ").filter(function (x) { return x && x !== c; }); if (on) l.push(c); self.className = l.join(" "); } };
}
El.prototype.has = function (c) { return (" " + this.className + " ").indexOf(" " + c + " ") >= 0; };
El.prototype.getAttribute = function (k) { return k in this.attrs ? this.attrs[k] : null; };
El.prototype.setAttribute = function (k, v) { this.attrs[k] = String(v); };
El.prototype.querySelector = function (sel) {
  var c = sel.replace(/\[.*\]$/, "").slice(1), attr = (sel.match(/\[(.*)\]$/) || [])[1];
  for (var i = 0; i < this.children.length; i++) {
    var k = this.children[i];
    if (k.has(c) && (!attr || k.getAttribute(attr) !== null)) return k;
    var deep = k.querySelector(sel); if (deep) return deep;
  }
  return null;
};
El.prototype.closest = function (sel) { for (var n = this; n; n = n.parentNode) if (n.has && n.has(sel.slice(1))) return n; return null; };
El.prototype.contains = function (n) { for (; n; n = n.parentNode) if (n === this) return true; return false; };
El.prototype.appendChild = function (c) { return this.insertBefore(c, null); };
El.prototype.insertBefore = function (c, ref) { var i = ref ? this.children.indexOf(ref) : -1; if (i < 0) this.children.push(c); else this.children.splice(i, 0, c); c.parentNode = this; return c; };
// Removing the focused element fires its blur first, as a browser does,
// while it is still in place; a second removal of it throws.
El.prototype.removeChild = function (c) {
  var i = this.children.indexOf(c);
  if (i < 0) throw new Error("removeChild: not a child");
  if (c.hasFocus) { c.hasFocus = false; c.fire("blur"); i = this.children.indexOf(c); if (i < 0) throw new Error("removeChild: not a child"); }
  this.children.splice(i, 1); c.parentNode = null; return c;
};
Object.defineProperty(El.prototype, "isConnected", { get: function () { for (var n = this; n; n = n.parentNode) if (n === H) return true; return false; } });
El.prototype.addEventListener = function (t, fn) { (this.on[t] = this.on[t] || []).push(fn); };
El.prototype.fire = function (t, e) { e = e || {}; e.type = t; e.preventDefault = function () { e.prevented = true; }; e.stopPropagation = function () {}; (this.on[t] || []).slice().forEach(function (fn) { fn(e); }); return e; };
El.prototype.focus = function () { this.focused++; this.hasFocus = true; };
El.prototype.select = function () {};
var doc = { createElement: function (t) { return new El(t); } };
var root = new El("html", "", { "data-rid": "sess" });
var H = new El("span", "r-head");
var sn = H.appendChild(new El("span", "meta sname", { "data-label": "", "data-max": "64", "data-title": "sess - ws dashboard" }));
var nm = sn.appendChild(new El("button", "snm", { "data-copy": '<mailbox id="id:sess" dir="/p"/>', "data-mailbox-open": '<mailbox id="id:sess" dir="/p"' }));
nm.textContent = "sess";
var pen = sn.appendChild(new El("button", "sren", { "aria-label": "Rename" }));
var penIcon = pen.appendChild(new El("svg", "i"));
function region(name) { return name === "head" ? H : null; }
var copied = [], posts = [], toasts = [], swaps = [], titles = [], answer = null;
function copyText(s) { copied.push(s); }
function post(route, body, done) { posts.push([route, body]); answer = done; }
function toast(t) { toasts.push(t); }
function wsT(k, a) { return k + ":" + a; }
function swap(name, hash) { swaps.push([name, hash]); }
function tlRebase(b) { titles.push(b); }
var headHeld = null;
function inp() { return sn.querySelector(".sinp"); }
function key(k) { return inp().fire("keydown", { key: k }); }
`

const labelJSCases = `
// The name copies the mailbox address the server built.
labelClick(nm);
check("a click on the name copies the address", copied, ['<mailbox id="id:sess" dir="/p"/>']);
check("and opens no input", inp(), null);

// The pencil (or its icon) opens the input in place of the name.
labelClick(penIcon);
var i1 = inp();
check("the pencil opens the input", [!!i1, labelEditing(), nm.hidden, pen.hidden], [true, true, true, true]);
check("the input's value, placeholder, limit, and name", [i1.value, i1.placeholder, i1.maxLength, i1.getAttribute("aria-label"), i1.focused > 0], ["", "sess", 64, "Rename", true]);
check("it stands where the name was", sn.children.indexOf(i1), 0);
labelClick(penIcon);
check("a second click opens no second input", sn.children.filter(function (c) { return c.has("sinp"); }).length, 1);

// Enter saves the trimmed value; a blur while it saves does not cancel.
i1.value = "  Build bot  ";
check("Enter is taken", key("Enter").prevented, true);
check("Enter posts the trimmed label", posts, [["label", { label: "Build bot" }]]);
i1.fire("blur");
check("a blur while saving keeps the input", inp() === i1, true);
key("Enter");
check("a second Enter while saving posts nothing", posts.length, 1);
answer(true, { label: "Build bot" });
check("saved: the name shows the label", [nm.textContent, sn.getAttribute("data-label"), inp(), nm.hidden, pen.hidden, labelEditing()], ["Build bot", "Build bot", null, false, false, false]);
check("saved: the name takes the label's body face", nm.has("lbl"), true);
check("saved: the name copies the address naming the label", nm.getAttribute("data-copy"), '<mailbox id="id:sess" dir="/p">Build bot</mailbox>');

// Escape cancels: nothing posted, the name kept.
labelClick(pen);
check("the input opens on the current label", inp().value, "Build bot");
inp().value = "something else";
check("Escape is taken", key("Escape").prevented, true);
check("Escape cancels", [posts.length, inp(), nm.textContent], [1, null, "Build bot"]);

// Leaving the input cancels as well.
labelClick(pen);
inp().value = "typed";
inp().hasFocus = false;
inp().fire("blur");
check("a blur cancels", [posts.length, inp(), nm.textContent], [1, null, "Build bot"]);

// An unchanged value closes without a post.
labelClick(pen);
key("Enter");
check("an unchanged label posts nothing", [posts.length, inp()], [1, null]);

// A refusal toasts why and keeps the input open.
labelClick(pen);
inp().value = "refused";
key("Enter");
answer(false, "a label holds at most 64 characters");
check("a refusal toasts", toasts, ["js.label.fail:a label holds at most 64 characters"]);
check("and keeps the input", [!!inp(), inp().value, nm.textContent], [true, "refused", "Build bot"]);
key("Escape");

// An empty value resets to the reply-id.
labelClick(pen);
inp().value = "   ";
key("Enter");
check("empty posts an empty label", posts[posts.length - 1], ["label", { label: "" }]);
answer(true, { label: "" });
check("reset: the name shows the reply-id", [nm.textContent, sn.getAttribute("data-label"), inp()], ["sess", "", null]);
check("reset: the reply-id keeps the mono face", nm.has("lbl"), false);
check("reset: the name copies the bare address", nm.getAttribute("data-copy"), '<mailbox id="id:sess" dir="/p"/>');

labelClick(pen);
inp().value = 'a" & <b>';
key("Enter");
answer(true, { label: 'a" & <b>' });
check("saved: label body is escaped", nm.getAttribute("data-copy"), '<mailbox id="id:sess" dir="/p">a" &amp; &lt;b&gt;</mailbox>');

// A head swap held while the input was open lands when it closes.
labelClick(pen);
headHeld = { hash: "h2", html: "<b>new head</b>" };
key("Escape");
check("the held head swap lands on close", [swaps, headHeld], [[["head", "h2"]], null]);
labelClick(pen);
inp().fire("keydown", { key: "Enter", isComposing: true });
check("an IME Enter neither saves nor closes", [posts.length, !!inp()], [4, true]);
key("Escape");

// The value the route stores: one line, trimmed, at most max characters.
check("control characters become spaces", labelClean("a\nb\tc\u2028d", 64), "a b c d");
check("trimmed", labelClean("  x  ", 64), "x");
check("cut at max characters, not code units", labelClean("\ud83d\ude00\ud83d\ude00\ud83d\ude00", 2), "\ud83d\ude00\ud83d\ude00");
check("a cut never leaves a trailing space", labelClean("ab cd", 3), "ab");

// A head swap retitles the tab from the head's name.
labelSwapped();
check("the tab title follows the head", titles, ["sess - ws dashboard"]);
process.stdout.write(bad.join("\n"));
`

// TestHeadRenameScript pins the head's rename (dashboard.js labelClick,
// labelStart, labelKey, labelSave, labelEnd): the name copies the mailbox
// tag, which a save or reset completes from the server's opening attributes;
// the pencil opens a one-line input; Enter saves the trimmed value
// through the label route, Escape and a blur cancel, an empty value resets
// to the reply-id, the name carries lbl (the label's body face) exactly
// while it shows a label, a refusal keeps the input open, and a head swap held
// while it was open lands when it closes. The cases execute under node when
// it is installed.
func TestHeadRenameScript(t *testing.T) {
	js := mustAsset(t, "dashboard.js")
	var src string
	for _, name := range []string{"labelEditing", "labelClean", "labelClick", "labelHead", "labelShow", "labelCopy", "labelStart", "labelKey", "labelSave", "labelEnd", "labelSwapped"} {
		f := regexp.MustCompile(`(?s)\n  function ` + name + `\(.*?\n  \}\n`).FindString(js)
		if f == "" {
			t.Fatalf("dashboard.js holds no %s", name)
		}
		src += f
	}
	for _, hook := range []string{
		`if (labelEditing()) { headHeld = { hash: hash, html: html }; return; }`,
		`if (name === "head") labelSwapped();`,
	} {
		if !strings.Contains(js, hook) {
			t.Errorf("dashboard.js swap lacks %q", hook)
		}
	}
	runNode(t, "head rename", labelJSPrelude+src+labelJSCases)
}

// TestSessionLabelBodyFace pins the manual label's face: a session with a
// label marks its name lbl (the body face, dashboard.css) in the head, the
// rail row, and the sessions index row; a bare reply-id keeps the mono face.
func TestSessionLabelBodyFace(t *testing.T) {
	cases := []struct {
		name          string
		labeled, bare any
		mark          string
	}{
		{"sname", SessionView{ReplyID: "s", Label: "빌드 봇"}, SessionView{ReplyID: "s"}, `class="snm lbl"`},
		{"rrow", RailRow{ReplyID: "s", Label: "빌드 봇", Live: true}, RailRow{ReplyID: "s", Live: true}, `class="rid lbl"`},
		{"srow", IndexRow{ReplyID: "s", Label: "빌드 봇", Live: true}, IndexRow{ReplyID: "s", Live: true}, `class="rid lbl"`},
	}
	for _, c := range cases {
		if got := renderNamed(t, c.name, c.labeled); !strings.Contains(got, c.mark) || !strings.Contains(got, "빌드 봇") {
			t.Errorf("%s with a label lacks %s:\n%s", c.name, c.mark, got)
		}
		if got := renderNamed(t, c.name, c.bare); strings.Contains(got, " lbl") {
			t.Errorf("%s without a label carries lbl:\n%s", c.name, got)
		}
	}
	css := mustAsset(t, "dashboard.css")
	for _, sel := range []string{".sname .snm.lbl {", ".sess .rid.lbl {", ".rail .rid.lbl {"} {
		i := strings.Index(css, sel)
		if i < 0 || !strings.Contains(css[i:i+strings.Index(css[i:], "}")], "font-family: var(--wb-sans); font-weight: 600;") {
			t.Errorf("dashboard.css: %s does not set the body face at 600", sel)
		}
	}
}

// TestMailboxRef pins attributes, omission rules, and XML escaping.
func TestMailboxRef(t *testing.T) {
	for _, c := range []struct {
		view SessionView
		want string
	}{
		{SessionView{ReplyID: "sess", Project: "/repo/sub", Harness: "pi", Label: `a" & <b>`, Git: &GitBadge{TopLevel: "/repo", Repo: wslive.Repo{Branch: "main"}}}, `<mailbox id="id:sess" dir="/repo/sub" root="/repo" branch="main" harness="pi">a" &amp; &lt;b&gt;</mailbox>`},
		{SessionView{ReplyID: "sess", Project: `/a"&<b>`, Git: &GitBadge{TopLevel: `/a"&<b>`, Repo: wslive.Repo{Branch: "abc", Detached: true}}}, `<mailbox id="id:sess" dir="/a&quot;&amp;&lt;b&gt;"/>`},
		{SessionView{ReplyID: "sess", Project: "/repo/.", Git: &GitBadge{TopLevel: "/repo"}}, `<mailbox id="id:sess" dir="/repo/."/>`},
	} {
		if got := c.view.MailboxRef(); got != c.want {
			t.Errorf("MailboxRef() = %q, want %q", got, c.want)
		}
	}
}

func TestMailboxBadgeTopLevel(t *testing.T) {
	root := t.TempDir()
	gitRun(t, root, "init", "-q")
	gitRun(t, root, "-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "--allow-empty", "-qm", "initial")
	sub := filepath.Join(root, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	badge := readGitBadge(sub)
	// git reports the resolved path; t.TempDir may sit behind a symlink (macOS /var).
	want, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := filepath.EvalSymlinks(badge.TopLevel); err != nil || got != want {
		t.Fatalf("badge toplevel = %q, want %q (err %v)", badge.TopLevel, want, err)
	}
}

// labelledPeers is the sessions a labelled-peer view looks up: "named" with
// a label that must be escaped, "plain" with none.
func labelledPeers(now time.Time) map[string]Session {
	named := liveSession("named", "/q", now)
	named.Label = `<b>메인 & "x"</b>`
	return map[string]Session{"named": named, "plain": liveSession("plain", "/r", now)}
}

// TestMailRowsShowPeerLabels pins the conversation's mail rows: a served
// peer named by its id: address shows its label, HTML-escaped, with the
// address in the tooltip; an unlabelled or unserved peer shows its address,
// and a named address stays the name even when its owner has a label. A
// rename re-renders the cached row.
func TestMailRowsShowPeerLabels(t *testing.T) {
	now := testNow
	mail := func(name, peer string) wslive.RingRecord {
		return wslive.RingRecord{Name: name, At: now, RingEntry: wslive.RingEntry{Kind: wslive.RingMail, Direction: wslive.MailRecv, Peer: peer, Title: "hi"}}
	}
	ring := []wslive.RingRecord{mail("m1", "id:named"), mail("m2", "id:plain"), mail("m3", "lead@machine"), mail("m4", "id:gone")}
	cache := newEntryCache()
	render := func(sessions map[string]Session) string {
		v := buildSessionView(pageInputs{Session: liveSession("self", "/p", now), Snapshot: liveSnap(now, wslive.Lead{}), Ring: ring, Now: now,
			Page:   PageContent{Contacts: []Contact{{Address: "lead@machine", ReplyID: "named", LastExchange: now}}},
			Lookup: lookupOf(sessions, nil)})
		set, err := renderSessionRegions(v, cache)
		if err != nil {
			t.Fatal(err)
		}
		return string(set[regionEntries].HTML)
	}
	sessions := labelledPeers(now)
	html := render(sessions)
	for _, want := range []string{
		`<a class="mpeer" href="/s/named" title="id:named">&lt;b&gt;메인 &amp; &#34;x&#34;&lt;/b&gt;</a>`,
		`<a class="mpeer" href="/s/plain">id:plain</a>`,
		`<a class="mpeer" href="/s/named">lead@machine</a>`,
		`<span class="mpeer">id:gone</span>`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("mail rows lack %s:\n%s", want, html)
		}
	}
	if strings.Contains(html, "<b>메인") {
		t.Errorf("a label rendered unescaped:\n%s", html)
	}

	renamed := sessions["named"]
	renamed.Label = "빌드 봇"
	sessions["named"] = renamed
	if html := render(sessions); !strings.Contains(html, `title="id:named">빌드 봇</a>`) {
		t.Errorf("a rename did not re-render the cached row:\n%s", html)
	}
}

// TestPeerRowsShowPeerLabels pins the agents panel's peer rows: a served
// peer known by its id: address shows its label, HTML-escaped, with the
// address and folder in the tooltip; an unlabelled peer shows its address
// with the folder alone in the tooltip; a peer reached by a named address
// keeps the name.
func TestPeerRowsShowPeerLabels(t *testing.T) {
	now := testNow
	sessions := labelledPeers(now)
	byName := liveSession("byname", "/s", now)
	byName.Label = "unused"
	sessions["byname"] = byName
	v := buildSessionView(pageInputs{Session: liveSession("self", "/p", now), Snapshot: liveSnap(now, wslive.Lead{}), Now: now,
		Page: PageContent{Contacts: []Contact{
			{Address: "id:named", ReplyID: "named", LastExchange: now},
			{Address: "id:plain", ReplyID: "plain", LastExchange: now.Add(-time.Minute)},
			{Address: "lead@machine", ReplyID: "byname", LastExchange: now.Add(-2 * time.Minute)},
		}},
		Lookup: lookupOf(sessions, nil)})
	html := renderNamed(t, "peers", v.Tree.Peers)
	for _, want := range []string{
		`<a class="pname" href="/s/named" title="id:named` + "\n" + `/q"><span class="pfolder">q/</span>&lt;b&gt;메인 &amp; &#34;x&#34;&lt;/b&gt;</a>`,
		`<a class="pname" href="/s/plain" title="/r"><span class="pfolder">r/</span>id:plain</a>`,
		`<a class="pname" href="/s/byname" title="/s"><span class="pfolder">s/</span>lead@machine</a>`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("peer rows lack %s:\n%s", want, html)
		}
	}
	if strings.Contains(html, "unused") {
		t.Errorf("a named address gave way to its owner's label:\n%s", html)
	}
}
