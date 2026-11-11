package wstracker

import (
	"maps"
	"testing"
	"time"
)

// workloadView derives the workload-balance case (testdata/cases) for one
// viewer clone, through the Go domain rather than the engine's expectation.
func workloadView(t *testing.T, viewer string) *View {
	t.Helper()
	doc, src := loadGolden(t, "testdata/golden/case-workload-balance__"+viewer+".json")
	now, err := time.Parse(time.RFC3339Nano, doc.Now)
	if err != nil {
		t.Fatal(err)
	}
	return Derive(src, viewer, now, 0)
}

func teamIDs(v *View) []string {
	var out []string
	for _, r := range v.My.Team {
		out = append(out, r.ID)
	}
	return out
}

func wantTeam(t *testing.T, v *View, want []string, outside map[string]bool, depth map[string]int) {
	t.Helper()
	got := teamIDs(v)
	if len(got) != len(want) {
		t.Fatalf("%s team = %v, want %v", v.Viewer.Stream, got, want)
	}
	for i, r := range v.My.Team {
		if r.ID != want[i] || r.Outside != outside[r.ID] || r.Depth != depth[r.ID] {
			t.Fatalf("%s team row %d = %s depth %d outside %v, want %s depth %d outside %v",
				v.Viewer.Stream, i, r.ID, r.Depth, r.Outside, want[i], depth[want[i]], outside[want[i]])
		}
	}
}

// A lead sees their whole subtree, however deep; a non-admin sees nobody
// outside it; a member who leads no one and is no admin sees nobody.
func TestTeamIsTheWholeSubtree(t *testing.T) {
	wantTeam(t, workloadView(t, "maya.pc"), []string{"lena", "omar", "kai"}, nil,
		map[string]int{"lena": 1, "omar": 2, "kai": 3})
	if got := teamIDs(workloadView(t, "kai.pc")); len(got) != 0 {
		t.Fatalf("kai (no followers, not an admin) team = %v, want none", got)
	}
}

// An admin sees every live member: the subtree first, depth from the viewer,
// then the outside group walked from the org-tree roots, depth from the root.
func TestAdminTeamAddsTheOutsideGroup(t *testing.T) {
	// ada is a root: kai, four levels below her, is in her subtree
	wantTeam(t, workloadView(t, "ada.pc"), []string{"maya", "lena", "omar", "kai", "bo", "zed"},
		map[string]bool{"bo": true, "zed": true}, map[string]int{"maya": 1, "lena": 2, "omar": 3, "kai": 4, "bo": 0, "zed": 1})
	// lena sits inside the tree: her lead chain and the other root's branch are outside
	wantTeam(t, workloadView(t, "lena.pc"), []string{"omar", "kai", "ada", "maya", "bo", "zed"},
		map[string]bool{"ada": true, "maya": true, "bo": true, "zed": true}, map[string]int{"omar": 1, "kai": 2, "ada": 0, "maya": 1, "bo": 0, "zed": 1})
}

// Counts per status leave out the creator-fallback holding and the held
// topic; load is todo, doing, and blocked; stale and overdue are inside
// load (a done or fallback item past its due date is not overdue); idea,
// awaiting, and fallback stay out of it.
func TestTeamRowCounts(t *testing.T) {
	v := workloadView(t, "ada.pc")
	kai := teamRow(t, v, "kai")
	wantCounts := map[string]int{"todo": 2, "doing": 1, "blocked": 1, "awaiting": 1, "idea": 1, "done": 2}
	if !maps.Equal(kai.Counts, wantCounts) {
		t.Fatalf("kai counts = %v, want %v", kai.Counts, wantCounts)
	}
	wantEq(t, "kai load (awaiting is not)", kai.Load, 4)
	wantEq(t, "kai stale (the doing set five days ago; the blocked set a day ago is not)", kai.Stale, 1)
	wantEq(t, "kai overdue (the todo past due; the awaiting, the done, and the fallback past due are not)", kai.Overdue, 1)
	wantEq(t, "kai fallback-held (the closed fallback is not)", kai.Fallback, 1)
	wantEq(t, "kai pending (bo's request)", len(kai.Pending), 1)
	// open lists every live holding not done: the four load rows, the awaiting, the idea, the fallback
	wantEq(t, "kai open", len(kai.Open), 7)
	wantEq(t, "bo waiting", len(teamRow(t, v, "bo").Waiting), 1)
	wantEq(t, "zed load", teamRow(t, v, "zed").Load, 1)

	// the viewer's own fallback holding shows in her row of an admin peer's view
	ada := teamRow(t, workloadView(t, "lena.pc"), "ada")
	wantEq(t, "ada fallback-held", ada.Fallback, 1)
	wantEq(t, "ada load", ada.Load, 0)
	wantEq(t, "ada counts", len(ada.Counts), 0)
}

// Overdue and stale need a clock: a zero now turns both off.
func TestTeamRowOverdueNeedsAClock(t *testing.T) {
	doc, src := loadGolden(t, "testdata/golden/case-workload-balance__ada.pc.json")
	kai := teamRow(t, Derive(src, doc.Viewer, time.Time{}, 0), "kai")
	wantEq(t, "kai overdue with no clock", kai.Overdue, 0)
	wantEq(t, "kai stale with no clock", kai.Stale, 0)
	wantEq(t, "kai load with no clock", kai.Load, 4)
}
