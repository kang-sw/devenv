package wsdashboard

import (
	"regexp"
	"strings"
	"testing"
)

// TestBoardKeepWiring pins how a board swap compensates the scroll and
// re-points the visibility source (dashboard.js swap): the keep session
// starts on the board in view before the morph, settles on its anchor
// after it, and is applied once every synchronous change (restored history
// pages included) has landed; restoreScroll leaves the scroller the
// session owns alone. After the morph the removed sections are swept and
// the inserted ones watched.
func TestBoardKeepWiring(t *testing.T) {
	data, err := dashboardFS.ReadFile("assets/dashboard.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	swap := regexp.MustCompile(`(?s)\n  function swap\(name, hash, html, still\) \{.*?\n  \}\n`).FindString(js)
	if swap == "" {
		t.Fatal("dashboard.js holds no swap")
	}
	order := []string{
		"var before = board && !still && !reducedMotion() ? boardSpots(el) : null;",
		"var kept = board ? boardKeep(el) : null, boardTop = before && kept ? el.getBoundingClientRect().top : null;",
		"ins = morphBoard(el, html);",
		"if (kept) boardKeepSettle(kept, el, ins);",
		"window.wbHistory.sweep(); ins.forEach(watchBoard);",
		`q("iframe.doc-frame", n).forEach(function (f) { f.wbKeep = true; });`,
		"if (kept) kept.h.apply();\n        startMotion(",
		"restoreScroll(scroll);",
		"if (kept) kept.h.apply();",
	}
	at := 0
	for _, s := range order {
		i := strings.Index(swap[at:], s)
		if i < 0 {
			t.Fatalf("swap lacks %q after offset %d", s, at)
		}
		at += i + len(s)
	}
	// A frame the swap brought keeps the board in place on its first
	// height change, which may land after the swap's session ended.
	// The height batch (applyFrameHeights) starts one session for every such
	// frame before any height lands and applies it after (behaviour:
	// TestBoardFrameHeightsBatch).
	if !strings.Contains(js, "if (!kept) kept = boardKept() || boardKeep(b);\n      if (kept) f.wbKeep = false;") || !strings.Contains(js, "      postTheme(f);\n    });\n    if (kept) kept.h.apply();\n  }") {
		t.Error("a swapped-in frame's late height is not compensated")
	}
	if !strings.Contains(js, "\n  boardBoxes().forEach(watchBoard);\n") {
		t.Error("the initial board's sections are not watched at load")
	}
	if !strings.Contains(js, "btn.parentNode.replaceChild(page, btn);\n      watchBoard(page);") {
		t.Error("a fetched closed page's sections are not watched")
	}
	for _, re := range []string{
		`(?s)\n  var modeHash = "", hiddenRegions = \{\};\n.*?\n  function pageHidden\(\) \{ return doc\.hidden \|\| shHidden; \}\n`,
		`(?s)function onRegion\(ev\) \{.*?if \(pageHidden\(\)\) \{\n        delete hiddenRegions\[ev\.name\];`,
		`function showRegions\(\) \{\n    if \(pageHidden\(\)\) return;`,
	} {
		if !regexp.MustCompile(re).MatchString(js) {
			t.Errorf("dashboard.js does not match %s", re)
		}
	}
}

// TestBoardKeepUnderNode runs the board's anchor pick, its settle after the
// morph, watchBoard, and restoreScroll under node: the anchor is never an
// entry the swap moves and a held one comes first; the session started on
// the topmost entry re-anchors on the pick at its offset from before, or
// ends when nothing in view stays; a section is watched once; and a pane a
// keep session owns is not restored while the others are. The cases
// execute under node when it is installed.
func TestBoardKeepUnderNode(t *testing.T) {
	data, err := dashboardFS.ReadFile("assets/dashboard.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	var fns []string
	for _, name := range []string{"boardAnchor", "boardKeepSettle", "watchBoard", "restoreScroll"} {
		f := regexp.MustCompile(`(?s)\n  function ` + name + `\(.*?\n  \}\n`).FindString(js)
		if f == "" {
			t.Fatalf("dashboard.js holds no %s", name)
		}
		fns = append(fns, f)
	}
	runNode(t, "board keep", `
var bad = [];
function check(what, got, want) { if (JSON.stringify(got) !== JSON.stringify(want)) bad.push(what + ": got " + JSON.stringify(got) + ", want " + JSON.stringify(want)); }
var log = [], owned = null;
var window = { wbHistory: {
  keep: function (n, opt) { log.push("keep:" + n.name + ":" + opt.offset); return {}; },
  watch: function (n) { log.push("watch:" + n.name); return { node: n }; },
  keeping: function (el) { return el === owned ? {} : null; }
} };
function q(sel, root) { return root.kids || []; }
function N(name, parent) { return { name: name, parentNode: parent || null, nodeType: 1, matches: function () { return !!this.section; } }; }
`+strings.Join(fns, "")+`
// --- the anchor pick ---
var box = N("box"), m = N("m", box), s1 = N("s1", box), s2 = N("s2", box);
function names(c) { return c ? c.n.name : null; }
var view = [{ n: m }, { n: s1 }, { n: s2 }];
function movingM(c) { return c.n === m; }
check("the topmost entry the swap does not move", names(boardAnchor(view, movingM)), "s1");
check("a held entry first, moving or not", names(boardAnchor([{ n: m }, { n: s1 }, { n: s2, held: true }], movingM)), "s2");
check("nothing in view stays", boardAnchor(view, function () { return true; }), null);

// --- settle after the morph ---
var el = { contains: function (n) { return n !== gone; } }, gone = N("gone", box), ended = 0;
var h = { end: function () { ended++; } };
function settle(entries, ins) {
  log = []; ended = 0;
  var k = { h: h, view: entries.map(function (e) { return { n: e[0], off: e[1], parent: box }; }) };
  boardKeepSettle(k, el, ins);
  return [log, ended];
}
check("an anchor that stays keeps the session as started", settle([[s1, 10], [s2, 90]], []), [[], 0]);
check("an inserted topmost entry: re-anchor on the next at its old offset", settle([[m, 10], [s1, 90]], [m]), [["keep:s1:90"], 0]);
check("a removed topmost entry", settle([[gone, 10], [s2, 140]], []), [["keep:s2:140"], 0]);
var moved = N("moved", N("page"));
check("an entry moved to another parent", settle([[moved, 0], [s1, 50]], []), [["keep:s1:50"], 0]);
check("everything in view moved: the session ends", settle([[m, 10]], [m]), [[], 1]);

// --- watchBoard: each section once ---
log = [];
var a = N("a"), b = N("b"); a.section = b.section = true;
var root = N("root"); root.kids = [a, b];
watchBoard(root); watchBoard(root); watchBoard(a);
check("each section is watched once", log, ["watch:a", "watch:b"]);
var c = N("c"); c.section = true; log = [];
watchBoard(c);
check("an inserted section itself is watched", log, ["watch:c"]);

// --- restoreScroll skips the owned scroller ---
var bcol = { scrollTop: 400 }, convo = { scrollTop: 5 }, page = { scrollTop: 70 };
owned = bcol;
restoreScroll([[bcol, 100], [convo, 50], [page, 0]]);
check("the owned scroller is the session's; the others are restored", [bcol.scrollTop, convo.scrollTop, page.scrollTop], [400, 50, 0]);
owned = null;
restoreScroll([[bcol, 100]]);
check("with no session it is restored", bcol.scrollTop, 100);
process.stdout.write(bad.join("\n"));`)
}
