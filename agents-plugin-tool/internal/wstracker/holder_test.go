package wstracker

import (
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsledger"
)

// Who holds an item: rule requests, which hold nothing, and their withdrawal
// one rule at a time. On the reasons.json genesis maya leads lena and omar,
// raj leads sven, so a rule maya roots at raj is a request to raj.

func TestUnassignOneRuleKeepsTheOtherOnTheSameRoot(t *testing.T) {
	base := []step{
		{"maya.pc", "item.create", map[string]any{"kind": "task", "title": "Index"}},
		// rule A: everyone one level below raj; raj accepts, so it goes live.
		{"maya.pc", "item.assign", map[string]any{"item": "maya.pc#1", "targets": []any{map[string]any{"root": "raj", "levels": 1}}}},
		{"raj.pc", "item.accept", map[string]any{"item": "maya.pc#1"}},
		// rule B: raj too; a second request to raj, pending.
		{"maya.pc", "item.assign", map[string]any{"item": "maya.pc#1", "targets": []any{map[string]any{"root": "raj", "include_root": true, "levels": 1}}}},
	}
	ruleA, ruleB := "maya.pc#2~raj", "maya.pc#3~raj"

	v := topicView(t, "maya.pc", append(append([]step(nil), base...),
		step{"maya.pc", "item.unassign", map[string]any{"item": "maya.pc#1", "root": "raj", "rule": ruleB}}))
	wantEq(t, "rule B", v.Rules[ruleB].State, "ended")
	wantEq(t, "rule B request", v.Assignments[ruleB].State, "withdrawn")
	wantEq(t, "rule A", v.Rules[ruleA].State, "live")
	wantEq(t, "sven under rule A", v.Assignments[ruleA+">sven"].State, "live")

	// Without a rule id, every open rule of the author's rooted at raj ends.
	v = topicView(t, "maya.pc", append(append([]step(nil), base...),
		step{"maya.pc", "item.unassign", map[string]any{"item": "maya.pc#1", "root": "raj"}}))
	wantEq(t, "rule A by root", v.Rules[ruleA].State, "ended")
	wantEq(t, "rule B by root", v.Rules[ruleB].State, "ended")
	wantEq(t, "sven by root", v.Assignments[ruleA+">sven"].State, "unassigned")
}

func TestPendingRuleRequestIsNotPending(t *testing.T) {
	v := topicView(t, "maya.pc", []step{
		{"maya.pc", "item.create", map[string]any{"kind": "task", "title": "Index"}},
		{"maya.pc", "item.assign", map[string]any{"item": "maya.pc#1", "targets": []any{map[string]any{"root": "raj"}}}},
	})
	wantEq(t, "request", v.Assignments["maya.pc#2~raj"].State, "pending")
	// (Decision 6) the creator holds it until raj answers, at todo.
	wantEq(t, "status", deref(v.Items["maya.pc#1"].Status), "todo")
	fb := v.Assignments["maya.pc#1@maya.pc#1>maya"]
	wantEq(t, "creator holds it", fb.State+" "+fb.To, "live maya")
	wantIn(t, "in maya's mine", v.My.Assignments, fb.ID)
}

// fallbackOn is the one live fallback on item, as "<id> <status>".
func fallbackOn(t *testing.T, v *View, item string) string {
	t.Helper()
	var out []string
	for _, a := range openOn(v, item) {
		if a.Fallback {
			out = append(out, a.ID+" "+a.Status)
		}
	}
	if len(out) != 1 {
		t.Fatalf("fallbacks on %s: %v (open %v)", item, out, openOn(v, item))
	}
	return out[0]
}

func TestFallbackHoldsATaskNobodyElseHolds(t *testing.T) {
	create := step{"maya.pc", "item.create", map[string]any{"kind": "task", "title": "Index"}}
	v := topicView(t, "maya.pc", []step{create})
	wantEq(t, "a new task", fallbackOn(t, v, "maya.pc#1"), "maya.pc#1@maya.pc#1>maya todo")
	wantEq(t, "its status", deref(v.Items["maya.pc#1"].Status), "todo")
	wantEq(t, "counted", v.Items["maya.pc#1"].Counted, true)
	if len(v.My.Created) != 0 {
		t.Fatalf("a held task is not in created: %v", v.My.Created)
	}

	// Every path that ends the last holding lands on the creator, in a record
	// named after the event that ended it.
	for _, c := range []struct {
		name  string
		steps []step
		want  string
	}{
		{"unassign", []step{
			{"maya.pc", "item.assign", map[string]any{"item": "maya.pc#1", "targets": []any{"lena"}}},
			{"maya.pc", "item.unassign", map[string]any{"item": "maya.pc#1", "assignee": "lena"}},
		}, "maya.pc#3@maya.pc#1>maya todo"},
		{"decline", []step{
			{"maya.pc", "item.assign", map[string]any{"item": "maya.pc#1", "targets": []any{"raj"}}},
			{"raj.pc", "item.decline", map[string]any{"item": "maya.pc#1"}},
		}, "raj.pc#1@maya.pc#1>maya todo"},
		{"return", []step{
			{"maya.pc", "item.assign", map[string]any{"item": "maya.pc#1", "targets": []any{"raj"}}},
			{"raj.pc", "item.accept", map[string]any{"item": "maya.pc#1"}},
			{"ada.pc", "item.return", map[string]any{"item": "maya.pc#1", "assignee": "raj"}},
		}, "ada.pc#1@maya.pc#1>maya todo"},
		{"member removal", []step{
			{"maya.pc", "item.assign", map[string]any{"item": "maya.pc#1", "targets": []any{"lena"}}},
			{"ada.pc", wsledger.TypeRemoveMember, map[string]any{"member": "lena"}},
		}, "ada.pc#1@maya.pc#1>maya todo"},
		{"rule end", []step{
			{"maya.pc", "item.assign", map[string]any{"item": "maya.pc#1", "targets": []any{map[string]any{"root": "maya", "levels": 1}}}},
			{"maya.pc", "item.unassign", map[string]any{"item": "maya.pc#1", "rule": "maya.pc#2~maya"}},
		}, "maya.pc#3@maya.pc#1>maya todo"},
	} {
		v := topicView(t, "maya.pc", append([]step{create}, c.steps...))
		wantEq(t, c.name, fallbackOn(t, v, "maya.pc#1"), c.want)
		wantEq(t, c.name+" status", deref(v.Items["maya.pc#1"].Status), "todo")
	}
}

func TestFallbackGoesToTheFirstAdminWhenTheCreatorIsRemoved(t *testing.T) {
	base := []step{
		{"lena.pc", "item.create", map[string]any{"kind": "task", "title": "Orphan"}},
		{"ada.pc", wsledger.TypeRemoveAdmin, map[string]any{"member": "ada"}}, // admins: bo
		{"ada.pc", wsledger.TypeRemoveMember, map[string]any{"member": "lena"}},
	}
	v := topicView(t, "bo.pc", base)
	wantEq(t, "bo, the first admin", fallbackOn(t, v, "lena.pc#1"), "ada.pc#2@lena.pc#1>bo todo")
	wantIn(t, "in bo's mine", v.My.Assignments, "ada.pc#2@lena.pc#1>bo")

	// bo stays the holder when no longer an admin; removing bo re-derives it.
	v = topicView(t, "ada.pc", append(append([]step(nil), base...),
		step{"bo.pc", wsledger.TypeAddAdmin, map[string]any{"member": "ada"}}, // admins: bo, ada
		step{"ada.pc", wsledger.TypeRemoveAdmin, map[string]any{"member": "bo"}},
		step{"ada.pc", wsledger.TypeRemoveMember, map[string]any{"member": "bo"}}))
	wantEq(t, "ada after bo's removal", fallbackOn(t, v, "lena.pc#1"), "ada.pc#4@lena.pc#1>ada todo")
	if _, ok := v.Assignments["ada.pc#2@lena.pc#1>bo"]; ok {
		t.Fatalf("an ended fallback period is not shown")
	}
}

func TestFallbackPeriodsKeepTheirOwnStatus(t *testing.T) {
	v := topicView(t, "maya.pc", []step{
		{"maya.pc", "item.create", map[string]any{"kind": "task", "title": "Index"}},
		{"maya.pc", "item.status", map[string]any{"item": "maya.pc#1", "status": "idea"}},
	})
	wantEq(t, "first period", fallbackOn(t, v, "maya.pc#1"), "maya.pc#1@maya.pc#1>maya idea")
	wantEq(t, "item at idea", deref(v.Items["maya.pc#1"].Status), "idea")

	v = topicView(t, "maya.pc", []step{
		{"maya.pc", "item.create", map[string]any{"kind": "task", "title": "Index"}},
		{"maya.pc", "item.status", map[string]any{"item": "maya.pc#1", "status": "doing"}},
		{"maya.pc", "item.assign", map[string]any{"item": "maya.pc#1", "targets": []any{"lena"}}},
		{"maya.pc", "item.unassign", map[string]any{"item": "maya.pc#1", "assignee": "lena"}},
	})
	wantEq(t, "second period starts at todo", fallbackOn(t, v, "maya.pc#1"), "maya.pc#4@maya.pc#1>maya todo")
}

func TestExplicitCreatorAssignDisplacesTheFallback(t *testing.T) {
	v := topicView(t, "maya.pc", []step{
		{"maya.pc", "item.create", map[string]any{"kind": "task", "title": "Index"}},
		{"maya.pc", "item.assign", map[string]any{"item": "maya.pc#1", "targets": []any{"maya"}, "completion": "assignee"}},
		{"maya.pc", "item.status", map[string]any{"item": "maya.pc#1", "status": "doing"}},
	})
	wantEq(t, "only the explicit record", strings.Join(v.Items["maya.pc#1"].Assignments, " "), "maya.pc#2>maya")
	a := v.Assignments["maya.pc#2>maya"]
	wantEq(t, "explicit, live, doing", a.State+" "+a.Status+" "+map[bool]string{true: "fallback", false: "explicit"}[a.Fallback], "live doing explicit")
}

func TestIdeaIsLeftOutOfRollups(t *testing.T) {
	v := topicView(t, "maya.pc", []step{
		{"maya.pc", "item.create", map[string]any{"kind": "task", "title": "Parent"}},
		{"maya.pc", "item.create", map[string]any{"kind": "task", "title": "Idea", "parent": "maya.pc#1"}},
		{"maya.pc", "item.status", map[string]any{"item": "maya.pc#2", "status": "idea"}},
		{"maya.pc", "item.create", map[string]any{"kind": "task", "title": "Todo", "parent": "maya.pc#1"}},
		// two holders: one at idea, one at todo
		{"maya.pc", "item.create", map[string]any{"kind": "task", "title": "Mixed", "parent": "maya.pc#1"}},
		{"maya.pc", "item.assign", map[string]any{"item": "maya.pc#5", "targets": []any{"lena", "omar"}}},
		{"lena.pc", "item.status", map[string]any{"item": "maya.pc#5", "status": "idea"}},
	})
	wantEq(t, "idea child", deref(v.Items["maya.pc#2"].Status)+" "+boolStr(v.Items["maya.pc#2"].Counted), "idea false")
	wantEq(t, "mixed idea and todo", deref(v.Items["maya.pc#5"].Status)+" "+boolStr(v.Items["maya.pc#5"].Counted), "todo true")
	ru := v.Items["maya.pc#1"].Rollup
	wantEq(t, "roll-up", jsNum(float64(ru.Total))+" todo "+jsNum(float64(ru.Counts["todo"])), "2 todo 2")

	v = topicView(t, "maya.pc", []step{
		{"maya.pc", "item.create", map[string]any{"kind": "task", "title": "Mixed"}},
		{"maya.pc", "item.assign", map[string]any{"item": "maya.pc#1", "targets": []any{"lena", "omar"}}},
		{"lena.pc", "item.status", map[string]any{"item": "maya.pc#1", "status": "idea"}},
		{"omar.pc", "item.status", map[string]any{"item": "maya.pc#1", "status": "idea"}},
	})
	wantEq(t, "every holder at idea", deref(v.Items["maya.pc#1"].Status), "idea")
}

func TestOlderReviewStageEventsFoldAsIdea(t *testing.T) {
	v := topicView(t, "maya.pc", []step{
		{"maya.pc", "item.create", map[string]any{"kind": "task", "title": "Old", "stage": "review"}},
		{"maya.pc", "item.create", map[string]any{"kind": "task", "title": "Promoted", "stage": "review"}},
		{"maya.pc", "item.edit", map[string]any{"item": "maya.pc#2", "stage": nil}},
		{"maya.pc", "item.create", map[string]any{"kind": "task", "title": "Demoted"}},
		{"maya.pc", "item.edit", map[string]any{"item": "maya.pc#4", "stage": "review"}},
	})
	wantEq(t, "created in review", fallbackOn(t, v, "maya.pc#1"), "maya.pc#1@maya.pc#1>maya idea")
	wantEq(t, "promoted", fallbackOn(t, v, "maya.pc#2"), "maya.pc#2@maya.pc#2>maya todo")
	wantEq(t, "demoted", fallbackOn(t, v, "maya.pc#4"), "maya.pc#4@maya.pc#4>maya idea")
	wantEq(t, "item status", deref(v.Items["maya.pc#1"].Status), "idea")
}

func TestNewWritesRefuseStage(t *testing.T) {
	tm := newTeam(t)
	wantEq(t, "create", tm.refused("maya.pc", TypeItemCreate, map[string]any{"kind": "task", "title": "x", "stage": "review"}), retiredStage)
	it := tm.do("maya.pc", TypeItemCreate, map[string]any{"kind": "task", "title": "x"})
	wantEq(t, "edit", tm.refused("maya.pc", TypeItemEdit, map[string]any{"item": it, "stage": nil}), retiredStage)
	tm.do("maya.pc", TypeItemStatus, map[string]any{"item": it, "status": "idea"})
	wantEq(t, "idea through status", deref(tm.view("maya.pc").Items[it].Status), "idea")
}

// A status write that had not seen the unassign starting a fallback period
// was meant for the record that unassign ended: it folds there (marked as
// removed), and the new fallback keeps its own todo.
func TestStatusWriteThatMissedTheFallbackStartFoldsOntoTheEndedRecord(t *testing.T) {
	tm := newTeam(t)
	it := tm.do("maya.pc", TypeItemCreate, map[string]any{"kind": "task", "title": "Guard"})
	self := tm.do("maya.pc", TypeItemAssign, map[string]any{"item": it, "targets": []any{"maya"}, "completion": "assignee"})
	tm.goOffline("maya.lap")
	// The unassign folds first (earlier ts) and starts maya's fallback period.
	un := tm.do("maya.pc", TypeItemUnassign, map[string]any{"item": it, "assignee": "maya"})
	st := tm.do("maya.lap", TypeItemStatus, map[string]any{"item": it, "status": "doing"})
	tm.comeOnline("maya.lap")
	for _, s := range []string{"maya.pc", "maya.lap", "ada.pc"} {
		v := tm.view(s)
		wantEq(t, s+": explicit record", asg(t, v, self+">maya").State, "unassigned")
		wantEq(t, s+": the new fallback", fallbackOn(t, v, it), un+"@"+it+">maya todo")
		wantEq(t, s+": item status", deref(v.Items[it].Status), "todo")
		e := event(t, v, st)
		wantEq(t, s+": status write", e.Status, "applied")
		wantIn(t, s+": status write markers", markerKinds(e), "removed:"+un)
	}
}

// The check-time fold (stateBefore) settles fallbacks after another
// thread's assign extends a live rule, as the full fold does. ada's rule
// skips lena, who holds the task; lena hands it to omar, whose unassign
// excludes only omar and leaves the task to ada's fallback; sven's assign
// on another task then extends the rule to lena, displacing that fallback,
// so ada's status write holds nothing.
func TestCheckTimeFoldSettlesAfterAnotherThreadsAssign(t *testing.T) {
	rc := loadReasonCases(t)
	evs := history([]step{
		{"ada.pc", "item.create", map[string]any{"kind": "task", "title": "X"}},
		{"ada.pc", "item.assign", map[string]any{"item": "ada.pc#1", "targets": []any{"lena"}}},
		{"ada.pc", "item.assign", map[string]any{"item": "ada.pc#1", "targets": []any{map[string]any{"root": "lena", "include_root": true, "levels": 1}}}},
		{"lena.pc", "item.handoff", map[string]any{"item": "ada.pc#1", "to": "omar"}},
		{"omar.pc", "item.accept", map[string]any{"item": "ada.pc#1"}},
		{"ada.pc", "item.unassign", map[string]any{"item": "ada.pc#1", "assignee": "omar"}},
		{"sven.pc", "item.create", map[string]any{"kind": "task", "title": "Y"}},
		{"sven.pc", "item.assign", map[string]any{"item": "sven.pc#1", "targets": []any{"sven"}}},
		{"ada.pc", "item.status", map[string]any{"item": "ada.pc#1", "status": "doing"}},
	})
	v := Derive(newMemSource(rc.Genesis, "ada.pc", evs), "ada.pc", time.Time{}, 0)
	var open []string
	for _, a := range openOn(v, "ada.pc#1") {
		open = append(open, a.ID+" "+a.State)
	}
	wantEq(t, "open on X, no fallback", strings.Join(open, ", "), "ada.pc#3~lena>lena live")
	e := event(t, v, "ada.pc#5")
	wantEq(t, "ada's status write", e.Status+": "+deref(e.Reason),
		"flagged: ada created ada.pc-1 but is not assigned to it (self-assign first)")

	// Every verdict equals judging against the whole causal past.
	src := newMemSource(rc.Genesis, "ada.pc", evs)
	r := replay(evs, rc.Genesis, src.Concurrent)
	for _, n := range r.order {
		S := newState(r.g, r.hb, false)
		for _, x := range r.order[:n.pos] {
			if x.status == "applied" && r.hb(x, n) {
				S.apply(x)
			}
		}
		want := "applied"
		if rej := S.check(n); rej != "" {
			want = "flagged: " + string(rej)
		}
		got := n.status
		if got == "flagged" {
			got += ": " + n.reason
		}
		wantEq(t, n.id+" "+n.typ, got, want)
	}
}


func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
