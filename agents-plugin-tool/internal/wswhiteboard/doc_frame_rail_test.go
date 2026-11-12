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
