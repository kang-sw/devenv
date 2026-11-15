package wsdashboard

import (
	"encoding/json"
	"os"
	"regexp"
	"strings"
	"testing"
)

// Compact dimensions belong to the explorer, not the shared toolbar or cards.
func TestExplorerCompactLayout(t *testing.T) {
	css := mustAsset(t, "dashboard.css")
	for selector, want := range map[string][]string{
		`.fbar`:                               {"height: 30px", "box-sizing: border-box"},
		`.fsrow`:                              {"height: 34px", "box-sizing: border-box"},
		`.ffooter`:                            {"height: 30px", "box-sizing: border-box"},
		`.fentry, .fdetail-head`:              {"height: 22px", "padding: 0 6px"},
		`.fpane .fr.cur`:                      {"background:"},
		`.fpane[data-fm="content"] .fopts`:    {"display: inline-flex"},
		`.fopts[hidden]`:                      {"display: none"},
		`.bcard[data-kind="explorer"] .bc-hd`: {"height: 28px", "min-height: 28px", "box-sizing: border-box"},
		`.bcard[data-kind="explorer"] .bc-hd .ibtn`: {"width: 22px", "height: 22px"},
		`.bc-hd`: {"min-height: 32px", "padding: 4px 6px 4px 12px"},
	} {
		rule := regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(selector) + ` \{[^}]+\}`).FindString(css)
		for _, value := range want {
			if !strings.Contains(rule, value) {
				t.Errorf("%s lacks %q", selector, value)
			}
		}
	}
	sprite, err := lucideSprite()
	if err != nil {
		t.Fatal(err)
	}
	for _, icon := range []string{"rows-3", "list", "layout-grid", "images", "columns-3", "paperclip", "copy", "plus"} {
		if !strings.Contains(string(sprite), `id="l-`+icon+`"`) {
			t.Errorf("compact explorer icon %s is not embedded", icon)
		}
	}
}

// Exercise the production registration, page adapters, composer edits, and
// shared sheet together; no daemon/browser process is needed for this contract.
func TestFilesExplorerCards(t *testing.T) {
	var src strings.Builder
	for _, pair := range [][2]string{{"CARDS", "cards.js"}, {"FILES", "files.js"}, {"MENTION", "mention.js"}} {
		value, _ := json.Marshal(mustAsset(t, pair[1]))
		src.WriteString("var " + pair[0] + " = " + string(value) + ";\n")
	}
	dashboard := mustAsset(t, "dashboard.js")
	start := strings.Index(dashboard, "  if ((live || worktree) && fpane && window.WsFiles) {")
	end := strings.Index(dashboard, "  var cardBoard =")
	if start < 0 || end <= start {
		t.Fatal("page-owned explorer initialization not found")
	}
	pageCode := "var worktree = '', cardIdentity = rid;\n" + dashboard[start:end]
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
