package wstracker

import (
	"strings"
	"testing"
)

// item.move: the accepted paths on the reasons.json genesis (the refusals
// are reasons.json cases, the concurrent folds case-move-* goldens).

func TestMoveByCreatorAndLiveAssignee(t *testing.T) {
	task := func(title string, kv ...any) map[string]any {
		p := map[string]any{"kind": "task", "title": title}
		for i := 0; i < len(kv); i += 2 {
			p[kv[i].(string)] = kv[i+1]
		}
		return p
	}
	v := topicView(t, "ada.pc", []step{
		{"ada.pc", "item.create", task("A")},                                                    // ada.pc#1
		{"ada.pc", "item.create", task("B")},                                                    // ada.pc#2
		{"ada.pc", "item.assign", map[string]any{"item": "ada.pc#2", "targets": []any{"maya"}}}, // maya live on B
		{"ada.pc", "item.create", map[string]any{"kind": "topic", "title": "T"}},                // ada.pc#4
		{"ada.pc", "item.create", task("T.1", "parent", "ada.pc#4")},                            // ada.pc#5
		{"ada.pc", "item.create", task("T.2", "parent", "ada.pc#4")},                            // ada.pc#6
		{"ada.pc", "item.assign", map[string]any{"item": "ada.pc#5", "targets": []any{"maya"}}},
		{"lena.pc", "comment.add", map[string]any{"item": "ada.pc#1", "body": "before the move"}},
		// the creator moves A under B (ada created both)
		{"ada.pc", "item.move", map[string]any{"item": "ada.pc#1", "parent": "ada.pc#2"}},
		// B's live assignee moves B to top level, first: the item rule alone
		{"maya.pc", "item.move", map[string]any{"item": "ada.pc#2", "parent": nil, "after": ""}},
		// her own M under B: the destination passes on her live assignment
		{"maya.pc", "item.create", task("M")},
		{"maya.pc", "item.move", map[string]any{"item": "maya.pc#2", "parent": "ada.pc#2"}},
		// T.1's live assignee reorders it under the topic T: the same parent,
		// so T's ownership does not apply
		{"maya.pc", "item.move", map[string]any{"item": "ada.pc#5", "parent": "ada.pc#4", "after": "ada.pc#6"}},
		// a reorder inside B back to first
		{"ada.pc", "item.move", map[string]any{"item": "ada.pc#1", "parent": "ada.pc#2", "after": ""}},
	})
	r := roots(v)
	wantEq(t, "B first at top level", r[0], "ada.pc#2")
	wantEq(t, "B's children", strings.Join(v.Items["ada.pc#2"].Children, " "), "ada.pc#1 maya.pc#2")
	wantEq(t, "T's children", strings.Join(v.Items["ada.pc#4"].Children, " "), "ada.pc#6 ada.pc#5")
	wantEq(t, "A's parent", deref(v.Items["ada.pc#1"].Parent), "ada.pc#2")
	wantEq(t, "A's depth", v.Items["ada.pc#1"].Depth, 1)

	// A move keeps the item's comments, assignments, and history.
	thread := strings.Join(v.Items["ada.pc#1"].Thread, " ")
	wantEq(t, "A's thread keeps the comment", strings.Contains(thread, "lena.pc#1"), true)
	wantEq(t, "A's thread keeps its moves", strings.Contains(thread, "ada.pc#8") && strings.Contains(thread, "ada.pc#9"), true)
	open := openOn(v, "ada.pc#2")
	if len(open) != 1 || open[0].To != "maya" || open[0].State != "live" {
		t.Fatalf("B's open assignments = %+v, want maya live", open)
	}
}

func TestMoveSubtreeTravels(t *testing.T) {
	v := topicView(t, "ada.pc", []step{
		{"ada.pc", "item.create", map[string]any{"kind": "task", "title": "A"}},
		{"ada.pc", "item.create", map[string]any{"kind": "task", "title": "A.1", "parent": "ada.pc#1"}},
		{"ada.pc", "item.create", map[string]any{"kind": "task", "title": "A.1.1", "parent": "ada.pc#2"}},
		{"ada.pc", "item.create", map[string]any{"kind": "task", "title": "D"}},
		{"ada.pc", "item.move", map[string]any{"item": "ada.pc#1", "parent": "ada.pc#4"}},
	})
	wantEq(t, "item order", strings.Join(v.ItemOrder, " "), "ada.pc#4 ada.pc#1 ada.pc#2 ada.pc#3")
	wantEq(t, "A.1.1 depth", v.Items["ada.pc#3"].Depth, 3)
}
