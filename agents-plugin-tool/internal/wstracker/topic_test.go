package wstracker

import (
	"strings"
	"testing"
	"time"
)

// Topics take an ownership-only assignment (one holder, no status), and a
// todo create folds as a task. The refusals are reasons.json cases; these
// pin the accepted paths and where a held topic shows.

// topicView folds a linear history on the reasons.json genesis for viewer
// and fails when any event is not applied.
func topicView(t *testing.T, viewer string, steps []step) *View {
	t.Helper()
	rc := loadReasonCases(t)
	evs := history(steps)
	v := Derive(newMemSource(rc.Genesis, viewer, evs), viewer, time.Time{}, 0)
	for _, e := range v.Events {
		if e.Status != "applied" {
			t.Fatalf("%s is %s: %v", e.ID, e.Status, deref(e.Reason))
		}
	}
	return v
}

// openOn lists the live and pending assignments on item.
func openOn(v *View, item string) []Assignment {
	var out []Assignment
	for _, id := range v.Items[item].Assignments {
		if a := v.Assignments[id]; a.State == "live" || a.State == "pending" {
			out = append(out, a)
		}
	}
	return out
}

func TestTopicAssignBothFormsStoreNoCompletion(t *testing.T) {
	v := topicView(t, "ada.pc", []step{
		{"ada.pc", "item.create", map[string]any{"kind": "topic", "title": "A"}},
		{"ada.pc", "item.assign", map[string]any{"item": "ada.pc#1", "targets": []any{"ada"}, "completion": "assigner"}},
		{"ada.pc", "item.create", map[string]any{"kind": "topic", "title": "B"}},
		{"ada.pc", "item.assign", map[string]any{"item": "ada.pc#3", "mode": "chosen", "people": []any{"maya"}, "completion": "assigner"}},
	})
	for item, holder := range map[string]string{"ada.pc#1": "ada", "ada.pc#3": "maya"} {
		open := openOn(v, item)
		if len(open) != 1 || open[0].To != holder || open[0].State != "live" {
			t.Fatalf("%s open assignments = %+v, want %s live", item, open, holder)
		}
		wantEq(t, item+" completion", open[0].Completion, "")
		if v.Items[item].Status != nil {
			t.Fatalf("%s status = %q, want none", item, *v.Items[item].Status)
		}
		wantEq(t, item+" counted", v.Items[item].Counted, false)
	}
}

func TestTopicHandoffReceiverReleases(t *testing.T) {
	v := topicView(t, "ada.pc", []step{
		{"ada.pc", "item.create", map[string]any{"kind": "topic", "title": "T"}},
		{"ada.pc", "item.assign", map[string]any{"item": "ada.pc#1", "targets": []any{"ada"}}},
		{"ada.pc", "item.handoff", map[string]any{"item": "ada.pc#1", "to": "maya"}},
		{"maya.pc", "item.accept", map[string]any{"item": "ada.pc#1"}},
		// maya holds it with ada as the assigner; the holder may release it
		{"maya.pc", "item.unassign", map[string]any{"item": "ada.pc#1", "assignee": "maya"}},
		// released: the creator may hand it to someone new
		{"ada.pc", "item.assign", map[string]any{"item": "ada.pc#1", "targets": []any{"raj"}}},
	})
	open := openOn(v, "ada.pc#1")
	if len(open) != 1 || open[0].To != "raj" {
		t.Fatalf("open assignments = %+v, want raj only", open)
	}
	released := v.Assignments["ada.pc#2>ada"]
	wantEq(t, "ada's record", released.State, "handed")
	got := v.Assignments["ada.pc#3>maya"]
	wantEq(t, "maya's record", got.State, "unassigned")
	wantEq(t, "maya's record ended by", got.Ended.By, "maya")
}

func TestHeldTopicShowsInMineOnly(t *testing.T) {
	steps := []step{
		{"ada.pc", "item.create", map[string]any{"kind": "topic", "title": "Topic"}},
		{"ada.pc", "item.assign", map[string]any{"item": "ada.pc#1", "targets": []any{"maya"}}},
		{"ada.pc", "item.create", map[string]any{"kind": "task", "title": "Task"}},
		{"ada.pc", "item.assign", map[string]any{"item": "ada.pc#3", "targets": []any{"maya"}}},
	}
	topicRec, taskRec := "ada.pc#2>maya", "ada.pc#4>maya"

	maya := topicView(t, "maya.pc", steps)
	wantIn(t, "maya's assignments", maya.My.Assignments, topicRec)
	wantNotIn(t, "maya's today work", maya.My.Today.Work, topicRec)
	wantIn(t, "maya's today work", maya.My.Today.Work, taskRec)
	wantNotIn(t, "maya's badge", maya.My.Badge, "a:"+topicRec)
	wantIn(t, "maya's badge", maya.My.Badge, "a:"+taskRec)

	ada := topicView(t, "ada.pc", steps)
	row := teamRow(t, ada, "maya")
	wantNotIn(t, "maya's team open", row.Open, topicRec)
	wantIn(t, "maya's team open", row.Open, taskRec)
	wantNotIn(t, "ada's created", ada.My.Created, "ada.pc#1")

	// a pending topic handoff shows to the receiver like any pending one,
	// and stays out of the team counts
	steps = append(steps, step{"maya.pc", "item.handoff", map[string]any{"item": "ada.pc#1", "to": "raj"}})
	raj := topicView(t, "raj.pc", steps)
	hand := "maya.pc#1>raj"
	wantIn(t, "raj's received", raj.My.Received.Pending, hand)
	wantIn(t, "raj's badge", raj.My.Badge, "p:"+hand)
	ada = topicView(t, "ada.pc", steps)
	wantNotIn(t, "raj's team pending", teamRow(t, ada, "raj").Pending, hand)
	wantNotIn(t, "maya's team waiting", teamRow(t, ada, "maya").Waiting, hand)
	steps = append(steps, step{"maya.pc", "item.close", map[string]any{"item": "ada.pc#3"}})
	wantEq(t, "maya's team done", teamRow(t, topicView(t, "ada.pc", steps), "maya").Counts["done"], 1)
}

func TestTodoCreateFoldsAsTask(t *testing.T) {
	v := topicView(t, "ada.pc", []step{
		{"ada.pc", "item.create", map[string]any{"kind": "todo", "title": "Old todo"}},
		{"ada.pc", "item.assign", map[string]any{"item": "ada.pc#1", "targets": []any{"ada"}}},
		{"ada.pc", "item.status", map[string]any{"item": "ada.pc#1", "status": "doing"}},
	})
	wantEq(t, "kind", v.Items["ada.pc#1"].Kind, "task")
	wantEq(t, "status", *v.Items["ada.pc#1"].Status, "doing")
	// the event keeps the payload it was written with
	if !strings.Contains(string(v.Events[0].Payload), `"kind":"todo"`) {
		t.Fatalf("create payload = %s, want the todo kind it was written with", v.Events[0].Payload)
	}
}
