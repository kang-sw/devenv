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

// TestRailMarkupKidBusy pins the parent row's working count, a subscript
// right after its toggle's child count on the second line: the layout sets
// KidBusy from the attached children, the render titles it, and a parent
// with no child at work renders none.
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
	want := `<span class="kn">2</span></button><sub class="drkwork" title="1 session under this one at work">1</sub>`
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
	if html := string(region.HTML); !strings.Contains(html, `title="2 sessions under this one at work">2</sub>`) {
		t.Errorf("the rail lacks a1's count of two:\n%s", html)
	}

	// No child at work: no working count at all.
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

// TestRailMarkupAvatarBadges pins a row's line 1 and a parent's second
// line: the harness icon in its wrap with the question dot (its count read
// aloud) and the own-subagent count, nothing between the wrap and the name,
// no robot badge or policy capsule; a parent's toggle (chevron, person,
// count) opening its second line, an empty message link after it when it
// has no message; and a child row with no second line, badges or not.
func TestRailMarkupAvatarBadges(t *testing.T) {
	kid := RailRow{ReplyID: "kid", Harness: "pi", Live: true, Child: true, Group: "/w/folder", CountPolicy: 1, OpenAskIDs: "40", Subagents: 1, Message: "child text"}
	par := RailRow{ReplyID: "par", Harness: "claude", Live: true, HasChildren: true, Children: []RailRow{kid}, KidBusy: 1}
	q := RailRow{ReplyID: "q", Harness: "claude", Live: true, CountPolicy: 2, OpenAskIDs: "12 15", Subagents: 3, Message: "last reply"}
	p := RailRow{ReplyID: "p", Harness: "codex", Live: true, Message: "plain"}
	view := RailView{Workspaces: []RailWorkspace{{Name: "folder", Path: "/w/folder", Repo: true, Worktrees: []RailWorktree{{Path: "/w/folder", Sessions: []RailRow{par, q, p}}}}}}
	region, err := renderRailRegion(view, "en")
	if err != nil {
		t.Fatal(err)
	}
	html := string(region.HTML)
	rowOf := func(id string) string {
		_, rest, _ := strings.Cut(html, `data-rid="`+id+`"`)
		row, _, _ := strings.Cut(rest, `<div class="rs`)
		row, _, _ = strings.Cut(row, `<div class="rkids"`)
		return row
	}
	for _, want := range []string{
		`">C</span><span class="qdot" role="img" aria-label="2 items to answer" title="2 items to answer"></span><span class="sbn" title="3 subagents running">3</span></span><span class="rid">q</span>`,
		`<a class="drmsg" href="/s/q" tabindex="-1"><span class="s2">last reply</span></a>`,
	} {
		if !strings.Contains(rowOf("q"), want) {
			t.Errorf("row q lacks %s:\n%s", want, rowOf("q"))
		}
	}
	if row := rowOf("p"); !strings.Contains(row, `<span class="hiw"><span class="hi codex"`) || !strings.Contains(row, `">X</span></span><span class="rid">p</span>`) {
		t.Errorf("a row without badges must keep the bare wrap before its name:\n%s", row)
	}
	if row := rowOf("par"); !strings.Contains(row, `<div class="drmeta"><button type="button" class="kidtog" aria-expanded="true"`) ||
		!strings.Contains(row, `<use href="#i-chev"/></svg><svg class="i" aria-hidden="true"><use href="#i-person"/></svg><span class="kn">1</span></button><sub class="drkwork" title="1 session under this one at work">1</sub><a class="drmsg" href="/s/par" tabindex="-1"></a></div>`) {
		t.Errorf("the parent's second line is not toggle, working count, empty message link:\n%s", row)
	}
	if row := rowOf("kid"); strings.Contains(row, "drmeta") || strings.Contains(row, "drmsg") || !strings.Contains(row, `aria-label="1 item to answer"`) || !strings.Contains(row, `<span class="sbn" title="1 subagents running">1</span>`) {
		t.Errorf("a child row is one line with its badges on the icon:\n%s", row)
	}
	for _, want := range []string{
		`data-rid="q" data-asks="12 15"`,
		`data-rid="kid" data-asks="40"`,
		`data-rid="p" data-asks=""`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("the rail lacks ask-ID attribute %s", want)
		}
	}
	for _, gone := range []string{`class="bots"`, `chip policy`, `#i-robot`} {
		if strings.Contains(html, gone) {
			t.Errorf("the rail still carries %s", gone)
		}
	}
}
