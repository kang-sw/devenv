package wswhiteboard

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

func timelineRows(tv TopicView) (live, held []timelineRow) {
	for _, p := range tv.Timeline {
		if p.Gap {
			held = append(held, p.Rows...)
		} else {
			live = append(live, p.Rows...)
		}
	}
	return
}

func TestTimelineAnchorsOutsidePagingAndGapRanges(t *testing.T) {
	st := &State{}
	mustApply(t, st, `<wb:topic name="t"><wb:panel recap="question context">rich context</wb:panel><wb:add class="policy"><wb:title>Choose</wb:title><wb:option>A</wb:option><wb:option>B</wb:option></wb:add><wb:add class="technical"><wb:title>Confirm</wb:title></wb:add></wb:topic>`)
	for i := 0; i < 120; i++ {
		mustApply(t, st, `<wb:topic name="t"><wb:log>line `+fmt.Sprint(i)+`</wb:log></wb:topic>`)
	}
	mustApply(t, st, `<wb:answer id="1" value="a">first quote</wb:answer>`)
	tv := topicView(t, st, LayoutFull, "t")
	live, held := timelineRows(tv)
	asks, answers, preceding := 0, 0, 0
	for _, row := range live {
		if row.Ask {
			asks += max(1, len(row.Created))
		}
		if row.Answer {
			answers++
		}
		if row.Preceding {
			preceding++
			if !row.Page.Runs[0].Entry.Open {
				t.Fatal("question panel folded")
			}
		}
	}
	if asks != 2 || answers != 1 || preceding != 1 || len(held) < 100 {
		t.Fatalf("anchors: asks=%d answers=%d preceding=%d hidden=%d", asks, answers, preceding, len(held))
	}
	for i, p := range tv.Timeline {
		if p.Gap && (p.Count < 1 || p.From == "" || p.To == "" || p.Above != (i > 0)) {
			t.Fatalf("gap descriptor: %+v", p)
		}
	}
	out := markup(render(t, BuildView(st, ViewInput{Layout: LayoutFull})))
	wantAll(t, out, `class="history-gap"`, `<template data-history-gap>`, `id="item-1"`, `id="item-2"`, `Original question`, `question-lead`)
	wantNone(t, out, `class="tbody"`, `history-clip`, `history-older`, `data-history-page`)
	if strings.Count(out, `id="item-1"`) != 1 {
		t.Fatal("question anchor duplicated")
	}
}

func TestTimelineReaskKeepsFirstPositionAndEveryAnswer(t *testing.T) {
	st := &State{}
	mustApply(t, st, `<wb:topic name="t"><wb:log>Before</wb:log><wb:add class="policy"><wb:title>Choose</wb:title><wb:option>A</wb:option><wb:option>B</wb:option></wb:add></wb:topic>`)
	mustApply(t, st, `<wb:answer id="1" value="a">old quote</wb:answer>`)
	mustApply(t, st, `<wb:patch id="1" status="open"/>`)
	reasked := markup(render(t, BuildView(st, ViewInput{Layout: LayoutFull})))
	wantAll(t, reasked, "old quote", `id="item-1" data-id="01"`, `Asked again`)
	wantNone(t, reasked, `— <b></b>`)
	assumed := st.clone()
	mustApply(t, assumed, `<wb:answer id="1" value="b" assumed="true">agent inference</wb:answer>`)
	assumedRows, _ := timelineRows(topicView(t, assumed, LayoutFull, "t"))
	for _, row := range assumedRows {
		if row.Answer && row.LatestAnswer {
			t.Fatal("agent assumption replaced the historical owner answer")
		}
	}
	mustApply(t, st, `<wb:answer id="1" value="b">new quote</wb:answer>`)
	st.Items[0].AnsweredAt = "2026-11-15T12:30:00Z"
	st.Topics[0].Entries[len(st.Topics[0].Entries)-1].AnswerSentAt = time.Date(2026, 11, 15, 12, 29, 0, 0, time.UTC).UnixMilli()
	tv := BuildView(st, ViewInput{Layout: LayoutFull, Location: time.UTC}).Topics[0]
	live, _ := timelineRows(tv)
	var asks, answers int
	for _, row := range live {
		if row.Ask {
			asks++
			if row.Reasked == "" {
				t.Fatal("reask clock absent")
			}
		}
		if row.Answer {
			answers++
			if row.LatestAnswer && row.Clock != "12:29" {
				t.Fatal("not owner send time", row.Clock)
			}
		}
	}
	if asks != 1 || answers != 2 {
		t.Fatalf("ask/answer counts %d/%d", asks, answers)
	}
	out := markup(render(t, BuildView(st, ViewInput{Layout: LayoutFull, Location: time.UTC})))
	wantAll(t, out, `Asked again`, "old quote", "new quote", `12:29`)
	// Legacy, unmatched chips use the latest item's AnsweredAt, not a fabricated stamp.
	st.Topics[0].Entries[len(st.Topics[0].Entries)-1].AnswerSentAt = 0
	live, _ = timelineRows(BuildView(st, ViewInput{Layout: LayoutFull, Location: time.UTC}).Topics[0])
	for _, row := range live {
		if row.LatestAnswer && row.Clock != "12:30" {
			t.Fatal("legacy fallback", row.Clock)
		}
	}
}

func TestTimelineConvertedItemsKeepControlsAtHead(t *testing.T) {
	legacy := &State{Items: []Item{
		{ID: 2, Topic: "t", Class: ClassTechnical, Title: "second", Status: StatusOpen},
		{ID: 1, Topic: "t", Class: ClassPolicy, Title: "first", Options: []string{"A", "B"}, Status: StatusOpen},
		{ID: 3, Topic: "t", Class: ClassTechnical, Title: "answered", Status: StatusAnswered, Answer: "ok", AnswerQuote: "retained"},
	}, Topics: []TopicDoc{{Name: "t", Recap: "converted log"}}}
	st := folderOpen(t, t.TempDir(), legacy)
	tv := topicView(t, st, LayoutFull, "t")
	live, _ := timelineRows(tv)
	for i, id := range []int{2, 1, 3} {
		if live[i].Item == nil || live[i].Item.ID != id || live[i].Clock != "" {
			t.Fatal("conversion order/time", live)
		}
	}
	out := markup(render(t, BuildView(st, ViewInput{Layout: LayoutFull})))
	wantAll(t, out, `id="item-1" data-id="01"`, `data-opt="a"`, `id="item-2" data-id="02"`, `id="item-3"`, `retained`)
	if strings.Index(out, `id="item-3"`) > strings.Index(out, `id="e1"`) {
		t.Fatal("orphans not at timeline head")
	}
}

func TestTimelineInputHeadersAreTopicLocalAndEscaped(t *testing.T) {
	st := &State{Seq: 2, Items: []Item{{ID: 1, Topic: "a", Class: ClassTechnical, Title: "yes", Status: StatusOpen}}, Topics: []TopicDoc{
		{Name: "a", Entries: []Entry{{ID: "e1", Kind: EntrySystem, ItemID: 1, Text: "Item {{1}} added [technical]: yes", Seq: 2, Inputs: json.RawMessage(`[{"inbox_id":"x","sent_at":1000,"excerpt":"<script>x</script>","chip_ids":["1"]}]`)}}},
		{Name: "b", Entries: []Entry{{ID: "e2", Kind: EntryAgent, Text: "updated", Seq: 2}}},
		{Name: "c", Entries: []Entry{{ID: "e3", Kind: EntrySystem, Text: "state metadata only", Seq: 2}}},
	}}
	view := BuildView(st, ViewInput{Layout: LayoutFull})
	for _, tv := range view.Topics {
		live, _ := timelineRows(tv)
		n := 0
		for _, row := range live {
			n += len(row.Inputs)
			if tv.Name == "b" && len(row.Inputs) > 0 && len(row.Inputs[0].ChipIDs) > 0 {
				t.Fatal("foreign answer chips")
			}
		}
		if (tv.Name == "c" && n != 0) || (tv.Name != "c" && n != 1) {
			t.Fatal("header attribution", tv.Name, n)
		}
	}
	out := markup(render(t, view))
	wantAll(t, out, `&lt;script&gt;x&lt;/script&gt;`, `class="owner-input"`)
	wantNone(t, out, `<script>x</script>`)
	st.Topics[0].Entries[0].Inputs = nil
	view = BuildView(st, ViewInput{Layout: LayoutFull})
	wantNone(t, markup(render(t, view)), `class="owner-input"`)
}
