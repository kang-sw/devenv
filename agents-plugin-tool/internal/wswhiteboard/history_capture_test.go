package wswhiteboard

import (
	"encoding/json"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestHistoryCaptureFixtures is an opt-in artifact generator, not a browser
// verdict. Copy this file alone to a baseline checkout to render the same
// deterministic state through that checkout's production renderer.
func TestHistoryCaptureFixtures(t *testing.T) {
	dir := os.Getenv("WS_HISTORY_FIXTURE_DIR")
	if dir == "" {
		t.Skip("set WS_HISTORY_FIXTURE_DIR to generate production-renderer specimens")
	}
	dir, err := filepath.Abs(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 11, 14, 9, 0, 0, 123000000, time.UTC)
	st := &State{}
	apply := func(commands string, clock time.Time) Outcome {
		t.Helper()
		out, problems := Apply(st, commands, Env{Now: clock})
		if len(problems) != 0 {
			t.Fatalf("fixture command refused: %v\n%s", problems, commands)
		}
		return out
	}
	marshal := func(value any) []byte {
		t.Helper()
		b, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	write := func(name string, data []byte) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), data, 0644); err != nil {
			t.Fatal(err)
		}
	}
	itemIDs := map[string]int{}
	add := func(key, class, title, rationale string, options []string, recommendation string) int {
		t.Helper()
		var command strings.Builder
		fmt.Fprintf(&command, `<wb:topic name="DENSITY"><wb:add class="%s"><wb:title>%s</wb:title><wb:why>%s</wb:why>`, class, html.EscapeString(title), html.EscapeString(rationale))
		for _, option := range options {
			fmt.Fprintf(&command, "<wb:option>%s</wb:option>", html.EscapeString(option))
		}
		if recommendation != "" {
			fmt.Fprintf(&command, "<wb:recommend>%s</wb:recommend>", html.EscapeString(recommendation))
		}
		command.WriteString("</wb:add></wb:topic>")
		out := apply(command.String(), at)
		if len(out.IDs) != 1 {
			t.Fatalf("fixture add returned IDs %v", out.IDs)
		}
		itemIDs[key] = out.IDs[0]
		return out.IDs[0]
	}
	answer := func(id int, value, quote string, assumed bool) {
		apply(fmt.Sprintf(`<wb:answer id="%d" value="%s" assumed="%t">%s</wb:answer>`, id, html.EscapeString(value), assumed, html.EscapeString(quote)), at)
	}
	rich := add("rich", ClassPolicy, "보존할 결정 / Rich decision",
		"| Choice | Evidence |\n|---|---|\n| Whole entries | Preserves context |\n\n```go\nreturn keepWholeEntry\n```\n\n<strong>Allowlisted HTML</strong> keeps meaningful context.",
		[]string{"Clip the original context", "Keep `whole_entry` and the evidence table"}, "b: preserve the complete evidence")
	answer(rich, "b: retain the evidence and {{2}}", "Choose the second option: keep the table, code, and every useful explanation visible when expanded.", false)
	plain := add("plain", ClassTechnical, "단순 확인 / Plain approval", "", nil, "")
	answer(plain, "ok", "ok", false)
	titleLike := add("titleLike", ClassTechnical, "Title-like rationale remains meaningful", "Title-like rationale remains meaningful", nil, "")
	answer(titleLike, "ok", "ok", false)
	assumed := add("assumed", ClassPolicy, "가정 / Assumed direction", "Interpretation remains provisional until the owner confirms or corrects it.", []string{"Discard context", "Preserve context"}, "b: reversible and explicit")
	answer(assumed, "b: keep the provisional interpretation", "This seems reasonable, but keep my correction control available.", true)
	withdrawn := add("withdrawn", ClassTechnical, "철회 / Withdrawn proposal", "Withdrawn because the dependency was removed; see {{1}}.", nil, "")
	apply(fmt.Sprintf(`<wb:patch id="%d" status="withdrawn"/>`, withdrawn), at)
	superseded := add("superseded", ClassPolicy, "대체 / Superseded proposal", "Superseded by the complete decision {{1}}, not silently deleted.", []string{"Old partial view", "Old clipped view"}, "")
	apply(fmt.Sprintf(`<wb:patch id="%d" status="superseded"><wb:link>Replacement {{1}}</wb:link></wb:patch>`, superseded), at)
	add("openPolicy", ClassPolicy, "열린 질문 / Keep answer controls", "The open policy remains independently answerable.", []string{"First choice", "Second choice"}, "b: keep native controls")
	add("openCheck", ClassCheck, "열린 확인 / Inspect the evidence", "Check the table before answering.", nil, "")

	panel := func(id, recap, body string) string {
		attr := ""
		if id != "" {
			attr = ` id="` + id + `"`
		}
		return `<wb:topic name="DENSITY"><wb:panel` + attr + ` recap="` + html.EscapeString(recap) + `">` + html.EscapeString(body) + `</wb:panel></wb:topic>`
	}
	log := func(topic, text string) string {
		return `<wb:topic name="` + topic + `"><wb:log>` + html.EscapeString(text) + `</wb:log></wb:topic>`
	}
	for p := 1; p <= 3; p++ {
		body := fmt.Sprintf("## Panel %d\n\nIndependent panel disclosure and immutable payload.\n\n", p)
		if p == 3 {
			body += strings.Repeat("A whole oversized panel keeps this complete line without pixel clipping.\n\n", 32)
		}
		apply(panel("", fmt.Sprintf("패널 %d / Panel %d original", p, p), body), at)
		for n := 1; n <= 8; n++ {
			apply(log("DENSITY", fmt.Sprintf("Recorded log %02d: same clock, command order remains authoritative.", (p-1)*8+n)), at)
		}
	}
	apply(log("CLOSED", "Recorded closed-topic history, still independently expandable.")+`<wb:topic name="CLOSED"><wb:add class="technical"><wb:title>Closed decision</wb:title><wb:why>Retained closed-topic explanation.</wb:why></wb:add></wb:topic>`, at)
	closedID := st.Counter
	itemIDs["closed"] = closedID
	answer(closedID, "ok", "Keep the closed decision and its recorded history.", false)
	apply(`<wb:topic name="CLOSED"><wb:close result="Completed with history retained."/></wb:topic>`, at)

	// These imported records have no settlement events. ChangedAt is an
	// explicitly stored mutable clock, not a row time or archive-time fallback.
	var legacyIDs []int
	for i := 0; i < 67; i++ {
		id := 101 + (i*17)%67
		legacyIDs = append(legacyIDs, id)
		it := Item{ID: id, Topic: "LEGACY", Class: ClassTechnical,
			Title:  fmt.Sprintf("Legacy ordered row %02d (fixture %d)", i+1, id),
			Status: StatusAnswered, Answer: "ok", Changed: 1,
			ChangedAt: "2026-11-01T08:00:00Z", Settled: 100 + (i*23)%67}
		if i == 0 {
			it.AnsweredAt = "2026-10-31T07:00:00Z"
		}
		st.Items = append(st.Items, it)
		if id > st.Counter {
			st.Counter = id
		}
	}
	st.SettleSeq = 166
	legacyDoc := TopicDoc{Name: "LEGACY"}
	for i := 0; i < 2; i++ {
		st.EntrySeq++
		legacyDoc.Entries = append(legacyDoc.Entries, Entry{ID: fmt.Sprintf("e%d", st.EntrySeq), Kind: EntrySystem,
			ItemID: legacyIDs[i], At: "2026-11-01T08:00:00Z",
			Text: fmt.Sprintf("Item {{%d}} added [technical]: immutable legacy creation %d", legacyIDs[i], i+1)})
	}
	st.Topics = append(st.Topics, legacyDoc)
	legacyRecords := func() []Item {
		var items []Item
		for _, item := range st.Items {
			if item.Topic == "LEGACY" {
				items = append(items, item)
			}
		}
		return items
	}
	legacyBefore := string(marshal(legacyRecords()))
	legacyEventsBefore := string(marshal(legacyDoc.Entries))
	latestPanels := func() map[string]string {
		out := map[string]string{}
		for _, doc := range st.Topics {
			if doc.Name == "DENSITY" {
				for _, entry := range doc.Entries {
					if entry.Kind == EntryPanel {
						out[entry.PanelID] = entry.ID
					}
				}
			}
		}
		return out
	}
	original := latestPanels()["p2"]
	if len(latestPanels()) != 3 || original == "" {
		t.Fatal("initial DENSITY must have exactly three distinct panels, including p2")
	}
	type specimen struct {
		Standalone string `json:"standalone"`
		Board      string `json:"board"`
	}
	type stage struct {
		Name         string              `json:"name"`
		PanelEntries map[string]string   `json:"panelEntries"`
		Languages    map[string]specimen `json:"languages"`
	}
	manifest := struct {
		FoldPanel        string         `json:"foldPanel"`
		OriginalEntry    string         `json:"originalEntry"`
		RepostEntry      string         `json:"repostEntry"`
		NewPanel         string         `json:"newPanel"`
		LegacyOrderedIDs []int          `json:"legacyOrderedIDs"`
		LegacyEntries    []Entry        `json:"legacyEntries"`
		ItemIDs          map[string]int `json:"itemIDs"`
		Stages           []stage        `json:"stages"`
	}{FoldPanel: "p2", OriginalEntry: original, NewPanel: "p4", LegacyOrderedIDs: legacyIDs, LegacyEntries: legacyDoc.Entries, ItemIDs: itemIDs}
	capture := func(name string) {
		t.Helper()
		if string(marshal(legacyRecords())) != legacyBefore {
			t.Fatalf("%s: legacy item bytes/order changed", name)
		}
		for _, doc := range st.Topics {
			if doc.Name == "LEGACY" && string(marshal(doc.Entries)) != legacyEventsBefore {
				t.Fatalf("%s: immutable per-item creation logs changed", name)
			}
		}
		before := string(marshal(st))
		s := stage{Name: name, PanelEntries: latestPanels(), Languages: map[string]specimen{}}
		for _, lang := range []string{"en", "ko"} {
			view := BuildView(st, ViewInput{Layout: LayoutFull, ReplyID: "history-capture", Location: time.UTC})
			view.Lang, view.StampFile = lang, ""
			standalone, err := RenderPage(view)
			if err != nil {
				t.Fatal(err)
			}
			regions, err := RenderRegions(view)
			if err != nil {
				t.Fatal(err)
			}
			if regions.CountPolicy != 2 {
				t.Fatalf("%s/%s: open policy/check count = %d, want 2", name, lang, regions.CountPolicy)
			}
			for host, output := range map[string]string{"standalone": standalone, "regions": string(regions.Board)} {
				for key, id := range itemIDs {
					if !strings.Contains(output, fmt.Sprintf(`id="item-%d"`, id)) {
						t.Fatalf("%s/%s/%s missing %s item %d", name, lang, host, key, id)
					}
				}
				for _, token := range []string{"<table", "<pre", "<strong>Allowlisted HTML</strong>", "preserve the complete evidence", "Choose the second option:", "Title-like rationale remains meaningful", fmt.Sprintf(`data-id="%02d"`, itemIDs["openPolicy"]), fmt.Sprintf(`data-id="%02d"`, itemIDs["openCheck"])} {
					if !strings.Contains(output, token) {
						t.Fatalf("%s/%s/%s missing real fixture content %q", name, lang, host, token)
					}
				}
				for _, id := range legacyIDs {
					if !strings.Contains(output, fmt.Sprintf(`id="item-%d"`, id)) {
						t.Fatalf("%s/%s/%s missing legacy item %d", name, lang, host, id)
					}
				}
				for _, entry := range legacyDoc.Entries {
					if !strings.Contains(output, `id="`+entry.ID+`"`) || !strings.Contains(output, "immutable legacy creation") {
						t.Fatalf("%s/%s/%s missing legacy creation log %s", name, lang, host, entry.ID)
					}
				}
			}
			file := name + "-" + lang + ".html"
			write(file, []byte(standalone))
			s.Languages[lang] = specimen{Standalone: file, Board: string(regions.Board)}
			t.Logf("%s: %s (%d bytes), regions board %d bytes", name, filepath.Join(dir, file), len(standalone), len(regions.Board))
		}
		if string(marshal(st)) != before {
			t.Fatalf("%s: rendering mutated fixture state", name)
		}
		manifest.Stages = append(manifest.Stages, s)
	}
	capture("initial")
	for n := 1; n <= 3; n++ {
		apply(log("DENSITY", fmt.Sprintf("Unrelated update %d leaves p2 identity unchanged.", n))+
			panel("p3", fmt.Sprintf("Panel 3 unrelated revision %d", n), fmt.Sprintf("## Updated p3\n\nRevision %d, not a p2 update.", n)), at.Add(time.Duration(n)*time.Minute))
		if latestPanels()["p2"] != original {
			t.Fatal("unrelated update changed p2's immutable entry ID")
		}
		capture(fmt.Sprintf("unrelated-%d", n))
	}
	apply(panel("p2", "패널 2 갱신 / Panel 2 republished", "## Republished p2\n\nA user-folded p2 keeps its fold and an unread-update badge until actually opened."), at.Add(4*time.Minute))
	manifest.RepostEntry = latestPanels()["p2"]
	if manifest.RepostEntry == original || manifest.RepostEntry == "" {
		t.Fatal("p2 republish did not mint an independent entry")
	}
	capture("republish-p2")
	apply(panel("", "새 패널 / Genuinely new panel", "## New p4\n\nA new panel retains its own default disclosure behavior."), at.Add(5*time.Minute))
	if panels := latestPanels(); len(panels) != 4 || panels["p4"] == "" || panels["p2"] != manifest.RepostEntry {
		t.Fatalf("new panel changed existing identity: %v", panels)
	}
	capture("new-panel")
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	write("fixtures.json", append(data, '\n'))
	t.Logf("manifest: %s; %d stages, %d standalone files, %d legacy items, %d retained creation logs", filepath.Join(dir, "fixtures.json"), len(manifest.Stages), 2*len(manifest.Stages), len(legacyIDs), len(legacyDoc.Entries))
}
