package wstracker

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsledger"
)

func viewJSON(t *testing.T, v *View) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// TestVetoLeadsNobody calls the exported veto directly with a crafted past.
func TestVetoLeadsNobody(t *testing.T) {
	genesis := json.RawMessage(`{"admins":["ada"],"members":[
		{"id":"ada","name":"Ada","keys":["ada-k1"],"leads":[]},
		{"id":"maya","name":"Maya","keys":["maya-k1"],"leads":["ada"]},
		{"id":"lena","name":"Lena","keys":["lena-k1"],"leads":["maya"]}]}`)
	veto := Veto(genesis)
	remove := step{"ada.pc", wsledger.TypeRemoveMember, map[string]any{"member": "maya"}}

	evs := history([]step{remove})
	err := veto(evs[0], nil)
	if err == nil || err.Error() != "maya still leads lena; a removed member must lead nobody (move them with roles.set_lead first)" {
		t.Fatalf("removal of a lead: %v", err)
	}

	evs = history([]step{{"ada.pc", TypeSetLead, map[string]any{"member": "lena", "leads": []string{"ada"}}}, remove})
	if err := veto(evs[1], evs[:1]); err != nil {
		t.Fatalf("removal after the follower moved away: %v", err)
	}

	// A set_lead the domain flags (by a non-admin) moves nobody.
	evs = history([]step{{"maya.pc", TypeSetLead, map[string]any{"member": "lena", "leads": []string{"ada"}}}, remove})
	if err := veto(evs[1], evs[:1]); err == nil {
		t.Fatal("a flagged set_lead moved the follower")
	}

	evs = history([]step{{"ada.pc", wsledger.TypeAddAdmin, map[string]any{"member": "maya"}}})
	if err := veto(evs[0], nil); err != nil {
		t.Fatalf("veto on another identity event: %v", err)
	}
}

// TestDispatchRefusal: a would-be-flagged event is refused with its reason,
// a call is all-or-nothing, and a later event of a call sees the earlier.
func TestDispatchRefusal(t *testing.T) {
	tm := newTeam(t)
	trk := tm.clone("maya.pc")
	before := len(tm.src["maya.pc"].events)
	raw := func(v any) json.RawMessage { b, _ := json.Marshal(v); return b }
	_, err := trk.Dispatch(context.Background(), []wsledger.NewEvent{
		{Type: TypeCommentAdd, Payload: raw(map[string]any{"item": "ada.pc#1", "body": "fine"})},
		{Type: TypeItemEdit, Payload: raw(map[string]any{"item": "ada.pc#1", "title": "mine now"})},
	})
	var ref *Refusal
	if !errors.As(err, &ref) {
		t.Fatalf("want a refusal, got %v", err)
	}
	wantEq(t, "index", ref.Index, 1)
	wantEq(t, "reason", ref.Reason, "only the creator (ada) edits ada.pc-1")
	wantEq(t, "appended", len(tm.src["maya.pc"].events), before)

	// The assign names the item the same call creates, by NextID.
	next := trk.NextID()
	item := next.String()
	ids, err := trk.Dispatch(context.Background(), []wsledger.NewEvent{
		{Type: TypeItemCreate, Payload: raw(map[string]any{"kind": "task", "title": "Write the release notes"})},
		{Type: TypeItemAssign, Payload: raw(map[string]any{"item": item, "mode": "chosen", "people": []string{"lena"}})},
	})
	if err != nil {
		t.Fatal(err)
	}
	wantEq(t, "created id", ids[0], next)
	v := trk.View(tm.now, 0)
	wantEq(t, "assign applied", event(t, v, ids[1].String()).Status, "applied")
	wantEq(t, "forced on a follower", asg(t, v, ids[1].String()+">lena").State, "live")

	// The candidate is judged after the call's earlier events.
	_, err = trk.Dispatch(context.Background(), []wsledger.NewEvent{
		{Type: TypeItemHide, Payload: raw(map[string]any{"item": item})},
	})
	if !errors.As(err, &ref) || ref.Reason != "unassign the open assignments first (lena)" {
		t.Fatalf("hide with an open assignment: %v", err)
	}
	if _, err := trk.Dispatch(context.Background(), []wsledger.NewEvent{
		{Type: TypeItemUnassign, Payload: raw(map[string]any{"item": item, "assignee": "lena"})},
		{Type: TypeItemHide, Payload: raw(map[string]any{"item": item})},
	}); err != nil {
		t.Fatalf("unassign then hide in one call: %v", err)
	}

	if _, err := trk.Dispatch(context.Background(), []wsledger.NewEvent{
		{Type: wsledger.TypeMemberJoin, Payload: raw(map[string]any{"member": "zed"})},
	}); !errors.As(err, &ref) {
		t.Fatalf("member.join dispatched: %v", err)
	}
	wantEq(t, "add_key payload", tm.refused("ada.pc", wsledger.TypeAddKey, map[string]any{"member": "maya", "key": "nope"}) != "", true)
	wantEq(t, "add an admin twice", tm.refused("ada.pc", wsledger.TypeAddAdmin, map[string]any{"member": "bo"}), "bo is already an admin")
	wantEq(t, "unknown member", tm.refused("ada.pc", wsledger.TypeAddAdmin, map[string]any{"member": "zed"}), `unknown member "zed"`)
	tm.do("ada.pc", wsledger.TypeRemoveAdmin, map[string]any{"member": "bo"})
	wantEq(t, "last admin", tm.refused("ada.pc", wsledger.TypeRemoveAdmin, map[string]any{"member": "ada"}), "cannot remove the last admin")
}

// TestRemovalEndsLiveAndPending (287): lena holds live assignments and a
// pending one; her removal ends them as unassigned and withdrawn.
func TestRemovalEndsLiveAndPending(t *testing.T) {
	tm := newTeam(t)
	rm := tm.do("ada.pc", wsledger.TypeRemoveMember, map[string]any{"member": "lena"})
	v := tm.view("ada.pc")
	for id, want := range map[string]string{"maya.pc#2>lena": "unassigned", "ada.pc#7>lena": "unassigned", "noa.pc#4>lena": "withdrawn"} {
		a := asg(t, v, id)
		wantEq(t, id, a.State, want)
		wantEq(t, id+" ended by", a.Ended.ID, rm)
	}
	wantEq(t, "the author is gone", tm.refused("lena.pc", TypeCommentAdd, map[string]any{"item": "ada.pc#1", "body": "x"}),
		`author "lena" was removed from the store (`+rm+`)`)
}

// TestAdminHandoffPendingThenRemoval: (286) an admin's handoff of a member's
// assignment is still pending when the member is removed; (295) the
// removal ends the original and withdraws the handoff.
func TestAdminHandoffPendingThenRemoval(t *testing.T) {
	tm := newTeam(t)
	ho := tm.do("ada.pc", TypeItemHandoff, map[string]any{"item": "maya.pc#7", "to": "jon", "assignee": "omar"})
	rm := tm.do("ada.pc", wsledger.TypeRemoveMember, map[string]any{"member": "omar"})
	v := tm.view("jon.pc")
	wantEq(t, "omar's", asg(t, v, "maya.pc#8>omar").State, "unassigned")
	h := asg(t, v, ho+">jon")
	wantEq(t, "the handoff", h.State, "withdrawn")
	wantEq(t, "ended by", h.Ended.ID, rm)
	wantEq(t, "accept", tm.refused("jon.pc", TypeItemAccept, map[string]any{"item": "maya.pc#7"}),
		"jon has no pending assignment on maya.pc-7")
}

// TestHandoffOriginalEndedConcurrent (295): an unassign of the original
// concurrent with an accept of its handoff wins whichever folds first.
func TestHandoffOriginalEndedConcurrent(t *testing.T) {
	for _, acceptFirst := range []bool{true, false} {
		name := "unassign first"
		if acceptFirst {
			name = "accept first"
		}
		t.Run(name, func(t *testing.T) {
			tm := newTeam(t)
			ho := tm.do("lena.pc", TypeItemHandoff, map[string]any{"item": "maya.pc#1", "to": "ivy"})
			tm.goOffline("ivy.pc")
			tm.goOffline("maya.pc")
			first, second := "ivy.pc", "maya.pc"
			if !acceptFirst {
				first, second = second, first
			}
			ids := map[string]string{}
			write := func(s string) {
				if s == "ivy.pc" {
					ids[s] = tm.do(s, TypeItemAccept, map[string]any{"item": "maya.pc#1"})
					ids["status"] = tm.do(s, TypeItemStatus, map[string]any{"item": "maya.pc#1", "status": "blocked"})
				} else {
					ids[s] = tm.do(s, TypeItemUnassign, map[string]any{"item": "maya.pc#1", "assignee": "lena"})
				}
			}
			write(first)
			write(second)
			tm.comeOnline("ivy.pc")
			tm.comeOnline("maya.pc")
			for _, s := range []string{"ivy.pc", "maya.pc", "ada.pc"} {
				v := tm.view(s)
				wantEq(t, s+": lena's", asg(t, v, "maya.pc#2>lena").State, "unassigned")
				h := asg(t, v, ho+">ivy")
				wantEq(t, s+": the handoff", h.State, "withdrawn")
				wantEq(t, s+": ended by", h.Ended.ID, ids["maya.pc"])
				wantEq(t, s+": accept applied", event(t, v, ids["ivy.pc"]).Status, "applied")
				if len(event(t, v, ids["ivy.pc"]).Markers) == 0 || len(event(t, v, ids["maya.pc"]).Markers) == 0 {
					t.Fatalf("%s: both events must be marked", s)
				}
				if *v.Items["maya.pc#1"].Status == "blocked" {
					t.Fatalf("%s: ivy's status write is in effect after the handoff was withdrawn", s)
				}
			}
		})
	}
}

// TestIncrementalRefresh: the tracker's cached, cursor-driven state equals a
// fresh derive after new events, a verdict change behind the cursor (a
// revocation cut), and a filled gap.
func TestIncrementalRefresh(t *testing.T) {
	doc, src := loadGolden(t, "testdata/golden/team-alpha__maya.pc.json")
	now, _ := time.Parse(time.RFC3339, doc.Now)
	trk := New(src, "maya.pc")
	same := func(what string) {
		t.Helper()
		if got, want := viewJSON(t, trk.View(now, 0)), viewJSON(t, Derive(src, "maya.pc", now, 0)); got != want {
			t.Fatalf("%s: the tracker's view differs from a fresh derive", what)
		}
	}
	same("initial")
	if trk.Refresh() {
		t.Fatal("refresh with nothing new re-folded")
	}

	// A comment by lena, with a gap: her stream's next event is missing.
	base := src.heads()
	lenaNext := base["lena.pc"] + 1
	gapped := wsledger.Event{ID: wsledger.EventID{Member: "lena", Clone: "pc", Seq: lenaNext + 1}, TS: now,
		Type: TypeCommentAdd, Payload: json.RawMessage(`{"item":"ada.pc#1","body":"second"}`), Seen: base,
		Verdict: wsledger.Waiting, Reason: "waiting: gap"}
	src.add(gapped)
	same("waiting event")
	wantEq(t, "gapped", event(t, trk.View(now, 0), gapped.ID.String()).Status, "waiting")

	filler := wsledger.Event{ID: wsledger.EventID{Member: "lena", Clone: "pc", Seq: lenaNext}, TS: now,
		Type: TypeCommentAdd, Payload: json.RawMessage(`{"item":"ada.pc#1","body":"first"}`), Seen: base, Verdict: wsledger.Valid}
	src.add(filler)
	src.setVerdict(gapped.ID, wsledger.Valid, "")
	same("filled gap")
	wantEq(t, "filled", event(t, trk.View(now, 0), gapped.ID.String()).Status, "applied")

	src.setVerdict(filler.ID, wsledger.Ignored, "signed with a revoked key")
	same("revocation cut")
	e := event(t, trk.View(now, 0), filler.ID.String())
	if e.Status != "flagged" || !e.Ignored {
		t.Fatalf("cut event = %s ignored=%v", e.Status, e.Ignored)
	}
}
