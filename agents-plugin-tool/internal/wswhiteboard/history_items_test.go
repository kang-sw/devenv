package wswhiteboard

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestHistoryItemsUseRecordedTransitionsWithoutChangingState(t *testing.T) {
	at := testNow.Format(time.RFC3339Nano)
	st := &State{Items: []Item{
		{ID: 1, Topic: "t", Class: ClassPolicy, Title: "choose", Status: StatusAnswered, Answer: "b", AnswerQuote: "Use b, preserving the full explanation.", Options: []string{"one", "two"}},
		{ID: 2, Topic: "t", Class: ClassTechnical, Title: "retired", Status: StatusWithdrawn, Rationale: "No longer needed"},
	}, Topics: []TopicDoc{{Name: "t", Entries: []Entry{
		{ID: "e1", Kind: EntrySystem, ItemID: 1, At: at, Text: "Item {{1}} added [policy]: choose"},
		{ID: "e2", Kind: EntryAgent, At: at, Text: "lead line"},
		{ID: "e3", Kind: EntrySystem, ItemID: 1, At: at, Text: "Item {{1}} answered [policy]: b; owner quote: original"},
		{ID: "e4", Kind: EntrySystem, ItemID: 2, At: at, Text: "Item {{2}} patched [technical] (open -> withdrawn): retired"},
		{ID: "e5", Kind: EntrySystem, ItemID: 1, At: at, Text: "Item {{1}} patched [policy] (answered -> answered): choose"},
	}}}}
	before := mustMarshal(t, st)
	tv := topicView(t, st, LayoutFull, "t")
	entries := pageEntries(tv.Stream)
	var ids, projected []string
	for _, e := range entries {
		ids = append(ids, e.ID)
		if e.Item != nil {
			projected = append(projected, fmt.Sprintf("%s:%d", e.ID, e.Item.ID))
			if e.At != at || e.Clock == "" {
				t.Fatalf("lost recorded transition clock: %+v", e)
			}
		}
	}
	if !reflect.DeepEqual(ids, []string{"e1", "e2", "e3", "e4", "e5"}) || !reflect.DeepEqual(projected, []string{"e3:1", "e4:2"}) {
		t.Fatalf("stream order/projection: %v / %v", ids, projected)
	}
	if string(before) != string(mustMarshal(t, st)) {
		t.Fatal("presentation rewrote durable history")
	}
	out := markup(render(t, BuildView(st, ViewInput{Layout: LayoutFull, ReplyID: "r"})))
	wantAll(t, out, `data-history-item="1"`, `id="item-1"`, "Use b, preserving the full explanation.", "No longer needed")
	wantNone(t, out, `<details class="settled"`, `data-key="history:t"`, "archive time")
}

func TestHistoryUnknownItemsKeepExistingOrderAndIgnoreMutableClocks(t *testing.T) {
	st := &State{Items: []Item{
		{ID: 9, Topic: "legacy", Class: ClassTechnical, Title: "first legacy", Status: StatusAnswered, Answer: "ok", ChangedAt: "2099-01-01T00:00:00Z", Settled: 99},
		{ID: 2, Topic: "legacy", Class: ClassCheck, Title: "second legacy", Status: StatusAssumed, Answer: "ok", AnswerQuote: "That seems reasonable.", AnsweredAt: "2020-01-01T00:00:00Z", Settled: 1},
	}}
	for _, closed := range []bool{false, true} {
		if closed {
			st.ClosedTopics = []string{"legacy"}
		}
		tv := topicView(t, st, LayoutFull, "legacy")
		entries := pageEntries(tv.Stream)
		if len(entries) != 2 || entries[0].ID != "item-9" || entries[1].ID != "item-2" {
			t.Fatalf("unknown-time rows reordered: %+v", entries)
		}
		for _, e := range entries {
			if e.At != "" || e.Clock != "" || e.Ms != 0 {
				t.Fatalf("manufactured event time: %+v", e)
			}
		}
		out := markup(render(t, BuildView(st, ViewInput{Layout: LayoutFull, ReplyID: "r"})))
		if strings.Count(out, `id="item-9"`) != 1 || strings.Count(out, `id="item-2"`) != 1 || strings.Count(out, "Time unknown") < 2 {
			t.Fatal("legacy rows must remain visible, uniquely anchored, and explicitly undated")
		}
	}
}

func TestFutureCreationCoalescingUsesOnlyKnownApplyMembership(t *testing.T) {
	st := &State{}
	mustApply(t, st, addXML(ClassTechnical, "t", "legacy one"))
	mustApply(t, st, addXML(ClassTechnical, "t", "legacy two"))
	original := append([]Entry(nil), st.Topics[0].Entries...)
	// The same supplied clock in separate updates does not establish a batch.
	if len(original) != 3 || original[0].ItemID != 1 || len(original[1].TopicState) == 0 || original[2].ItemID != 2 || original[0].At != original[2].At {
		t.Fatal("fixture requires separate equal-time creation updates")
	}
	mustApply(t, st, addXML(ClassTechnical, "t", "future one")+addXML(ClassCheck, "t", "future two"))
	entries := st.Topics[0].Entries
	if len(entries) != len(original)+1 || !reflect.DeepEqual(entries[:len(original)], original) {
		t.Fatalf("rewrote or regrouped legacy creation logs: %+v", entries)
	}
	last := entries[len(original)]
	if last.Kind != EntrySystem || last.ItemID != 0 || !strings.Contains(last.Text, "{{3}}") || !strings.Contains(last.Text, "{{4}}") || last.At != testNow.Format(time.RFC3339Nano) {
		t.Fatalf("missing known future creation batch: %+v", last)
	}
}

func TestHistoryRichDetailIsContentDrivenAndStatic(t *testing.T) {
	items := []Item{
		{ID: 1, Topic: "t", Class: ClassTechnical, Title: "short", Status: StatusAnswered, Answer: "ok", AnswerQuote: "ok"},
		{ID: 2, Topic: "t", Class: ClassPolicy, Title: "rich", Status: StatusAnswered, Answer: "b", AnswerQuote: "The second option preserves every useful detail.", Rationale: "| A | B |\n|---|---|\n| one | two |\n\n```go\nreturn true\n```\n\n<strong>Useful HTML</strong>", Options: []string{"one", "two"}, Recommendation: "b: keep details"},
		{ID: 3, Topic: "t", Class: ClassTechnical, Title: "title-like", Rationale: "title-like", Status: StatusAnswered, Answer: "ok", AnswerQuote: "ok"},
		{ID: 4, Topic: "t", Class: ClassTechnical, Title: "genuinely empty", Rationale: " \n\t ", Status: StatusAnswered, Answer: "ok", AnswerQuote: "ok"},
	}
	st := &State{Items: items}
	for _, regions := range []bool{false, true} {
		view := BuildView(st, ViewInput{Layout: LayoutFull, ReplyID: "r"})
		var out string
		if regions {
			r, err := RenderRegions(view)
			if err != nil {
				t.Fatal(err)
			}
			out = string(r.Board)
		} else {
			out = markup(render(t, view))
		}
		row := func(id int) string {
			return regexpHistoryArticle(out, fmt.Sprintf("item-%d", id))
		}
		wantNone(t, row(1), `<details`, `sbody`, `class="quote"`)
		wantAll(t, row(2), `<table`, `<pre`, `<strong>Useful HTML</strong>`, "keep details", "The second option preserves every useful detail.", `data-key="item:2"`)
		wantNone(t, row(2), `data-opt=`, `aria-pressed=`, `role="button"`)
		wantAll(t, row(3), `data-key="item:3"`, `class="ctx md">title-like`)
		wantNone(t, row(4), `<details`, `sbody`, `ctx md`)
	}
}

func TestHistoryLongAnswersShareTitleWidth(t *testing.T) {
	long := strings.TrimSpace(strings.Repeat("Preserve the complete policy explanation and natural wrapping. ", 12))
	st := &State{Items: []Item{
		{ID: 1, Topic: "t", Class: ClassPolicy, Title: "Long picked policy", Status: StatusAnswered, Answer: "a", Options: []string{long, "Other option"}, Rationale: "Keep the rationale", AnswerQuote: "Keep the owner quote"},
		{ID: 2, Topic: "t", Class: ClassTechnical, Title: "Long free answer", Status: StatusAnswered, Answer: long},
		{ID: 3, Topic: "t", Class: ClassTechnical, Title: "Correctable assumption", Status: StatusAssumed, Answer: long},
	}}
	view := BuildView(st, ViewInput{Layout: LayoutFull, ReplyID: "r"})
	regions, err := RenderRegions(view)
	if err != nil {
		t.Fatal(err)
	}
	for name, out := range map[string]string{"page": markup(render(t, view)), "regions": string(regions.Board)} {
		t.Run(name, func(t *testing.T) {
			for _, id := range []string{"item-1", "item-2", "item-3"} {
				row := regexpHistoryArticle(out, id)
				wantAll(t, row, long, `class="wb-hmain"`, `class="wb-hstatus"`)
				// The answer is inline inside the title column, not a fourth grid child.
				wantAll(t, row, `</b> <span class="wb-hstatus"`)
				wantNone(t, row, `</b></span><span class="wb-hstatus"`)
			}
			picked := regexpHistoryArticle(out, "item-1")
			wantAll(t, picked, `data-key="item:1"`, `class="pick"`, "Other option", "Keep the rationale", "Keep the owner quote")
			wantNone(t, picked, `data-opt=`, `aria-pressed=`, `role="button"`)
			wantAll(t, regexpHistoryArticle(out, "item-3"), `note-toggle`, `title="correction"`)
		})
	}
	css, err := whiteboardTestAsset("page.css")
	if err != nil {
		t.Fatal(err)
	}
	wantAll(t, string(css),
		`grid-template-columns: auto 20px minmax(0, 1fr);`,
		`.wb-hstatus { display: inline;`,
		`.wb-hstatus .pick { display: inline; box-decoration-break: clone; }`,
		`.wb-hitem details.sx > summary::before { align-self: flex-start; margin-top: 8px; }`,
		`.pick { display: inline-flex;`,
		`.wb-hitem .wb-htext { display: block; position: relative; padding-left: 26px;`)
	wantNone(t, string(css), `grid-template-columns: auto 20px minmax(0, 1fr) auto;`, `.wb-hstatus { max-width: 15em;`)
}

func TestHistoryAllAnsweredCheckKeepsVisibilityToggle(t *testing.T) {
	st := &State{}
	mustApply(t, st, addXML(ClassCheck, "only", "completed check"))
	mustApply(t, st, `<wb:answer id="1" value="ok">done</wb:answer>`)
	view := BuildView(st, ViewInput{Layout: LayoutFull, ReplyID: "r"})
	topic := topicView(t, st, LayoutFull, "only")
	if topic.ChecksShown() {
		t.Fatal("all-answered full history must not render an empty open-check list")
	}
	regions, err := RenderRegions(view)
	if err != nil {
		t.Fatal(err)
	}
	for name, out := range map[string]string{"page": markup(render(t, view)), "regions": string(regions.Board)} {
		t.Run(name, func(t *testing.T) {
			wantAll(t, out, `class="ckpill"`, `class="btn mini ckhide"`, `id="item-1"`, `wb-hitem check`)
			wantNone(t, out, `<div class="checks">`)
		})
	}

	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed; all-answered toggle runtime not executed")
	}
	board, _ := json.Marshal(string(regions.Board))
	answers, err := embeddedAsset("answers.js")
	if err != nil {
		t.Fatal(err)
	}
	form, err := embeddedAsset("form.js")
	if err != nil {
		t.Fatal(err)
	}
	script := "var BOARD = " + string(board) + ";\nvar BODY_CLASS = 'full';\n" + formPrelude + `
var stored = {};
window.localStorage = { getItem: function(k) { return stored[k] || null; }, setItem: function(k,v) { stored[k] = String(v); } };
` + string(answers) + string(form) + `
var toggle = document.querySelector('button.ckhide');
check('all-answered board retains its normally hidden history row', document.querySelector('.wb-hitem.check').getAttribute('id'), 'item-1');
check('fresh browser hides answered checks by default', document.documentElement.getAttribute('data-ck-hide'), '1');
check('the only topic still exposes its visibility toggle', !!toggle, true);
if (toggle) {
  click(toggle);
  check('the native handler exposes answered history', [document.documentElement.getAttribute('data-ck-hide'), toggle.getAttribute('aria-pressed'), stored['ws.wb.checks.hideAnswered']], ['0', 'false', '0']);
  click(toggle);
  check('the native handler restores the hidden state', document.documentElement.getAttribute('data-ck-hide'), '1');
}
process.stdout.write(bad.join('\n'));
`
	if out, err := nodeScript(node, script).CombinedOutput(); err != nil || len(out) != 0 {
		t.Fatalf("all-answered visibility toggle: %v\n%s", err, out)
	}
}

func TestHistoryCompletedCheckFilterPreservesUndatedAndAssumedRows(t *testing.T) {
	css, err := whiteboardTestAsset("page.css")
	if err != nil {
		t.Fatal(err)
	}
	wantAll(t, string(css), `html[data-ck-hide="1"] .topic:not(.closed) .wb-hitem.check:not([data-id]):not(.void):not(:has(.wb-hunknown)) { display: none; }`)
	st := &State{Items: []Item{
		{ID: 1, Topic: "legacy", Class: ClassCheck, Title: "undated completed check", Status: StatusAnswered, Answer: "ok"},
		{ID: 2, Topic: "legacy", Class: ClassCheck, Title: "undated assumed check", Status: StatusAssumed, Answer: "ok", AnswerQuote: "probably"},
		{ID: 3, Topic: "legacy", Class: ClassCheck, Title: "withdrawn check", Status: StatusWithdrawn},
		{ID: 4, Topic: "legacy", Class: ClassCheck, Title: "superseded check", Status: StatusSuperseded},
	}, Topics: []TopicDoc{{Name: "legacy", Entries: []Entry{
		{ID: "e1", Kind: EntrySystem, ItemID: 3, At: "2021-01-01T00:00:00Z", Text: "Item {{3}} patched [check] (open -> withdrawn): withdrawn check"},
		{ID: "e2", Kind: EntrySystem, ItemID: 4, At: "2022-01-01T00:00:00Z", Text: "Item {{4}} patched [check] (open -> superseded): superseded check"},
	}}}}
	out := markup(render(t, BuildView(st, ViewInput{Layout: LayoutFull, ReplyID: "r"})))
	wantAll(t, regexpHistoryArticle(out, "item-1"), `wb-hitem check`, `wb-hunknown`)
	wantAll(t, regexpHistoryArticle(out, "item-2"), `wb-hitem check wb-hassumed`, `data-id="02"`, `wb-hunknown`, `title="correction"`)
	for _, id := range []string{"item-3", "item-4"} {
		row := regexpHistoryArticle(out, id)
		wantAll(t, row, `class="srow wb-hitem check void"`, `datetime=`)
		wantNone(t, row, `wb-hunknown`)
	}
}

func TestHistoryRetirementRetainsRecordedPriorAnswer(t *testing.T) {
	st := &State{Items: []Item{{ID: 1, Topic: "t", Class: ClassTechnical, Title: "retired after approval", Status: StatusWithdrawn,
		Answer: "ok", AnswerQuote: "ok", AnsweredAt: "2020-01-01T00:00:00Z"}}, Topics: []TopicDoc{{Name: "t", Entries: []Entry{
		{ID: "e1", Kind: EntrySystem, ItemID: 1, At: "2020-01-01T00:00:00Z", Text: "Item {{1}} answered [technical]: ok; owner quote: ok"},
		{ID: "e2", Kind: EntrySystem, ItemID: 1, At: "2021-01-01T00:00:00Z", Text: "Item {{1}} patched [technical] (answered -> withdrawn): retired after approval"},
	}}}}
	out := markup(render(t, BuildView(st, ViewInput{Layout: LayoutFull, ReplyID: "r"})))
	row := regexpHistoryArticle(out, "item-1")
	wantAll(t, row, `data-entry="e2"`, `datetime="2021-01-01T00:00:00Z"`, `wb-hpast">answered: ok`, `datetime="2020-01-01T00:00:00Z"`, ">withdrawn</span>")
	wantNone(t, row, "archive time")
}

func regexpHistoryArticle(out, id string) string {
	at := strings.Index(out, `id="`+id+`"`)
	if at < 0 {
		return ""
	}
	start := strings.LastIndex(out[:at], "<article")
	if start < 0 {
		return ""
	}
	end := strings.Index(out[at:], "</article>")
	if end < 0 {
		return ""
	}
	return out[start : at+end+len("</article>")]
}
