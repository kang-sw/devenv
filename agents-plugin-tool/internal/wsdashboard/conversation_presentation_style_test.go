package wsdashboard

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Static contracts complement, rather than claim, browser overflow/focus coverage.
func TestConversationPresentationStyleContract(t *testing.T) {
	data, err := os.ReadFile("assets/dashboard-conversation.css")
	if err != nil {
		t.Fatal(err)
	}
	css := string(data)
	start := strings.Index(css, "/* Display-only conversation projections;")
	end := strings.Index(css, "/* ===== composer: never swapped ===== */")
	if start < 0 || end <= start {
		t.Fatal("missing scoped presentation section")
	}
	section := css[start:end]
	for _, want := range []string{
		".mail-notification", ".input-record { border-style: dashed; }",
		".mailline.child-report", ".mailline.child-report .mfirst { margin-left: 0; }",
		"summary.conv-row::-webkit-details-marker { display: none; }",
		"grid-template-columns: 28px minmax(0, 1fr) 44px 18px; column-gap: 8px;",
		"font: 13px/1.5 system-ui", ".conv-row > .row-content",
		".conv-row > .row-time { text-align: right;", "font-variant-numeric: tabular-nums;",
		".conv-row > .row-chevron", "transform: rotate(-90deg)", "transform: rotate(0deg)",
		".conv-row .row-avatar", ".row-details { margin: 0 62px 4px 36px;",
		"summary.conv-row:focus-visible", ".conv-row a:focus-visible",
		".src.peer-lead { color: var(--wb-ck-ok);",
		".wbeffects.wbupdate {", ".wbupdate-bar", ".wbupdate-l",
		".wbupdate-panel .wbupdate-text", "text-overflow: ellipsis; white-space: nowrap;",
		".wbupdate-items { display: flex; flex-wrap: wrap;", ".wbeffects.wbupdate-line > summary",
		".wbupdate-line[open] > summary .i", ".wbupdate-status:empty",
		"overflow-wrap: anywhere", ":focus-visible", "outline-offset: 3px",
	} {
		if !strings.Contains(section, want) {
			t.Errorf("missing presentation contract %q", want)
		}
	}
	for _, forbidden := range []string{"@media", "@container", ".composer", "--wb-topic:", ".delivery-details", ".wbupdate-options", ".wbupdate-class", ".wbeffect-details"} {
		if strings.Contains(section, forbidden) {
			t.Errorf("presentation section crosses existing ownership: %q", forbidden)
		}
	}
	for _, want := range []string{
		".toolsline .tsl > span, .toolsline .tool { font-family: var(--wb-mono); }",
		".toolsline .tsn { min-width: 0; font-size: 12px; }",
		".mailline .mpeer { font-weight: 700; color: var(--wb-fg); text-decoration: none; flex: 0 1 auto; min-width: 0; overflow-wrap: anywhere; }",
		".brow .ba", "font-family: inherit;",
		".msg.fromlead .bub { margin: 0 62px 0 36px;",
		".turnline { align-self: flex-start;", "margin-left: 36px; font-size: 12px;", "max-width: calc(100% - 98px);",
	} {
		if !strings.Contains(css, want) {
			t.Errorf("missing row typography/alignment contract %q", want)
		}
	}
	for _, forbidden := range []string{".src.mail {", ".src.wb {", ".toolsline > summary {", ".toolsline[open] .tsum"} {
		if strings.Contains(css, forbidden) {
			t.Errorf("obsolete row styling %q", forbidden)
		}
	}
}

func TestConversationPresentationTranslationContract(t *testing.T) {
	data, err := os.ReadFile("assets/dashboard.tmpl")
	if err != nil {
		t.Fatal(err)
	}
	keys := map[string]bool{
		"js.whiteboard.jump.missing":        true,
		"js.whiteboard.jump.elementmissing": true,
	}
	for _, match := range regexp.MustCompile(`\bt "(entry\.[^"]+)"`).FindAllStringSubmatch(string(data), -1) {
		keys[match[1]] = true
	}
	paths, err := filepath.Glob("../wsi18n/tables/*.json")
	if err != nil || len(paths) == 0 {
		t.Fatalf("translation tables: %v", err)
	}
	for _, path := range paths {
		t.Run(filepath.Base(path), func(t *testing.T) {
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var table map[string]string
			if err := json.Unmarshal(data, &table); err != nil {
				t.Fatal(err)
			}
			for _, key := range []string{"entry.mail.details", "entry.whiteboard.answer", "entry.whiteboard.details", "entry.whiteboard.before", "entry.whiteboard.after"} {
				if _, exists := table[key]; exists {
					t.Errorf("obsolete translation %s", key)
				}
			}
			for key := range keys {
				if strings.TrimSpace(table[key]) == "" {
					t.Errorf("missing translation %s", key)
				}
			}
			if filepath.Base(path) == "ko.json" && table["entry.mail.peerlead"] != "피어 리드" {
				t.Error("Korean peer-lead badge differs from approved wording")
			}
			if filepath.Base(path) == "en.json" {
				if table["js.whiteboard.jump.missing"] != "This topic is no longer available." || table["js.whiteboard.jump.elementmissing"] != "The exact element is unavailable; showing its topic." {
					t.Error("navigation fallback copy changed")
				}
			}
		})
	}
}
