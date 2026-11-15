package wsdashboard

import (
	"encoding/json"
	"os"
	"testing"
)

// Execute the production preview renderer, shared by Files and explorer cards.
func TestFilesPreviewContentRenderer(t *testing.T) {
	src, _ := json.Marshal(mustAsset(t, "preview.js"))
	fixture, err := os.ReadFile("testdata/preview-content.test.cjs")
	if err != nil {
		t.Fatal(err)
	}
	runNode(t, "preview highlighting and PDF fallback", "var PREVIEW = "+string(src)+";\n"+outsideDOM+string(fixture))
}
