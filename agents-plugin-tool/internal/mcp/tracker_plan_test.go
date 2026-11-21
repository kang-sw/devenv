// Tracker plan tests.
package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsledger"
	"github.com/kang-sw/devenv/internal/wstracker"
	"github.com/kang-sw/devenv/internal/wstrackerhost"
	"github.com/kang-sw/devenv/internal/wsxmlcmd"
)

// trackerTestReach is the org tree trackerPlan's writer ada sees: bo leads
// cy and ada leads bo, so bo and cy are below ada; raj and lena are peers.
func trackerTestReach(store string) (map[string]int, error) {
	return map[string]int{"bo": 1, "cy": 2}, nil
}

// trackerPlan plans commands for the store team with ada as its member and
// fails the test on any problem.
func trackerPlan(t *testing.T, commands string) *trackerBatch {
	t.Helper()
	var problems wsxmlcmd.Problems
	b := planTrackerBatch(&problems, `<tk:store name="team">`+commands+`</tk:store>`, []wstrackerhost.StoreEntry{{ID: "team", Member: "ada"}}, trackerTestReach)
	if len(problems.List) > 0 {
		t.Fatalf("problems: %q", problems.List)
	}
	return b
}

// start rides item.create and item.edit like due: an attribute sets it,
// clear="start" writes null, and an edit that names only start carries no
// other field.
func TestTrackerUpdateStart(t *testing.T) {
	b := trackerPlan(t, `<tk:item.create kind="task" title="t" start="2026-11-05" due="2026-11-10"/>`+
		`<tk:item.edit item="$1" start="2026-11-07"/>`+
		`<tk:item.edit item="$1" clear="start"/>`+
		`<tk:item.edit item="$1" start="2026-11-08" clear="due"/>`)
	for i, want := range []map[string]any{
		{"kind": "task", "title": "t", "start": "2026-11-05", "due": "2026-11-10"},
		{"start": "2026-11-07"},
		{"start": nil},
		{"start": "2026-11-08", "due": nil},
	} {
		got := map[string]any{}
		for k, v := range b.events[i].payload {
			if k != "item" {
				got[k] = v
			}
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("event %d (%s) payload %v, want %v", i+1, b.events[i].typ, got, want)
		}
	}

	var problems wsxmlcmd.Problems
	planTrackerBatch(&problems, `<tk:store name="team"><tk:item.create kind="task" title="t"/>`+
		`<tk:item.edit item="$1" start="2026-11-07" clear="start"/>`+
		`<tk:item.edit item="$1" start=""/></tk:store>`, []wstrackerhost.StoreEntry{{ID: "team", Member: "ada"}}, trackerTestReach)
	for _, want := range []string{
		`command 2 <tk:item.edit> (line 1): clear="start" with a start attribute; send one or the other`,
		`command 3 <tk:item.edit> (line 1) start (clear="start" removes it)`,
	} {
		if !slices.ContainsFunc(problems.List, func(p string) bool { return strings.HasPrefix(p, want) }) {
			t.Errorf("problems missing %q:\n%q", want, problems.List)
		}
	}
}

// An item.assign without completion gets it stored by the writer's
// relationship to its targets: assigner for a member below the writer (any
// depth) or a rule rooted at the writer or below them, assignee for the
// writer, a peer, or a rule rooted at a peer; an explicit one is kept and
// never split (the store refuses an explicit assignee below the writer).
func TestTrackerUpdateCompletionDefault(t *testing.T) {
	for attrs, want := range map[string]string{
		`people="ada"`:                          "assignee",
		`people="ada ada"`:                      "assignee",
		`people="bo"`:                           "assigner",
		`people="cy"`:                           "assigner",
		`people="bo cy"`:                        "assigner",
		`people="raj"`:                          "assignee",
		`people="raj lena"`:                     "assignee",
		`rules="ada:1"`:                         "assigner",
		`rules="ada+:*"`:                        "assigner",
		`rules="bo:*"`:                          "assigner",
		`rules="raj:1"`:                         "assignee",
		`people="bo" rules="cy+:1"`:             "assigner",
		`people="raj" rules="lena:*"`:           "assignee",
		`levels="1"`:                            "assigner",
		`people="raj" completion="assigner"`:    "assigner",
		`people="bo" completion="assignee"`:     "assignee",
		`people="ada bo" completion="assigner"`: "assigner",
	} {
		b := trackerPlan(t, `<tk:item.assign item="ada.pc-1" `+attrs+`/>`)
		if len(b.events) != 1 {
			t.Fatalf("%s: %d events, want 1", attrs, len(b.events))
		}
		if got := b.events[0].payload["completion"]; got != want {
			t.Errorf("%s: completion = %v, want %s", attrs, got, want)
		}
	}
}

// An assign whose default-completion targets fall in both classes is
// written as two item.assign events, the second right after the first: the
// first keeps its first target's class, the second holds the rest with the
// other completion. Both carry the command, a $N item, and the origin, and
// every later $N still names its command's event.
func TestTrackerUpdateSplitsAssignByRelationship(t *testing.T) {
	b := trackerPlan(t, `<tk:item.create kind="task" title="a"/>`+
		`<tk:item.assign item="$1" people="raj bo ada cy" origin="agent-auto"/>`+
		`<tk:item.assign item="$1" people="bo" rules="raj:1 cy:*"/>`+
		`<tk:item.create kind="task" title="b" parent="$1"/>`+
		`<tk:comment.add item="$4" refs="$1 $4">hi</tk:comment.add>`)
	next := wsledger.EventID{Member: "ada", Clone: "pc", Seq: 10}
	evs, err := b.newEvents(next)
	if err != nil {
		t.Fatal(err)
	}
	id := func(seq int) string { e := next; e.Seq = seq; return e.String() }
	want := []struct {
		typ, origin string
		payload     map[string]any
	}{
		{wstracker.TypeItemCreate, wstracker.OriginAgent, map[string]any{"kind": "task", "title": "a"}},
		{wstracker.TypeItemAssign, wstracker.OriginAgentAuto, map[string]any{"item": id(10), "mode": "chosen", "people": []any{"raj", "ada"}, "completion": "assignee"}},
		{wstracker.TypeItemAssign, wstracker.OriginAgentAuto, map[string]any{"item": id(10), "mode": "chosen", "people": []any{"bo", "cy"}, "completion": "assigner"}},
		{wstracker.TypeItemAssign, wstracker.OriginAgent, map[string]any{"item": id(10), "targets": []any{"bo", map[string]any{"root": "cy"}}, "completion": "assigner"}},
		{wstracker.TypeItemAssign, wstracker.OriginAgent, map[string]any{"item": id(10), "targets": []any{map[string]any{"root": "raj", "levels": float64(1)}}, "completion": "assignee"}},
		{wstracker.TypeItemCreate, wstracker.OriginAgent, map[string]any{"kind": "task", "title": "b", "parent": id(10)}},
		{wstracker.TypeCommentAdd, wstracker.OriginAgent, map[string]any{"item": id(15), "refs": []any{id(10), id(15)}, "body": "hi"}},
	}
	if len(evs) != len(want) {
		t.Fatalf("got %d events, want %d", len(evs), len(want))
	}
	for i, w := range want {
		var got map[string]any
		if err := json.Unmarshal(evs[i].Payload, &got); err != nil {
			t.Fatal(err)
		}
		if evs[i].Type != w.typ || evs[i].Origin != w.origin || !reflect.DeepEqual(got, w.payload) {
			t.Errorf("event %d = %s %s %s, want %s %s %v", i, evs[i].Type, evs[i].Origin, evs[i].Payload, w.typ, w.origin, w.payload)
		}
	}
	for i, split := range []bool{false, false, true, false, true, false, false} {
		if b.events[i].splitOff != split {
			t.Errorf("event %d splitOff = %v, want %v", i, b.events[i].splitOff, split)
		}
	}
	if b.events[1].cmd.Pos != 2 || b.events[2].cmd.Pos != 2 || b.events[4].cmd.Pos != 3 {
		t.Fatalf("both halves of a split carry their command")
	}
	if b.eventOf[4] != 5 || b.eventOf[5] != 6 {
		t.Fatalf("eventOf after the splits = %v", b.eventOf)
	}
}

// The org tree is read only for an assign that leaves completion to the
// writer and names someone else; a failed read refuses the batch.
func TestTrackerUpdateCompletionDefaultReadsReachOnlyWhenNeeded(t *testing.T) {
	plan := func(commands string) (int, []string) {
		calls := 0
		var problems wsxmlcmd.Problems
		planTrackerBatch(&problems, `<tk:store name="team">`+commands+`</tk:store>`, []wstrackerhost.StoreEntry{{ID: "team", Member: "ada"}},
			func(store string) (map[string]int, error) {
				calls++
				return nil, errors.New("no view")
			})
		return calls, problems.List
	}
	for _, commands := range []string{
		`<tk:item.assign item="ada.pc-1" people="ada"/>`,
		`<tk:item.assign item="ada.pc-1" levels="2"/>`,
		`<tk:item.assign item="ada.pc-1" rules="ada:1"/>`,
		`<tk:item.assign item="ada.pc-1" people="bo" completion="assigner"/>`,
		`<tk:item.create kind="topic" title="t"/>`,
	} {
		if calls, problems := plan(commands); calls != 0 || len(problems) > 0 {
			t.Errorf("%s: %d reads, problems %q", commands, calls, problems)
		}
	}
	calls, problems := plan(`<tk:item.assign item="ada.pc-1" people="bo"/><tk:item.assign item="ada.pc-1" people="raj"/>`)
	if calls != 1 || len(problems) != 1 || !strings.Contains(problems[0], "command 1 <tk:item.assign> (line 1): the org tree, which picks the default completion, could not be read (no view); set completion explicitly") {
		t.Fatalf("a failed read: %d reads, problems %q", calls, problems)
	}
}

// tk:item.status takes every status, the store judging which apply, and an
// assignee naming another member's assignment.
func TestTrackerUpdateStatusTakesAssigneeAndEveryStatus(t *testing.T) {
	for _, st := range []string{"idea", "todo", "doing", "blocked", "in-review", "done", "dropped"} {
		b := trackerPlan(t, `<tk:item.status item="ada.pc-1" status="`+st+`"/>`)
		if got := b.events[0].payload["status"]; got != st {
			t.Errorf("status %s = %v", st, got)
		}
		if _, ok := b.events[0].payload["assignee"]; ok {
			t.Errorf("status %s without assignee writes one", st)
		}
	}
	b := trackerPlan(t, `<tk:item.status item="ada.pc-1" assignee="bo" status="done">wrapped up</tk:item.status>`)
	if want := map[string]any{"item": "ada.pc#1", "assignee": "bo", "status": "done", "note": "wrapped up"}; !reflect.DeepEqual(b.events[0].payload, want) {
		t.Fatalf("payload = %v, want %v", b.events[0].payload, want)
	}
	var problems wsxmlcmd.Problems
	planTrackerBatch(&problems, `<tk:store name="team"><tk:item.status item="ada.pc-1" status="awaiting"/></tk:store>`, []wstrackerhost.StoreEntry{{ID: "team", Member: "ada"}}, trackerTestReach)
	if len(problems.List) != 1 || !strings.Contains(problems.List[0], "status: must be one of idea, todo, doing, blocked, in-review, done, dropped") {
		t.Fatalf("awaiting is not a status to set: %q", problems.List)
	}
}

// A topic the batch creates and does not assign is self-assigned right
// after its create (the writer holds it); a task, or a create still written
// as todo, gets none (the store holds it for its creator), nor does a topic
// the batch assigns by $N, and every $N still names its command's event.
func TestTrackerUpdateSelfAssignsCreates(t *testing.T) {
	b := trackerPlan(t, `<tk:item.create kind="task" title="a"/>`+
		`<tk:item.create kind="todo" title="b" parent="$1" origin="agent-auto"/>`+
		`<tk:item.create kind="topic" title="c"/>`+
		`<tk:item.create kind="topic" title="d"/>`+
		`<tk:item.assign item="$4" people="bo"/>`+
		`<tk:comment.add item="$2" refs="$1 $4">hi</tk:comment.add>`)
	next := wsledger.EventID{Member: "ada", Clone: "pc", Seq: 10}
	evs, err := b.newEvents(next)
	if err != nil {
		t.Fatal(err)
	}
	id := func(seq int) string { e := next; e.Seq = seq; return e.String() }
	want := []struct {
		typ, origin string
		payload     map[string]any
	}{
		{wstracker.TypeItemCreate, wstracker.OriginAgent, map[string]any{"kind": "task", "title": "a"}},
		{wstracker.TypeItemCreate, wstracker.OriginAgentAuto, map[string]any{"kind": "todo", "title": "b", "parent": id(10)}},
		{wstracker.TypeItemCreate, wstracker.OriginAgent, map[string]any{"kind": "topic", "title": "c"}},
		{wstracker.TypeItemAssign, wstracker.OriginAgent, map[string]any{"item": id(12), "mode": "chosen", "people": []any{"ada"}, "completion": "assignee"}},
		{wstracker.TypeItemCreate, wstracker.OriginAgent, map[string]any{"kind": "topic", "title": "d"}},
		{wstracker.TypeItemAssign, wstracker.OriginAgent, map[string]any{"item": id(14), "mode": "chosen", "people": []any{"bo"}, "completion": "assigner"}},
		{wstracker.TypeCommentAdd, wstracker.OriginAgent, map[string]any{"item": id(11), "refs": []any{id(10), id(14)}, "body": "hi"}},
	}
	if len(evs) != len(want) {
		t.Fatalf("got %d events, want %d", len(evs), len(want))
	}
	for i, w := range want {
		var got map[string]any
		if err := json.Unmarshal(evs[i].Payload, &got); err != nil {
			t.Fatal(err)
		}
		if evs[i].Type != w.typ || evs[i].Origin != w.origin || !reflect.DeepEqual(got, w.payload) {
			t.Errorf("event %d = %s %s %s, want %s %s %v", i, evs[i].Type, evs[i].Origin, evs[i].Payload, w.typ, w.origin, w.payload)
		}
	}
	if !b.events[3].selfAssign || b.events[3].cmd.Pos != 3 || b.events[5].selfAssign {
		t.Fatalf("the topic's self-assignment is marked and carries its create's command")
	}
}

// Through the tool: a created task is written alone and held by its
// creator at todo, item.status idea in the same batch makes it an idea, and
// that holding cannot be unassigned; assigning someone else replaces it.
func TestTrackerUpdateCreatorHoldsAnUnassignedTask(t *testing.T) {
	f := newTrackerLeadFixture(t)
	f.trackerUpdate(t, trackerPersonal)
	out := f.trackerUpdate(t, `<tk:store name="me">
<tk:item.create kind="task" title="mine"/>
<tk:item.create kind="task" title="later"/>
<tk:item.status item="$2" status="idea"/>
</tk:store>`)
	ids := trackerWrote(t, out)
	mine, later := ids["1"], ids["2"]
	if !strings.Contains(out, "me: wrote 3 events") || strings.Contains(out, "self-assignment") {
		t.Fatalf("a created task gets no self-assignment:\n%s", out)
	}
	if got := f.trackerQuery(t, map[string]any{"store": "me", "item": mine}); !strings.Contains(got, "status todo") || !strings.Contains(got, "ada todo (by ada)") {
		t.Fatalf("a created task is its creator's at todo:\n%s", got)
	}
	if got := f.trackerQuery(t, map[string]any{"store": "me", "scope": "all", "status": "idea"}); !strings.Contains(got, later+" [task idea]") || strings.Contains(got, mine) {
		t.Fatalf("the idea item alone shows status idea:\n%s", got)
	}
	text := f.trackerRefused(t, "tracker.update", map[string]any{"commands": `<tk:store name="me"><tk:item.unassign item="` + mine + `" assignee="ada"/></tk:store>`})
	if !strings.Contains(text, "the store would flag this write:") || !strings.Contains(text, "ada has no live or pending assignment on ") {
		t.Fatalf("the creator's holding cannot be unassigned:\n%s", text)
	}

	// Assigning someone else replaces the creator's holding.
	remote := initTrackerRemote(t)
	f.trackerUpdate(t, `<tk:register store="team" member="ada" remote="`+remote+`"/>`)
	bo := trackerOtherMachine(t)
	if _, err := bo.Register(context.Background(), wstrackerhost.RegisterRequest{ID: "team", Remote: remote, Member: "bo", Profile: &wstracker.Profile{Given: "Bo"}}); err != nil {
		t.Fatalf("bo joins: %v", err)
	}
	f.trackerUpdate(t, `<tk:sync store="team"/>`)
	task := trackerWrote(t, f.trackerUpdate(t, `<tk:store name="team">
<tk:item.create kind="task" title="for bo"/>
<tk:item.assign item="$1" people="bo"/>
</tk:store>`))["1"]
	if got := f.trackerQuery(t, map[string]any{"store": "team", "item": task}); !strings.Contains(got, "- bo pending (by ada") || strings.Contains(got, "ada todo (by ada)") {
		t.Fatalf("assigning bo replaces the creator's holding:\n%s", got)
	}
}
