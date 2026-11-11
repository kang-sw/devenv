package wstracker

import (
	"strings"
	"testing"
	"time"
)

// Taking work: the creator opens a task nobody else holds and a member in
// scope claims it; a member requests someone's live assignment and its
// holder or the creator approves. The refusals and the fold-time flags are
// reasons.json cases and the concurrent orders are the claim-* golden cases;
// these pin the accepted paths. On the reasons.json genesis ada leads maya,
// raj, and bo; maya leads lena and omar; raj leads sven.

// stepsView folds a linear history on the reasons.json genesis for viewer,
// with no check that every event applied.
func stepsView(t *testing.T, viewer string, steps []step) *View {
	t.Helper()
	rc := loadReasonCases(t)
	return Derive(newMemSource(rc.Genesis, viewer, history(steps)), viewer, time.Time{}, 0)
}

// lastVerdict is the last event's status and reason.
func lastVerdict(v *View) string {
	e := v.Events[len(v.Events)-1]
	return e.Status + " " + deref(e.Reason)
}

func task(by, title string) step {
	return step{by, "item.create", map[string]any{"kind": "task", "title": title}}
}

func TestClaimEndsTheFallbackAndTheOpening(t *testing.T) {
	opened := []step{
		task("ada.pc", "Free"),
		{"ada.pc", "item.open", map[string]any{"item": "ada.pc#1", "scope": "team"}},
	}
	v := topicView(t, "lena.pc", opened)
	iv := v.Items["ada.pc#1"]
	if iv.Open == nil || iv.Open.ID != "ada.pc#2" || iv.Open.By != "ada" || iv.Open.Scope != "team" {
		t.Fatalf("open: %+v", iv.Open)
	}
	wantEq(t, "lena may claim", iv.Claimable, true)
	wantEq(t, "the fallback holder may not", topicView(t, "ada.pc", opened).Items["ada.pc#1"].Claimable, false)

	v = topicView(t, "lena.pc", append(opened, step{"lena.pc", "item.claim", map[string]any{"item": "ada.pc#1"}}))
	iv = v.Items["ada.pc#1"]
	if iv.Open != nil || iv.Claimable {
		t.Fatalf("a claimed item is not open: %+v %v", iv.Open, iv.Claimable)
	}
	for _, a := range openOn(v, "ada.pc#1") {
		if a.Fallback {
			t.Fatalf("the claim displaces the fallback: %+v", a)
		}
	}
	c := v.Assignments["lena.pc#1>lena"]
	wantEq(t, "claim record", c.State+" "+c.To+" by "+c.By+" from "+c.Sender+" at "+c.Status, "live lena by ada from lena at todo")
	wantEq(t, "not forced", c.Forced, false)
	wantIn(t, "in lena's mine", v.My.Assignments, c.ID)
	// the history records the opening and the claim
	wantIn(t, "thread open", iv.Thread, "ada.pc#2")
	wantIn(t, "thread claim", iv.Thread, "lena.pc#1")
}

func TestOpeningScope(t *testing.T) {
	team := []step{task("maya.pc", "Free"), {"maya.pc", "item.open", map[string]any{"item": "maya.pc#1", "scope": "team"}}}
	wantEq(t, "raj outside maya's team", topicView(t, "raj.pc", team).Items["maya.pc#1"].Claimable, false)
	wantEq(t, "omar inside it", topicView(t, "omar.pc", team).Items["maya.pc#1"].Claimable, true)

	store := []step{task("maya.pc", "Free"), {"maya.pc", "item.open", map[string]any{"item": "maya.pc#1", "scope": "store"}},
		{"raj.pc", "item.claim", map[string]any{"item": "maya.pc#1"}}}
	v := topicView(t, "raj.pc", store)
	wantEq(t, "a store opening admits raj", v.Assignments["raj.pc#1>raj"].State, "live")
}

func TestOpeningEndsOnWithdrawAndAssignAndNeverRevives(t *testing.T) {
	base := []step{task("ada.pc", "Free"), {"ada.pc", "item.open", map[string]any{"item": "ada.pc#1", "scope": "store"}}}
	v := topicView(t, "maya.pc", append(append([]step(nil), base...), step{"ada.pc", "item.unopen", map[string]any{"item": "ada.pc#1"}}))
	if v.Items["ada.pc#1"].Open != nil {
		t.Fatal("a withdrawn opening is not open")
	}
	wantEq(t, "the creator still holds it", fallbackOn(t, v, "ada.pc#1"), "ada.pc#1@ada.pc#1>ada todo")
	// withdrawn, it may be opened again
	topicView(t, "maya.pc", append(append([]step(nil), base...),
		step{"ada.pc", "item.unopen", map[string]any{"item": "ada.pc#1"}},
		step{"ada.pc", "item.open", map[string]any{"item": "ada.pc#1", "scope": "team"}}))

	// an assign ends it; the fallback that returns after the unassign is not open
	v = topicView(t, "maya.pc", append(append([]step(nil), base...),
		step{"ada.pc", "item.assign", map[string]any{"item": "ada.pc#1", "targets": []any{"raj"}}},
		step{"ada.pc", "item.unassign", map[string]any{"item": "ada.pc#1", "assignee": "raj"}}))
	if v.Items["ada.pc#1"].Open != nil || v.Items["ada.pc#1"].Claimable {
		t.Fatal("an assign ends the opening for good")
	}
	v = stepsView(t, "maya.pc", append(append([]step(nil), base...),
		step{"ada.pc", "item.assign", map[string]any{"item": "ada.pc#1", "targets": []any{"raj"}}},
		step{"ada.pc", "item.unassign", map[string]any{"item": "ada.pc#1", "assignee": "raj"}},
		step{"maya.pc", "item.claim", map[string]any{"item": "ada.pc#1"}}))
	wantEq(t, "claim after the assign", lastVerdict(v), "flagged ada assigned ada.pc-1 (ada.pc-3), which ended its opening")
}

func TestHandoffOfAnOpenedItemIsRejected(t *testing.T) {
	v := stepsView(t, "ada.pc", []step{
		task("ada.pc", "Free"),
		{"ada.pc", "item.open", map[string]any{"item": "ada.pc#1", "scope": "team"}},
		{"ada.pc", "item.handoff", map[string]any{"item": "ada.pc#1", "to": "maya"}},
	})
	if got := lastVerdict(v); !strings.HasPrefix(got, "flagged ") || !strings.Contains(got, "only because nobody else does") {
		t.Fatalf("handoff: %s", got)
	}
}

func TestCreatorRemovedTheFallbackAdminOpens(t *testing.T) {
	v := topicView(t, "lena.pc", []step{
		task("raj.pc", "Orphan"),
		{"ada.pc", "roles.remove_member", map[string]any{"member": "raj"}},
		{"ada.pc", "item.open", map[string]any{"item": "raj.pc#1", "scope": "team"}},
		{"lena.pc", "item.claim", map[string]any{"item": "raj.pc#1"}},
	})
	c := v.Assignments["lena.pc#1>lena"]
	wantEq(t, "claim under the admin's opening", c.State+" by "+c.By, "live by ada")
}

// held: ada assigned maya (completion assigner) and maya is doing it.
var held = []step{
	task("ada.pc", "Index"),
	{"ada.pc", "item.assign", map[string]any{"item": "ada.pc#1", "targets": []any{"maya"}, "completion": "assigner"}},
	{"maya.pc", "item.status", map[string]any{"item": "ada.pc#1", "status": "doing", "note": "half done"}},
	{"omar.pc", "item.request", map[string]any{"item": "ada.pc#1", "assignee": "maya", "note": "I can take it"}},
}

func TestApprovalMovesTheAssignment(t *testing.T) {
	for _, by := range []string{"maya.pc", "ada.pc"} {
		pending := topicView(t, "maya.pc", held)
		q := pending.Requests["omar.pc#1"]
		wantEq(t, "request", q.State+" "+q.By+" for "+q.Assignee+" "+deref(q.Assignment), "pending omar for maya ada.pc#2>maya")
		wantIn(t, "incoming to the holder", pending.My.Requests.Incoming, "omar.pc#1")
		wantIn(t, "the holder's badge", pending.My.Badge, "q:omar.pc#1")
		wantIn(t, "outgoing for omar", topicView(t, "omar.pc", held).My.Requests.Outgoing, "omar.pc#1")
		wantIn(t, "incoming to the creator", topicView(t, "ada.pc", held).My.Requests.Incoming, "omar.pc#1")
		if in := topicView(t, "raj.pc", held).My.Requests.Incoming; len(in) != 0 {
			t.Fatalf("raj answers nothing: %v", in)
		}

		v := topicView(t, "omar.pc", append(append([]step(nil), held...),
			step{by, "item.approve", map[string]any{"item": "ada.pc#1", "request": "omar.pc#1", "note": "yours"}}))
		wantEq(t, by+": maya's", v.Assignments["ada.pc#2>maya"].State, "handed")
		id := map[string]string{"maya.pc": "maya.pc#2>omar", "ada.pc": "ada.pc#3>omar"}[by]
		o := v.Assignments[id]
		wantEq(t, by+": omar's", o.State+" by "+o.By+" "+o.Status+" "+deref(o.Note)+" "+deref(o.HandoffOf), "live by ada doing half done ada.pc#2>maya")
		wantEq(t, by+": completion", o.Completion, "assigner")
		q = v.Requests["omar.pc#1"]
		wantEq(t, by+": answered", q.State+" "+deref(q.AnswerNote), "approved yours")
		if len(v.My.Requests.Outgoing) != 0 {
			t.Fatalf("an approved request is not outgoing: %v", v.My.Requests.Outgoing)
		}
	}
}

func TestApprovalAddsNoSecondRecord(t *testing.T) {
	v := topicView(t, "omar.pc", append(append([]step(nil), held...),
		step{"ada.pc", "item.assign", map[string]any{"item": "ada.pc#1", "targets": []any{"omar"}}},
		step{"maya.pc", "item.approve", map[string]any{"item": "ada.pc#1", "request": "omar.pc#1"}}))
	wantEq(t, "maya's ends", v.Assignments["ada.pc#2>maya"].State, "handed")
	if _, ok := v.Assignments["maya.pc#2>omar"]; ok {
		t.Fatal("omar already held a record")
	}
	wantEq(t, "omar's assign", v.Assignments["ada.pc#3>omar"].State, "live")
}

func TestReturningAnApprovedTakeRestoresTheHolder(t *testing.T) {
	v := topicView(t, "maya.pc", []step{
		task("ada.pc", "Index"),
		{"ada.pc", "item.assign", map[string]any{"item": "ada.pc#1", "targets": []any{"raj"}}},
		{"lena.pc", "item.request", map[string]any{"item": "ada.pc#1", "assignee": "raj"}},
		{"raj.pc", "item.approve", map[string]any{"item": "ada.pc#1", "request": "lena.pc#1"}},
		{"maya.pc", "item.return", map[string]any{"item": "ada.pc#1", "assignee": "lena"}},
	})
	wantEq(t, "lena's", v.Assignments["raj.pc#1>lena"].State, "returned")
	wantEq(t, "raj's", v.Assignments["ada.pc#2>raj"].State, "live")
}

func TestRequestEnds(t *testing.T) {
	end := func(s step) RequestView {
		v := topicView(t, "omar.pc", append(append([]step(nil), held...), s))
		if len(v.My.Requests.Outgoing) != 0 {
			t.Fatalf("%v: still outgoing", s)
		}
		return v.Requests["omar.pc#1"]
	}
	q := end(step{"ada.pc", "item.unassign", map[string]any{"item": "ada.pc#1", "assignee": "maya"}})
	wantEq(t, "lapsed", q.State+" "+q.Ended.ID, "lapsed ada.pc#3")
	q = end(step{"maya.pc", "item.deny", map[string]any{"item": "ada.pc#1", "request": "omar.pc#1", "note": "staying"}})
	wantEq(t, "holder denies", q.State+" "+deref(q.AnswerNote)+" "+q.Ended.By, "denied staying maya")
	q = end(step{"ada.pc", "item.deny", map[string]any{"item": "ada.pc#1", "request": "omar.pc#1"}})
	wantEq(t, "creator denies", q.State+" "+q.Ended.By, "denied ada")
	q = end(step{"omar.pc", "item.unrequest", map[string]any{"item": "ada.pc#1", "request": "omar.pc#1"}})
	wantEq(t, "withdrawn", q.State+" "+q.Ended.ID, "withdrawn omar.pc#2")
	v := topicView(t, "maya.pc", append(append([]step(nil), held...), step{"ada.pc", "roles.remove_member", map[string]any{"member": "omar"}}))
	wantEq(t, "requester removed", v.Requests["omar.pc#1"].State, "withdrawn")
	if len(v.My.Requests.Incoming) != 0 {
		t.Fatalf("a withdrawn request is not incoming: %v", v.My.Requests.Incoming)
	}
}

func TestRequestOnAMultiAssigneeItemNamesOneAssignment(t *testing.T) {
	steps := []step{
		task("ada.pc", "Pair"),
		{"ada.pc", "item.assign", map[string]any{"item": "ada.pc#1", "targets": []any{"lena", "raj"}}},
		{"omar.pc", "item.request", map[string]any{"item": "ada.pc#1", "assignee": "lena"}},
	}
	wantIn(t, "lena answers", topicView(t, "lena.pc", steps).My.Requests.Incoming, "omar.pc#1")
	if in := topicView(t, "raj.pc", steps).My.Requests.Incoming; len(in) != 0 {
		t.Fatalf("raj answers nothing: %v", in)
	}
	v := topicView(t, "raj.pc", append(steps, step{"lena.pc", "item.approve", map[string]any{"item": "ada.pc#1", "request": "omar.pc#1"}}))
	wantEq(t, "lena's", v.Assignments["ada.pc#2>lena"].State, "handed")
	wantEq(t, "raj's stays", v.Assignments["ada.pc#2>raj"].State, "live")
	wantEq(t, "omar's", v.Assignments["lena.pc#1>omar"].State, "live")
}
