package wstracker

import (
	"slices"
	"testing"
	"time"
)

// wrapUpView derives the wrap-up-statuses case (testdata/cases) in Go. The
// golden test holds it to the reference engine; these assertions pin what
// the case is for, so the engine and Go cannot drift together.
func wrapUpView(t *testing.T, viewer string) *View {
	t.Helper()
	doc, src := loadGolden(t, "testdata/golden/case-wrap-up-statuses__"+viewer+".json")
	now, err := time.Parse(time.RFC3339Nano, doc.Now)
	if err != nil {
		t.Fatal(err)
	}
	v := Derive(src, viewer, now, 0)
	for _, e := range v.Events {
		if e.Status != "applied" {
			t.Fatalf("%s is %s: %v", e.ID, e.Status, deref(e.Reason))
		}
	}
	return v
}

func itemStatusOf(t *testing.T, v *View, id string) string {
	t.Helper()
	it, ok := v.Items[id]
	if !ok || it.Status == nil {
		t.Fatalf("no status for %s", id)
	}
	return *it.Status
}

func asgOf(t *testing.T, v *View, id string) Assignment {
	t.Helper()
	a, ok := v.Assignments[id]
	if !ok {
		t.Fatalf("no assignment %s", id)
	}
	return a
}

func TestSuperiorWrapUp(t *testing.T) {
	v := wrapUpView(t, "ada.pc")
	// an indirect superior's done on a peer's assignment is final: no review
	a := asgOf(t, v, "raj.pc#2>lena")
	wantEq(t, "peer ask status", a.Status, "done")
	wantEq(t, "peer ask set by", deref(a.StatusBy), "ada")
	wantEq(t, "peer ask item", itemStatusOf(t, v, "raj.pc#1"), "done")
	// the direct superior's blocked, left by the assignee, then a superior's
	// blocked left by a superior's drop: the item shows dropped, uncounted
	a = asgOf(t, v, "maya.pc#2>lena")
	wantEq(t, "blocked twice status", a.Status, "dropped")
	wantEq(t, "blocked twice item", itemStatusOf(t, v, "maya.pc#1"), "dropped")
	wantEq(t, "dropped item counted", v.Items["maya.pc#1"].Counted, false)
	// the creator-fallback record takes a superior's blocked
	wantEq(t, "fallback item", itemStatusOf(t, v, "lena.pc#16"), "blocked")
	// a superior's done concurrent with the assignee's doing: one register
	wantEq(t, "concurrent status", asgOf(t, v, "maya.pc#10>lena").Status, "done")
}

func TestInReview(t *testing.T) {
	v := wrapUpView(t, "ada.pc")
	wantEq(t, "closed under assigner then confirmed", asgOf(t, v, "ada.pc#5>maya").Status, "done")
	wantEq(t, "rejected by a superior who is not the assigner", asgOf(t, v, "bo.pc#2>lena").Status, "doing")
	wantEq(t, "waiting review", asgOf(t, v, "bo.pc#4>lena").Status, "in-review")
	wantEq(t, "waiting review item", itemStatusOf(t, v, "bo.pc#3"), "in-review")
	wantEq(t, "self-assigned review resolved by its holder", asgOf(t, v, "lena.pc#8>lena").Status, "done")
	wantEq(t, "voluntary review left with a status write", asgOf(t, v, "lena.pc#12>lena").Status, "doing")
	wantEq(t, "voluntary review on an assignee-completion ask", asgOf(t, v, "ada.pc#8>raj").Status, "in-review")
	// an assign without completion keeps the fold's assignee default
	legacy := asgOf(t, v, "ada.pc#19>maya")
	wantEq(t, "legacy completion", legacy.Completion, "assignee")
	wantEq(t, "legacy close", legacy.Status, "done")
	// the assigner's confirm list; bo's waiting review is bo's
	if !slices.Equal(v.My.Today.Confirms, []string{"ada.pc#8>raj"}) {
		t.Fatalf("ada confirms = %v", v.My.Today.Confirms)
	}
	if bo := wrapUpView(t, "bo.pc"); !slices.Equal(bo.My.Today.Confirms, []string{"bo.pc#4>lena"}) {
		t.Fatalf("bo confirms = %v", bo.My.Today.Confirms)
	}
	// in-review is not work in hand
	if lena := wrapUpView(t, "lena.pc"); slices.Contains(lena.My.Today.Work, "bo.pc#4>lena") {
		t.Fatalf("lena work lists her in-review: %v", lena.My.Today.Work)
	}
}

func TestDroppedRollup(t *testing.T) {
	v := wrapUpView(t, "ada.pc")
	wantEq(t, "done and dropped mix", itemStatusOf(t, v, "ada.pc#9"), "done")
	wantEq(t, "reopened dropped", asgOf(t, v, "ada.pc#12>raj").Status, "doing")
	wantEq(t, "reopened item", itemStatusOf(t, v, "ada.pc#11"), "doing")
	r := v.Items["ada.pc#13"].Rollup
	wantEq(t, "parent roll-up total (the dropped child is out)", r.Total, 1)
	wantEq(t, "parent roll-up done", r.Counts["done"], 1)
	wantEq(t, "parent roll-up dropped", r.Counts["dropped"], 0)
	// the assigner hears of a drop someone else wrote
	var kinds []string
	for _, n := range v.Notices {
		if n.Assignment == "ada.pc#15>maya" {
			kinds = append(kinds, n.Kind)
		}
	}
	if !slices.Equal(kinds, []string{"dropped"}) {
		t.Fatalf("notices on the dropped child = %v", kinds)
	}
}
