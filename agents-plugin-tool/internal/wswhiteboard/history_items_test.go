package wswhiteboard

import (
	"fmt"
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
	if len(original) != 2 || original[0].At != original[1].At {
		t.Fatal("fixture requires separate equal-time creation updates")
	}
	mustApply(t, st, addXML(ClassTechnical, "t", "future one")+addXML(ClassCheck, "t", "future two"))
	entries := st.Topics[0].Entries
	if len(entries) != 3 || !reflect.DeepEqual(entries[:2], original) {
		t.Fatalf("rewrote or regrouped legacy creation logs: %+v", entries)
	}
	last := entries[2]
	if last.Kind != EntrySystem || last.ItemID != 0 || !strings.Contains(last.Text, "{{3}}") || !strings.Contains(last.Text, "{{4}}") || last.At != testNow.Format(time.RFC3339Nano) {
		t.Fatalf("missing known future creation batch: %+v", last)
	}
}

func TestHistoryRichDetailIsContentDrivenAndStatic(t *testing.T) {
	items := []Item{
		{ID: 1, Topic: "t", Class: ClassTechnical, Title: "short", Status: StatusAnswered, Answer: "ok", AnswerQuote: "ok"},
		{ID: 2, Topic: "t", Class: ClassPolicy, Title: "rich", Status: StatusAnswered, Answer: "b", AnswerQuote: "The second option preserves every useful detail.", Rationale: "| A | B |\n|---|---|\n| one | two |\n\n```go\nreturn true\n```\n\n<strong>Useful HTML</strong>", Options: []string{"one", "two"}, Recommendation: "b: keep details"},
		{ID: 3, Topic: "t", Class: ClassTechnical, Title: "title-like", Rationale: "title-like", Status: StatusAnswered, Answer: "ok", AnswerQuote: "ok"},
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
	}
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
