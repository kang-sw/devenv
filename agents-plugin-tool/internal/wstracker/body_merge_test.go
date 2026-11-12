package wstracker

import (
	"reflect"
	"strings"
	"testing"
)

// The live assignee edits an item, and a body edit merges three-way from
// the base its editor saw. The refusals (a non-assignee, a pending
// assignee, base shape; hide and assign by a live assignee stay
// creator-only) are reasons.json cases; the Go/engine agreement on merges
// is the case-body-merge golden.

// bodyItem is ada's task "Doc" (ada.pc#1) with body, assigned to maya, who
// holds it live.
func bodyItem(body string) []step {
	return []step{
		{"ada.pc", "item.create", map[string]any{"kind": "task", "title": "Doc", "body": body}},
		{"ada.pc", "item.assign", map[string]any{"item": "ada.pc#1", "targets": []any{"maya"}}},
	}
}

// fromCreate is the seen of a write made having seen only bodyItem.
var fromCreate = map[string]int{"ada.pc": 2}

func edit(stream, body string, base []any, origin string, seen map[string]int) step {
	p := map[string]any{"item": "ada.pc#1", "body": body}
	if base != nil {
		p["base"] = base
	}
	if seen == nil {
		return step{stream, "item.edit", p, origin}
	}
	return step{stream, "item.edit", p, origin, seen}
}

func TestLiveAssigneeEditsEveryField(t *testing.T) {
	v := topicView(t, "ada.pc", append(bodyItem("x"),
		step{"maya.pc", "item.edit", map[string]any{"item": "ada.pc#1", "title": "Doc v2", "project": "core", "priority": 2,
			"due": "2026-12-01", "body": "y", "base": []any{"ada.pc#1"}}},
		// an older stage write (todo) by the live assignee: judged by the
		// widened rule, and with the item held by maya it changes nothing
		step{"maya.pc", "item.edit", map[string]any{"item": "ada.pc#1", "stage": nil}},
	))
	it := v.Items["ada.pc#1"]
	wantEq(t, "title", it.Title, "Doc v2")
	wantEq(t, "body", it.Body, "y")
	wantEq(t, "project", it.Project, "core")
	wantEq(t, "priority", it.Priority, 2)
	wantEq(t, "body revision", len(it.BodyRev), 1)
	wantEq(t, "body revision head", it.BodyRev[0], "maya.pc#1")
}

func TestDispatchEditByLiveAssignee(t *testing.T) {
	tm := newTeam(t)
	v := tm.view("maya.pc")
	var mine string
	for id, it := range v.Items {
		if it.Author == "maya" {
			continue
		}
		for _, aid := range it.Assignments {
			if a := v.Assignments[aid]; a.To == "maya" && a.State == "live" && !a.Fallback && !it.Hidden {
				mine = id
			}
		}
	}
	if mine == "" {
		t.Fatal("the team fixture has no item maya holds live and did not create")
	}
	rev := v.Items[mine].BodyRev
	base := make([]any, len(rev))
	for i, x := range rev {
		base[i] = x
	}
	tm.do("maya.pc", TypeItemEdit, map[string]any{"item": mine, "body": "edited by the assignee", "base": base})
	wantEq(t, "body", tm.view("maya.pc").Items[mine].Body, "edited by the assignee")
}

func TestBodyMergeFold(t *testing.T) {
	const base = "L1\nL2\nL3\nL4\nL5"
	create := []any{"ada.pc#1"}
	for _, c := range []struct {
		name  string
		steps []step
		body  string
		// marked is the events carrying a "field" marker on the body;
		// superseded those carrying a superseded note.
		marked, superseded []string
	}{
		{
			name: "a base covering every applied write replaces",
			steps: []step{
				edit("ada.pc", "one", create, "", nil),
				edit("maya.pc", "two", []any{"ada.pc#3"}, "", nil),
			},
			body: "two",
		},
		{
			name: "two edits from one base on different lines both survive, unmarked across members",
			steps: []step{
				edit("ada.pc", "L1 ada\nL2\nL3\nL4\nL5", create, "", fromCreate),
				edit("maya.pc", "L1\nL2\nL3\nL4\nL5 maya", create, "", fromCreate),
			},
			body: "L1 ada\nL2\nL3\nL4\nL5 maya",
		},
		{
			name: "touching hunks overlap: the later in fold order wins, both marked",
			steps: []step{
				edit("ada.pc", "L1\nL2 ada\nL3\nL4\nL5", create, "", fromCreate),
				edit("maya.pc", "L1\nL2\nL3 maya\nL4\nL5", create, "", fromCreate),
			},
			body:   "L1\nL2\nL3 maya\nL4\nL5",
			marked: []string{"ada.pc#3", "maya.pc#1"},
		},
		{
			name: "one change made on both sides is no overlap",
			steps: []step{
				edit("ada.pc", "L1 fix\nL2\nL3\nL4\nL5", create, "", fromCreate),
				edit("maya.pc", "L1 fix\nL2\nL3\nL4\nL5 maya", create, "", fromCreate),
			},
			body: "L1 fix\nL2\nL3\nL4\nL5 maya",
		},
		{
			name: "a human write beats its member's concurrent agent write on an overlapping hunk",
			steps: []step{
				edit("maya.pc", "L1 human\nL2\nL3\nL4\nL5", create, "", fromCreate),
				edit("maya.lap", "L1 agent\nL2\nL3\nL4\nL5 agent", create, OriginAgent, fromCreate),
			},
			body:       "L1 human\nL2\nL3\nL4\nL5 agent",
			superseded: []string{"maya.lap#1"},
		},
		{
			name: "a human write beats its member's concurrent agent whole replace",
			steps: []step{
				edit("maya.pc", "L1 human\nL2\nL3\nL4\nL5", create, "", fromCreate),
				edit("maya.lap", "agent", nil, OriginAgent, fromCreate),
			},
			body:       "L1 human\nL2\nL3\nL4\nL5",
			superseded: []string{"maya.lap#1"},
		},
		{
			name: "an agent write folding first loses to the later human write as usual",
			steps: []step{
				edit("maya.lap", "L1 agent\nL2\nL3\nL4\nL5", create, OriginAgent, fromCreate),
				edit("maya.pc", "L1 human\nL2\nL3\nL4\nL5", create, "", fromCreate),
			},
			body:       "L1 human\nL2\nL3\nL4\nL5",
			superseded: []string{"maya.lap#1"},
		},
		{
			name: "an edit without a base replaces whole",
			steps: []step{
				edit("ada.pc", "L1 ada\nL2\nL3\nL4\nL5", create, "", fromCreate),
				edit("maya.pc", "L1\nL2\nL3\nL4\nL5 maya", nil, "", fromCreate),
			},
			body:   "L1\nL2\nL3\nL4\nL5 maya",
			marked: []string{"ada.pc#3", "maya.pc#1"},
		},
		{
			name: "an edit with an unknown base replaces whole",
			steps: []step{
				edit("ada.pc", "L1 ada\nL2\nL3\nL4\nL5", create, "", fromCreate),
				edit("maya.pc", "L1\nL2\nL3\nL4\nL5 maya", []any{"zed.pc#4"}, "", fromCreate),
			},
			body:   "L1\nL2\nL3\nL4\nL5 maya",
			marked: []string{"ada.pc#3", "maya.pc#1"},
		},
		{
			name: "an overlap names only the writes whose lines it touched",
			steps: []step{
				edit("ada.pc", "L1 ada\nL2\nL3\nL4\nL5", create, "", fromCreate),
				edit("maya.pc", "L1\nL2\nL3\nL4\nL5 maya", create, "", fromCreate),
				edit("ada.lap", "L1 lap\nL2\nL3\nL4\nL5", create, "", fromCreate),
			},
			body:       "L1 lap\nL2\nL3\nL4\nL5 maya",
			superseded: []string{"ada.pc#3"},
		},
		{
			name: "the human precedence holds per hunk: the agent still wins another member's earlier hunk",
			steps: []step{
				edit("maya.pc", "L1 human\nL2\nL3\nL4\nL5", create, "", fromCreate),
				edit("ada.pc", "L1\nL2\nL3\nL4\nL5 ada", create, "", fromCreate),
				edit("maya.lap", "L1 agent\nL2\nL3\nL4\nL5 agent", create, OriginAgent, fromCreate),
			},
			body:       "L1 human\nL2\nL3\nL4\nL5 agent",
			marked:     []string{"ada.pc#3", "maya.lap#1"},
			superseded: []string{"maya.lap#1"},
		},
		{
			name: "a same-member overlap gets the quiet note, not a marker",
			steps: []step{
				edit("maya.pc", "L1 pc\nL2\nL3\nL4\nL5", create, "", fromCreate),
				edit("maya.lap", "L1 lap\nL2\nL3\nL4\nL5", create, "", fromCreate),
			},
			body:       "L1 lap\nL2\nL3\nL4\nL5",
			superseded: []string{"maya.pc#1"},
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			v := topicView(t, "ada.pc", append(bodyItem(base), c.steps...))
			wantEq(t, "body", v.Items["ada.pc#1"].Body, c.body)
			var marked, superseded []string
			for _, e := range v.Events {
				for _, m := range e.Markers {
					if m.Field == "body of ada.pc-1" {
						marked = append(marked, e.ID)
						break
					}
				}
				for _, x := range e.Superseded {
					if x.Field == "body of ada.pc-1" {
						superseded = append(superseded, e.ID)
						break
					}
				}
			}
			if !reflect.DeepEqual(marked, c.marked) {
				t.Errorf("marked = %v, want %v", marked, c.marked)
			}
			if !reflect.DeepEqual(superseded, c.superseded) {
				t.Errorf("superseded = %v, want %v", superseded, c.superseded)
			}
		})
	}
}

// att is a test attachment whose oid repeats c.
func att(c string) map[string]any {
	return map[string]any{"oid": strings.Repeat(c, 64), "name": c + ".txt", "mime": "text/plain", "size": 1}
}

// TestBodyMergeAttachments pins the three-way attachment merge: from the
// base value, an oid either side removed is gone and one either side added
// is kept, whether or not the adding write's lines survive.
func TestBodyMergeAttachments(t *testing.T) {
	const base = "L1\nL2\nL3\nL4\nL5"
	create := []any{"ada.pc#1"}
	withAtts := func(s step, atts ...string) step {
		p := s[2].(map[string]any)
		l := []any{}
		for _, c := range atts {
			l = append(l, att(c))
		}
		p["attachments"] = l
		return s
	}
	for _, c := range []struct {
		name  string
		steps []step
		want  string
	}{
		{
			name: "a merging edit that removes an attachment keeps the removal",
			steps: []step{
				withAtts(edit("ada.pc", "L1 ada\nL2\nL3\nL4\nL5", create, "", fromCreate), "a", "b"),
				withAtts(edit("maya.pc", "L1\nL2\nL3\nL4\nL5 maya", create, "", fromCreate), "a"),
			},
			want: "a",
		},
		{
			name: "a removal folded first stays removed under a later merge that kept it",
			steps: []step{
				withAtts(edit("maya.pc", "L1\nL2\nL3\nL4\nL5 maya", create, "", fromCreate), "a"),
				withAtts(edit("ada.pc", "L1 ada\nL2\nL3\nL4\nL5", create, "", fromCreate), "a", "b"),
			},
			want: "a",
		},
		{
			name: "a text-neutral merging edit keeps the attachment it adds",
			steps: []step{
				withAtts(edit("ada.pc", "L1 ada\nL2\nL3\nL4\nL5", create, "", fromCreate), "a", "b"),
				withAtts(edit("maya.pc", base, create, "", fromCreate), "a", "b", "c"),
			},
			want: "abc",
		},
		{
			name: "both sides add: cur's order, then the merging edit's",
			steps: []step{
				withAtts(edit("ada.pc", "L1 ada\nL2\nL3\nL4\nL5", create, "", fromCreate), "a", "b", "d"),
				withAtts(edit("maya.pc", "L1\nL2\nL3\nL4\nL5 maya", create, "", fromCreate), "e", "a", "b"),
			},
			want: "abde",
		},
		{
			name: "an addition whose lines an overlap overwrote is kept",
			steps: []step{
				withAtts(edit("ada.pc", "L1 ada\nL2\nL3\nL4\nL5", create, "", fromCreate), "a", "b", "d"),
				withAtts(edit("maya.pc", "L1 maya\nL2\nL3\nL4\nL5", create, "", fromCreate), "a", "b"),
			},
			want: "abd",
		},
		{
			name: "a plain replace sets its own list",
			steps: []step{
				withAtts(edit("ada.pc", "one", create, "", nil), "a", "b"),
				withAtts(edit("maya.pc", "two", []any{"ada.pc#3"}, "", nil), "c"),
			},
			want: "c",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			steps := bodyItem(base)
			steps[0] = withAtts(steps[0], "a", "b")
			v := topicView(t, "ada.pc", append(steps, c.steps...))
			got := ""
			for _, a := range v.Items["ada.pc#1"].Attachments {
				got += a.OID[:1]
			}
			wantEq(t, "attachments", got, c.want)
		})
	}
}

func TestDiffLines(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want []hunk
	}{
		{"a\nb\nc", "a\nb\nc", nil},
		{"a\nb\nc", "a\nB\nc", []hunk{{1, 2, 1, 2}}},
		{"a\nb\nc", "a\nc", []hunk{{1, 2, 1, 1}}},
		{"a\nc", "a\nb\nc", []hunk{{1, 1, 1, 2}}},
		{"a\nb\nc\nd", "x\nb\nc\ny", []hunk{{0, 1, 0, 1}, {3, 4, 3, 4}}},
		{"a\nb", "c\nd", []hunk{{0, 2, 0, 2}}},
	} {
		got := diffLines(splitLines(c.a), splitLines(c.b))
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("diffLines(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}
