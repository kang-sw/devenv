// Tracker query tests.
package mcp

import (
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wstracker"
	"github.com/kang-sw/devenv/internal/wstrackerhost"
)

// The priority line names where the levels are changed, so an agent asked
// "where do I change the priorities" can answer from the query alone.
func TestTrackerQueryPriorityLineNamesWhereLevelsChange(t *testing.T) {
	f := newTrackerLeadFixture(t)
	f.trackerUpdate(t, trackerPersonal)
	out := f.trackerQuery(t, map[string]any{"store": "me"})
	want := "\npriority: 0 Notice, 1 Task (default), 2 Urgent; an admin changes the levels in the store settings of the dashboard's Team tab, or with tk:roles.set_priority_levels\n"
	if !strings.Contains(out, want) {
		t.Fatalf("priority line must carry the hint %q:\n%s", want, out)
	}
}

// A store with labels prints them after the priority line, with how an item
// takes one; rows and the item detail name an item's labels, and q's
// label: filters by name.
func TestTrackerQueryShowsLabels(t *testing.T) {
	f := newTrackerLeadFixture(t)
	f.trackerUpdate(t, trackerPersonal)
	out := f.trackerUpdate(t, `<tk:store name="me">
<tk:roles.add_label name="docs" color="#3366cc"/>
<tk:roles.add_label name="bug" color="#d14343"/>
<tk:item.create kind="task" title="Write guide" labels="$1 $2"/>
<tk:item.create kind="task" title="Plain"/>
</tk:store>`)
	ids := trackerWrote(t, out)
	docs, bug, guide, plain := trackerAlias(ids["1"]), trackerAlias(ids["2"]), ids["3"], ids["4"]
	q := f.trackerQuery(t, map[string]any{"store": "me", "scope": "all"})
	line := "\nlabels: " + docs + " docs #3366cc, " + bug + " bug #d14343; " + trackerLabelsHint + "\n"
	if !strings.Contains(q, line) {
		t.Fatalf("query must print the labels line %q:\n%s", line, q)
	}
	if !strings.Contains(q, "Write guide (labels docs, bug)") {
		t.Fatalf("the row must name its labels:\n%s", q)
	}
	if one := f.trackerQuery(t, map[string]any{"store": "me", "item": guide}); !strings.Contains(one, ", labels docs, bug\n") {
		t.Fatalf("the item detail must name its labels:\n%s", one)
	}
	got := f.trackerQuery(t, map[string]any{"store": "me", "scope": "all", "q": "label:BUG"})
	if !strings.Contains(got, guide+" [") || strings.Contains(got, plain+" [") {
		t.Fatalf("label:BUG must list only the labelled item:\n%s", got)
	}
}

// payload.q takes the dashboard search grammar and AND-s with kind,
// status, project, and text; text keeps its substring meaning; a malformed
// known key is refused naming the token.
func TestTrackerQuerySearchField(t *testing.T) {
	f := newTrackerLeadFixture(t)
	f.trackerUpdate(t, trackerPersonal)
	out := f.trackerUpdate(t, `<tk:store name="me">
<tk:item.create kind="task" title="Ship release" due="2026-11-20" priority="2" project="web">body</tk:item.create>
<tk:item.assign item="$1" people="ada"/>
<tk:item.status item="$1" status="doing"/>
<tk:comment.add item="$1">needs a changelog</tk:comment.add>
<tk:item.create kind="task" title="Write docs" due="2026-12-01" project="web">a:b in the body</tk:item.create>
<tk:item.create kind="task" title="Gone"/>
<tk:item.hide item="$6"/>
</tk:store>`)
	ids := trackerWrote(t, out)
	ship, docs, gone := ids["1"], ids["5"], ids["6"]
	all := func(payload map[string]any) string {
		payload["store"], payload["scope"] = "me", "all"
		return f.trackerQuery(t, payload)
	}
	for _, c := range []struct {
		payload   map[string]any
		hit, miss []string
	}{
		{map[string]any{"q": "status:doing"}, []string{ship}, []string{docs}},
		{map[string]any{"q": "changelog"}, []string{ship}, []string{docs}},
		{map[string]any{"q": `due:<2026-12-01 "ship release"`}, []string{ship}, []string{docs}},
		{map[string]any{"q": "priority:>=2 assignee:ada"}, []string{ship}, []string{docs}},
		{map[string]any{"q": "project:web", "status": "todo"}, []string{docs}, []string{ship}},
		{map[string]any{"q": "project:web", "text": "release"}, []string{ship}, []string{docs}},
		// text is a plain substring: the colon does not start a filter
		{map[string]any{"text": "a:b"}, []string{docs}, []string{ship}},
		// an unknown key is free text
		{map[string]any{"q": "a:b"}, []string{docs}, []string{ship}},
		// a hidden item shows only for status hidden, in q too
		{map[string]any{"q": "gone"}, nil, []string{gone}},
		{map[string]any{"q": "status:hidden"}, []string{gone}, []string{ship, docs}},
	} {
		got := all(c.payload)
		for _, id := range c.hit {
			if !strings.Contains(got, id+" [") {
				t.Errorf("%v must list %s:\n%s", c.payload, id, got)
			}
		}
		for _, id := range c.miss {
			if strings.Contains(got, id+" [") {
				t.Errorf("%v must not list %s:\n%s", c.payload, id, got)
			}
		}
	}
	if none := all(map[string]any{"q": "status:done"}); !strings.Contains(none, "no rows pass the filters") {
		t.Fatalf("a q that matches nothing must say so: %s", none)
	}
	// the mine scope filters its assignment rows by the item's status
	if mine := f.trackerQuery(t, map[string]any{"store": "me", "q": "status:doing"}); !strings.Contains(mine, ship+" P") {
		t.Fatalf("mine with status:doing must list the doing assignment: %s", mine)
	}
	bad := f.trackerRefused(t, "tracker.query", map[string]any{"payload": map[string]any{"store": "me", "q": "ship due:<tomorrow"}})
	if !strings.Contains(bad, `payload.q: search "due:<tomorrow": due takes a date YYYY-MM-DD`) {
		t.Fatalf("a malformed due token must be refused by name: %s", bad)
	}
	if bad := f.trackerRefused(t, "tracker.query", map[string]any{"payload": map[string]any{"q": "ship"}}); !strings.Contains(bad, "need store") {
		t.Fatalf("q without store must be refused: %s", bad)
	}
}

// The status filter takes in-review and dropped, accepts awaiting as the
// alias of in-review, and open leaves out done and dropped items.
func TestTrackerQueryStatusFilter(t *testing.T) {
	for in, want := range map[string]string{"awaiting": "in-review", "in-review": "in-review", "dropped": "dropped", "open": "open"} {
		q, problems := parseTrackerQuery(map[string]any{"store": "team", "status": in}, nil)
		if len(problems) > 0 || q.status != want {
			t.Errorf("status %s: %q, problems %q", in, q.status, problems)
		}
	}
	if _, problems := parseTrackerQuery(map[string]any{"store": "team", "status": "closed"}, nil); len(problems) != 1 || !strings.Contains(problems[0], "in-review, done, dropped, pending") {
		t.Fatalf("an unknown status is refused with the values: %q", problems)
	}
	str := func(s string) *string { return &s }
	v := &wstrackerhost.StoreView{View: &wstracker.View{Items: map[string]wstracker.ItemView{
		"a": {Status: str("doing")}, "b": {Status: str("in-review")}, "c": {Status: str("done")}, "d": {Status: str("dropped")},
	}}}
	for status, hits := range map[string]string{"open": "ab", "in-review": "b", "dropped": "d", "done": "c"} {
		q, _ := parseTrackerQuery(map[string]any{"store": "team", "status": status}, nil)
		got := ""
		for _, id := range []string{"a", "b", "c", "d"} {
			if trackerItemMatches(v, id, q) {
				got += id
			}
		}
		if got != hits {
			t.Errorf("status %s matches %q, want %q", status, got, hits)
		}
	}
	q, _ := parseTrackerQuery(map[string]any{"store": "team", "q": "status:awaiting"}, nil)
	if !trackerItemMatches(v, "b", q) || trackerItemMatches(v, "a", q) {
		t.Fatalf("q status:awaiting matches the in-review item alone")
	}
}
