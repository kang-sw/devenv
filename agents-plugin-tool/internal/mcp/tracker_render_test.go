package mcp

import (
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wstracker"
	"github.com/kang-sw/devenv/internal/wstrackerhost"
)

// The team scope prints a legend, each person's load with its per-status
// breakdown and the stale, overdue, idea, fallback-held, and done counts,
// and an admin's outside-subtree members under their own heading.
func TestTrackerTeamScopeRendersTheWorkloadRollup(t *testing.T) {
	last := "2026-11-05T10:00:00Z"
	v := &wstrackerhost.StoreView{
		View: &wstracker.View{
			Viewer:  wstracker.Viewer{Member: "lena", Admin: true, IsMember: true},
			Members: map[string]wstracker.MemberView{"kai": {ID: "kai", Name: "Kai"}, "ada": {ID: "ada", Name: "Ada"}},
			My: wstracker.My{Team: []wstracker.Team{
				{ID: "kai", Depth: 2, Counts: map[string]int{"todo": 2, "doing": 1, "blocked": 1, "awaiting": 1, "idea": 1, "done": 2},
					Load: 5, Stale: 1, Overdue: 1, Fallback: 1, Last: &last},
				{ID: "ada", Depth: 0, Outside: true, Counts: map[string]int{}, Fallback: 1},
			}},
		},
		Store: wstrackerhost.StoreInfo{Name: "team"},
	}
	out := formatTrackerScope(v, trackerQuery{scope: "team"})
	for _, want := range []string{
		"team: " + trackerTeamLegend,
		"kai (Kai) (depth 2): load 5 (todo 2, doing 1, blocked 1, awaiting 1), 1 stale, 1 overdue, 1 idea, 1 fallback-held, 2 done, last update " + last,
		"outside my subtree (shown to an admin; depth from the org-tree root):\n\nada (Ada) (depth 0): load 0, 0 stale, 0 overdue, 0 idea, 1 fallback-held, 0 done\n",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("team scope lacks %q:\n%s", want, out)
		}
	}
	if strings.Index(out, "kai (Kai)") > strings.Index(out, "outside my subtree") {
		t.Fatalf("the subtree rows must precede the outside heading:\n%s", out)
	}

	v.My.Team = nil
	if out := formatTrackerScope(v, trackerQuery{scope: "team"}); !strings.Contains(out, "team: no followers") || strings.Contains(out, trackerTeamLegend) {
		t.Fatalf("an empty team prints no legend:\n%s", out)
	}
}
