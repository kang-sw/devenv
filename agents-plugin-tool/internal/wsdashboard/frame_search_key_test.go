package wsdashboard

import (
	"regexp"
	"strings"
	"testing"
)

// TestSearchKeyFromDocFrame pins Ctrl+K / Ctrl+Space pressed inside a topic
// body frame: the frame hands the key to the host as {wbDoc: "search"}
// (wswhiteboard TestDocFrameHandsSearchKey), and the host's frame listener,
// which answers only its own body frames (docFrame), opens the palette
// through tkSearch, the shell's own path, but only while the user is acting:
// a body's script posting the message alone opens nothing.
func TestSearchKeyFromDocFrame(t *testing.T) {
	js := mustAsset(t, "dashboard.js")
	listener := regexp.MustCompile(`window\.addEventListener\("message", function \(e\) \{\n    var f = docFrame\(e\.source\), m = e\.data;\n    if \(!f \|\| !m \|\| typeof m !== "object"\) return;\n[\s\S]*?\n  \}\);\n`).FindString(js)
	if !strings.Contains(listener, `m.wbDoc === "search"`) {
		t.Fatalf("the docFrame-gated listener does not take {wbDoc: \"search\"}:\n%s", listener)
	}
	search := regexp.MustCompile(`(?s)\n  var tkWantSearch = false;\n  function tkSearch\(\) \{.*?\n  \}\n`).FindString(js)
	if search == "" {
		t.Fatal("dashboard.js holds no tkSearch")
	}
	runNode(t, "search key from a doc frame", `(function () {
var bad = [];
function check(what, got, want) { if (JSON.stringify(got) !== JSON.stringify(want)) bad.push(what + ": got " + JSON.stringify(got) + ", want " + JSON.stringify(want)); }
var opens = 0, tkApi = { openSearch: function () { opens++; } };
var navigator = { userActivation: { isActive: true } };
var frameWin = {}, otherWin = {}, frameEl = { getAttribute: function () { return "topic:body"; } };
function docFrame(win) { return win === frameWin ? frameEl : undefined; }
var handler = null;
var window = { addEventListener: function (t, fn) { if (t === "message") handler = fn; } };
`+search+listener+`
function send(src, data) { if (data.docKey === undefined) data.docKey = "topic:body"; handler({ source: src, data: data }); }
send(frameWin, { wbDoc: "search" });
check("a key in a body frame opens the palette", opens, 1);
send(otherWin, { wbDoc: "search" });
check("a window that is no doc frame is ignored", opens, 1);
send(frameWin, { wbDoc: "search", docKey: "entry:e7" });
check("the wrong document identity is ignored", opens, 1);
navigator.userActivation.isActive = false;
send(frameWin, { wbDoc: "search" });
check("a post with no user key press opens nothing", opens, 1);
navigator.userActivation.isActive = true;
tkApi = null;
send(frameWin, { wbDoc: "search" });
check("before mount the key is kept as a pending open", tkWantSearch, true);
process.stdout.write(bad.join("\n"));
})();
`)
}
