package wsdashboard

import (
	"regexp"
	"strings"
	"testing"
)

// TestRailKeyFromDocFrame pins Ctrl+` pressed inside a topic body frame:
// the frame hands the key to the host as {wbDoc: "rail"} (wswhiteboard
// TestDocFrameHandsRailKey), and the host's frame listener, which answers
// only its own body frames (docFrame), runs railKey, the page keydown's own
// path: it toggles the overlay rail below 1600px and leaves the permanent
// column alone. The cases execute under node when it is installed.
func TestRailKeyFromDocFrame(t *testing.T) {
	js := mustAsset(t, "dashboard.js")
	if !strings.Contains(js, "e.shiftKey || e.isComposing || railWide()) return;\n      e.preventDefault();\n      railKey();") {
		t.Error("the page's Ctrl+` keydown does not run railKey")
	}
	listener := regexp.MustCompile(`window\.addEventListener\("message", function \(e\) \{\n    var f = docFrame\(e\.source\), m = e\.data;\n    if \(!f \|\| !m \|\| typeof m !== "object"\) return;\n[\s\S]*?\n  \}\);\n`).FindString(js)
	if !strings.Contains(listener, `m.wbDoc === "rail"`) {
		t.Fatalf("the docFrame-gated listener does not take {wbDoc: \"rail\"}:\n%s", listener)
	}
	var src string
	for _, name := range []string{"railWide", "railShown", "railOpen", "railKey"} {
		f := regexp.MustCompile(`\n  function ` + name + `\([^)]*\) \{[^\n]*\}\n`).FindString(js)
		if f == "" {
			f = regexp.MustCompile(`(?s)\n  function ` + name + `\([^)]*\) \{\n.*?\n  \}\n`).FindString(js)
		}
		if f == "" {
			t.Fatalf("dashboard.js holds no %s", name)
		}
		src += f
	}
	runNode(t, "rail key from a doc frame", `
var bad = [];
function check(what, got, want) { if (JSON.stringify(got) !== JSON.stringify(want)) bad.push(what + ": got " + JSON.stringify(got) + ", want " + JSON.stringify(want)); }
var cls = {};
var root = { classList: { contains: function (c) { return !!cls[c]; }, toggle: function (c, on) { cls[c] = !!on; } } };
var railEl = {}, railBtn = { setAttribute: function () {} }, WsRail = {};
var RAIL_WIDE = { matches: false };
var frameWin = {}, otherWin = {}, frameEl = { tag: "iframe.doc-frame" };
function docFrame(win) { return win === frameWin ? frameEl : undefined; }
var handler = null;
var window = { addEventListener: function (t, fn) { if (t === "message") handler = fn; } };
`+src+listener+`
function send(src, data) { handler({ source: src, data: data }); }
send(frameWin, { wbDoc: "rail" });
check("a doc frame's key opens the rail below 1600px", !!cls["rail-open"], true);
send(frameWin, { wbDoc: "rail" });
check("and closes it again", !!cls["rail-open"], false);
send(otherWin, { wbDoc: "rail" });
check("a window that is no doc frame is ignored", !!cls["rail-open"], false);
RAIL_WIDE.matches = true;
send(frameWin, { wbDoc: "rail" });
check("at 1600px and up the key is left alone", !!cls["rail-open"], false);
process.stdout.write(bad.join("\n"));
`)
}
