package wstracker

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// The nine scenarios of the mockup's SCENARIOS.md, walked as dispatch
// sequences over the team-alpha fixture with assertions on the view state.

func TestScenario1MorningCheck(t *testing.T) {
	tm := newTeam(t)
	v := tm.view("maya.pc")
	demo, ship, training := itemID(t, v, "Prepare Friday demo"), itemID(t, v, "Ship search v2 (index + query path)"), itemID(t, v, "Security training")
	review, tok := itemID(t, v, "Review the emoji tokenizer fix"), itemID(t, v, "Tokenizer")

	// Today: my open work by priority then due, the pending assignment to
	// answer, and the close to confirm.
	var work []string
	for _, id := range v.My.Today.Work {
		work = append(work, asg(t, v, id).Item)
	}
	wantEq(t, "today work items", strings.Join(work, " "), demo+" "+ship+" "+training)
	pend := v.My.Today.Pending
	if len(pend) != 1 || asg(t, v, pend[0]).Item != review || asg(t, v, pend[0]).Completion != "assigner" {
		t.Fatalf("today pending = %v, want ivy's review (assigner confirms)", pend)
	}
	reviewAsg := pend[0]
	conf := v.My.Today.Confirms
	if len(conf) != 1 || asg(t, v, conf[0]).Item != tok || asg(t, v, conf[0]).To != "ivy" {
		t.Fatalf("today confirms = %v, want ivy on Tokenizer", conf)
	}

	tm.do("maya.pc", TypeItemAccept, map[string]any{"item": review})
	tm.do("maya.pc", TypeItemConfirm, map[string]any{"item": tok, "assignee": "ivy"})

	v = tm.view("maya.pc")
	a := asg(t, v, reviewAsg)
	wantEq(t, "review state", a.State, "live")
	wantEq(t, "review assigner", a.By, "ivy")
	wantIn(t, "today work", v.My.Today.Work, reviewAsg)
	wantEq(t, "ivy's Tokenizer status", asg(t, v, conf[0]).Status, "done")
	wantEq(t, "Tokenizer status", *v.Items[tok].Status, "done")
	wantEq(t, "today pending", len(v.My.Today.Pending), 0)
	wantEq(t, "today confirms", len(v.My.Today.Confirms), 0)
}

func TestScenario2LeadFindsStuckWork(t *testing.T) {
	tm := newTeam(t)
	maya := tm.view("maya.pc")
	lena := teamRow(t, maya, "lena")
	if len(lena.Pending) != 1 || asg(t, maya, lena.Pending[0]).By != "noa" {
		t.Fatalf("lena's unaccepted = %v, want noa's request", lena.Pending)
	}
	wantEq(t, "omar blocked (his late commit wins over maya's reject)", teamRow(t, maya, "omar").Blocked, 1)
	wantEq(t, "ivy stale", teamRow(t, maya, "ivy").Stale, 1)

	raj := tm.view("raj.pc")
	for _, m := range []string{"sven", "tia", "noa"} {
		wantEq(t, m+" stale", teamRow(t, raj, m).Stale, 1)
	}
	wantEq(t, "pia blocked", teamRow(t, raj, "pia").Blocked, 1)
	// V2: a follower's own pending assignment to others shows as waiting.
	wantEq(t, "noa waiting on others", len(teamRow(t, raj, "noa").Waiting), 1)

	ada := tm.view("ada.pc")
	for _, m := range []string{"maya", "raj", "bo", "lena", "omar", "ivy", "sven", "tia", "noa", "pia"} {
		teamRow(t, ada, m)
	}
	wantEq(t, "ada sees lena at depth", teamRow(t, ada, "lena").Depth, 2)
	wantEq(t, "ada sees omar blocked", teamRow(t, ada, "omar").Blocked, 1)

	// M1 (284): the stale threshold is a caller option.
	wantEq(t, "ivy stale at 10 days", tm.clone("maya.pc").View(tm.now, 10).My.Team[2].Stale, 0)
}

func TestScenario3AcceptedReturnedSentAgain(t *testing.T) {
	tm := newTeam(t)
	v := tm.view("lena.pc")
	explain := itemID(t, v, "Explain the tokenizer API to the query team")
	recv := v.My.Received.Pending
	if len(recv) != 1 || asg(t, v, recv[0]).Item != explain {
		t.Fatalf("lena's received = %v, want noa's request", recv)
	}
	first := recv[0]
	tm.do("lena.pc", TypeItemAccept, map[string]any{"item": explain})

	maya := tm.view("maya.pc")
	wantIn(t, "maya's notices", noticeKeys(maya), "acc:"+first)
	wantIn(t, "maya's badge", maya.My.Badge, "acc:"+first)

	tm.do("maya.pc", TypeItemReturn, map[string]any{"item": explain, "assignee": "lena", "note": "lena is on the index builder this week"})
	maya = tm.view("maya.pc")
	wantEq(t, "returned state", asg(t, maya, first).State, "returned")
	wantEq(t, "item status", *maya.Items[explain].Status, "returned")
	wantEq(t, "item counted", maya.Items[explain].Counted, false)

	// M2 (285): the sender is notified of the return.
	noa := tm.view("noa.pc")
	wantIn(t, "noa's badge", noa.My.Badge, "ret:"+first)

	again := tm.do("noa.pc", TypeItemAssign, map[string]any{"item": explain, "mode": "chosen", "people": []string{"lena"}})
	v = tm.view("lena.pc")
	wantEq(t, "new assignment", asg(t, v, again+">lena").State, "pending")
	wantEq(t, "returned one stays", asg(t, v, first).State, "returned")
}

func TestScenario4MemberLeaves(t *testing.T) {
	tm := newTeam(t)
	v := tm.view("ada.pc")
	shard, training := itemID(t, v, "Shard writer docs"), itemID(t, v, "Security training")

	// Unassign stays with the assigner; (286) an admin hands off instead.
	wantEq(t, "admin unassign", tm.refused("ada.pc", TypeItemUnassign, map[string]any{"item": shard, "assignee": "omar"}),
		"only the assigner (maya) unassigns")
	ho := tm.do("ada.pc", TypeItemHandoff, map[string]any{"item": shard, "to": "jon", "assignee": "omar"})
	jon := tm.view("jon.pc")
	wantIn(t, "jon's received", jon.My.Received.Pending, ho+">jon")
	wantEq(t, "handoff from", *asg(t, jon, ho+">jon").HandoffFrom, "omar")
	tm.do("jon.pc", TypeItemAccept, map[string]any{"item": shard})

	// (209) and the protocol rules hold for removal too.
	wantEq(t, "remove a lead", tm.refused("ada.pc", "roles.remove_member", map[string]any{"member": "maya"}),
		"maya still leads lena, omar, ivy, jon, kai; a removed member must lead nobody (move them with roles.set_lead first)")
	wantEq(t, "remove an admin", tm.refused("ada.pc", "roles.remove_member", map[string]any{"member": "bo"}),
		"remove the admin role of bo first")
	wantEq(t, "non-admin removal", tm.refused("maya.pc", "roles.remove_member", map[string]any{"member": "omar"}),
		"roles.remove_member is admin-only (maya is not an admin in its causal past)")

	rm := tm.do("ada.pc", "roles.remove_member", map[string]any{"member": "omar"})
	v = tm.view("ada.pc")
	wantEq(t, "jon's shard status (290)", asg(t, v, ho+">jon").Status, "blocked")
	wantEq(t, "omar's shard", asg(t, v, "maya.pc#8>omar").State, "handed")
	a := asg(t, v, "ada.pc#7>omar")
	wantEq(t, "omar's training (287)", a.State, "unassigned")
	wantEq(t, "ended by the removal", a.Ended.ID, rm)
	for _, x := range v.Assignments {
		if x.To == "omar" && (x.State == "live" || x.State == "pending") {
			t.Fatalf("omar still holds %s (%s)", x.ID, x.State)
		}
	}
	wantEq(t, "omar removed", *v.Members["omar"].Removed, rm)
	wantEq(t, "training status", *v.Items[training].Status, "doing")
	wantEq(t, "remove twice", tm.refused("ada.pc", "roles.remove_member", map[string]any{"member": "omar"}),
		"omar was already removed ("+rm+")")
}

func TestScenario5AgentStatusHumanCorrection(t *testing.T) {
	tm := newTeam(t)
	v := tm.view("lena.pc")
	ib, ship := itemID(t, v, "Index builder"), itemID(t, v, "Ship search v2 (index + query path)")
	agent := tm.doAs("lena.pc", TypeItemStatus, map[string]any{"item": ib, "status": "blocked", "note": "CI is red on the shard merger"}, OriginAgentAuto)
	v = tm.view("lena.pc")
	a := asg(t, v, "maya.pc#2>lena")
	wantEq(t, "agent status", a.Status, "blocked")
	wantEq(t, "status origin", *a.StatusOrigin, OriginAgentAuto)
	wantIn(t, "blocked below on the parent", v.Items[ship].Rollup.Blocked, ib)
	wantEq(t, "agent event origin", event(t, v, agent).Origin, OriginAgentAuto)

	human := tm.do("lena.pc", TypeItemStatus, map[string]any{"item": ib, "status": "doing", "note": "flaky test, rerun is green"})
	v = tm.view("lena.pc")
	a = asg(t, v, "maya.pc#2>lena")
	wantEq(t, "corrected status", a.Status, "doing")
	wantEq(t, "status origin", *a.StatusOrigin, OriginHuman)
	wantEq(t, "agent write stays applied", event(t, v, agent).Status, "applied")
	wantEq(t, "human write applied", event(t, v, human).Status, "applied")
	wantNotIn(t, "blocked below", v.Items[ship].Rollup.Blocked, ib)
	// An agent never answers an assignment on its own.
	_, err := tm.dispatch("lena.pc", TypeItemAccept, map[string]any{"item": "noa.pc#3"}, OriginAgentAuto)
	var ref *Refusal
	if !errors.As(err, &ref) || ref.Reason != "an agent never accepts, declines, returns or hands off an assignment on its own (agent-auto is invalid here)" {
		t.Fatalf("agent-auto accept: %v", err)
	}
}

func TestScenario6OfflineThenSync(t *testing.T) {
	tm := newTeam(t)
	v := tm.view("maya.lap")
	tok := itemID(t, v, "Tokenizer")
	tm.goOffline("maya.lap")
	reject := tm.do("maya.lap", TypeItemReject, map[string]any{"item": tok, "assignee": "ivy", "note": "the emoji case still fails"})
	tm.now = tm.now.Add(24 * time.Hour)
	comment := tm.do("maya.lap", TypeCommentAdd, map[string]any{"item": tok, "body": "see the failing case in the thread"})
	tm.now = tm.now.Add(24 * time.Hour)

	// ivy sees none of lap's writes; her status write withdraws the close.
	ivy := tm.view("ivy.pc")
	for _, e := range ivy.Events {
		if e.ID == reject || e.ID == comment {
			t.Fatalf("ivy holds lap's offline write %s", e.ID)
		}
	}
	blocked := tm.do("ivy.pc", TypeItemStatus, map[string]any{"item": tok, "status": "blocked", "note": "waiting on the unicode tables"})
	wantEq(t, "lap before sync (her reject)", asg(t, tm.view("maya.lap"), "maya.pc#6>ivy").Status, "doing")

	tm.comeOnline("maya.lap")
	for _, s := range []string{"maya.lap", "ivy.pc"} {
		v := tm.view(s)
		wantEq(t, s+": ivy's Tokenizer status", asg(t, v, "maya.pc#6>ivy").Status, "blocked")
		wantIn(t, s+": reject markers", markerKinds(event(t, v, reject)), "field:"+blocked)
		wantIn(t, s+": status markers", markerKinds(event(t, v, blocked)), "field:"+reject)
		wantEq(t, s+": reject applied", event(t, v, reject).Status, "applied")
		wantEq(t, s+": comment ts two days apart", event(t, v, comment).TS > event(t, v, reject).TS, true)
	}
}

func TestScenario7SameBodyFromTwoClones(t *testing.T) {
	tm := newTeam(t)
	v := tm.view("maya.lap")
	shard := itemID(t, v, "Shard writer docs")
	tm.goOffline("maya.lap")
	lap := tm.do("maya.lap", TypeItemEdit, map[string]any{"item": shard, "body": "laptop draft of the shard writer docs"})
	tm.now = tm.now.Add(time.Hour)
	pc := tm.do("maya.pc", TypeItemEdit, map[string]any{"item": shard, "body": "desktop version of the shard writer docs"})
	tm.comeOnline("maya.lap")
	for _, s := range []string{"maya.lap", "maya.pc"} {
		v := tm.view(s)
		wantEq(t, s+": body", v.Items[shard].Body, "desktop version of the shard writer docs")
		le, pe := event(t, v, lap), event(t, v, pc)
		wantEq(t, s+": no marker on lap", len(le.Markers), 0)
		wantEq(t, s+": no marker on pc", len(pe.Markers), 0)
		if len(le.Superseded) != 1 || le.Superseded[0].By != pc || le.Superseded[0].Why != "later" {
			t.Fatalf("%s: lap edit superseded = %+v, want by %s (later) (289)", s, le.Superseded, pc)
		}
		wantIn(t, s+": thread holds both", v.Items[shard].Thread, lap)
		wantIn(t, s+": thread holds both", v.Items[shard].Thread, pc)
	}
}

func TestScenario8HandoffToAPeer(t *testing.T) {
	tm := newTeam(t)
	v := tm.view("lena.pc")
	ib := itemID(t, v, "Index builder")
	ho := tm.do("lena.pc", TypeItemHandoff, map[string]any{"item": ib, "to": "ivy", "note": "ivy knows the merger"})
	rid := ho + ">ivy"

	ivy := tm.view("ivy.pc")
	wantIn(t, "ivy's received", ivy.My.Received.Pending, rid)
	wantEq(t, "assigner stays", asg(t, ivy, rid).By, "maya")
	wantEq(t, "lena still holds it", asg(t, ivy, "maya.pc#2>lena").State, "live")

	tm.do("ivy.pc", TypeItemAccept, map[string]any{"item": ib})
	maya := tm.view("maya.pc")
	a := asg(t, maya, rid)
	wantEq(t, "ivy's state", a.State, "live")
	wantEq(t, "status carries over (290)", a.Status, "doing")
	wantEq(t, "lena's", asg(t, maya, "maya.pc#2>lena").State, "handed")
	wantEq(t, "item status", *maya.Items[ib].Status, "doing")
	var mine []string
	for _, k := range noticeKeys(maya) {
		if strings.HasSuffix(k, rid) {
			mine = append(mine, k)
		}
	}
	if len(mine) != 1 {
		t.Fatalf("maya's notices on the handoff = %v, want one", mine)
	}

	// (291) returning the accepted handoff restores lena's assignment.
	tm.do("maya.pc", TypeItemReturn, map[string]any{"item": ib, "assignee": "ivy", "note": "lena finishes it"})
	lena := tm.view("lena.pc")
	l := asg(t, lena, "maya.pc#2>lena")
	wantEq(t, "lena restored", l.State, "live")
	wantEq(t, "lena's status", l.Status, "doing")
	if l.Restored == nil {
		t.Fatal("lena's assignment carries no restored")
	}
	wantEq(t, "ivy's", asg(t, lena, rid).State, "returned")
	wantEq(t, "item status", *lena.Items[ib].Status, "doing")
}

func TestScenario9TypoInAMembersItem(t *testing.T) {
	tm := newTeam(t)
	v := tm.view("maya.pc")
	cache, review, explain := itemID(t, v, "Cache layer"), itemID(t, v, "Review the emoji tokenizer fix"), itemID(t, v, "Explain the tokenizer API to the query team")
	wantEq(t, "edit raj's item", tm.refused("maya.pc", TypeItemEdit, map[string]any{"item": cache, "body": "fixed"}),
		"only the creator (raj) edits "+alias(cache))
	c := tm.do("maya.pc", TypeCommentAdd, map[string]any{"item": review, "body": "typo: 'tokeniser' in the second line"})
	e := tm.do("ivy.pc", TypeItemEdit, map[string]any{"item": review, "body": "Review the emoji tokenizer fix (tokenizer spelled right)"})
	v = tm.view("maya.pc")
	wantIn(t, "thread", v.Items[review].Thread, c)
	wantEq(t, "edit applied", event(t, v, e).Status, "applied")
	wantEq(t, "body", v.Items[review].Body, "Review the emoji tokenizer fix (tokenizer spelled right)")

	// (292) an admin edits a removed creator's item; a non-admin still cannot.
	tm.do("ada.pc", "roles.remove_member", map[string]any{"member": "noa"})
	tm.do("ada.pc", TypeItemEdit, map[string]any{"item": explain, "title": "Explain the tokenizer API"})
	wantEq(t, "non-admin edit", tm.refused("maya.pc", TypeItemEdit, map[string]any{"item": explain, "body": "x"}),
		"only the creator (noa) edits "+alias(explain))
	wantEq(t, "title", tm.view("maya.pc").Items[explain].Title, "Explain the tokenizer API")
}

func noticeKeys(v *View) []string {
	var out []string
	for _, n := range v.Notices {
		out = append(out, n.Key)
	}
	return out
}
