package wswhiteboard

import (
	"os/exec"
	"regexp"
	"strings"
	"testing"
)

// TestDocFrameHandsRailKey pins the topic body frame's half of the host's
// rail key: a key pressed in the frame reaches only the frame's document, so
// Ctrl+` (the host's own test) is posted to the host as {wbDoc: "rail"}; the
// host decides whether the rail toggles (wsdashboard TestRailKeyFromDocFrame).
func TestDocFrameHandsRailKey(t *testing.T) {
	raw, err := assetFS.ReadFile("assets/doc-frame.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(raw)
	want := "if (e.code !== \"Backquote\" || !e.ctrlKey || e.altKey || e.metaKey || e.shiftKey || e.isComposing) return;\n    e.preventDefault();\n    post({ wbDoc: \"rail\" });"
	if !strings.Contains(js, want) {
		t.Errorf("doc-frame.js does not hand Ctrl+` to the host:\n%s", want)
	}
}

// TestDocFrameHandsSearchKey runs the frame's half of the host palette's
// shortcut: Ctrl+K, Ctrl+Space and Cmd+K pressed in a topic body frame reach
// only the frame's document, so the frame posts {wbDoc: "search"} (wsdashboard
// TestSearchKeyFromDocFrame). A body has no terminal, the only target that
// keeps them (tracker.js qShortcut), so an editor in the body hands them on
// too. The cases execute under node when it is installed.
func TestDocFrameHandsSearchKey(t *testing.T) {
	raw, err := assetFS.ReadFile("assets/doc-frame.js")
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`(?s)\n(  d\.addEventListener\("keydown", function \(e\) \{\n    if \(!\(e\.ctrlKey \|\| e\.metaKey\).*?\n  \}\);\n)`).FindStringSubmatch(string(raw))
	if m == nil {
		t.Fatal("doc-frame.js holds no search key listener")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed; search key cases not executed")
	}
	script := `var posted = 0, handler, bad = [];
var d = { addEventListener: function (t, fn) { if (t === "keydown") handler = fn; } };
function post(m) { if (m.wbDoc === "search") posted++; }
` + m[1] + `
function fire(e) { posted = 0; e.preventDefault = function () {}; e.target = { closest: function () { return {}; } }; handler(e); return posted; }
function check(what, got, want) { if (got !== want) bad.push(what + ": got " + got + ", want " + want); }
check("Ctrl+K", fire({ ctrlKey: true, key: "k" }), 1);
check("Ctrl+Space", fire({ ctrlKey: true, key: " ", code: "Space" }), 1);
check("Cmd+K", fire({ metaKey: true, key: "k" }), 1);
check("Cmd+Space stays the system's", fire({ metaKey: true, key: " ", code: "Space" }), 0);
check("Ctrl+Cmd+K", fire({ ctrlKey: true, metaKey: true, key: "k" }), 0);
check("Ctrl+Shift+K", fire({ ctrlKey: true, shiftKey: true, key: "K" }), 0);
check("Ctrl+Alt+K", fire({ ctrlKey: true, altKey: true, key: "k" }), 0);
check("Ctrl+K while composing", fire({ ctrlKey: true, key: "k", isComposing: true }), 0);
check("plain k", fire({ key: "k" }), 0);
process.stdout.write(bad.join("\n"));
`
	out, err := exec.Command(node, "-e", script).CombinedOutput()
	if err != nil || len(out) > 0 {
		t.Fatalf("doc-frame search key: %v\n%s", err, out)
	}
}
