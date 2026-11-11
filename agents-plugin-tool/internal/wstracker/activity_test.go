package wstracker

import (
	"fmt"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsledger"
)

// activityOf builds the reasons.json store from steps and returns each
// event's activity as one line per activity, "<kind> actor=<a> target=<t>
// status=<s> concerns=<c,...>", keyed by the event's 1-based step position.
func activityOf(t *testing.T, steps []step) map[int][]string {
	t.Helper()
	rc := loadReasonCases(t)
	evs := history(steps)
	tr := New(newMemSource(rc.Genesis, "ada.pc", evs), "ada.pc")
	out := map[int][]string{}
	for i, e := range evs {
		acts, settled := tr.Activity(e.ID)
		if !settled {
			t.Fatalf("step %d (%s) not settled", i+1, e.ID)
		}
		for _, a := range acts {
			if a.Event != e.ID || !a.TS.Equal(e.TS) {
				t.Fatalf("step %d: activity names %s at %s", i+1, a.Event, a.TS)
			}
			out[i+1] = append(out[i+1], fmt.Sprintf("%s actor=%s target=%s status=%s concerns=%s",
				a.Kind, a.Actor, a.Target, a.Status, strings.Join(a.Concerns, ",")))
		}
	}
	return out
}

func wantActs(t *testing.T, got map[int][]string, pos int, want ...string) {
	t.Helper()
	if strings.Join(got[pos], "\n") != strings.Join(want, "\n") {
		t.Fatalf("step %d activity:\n got %q\nwant %q", pos, got[pos], want)
	}
}

// On reasons.json maya leads lena and omar, raj leads sven, ada leads maya,
// raj, and bo.
func TestActivityAssignmentsCommentsAndStatus(t *testing.T) {
	got := activityOf(t, []step{
		{"maya.pc", "item.create", map[string]any{"kind": "task", "title": "Index"}},                                        // 1
		{"raj.pc", "comment.add", map[string]any{"item": "maya.pc#1", "body": "before raj holds it"}},                       // 2
		{"maya.pc", "item.assign", map[string]any{"item": "maya.pc#1", "targets": []any{"lena"}, "completion": "assigner"}}, // 3
		{"maya.pc", "item.assign", map[string]any{"item": "maya.pc#1", "targets": []any{"raj"}}},                            // 4
		{"lena.pc", "item.status", map[string]any{"item": "maya.pc#1", "status": "doing"}},                                  // 5
		{"lena.pc", "comment.add", map[string]any{"item": "maya.pc#1", "body": "on it"}},                                    // 6
		{"lena.pc", "item.close", map[string]any{"item": "maya.pc#1"}},                                                      // 7
		{"maya.pc", "item.reject", map[string]any{"item": "maya.pc#1", "assignee": "lena"}},                                 // 8
		{"lena.pc", "item.close", map[string]any{"item": "maya.pc#1"}},                                                      // 9
		{"maya.pc", "item.confirm", map[string]any{"item": "maya.pc#1", "assignee": "lena"}},                                // 10
		{"maya.pc", "item.unassign", map[string]any{"item": "maya.pc#1", "assignee": "lena"}},                               // 11
		{"omar.pc", "comment.add", map[string]any{"item": "maya.pc#1", "body": "after lena left"}},                          // 12
		{"maya.pc", "item.edit", map[string]any{"item": "maya.pc#1", "title": "Index 2"}},                                   // 13
	})
	wantActs(t, got, 1)
	// raj did not hold it yet: the comment concerns the creator only.
	wantActs(t, got, 2, "comment actor=raj target= status= concerns=maya")
	// lena is below maya: forced, live at once; raj is not: pending.
	wantActs(t, got, 3, "assigned actor=maya target=lena status= concerns=lena")
	wantActs(t, got, 4, "assign_pending actor=maya target=raj status= concerns=raj")
	wantActs(t, got, 5, "status actor=lena target= status=doing concerns=lena,maya")
	wantActs(t, got, 6, "comment actor=lena target= status= concerns=lena,maya")
	// assigner completion: the close awaits maya's confirm.
	wantActs(t, got, 7, "closed actor=lena target= status=awaiting concerns=lena,maya")
	wantActs(t, got, 8, "status actor=maya target=lena status=doing concerns=lena,maya")
	wantActs(t, got, 10, "closed actor=maya target=lena status=done concerns=lena,maya")
	wantActs(t, got, 11)
	// judged at the event: lena no longer holds it, and the fallback holder
	// is the creator.
	wantActs(t, got, 12, "comment actor=omar target= status= concerns=maya")
	wantActs(t, got, 13)
}

func TestActivityRuleRequestAndAccept(t *testing.T) {
	got := activityOf(t, []step{
		{"maya.pc", "item.create", map[string]any{"kind": "task", "title": "Index"}},
		{"maya.pc", "item.assign", map[string]any{"item": "maya.pc#1", "targets": []any{map[string]any{"root": "raj", "levels": 1}}}},
		{"raj.pc", "item.accept", map[string]any{"item": "maya.pc#1"}},
	})
	// a rule rooted outside maya's reach is a request to its root.
	wantActs(t, got, 2, "assign_pending actor=maya target=raj status= concerns=raj")
	// raj's accept makes the rule live: its record goes to sven at once.
	wantActs(t, got, 3, "assigned actor=raj target=sven status= concerns=sven")
}

func TestActivityFlaggedAndUnknownEvents(t *testing.T) {
	rc := loadReasonCases(t)
	evs := history([]step{
		{"maya.pc", "item.create", map[string]any{"kind": "task", "title": "Index"}},
		// only the creator assigns: flagged.
		{"raj.pc", "item.assign", map[string]any{"item": "maya.pc#1", "targets": []any{"raj"}}},
	})
	tr := New(newMemSource(rc.Genesis, "ada.pc", evs), "ada.pc")
	if acts, settled := tr.Activity(evs[1].ID); !settled || len(acts) != 0 {
		t.Fatalf("flagged: settled %v, %v", settled, acts)
	}
	if acts, settled := tr.Activity(wsledger.EventID{Member: "maya", Clone: "pc", Seq: 9}); settled || len(acts) != 0 {
		t.Fatalf("unknown: settled %v, %v", settled, acts)
	}
}

// Taking work: a claim is the claimer's assignment and concerns the opener;
// a take request concerns the holder and the creator; an approval is the
// requester's assignment and concerns the holder it takes from; a denial
// goes back to the requester. On the reasons.json genesis ada leads maya;
// maya leads lena and omar.
func TestActivityClaimsAndTakeRequests(t *testing.T) {
	got := activityOf(t, []step{
		task("ada.pc", "Free"), // 1
		{"ada.pc", "item.open", map[string]any{"item": "ada.pc#1", "scope": "team"}}, // 2
		{"lena.pc", "item.claim", map[string]any{"item": "ada.pc#1"}},                // 3
	})
	wantActs(t, got, 2)
	wantActs(t, got, 3, "assigned actor=lena target=lena status= concerns=ada,lena")

	steps := append(append([]step(nil), held...), // omar.pc#1 asks for maya's assignment (step 4)
		step{"ada.pc", "item.approve", map[string]any{"item": "ada.pc#1", "request": "omar.pc#1"}}) // 5
	got = activityOf(t, steps)
	wantActs(t, got, 4, "take_request actor=omar target=maya status= concerns=ada,maya")
	wantActs(t, got, 5, "assigned actor=ada target=omar status= concerns=maya,omar")

	got = activityOf(t, append(append([]step(nil), held...),
		step{"maya.pc", "item.deny", map[string]any{"item": "ada.pc#1", "request": "omar.pc#1"}},      // 5
		step{"ada.pc", "item.deny", map[string]any{"item": "ada.pc#1", "request": "omar.pc#1"}},       // 6: already answered
		step{"omar.pc", "item.request", map[string]any{"item": "ada.pc#1", "assignee": "maya"}},       // 7
		step{"omar.pc", "item.unrequest", map[string]any{"item": "ada.pc#1", "request": "omar.pc#2"}}, // 8
	))
	wantActs(t, got, 5, "take_denied actor=maya target=omar status= concerns=omar")
	wantActs(t, got, 6)
	wantActs(t, got, 8)
}
