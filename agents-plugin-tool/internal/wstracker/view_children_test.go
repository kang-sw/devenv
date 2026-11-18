package wstracker

import (
	"strings"
	"testing"
)

func TestViewRollupVisibleChildrenKeepFoldOrder(t *testing.T) {
	v := topicView(t, "maya.pc", []step{
		{"maya.pc", TypeItemCreate, map[string]any{"kind": "task", "title": "Parent"}},
		{"maya.pc", TypeItemCreate, map[string]any{"kind": "task", "title": "First", "parent": "maya.pc#1"}},
		{"maya.pc", TypeItemCreate, map[string]any{"kind": "task", "title": "Second", "parent": "maya.pc#1", "after": ""}},
		{"maya.pc", TypeItemCreate, map[string]any{"kind": "task", "title": "Grandchild", "parent": "maya.pc#2"}},
		{"maya.pc", TypeItemCreate, map[string]any{"kind": "task", "title": "Hidden branch", "parent": "maya.pc#1"}},
		{"maya.pc", TypeItemCreate, map[string]any{"kind": "task", "title": "Pruned grandchild", "parent": "maya.pc#5"}},
		{"maya.pc", TypeItemStatus, map[string]any{"item": "maya.pc#2", "status": "blocked"}},
		{"maya.pc", TypeItemStatus, map[string]any{"item": "maya.pc#3", "status": "blocked"}},
		{"maya.pc", TypeItemStatus, map[string]any{"item": "maya.pc#4", "status": "blocked"}},
		{"maya.pc", TypeItemStatus, map[string]any{"item": "maya.pc#6", "status": "blocked"}},
		{"maya.pc", TypeItemHide, map[string]any{"item": "maya.pc#5"}},
	})
	parent := v.Items["maya.pc#1"]
	// Display placement is deliberately different from fold order. Rollup
	// traversal must retain iorder, not reuse the display sibling index.
	wantEq(t, "display children", strings.Join(parent.Children, " "), "maya.pc#3 maya.pc#2 maya.pc#5")
	wantEq(t, "direct visible total", parent.Rollup.Total, 2)
	wantEq(t, "direct blocked count", parent.Rollup.Counts["blocked"], 2)
	wantEq(t, "blocked depth-first fold order", strings.Join(parent.Rollup.Blocked, " "), "maya.pc#2 maya.pc#4 maya.pc#3")
	// Hidden children prune traversal from their parent, but a hidden item's
	// own view still derives its visible children exactly as before.
	hidden := v.Items["maya.pc#5"]
	wantEq(t, "hidden item", hidden.Hidden, true)
	wantEq(t, "hidden parent's visible total", hidden.Rollup.Total, 1)
	wantEq(t, "hidden parent's blocked child", strings.Join(hidden.Rollup.Blocked, " "), "maya.pc#6")
	wantEq(t, "leaf total", v.Items["maya.pc#4"].Rollup.Total, 0)
}
