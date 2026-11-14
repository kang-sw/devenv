package wsdashboard

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// layoutTestView is a rail as buildRailView lists it, unordered by any
// layout: a plain folder, a repository with a root and a linked worktree,
// and a second plain folder. Each group lists its live rows, then its ended
// ones.
func layoutTestView() RailView {
	live := func(ids ...string) []RailRow {
		var rows []RailRow
		for _, id := range ids {
			rows = append(rows, RailRow{ReplyID: id, Live: true})
		}
		return rows
	}
	ended := func(ids ...string) []RailRow {
		var rows []RailRow
		for _, id := range ids {
			rows = append(rows, RailRow{ReplyID: id})
		}
		return rows
	}
	return RailView{Workspaces: []RailWorkspace{
		{Path: "/w/plain", Sessions: live("p1", "p2"), Ended: ended("pe", "pf")},
		{Path: "/w/repo", Repo: true, Worktrees: []RailWorktree{
			{Path: "/w/repo", Root: true, Sessions: live("a1", "a2", "a3", "a4"), Ended: ended("ae")},
			{Path: "/w/feat", Sessions: live("b1", "b2"), Ended: ended("be")},
		}},
		{Path: "/w/zed", Sessions: live("z1")},
	}}
}

func layoutTestFacts() railFacts { return railFactsOf(layoutTestView()) }

func emptyLayout() *railLayoutDoc { return &railLayoutDoc{Workspaces: map[string]*railLayoutWS{}} }

// layoutRendered is the test view ordered by doc: each group's live ids
// in reading order, a parent's children right after it (by the group's
// path), the plain folder's ended ids ("ended"), and the workspaces
// ("ws"), each joined by spaces; starred holds the ids whose star counts.
func layoutRendered(doc *railLayoutDoc) (order map[string]string, starred []string) {
	view := layoutTestView()
	orderRailView(&view, doc, layoutTestFacts())
	order = map[string]string{}
	var ws []string
	add := func(key string, rows []RailRow) {
		var flat []RailRow
		for _, r := range rows {
			flat = append(append(flat, r), r.Children...)
		}
		order[key] = strings.Join(railIDs(flat), " ")
		for _, r := range flat {
			if r.Starred {
				starred = append(starred, r.ReplyID)
			}
		}
	}
	for _, w := range view.Workspaces {
		ws = append(ws, w.Path)
		if !w.Repo {
			add(w.Path, w.Sessions)
			add(w.Path+" ended", w.Ended)
			continue
		}
		for _, wt := range w.Worktrees {
			add(wt.Path, wt.Sessions)
			add(wt.Path+" ended", wt.Ended)
		}
	}
	order["ws"] = strings.Join(ws, " ")
	return order, starred
}

// mustApply applies req to doc against the test view (loaded) and fails
// on any reason.
func mustApply(t *testing.T, doc *railLayoutDoc, req railLayoutRequest) {
	t.Helper()
	if reason := applyRailLayoutOp(doc, req, layoutTestFacts(), true); reason != "" {
		t.Fatalf("%+v refused: %s", req, reason)
	}
}

func wantRendered(t *testing.T, doc *railLayoutDoc, key, want string) {
	t.Helper()
	if got, _ := layoutRendered(doc); got[key] != want {
		t.Errorf("%s renders %q, want %q", key, got[key], want)
	}
}

// TestRailLayoutRefusals pins every op's validation: each refused request
// names its reason, and the layout it was tried on stays as it was.
func TestRailLayoutRefusals(t *testing.T) {
	// a1 is starred; a3 is a1's child, ae (ended) a2's.
	base := func() *railLayoutDoc {
		doc := emptyLayout()
		doc.Workspaces["/w/repo"] = &railLayoutWS{Stars: []string{"a1"}, Children: map[string][]string{"a1": {"a3"}, "a2": {"ae"}}}
		return doc
	}
	for _, c := range []struct {
		name string
		req  railLayoutRequest
		want string
	}{
		{"unknown op", railLayoutRequest{Op: "shuffle", RID: "a1"}, layoutInvalid},
		{"both places", railLayoutRequest{Op: "move", RID: "a2", Before: "a4", After: "a4"}, layoutInvalid},
		{"move without a place", railLayoutRequest{Op: "move", RID: "a2"}, layoutInvalid},
		{"move unknown", railLayoutRequest{Op: "move", RID: "nope", Before: "a2"}, layoutUnknown},
		{"move ended", railLayoutRequest{Op: "move", RID: "ae", Before: "a2"}, layoutEnded},
		{"move onto itself", railLayoutRequest{Op: "move", RID: "a2", Before: "a2"}, layoutSelf},
		{"move before unknown", railLayoutRequest{Op: "move", RID: "a2", Before: "nope"}, layoutUnknown},
		{"move before ended", railLayoutRequest{Op: "move", RID: "a2", Before: "ae"}, layoutEnded},
		{"move to another workspace", railLayoutRequest{Op: "move", RID: "a2", Before: "p1"}, layoutWorkspace},
		{"move to another group", railLayoutRequest{Op: "move", RID: "a2", Before: "b1"}, layoutGroup},
		{"move across the stars", railLayoutRequest{Op: "move", RID: "a2", Before: "a1"}, layoutStars},
		{"move before a child", railLayoutRequest{Op: "move", RID: "a2", Before: "a3"}, layoutLevel},
		{"move a child to the top level", railLayoutRequest{Op: "move", RID: "a3", Before: "a4"}, layoutLevel},
		{"attach without a parent", railLayoutRequest{Op: "attach", RID: "a4"}, layoutInvalid},
		{"attach unknown", railLayoutRequest{Op: "attach", RID: "nope", Parent: "a1"}, layoutUnknown},
		{"attach to itself", railLayoutRequest{Op: "attach", RID: "a4", Parent: "a4"}, layoutSelf},
		{"attach to unknown", railLayoutRequest{Op: "attach", RID: "a4", Parent: "nope"}, layoutUnknown},
		{"attach to ended", railLayoutRequest{Op: "attach", RID: "a4", Parent: "ae"}, layoutEnded},
		{"attach to another workspace", railLayoutRequest{Op: "attach", RID: "p1", Parent: "a4"}, layoutWorkspace},
		{"attach to a child", railLayoutRequest{Op: "attach", RID: "a4", Parent: "a3"}, layoutDepth},
		{"attach a parent", railLayoutRequest{Op: "attach", RID: "a1", Parent: "a4"}, layoutDepth},
		{"attach a parent of an ended child", railLayoutRequest{Op: "attach", RID: "a2", Parent: "a4"}, layoutDepth},
		{"attach before itself", railLayoutRequest{Op: "attach", RID: "a4", Parent: "a1", Before: "a4"}, layoutSelf},
		{"attach before a non-child", railLayoutRequest{Op: "attach", RID: "a4", Parent: "a1", Before: "b1"}, layoutLevel},
		{"attach before an ended child", railLayoutRequest{Op: "attach", RID: "a4", Parent: "a2", Before: "ae"}, layoutEnded},
		{"detach a top-level row", railLayoutRequest{Op: "detach", RID: "a4"}, layoutTopLevel},
		{"detach unknown", railLayoutRequest{Op: "detach", RID: "nope"}, layoutUnknown},
		{"detach before another group", railLayoutRequest{Op: "detach", RID: "a3", Before: "b1"}, layoutGroup},
		{"detach before another workspace", railLayoutRequest{Op: "detach", RID: "a3", Before: "p1"}, layoutWorkspace},
		{"detach before ended", railLayoutRequest{Op: "detach", RID: "a3", Before: "ae"}, layoutEnded},
		{"detach before itself", railLayoutRequest{Op: "detach", RID: "a3", After: "a3"}, layoutSelf},
		{"star unknown", railLayoutRequest{Op: "star", RID: "nope"}, layoutUnknown},
		{"unstar unknown", railLayoutRequest{Op: "unstar", RID: "nope"}, layoutUnknown},
		{"star a child", railLayoutRequest{Op: "star", RID: "a3"}, layoutChild},
		{"star with a place", railLayoutRequest{Op: "star", RID: "a4", Before: "a1"}, layoutInvalid},
		{"wsmove unknown", railLayoutRequest{Op: "wsmove", Path: "/w/gone", Before: "/w/repo"}, layoutUnknown},
		{"wsmove before unknown", railLayoutRequest{Op: "wsmove", Path: "/w/zed", Before: "/w/gone"}, layoutUnknown},
		{"wsmove onto itself", railLayoutRequest{Op: "wsmove", Path: "/w/zed", After: "/w/zed"}, layoutSelf},
		{"wsmove without a place", railLayoutRequest{Op: "wsmove", Path: "/w/zed"}, layoutInvalid},
	} {
		doc := base()
		before, _ := doc.encode()
		if got := applyRailLayoutOp(doc.clone(), c.req, layoutTestFacts(), true); got != c.want {
			t.Errorf("%s: reason %q, want %q", c.name, got, c.want)
		}
		if after, _ := doc.encode(); !bytes.Equal(before, after) {
			t.Errorf("%s: the refusal changed the layout", c.name)
		}
	}
	// A child moves among its siblings only.
	doc := base()
	doc.Workspaces["/w/repo"].Children["a1"] = []string{"a3", "a4"}
	if got := applyRailLayoutOp(doc, railLayoutRequest{Op: "move", RID: "a3", Before: "a2"}, layoutTestFacts(), true); got != layoutLevel {
		t.Errorf("a child moved before a top-level row: %q, want level", got)
	}
	doc.Workspaces["/w/repo"].Children["a2"] = []string{"b1"}
	if got := applyRailLayoutOp(doc, railLayoutRequest{Op: "move", RID: "a3", Before: "b1"}, layoutTestFacts(), true); got != layoutLevel {
		t.Errorf("a child moved among another parent's children: %q, want level", got)
	}
}

// TestRailLayoutMove pins a reorder: within its group's top-level rows on
// its side of the stars, kept as the group's whole order (a row the layout
// did not hold, new since, goes last), never touching another group's.
func TestRailLayoutMove(t *testing.T) {
	doc := emptyLayout()
	mustApply(t, doc, railLayoutRequest{Op: "move", RID: "a3", Before: "a1"})
	wantRendered(t, doc, "/w/repo", "a3 a1 a2 a4")
	mustApply(t, doc, railLayoutRequest{Op: "move", RID: "a1", After: "a4"})
	wantRendered(t, doc, "/w/repo", "a3 a2 a4 a1")
	mustApply(t, doc, railLayoutRequest{Op: "move", RID: "b2", Before: "b1"})
	wantRendered(t, doc, "/w/feat", "b2 b1")
	wantRendered(t, doc, "/w/repo", "a3 a2 a4 a1")

	// The starred rows come first, each side in its own order.
	mustApply(t, doc, railLayoutRequest{Op: "star", RID: "a2"})
	wantRendered(t, doc, "/w/repo", "a2 a3 a4 a1")
	mustApply(t, doc, railLayoutRequest{Op: "move", RID: "a4", Before: "a3"})
	wantRendered(t, doc, "/w/repo", "a2 a4 a3 a1")

	// A child moves among its siblings.
	mustApply(t, doc, railLayoutRequest{Op: "attach", RID: "a3", Parent: "a1"})
	mustApply(t, doc, railLayoutRequest{Op: "attach", RID: "a4", Parent: "a1"})
	mustApply(t, doc, railLayoutRequest{Op: "move", RID: "a4", Before: "a3"})
	if got := doc.Workspaces["/w/repo"].Children["a1"]; !slices.Equal(got, []string{"a4", "a3"}) {
		t.Errorf("a1's children = %v, want [a4 a3]", got)
	}
	wantRendered(t, doc, "/w/repo", "a2 a1 a4 a3")
}

// TestRailLayoutAttach pins an attach: the child leaves the top-level order
// and joins the parent's children, last or at its place, from any group of
// the workspace; an ended session may become a child.
func TestRailLayoutAttach(t *testing.T) {
	doc := emptyLayout()
	mustApply(t, doc, railLayoutRequest{Op: "move", RID: "a2", Before: "a1"})
	mustApply(t, doc, railLayoutRequest{Op: "attach", RID: "a2", Parent: "a1"})
	w := doc.Workspaces["/w/repo"]
	if slices.Contains(w.Order, "a2") || !slices.Equal(w.Children["a1"], []string{"a2"}) {
		t.Fatalf("after the attach: order %v, children %v", w.Order, w.Children)
	}
	wantRendered(t, doc, "/w/repo", "a1 a2 a3 a4")
	mustApply(t, doc, railLayoutRequest{Op: "attach", RID: "a3", Parent: "a1", Before: "a2"})
	wantRendered(t, doc, "/w/repo", "a1 a3 a2 a4")
	mustApply(t, doc, railLayoutRequest{Op: "attach", RID: "b1", Parent: "a1"})
	mustApply(t, doc, railLayoutRequest{Op: "attach", RID: "ae", Parent: "a1"})
	if got := w.Children["a1"]; !slices.Equal(got, []string{"a3", "a2", "b1", "ae"}) {
		t.Errorf("a1's children = %v, want [a3 a2 b1 ae]", got)
	}
	// A child attached again moves to its new parent.
	mustApply(t, doc, railLayoutRequest{Op: "attach", RID: "a2", Parent: "a4"})
	if !slices.Equal(w.Children["a1"], []string{"a3", "b1", "ae"}) || !slices.Equal(w.Children["a4"], []string{"a2"}) {
		t.Errorf("after the re-attach: children %v", w.Children)
	}
}

// TestRailLayoutDetach pins a detach: next to its old parent when both are
// in one group, else at the head of its own group, or at the place given.
func TestRailLayoutDetach(t *testing.T) {
	doc := emptyLayout()
	doc.Workspaces["/w/repo"] = &railLayoutWS{Order: []string{"a2", "a1", "a4"}, Children: map[string][]string{"a1": {"a3"}}}
	mustApply(t, doc, railLayoutRequest{Op: "detach", RID: "a3"})
	if len(doc.Workspaces["/w/repo"].Children) != 0 {
		t.Errorf("the link stayed: %v", doc.Workspaces["/w/repo"].Children)
	}
	wantRendered(t, doc, "/w/repo", "a2 a1 a3 a4")

	// In another group than its parent: the head of its own group.
	doc = emptyLayout()
	doc.Workspaces["/w/repo"] = &railLayoutWS{Order: []string{"b1"}, Children: map[string][]string{"a1": {"b2"}}}
	mustApply(t, doc, railLayoutRequest{Op: "detach", RID: "b2"})
	wantRendered(t, doc, "/w/feat", "b2 b1")

	// At a place given.
	doc = emptyLayout()
	doc.Workspaces["/w/repo"] = &railLayoutWS{Children: map[string][]string{"a1": {"a3"}}}
	mustApply(t, doc, railLayoutRequest{Op: "detach", RID: "a3", After: "a4"})
	wantRendered(t, doc, "/w/repo", "a1 a2 a4 a3")

	// The place is a top-level row.
	doc = emptyLayout()
	doc.Workspaces["/w/repo"] = &railLayoutWS{Children: map[string][]string{"a1": {"a3", "a4"}}}
	if got := applyRailLayoutOp(doc, railLayoutRequest{Op: "detach", RID: "a3", Before: "a4"}, layoutTestFacts(), true); got != layoutLevel {
		t.Errorf("detach before a child: %q, want level", got)
	}
}

// TestRailLayoutStar pins star and unstar: idempotent, refused on a child,
// and a child's stored star inert until it is top-level again.
func TestRailLayoutStar(t *testing.T) {
	doc := emptyLayout()
	mustApply(t, doc, railLayoutRequest{Op: "star", RID: "a2"})
	mustApply(t, doc, railLayoutRequest{Op: "star", RID: "a2"})
	if got := doc.Workspaces["/w/repo"].Stars; !slices.Equal(got, []string{"a2"}) {
		t.Errorf("stars after two stars = %v, want [a2]", got)
	}
	mustApply(t, doc, railLayoutRequest{Op: "unstar", RID: "a2"})
	mustApply(t, doc, railLayoutRequest{Op: "unstar", RID: "a2"})
	if got := doc.Workspaces["/w/repo"].Stars; len(got) != 0 {
		t.Errorf("stars after two unstars = %v, want none", got)
	}
	// An ended session takes a star.
	mustApply(t, doc, railLayoutRequest{Op: "star", RID: "pf"})
	wantRendered(t, doc, "/w/plain ended", "pf pe")

	doc = emptyLayout()
	doc.Workspaces["/w/repo"] = &railLayoutWS{Stars: []string{"a3"}, Children: map[string][]string{"a1": {"a3"}}}
	if _, starred := layoutRendered(doc); slices.Contains(starred, "a3") {
		t.Error("a child's star counts")
	}
	mustApply(t, doc, railLayoutRequest{Op: "detach", RID: "a3"})
	if _, starred := layoutRendered(doc); !slices.Contains(starred, "a3") {
		t.Error("a detached child's star does not count again")
	}
	wantRendered(t, doc, "/w/repo", "a3 a1 a2 a4")
}

// TestRailLayoutWorkspaceMove pins a workspace reorder: among the listed
// workspaces, an unlisted one ranked last in the rail's order, and a path
// the rail does not list left where it is.
func TestRailLayoutWorkspaceMove(t *testing.T) {
	doc := emptyLayout()
	mustApply(t, doc, railLayoutRequest{Op: "wsmove", Path: "/w/zed", Before: "/w/plain"})
	wantRendered(t, doc, "ws", "/w/zed /w/plain /w/repo")
	mustApply(t, doc, railLayoutRequest{Op: "wsmove", Path: "/w/zed", After: "/w/repo"})
	wantRendered(t, doc, "ws", "/w/plain /w/repo /w/zed")

	doc = emptyLayout()
	doc.WorkspaceOrder = []string{"/w/gone", "/w/repo"}
	wantRendered(t, doc, "ws", "/w/repo /w/plain /w/zed")
	if reason := applyRailLayoutOp(doc, railLayoutRequest{Op: "wsmove", Path: "/w/zed", Before: "/w/repo"}, layoutTestFacts(), true); reason != "" {
		t.Fatal(reason)
	}
	if want := []string{"/w/gone", "/w/zed", "/w/repo", "/w/plain"}; !slices.Equal(doc.WorkspaceOrder, want) {
		t.Errorf("workspace order = %v, want %v", doc.WorkspaceOrder, want)
	}
}

// TestRailLayoutImport pins the import: taken only into an empty layout,
// each id to its workspace, unknown and repeated entries dropped, and
// refused as retryable before the index has read the registry.
func TestRailLayoutImport(t *testing.T) {
	req := railLayoutRequest{Op: "import",
		Order:   []string{"a2", "a1", "nope", "a2", "p1", "b2"},
		Stars:   []string{"b1", "nope", "b1"},
		WSOrder: []string{"/w/repo", "/w/gone", "/w/repo"}}
	f := layoutTestFacts()
	if got := applyRailLayoutOp(emptyLayout(), req, f, false); got != layoutRetry {
		t.Errorf("an import before the index loads: %q, want retry", got)
	}
	doc := emptyLayout()
	if got := applyRailLayoutOp(doc, req, f, true); got != "" {
		t.Fatalf("import into an empty layout: %q", got)
	}
	repo := doc.Workspaces["/w/repo"]
	if !slices.Equal(repo.Order, []string{"a2", "a1", "b2"}) || !slices.Equal(repo.Stars, []string{"b1"}) || len(repo.Children) != 0 {
		t.Errorf("the repository's layout = %+v", repo)
	}
	if got := doc.Workspaces["/w/plain"].Order; !slices.Equal(got, []string{"p1"}) {
		t.Errorf("the plain folder's order = %v, want [p1]", got)
	}
	if !slices.Equal(doc.WorkspaceOrder, []string{"/w/repo"}) {
		t.Errorf("workspace order = %v, want [/w/repo]", doc.WorkspaceOrder)
	}
	wantRendered(t, doc, "/w/repo", "a2 a1 a3 a4")
	wantRendered(t, doc, "/w/feat", "b1 b2")
	wantRendered(t, doc, "ws", "/w/repo /w/plain /w/zed")

	// Any entry makes a layout non-empty, even one the rail does not list.
	for _, held := range []*railLayoutDoc{
		{WorkspaceOrder: []string{"/w/gone"}},
		{Workspaces: map[string]*railLayoutWS{"/w/gone": {Order: []string{"x"}}}},
		{Workspaces: map[string]*railLayoutWS{"/w/repo": {Stars: []string{"x"}}}},
		{Workspaces: map[string]*railLayoutWS{"/w/repo": {Children: map[string][]string{"x": {"y"}}}}},
	} {
		before, _ := held.encode()
		if got := applyRailLayoutOp(held, req, f, true); got != layoutNotApplied {
			t.Errorf("import into %s: %q, want not_applied", before, got)
		}
		if after, _ := held.encode(); !bytes.Equal(before, after) {
			t.Errorf("an import not applied changed %s to %s", before, after)
		}
	}
	if !railLayoutEmpty(&railLayoutDoc{Workspaces: map[string]*railLayoutWS{"/w/repo": {}}}) {
		t.Error("a workspace entry without an entry reads as non-empty")
	}
}

// TestRailLayoutOrphans pins the orphan rule: a live child of an ended
// parent shows in the parent's slot; an op on it, or an attach under it,
// drops the dormant link first.
func TestRailLayoutOrphans(t *testing.T) {
	orphaned := func() *railLayoutDoc {
		doc := emptyLayout()
		doc.Workspaces["/w/repo"] = &railLayoutWS{Order: []string{"a1", "ae", "a2"}, Children: map[string][]string{"ae": {"a3"}}}
		return doc
	}
	wantRendered(t, orphaned(), "/w/repo", "a1 a3 a2 a4")
	dropped := func(t *testing.T, doc *railLayoutDoc) {
		t.Helper()
		if _, ok := doc.Workspaces["/w/repo"].Children["ae"]; ok {
			t.Errorf("the dormant link stayed: %v", doc.Workspaces["/w/repo"].Children)
		}
	}

	doc := orphaned()
	mustApply(t, doc, railLayoutRequest{Op: "move", RID: "a3", Before: "a1"})
	dropped(t, doc)
	wantRendered(t, doc, "/w/repo", "a3 a1 a2 a4")

	doc = orphaned()
	mustApply(t, doc, railLayoutRequest{Op: "move", RID: "a2", Before: "a3"})
	if got := doc.Workspaces["/w/repo"].Children["ae"]; !slices.Equal(got, []string{"a3"}) {
		t.Errorf("a move beside an orphan dropped its link: %v", doc.Workspaces["/w/repo"].Children)
	}
	wantRendered(t, doc, "/w/repo", "a1 a2 a3 a4")

	doc = orphaned()
	mustApply(t, doc, railLayoutRequest{Op: "attach", RID: "a2", Parent: "a3"})
	dropped(t, doc)
	if got := doc.Workspaces["/w/repo"].Children["a3"]; !slices.Equal(got, []string{"a2"}) {
		t.Errorf("a3's children = %v, want [a2]", got)
	}
	wantRendered(t, doc, "/w/repo", "a1 a3 a2 a4")

	doc = orphaned()
	mustApply(t, doc, railLayoutRequest{Op: "star", RID: "a3"})
	dropped(t, doc)
	wantRendered(t, doc, "/w/repo", "a3 a1 a2 a4")

	doc = orphaned()
	mustApply(t, doc, railLayoutRequest{Op: "detach", RID: "a3"})
	dropped(t, doc)
	wantRendered(t, doc, "/w/repo", "a1 a3 a2 a4")

	// A placement drops a nested child's entry, so it cannot outrank its
	// parent's slot should the parent end.
	doc = emptyLayout()
	doc.Workspaces["/w/repo"] = &railLayoutWS{Order: []string{"a1", "a3", "a2"}, Children: map[string][]string{"a1": {"a3"}}}
	mustApply(t, doc, railLayoutRequest{Op: "move", RID: "a2", Before: "a1"})
	if got := doc.Workspaces["/w/repo"].Order; slices.Contains(got, "a3") {
		t.Errorf("the nested child kept its entry: %v", got)
	}

	// A row placed after or before an orphan lands on that side of it, and
	// the orphan keeps its link.
	doc = orphaned()
	mustApply(t, doc, railLayoutRequest{Op: "move", RID: "a4", After: "a3"})
	wantRendered(t, doc, "/w/repo", "a1 a3 a4 a2")
	if got := doc.Workspaces["/w/repo"].Children["ae"]; !slices.Equal(got, []string{"a3"}) {
		t.Errorf("a move after an orphan dropped its link: %v", doc.Workspaces["/w/repo"].Children)
	}
	mustApply(t, doc, railLayoutRequest{Op: "move", RID: "a2", Before: "a3"})
	wantRendered(t, doc, "/w/repo", "a1 a2 a3 a4")

	// An orphan in another group than its ended parent shows at the head
	// of its own group, and a move on it keeps that place as the order's.
	doc = emptyLayout()
	doc.Workspaces["/w/repo"] = &railLayoutWS{Order: []string{"a1", "a2"}, Children: map[string][]string{"be": {"a3"}}}
	wantRendered(t, doc, "/w/repo", "a3 a1 a2 a4")
	mustApply(t, doc, railLayoutRequest{Op: "move", RID: "a1", Before: "a4"})
	wantRendered(t, doc, "/w/repo", "a3 a2 a1 a4")
	mustApply(t, doc, railLayoutRequest{Op: "move", RID: "a2", Before: "a3"})
	wantRendered(t, doc, "/w/repo", "a2 a3 a1 a4")
	if got := doc.Workspaces["/w/repo"].Children["be"]; !slices.Equal(got, []string{"a3"}) {
		t.Errorf("a move before the other group's orphan dropped its link: %v", doc.Workspaces["/w/repo"].Children)
	}
	mustApply(t, doc, railLayoutRequest{Op: "move", RID: "a3", After: "a4"})
	dropped(t, doc)
	if _, ok := doc.Workspaces["/w/repo"].Children["be"]; ok {
		t.Error("the other group's orphan kept its link")
	}
	wantRendered(t, doc, "/w/repo", "a2 a1 a4 a3")
}

// TestRailViewOrder pins the server's order: starred units first, then the
// layout's order, unlisted ordinary live rows last in the rail's own order;
// a child right after its parent; the ended rows starred first; the
// workspaces by the workspace order, the rest after.
func TestRailViewOrder(t *testing.T) {
	order, starred := layoutRendered(emptyLayout())
	for key, want := range map[string]string{
		"/w/plain": "p1 p2", "/w/plain ended": "pe pf", "/w/repo": "a1 a2 a3 a4", "/w/feat": "b1 b2",
		"ws": "/w/plain /w/repo /w/zed",
	} {
		if order[key] != want {
			t.Errorf("without a layout %s renders %q, want %q", key, order[key], want)
		}
	}
	if len(starred) != 0 {
		t.Errorf("without a layout %v are starred", starred)
	}
	doc := emptyLayout()
	doc.WorkspaceOrder = []string{"/w/repo"}
	doc.Workspaces["/w/repo"] = &railLayoutWS{Order: []string{"a3", "a1", "b2"}, Stars: []string{"a4"}, Children: map[string][]string{"a1": {"a2"}}}
	doc.Workspaces["/w/plain"] = &railLayoutWS{Stars: []string{"pf"}}
	order, starred = layoutRendered(doc)
	for key, want := range map[string]string{
		"/w/repo": "a4 a3 a1 a2", "/w/feat": "b2 b1", "/w/plain ended": "pf pe", "ws": "/w/repo /w/plain /w/zed",
	} {
		if order[key] != want {
			t.Errorf("%s renders %q, want %q", key, order[key], want)
		}
	}
	slices.Sort(starred)
	if !slices.Equal(starred, []string{"a4", "pf"}) {
		t.Errorf("starred = %v, want [a4 pf]", starred)
	}
	view := layoutTestView()
	orderRailView(&view, doc, layoutTestFacts())
	if view.LayoutEmpty {
		t.Error("a layout with entries reads as empty")
	}
	view = layoutTestView()
	orderRailView(&view, emptyLayout(), layoutTestFacts())
	if !view.LayoutEmpty {
		t.Error("an empty layout reads as non-empty")
	}
}

// railTreeView is the test view on a tree: a1 holds b1 (of the linked
// worktree, on feat/x) and a2 (waiting on an approval, its message long,
// its stored star inert), b1 works, and a3 holds the ended ae.
func railTreeView() RailView {
	view := layoutTestView()
	repo := &view.Workspaces[1]
	repo.Worktrees[0].Branch = "main"
	repo.Worktrees[1].Branch, repo.Worktrees[1].Folder = "feat/x", "feat"
	repo.Worktrees[0].Sessions[1].Approval = &ApprovalWait{Tool: "Bash", Args: "git push"}
	repo.Worktrees[0].Sessions[1].Message = "the whole last message"
	repo.Worktrees[1].Sessions[0].Working = true
	f := railFactsOf(view)
	doc := emptyLayout()
	doc.Workspaces["/w/repo"] = &railLayoutWS{Stars: []string{"a2", "ae"}, Children: map[string][]string{"a1": {"b1", "a2"}, "a3": {"ae"}}}
	orderRailView(&view, doc, f)
	return view
}

// TestRailViewTree pins the tree the view renders: a live child of a live
// top-level parent leaves its own group's list for its parent's Children,
// whatever group it is in, in the children order, starless, with a branch
// tag when its worktree is not its parent's; the parent rolls its
// children's approval waits and work up; an ended child stays in its
// group's ended fold, its star hidden.
func TestRailViewTree(t *testing.T) {
	repo := railTreeView().Workspaces[1]
	root, feat := repo.Worktrees[0], repo.Worktrees[1]
	if got := strings.Join(railIDs(root.Sessions), " "); got != "a1 a3 a4" {
		t.Errorf("root lists %q, want a1 a3 a4", got)
	}
	if got := strings.Join(railIDs(feat.Sessions), " "); got != "b2" {
		t.Errorf("feat lists %q, want b2", got)
	}
	a1 := root.Sessions[0]
	if got := strings.Join(railIDs(a1.Children), " "); got != "b1 a2" {
		t.Fatalf("a1's children = %q, want b1 a2", got)
	}
	b1, a2 := a1.Children[0], a1.Children[1]
	if !b1.Child || b1.BranchTag != "feat/x" || b1.Group != "/w/feat" || !b1.StarHidden {
		t.Errorf("b1 = %+v, want a starless child tagged feat/x", b1)
	}
	if !a2.Child || a2.BranchTag != "" || a2.Starred || !a2.StarHidden {
		t.Errorf("a2 = %+v, want a starless untagged child", a2)
	}
	if !a1.HasChildren || a1.KidApprovals != 1 || !a1.KidWorking || a1.Child {
		t.Errorf("a1 = %+v, want a parent with one approval and work rolled up", a1)
	}
	a3 := root.Sessions[1]
	if !a3.HasChildren || len(a3.Children) != 0 || a3.KidApprovals != 0 || a3.KidWorking {
		t.Errorf("a3 = %+v, want a stored parent with no live child", a3)
	}
	if ae := root.Ended[0]; ae.ReplyID != "ae" || !ae.StarHidden || ae.Starred || ae.Child {
		t.Errorf("ae = %+v, want an ended child in the fold, star hidden", ae)
	}
	if a4 := root.Sessions[2]; a4.HasChildren || a4.StarHidden {
		t.Errorf("a4 = %+v, want a plain row", a4)
	}
}

// TestRailViewOrphanPlacement pins an ended parent's live children: each
// renders top-level in its own worktree group, in the parent's slot when
// it shares the parent's worktree, else at the head of its group.
func TestRailViewOrphanPlacement(t *testing.T) {
	doc := emptyLayout()
	doc.Workspaces["/w/repo"] = &railLayoutWS{Order: []string{"a1", "ae", "a2", "b1"}, Children: map[string][]string{"ae": {"a3", "b2"}}}
	view := layoutTestView()
	orderRailView(&view, doc, layoutTestFacts())
	root, feat := view.Workspaces[1].Worktrees[0], view.Workspaces[1].Worktrees[1]
	if got := strings.Join(railIDs(root.Sessions), " "); got != "a1 a3 a2 a4" {
		t.Errorf("root lists %q, want a1 a3 a2 a4 (a3 in ae's slot, ordinary a4 last)", got)
	}
	if got := strings.Join(railIDs(feat.Sessions), " "); got != "b2 b1" {
		t.Errorf("feat lists %q, want b2 b1 (b2 at its group's head)", got)
	}
	for _, r := range append(root.Sessions, feat.Sessions...) {
		if r.Child || r.StarHidden || len(r.Children) != 0 {
			t.Errorf("%s = %+v, want a top-level row", r.ReplyID, r)
		}
	}
}

// writeLayoutFile writes raw as the layout file in dir.
func writeLayoutFile(t *testing.T, dir, raw string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, railLayoutFileName), []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
}

func readLayoutFile(t *testing.T, dir string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, railLayoutFileName))
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// TestRailLayoutStoreRoundTrip pins the file: written with schema_version
// 1, fields this version does not know kept through a write, a missing
// schema_version read as 1; a newer schema or a malformed file refuses
// every write and leaves the file as it is, and the rail keeps the layout
// last read.
func TestRailLayoutStoreRoundTrip(t *testing.T) {
	dir := t.TempDir()
	writeLayoutFile(t, dir, `{"future":{"k":1},"workspaces":{"/w/repo":{"order":["a1"],"later":"keep"}}}`)
	s := openRailLayoutStore(dir)
	doc, gen0 := s.current()
	if got := doc.Workspaces["/w/repo"].Order; !slices.Equal(got, []string{"a1"}) {
		t.Fatalf("read order %v, want [a1]", got)
	}
	answer, err := s.apply(railLayoutRequest{Op: "star", RID: "a2"}, layoutTestFacts(), true)
	if err != nil || answer.Reason != "" || !answer.Changed || answer.Gen <= gen0 {
		t.Fatalf("star = %+v, %v (gen before %d)", answer, err, gen0)
	}
	var got map[string]any
	if err := json.Unmarshal(readLayoutFile(t, dir), &got); err != nil {
		t.Fatal(err)
	}
	ws, _ := got["workspaces"].(map[string]any)
	repo, _ := ws["/w/repo"].(map[string]any)
	if got["schema_version"] != float64(1) || got["future"] == nil || repo["later"] != "keep" {
		t.Errorf("the written file lost a field: %v", got)
	}
	if stars, _ := repo["stars"].([]any); len(stars) != 1 || stars[0] != "a2" {
		t.Errorf("written stars = %v, want [a2]", repo["stars"])
	}
	again, err := s.apply(railLayoutRequest{Op: "star", RID: "a2"}, layoutTestFacts(), true)
	if err != nil || again.Reason != "" || again.Changed || again.Gen != answer.Gen {
		t.Errorf("an op that changes nothing = %+v, %v, want applied, unchanged, gen %d", again, err, answer.Gen)
	}
	if doc, _ := openRailLayoutStore(dir).current(); !slices.Equal(doc.Workspaces["/w/repo"].Stars, []string{"a2"}) {
		t.Errorf("a reopened store reads stars %v", doc.Workspaces["/w/repo"].Stars)
	}

	for name, raw := range map[string]string{
		"newer":     `{"schema_version":2,"workspaces":{}}`,
		"malformed": `{"schema_version":1,"workspaces":`,
	} {
		writeLayoutFile(t, dir, raw)
		answer, err := s.apply(railLayoutRequest{Op: "star", RID: "a3"}, layoutTestFacts(), true)
		if answer.Reason != layoutStore || err == nil {
			t.Errorf("%s: star = %+v, %v, want store", name, answer, err)
		}
		if name == "newer" && !errors.Is(err, errRailLayoutNewer) {
			t.Errorf("newer: error %v, want errRailLayoutNewer", err)
		}
		if string(readLayoutFile(t, dir)) != raw {
			t.Errorf("%s: the file was written over", name)
		}
		if doc, _ := s.current(); !slices.Equal(doc.Workspaces["/w/repo"].Stars, []string{"a2"}) {
			t.Errorf("%s: the rail lost the layout last read", name)
		}
	}
}

// TestRailLayoutStoreRereadsBeforeEveryWrite pins the merge: two stores on
// one file (two daemons, or a daemon and an editor) each write on the file
// as it is, so both edits stay, and each store's rail picks up the other's.
func TestRailLayoutStoreRereadsBeforeEveryWrite(t *testing.T) {
	dir := t.TempDir()
	one, two := openRailLayoutStore(dir), openRailLayoutStore(dir)
	f := layoutTestFacts()
	if a, err := one.apply(railLayoutRequest{Op: "star", RID: "a1"}, f, true); err != nil || a.Reason != "" {
		t.Fatal(a, err)
	}
	_, gen := one.current()
	if a, err := two.apply(railLayoutRequest{Op: "star", RID: "p1"}, f, true); err != nil || a.Reason != "" {
		t.Fatal(a, err)
	}
	var doc railLayoutDoc
	if err := json.Unmarshal(readLayoutFile(t, dir), &doc); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(doc.Workspaces["/w/repo"].Stars, []string{"a1"}) || !slices.Equal(doc.Workspaces["/w/plain"].Stars, []string{"p1"}) {
		t.Errorf("the file lost an edit: %s", readLayoutFile(t, dir))
	}
	cur, gen2 := one.current()
	if gen2 <= gen || !slices.Equal(cur.Workspaces["/w/plain"].Stars, []string{"p1"}) {
		t.Errorf("the first store's rail missed the second's write (gen %d -> %d)", gen, gen2)
	}
}

// TestRailLayoutUnknownEntries pins the edge rules for entries the rail
// does not list: ignored on read, kept by a write before the index has read
// the registry, and dropped by one after.
func TestRailLayoutUnknownEntries(t *testing.T) {
	dir := t.TempDir()
	writeLayoutFile(t, dir, `{"schema_version":1,"workspace_order":["/w/gone","/w/zed"],"workspaces":{
		"/w/repo":{"order":["gone","a2","a1"],"stars":["gone2","a1"],"children":{"gone3":["a3"],"a1":["gone4"]}},
		"/w/gone":{"order":["x"]}}}`)
	s := openRailLayoutStore(dir)
	doc, _ := s.current()
	order, starred := layoutRendered(doc)
	if order["/w/repo"] != "a1 a2 a3 a4" || order["ws"] != "/w/zed /w/plain /w/repo" || !slices.Equal(starred, []string{"a1"}) {
		t.Errorf("read with unknown entries: %v, starred %v", order, starred)
	}
	f := layoutTestFacts()
	if a, err := s.apply(railLayoutRequest{Op: "star", RID: "p1"}, f, false); err != nil || a.Reason != "" {
		t.Fatal(a, err)
	}
	raw := string(readLayoutFile(t, dir))
	for _, kept := range []string{`"gone"`, `"gone2"`, `"gone3"`, `"gone4"`, `"/w/gone"`} {
		if !strings.Contains(raw, kept) {
			t.Errorf("a write before the index loaded dropped %s:\n%s", kept, raw)
		}
	}
	if a, err := s.apply(railLayoutRequest{Op: "star", RID: "p2"}, f, true); err != nil || a.Reason != "" {
		t.Fatal(a, err)
	}
	raw = string(readLayoutFile(t, dir))
	for _, gone := range []string{`"gone`, `"x"`, `"/w/gone"`, `"a1": [`} {
		if strings.Contains(raw, gone) {
			t.Errorf("a write after the index loaded kept %s:\n%s", gone, raw)
		}
	}
	doc, _ = s.current()
	if repo := doc.Workspaces["/w/repo"]; !slices.Equal(repo.Order, []string{"a2", "a1"}) || !slices.Equal(repo.Stars, []string{"a1"}) || len(repo.Children) != 0 {
		t.Errorf("the pruned repository layout = %+v", repo)
	}
	if !slices.Equal(doc.WorkspaceOrder, []string{"/w/zed"}) {
		t.Errorf("the pruned workspace order = %v", doc.WorkspaceOrder)
	}
}

// TestRailLayoutRoute pins POST /rail/layout end to end: an import into the
// empty layout and an op apply, change the rail's stamp, and reach an open
// stream's rail; a refusal answers 409 with its reason, an import over a
// layout 200 not_applied; the route needs a POST with the daemon's Origin.
func TestRailLayoutRoute(t *testing.T) {
	f := startPageFixture(t)
	waitFor(t, "the index to load", f.d.Index().Loaded)
	stream := openStream(t, f.base+"/s/livesession/events")
	quiet(t, stream)
	post := func(body string) (int, railLayoutReply) {
		t.Helper()
		code, _, raw := postJSON(t, f.base+railLayoutRoute, f.base, body)
		var reply railLayoutReply
		if err := json.Unmarshal([]byte(raw), &reply); err != nil {
			t.Fatalf("POST %s = %d %q: %v", body, code, raw, err)
		}
		return code, reply
	}
	stamp := func() string {
		s, _ := f.d.railPageSource("en", "").stamp(refreshInputs{})
		return s
	}
	if code, page := get(t, f.base+"/s/livesession"); code != http.StatusOK || !strings.Contains(page, `data-layout-empty="1"`) {
		t.Errorf("the empty layout's rail lacks data-layout-empty")
	}

	before := stamp()
	code, reply := post(`{"op":"import","order":["boardsession","livesession"],"stars":["boardsession","nope"]}`)
	if code != http.StatusOK || !reply.Applied || reply.Gen == 0 {
		t.Fatalf("import = %d %+v", code, reply)
	}
	if stamp() == before {
		t.Error("a layout write left the rail's stamp as it was")
	}
	ev := waitRegionHolding(t, stream, regionRail, `aria-pressed="true"`)
	board, live := strings.Index(ev.HTML, `data-rid="boardsession"`), strings.Index(ev.HTML, `data-rid="livesession"`)
	if board < 0 || live < 0 || board > live || strings.Contains(ev.HTML, "data-layout-empty") {
		t.Errorf("the rail after the import does not lead with the starred boardsession:\n%s", ev.HTML)
	}
	if !strings.Contains(ev.HTML, `data-lgen="`) {
		t.Error("the rail lacks the layout generation")
	}

	code, reply = post(`{"op":"import","order":["livesession"]}`)
	if code != http.StatusOK || reply.Applied || reply.Reason != layoutNotApplied {
		t.Errorf("a second import = %d %+v, want 200 not_applied", code, reply)
	}
	code, reply = post(`{"op":"move","rid":"livesession","before":"boardsession"}`)
	if code != http.StatusConflict || reply.Reason != layoutStars {
		t.Errorf("a move across the stars = %d %+v, want 409 stars", code, reply)
	}
	code, reply = post(`{"op":"star","rid":"nope"}`)
	if code != http.StatusConflict || reply.Reason != layoutUnknown {
		t.Errorf("star unknown = %d %+v, want 409 unknown", code, reply)
	}
	code, reply = post(`{"op":`)
	if code != http.StatusBadRequest || reply.Reason != layoutInvalid {
		t.Errorf("a malformed body = %d %+v, want 400 invalid", code, reply)
	}
	quiet(t, stream)
	code, reply = post(`{"op":"unstar","rid":"boardsession"}`)
	if code != http.StatusOK || !reply.Applied {
		t.Fatalf("unstar = %d %+v", code, reply)
	}
	ev = waitRegionHolding(t, stream, regionRail, `data-rid="livesession"`)
	if strings.Contains(ev.HTML, `aria-pressed="true"`) {
		t.Error("the rail still shows a star after the unstar")
	}
	if !strings.Contains(string(readLayoutFile(t, f.dir)), `"boardsession"`) {
		t.Error("the layout file lacks the imported order")
	}

	if code, _, _ := postJSON(t, f.base+railLayoutRoute, "http://evil.example", `{"op":"star","rid":"livesession"}`); code != http.StatusForbidden {
		t.Errorf("a cross-origin write = %d, want 403", code)
	}
	if code, _ := get(t, f.base+railLayoutRoute); code != http.StatusMethodNotAllowed {
		t.Errorf("GET = %d, want 405", code)
	}
	for reason, want := range map[string]int{
		layoutRetry: http.StatusServiceUnavailable, layoutStore: http.StatusInternalServerError,
		layoutNotApplied: http.StatusOK, layoutDepth: http.StatusConflict,
	} {
		if got := railLayoutStatus(reason); got != want {
			t.Errorf("status of %s = %d, want %d", reason, got, want)
		}
	}
}
