package wstracker

import (
	"strings"
	"testing"
)

func orgSetPayload(id, name, head string) map[string]any {
	return map[string]any{"org": id, "name": name, "head": head}
}

func TestOrgLifecycleAndMembership(t *testing.T) {
	tm := newTeam(t)
	wantEq(t, "org events are admin-only",
		strings.Contains(tm.refused("maya.pc", TypeOrgSet, orgSetPayload("o-platform", "Platform", "maya")), "admin-only"), true)
	tm.do("ada.pc", TypeOrgSet, orgSetPayload("o-platform", "Platform", "maya"))
	tm.do("ada.pc", TypeOrgSet, orgSetPayload("o-search", "Search", "lena"))

	s := tm.clone("ada.pc").r.S
	wantEq(t, "parent org head", s.orgs["o-platform"].head, "maya")
	wantEq(t, "nested org head", s.orgs["o-search"].head, "lena")
	wantEq(t, "head nearest org", s.nearestOrg("maya").id, "o-platform")
	wantEq(t, "nested head nearest org", s.nearestOrg("lena").id, "o-search")
	wantEq(t, "nested member keeps nearest child org", s.nearestOrg("lena").id, "o-search")
	wantIn(t, "parent org scope includes nested head", s.orgMembers(s.orgs["o-platform"]), "lena")

	wantEq(t, "one org per head",
		strings.Contains(tm.refused("ada.pc", TypeOrgSet, orgSetPayload("o-other", "Other", "maya")), "already heads"), true)
	wantEq(t, "unique live name",
		strings.Contains(tm.refused("ada.pc", TypeOrgSet, orgSetPayload("o-other", "platform", "raj")), "already exists"), true)

	tm.do("ada.pc", TypeOrgSet, orgSetPayload("o-platform", "Core Platform", ""))
	wantEq(t, "vacant org", tm.clone("ada.pc").r.S.orgs["o-platform"].head, "")
	tm.do("ada.pc", TypeOrgSet, orgSetPayload("o-platform", "Core Platform", "raj"))
	tm.do("ada.pc", TypeOrgRemove, map[string]any{"org": "o-platform"})
	o := tm.clone("ada.pc").r.S.orgs["o-platform"]
	wantEq(t, "removed org releases head", o.head, "")
	wantEq(t, "removed org is a tombstone", o.removed != nil, true)
	wantEq(t, "removed id cannot be restored",
		strings.Contains(tm.refused("ada.pc", TypeOrgSet, orgSetPayload("o-platform", "Restored", "raj")), "was removed"), true)
	// A deleted org's name is reusable under a new id.
	tm.do("ada.pc", TypeOrgSet, orgSetPayload("o-platform-2", "Core Platform", "raj"))
}

func TestRemovingOrgHeadLeavesVacantSeat(t *testing.T) {
	tm := newTeam(t)
	tm.do("ada.pc", TypeOrgSet, orgSetPayload("o-leaf", "Leaf", "lena"))
	tm.do("ada.pc", "roles.remove_member", map[string]any{"member": "lena"})
	o := tm.clone("ada.pc").r.S.orgs["o-leaf"]
	wantEq(t, "member removal vacates org", o.head, "")
	wantEq(t, "member removal does not delete org", o.removed == nil, true)
}

func TestConcurrentOrgListConflictsUseFoldOrder(t *testing.T) {
	t.Run("one head in two orgs", func(t *testing.T) {
		tm := newTeam(t)
		tm.goOffline("ada.pc")
		tm.goOffline("bo.pc")
		a := tm.do("ada.pc", TypeOrgSet, orgSetPayload("o-a", "A", "maya"))
		b := tm.do("bo.pc", TypeOrgSet, orgSetPayload("o-b", "B", "maya"))
		tm.comeOnline("ada.pc")
		tm.comeOnline("bo.pc")
		v := tm.view("ada.pc")
		wantEq(t, "first head claim", event(t, v, a).Status, "applied")
		wantEq(t, "later head claim", event(t, v, b).Status, "flagged")
	})

	t.Run("set after concurrent remove", func(t *testing.T) {
		tm := newTeam(t)
		tm.do("ada.pc", TypeOrgSet, orgSetPayload("o-a", "A", "maya"))
		tm.goOffline("ada.pc")
		tm.goOffline("bo.pc")
		rm := tm.do("ada.pc", TypeOrgRemove, map[string]any{"org": "o-a"})
		set := tm.do("bo.pc", TypeOrgSet, orgSetPayload("o-a", "Renamed", "raj"))
		tm.comeOnline("ada.pc")
		tm.comeOnline("bo.pc")
		v := tm.view("ada.pc")
		wantEq(t, "remove", event(t, v, rm).Status, "applied")
		wantEq(t, "later set", event(t, v, set).Status, "flagged")
	})

	t.Run("same org concurrent edits", func(t *testing.T) {
		tm := newTeam(t)
		tm.do("ada.pc", TypeOrgSet, orgSetPayload("o-a", "A", "maya"))
		tm.goOffline("ada.pc")
		tm.goOffline("bo.pc")
		a := tm.do("ada.pc", TypeOrgSet, orgSetPayload("o-a", "First", "maya"))
		b := tm.do("bo.pc", TypeOrgSet, orgSetPayload("o-a", "Second", "maya"))
		tm.comeOnline("ada.pc")
		tm.comeOnline("bo.pc")
		v := tm.view("ada.pc")
		wantEq(t, "first edit", event(t, v, a).Status, "applied")
		wantEq(t, "second edit", event(t, v, b).Status, "applied")
		wantEq(t, "fold-order winner", tm.clone("ada.pc").r.S.orgs["o-a"].name, "Second")
	})
}

func TestOrgCCProvenanceAndDynamicScope(t *testing.T) {
	tm := newTeam(t)
	tm.do("ada.pc", TypeOrgSet, orgSetPayload("o-platform", "Platform", "maya"))
	tm.do("ada.pc", TypeOrgSet, orgSetPayload("o-nested", "Nested", "lena"))
	it := tm.do("maya.pc", TypeItemCreate, map[string]any{"kind": "task", "title": "Org CC"})
	tm.do("raj.pc", TypeItemCCOrg, map[string]any{"item": it, "org": "o-platform"})

	v := tm.view("maya.pc")
	wantIn(t, "org CC includes head", v.Items[it].CC, "maya")
	wantIn(t, "org CC includes nested org head", v.Items[it].CC, "lena")
	wantIn(t, "org CC includes whole subtree", v.Items[it].CC, "omar")

	// Newcomers are added; leavers retain the source already granted.
	tm.do("ada.pc", TypeSetLead, map[string]any{"member": "uma", "leads": []string{"maya"}})
	wantIn(t, "newcomer is added", tm.view("maya.pc").Items[it].CC, "uma")
	tm.do("ada.pc", TypeSetLead, map[string]any{"member": "uma", "leads": []string{"bo"}})
	wantIn(t, "leaver retains CC", tm.view("maya.pc").Items[it].CC, "uma")

	// Personal provenance survives unregistering the org.
	tm.do("raj.pc", TypeItemCC, map[string]any{"item": it, "member": "lena"})
	tm.do("raj.pc", TypeItemUnccOrg, map[string]any{"item": it, "org": "o-platform"})
	v = tm.view("maya.pc")
	wantIDs(t, "only personal CC remains", v.Items[it].CC, []string{"lena"})
	e, _ := ccEntryAt(tm.clone("maya.pc").r.S.items[it], "lena")
	wantEq(t, "personal source remains", e != nil && e.personal && len(e.orgs) == 0, true)
}

func TestPersonalUnccExcludesOrgReadds(t *testing.T) {
	tm := newTeam(t)
	tm.do("ada.pc", TypeOrgSet, orgSetPayload("o-platform", "Platform", "maya"))
	it := tm.do("maya.pc", TypeItemCreate, map[string]any{"kind": "task", "title": "Exclusions"})
	tm.do("raj.pc", TypeItemCCOrg, map[string]any{"item": it, "org": "o-platform"})
	tm.do("raj.pc", TypeItemUncc, map[string]any{"item": it, "member": "lena"})
	wantNotIn(t, "personal uncc removes org CC", tm.view("maya.pc").Items[it].CC, "lena")

	// A later roles refresh does not re-add the excluded member.
	tm.do("ada.pc", TypeSetLead, map[string]any{"member": "omar", "leads": []string{"maya"}})
	wantNotIn(t, "excluded member stays out", tm.view("maya.pc").Items[it].CC, "lena")
	tm.do("raj.pc", TypeItemCC, map[string]any{"item": it, "member": "lena"})
	wantIn(t, "personal CC restores member", tm.view("maya.pc").Items[it].CC, "lena")

	tm.view("maya.pc")
	s := tm.clone("maya.pc").r.S
	reg := s.ccOrgs[ccOrgKey(it, "o-platform")]
	wantEq(t, "registration keeps exclusion", reg.excluded["lena"], true)
	e, _ := ccEntryAt(s.items[it], "lena")
	wantEq(t, "restored CC is personal only", e.personal && !e.orgs["o-platform"], true)
}

func TestOrgCCVacancyAndTombstone(t *testing.T) {
	tm := newTeam(t)
	tm.do("ada.pc", TypeOrgSet, orgSetPayload("o-platform", "Platform", ""))
	it := tm.do("maya.pc", TypeItemCreate, map[string]any{"kind": "task", "title": "Vacant"})
	wantEq(t, "vacant registration refused",
		strings.Contains(tm.refused("raj.pc", TypeItemCCOrg, map[string]any{"item": it, "org": "o-platform"}), "vacant"), true)
	tm.do("ada.pc", TypeOrgSet, orgSetPayload("o-platform", "Platform", "maya"))
	tm.do("raj.pc", TypeItemCCOrg, map[string]any{"item": it, "org": "o-platform"})
	tm.do("ada.pc", TypeOrgRemove, map[string]any{"org": "o-platform"})

	tm.view("maya.pc")
	s := tm.clone("maya.pc").r.S
	reg := s.ccOrgs[ccOrgKey(it, "o-platform")]
	wantEq(t, "tombstoned registration stays active", reg.active, true)
	wantIn(t, "tombstone keeps prior people", tm.view("maya.pc").Items[it].CC, "lena")
	wantEq(t, "new registration on tombstone refused",
		strings.Contains(tm.refused("raj.pc", TypeItemCCOrg, map[string]any{"item": it, "org": "o-platform"}), "removed"), true)
	// Removing a dead registration remains valid and revokes its sole sources.
	tm.do("raj.pc", TypeItemUnccOrg, map[string]any{"item": it, "org": "o-platform"})
	wantEq(t, "dead registration removed", len(tm.view("maya.pc").Items[it].CC), 0)
}

func TestOrgAssignFollowsHeadAndKeepsAssignRoot(t *testing.T) {
	tm := newTeam(t)
	tm.do("ada.pc", TypeOrgSet, orgSetPayload("o-platform", "Platform", "maya"))
	it := tm.do("maya.pc", TypeItemCreate, map[string]any{"kind": "task", "title": "Org assign"})
	ev := tm.do("maya.pc", TypeItemAssign, map[string]any{"item": it, "completion": "assigner", "targets": []any{
		map[string]any{"root": "maya", "org": "o-platform", "include_root": true},
	}})
	rid := ev + "~maya"
	v := tm.view("maya.pc")
	wantIn(t, "initial org rule includes head", v.Items[it].Assignments, rid+">maya")
	wantIn(t, "initial org rule includes subtree", v.Items[it].Assignments, rid+">lena")

	tm.do("ada.pc", TypeOrgSet, orgSetPayload("o-platform", "Platform", "raj"))
	v = tm.view("maya.pc")
	ru := tm.clone("maya.pc").r.S.ruleByID[rid]
	wantEq(t, "rule id stays stable", ru.id, rid)
	wantEq(t, "rule reroots", ru.root, "raj")
	wantEq(t, "assign-time root stays", ru.assignRoot, "maya")
	wantIn(t, "new head subtree is added", v.Items[it].Assignments, rid+">sven")
	wantIn(t, "old subtree is not revoked", v.Items[it].Assignments, rid+">lena")

	wantEq(t, "current root does not match unassign root",
		strings.Contains(tm.refused("maya.pc", TypeItemUnassign, map[string]any{"item": it, "root": "raj"}), "no rule"), true)
	tm.do("maya.pc", TypeItemUnassign, map[string]any{"item": it, "root": "maya"})
	wantEq(t, "assign-time root ends rerooted rule", tm.view("maya.pc").Rules[rid].State, "ended")
}

func TestOrgAssignVacancyRemovalAndTargetChecks(t *testing.T) {
	tm := newTeam(t)
	tm.do("ada.pc", TypeOrgSet, orgSetPayload("o-platform", "Platform", "maya"))
	it := tm.do("maya.pc", TypeItemCreate, map[string]any{"kind": "task", "title": "Org vacancy"})
	wantEq(t, "wrong current head refused",
		strings.Contains(tm.refused("maya.pc", TypeItemAssign, map[string]any{"item": it, "targets": []any{map[string]any{"root": "raj", "org": "o-platform"}}}), "headed by"), true)
	ev := tm.do("maya.pc", TypeItemAssign, map[string]any{"item": it, "completion": "assigner", "targets": []any{map[string]any{"root": "maya", "org": "o-platform", "include_root": true}}})
	rid := ev + "~maya"
	tm.do("ada.pc", TypeOrgSet, orgSetPayload("o-platform", "Platform", ""))

	// While vacant the rule keeps its last root and adds nobody on role refreshes.
	tm.do("ada.pc", TypeSetLead, map[string]any{"member": "uma", "leads": []string{"bo"}})
	s := tm.clone("ada.pc").r.S
	wantEq(t, "vacant rule keeps last root", s.ruleByID[rid].root, "maya")
	if s.asg[rid+">uma"] != nil {
		t.Fatal("vacant org rule added uma")
	}
	wantEq(t, "new assign on vacant org refused",
		strings.Contains(tm.refused("maya.pc", TypeItemAssign, map[string]any{"item": it, "targets": []any{map[string]any{"root": "maya", "org": "o-platform"}}}), "vacant"), true)

	tm.do("ada.pc", TypeOrgSet, orgSetPayload("o-platform", "Platform", "bo"))
	v := tm.view("maya.pc")
	wantIn(t, "refilled org reroots and extends", v.Items[it].Assignments, rid+">uma")
	tm.do("ada.pc", TypeOrgRemove, map[string]any{"org": "o-platform"})
	wantEq(t, "new assign on removed org refused",
		strings.Contains(tm.refused("maya.pc", TypeItemAssign, map[string]any{"item": it, "targets": []any{map[string]any{"root": "bo", "org": "o-platform"}}}), "removed"), true)
	// The dead registration remains addressable by its stable rule id.
	tm.do("maya.pc", TypeItemUnassign, map[string]any{"item": it, "rule": rid})
	wantEq(t, "dead org rule removed", tm.view("maya.pc").Rules[rid].State, "ended")
}

func TestOrgAssignConsentStaysWithAssignTimeHead(t *testing.T) {
	tm := newTeam(t)
	tm.do("ada.pc", TypeOrgSet, orgSetPayload("o-sales", "Sales", "raj"))
	it := tm.do("maya.pc", TypeItemCreate, map[string]any{"kind": "task", "title": "Consent"})
	ev := tm.do("maya.pc", TypeItemAssign, map[string]any{"item": it, "targets": []any{map[string]any{"root": "raj", "org": "o-sales", "include_root": true}}})
	rid := ev + "~raj"
	v := tm.view("raj.pc")
	wantEq(t, "org rule starts pending", v.Rules[rid].State, "pending")
	wantEq(t, "request stays on assign-time head", asg(t, v, rid).To, "raj")
	if _, ok := v.Assignments[rid+">raj"]; ok {
		t.Fatal("pending org rule assigned its head")
	}

	tm.do("ada.pc", TypeOrgSet, orgSetPayload("o-sales", "Sales", "bo"))
	wantEq(t, "new head cannot accept old request",
		strings.Contains(tm.refused("bo.pc", TypeItemAccept, map[string]any{"item": it}), "no pending"), true)
	tm.do("raj.pc", TypeItemAccept, map[string]any{"item": it})
	v = tm.view("maya.pc")
	wantEq(t, "accepted rule follows current head", v.Rules[rid].Root, "bo")
	wantEq(t, "accepted rule is live", v.Rules[rid].State, "live")
	wantIn(t, "current head receives assignment", v.Items[it].Assignments, rid+">bo")

	// The old-shape return still names the assign-time root and is performed by
	// that member's direct lead, not by the new org head.
	wantEq(t, "new head cannot return consent",
		strings.Contains(tm.refused("bo.pc", TypeItemReturn, map[string]any{"item": it, "assignee": "raj"}), "only raj's direct lead"), true)
	tm.do("ada.pc", TypeItemReturn, map[string]any{"item": it, "assignee": "raj"})
	wantEq(t, "return ends org rule", tm.view("maya.pc").Rules[rid].State, "ended")
}

func TestEndingOrgRuleReextendsOverlappingRule(t *testing.T) {
	tm := newTeam(t)
	tm.do("ada.pc", TypeSetLead, map[string]any{"member": "uma", "leads": []string{"lena"}})
	tm.do("ada.pc", TypeOrgSet, orgSetPayload("o-search", "Search", "lena"))
	it := tm.do("maya.pc", TypeItemCreate, map[string]any{"kind": "task", "title": "Overlap"})
	ev := tm.do("maya.pc", TypeItemAssign, map[string]any{"item": it, "completion": "assigner", "targets": []any{
		map[string]any{"root": "lena", "org": "o-search", "include_root": true},
		map[string]any{"root": "maya", "levels": 1},
	}})
	orgRule, plainRule := ev+"~lena", ev+"~maya"
	s := tm.clone("maya.pc").r.S
	wantEq(t, "org rule owns overlap before removal", s.openRec(it, "lena").rule, orgRule)
	if s.asg[plainRule+">lena"] != nil {
		t.Fatal("overlapping plain rule unexpectedly owned lena before removal")
	}
	tm.do("maya.pc", TypeItemUnassign, map[string]any{"item": it, "rule": orgRule})
	v := tm.view("maya.pc")
	wantEq(t, "other rule fills gap in same step", asg(t, v, plainRule+">lena").State, "live")
}
