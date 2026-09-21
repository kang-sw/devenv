package mcp

import (
	"github.com/kang-sw/devenv/internal/wskey"
	"testing"
)

func TestImplIdentityAuthority(t *testing.T) {
	stem := "260101-feat-example"
	suffix := wskey.Derive(stem, 3)
	for _, tc := range []struct{ base, want string }{
		{"develop", "impl/develop/" + suffix},
		{"goal/develop/topic", "impl/goal/develop/topic/" + suffix},
		{"impl/goal/develop/topic/manual", "impl/goal/develop/topic/" + suffix},
		{"impl/legacy", "impl/" + suffix},
		{"implement/legacy", "impl/" + suffix},
	} {
		for i := 0; i < 2; i++ {
			if got := implTicketBranch(tc.base, stem); got != tc.want {
				t.Fatalf("%s: %s != %s", tc.base, got, tc.want)
			}
		}
	}
	if got := matchImplTicketStems(suffix, []string{"other", stem}); len(got) != 1 || got[0] != stem {
		t.Fatal(got)
	}
	if got := matchImplTicketStems("no-match", []string{stem}); len(got) != 0 {
		t.Fatal(got)
	}
	if got := matchImplTicketStems(suffix, []string{stem, stem}); len(got) != 2 {
		t.Fatal("ambiguity lost", got)
	}
}

func TestImplIdentityVouch(t *testing.T) {
	stem := "260101-feat-example"
	branch := "impl/develop/manual"
	n := normalizedImplementFacts{TicketStem: stem, ScopeSlug: implTicketSuffix(stem), AllowRename: "yes", MergeConfirmPolicy: "ask"}
	obs := implementBranchObservation{CurrentBranch: branch, AheadOfMergeRoot: 1}
	if got := deriveImplementBranchPlan(n, obs); got.Action != "stop" {
		t.Fatal(got)
	}
	for _, tc := range []struct {
		name, branch, stem  string
		tracking, collision bool
		want                string
	}{
		{"valid", branch, stem, false, false, "rename"},
		{"wrong branch", "impl/develop/else", stem, false, false, "stop"},
		{"wrong ticket", branch, "other", false, false, "stop"},
		{"shared", branch, stem, true, false, "continue"},
		{"collision", branch, stem, false, true, "stop"},
		{"shared collision", branch, stem, true, true, "stop"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			nn, oo := n, obs
			nn.IdentityVouch = &implementIdentityVouch{Branch: tc.branch, TicketStem: tc.stem}
			oo.TargetExists = tc.collision
			if tc.tracking {
				oo.Upstream = "origin/manual"
			}
			got := deriveImplementBranchPlan(nn, oo)
			if got.Action != tc.want {
				t.Fatal(got)
			}
			if got.MergeConfirm != "ask" {
				t.Fatal(got)
			}
			if tc.want == "continue" && got.TargetBranch != branch {
				t.Fatal(got)
			}
		})
	}
}
