package wswhiteboard

import (
	"encoding/json"
	"fmt"
	"regexp"
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

// allTimelineRows returns live and folded rows in timeline order.
func allTimelineRows(tv TopicView) []timelineRow {
	var out []timelineRow
	for _, p := range tv.Timeline {
		out = append(out, p.Rows...)
	}
	return out
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
	asks, answers := 0, 0
	for _, row := range live {
		if row.Ask {
			asks += max(1, len(row.Created))
		}
		if row.Answer {
			answers++
		}
	}
	// Ask lines, open or not, fold like logs; the latest batch (the answer)
	// stays live. The open ask stacks after the history instead.
	if asks != 0 || answers != 1 || len(held) < 100 {
		t.Fatalf("anchors: asks=%d answers=%d hidden=%d", asks, answers, len(held))
	}
	if len(tv.Asks) != 1 || tv.Asks[0].Kind != "ack" || len(tv.Asks[0].Asks) != 1 || tv.Asks[0].Asks[0].Item.ID != 2 || tv.Asks[0].Asks[0].Entry == "" {
		t.Fatalf("open asks: %+v", tv.Asks)
	}
	for i, p := range tv.Timeline {
		if p.Gap && (p.Count < 1 || p.From == "" || p.To == "" || p.Above != (i > 0)) {
			t.Fatalf("gap descriptor: %+v", p)
		}
	}
	out := markup(render(t, BuildView(st, ViewInput{Layout: LayoutFull})))
	wantAll(t, out, `class="history-gap"`, `<template data-history-gap>`, `id="item-1"`, `<div class="tbody wb-asks"><div class="acks"><div class="arow" id="item-2"`, `<a class="wb-askjump" href="#`+tv.Asks[0].Asks[0].Entry+`"`, `Original question`)
	wantNone(t, out, `class="tbody"`, `question-lead`, `history-clip`, `history-older`, `data-history-page`)
	if strings.Count(out, `id="item-1"`) != 1 || strings.Count(out, `id="item-2"`) != 1 {
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
	live := allTimelineRows(tv)
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
	live = allTimelineRows(BuildView(st, ViewInput{Layout: LayoutFull, Location: time.UTC}).Topics[0])
	for _, row := range live {
		if row.LatestAnswer && row.Clock != "12:30" {
			t.Fatal("legacy fallback", row.Clock)
		}
	}
}

func TestTimelineConvertedItemsStackOpenAsksAndKeepSettledAtHead(t *testing.T) {
	legacy := &State{Items: []Item{
		{ID: 2, Topic: "t", Class: ClassTechnical, Title: "second", Status: StatusOpen},
		{ID: 1, Topic: "t", Class: ClassPolicy, Title: "first", Options: []string{"A", "B"}, Status: StatusOpen},
		{ID: 3, Topic: "t", Class: ClassTechnical, Title: "answered", Status: StatusAnswered, Answer: "ok", AnswerQuote: "retained"},
	}, Topics: []TopicDoc{{Name: "t", Recap: "converted log"}}}
	st := folderOpen(t, t.TempDir(), legacy)
	tv := topicView(t, st, LayoutFull, "t")
	live := allTimelineRows(tv)
	if live[0].Item == nil || live[0].Item.ID != 3 || live[0].Clock != "" {
		t.Fatal("settled conversion not at the timeline head", live)
	}
	// Open converted items keep stored order at the bottom, with no
	// ask-time line to jump to.
	var stacked []int
	for _, g := range tv.Asks {
		for _, a := range g.Asks {
			stacked = append(stacked, a.Item.ID)
			if a.Entry != "" || a.Clock != "" {
				t.Fatal("converted ask invented a time", a)
			}
		}
	}
	if fmt.Sprint(stacked) != "[2 1]" {
		t.Fatal("open conversion order", stacked)
	}
	out := markup(render(t, BuildView(st, ViewInput{Layout: LayoutFull})))
	wantAll(t, out, `id="item-1" data-id="01"`, `data-opt="a"`, `id="item-2" data-id="02"`, `id="item-3"`, `retained`)
	wantNone(t, out, `class="wb-askjump"`)
	if strings.Index(out, `id="item-3"`) > strings.Index(out, `id="e1"`) || strings.Index(out, `id="item-2"`) < strings.Index(out, `id="item-3"`) || strings.Index(out, `id="item-1"`) < strings.Index(out, `id="item-2"`) {
		t.Fatal("orphans not at timeline head or open asks not stacked in stored order")
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

// Owner-input clocks carry the same data-at/data-stamp as agent entry clocks,
// so ago.js gives owner rows the same relative label as lead rows.
func TestTimelineInputHeaderClockIsRelativeTimeTarget(t *testing.T) {
	sent := time.Date(2026, 11, 15, 21, 35, 0, 0, time.UTC).UnixMilli()
	st := &State{Seq: 2, Items: []Item{{ID: 1, Topic: "a", Class: ClassTechnical, Title: "yes", Status: StatusOpen}}, Topics: []TopicDoc{
		{Name: "a", Entries: []Entry{{ID: "e1", Kind: EntrySystem, ItemID: 1, Text: "Item {{1}} added [technical]: yes", Seq: 2, Inputs: json.RawMessage(fmt.Sprintf(`[{"inbox_id":"x","sent_at":%d,"excerpt":"hello","chip_ids":[]}]`, sent))}}},
	}}
	out := markup(render(t, BuildView(st, ViewInput{Layout: LayoutFull, Location: time.UTC})))
	wantAll(t, out, fmt.Sprintf(`<span class="evi owner">O</span><time class="entry-at" title="2026-11-15 21:35" data-stamp="2026-11-15 21:35" data-at="%d">21:35</time>`, sent))
}

// Every history clock goes through the shared "clock" template, so no row kind
// can drop the data-at/data-stamp that ago.js needs for its relative label.
// The board holds every row kind: lead log, owner input, an ask of each class
// (check included), an owner answer, a panel and topic-state/system rows.
func TestEveryHistoryClockIsRelativeTimeTarget(t *testing.T) {
	st := &State{}
	mustApply(t, st, logXML("a", "lead note"))
	mustApply(t, st, addXML(ClassPolicy, "a", "pick", "x", "y"))
	mustApply(t, st, addXML(ClassTechnical, "a", "tech"))
	mustApply(t, st, addXML(ClassAnnounced, "a", "said"))
	mustApply(t, st, addXML(ClassCheck, "a", "look"))
	mustApply(t, st, panelXML("a", "panel recap", "<p>panel</p>"))
	mustApply(t, st, `<wb:answer ids="1" value="a">yes, x</wb:answer>`)
	mustApply(t, st, logXML("a", "after the answer"))
	doc := &st.Topics[0]
	var last *Entry
	for i := range doc.Entries {
		if doc.Entries[i].ItemID == 4 { // the check ask carries the owner message
			last = &doc.Entries[i]
		}
	}
	if last.Seq == 0 {
		last.Seq = 99 // owner inputs attach to a recorded update batch
	}
	sent := time.Date(2026, 11, 15, 21, 35, 0, 0, time.UTC).UnixMilli()
	last.Inputs = json.RawMessage(fmt.Sprintf(`[{"inbox_id":"x","sent_at":%d,"excerpt":"owner says","chip_ids":["4"]}]`, sent))

	out := markup(render(t, BuildView(st, ViewInput{Layout: LayoutFull, Location: time.UTC})))
	clocks := historyClock.FindAllStringSubmatch(out, -1)
	if len(clocks) == 0 {
		t.Fatalf("no entry-at clocks rendered:\n%s", out)
	}
	var ask, owner bool
	for _, m := range clocks {
		if m[2] == "" {
			continue
		}
		if !strings.Contains(m[1], ` data-at="`) || !strings.Contains(m[1], ` data-stamp="`) {
			t.Errorf("history clock without relative-time data: %s", m[0])
		}
		ask = ask || strings.Contains(m[1], "wb-htime") && !strings.Contains(m[1], "datetime=\"\"")
		owner = owner || !strings.Contains(m[1], "datetime=")
	}
	if !ask || !owner {
		t.Fatalf("board did not render both ask-row and owner-input clocks (ask=%v owner=%v):\n%s", ask, owner, out)
	}
	if !strings.Contains(out, `data-kind`) {
		t.Fatalf("history rows missing:\n%s", out)
	}
}

var historyClock = regexp.MustCompile(`<time (class="entry-at[^"]*"[^>]*)>([^<]*)</time>`)
