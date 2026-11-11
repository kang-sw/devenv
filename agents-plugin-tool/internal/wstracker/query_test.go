package wstracker

import (
	"errors"
	"slices"
	"testing"
)

// queryView is the fixture the search table runs on:
//
//	ada.pc#1   task "Ship release" project Web, due 2026-11-20, priority 2,
//	           assigned to maya and set doing; lena comments "needs a changelog"
//	maya.pc#2  task "Write docs" project web-docs, due 2026-12-01, priority 0,
//	           pending to raj (maya does not lead raj)
//	ada.pc#3   topic "Roadmap", no due
//	ada.pc#4   task "Old idea", hidden
//	ada.pc#6   task "Ended", assigned to bo then unassigned; ada, holding it
//	           again, sets it idea
func queryView(t *testing.T) *View {
	t.Helper()
	return topicView(t, "ada.pc", []step{
		{"ada.pc", "item.create", map[string]any{"kind": "task", "title": "Ship release", "project": "Web", "due": "2026-11-20", "priority": 2}},
		{"ada.pc", "item.assign", map[string]any{"item": "ada.pc#1", "targets": []any{"maya"}}},
		{"maya.pc", "item.status", map[string]any{"item": "ada.pc#1", "status": "doing"}},
		{"maya.pc", "item.create", map[string]any{"kind": "task", "title": "Write docs", "project": "web-docs", "due": "2026-12-01", "priority": 0, "body": "API reference"}},
		{"maya.pc", "item.assign", map[string]any{"item": "maya.pc#2", "mode": "chosen", "people": []any{"raj"}}},
		{"lena.pc", "comment.add", map[string]any{"item": "ada.pc#1", "body": "needs a changelog"}},
		{"ada.pc", "item.create", map[string]any{"kind": "topic", "title": "Roadmap"}},
		{"ada.pc", "item.create", map[string]any{"kind": "task", "title": "Old idea"}},
		{"ada.pc", "item.hide", map[string]any{"item": "ada.pc#4"}},
		{"ada.pc", "item.create", map[string]any{"kind": "task", "title": "Ended"}},
		{"ada.pc", "item.assign", map[string]any{"item": "ada.pc#6", "targets": []any{"bo"}}},
		{"ada.pc", "item.unassign", map[string]any{"item": "ada.pc#6", "assignee": "bo"}},
		{"ada.pc", "item.status", map[string]any{"item": "ada.pc#6", "status": "idea"}},
	})
}

func TestQuerySearchTable(t *testing.T) {
	v := queryView(t)
	const ship, docs, road, old, ended = "ada.pc#1", "maya.pc#2", "ada.pc#3", "ada.pc#4", "ada.pc#6"
	cases := []struct {
		q    string
		want []string
	}{
		{"", []string{ship, docs, road, old, ended}},
		{"status:doing", []string{ship}},
		{"STATUS:Doing", []string{ship}},
		{"status:pending", []string{docs}},
		{"status:open", []string{ship, docs, road, ended}},
		{"status:hidden", []string{old}},
		{"status:idea", []string{ended}},
		{"assignee:maya", []string{ship}},
		{"assignee:Ma", []string{ship}},
		{"assignee:raj", []string{docs}}, // pending counts
		{"assignee:bo", nil},             // ended does not
		{"kind:topic", []string{road}},
		{"kind:task status:doing", []string{ship}},
		{"project:web", []string{ship}},
		{"project:WEB-docs", []string{docs}},
		{"due:2026-11-20", []string{ship}},
		{"due:<2026-11-21", []string{ship}},
		{"due:<=2026-12-01", []string{ship, docs}},
		{"due:>2026-11-20", []string{docs}},
		{"due:>=2026-01-01", []string{ship, docs}}, // no due never matches
		{"priority:2", []string{ship}},
		{"priority:>=1", []string{ship, road, old, ended}},
		{"priority:<1", []string{docs}},
		{"ship", []string{ship}},
		{"changelog", []string{ship}}, // comment text
		{`"needs a"`, []string{ship}},
		{`"needs the"`, nil},
		{"api reference", []string{docs}},
		{"maya.pc-2", []string{docs}},
		{"https://example.com/x", nil}, // unknown key: free text
		{"foo:bar", nil},
		{"ship status:todo", nil},
		{`project:"web-docs" docs`, []string{docs}},
	}
	for _, c := range cases {
		q, err := ParseQuery(c.q)
		if err != nil {
			t.Fatalf("ParseQuery(%q): %v", c.q, err)
		}
		got := v.Search(q)
		if !slices.Equal(got, c.want) && !(len(got) == 0 && len(c.want) == 0) {
			t.Errorf("search %q = %v, want %v", c.q, got, c.want)
		}
	}
}

func TestQueryFreeTextColonToken(t *testing.T) {
	v := queryView(t)
	q, err := ParseQuery("note:")
	if err != nil {
		t.Fatal(err)
	}
	if len(v.Search(q)) != 0 {
		t.Fatalf("note: matched %v", v.Search(q))
	}
}

func TestQueryMalformedKnownKeyRefused(t *testing.T) {
	for tok, reason := range map[string]string{
		"due:<tomorrow":  "due takes a date YYYY-MM-DD, optionally after <, >, <=, or >=",
		"due:2026-13-01": "due takes a date YYYY-MM-DD, optionally after <, >, <=, or >=",
		"priority:high":  "priority takes a level number, optionally after <, >, <=, or >=",
		"priority:>=-1":  "priority takes a level number, optionally after <, >, <=, or >=",
		"status:review2": "status takes one of open, hidden, idea, todo, doing, blocked, awaiting, done, pending",
		"kind:epic":      "kind takes one of task, topic",
		"assignee:":      "assignee takes a value",
		"project:":       "project takes a value",
		"is:open":        "is takes one of claimable",
	} {
		_, err := ParseQuery("ship " + tok)
		var qe *QueryError
		if !errors.As(err, &qe) {
			t.Fatalf("ParseQuery(%q) = %v, want a QueryError", tok, err)
		}
		if qe.Token != tok || qe.Reason != reason {
			t.Errorf("ParseQuery(%q) = %+v, want token %q reason %q", tok, qe, tok, reason)
		}
	}
}

func TestQueryWantsHidden(t *testing.T) {
	for s, want := range map[string]bool{"status:hidden": true, "x STATUS:HIDDEN": true, "status:open": false, "hidden": false, "": false} {
		q, err := ParseQuery(s)
		if err != nil {
			t.Fatal(err)
		}
		if q.WantsHidden() != want {
			t.Errorf("WantsHidden(%q) = %v, want %v", s, !want, want)
		}
	}
}

// is:claimable follows ItemView.Claimable for the viewer: maya opens a task
// to her team; omar, inside it, matches; raj, outside it, and maya, its
// fallback holder, do not; nor does an item nobody opened.
func TestQueryIsClaimable(t *testing.T) {
	steps := []step{
		task("maya.pc", "Free"),
		{"maya.pc", "item.open", map[string]any{"item": "maya.pc#1", "scope": "team"}},
		task("maya.pc", "Closed"),
	}
	q, err := ParseQuery("IS:Claimable")
	if err != nil {
		t.Fatal(err)
	}
	for viewer, want := range map[string][]string{
		"omar.pc": {"maya.pc#1"},
		"raj.pc":  nil,
		"maya.pc": nil,
	} {
		if got := topicView(t, viewer, steps).Search(q); !slices.Equal(got, want) && !(len(got) == 0 && len(want) == 0) {
			t.Errorf("is:claimable for %s = %v, want %v", viewer, got, want)
		}
	}
}
