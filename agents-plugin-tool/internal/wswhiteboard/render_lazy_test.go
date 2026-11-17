package wswhiteboard

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// lazyFixture is a board whose topic "lazy" holds an inert gap (batch 1: a
// panel and an agent line), a latest batch with an earlier and the latest
// version of p2, and ClosedCap+1 closed topics, so the oldest is capped.
// The topic has no Body: Apply would import one as a later legacy content
// panel, which would become the latest batch.
func lazyFixture(t *testing.T) *State {
	t.Helper()
	at := testNow.Format(time.RFC3339Nano)
	st := &State{Seq: 2, EntrySeq: 5, PanelCounter: 2, Topics: []TopicDoc{{Name: "lazy", Entries: []Entry{
		{ID: "e1", Kind: EntryPanel, PanelID: "p1", Recap: "gap panel", Text: "gap-era p1 payload", At: at, Seq: 1},
		{ID: "e2", Kind: EntryAgent, Text: "gap-era agent line", At: at, Seq: 1},
		{ID: "e3", Kind: EntryPanel, PanelID: "p2", Recap: "first p2", Text: "batch p2 first payload", At: at, Seq: 2},
		{ID: "e4", Kind: EntryPanel, PanelID: "p2", Recap: "latest p2", Text: "batch p2 latest payload", At: at, Seq: 2},
		{ID: "e5", Kind: EntryAgent, Text: "latest agent line", At: at, Seq: 2},
	}}}}
	var b strings.Builder
	for i := 0; i <= ClosedCap; i++ {
		fmt.Fprintf(&b, `<wb:topic name="c%02d"><wb:log>closed log c%02d</wb:log><wb:close result="done"/></wb:topic>`, i, i)
	}
	mustApply(t, st, b.String())
	return st
}

func renderLazyBoard(t *testing.T, st *State, lazy bool) string {
	t.Helper()
	view := BuildView(st, ViewInput{Layout: LayoutFull, ReplyID: "fixture", LazyHistory: lazy})
	if len(view.DeferredTopics) != 1 || view.DeferredTopics[0].Name != "c00" {
		t.Fatalf("fixture must cap exactly the oldest closed topic, got %d deferred", len(view.DeferredTopics))
	}
	regions, units, err := RenderRegionsWithTopics(view, nil)
	if err != nil {
		t.Fatal(err)
	}
	if units == nil {
		t.Fatal("lazy board must still split into topic units")
	}
	return string(regions.Board)
}

func TestLazyHistoryLeavesHistoryOutOfTheServedBoard(t *testing.T) {
	st := lazyFixture(t)
	lazy := renderLazyBoard(t, st, true)
	for _, want := range []string{
		`<template data-history-topic="c00" data-lazy></template>`,
		`<div class="history-gap" data-lazy`,
		`<template data-history-gap></template>`,
		`" data-lazy></div>`,
		"batch p2 latest payload",
		"latest agent line",
	} {
		if !strings.Contains(lazy, want) {
			t.Errorf("lazy board lacks %q", want)
		}
	}
	// The earlier p2 version's text may remain in the latest version's
	// diff gutter, but its own body is left out.
	for _, gone := range []string{"gap-era p1 payload", "gap-era agent line", "closed log c00"} {
		if strings.Contains(lazy, gone) {
			t.Errorf("lazy board still carries history %q", gone)
		}
	}
	_, e3, _ := strings.Cut(lazy, `data-entry="e3"`)
	e3, _, _ = strings.Cut(e3, "</article>")
	if !strings.Contains(e3, `" data-lazy></div>`) {
		t.Errorf("the earlier p2 version's body is not lazy:\n%s", e3)
	}
	eager := renderLazyBoard(t, st, false)
	if strings.Contains(eager, "data-lazy") {
		t.Error("an eager board must carry no lazy marker")
	}
	for _, want := range []string{"gap-era p1 payload", "gap-era agent line", "batch p2 first payload", "closed log c00"} {
		if !strings.Contains(eager, want) {
			t.Errorf("eager board lost %q", want)
		}
	}
	if !(len(lazy) < len(eager)) {
		t.Errorf("lazy board (%d B) must be smaller than the eager one (%d B)", len(lazy), len(eager))
	}
}

func TestRenderTopicFragmentsCarryCompleteHistory(t *testing.T) {
	st := lazyFixture(t)
	view := BuildView(st, ViewInput{Layout: LayoutFull, ReplyID: "fixture", LazyHistory: true})
	frags, err := RenderTopicFragments(view, []string{"lazy", "c00", "missing", "lazy"})
	if err != nil {
		t.Fatal(err)
	}
	if len(frags) != 2 || frags[0].Name != "lazy" || frags[1].Name != "c00" {
		t.Fatalf("fragments = %+v, want lazy then c00 once each", frags)
	}
	lazy, closed := string(frags[0].HTML), string(frags[1].HTML)
	if strings.Contains(lazy, "data-lazy") || strings.Contains(closed, "data-lazy") {
		t.Error("a fragment must be complete, without lazy markers")
	}
	for _, want := range []string{"gap-era p1 payload", "gap-era agent line", "batch p2 first payload", "batch p2 latest payload", `data-th="`} {
		if !strings.Contains(lazy, want) {
			t.Errorf("topic fragment lacks %q", want)
		}
	}
	if !strings.HasPrefix(lazy, `<section `) || !strings.HasSuffix(lazy, "</section>") {
		t.Error("a fragment is exactly one topic section")
	}
	if !strings.Contains(closed, "closed log c00") || !strings.Contains(closed, `data-topic="c00"`) {
		t.Error("a capped closed topic renders as its own section")
	}
}

func TestStateTargetTopic(t *testing.T) {
	st := lazyFixture(t)
	mustApply(t, st, `<wb:topic name="c05"><wb:add class="technical"><wb:title>a check</wb:title></wb:add></wb:topic>`)
	item := st.Items[len(st.Items)-1].ID
	for target, want := range map[string]string{
		"e3": "lazy", "p1": "lazy", "p2": "lazy",
		fmt.Sprintf("item-%d", item): "c05",
	} {
		if got, ok := st.TargetTopic(target); !ok || got != want {
			t.Errorf("TargetTopic(%q) = %q, %t; want %q", target, got, ok, want)
		}
	}
	for _, target := range []string{"e999", "p999", "item-999", "topic-x"} {
		if got, ok := st.TargetTopic(target); ok {
			t.Errorf("TargetTopic(%q) = %q, want none", target, got)
		}
	}
}
