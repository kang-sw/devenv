package wsdashboard

import (
	"encoding/json"
	"os"
	"regexp"
	"strings"
	"testing"
)

// Exercise the production registration, page adapters, composer edits, and
// shared sheet together; no daemon/browser process is needed for this contract.
func TestFilesExplorerCards(t *testing.T) {
	var src strings.Builder
	for _, pair := range [][2]string{{"CARDS", "cards.js"}, {"FILES", "files.js"}, {"MENTION", "mention.js"}} {
		value, _ := json.Marshal(mustAsset(t, pair[1]))
		src.WriteString("var " + pair[0] + " = " + string(value) + ";\n")
	}
	dashboard := mustAsset(t, "dashboard.js")
	start := strings.Index(dashboard, "  if (live && fpane && window.WsFiles) {")
	end := strings.Index(dashboard, "  var cardBoard =")
	if start < 0 || end <= start {
		t.Fatal("page-owned explorer initialization not found")
	}
	pageCode := dashboard[start:end]
	for _, name := range []string{"editField", "editText", "attachExplorerPath"} {
		fn := regexp.MustCompile(`(?s)\n  function ` + name + `\(.*?\n  \}\n`).FindString(dashboard)
		// editText is deliberately a single-line wrapper.
		if name == "editText" {
			fn = regexp.MustCompile(`\n  function editText\([^\n]+\n`).FindString(dashboard)
		}
		if fn == "" {
			t.Fatalf("composer function %s not found", name)
		}
		pageCode += fn + "\n"
	}
	encoded, _ := json.Marshal(pageCode)
	src.WriteString("var PAGE_FILES = " + string(encoded) + ";\n")
	fixture, err := os.ReadFile("testdata/explorer-cards.test.cjs")
	if err != nil {
		t.Fatal(err)
	}
	runNode(t, "explorer board integration", src.String()+cardsDOM+string(fixture))
}
