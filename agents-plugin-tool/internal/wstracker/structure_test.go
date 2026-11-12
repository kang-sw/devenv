package wstracker

import (
	"strings"
	"testing"
)

// Sibling order (528), item refs (531), and assignment rules (490)-(492),
// walked over the team-alpha fixture with assertions on the view state.

// roots is the top-level part of the view's item order.
func roots(v *View) []string {
	var out []string
	for _, id := range v.ItemOrder {
		if v.Items[id].Parent == nil {
			out = append(out, id)
		}
	}
	return out
}

// follows reports that b directly follows a in l.
func follows(l []string, a, b string) bool {
	for i := 0; i+1 < len(l); i++ {
		if l[i] == a {
			return l[i+1] == b
		}
	}
	return false
}

func TestSiblingOrder(t *testing.T) {
	tm := newTeam(t)
	task := func(title string, kv ...any) map[string]any {
		p := map[string]any{"kind": "task", "title": title}
		for i := 0; i < len(kv); i += 2 {
			p[kv[i].(string)] = kv[i+1]
		}
		return p
	}
	s1 := tm.do("maya.pc", TypeItemCreate, task("S1"))
	s0 := tm.do("maya.pc", TypeItemCreate, task("S0", "after", ""))
	s1b := tm.do("maya.pc", TypeItemCreate, task("S1b", "after", s1))
	k2 := tm.do("maya.pc", TypeItemCreate, task("K2", "parent", s1))
	k1 := tm.do("maya.pc", TypeItemCreate, task("K1", "parent", s1, "after", nil))
	cross := tm.do("maya.pc", TypeItemCreate, task("Cross", "parent", s1, "after", s0))

	v := tm.view("maya.pc")
	r := roots(v)
	wantEq(t, "first root (after \"\")", r[0], s0)
	wantEq(t, "S1b right after S1", follows(r, s1, s1b), true)
	wantEq(t, "S1's children", strings.Join(v.Items[s1].Children, " "), k1+" "+k2+" "+cross)
	wantEq(t, "item order walks S1's children under it", follows(v.ItemOrder, s1, k1), true)

	// Two inserts after S1 that do not see each other: the later lands
	// nearer the anchor.
	tm.goOffline("lena.pc")
	l := tm.do("lena.pc", TypeItemCreate, task("L", "after", s1))
	m := tm.do("maya.pc", TypeItemCreate, task("M", "after", s1))
	tm.comeOnline("lena.pc")
	r = roots(tm.view("maya.pc"))
	wantEq(t, "M after S1", follows(r, s1, m), true)
	wantEq(t, "L after M", follows(r, m, l), true)
	wantEq(t, "S1b after L", follows(r, l, s1b), true)

	// A hidden anchor keeps its place: hiding S1b moves no row.
	s2 := tm.do("maya.pc", TypeItemCreate, task("S2", "after", s1b))
	tm.do("maya.pc", TypeItemHide, map[string]any{"item": s1b})
	r = roots(tm.view("maya.pc"))
	wantEq(t, "S2 still after the hidden S1b", follows(r, s1b, s2), true)

	// A missing anchor falls back to the end.
	z := tm.do("maya.pc", TypeItemCreate, task("Z", "after", "zed.pc#9"))
	r = roots(tm.view("maya.pc"))
	wantEq(t, "Z last", r[len(r)-1], z)

	wantEq(t, "after a number", tm.refused("maya.pc", TypeItemCreate, task("x", "after", 3)), "after must be a sibling item id")
}

func TestItemRefsAndBackrefs(t *testing.T) {
	tm := newTeam(t)
	target := tm.do("maya.pc", TypeItemCreate, map[string]any{"kind": "task", "title": "Target"})
	holder := tm.do("maya.pc", TypeItemCreate, map[string]any{"kind": "topic", "title": "Holder", "body": "see #" + alias(target), "refs": []string{target}})
	cm := tm.do("lena.pc", TypeCommentAdd, map[string]any{"item": holder, "body": "cf #" + alias(target), "refs": []string{target}})
	tm.do("lena.pc", TypeCommentAdd, map[string]any{"item": target, "body": "this one #" + alias(target), "refs": []string{target}})

	v := tm.view("maya.pc")
	wantEq(t, "holder refs", strings.Join(v.Items[holder].Refs, " "), target)
	wantEq(t, "target backrefs: items, then comments; its own thread excluded", strings.Join(v.Items[target].Backrefs, " "), holder+" "+cm)
	wantEq(t, "comment refs", strings.Join(v.Comments[cm].Refs, " "), target)

	// A body edit replaces the refs; one without refs drops them.
	tm.do("maya.pc", TypeItemEdit, map[string]any{"item": holder, "body": "nothing here now"})
	tm.do("lena.pc", TypeCommentHide, map[string]any{"comment": cm})
	v = tm.view("maya.pc")
	wantEq(t, "holder refs after the edit", len(v.Items[holder].Refs), 0)
	wantEq(t, "target backrefs after the edit and the hide", len(v.Items[target].Backrefs), 0)

	wantEq(t, "unseen ref", tm.refused("maya.pc", TypeItemCreate, map[string]any{"kind": "task", "title": "x", "refs": []string{"zed.pc#9"}}),
		"referenced zed.pc-9 is not an item its author had seen")
	wantEq(t, "refs without a body", tm.refused("maya.pc", TypeItemEdit, map[string]any{"item": holder, "title": "t", "refs": []string{target}}),
		"refs go with a body edit")
}

func TestAssignRulesWithinReach(t *testing.T) {
	tm := newTeam(t)
	wide := tm.do("maya.pc", TypeItemCreate, map[string]any{"kind": "task", "title": "Wide"})
	ev := tm.do("maya.pc", TypeItemAssign, map[string]any{"item": wide, "targets": []any{"sven", map[string]any{"root": "maya", "levels": 1}}, "completion": "assigner"})
	rid := ev + "~maya"

	v := tm.view("maya.pc")
	ru, ok := v.Rules[rid]
	if !ok {
		t.Fatalf("no rule %s in %v", rid, v.Rules)
	}
	wantEq(t, "rule state", ru.State, "live")
	wantEq(t, "rule request", ru.Request == nil, true)
	lena := asg(t, v, rid+">lena")
	wantEq(t, "lena via the rule", lena.State+" "+deref(lena.Rule), "live "+rid)
	wantEq(t, "lena forced", lena.Forced, true)
	wantEq(t, "lena rule levels", *lena.RuleLevels, 1.0)
	sven := asg(t, v, ev+">sven")
	wantEq(t, "sven, outside maya's reach, in the same event", sven.State, "pending")
	wantEq(t, "sven's record is no rule's", sven.Rule == nil, true)

	// uma joins maya's team: she inherits the live rule.
	tm.do("ada.pc", TypeSetLead, map[string]any{"member": "uma", "leads": []string{"maya"}})
	uma := asg(t, tm.view("maya.pc"), rid+">uma")
	wantEq(t, "uma inherits", uma.State+" inherited="+map[bool]string{true: "yes", false: "no"}[uma.Inherited], "live inherited=yes")

	// The older levels form folds as before: the rule id is the event's.
	legacy := tm.do("maya.pc", TypeItemCreate, map[string]any{"kind": "task", "title": "Legacy"})
	old := tm.do("maya.pc", TypeItemAssign, map[string]any{"item": legacy, "mode": "all", "levels": 1, "completion": "assigner"})
	a := asg(t, tm.view("maya.pc"), old+">lena")
	wantEq(t, "old form record", deref(a.Rule)+" "+jsNum(*a.RuleLevels), old+" 1")

	wantEq(t, "a rule with nobody in it",
		tm.refused("maya.pc", TypeItemAssign, map[string]any{"item": wide, "targets": []any{map[string]any{"root": "lena", "levels": 1}}}),
		"nobody is within 1 levels below lena")
	wantEq(t, "covered by the old-form rule",
		tm.refused("maya.pc", TypeItemAssign, map[string]any{"item": legacy, "targets": []any{map[string]any{"root": "maya", "levels": 1}}}),
		"the rule rooted at maya is already covered by "+alias(old))
}

func TestAssignRuleOutsideReach(t *testing.T) {
	tm := newTeam(t)
	cross := tm.do("maya.pc", TypeItemCreate, map[string]any{"kind": "task", "title": "Cross"})
	ev := tm.do("maya.pc", TypeItemAssign, map[string]any{"item": cross, "targets": []any{map[string]any{"root": "raj", "levels": 1}}})
	rid := ev + "~raj"

	v := tm.view("raj.pc")
	wantEq(t, "rule pending", v.Rules[rid].State, "pending")
	req := asg(t, v, rid)
	wantEq(t, "request to raj", req.To+" "+req.State, "raj pending")
	wantEq(t, "request flag", req.RuleRequest, true)
	wantIn(t, "raj's received pending", v.My.Received.Pending, rid)
	if _, ok := v.Assignments[rid+">sven"]; ok {
		t.Fatalf("sven holds a record before raj accepts")
	}
	// (535 b) a rule request holds nothing, so it does not make the item
	// pending; the creator holds it meanwhile.
	wantEq(t, "item status", *v.Items[cross].Status, "todo")
	wantEq(t, "a request holds nothing to unassign",
		tm.refused("maya.pc", TypeItemUnassign, map[string]any{"item": cross, "assignee": "raj"}),
		"raj has no live or pending assignment on "+alias(cross))

	tm.do("raj.pc", TypeItemAccept, map[string]any{"item": cross, "note": "ok"})
	v = tm.view("ada.pc")
	wantEq(t, "rule live", v.Rules[rid].State, "live")
	wantEq(t, "request accepted", asg(t, v, rid).State, "accepted")
	sven := asg(t, v, rid+">sven")
	wantEq(t, "sven live and forced", sven.State+" "+map[bool]string{true: "forced", false: "not forced"}[sven.Forced], "live forced")
	var acc []string
	for _, n := range v.Notices {
		acc = append(acc, n.Key)
	}
	wantIn(t, "ada (raj's lead) is notified", acc, "acc:"+rid)

	// Only raj's direct lead returns it; the return ends the rule and its records.
	wantEq(t, "maya returns", tm.refused("maya.pc", TypeItemReturn, map[string]any{"item": cross, "assignee": "raj"}),
		"only raj's direct lead (ada) returns an accepted assignment")
	tm.do("ada.pc", TypeItemReturn, map[string]any{"item": cross, "assignee": "raj", "note": "busy"})
	v = tm.view("maya.pc")
	wantEq(t, "rule ended", v.Rules[rid].State, "ended")
	wantEq(t, "request returned", asg(t, v, rid).State, "returned")
	wantEq(t, "sven unassigned", asg(t, v, rid+">sven").State, "unassigned")
	wantEq(t, "item status", *v.Items[cross].Status, "todo")
	var ret []string
	for _, n := range v.Notices {
		ret = append(ret, n.Key)
	}
	wantIn(t, "maya is notified", ret, "ret:"+rid)

	// A pending rule is withdrawn with unassign root.
	ev2 := tm.do("maya.pc", TypeItemAssign, map[string]any{"item": cross, "targets": []any{map[string]any{"root": "raj", "include_root": true}}})
	tm.do("maya.pc", TypeItemUnassign, map[string]any{"item": cross, "root": "raj"})
	v = tm.view("maya.pc")
	wantEq(t, "withdrawn rule", v.Rules[ev2+"~raj"].State, "ended")
	wantEq(t, "withdrawn request", asg(t, v, ev2+"~raj").State, "withdrawn")
	wantEq(t, "nothing left to accept", tm.refused("raj.pc", TypeItemAccept, map[string]any{"item": cross}),
		"raj has no pending assignment on "+alias(cross))
}
