package wstracker

import "testing"

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
	if st := deref(v.Items["maya.pc#1"].Status); st == "pending" {
		t.Fatalf("status of a rule-request-only item = pending; a rule request holds nothing")
	}
}
