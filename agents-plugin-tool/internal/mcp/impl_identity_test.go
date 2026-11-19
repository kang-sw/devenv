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
		targetAhead         int
		want                string
	}{
		{"valid", branch, stem, false, false, 0, "rename"},
		{"wrong branch", "impl/develop/else", stem, false, false, 0, "stop"},
		{"wrong ticket", branch, "other", false, false, 0, "stop"},
		{"shared", branch, stem, true, false, 0, "continue"},
		{"collision", branch, stem, false, true, 0, "recreate"},
		{"shared collision", branch, stem, true, true, 0, "recreate"},
		{"unique target collision", branch, stem, false, true, 1, "stop"},
		{"shared unique target collision", branch, stem, true, true, 1, "stop"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			nn, oo := n, obs
			nn.IdentityVouch = &implementIdentityVouch{Branch: tc.branch, TicketStem: tc.stem}
			oo.TargetExists = tc.collision
			oo.TargetAheadOfMergeRoot = tc.targetAhead
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

func TestImplIdentityLandedTargetVouch(t *testing.T) {
	stem := "260101-feat-example"
	for _, base := range []struct {
		branch, mergeRoot string
		legacy            bool
	}{
		{"impl/develop/manual", "develop", false},
		{"impl/goal/develop/topic/manual", "goal/develop/topic", false},
		{"impl/legacy", "develop", true},
		{"implement/legacy", "develop", true},
	} {
		t.Run(base.branch, func(t *testing.T) {
			for _, tc := range []struct {
				name, vouch, allowRename, upstream string
				ahead, behind, targetAhead         int
				cleanCurrent, canonicalCurrent     bool
				want                               string
			}{
				{name: "matching vouch", want: "recreate"},
				{name: "clean current with vouch", cleanCurrent: true, want: "recreate"},
				{name: "rename disabled", allowRename: "no", want: "recreate"},
				{name: "upstream", upstream: "origin/manual", want: "recreate"},
				{name: "tracking ahead", ahead: 1, want: "recreate"},
				{name: "tracking behind", behind: 1, want: "recreate"},
				{name: "absent vouch", vouch: "absent", want: "stop"},
				{name: "wrong branch vouch", vouch: "wrong branch", want: "stop"},
				{name: "wrong ticket vouch", vouch: "wrong ticket", want: "stop"},
				{name: "clean current without vouch", vouch: "absent", cleanCurrent: true, want: "stop"},
				{name: "clean current wrong branch vouch", vouch: "wrong branch", cleanCurrent: true, want: "stop"},
				{name: "clean current wrong ticket vouch", vouch: "wrong ticket", cleanCurrent: true, want: "stop"},
				{name: "unique target", targetAhead: 1, want: "stop"},
				{name: "unique target rename disabled", targetAhead: 1, allowRename: "no", want: "stop"},
				{name: "unique target with tracking", targetAhead: 1, upstream: "origin/manual", want: "stop"},
				{name: "canonical current", vouch: "absent", canonicalCurrent: true, targetAhead: 1, want: "continue"},
			} {
				t.Run(tc.name, func(t *testing.T) {
					n := normalizedImplementFacts{
						TicketStem:         stem,
						ScopeSlug:          implTicketSuffix(stem),
						AllowRename:        firstNonEmpty(tc.allowRename, "yes"),
						MergeTargetPolicy:  "develop",
						MergeConfirmPolicy: "ask",
						IdentityVouch:      &implementIdentityVouch{Branch: base.branch, TicketStem: stem},
					}
					switch tc.vouch {
					case "absent":
						n.IdentityVouch = nil
					case "wrong branch":
						n.IdentityVouch.Branch = base.branch + "-other"
					case "wrong ticket":
						n.IdentityVouch.TicketStem = "260101-feat-other"
					}
					target := implTicketBranch(base.branch, stem)
					obs := implementBranchObservation{
						CurrentBranch:          base.branch,
						StartCommit:            "start-commit",
						AheadOfMergeRoot:       1,
						TargetExists:           true,
						TargetAheadOfMergeRoot: tc.targetAhead,
						Upstream:               tc.upstream,
						Ahead:                  tc.ahead,
						Behind:                 tc.behind,
					}
					if tc.cleanCurrent {
						obs.AheadOfMergeRoot = 0
					}
					if tc.canonicalCurrent {
						obs.CurrentBranch = target
					}
					want := tc.want
					if base.legacy && want == "recreate" {
						want = "stop"
					}
					got := deriveImplementBranchPlan(n, obs)
					if got.Action != want || got.TargetBranch != target || got.MergeTarget != base.mergeRoot ||
						got.CurrentBranch != obs.CurrentBranch || got.StartCommit != obs.StartCommit || got.MergeConfirm != "ask" {
						t.Fatalf("want action %s, target %s, merge root %s and unchanged current/start/confirmation; got %+v", want, target, base.mergeRoot, got)
					}
				})
			}
		})
	}
}
