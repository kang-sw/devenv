package wswhiteboard

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"
)

func panelXML(topic, recap, payload string) string {
	return fmt.Sprintf(`<wb:topic name=%q><wb:panel recap=%q>%s</wb:panel></wb:topic>`, topic, recap, payload)
}

func logXML(topic, line string) string {
	return fmt.Sprintf(`<wb:topic name=%q><wb:log>%s</wb:log></wb:topic>`, topic, line)
}

func entries(t *testing.T, st *State, topic string) []Entry {
	t.Helper()
	return topicEntry(t, st, topic).Entries
}

func eventTexts(t *testing.T, st *State, topic string) []string {
	t.Helper()
	var out []string
	for _, entry := range entries(t, st, topic) {
		if entry.Kind == EntrySystem {
			out = append(out, entry.Text)
		}
	}
	return out
}

func TestEntryStreamPanelsLogsAndDurableIDs(t *testing.T) {
	st := &State{}
	mustApply(t, st, panelXML("a", "  First &amp; recap  ", "  payload &amp; <b>raw</b>  ")+logXML("a", "**ready**; {{1}}")+panelXML("b", "Second", ""))
	want := []Entry{
		{ID: "e1", Kind: EntryPanel, PanelID: "p1", Recap: "First & recap", Text: "  payload &amp; <b>raw</b>  ", At: testNow.Format(time.RFC3339)},
		{ID: "e2", Kind: EntrySystem, Text: "Panel [p1](#p1) added: First & recap", At: testNow.Format(time.RFC3339)},
		{ID: "e3", Kind: EntryAgent, Text: "**ready**; {{1}}", At: testNow.Format(time.RFC3339)},
	}
	if got := entries(t, st, "a"); !reflect.DeepEqual(got, want) {
		t.Fatalf("entries = %+v, want %+v", got, want)
	}
	if st.EntrySeq != 5 || st.PanelCounter != 2 || entries(t, st, "b")[0].PanelID != "p2" || entries(t, st, "b")[0].ID != "e4" {
		t.Fatalf("board-wide counters/IDs: %+v", st)
	}
	var loaded State
	if err := json.Unmarshal(mustMarshal(t, st), &loaded); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(st, &loaded) {
		t.Fatal("entry state did not round-trip")
	}
	mustApply(t, &loaded, `<wb:topic name="a"><wb:panel id="p1" recap="Later"/><wb:panel id="p1" recap="Later"/></wb:topic>`+panelXML("b", "Third", ""))
	got := entries(t, &loaded, "a")
	if !reflect.DeepEqual(got[:3], want) || got[3].ID != "e6" || got[5].ID != "e8" || got[3].PanelID != "p1" || got[5].PanelID != "p1" || got[3].Text != "" {
		t.Fatalf("re-post must append independent full versions, including identical re-posts: %+v", got)
	}
	if loaded.EntrySeq != 11 || loaded.PanelCounter != 3 || entries(t, &loaded, "b")[2].PanelID != "p3" {
		t.Fatalf("restart reused IDs: %+v", loaded)
	}
	if st.Topics[0].Recap != "" || loaded.Topics[0].Recap != "" {
		t.Fatal("new entries must not dual-write legacy recap")
	}
}

func TestEntryCommandRefusalsAreAtomic(t *testing.T) {
	st := &State{}
	mustApply(t, st, panelXML("a", "First", ""))
	for name, tc := range map[string]struct{ command, problem string }{
		"panel scope":       {`<wb:panel recap="x"/>`, "goes inside"},
		"log scope":         {`<wb:log>x</wb:log>`, "goes inside"},
		"missing recap":     {`<wb:topic name="a"><wb:panel/></wb:topic>`, "recap: required"},
		"empty recap":       {`<wb:topic name="a"><wb:panel recap=" "/></wb:topic>`, "recap: must be non-empty"},
		"bad id":            {`<wb:topic name="a"><wb:panel id="p0" recap="x"/></wb:topic>`, "existing panel ID"},
		"noncanonical id":   {`<wb:topic name="a"><wb:panel id="p01" recap="x"/></wb:topic>`, "existing panel ID"},
		"unknown id":        {`<wb:topic name="a"><wb:panel id="p99" recap="x"/></wb:topic>`, "does not exist"},
		"other topic":       {`<wb:topic name="b"><wb:panel id="p1" recap="x"/></wb:topic>`, "belongs to topic"},
		"panel edit":        {`<wb:topic name="a"><wb:edit panel="p1"><wb:old>x</wb:old><wb:new>y</wb:new></wb:edit></wb:topic>`, "unknown attribute panel"},
		"removed recap":     {`<wb:topic name="a"><wb:recap>x</wb:recap></wb:topic>`, "unknown command"},
		"superseded append": {`<wb:topic name="a"><wb:append>x</wb:append></wb:topic>`, "unknown command"},
		"unknown log attr":  {`<wb:topic name="a"><wb:log kind="system">forged</wb:log></wb:topic>`, "unknown attribute kind"},
		"full doc":          {panelXML("a", "x", "<!DOCTYPE html><html><body>x</body></html>"), "not a full document"},
	} {
		t.Run(name, func(t *testing.T) {
			wantProblems(t, refused(t, st, panelXML("a", "not committed", "")+tc.command), tc.problem)
		})
	}
	// Even an event from an otherwise successful earlier command is refused.
	refused(t, st, addXML(ClassTechnical, "a", "not committed")+logXML("a", "not committed")+`<wb:nope/>`)
	if st.EntrySeq != 2 || st.PanelCounter != 1 || len(st.Items) != 0 {
		t.Fatalf("refused batch consumed IDs or emitted events: %+v", st)
	}
}

func TestAgentLogIsInlineOnly(t *testing.T) {
	for _, line := range []string{
		"", "  ", "first\nsecond", "# heading", "> quote", "- list", "1. list", "---", "    indented code",
		"```code```\nnext", "![image](shot.png)", "<script>alert(1)</script>", "<b>HTML</b>", "a\n\nparagraph",
	} {
		t.Run(fmt.Sprintf("%q", line), func(t *testing.T) {
			refused(t, &State{}, logXML("t", line))
		})
	}
	for _, line := range []string{
		"한 줄 **ready** *now* ~~old~~", "[panel](#p1) and {{1}}", "`<script>` and `![image](x)`", "<https://example.com>", "A &amp; B < C",
	} {
		st := &State{}
		mustApply(t, st, logXML("t", line))
		if got := entries(t, st, "t"); len(got) != 1 || got[0].Kind != EntryAgent || got[0].Text != line {
			t.Fatalf("agent line = %+v", got)
		}
	}
}

func TestPanelPayloadByteCap(t *testing.T) {
	// Measured repository fixtures on 2026-11-13 (bytes, not characters):
	// ai-docs/ref/whiteboard-mockup/r5-status-band/index.html: 144,150 UTF-8.
	// ai-docs/ref/whiteboard-mockup/r6-check-items/staged.png: 460,493 binary,
	//   614,025 as ![mockup](data:image/png;base64,...).
	// ai-docs/ref/agents-panel-stats-mockup/overview.png: 454,497 binary,
	//   606,029 in the same Markdown/data-URI form.
	// 2 MiB accommodates these mockups/images with >3x image headroom while
	// bounding one version. Whole HTML documents still need inline fragments.
	if MaxPanelBytes != 2*1024*1024 {
		t.Fatal("panel payload budget changed without fixture evidence")
	}
	payload := strings.Repeat("가", MaxPanelBytes/3) + strings.Repeat("x", MaxPanelBytes%3)
	st := &State{}
	mustApply(t, st, panelXML("t", "at the cap", payload))
	if entries(t, st, "t")[0].Text != payload {
		t.Fatal("accepted payload bytes changed")
	}
	wantProblems(t, refused(t, st, panelXML("t", "too large", payload+"x")), "exceeds 2 MiB of UTF-8", "link an external file")
	wantProblems(t, refused(t, st, panelXML("t", "invalid UTF-8", string([]byte{0xff}))), "valid UTF-8")
	// A document tag inside code is not active HTML.
	mustApply(t, st, panelXML("t", "code sample", "`<html>` and `<!DOCTYPE html>`"))
}

func TestLatestAgentIgnoresPanelsAndSystemEvents(t *testing.T) {
	st := &State{}
	if _, ok := st.LatestAgent("t"); ok {
		t.Fatal("empty topic has an agent line")
	}
	if _, ok := (*State)(nil).LatestAgent("t"); ok {
		t.Fatal("nil board has an agent line")
	}
	mustApply(t, st, logXML("t", "first")+logXML("t", "second")+panelXML("t", "not a summary", "")+addXML(ClassTechnical, "t", "new"))
	got, ok := st.LatestAgent("t")
	if !ok || got.ID != "e2" || got.Text != "second" {
		t.Fatalf("latest = %+v, %v", got, ok)
	}
	d := TopicDoc{Entries: []Entry{{ID: "e1", Kind: EntryAgent, Text: "imported", Imported: true}, {ID: "e2", Kind: EntrySystem, Text: "later"}}}
	if got, ok := d.LatestAgent(); !ok || !got.Imported || got.Text != "imported" {
		t.Fatalf("imported latest = %+v, %v", got, ok)
	}
}

func TestEntryOnlyTopicsSurviveCloseReopenAndAdoption(t *testing.T) {
	for _, seed := range []string{panelXML("Old title", "message", ""), logXML("Old title", "status")} {
		st := &State{}
		mustApply(t, st, seed+`<wb:topic name="Old title"><wb:close result="done"/></wb:topic>`)
		before := append([]Entry(nil), entries(t, st, "Old title")...)
		mustApply(t, st, `<wb:topic name="adopted" label="Old title"/>`)
		if !st.topicKnown("adopted") || !st.topicClosed("adopted") || !reflect.DeepEqual(entries(t, st, "adopted"), before) {
			t.Fatalf("entry-only adoption lost history/closed state: %+v", st)
		}
		mustApply(t, st, logXML("Old title", "active again"))
		got := entries(t, st, "adopted")
		if st.topicClosed("adopted") || st.TopicLabel("adopted") != "Old title" || got[len(before)].Text != "Topic reopened." || got[len(before)+1].Kind != EntryAgent {
			t.Fatalf("agent activity did not reopen retained topic: %+v", st)
		}
		mustApply(t, st, `<wb:topic name="adopted"><wb:close result="done"/></wb:topic>`+panelXML("adopted", "new message", ""))
		if st.topicClosed("adopted") {
			t.Fatal("panel activity must reopen a topic")
		}
		mustApply(t, st, `<wb:topic name="adopted"><wb:panel id="p1" recap="version after adoption"/></wb:topic>`)
		got = entries(t, st, "adopted")
		if got[len(got)-2].PanelID != "p1" || got[len(got)-2].Recap != "version after adoption" {
			t.Fatal("panel identity did not survive topic adoption")
		}
	}
}

func TestSystemEventsFromAcceptedTransitions(t *testing.T) {
	st := &State{}
	mustApply(t, st, addXML(ClassPolicy, "t", "pick", "x", "y")+addXML(ClassCheck, "t", "look")+addXML(ClassTechnical, "t", "withdraw later"))
	mustApply(t, st, `<wb:answer ids="1 2" value="a" assumed="true">seems good</wb:answer><wb:answer ids="1 2" value="a">yes, ship</wb:answer>`)
	mustApply(t, st, `<wb:patch id="3"><wb:why>correction</wb:why></wb:patch><wb:patch id="3" status="withdrawn"/>`+topicXML("t", "alpha")+`<wb:topic name="t"><wb:edit><wb:old>alpha</wb:old><wb:new>beta</wb:new></wb:edit><wb:close result="Shipped."/></wb:topic>`)
	want := []string{
		"Item {{1}} added [policy]: pick", "Item {{2}} added [check]: look", "Item {{3}} added [technical]: withdraw later",
		`Item {{1}} assumed [policy]: a; owner quote: "seems good"`, `Item {{2}} assumed [check]: a; owner quote: "seems good"`,
		`Item {{1}} answered [policy]: a; owner quote: "yes, ship"`, `Item {{2}} answered [check]: a; owner quote: "yes, ship"`,
		"Item {{3}} patched [technical] (open -> open): withdraw later", "Item {{3}} patched [technical] (open -> withdrawn): withdraw later",
		"Content updated.", "Content edited.", "Topic closed: Shipped.",
	}
	if got := eventTexts(t, st, "t"); !reflect.DeepEqual(got, want) {
		t.Fatalf("events = %q, want %q", got, want)
	}
	itemIDs := []int{1, 2, 3, 1, 2, 1, 2, 3, 3, 0, 0, 0}
	for i, entry := range entries(t, st, "t") {
		if entry.ID != fmt.Sprintf("e%d", i+1) || entry.ItemID != itemIDs[i] || entry.At != testNow.Format(time.RFC3339) {
			t.Fatalf("event order/attribution/time %d = %+v", i, entry)
		}
	}
	var loaded State
	if err := json.Unmarshal(mustMarshal(t, st), &loaded); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(loaded.Topics, st.Topics) {
		t.Fatal("item attribution did not survive serialization")
	}
	if !st.topicClosed("t") || item(t, st, 1).Status != StatusAnswered || item(t, st, 2).Status != StatusAnswered {
		t.Fatal("closing after grouped answers must not withdraw answered items")
	}
}

func TestSystemEventsNoopsWithdrawalsReasksAndReopen(t *testing.T) {
	st := &State{}
	mustApply(t, st, addXML(ClassTechnical, "t", "x")+topicXML("t", "alpha"))
	mustApply(t, st, `<wb:answer id="1" value="ok">exact owner words</wb:answer>`)
	before := st.clone()
	later := testNow.Add(time.Hour)
	applyAt(t, st, `<wb:answer id="1" value="ok">exact owner words</wb:answer><wb:patch id="1"><wb:title>x</wb:title></wb:patch>`+topicXML("t", "alpha")+`<wb:topic name="t"><wb:edit><wb:old>alpha</wb:old><wb:new>alpha</wb:new></wb:edit></wb:topic>`, later)
	if st.EntrySeq != before.EntrySeq || !reflect.DeepEqual(st.Items, before.Items) {
		t.Fatalf("no-op changed item marks/settle time or emitted an event: %+v", st)
	}
	mustApply(t, st, `<wb:patch id="1" status="open"/><wb:topic name="t"><wb:close result="done"/></wb:topic>`)
	got := eventTexts(t, st, "t")
	wantTail := []string{"Item {{1}} patched [technical] (answered -> open): x", "Item {{1}} withdrawn [technical] by topic close: x", "Topic closed: done"}
	if !reflect.DeepEqual(got[len(got)-3:], wantTail) {
		t.Fatalf("withdraw/re-ask events: %q", got)
	}
	stream := entries(t, st, "t")
	if stream[len(stream)-2].ItemID != 1 || stream[len(stream)-1].ItemID != 0 {
		t.Fatalf("withdrawal attribution must be server-owned: %+v", stream)
	}
	seq := st.EntrySeq
	mustApply(t, st, `<wb:topic name="t"><wb:close result="done"/></wb:topic>`+topicXML("t", "alpha"))
	if st.EntrySeq != seq || !st.topicClosed("t") {
		t.Fatal("unchanged close/body must not emit or reopen")
	}
	mustApply(t, st, `<wb:topic name="t"><wb:close result="corrected"/></wb:topic><wb:patch id="1"><wb:why>changed while closed</wb:why></wb:patch>`)
	if !st.topicClosed("t") {
		t.Fatal("system events alone must not reopen a topic")
	}
	mustApply(t, st, topicXML("t", "beta"))
	got = eventTexts(t, st, "t")
	if st.topicClosed("t") || got[len(got)-2] != "Topic reopened." || got[len(got)-1] != "Content updated." {
		t.Fatalf("content reopen: %q", got)
	}
	seq = st.EntrySeq
	st.BeginRender()
	if st.EntrySeq != seq {
		t.Fatal("render-only refresh appended an event")
	}
}

func TestContentClearAndRevertKeepHistory(t *testing.T) {
	st := &State{}
	mustApply(t, st, topicXML("t", "alpha"))
	before := topicEntry(t, st, "t")
	mustApply(t, st, topicXML("t", "beta")+topicXML("t", "alpha"))
	got := topicEntry(t, st, "t")
	if got.Changed != before.Changed || got.ChangedAt != before.ChangedAt || len(got.Entries) != 3 {
		t.Fatalf("revert must preserve body marks but record both real transitions: %+v", got)
	}
	mustApply(t, st, `<wb:topic name="t"><wb:close result="done"/><wb:body/></wb:topic>`)
	got = topicEntry(t, st, "t")
	if got.Body != "" || got.Changed != 0 || got.ChangedAt != "" || !st.topicClosed("t") || len(got.Entries) != 5 {
		t.Fatalf("clearing content must keep history without reopening: %+v", got)
	}
	seq := st.EntrySeq
	mustApply(t, st, `<wb:topic name="t"><wb:body/></wb:topic><wb:topic name="never"><wb:body/></wb:topic>`)
	if st.EntrySeq != seq || st.topicKnown("never") {
		t.Fatal("clearing absent content must do nothing")
	}
}
