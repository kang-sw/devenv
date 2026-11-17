package wsdashboard

import (
	"encoding/json"
	"os"
	"testing"
)

// Execute the memory-only pane against the production preview state machine.
func TestFilesPreviewPane(t *testing.T) {
	src, _ := json.Marshal(mustAsset(t, "preview.js"))
	fixture, err := os.ReadFile("testdata/preview-pane.test.cjs")
	if err != nil {
		t.Fatal(err)
	}
	runNode(t, "dedicated file preview pane", "var PREVIEW = "+string(src)+";\n"+outsideDOM+string(fixture))
}

// Execute the production preview renderer, shared by Files and explorer cards.
func TestFilesPreviewContentRenderer(t *testing.T) {
	src, _ := json.Marshal(mustAsset(t, "preview.js"))
	fixture, err := os.ReadFile("testdata/preview-content.test.cjs")
	if err != nil {
		t.Fatal(err)
	}
	runNode(t, "preview highlighting and PDF/media fallback", "var PREVIEW = "+string(src)+";\n"+outsideDOM+string(fixture))
}
