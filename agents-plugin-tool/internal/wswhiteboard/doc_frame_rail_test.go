package wswhiteboard

import (
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

// TestDocFrameHandsSearchKey pins the frame's half of the host palette's
// shortcut: Ctrl+K and Ctrl+Space pressed in a topic body frame reach only
// the frame's document, so the frame posts {wbDoc: "search"} (wsdashboard
// TestSearchKeyFromDocFrame), except from an editor in the body, which keeps
// them by the palette's own target rule (tracker.js qShortcut).
func TestDocFrameHandsSearchKey(t *testing.T) {
	raw, err := assetFS.ReadFile("assets/doc-frame.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(raw)
	for _, want := range []string{
		"if (!e.ctrlKey || e.altKey || e.metaKey || e.shiftKey || e.isComposing || e.keyCode === 229) return;",
		"if (e.code !== \"Space\" && e.key !== \" \" && (e.key || \"\").toLowerCase() !== \"k\") return;",
		"if (t && t.closest && t.closest(\"textarea, [contenteditable]:not([contenteditable=\\\"false\\\"])\")) return;\n    e.preventDefault();\n    post({ wbDoc: \"search\" });",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("doc-frame.js does not hand the palette shortcut to the host:\n%s", want)
		}
	}
}
