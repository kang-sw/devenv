package wsdashboard

import (
	"strings"
	"testing"
)

// TestRailKidBusy pins the working-descendant count beside a parent's
// toggle: a row counts when its lead works or it runs a subagent, once
// even when both, at any depth; an idle row (an approval wait is not work)
// does not.
func TestRailKidBusy(t *testing.T) {
	cases := []struct {
		name string
		rows []RailRow
		want int
	}{
		{"none", nil, 0},
		{"all idle", []RailRow{{ReplyID: "a"}, {ReplyID: "b", Approval: &ApprovalWait{Tool: "Bash"}}}, 0},
		{"working", []RailRow{{ReplyID: "a", Working: true}, {ReplyID: "b"}}, 1},
		{"idle with subagents", []RailRow{{ReplyID: "a", Subagents: 2}}, 1},
		{"working with subagents counts once", []RailRow{{ReplyID: "a", Working: true, Subagents: 3}}, 1},
		{"nested descendants", []RailRow{
			{ReplyID: "a", Children: []RailRow{{ReplyID: "a1", Working: true}, {ReplyID: "a2", Subagents: 1, Children: []RailRow{{ReplyID: "a2x", Working: true}}}}},
			{ReplyID: "b", Working: true},
		}, 4},
	}
	for _, c := range cases {
		if got := railKidBusy(c.rows); got != c.want {
			t.Errorf("%s: railKidBusy = %d, want %d", c.name, got, c.want)
		}
	}
}

// TestRailMarkupKidBusy pins the parent row's "(N)" beside its toggle's
// child count: the layout sets KidBusy from the attached children, the
// render titles it, and a parent with no child at work renders none.
func TestRailMarkupKidBusy(t *testing.T) {
	view := railTreeView()
	a1 := view.Workspaces[1].Worktrees[0].Sessions[0]
	if a1.ReplyID != "a1" || a1.KidBusy != 1 {
		t.Fatalf("a1 = %s KidBusy %d, want a1 with 1 (b1 works, a2 waits on approval)", a1.ReplyID, a1.KidBusy)
	}
	region, err := renderRailRegion(view, "en")
	if err != nil {
		t.Fatal(err)
	}
	html := string(region.HTML)
	want := `<span class="kn">2</span></button><span class="drkwork" title="1 session under this one at work">(1)</span>`
	if !strings.Contains(html, want) || strings.Count(html, `class="drkwork"`) != 1 {
		t.Errorf("the rail lacks a1's single working count %s:\n%s", want, html)
	}

	// The child waiting on approval also runs a subagent: it now counts.
	view = layoutTestView()
	repo := &view.Workspaces[1]
	repo.Worktrees[0].Sessions[1].Approval = &ApprovalWait{Tool: "Bash"}
	repo.Worktrees[0].Sessions[1].Subagents = 1
	repo.Worktrees[1].Sessions[0].Working = true
	repo.Worktrees[1].Sessions[0].Subagents = 2
	f := railFactsOf(view)
	doc := emptyLayout()
	doc.Workspaces["/w/repo"] = &railLayoutWS{Children: map[string][]string{"a1": {"b1", "a2"}}}
	orderRailView(&view, doc, f)
	if region, err = renderRailRegion(view, "en"); err != nil {
		t.Fatal(err)
	}
	if html := string(region.HTML); !strings.Contains(html, `title="2 sessions under this one at work">(2)</span>`) {
		t.Errorf("the rail lacks a1's count of two:\n%s", html)
	}

	// No child at work: no parentheses at all.
	view = layoutTestView()
	f = railFactsOf(view)
	doc = emptyLayout()
	doc.Workspaces["/w/repo"] = &railLayoutWS{Children: map[string][]string{"a1": {"b1", "a2"}}}
	orderRailView(&view, doc, f)
	if region, err = renderRailRegion(view, "en"); err != nil {
		t.Fatal(err)
	}
	if html := string(region.HTML); strings.Contains(html, "drkwork") || !strings.Contains(html, `<span class="kn">2</span></button>`) {
		t.Errorf("an idle parent renders a working count or lost its toggle:\n%s", html)
	}
}
