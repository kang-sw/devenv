package wstracker

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsledger"
)

// labelView derives the view of a linear history over the reasons genesis
// (ada and bo are admins) and requires every event applied.
func labelView(t *testing.T, steps []step) *View {
	t.Helper()
	v := Derive(newMemSource(loadReasonCases(t).Genesis, "ada.pc", history(steps)), "ada.pc", time.Time{}, 0)
	for _, e := range v.Events {
		if e.Status != "applied" {
			t.Fatalf("%s %s is %s: %v", e.ID, e.Type, e.Status, deref(e.Reason))
		}
	}
	return v
}

func labelMatches(t *testing.T, v *View, q, id string) bool {
	t.Helper()
	pq, err := ParseQuery(q)
	if err != nil {
		t.Fatal(err)
	}
	return pq.Match(v, id)
}

// TestItemLabelsFollowTheLabelList covers a label's life on an item: the item
// carries label ids, so a rename shows on it, a removal takes the label off
// it, its chips, and label: results, and re-adding a removed name is a new
// label the item does not regain.
func TestItemLabelsFollowTheLabelList(t *testing.T) {
	steps := []step{
		{"ada.pc", TypeItemCreate, map[string]any{"kind": "task", "title": "Ship"}},
		{"ada.pc", TypeAddLabel, map[string]any{"name": "bug", "color": "#d14343"}},
		{"ada.pc", TypeAddLabel, map[string]any{"name": "ui", "color": "#3366cc"}},
		{"ada.pc", TypeItemEdit, map[string]any{"item": "ada.pc#1", "labels": []string{"ada.pc#2", "ada.pc#3"}}},
	}
	v := labelView(t, steps)
	if got := v.Items["ada.pc#1"].Labels; !reflect.DeepEqual(got, []string{"ada.pc#2", "ada.pc#3"}) {
		t.Fatalf("item labels %v", got)
	}
	if want := []LabelView{{"ada.pc#2", "bug", "#d14343"}, {"ada.pc#3", "ui", "#3366cc"}}; !reflect.DeepEqual(v.Labels, want) {
		t.Fatalf("label list %v, want %v", v.Labels, want)
	}
	if !labelMatches(t, v, "label:BUG", "ada.pc#1") || !labelMatches(t, v, `label:"ui"`, "ada.pc#1") || labelMatches(t, v, "label:bu", "ada.pc#1") {
		t.Fatal("label: does not match the item's labels by exact name, ignoring case")
	}

	steps = append(steps, step{"ada.pc", TypeEditLabel, map[string]any{"label": "ada.pc#2", "name": "defect", "color": "#aa0000"}})
	v = labelView(t, steps)
	if got := v.Items["ada.pc#1"].Labels; !reflect.DeepEqual(got, []string{"ada.pc#2", "ada.pc#3"}) {
		t.Fatalf("a rename changed the item's label ids: %v", got)
	}
	if v.Labels[0] != (LabelView{"ada.pc#2", "defect", "#aa0000"}) {
		t.Fatalf("renamed label shows %v", v.Labels[0])
	}
	if !labelMatches(t, v, "label:defect", "ada.pc#1") || labelMatches(t, v, "label:bug", "ada.pc#1") {
		t.Fatal("label: does not follow the rename")
	}

	steps = append(steps, step{"bo.pc", TypeRemoveLabel, map[string]any{"label": "ada.pc#2"}})
	v = labelView(t, steps)
	if got := v.Items["ada.pc#1"].Labels; !reflect.DeepEqual(got, []string{"ada.pc#3"}) {
		t.Fatalf("a removed label stays on the item: %v", got)
	}
	if len(v.Labels) != 1 || v.Labels[0].ID != "ada.pc#3" {
		t.Fatalf("a removed label stays in the list: %v", v.Labels)
	}
	if labelMatches(t, v, "label:defect", "ada.pc#1") {
		t.Fatal("label: still matches a removed label")
	}

	steps = append(steps, step{"ada.pc", TypeAddLabel, map[string]any{"name": "Defect", "color": "#aa0000"}})
	v = labelView(t, steps)
	if want := []LabelView{{"ada.pc#3", "ui", "#3366cc"}, {"ada.pc#6", "Defect", "#aa0000"}}; !reflect.DeepEqual(v.Labels, want) {
		t.Fatalf("re-added name: label list %v, want %v", v.Labels, want)
	}
	if got := v.Items["ada.pc#1"].Labels; !reflect.DeepEqual(got, []string{"ada.pc#3"}) {
		t.Fatalf("the item regained a re-added name: %v", got)
	}
	if labelMatches(t, v, "label:defect", "ada.pc#1") {
		t.Fatal("label: matches the item through a re-added name")
	}

	// a create carries labels too, and null on edit clears them
	steps = append(steps,
		step{"maya.pc", TypeItemCreate, map[string]any{"kind": "task", "title": "x", "labels": []string{"ada.pc#6"}}},
		step{"ada.pc", TypeItemEdit, map[string]any{"item": "ada.pc#1", "labels": nil}})
	v = labelView(t, steps)
	if got := v.Items["maya.pc#1"].Labels; !reflect.DeepEqual(got, []string{"ada.pc#6"}) {
		t.Fatalf("created item labels %v", got)
	}
	if got := v.Items["ada.pc#1"].Labels; len(got) != 0 {
		t.Fatalf("labels: null left %v", got)
	}
}

// TestConcurrentLabelNameKeepsTheFirstInFoldOrder: two admins add one name
// concurrently; each passes its causal check, the first in fold order keeps
// the name, and the later one is flagged naming the holder. The flagged add
// still counts for later checks of those who saw it: its writer's own
// rename to the name is a plain duplicate (the concurrent-rename clash is
// the labels-concurrent-name golden case). An item written by one who saw only a
// flagged add stays, and the view drops the lost label from it.
func TestConcurrentLabelNameKeepsTheFirstInFoldOrder(t *testing.T) {
	at := func(m int) time.Time { return time.Date(2026, 11, 1, 10, m, 0, 0, time.UTC) }
	ev := func(member string, seq, m int, typ string, p map[string]any, seen map[string]int) wsledger.Event {
		raw, _ := json.Marshal(p)
		return wsledger.Event{ID: wsledger.EventID{Member: member, Clone: "pc", Seq: seq}, TS: at(m), Type: typ,
			Payload: raw, Seen: seen, Verdict: wsledger.Valid}
	}
	evs := []wsledger.Event{
		ev("ada", 1, 0, TypeAddLabel, map[string]any{"name": "ui", "color": "#3366cc"}, map[string]int{}),
		ev("bo", 1, 1, TypeAddLabel, map[string]any{"name": "Bug", "color": "#d14343"}, map[string]int{"ada.pc": 1}),
		ev("ada", 2, 2, TypeAddLabel, map[string]any{"name": "bug", "color": "#000000"}, map[string]int{}),
		ev("ada", 3, 3, TypeEditLabel, map[string]any{"label": "ada.pc#1", "name": "BUG"}, map[string]int{}),
		// maya saw only ada's flagged add: her writes carrying it stay applied
		ev("maya", 1, 4, TypeItemCreate, map[string]any{"kind": "task", "title": "Docs", "labels": []string{"ada.pc#2"}}, map[string]int{"ada.pc": 2}),
		ev("maya", 2, 5, TypeItemEdit, map[string]any{"item": "maya.pc#1", "title": "Docs v2", "labels": []string{"ada.pc#2"}}, map[string]int{"ada.pc": 2, "maya.pc": 1}),
	}
	v := Derive(newMemSource(loadReasonCases(t).Genesis, "ada.pc", evs), "ada.pc", time.Time{}, 0)
	got := map[string]string{}
	for _, e := range v.Events {
		got[e.ID] = e.Status + " " + deref(e.Reason)
	}
	want := map[string]string{
		"ada.pc#1": "applied ",
		"bo.pc#1":  "applied ",
		"ada.pc#2": `flagged label "bug" already exists (bo.pc-1, written concurrently; the first in fold order keeps the name)`,
		// ada saw her own add of bug, flagged or not: her rename is a plain duplicate
		"ada.pc#3": `flagged label "BUG" already exists (ada.pc-2)`,
		"maya.pc#1": "applied ",
		"maya.pc#2": "applied ",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("verdicts %v, want %v", got, want)
	}
	if want := []LabelView{{"ada.pc#1", "ui", "#3366cc"}, {"bo.pc#1", "Bug", "#d14343"}}; !reflect.DeepEqual(v.Labels, want) {
		t.Fatalf("label list %v, want %v", v.Labels, want)
	}
	if it, ok := v.Items["maya.pc#1"]; !ok || it.Title != "Docs v2" || len(it.Labels) != 0 {
		t.Fatalf("the item carrying the lost label: %+v", it)
	}
}
