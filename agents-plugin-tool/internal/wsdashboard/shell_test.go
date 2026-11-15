package wsdashboard

import (
	"net/http"
	"path/filepath"
	"strings"
	"testing"
)

// A session page's URL serves the iframe shell: the session rail and the
// header, and a frame loading the page itself with ?frame=1. The page in the
// frame holds the session's regions under a placeholder for the rail, and no
// rail of its own.
func TestSessionURLServesTheShellAroundItsFramedPage(t *testing.T) {
	f := startPageFixture(t)
	code, shell := get(t, f.base+"/s/livesession")
	if code != http.StatusOK {
		t.Fatalf("GET /s/livesession = %d", code)
	}
	for _, want := range []string{
		`data-mode="shell" data-shell="1" data-rid="livesession"`,
		`<aside id="rail" class="rail"`, `data-region="rail"`, `href="/s/boardsession"`,
		`data-region="head"`, `id="railbtn"`, `id="qsslot"`,
		`<iframe class="sframe on" src="/s/livesession?frame=1"`,
		"window.WsShell", "window.WsRail",
	} {
		if !strings.Contains(shell, want) {
			t.Errorf("the shell lacks %q", want)
		}
	}
	for _, not := range []string{`data-region="entries"`, `data-region="board"`, `id="convo"`, `data-frame="1"`} {
		if strings.Contains(shell, not) {
			t.Errorf("the shell holds the page's own %q", not)
		}
	}
	// The rail's stored marks apply before it shows: rail.js loads in the
	// head, and the inline script right after the rail lifts data-premark.
	head, body, _ := strings.Cut(shell, "</head>")
	if !strings.Contains(head, "window.WsRail") {
		t.Error("rail.js does not load in the shell's head")
	}
	_, afterRail, _ := strings.Cut(body, `data-premark>`)
	if i := strings.Index(afterRail, "</aside><script>"); i < 0 || !strings.Contains(afterRail[i:i+600], "R.marks(") {
		t.Error("no inline script applies the rail's marks right after the rail")
	}

	code, page := get(t, f.base+"/s/livesession?frame=1")
	if code != http.StatusOK {
		t.Fatalf("GET /s/livesession?frame=1 = %d", code)
	}
	for _, want := range []string{`data-frame="1"`, `class="rail rail-ph"`, `data-region="entries"`, `data-region="head"`, "window.WsShell"} {
		if !strings.Contains(page, want) {
			t.Errorf("the framed page lacks %q", want)
		}
	}
	for _, not := range []string{`data-region="rail"`, `<aside id="rail"`, `data-shell="1"`} {
		if strings.Contains(page, not) {
			t.Errorf("the framed page holds the shell's %q", not)
		}
	}
	// An unknown session is not found, framed or not.
	for _, path := range []string{"/s/nosuchsession", "/s/nosuchsession?frame=1"} {
		if code, body := get(t, f.base+path); code != http.StatusNotFound || strings.Contains(body, "<iframe") {
			t.Errorf("GET %s = %d, want a 404 with no shell", path, code)
		}
	}
}

// The root serves the ordinary shell with no selected session. Its frame
// keeps the session geometry but has an empty main panel and header.
func TestIndexServesTheShellAroundItsFramedPage(t *testing.T) {
	f := startPageFixture(t)
	code, shell := get(t, f.base+"/")
	if code != http.StatusOK {
		t.Fatalf("GET / = %d", code)
	}
	for _, want := range []string{
		`data-mode="shell" data-shell="1" data-rid=""`, `class="top"`,
		`<aside id="rail" class="rail"`, `data-region="rail"`, `id="railbtn"`,
		`<iframe class="sframe on" src="/?frame=1"`, "window.WsShell", "window.WsRail",
	} {
		if !strings.Contains(shell, want) {
			t.Errorf("the index shell lacks %q", want)
		}
	}
	for _, not := range []string{`data-region="sessions"`, `data-frame="1"`} {
		if strings.Contains(shell, not) {
			t.Errorf("the index shell holds %q", not)
		}
	}
	// The head slot a session frame fills stays empty.
	if !strings.Contains(shell, `data-region="head" data-h=""></div>`) {
		t.Error("the index shell's head slot is not empty")
	}

	code, page := get(t, f.base+"/?frame=1")
	if code != http.StatusOK {
		t.Fatalf("GET /?frame=1 = %d", code)
	}
	for _, want := range []string{
		`data-mode="index" data-rid=""`, `data-frame="1"`, `class="dwrap"`, `id="railbtn"`,
		`class="rail rail-ph"`, `<main id="grid" class="dgrid"><div class="dlanding" id="guide">`, "window.WsShell",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("the framed index lacks %q", want)
		}
	}
	for _, not := range []string{`data-region="rail"`, `<aside id="rail"`, `data-shell="1"`, `dwrap narrow`, `data-region="sessions"`, `<h1>ws dashboard</h1>`} {
		if strings.Contains(page, not) {
			t.Errorf("the framed index holds %q", not)
		}
	}
}

// pageFraming reads a page's framing headers.
func pageFraming(t *testing.T, url string) (status int, xfo, csp string) {
	t.Helper()
	resp, err := testClient().Get(url)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	return resp.StatusCode, resp.Header.Get("X-Frame-Options"), resp.Header.Get("Content-Security-Policy")
}

// TestPagesRefuseFraming pins the clickjacking guard: every daemon HTML page
// refuses to be framed, since a framed page's posts pass the same-origin
// guards, but for the pages one of the daemon's own pages frames, which
// only the daemon's own origin may frame. sameOrigin lists those; a route
// some daemon page frames joins it.
func TestPagesRefuseFraming(t *testing.T) {
	f := startPageFixture(t)
	writeFile(t, filepath.Join(f.project, "a.md"), "# A\n")
	writeFile(t, filepath.Join(f.project, "blob.bin"), "BIN\x00")
	// Each path answers its status: a session page, or the finding page
	// (404) of a session not registered yet.
	refused := map[string]int{
		"/": http.StatusOK, "/s/livesession": http.StatusOK, "/s/boardsession": http.StatusOK,
		"/s/livesession/file?path=a.md": http.StatusOK, "/s/livesession/file?path=blob.bin": http.StatusOK,
		"/s/livesession/file?path=a.md&frame=1": http.StatusOK, "/s/newlead": http.StatusNotFound,
	}
	sameOrigin := map[string]int{
		// The iframe shell's frames: the index, a session page, and the
		// finding page a frame shows before its session registers.
		"/?frame=1": http.StatusOK, "/s/livesession?frame=1": http.StatusOK, "/s/boardsession?frame=1": http.StatusOK, "/s/newlead?frame=1": http.StatusNotFound,
	}
	for path, want := range refused {
		status, xfo, csp := pageFraming(t, f.base+path)
		if status != want {
			t.Errorf("GET %s = %d, want %d", path, status, want)
			continue
		}
		if xfo != "DENY" || !strings.Contains(csp, "frame-ancestors 'none'") {
			t.Errorf("GET %s is frameable: X-Frame-Options %q, CSP %q", path, xfo, csp)
		}
	}
	for path, want := range sameOrigin {
		status, xfo, csp := pageFraming(t, f.base+path)
		if status != want {
			t.Errorf("GET %s = %d, want %d", path, status, want)
			continue
		}
		if xfo != "SAMEORIGIN" || !strings.Contains(csp, "frame-ancestors 'self'") || strings.Contains(csp, "'none'") {
			t.Errorf("GET %s: X-Frame-Options %q, CSP %q, want only its own origin to frame it", path, xfo, csp)
		}
	}
}

// TestSessionPageFramesOnlyItsOwnOrigin pins the session page's frame-src,
// the shell's and the framed page's alike: a frame on it (the shell's
// frames, a preview's sandboxed document) cannot navigate itself to another
// origin, where it would carry off its capability.
func TestSessionPageFramesOnlyItsOwnOrigin(t *testing.T) {
	f := startPageFixture(t)
	for path, want := range map[string]string{
		"/s/livesession":         "frame-src 'self'; frame-ancestors 'none'",
		"/s/livesession?frame=1": "frame-src 'self'; frame-ancestors 'self'",
	} {
		if status, _, csp := pageFraming(t, f.base+path); status != http.StatusOK || csp != want {
			t.Errorf("GET %s = %d, policy %q, want %q", path, status, csp, want)
		}
	}
}

// The shell streams the rail alone (/rail/events), and a page it frames
// streams its own regions without the rail, which the session page standing
// alone carries over its own stream.
func TestShellStreamsTheRailAndItsFramesTheirRegions(t *testing.T) {
	f := startPageFixture(t)
	rail := openStream(t, f.base+railEventsRoute)
	framed := openStream(t, f.base+"/s/livesession/events?frame=1")
	alone := openStream(t, f.base+"/s/livesession/events")
	first := quiet(t, rail)
	if len(first) == 0 {
		t.Fatal("the rail stream sent no first rail")
	}
	for _, ev := range first {
		if ev.Name != regionRail {
			t.Errorf("the rail stream sent %s", ev.Name)
		}
	}
	for _, ev := range quiet(t, framed) {
		if ev.Name == regionRail {
			t.Error("a framed page's stream sent the rail")
		}
	}
	quiet(t, alone)

	f.addSession(t, "lonersession", t.TempDir(), "alone")
	waitRegionHolding(t, rail, regionRail, `data-rid="lonersession"`)
	waitRegionHolding(t, alone, regionRail, `data-rid="lonersession"`)
	for _, ev := range quiet(t, framed) {
		if ev.Name == regionRail {
			t.Error("a framed page's stream sent the rail on a new session")
		}
	}
	for _, ev := range quiet(t, rail) {
		if ev.Name != regionRail {
			t.Errorf("the rail stream sent %s", ev.Name)
		}
	}
}

// shellJSCases runs shell.js under node: its message check, the URLs the
// shell frames (the index and the session pages), the frames' recency
// order, and the backdrop alpha.
const shellJSCases = `
var S = window.WsShell, O = "http://h:1";
function U(s) { return new URL(s, O); }
check("a known kind with its fields", !!S.message({ wsShell: "head", html: "<b>", qs: "", pad: 0, top: 50 }), true);
check("a nullable field takes null", !!S.message({ wsShell: "head", html: null, qs: "", pad: 0, top: 50 }), true);
check("a field of the wrong type", S.message({ wsShell: "layout", pincol: "yes", dim: 0 }), null);
check("a missing field", S.message({ wsShell: "nav" }), null);
check("an unknown kind", S.message({ wsShell: "eval", code: "x" }), null);
check("no kind", [S.message(null), S.message("head"), S.message({ wbDoc: "rail" })], [null, null, null]);
check("a field-less kind", !!S.message({ wsShell: "sync" }), true);
check("a session page", S.framed(U("/s/abc123#files"), O), { path: "/s/abc123", hash: "#files", rid: "abc123" });
check("a framed page's own URL", S.framed(U("/s/abc123?frame=1"), O), { path: "/s/abc123", hash: "", rid: "abc123" });
check("another origin", S.framed(new URL("http://evil:1/s/abc123"), O), null);
check("a query the shell does not pass on", S.framed(U("/s/abc123?x=1"), O), null);
check("a session's sub-route", S.framed(U("/s/abc123/file?path=a"), O), null);
check("a malformed reply-id", S.framed(U("/s/ABC"), O), null);
check("the index", [S.framed(U("/#/team"), O), S.framed(U("/?frame=1"), O)], [{ path: "/", hash: "#/team", rid: "" }, { path: "/", hash: "", rid: "" }]);
check("Unix worktree has no session identity", S.framed(U("/w/Users/me/work%20tree?frame=1"), O), { path: "/w/Users/me/work%20tree", hash: "", rid: "" });
check("Windows worktree preserves its drive", S.framed(U("/w/C:/work/repo#file"), O), { path: "/w/C:/work/repo", hash: "#file", rid: "" });
check("worktrees keep the shell origin and query guards", [S.framed(new URL("http://evil:1/w/home/me"), O), S.framed(U("/w/home/me?root=other"), O), S.framed(U("/w/"), O)], [null, null, null]);
check("a route the shell does not frame", S.framed(U("/tracker/stores/x/view"), O), null);
check("src", S.src({ path: "/s/abc123", hash: "#files" }), "/s/abc123?frame=1#files");
var t = S.touch([], "a");
check("the first frame", t, { order: ["a"], evict: [] });
t = S.touch(["e", "d", "c", "b", "a"], "f");
check("a sixth frame evicts the least recently shown", t, { order: ["f", "e", "d", "c", "b"], evict: ["a"] });
t = S.touch(["e", "d", "c", "b", "a"], "a");
check("showing a kept frame evicts none", t, { order: ["a", "e", "d", "c", "b"], evict: [] });
check("a given limit", S.touch(["b", "a"], "c", 2), { order: ["c", "b"], evict: ["a"] });
check("LIMIT", S.LIMIT, 5);
check("alpha", [S.alpha("rgb(0, 0, 0)"), S.alpha("rgba(0, 0, 0, 0.4)"), S.alpha("rgb(0 0 0 / 50%)"), S.alpha("transparent"), S.alpha("")], [1, 0.4, 0.5, 0, 0]);
if (bad.length) console.log(bad.join("\n"));
`

func TestShellScript(t *testing.T) {
	runNode(t, "shell.js", "var window = {};\nvar bad = [];\nfunction check(what, got, want) { if (JSON.stringify(got) !== JSON.stringify(want)) bad.push(what + \": got \" + JSON.stringify(got) + \", want \" + JSON.stringify(want)); }\n"+
		mustAsset(t, "shell.js")+shellJSCases)
}

// railMarksHarness extends railJSHarness's DOM with what marks reads: tag
// names, querySelectorAll and closest over simple selectors
// (tag.class[attr]), and details' open.
const railMarksHarness = `
function matches(el, sel) {
  var m = /^([a-z]*)((?:\.[a-z-]+)*)((?:\[[a-z-]+\])*)$/.exec(sel);
  if ((m[1] && el.tag !== m[1])) return false;
  var cls = m[2].split(".").filter(Boolean), at = (m[3].match(/[a-z-]+/g) || []);
  return cls.every(function (c) { return el.classList.contains(c); }) && at.every(function (a) { return el.getAttribute(a) !== null; });
}
El.prototype.querySelectorAll = function (sel) {
  var out = [];
  (function walk(n) { n.children.forEach(function (k) { if (matches(k, sel)) out.push(k); walk(k); }); })(this);
  return out;
};
El.prototype.querySelector = function (sel) { return this.querySelectorAll(sel)[0] || null; };
El.prototype.closest = function (sel) { for (var n = this; n; n = n.parentNode) if (n.classList && matches(n, sel)) return n; return null; };
function tagged(tag, cls, attrs) { var e = new El(cls, attrs); e.tag = tag; return e; }
function ws(path, rids, ended) {
  var s = tagged("section", "rws", { "data-ws": path }), tog = tagged("button", "wstog", {});
  s.appendChild(tog);
  var box = tagged("div", "rsessions", {});
  rids.forEach(function (r) { var x = row(r); x.tag = "div"; box.appendChild(x); });
  s.appendChild(box);
  if (ended) {
    // The fold row: an archived half (no archived rows here, so disabled)
    // and an ended half over its list.
    var f = tagged("div", "rfold", { "data-key": path });
    var ha = tagged("button", "rfh", { "data-k": "arch", "aria-expanded": "false" }); ha.disabled = true;
    var hd = tagged("button", "rfh", { "data-k": "del", "aria-expanded": "false" });
    f.appendChild(ha); f.appendChild(hd); box.appendChild(f);
    var d = tagged("div", "rended", { "data-k": "del" });
    var eb = tagged("div", "rsessions", {});
    ended.forEach(function (r) { var x = row(r); x.tag = "div"; x.classList.add("ended"); eb.appendChild(x); });
    d.appendChild(eb); box.appendChild(d);
  }
  return s;
}
function rids(box) { return box.children.filter(function (k) { return k.classList.contains("rs"); }).map(function (k) { return k.getAttribute("data-rid") + (k.classList.contains("cur") ? "*" : ""); }); }
`

const railMarksCases = `
var R = window.WsRail, st = memStore();
var r = tagged("div", "r-rail", {}), list = tagged("div", "rlist", {});
r.appendChild(list);
var a = ws("/a", ["a1", "a2", "a3"], ["a9", "a8"]), b = ws("/b", ["b1"]);
list.appendChild(a); list.appendChild(b);
st.save("ws.dashboard.stars", JSON.stringify(["a3"]));
st.save("ws.dashboard.railorder", JSON.stringify(["a2", "a1", "a3", "a8", "a9"]));
st.save("ws.dashboard.wsorder", JSON.stringify(["/b", "/a"]));
st.save("ws.dashboard.railcol", JSON.stringify(["/b"]));
st.save("ws.dashboard.railended", JSON.stringify(["rended:/a"]));
st.save("ws.dashboard.railkids", JSON.stringify(["a2"]));
var abox = a.children[1], ended = abox.children[abox.children.length - 1];
var a2 = abox.children[1], kt = tagged("button", "kidtog", {});
a2.appendChild(kt);
var s = R.state(st);
R.marks(r, s, "a1");
check("the rows and workspaces keep the server's order (the retired local stars and orders move nothing); the viewed row marked", rids(abox), ["a1*", "a2", "a3"]);
check("an ended fold keeps the server's order and stays last", [rids(ended.children[0]), ended.getAttribute("data-k")], [["a9", "a8"], "del"]);
check("the workspaces in the server's order", list.children.map(function (w) { return w.getAttribute("data-ws"); }), ["/a", "/b"]);
check("the collapsed parent", [a2.classList.contains("kcol"), kt.getAttribute("aria-expanded")], [true, "false"]);
check("the collapsed workspace", [b.classList.contains("col"), b.children[0].getAttribute("aria-expanded"), a.classList.contains("col"), a.children[0].getAttribute("aria-expanded")], [true, "false", false, "true"]);
var fold = abox.children[abox.children.length - 2], halfA = fold.children[0], halfD = fold.children[1];
check("the ended fold left open", [ended.classList.contains("on"), halfD.getAttribute("aria-expanded"), halfA.getAttribute("aria-expanded")], [true, "true", "false"]);
s.ended.put("rended:/a", false);
s.collapsed.toggle("/b");
s.kids.toggle("a2");
R.marks(r, R.state(st), "b1");
check("a closed fold, an expanded workspace and parent", [ended.classList.contains("on"), b.classList.contains("col"), rids(b.children[1]), rids(abox), a2.classList.contains("kcol"), kt.getAttribute("aria-expanded")], [false, false, ["b1*"], ["a1", "a2", "a3"], false, "true"]);
check("the folds are kept by key", [st.load("ws.dashboard.railended"), R.endedOpen(st).has("rended:/a")], [null, false]);
s.ended.put("rended:/a", true);
s.ended.put("rended:/a", true);
check("put is idempotent", JSON.parse(st.load("ws.dashboard.railended")), ["rended:/a"]);
var s2 = R.state(st);
R.marks(r, s2, "b1");
R.foldPick(s2, halfD);
check("a pick on the open half closes it and forgets it", [ended.classList.contains("on"), halfD.getAttribute("aria-expanded"), st.load("ws.dashboard.railended")], [false, "false", null]);
R.foldPick(s2, halfA);
check("a disabled half never opens", [halfA.getAttribute("aria-expanded"), st.load("ws.dashboard.railended")], ["false", null]);
halfA.disabled = false;
st.save("ws.dashboard.railended", JSON.stringify(["rended:/a"]));
R.foldPick(s2, halfA);
check("the halves are one radio: archived opens, ended closes", [halfA.getAttribute("aria-expanded"), halfD.getAttribute("aria-expanded"), ended.classList.contains("on"), JSON.parse(st.load("ws.dashboard.railended"))], ["true", "false", false, ["rarch:/a"]]);
R.marks(r, R.state(st), "b1");
check("marks restores the archived pick", [halfA.getAttribute("aria-expanded"), halfD.getAttribute("aria-expanded")], ["true", "false"]);
if (bad.length) console.log(bad.join("\n"));
`

// TestRailMarks runs rail.js's marks, which the page script runs after
// every rail swap and the shell before its rail first shows: the viewed
// row, collapsed workspaces and parents, and the ended folds left open
// (kept by their key). The order and stars come rendered from the daemon's
// rail layout, so the retired local keys move nothing.
func TestRailMarks(t *testing.T) {
	runNode(t, "rail marks", railJSHarness+railMarksHarness+mustAsset(t, "rail.js")+railMarksCases)
}
