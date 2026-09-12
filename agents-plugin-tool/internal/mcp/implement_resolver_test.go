package mcp

import (
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wskey"
)

// TestResolveImplementSmallestSafeChangeStillGetsIndependentReview pins the
// collapse: the fact set that used to select the caller-edits/caller-reviews
// fast path — single-file, internal, no new public symbol or type contract, and
// all four risk axes genuinely low — now resolves to the one execution mode with
// an independent reviewer allocated. No fact combination reaching this resolver
// may drop review; the legacy top-level enter path still honors an explicit
// need_review=false from its caller and is out of this test's scope.
func TestResolveImplementSmallestSafeChangeStillGetsIndependentReview(t *testing.T) {
	input := implementInput{
		Target: implementTargetInput{Kind: "inline", Label: "tiny edit", ScopeLabel: "tiny edit", ScopeSlug: "tiny-edit"},
		Facts: implementFactsInput{
			Scope: implementScopeFactsInput{
				Span:            factString{Value: "single-file", Present: true},
				Surface:         factString{Value: "internal", Present: true},
				NewPublicSymbol: factString{Value: "no", Present: true},
				NewTypeContract: factString{Value: "no", Present: true},
				TestSurface:     factString{Value: "none", Present: true},
			},
			Complexity: implementComplexityFactsInput{
				ReusePoints:    factString{Value: "not-applicable", Present: true},
				SideEffectRisk: factString{Value: "low", Present: true},
			},
			Risk: implementRiskFactsInput{
				Correctness:        factString{Value: "low", Present: true},
				Fit:                factString{Value: "low", Present: true},
				Test:               factString{Value: "low", Present: true},
				SecurityOrContract: factString{Value: "low", Present: true},
			},
		},
	}
	result := resolveImplement(input, factsFromTicket(input), implementBranchObservation{CurrentBranch: "feature/demo", StartCommit: "abc123"})
	if result.Verdict.Delegation != "delegated" {
		t.Fatalf("delegation = %q, want delegated", result.Verdict.Delegation)
	}
	if result.Verdict.ReviewAlloc != "single" || !result.Verdict.NeedReview {
		t.Fatalf("review = %q need=%v, want single true", result.Verdict.ReviewAlloc, result.Verdict.NeedReview)
	}
	for _, forbidden := range []string{"plan-populator", "path.generate", "Plan Depth", "direct-edit", "lead-only"} {
		if strings.Contains(result.Raw, forbidden) {
			t.Fatalf("verdict retained removed stage or fast path %q:\n%s", forbidden, result.Raw)
		}
	}
	for _, gone := range []string{"explicit-delegation-request", "explicit-direct-edit-request", "low-ceremony-if-safe", "change-points", "strategy-shape", "cold-context"} {
		if containsPrefixed(result.Conditions, gone) {
			t.Fatalf("conditions retained removed fact %q: %v", gone, result.Conditions)
		}
	}
}

// factsFromTicket is the test stand-in for the ticket read handleEnterImplement
// performs: it presents a fixture's facts to the resolver exactly as a
// populated `## Route Facts` section would. Tests about the read itself build
// their own implementRouteFactsSource.
func factsFromTicket(input implementInput) implementRouteFactsSource {
	return implementRouteFactsSource{Facts: input.Facts, Status: "ticket"}
}

// containsPrefixed reports whether any condition line starts with prefix+"=".
func containsPrefixed(conditions []string, prefix string) bool {
	for _, c := range conditions {
		if strings.HasPrefix(c, prefix+"=") {
			return true
		}
	}
	return false
}

func TestDeriveImplementReviewAllocProportionalPartitions(t *testing.T) {
	for _, tc := range []struct {
		name  string
		facts normalizedImplementFacts
		want  string
	}{
		{
			name: "bounded public surface with existing tests uses one reviewer",
			facts: normalizedImplementFacts{
				Surface: "public-interface", TestSurface: "existing", ReusePoints: "confirmed",
				CorrectnessRisk: "low", FitRisk: "low", TestRisk: "low", SecurityOrContractRisk: "low",
			},
			want: "single",
		},
		{
			name: "one correctness partition still uses one reviewer",
			facts: normalizedImplementFacts{
				Surface: "public-interface", TestSurface: "existing", ReusePoints: "confirmed", NewPublicSymbol: "yes",
				CorrectnessRisk: "low", FitRisk: "low", TestRisk: "low", SecurityOrContractRisk: "low",
			},
			want: "single",
		},
		{
			name: "independent cross-module and new-test risks stay partitioned",
			facts: normalizedImplementFacts{
				Surface: "cross-module", TestSurface: "new-files", ReusePoints: "confirmed",
				CorrectnessRisk: "low", FitRisk: "low", TestRisk: "low", SecurityOrContractRisk: "low",
			},
			want: "partitioned: fit, test",
		},
		{
			name: "all-unknown fact set is a non-signal and lands on single",
			facts: normalizedImplementFacts{
				Surface: "unknown", TestSurface: "unknown", ReusePoints: "unknown",
				CorrectnessRisk: "unknown", FitRisk: "unknown", TestRisk: "unknown", SecurityOrContractRisk: "unknown",
			},
			want: "single",
		},
		{
			name: "correctness risk plus unconfirmed fit yields two partitions",
			facts: normalizedImplementFacts{
				Surface: "internal", TestSurface: "existing", ReusePoints: "unconfirmed",
				CorrectnessRisk: "moderate", FitRisk: "low", TestRisk: "low", SecurityOrContractRisk: "low",
			},
			want: "partitioned: correctness, fit",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := deriveImplementReviewAlloc(tc.facts); got != tc.want {
				t.Fatalf("review allocation = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestImplementReviewPartitionsTreatUnknownAsNonSignal(t *testing.T) {
	for _, tc := range []struct {
		name  string
		facts normalizedImplementFacts
		want  []string
	}{
		{
			name: "all-unknown fact set yields zero partitions",
			facts: normalizedImplementFacts{
				Surface: "unknown", TestSurface: "unknown", ReusePoints: "unknown",
				CorrectnessRisk: "unknown", FitRisk: "unknown", TestRisk: "unknown", SecurityOrContractRisk: "unknown",
			},
			want: []string{},
		},
		{
			name: "moderate correctness risk alone signals correctness only",
			facts: normalizedImplementFacts{
				Surface: "unknown", TestSurface: "unknown", ReusePoints: "unknown",
				CorrectnessRisk: "moderate", FitRisk: "unknown", TestRisk: "unknown", SecurityOrContractRisk: "unknown",
			},
			want: []string{"correctness"},
		},
		{
			name: "new type contract alone signals correctness only",
			facts: normalizedImplementFacts{
				NewTypeContract: "yes",
				Surface:         "unknown", TestSurface: "unknown", ReusePoints: "unknown",
				CorrectnessRisk: "unknown", FitRisk: "unknown", TestRisk: "unknown", SecurityOrContractRisk: "unknown",
			},
			want: []string{"correctness"},
		},
		{
			name: "new public symbol alone signals correctness only",
			facts: normalizedImplementFacts{
				NewPublicSymbol: "yes",
				Surface:         "unknown", TestSurface: "unknown", ReusePoints: "unknown",
				CorrectnessRisk: "unknown", FitRisk: "unknown", TestRisk: "unknown", SecurityOrContractRisk: "unknown",
			},
			want: []string{"correctness"},
		},
		{
			name: "cross-module surface alone signals fit only",
			facts: normalizedImplementFacts{
				Surface: "cross-module", TestSurface: "unknown", ReusePoints: "unknown",
				CorrectnessRisk: "unknown", FitRisk: "unknown", TestRisk: "unknown", SecurityOrContractRisk: "unknown",
			},
			want: []string{"fit"},
		},
		{
			name: "unconfirmed reuse points alone signals fit only",
			facts: normalizedImplementFacts{
				Surface: "unknown", TestSurface: "unknown", ReusePoints: "unconfirmed",
				CorrectnessRisk: "unknown", FitRisk: "unknown", TestRisk: "unknown", SecurityOrContractRisk: "unknown",
			},
			want: []string{"fit"},
		},
		{
			name: "new-files test surface alone signals test only",
			facts: normalizedImplementFacts{
				Surface: "unknown", TestSurface: "new-files", ReusePoints: "unknown",
				CorrectnessRisk: "unknown", FitRisk: "unknown", TestRisk: "unknown", SecurityOrContractRisk: "unknown",
			},
			want: []string{"test"},
		},
		{
			name: "mixed set surfaces only the signalled partitions",
			facts: normalizedImplementFacts{
				Surface: "unknown", TestSurface: "new-files", ReusePoints: "unknown",
				CorrectnessRisk: "high", FitRisk: "unknown", TestRisk: "unknown", SecurityOrContractRisk: "unknown",
			},
			want: []string{"correctness", "test"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := implementReviewPartitions(tc.facts)
			if len(got) != len(tc.want) {
				t.Fatalf("partitions = %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("partitions = %v, want %v", got, tc.want)
				}
			}
		})
	}
}

func TestResolveImplementBranchStopOmitsPlannerInstructions(t *testing.T) {
	input := implementInput{
		Target: implementTargetInput{Kind: "ticket", Label: "feature", ScopeLabel: "Phase 1", ScopeSlug: "feature"},
		Facts: implementFactsInput{
			Scope: implementScopeFactsInput{
				Span:    factString{Value: "multi-file", Present: true},
				Surface: factString{Value: "public-interface", Present: true},
			},
		},
		Policy: implementPolicyInput{
			Branch: implementBranchPolicyInput{MergeTarget: factString{Value: "main", Present: true}, AllowRename: factString{Value: "no", Present: true}},
		},
	}
	result := resolveImplement(input, factsFromTicket(input), implementBranchObservation{CurrentBranch: "implement/old", StartCommit: "abc123"})
	if result.Verdict.BranchPlan.Action != "stop" {
		t.Fatalf("branch action = %q, want stop", result.Verdict.BranchPlan.Action)
	}
	for _, forbidden := range []string{"path.generate", "plan-populator-survey", "plan-populator-research", "render implementer"} {
		if strings.Contains(result.NextInstruction, forbidden) {
			t.Fatalf("branch-stop next instruction includes unreachable %q: %q", forbidden, result.NextInstruction)
		}
	}
}

func TestResolveImplementBranchRenameDefaultsToAllowedWhenUnset(t *testing.T) {
	input := implementInput{
		Target: implementTargetInput{Kind: "ticket", Label: "feature", ScopeLabel: "Phase 1", ScopeSlug: "feature"},
		Facts: implementFactsInput{
			Scope: implementScopeFactsInput{
				Span:    factString{Value: "multi-file", Present: true},
				Surface: factString{Value: "public-interface", Present: true},
			},
		},
		Policy: implementPolicyInput{
			Branch: implementBranchPolicyInput{MergeTarget: factString{Value: "main", Present: true}},
		},
	}
	result := resolveImplement(input, factsFromTicket(input), implementBranchObservation{CurrentBranch: "impl/old", StartCommit: "abc123"})
	if result.Verdict.BranchPlan.Action != "rename" {
		t.Fatalf("branch action = %q, want rename (allow_rename absent should default to yes)", result.Verdict.BranchPlan.Action)
	}
}

func TestResolveImplementAheadOfMergeRootBlocksRenameRegardlessOfAllowRename(t *testing.T) {
	input := implementInput{
		Target: implementTargetInput{Kind: "ticket", Label: "feature", ScopeLabel: "Phase 2", ScopeSlug: "new", TicketStem: "260900-feat-new-thing"},
		Facts: implementFactsInput{
			Scope: implementScopeFactsInput{
				Span:    factString{Value: "multi-file", Present: true},
				Surface: factString{Value: "public-interface", Present: true},
			},
		},
		Policy: implementPolicyInput{
			Branch: implementBranchPolicyInput{AllowRename: factString{Value: "yes", Present: true}},
		},
	}
	obs := implementBranchObservation{CurrentBranch: "impl/root-branch/old", StartCommit: "abc123", AheadOfMergeRoot: 2}
	result := resolveImplement(input, factsFromTicket(input), obs)
	if result.Verdict.BranchPlan.Action != "stop" {
		t.Fatalf("branch action = %q, want stop even with allow_rename=yes; plan=%+v", result.Verdict.BranchPlan.Action, result.Verdict.BranchPlan)
	}
	if result.Verdict.BranchPlan.SuspectedOwnerStem != "old" {
		t.Fatalf("suspected owner stem = %q, want %q", result.Verdict.BranchPlan.SuspectedOwnerStem, "old")
	}
	for _, want := range []string{"session context", "explore", "old"} {
		if !strings.Contains(result.NextInstruction, want) {
			t.Fatalf("next instruction missing %q: %q", want, result.NextInstruction)
		}
	}
	combined := strings.ToLower(result.Verdict.BranchPlan.Reason + " " + result.NextInstruction)
	if strings.Contains(combined, "commit content") || strings.Contains(combined, "parsed the commit") {
		t.Fatalf("stop message must not claim commit-content parsing: reason=%q instruction=%q", result.Verdict.BranchPlan.Reason, result.NextInstruction)
	}
}

func TestObserveImplementBranchFailsClosedWhenAheadStateCannotBeVerified(t *testing.T) {
	root := t.TempDir()
	initGit(t, root)
	runGit(t, root, "checkout", "-b", "impl/missing-root/owner")
	if _, err := observeImplementBranch(root, ""); err == nil {
		t.Fatal("observeImplementBranch unexpectedly treated an unresolvable merge root as zero commits ahead")
	}
}

func TestResolveImplementNoAheadOfMergeRootAllowsRename(t *testing.T) {
	input := implementInput{
		Target: implementTargetInput{Kind: "ticket", Label: "feature", ScopeLabel: "Phase 2", ScopeSlug: "new"},
		Facts: implementFactsInput{
			Scope: implementScopeFactsInput{
				Span:    factString{Value: "multi-file", Present: true},
				Surface: factString{Value: "public-interface", Present: true},
			},
		},
		Policy: implementPolicyInput{
			Branch: implementBranchPolicyInput{AllowRename: factString{Value: "yes", Present: true}},
		},
	}
	obs := implementBranchObservation{CurrentBranch: "impl/root-branch/old", StartCommit: "abc123", AheadOfMergeRoot: 0}
	result := resolveImplement(input, factsFromTicket(input), obs)
	if result.Verdict.BranchPlan.Action != "rename" {
		t.Fatalf("branch action = %q, want rename when AheadOfMergeRoot is 0; plan=%+v", result.Verdict.BranchPlan.Action, result.Verdict.BranchPlan)
	}
}

func TestResolveImplementSameScopeContinuesRegardlessOfAheadOfMergeRoot(t *testing.T) {
	input := implementInput{
		Target: implementTargetInput{Kind: "ticket", Label: "feature", ScopeLabel: "Phase 1", ScopeSlug: "old"},
		Facts: implementFactsInput{
			Scope: implementScopeFactsInput{
				Span:    factString{Value: "multi-file", Present: true},
				Surface: factString{Value: "public-interface", Present: true},
			},
		},
		Policy: implementPolicyInput{
			Branch: implementBranchPolicyInput{AllowRename: factString{Value: "yes", Present: true}},
		},
	}
	obs := implementBranchObservation{CurrentBranch: "impl/root-branch/old", StartCommit: "abc123", AheadOfMergeRoot: 5}
	result := resolveImplement(input, factsFromTicket(input), obs)
	if result.Verdict.BranchPlan.Action != "continue" {
		t.Fatalf("branch action = %q, want continue when target scope matches current, regardless of AheadOfMergeRoot; plan=%+v", result.Verdict.BranchPlan.Action, result.Verdict.BranchPlan)
	}
}

func TestResolveImplementAheadOfMergeRootInertOnCreatePath(t *testing.T) {
	input := implementInput{
		Target: implementTargetInput{Kind: "ticket", Label: "feature", ScopeLabel: "Phase 1", ScopeSlug: "new"},
		Facts: implementFactsInput{
			Scope: implementScopeFactsInput{
				Span:    factString{Value: "multi-file", Present: true},
				Surface: factString{Value: "public-interface", Present: true},
			},
		},
	}
	obs := implementBranchObservation{CurrentBranch: "goal/some-slug", StartCommit: "abc123", AheadOfMergeRoot: 7}
	result := resolveImplement(input, factsFromTicket(input), obs)
	if result.Verdict.BranchPlan.Action != "create" {
		t.Fatalf("branch action = %q, want create on non-impl/-prefixed branch regardless of AheadOfMergeRoot; plan=%+v", result.Verdict.BranchPlan.Action, result.Verdict.BranchPlan)
	}
}

func TestResolveImplementMergeConfirmDefaultsToAskWhenUnset(t *testing.T) {
	input := implementInput{
		Target: implementTargetInput{Kind: "ticket", Label: "feature", ScopeLabel: "Phase 1", ScopeSlug: "feature"},
		Facts: implementFactsInput{
			Scope: implementScopeFactsInput{
				Span:    factString{Value: "multi-file", Present: true},
				Surface: factString{Value: "public-interface", Present: true},
			},
		},
		Policy: implementPolicyInput{
			Branch: implementBranchPolicyInput{MergeTarget: factString{Value: "main", Present: true}},
		},
	}
	result := resolveImplement(input, factsFromTicket(input), implementBranchObservation{CurrentBranch: "impl/old", StartCommit: "abc123"})
	if result.Verdict.BranchPlan.MergeConfirm != "ask" {
		t.Fatalf("merge confirm = %q, want ask (absent should default to ask)", result.Verdict.BranchPlan.MergeConfirm)
	}
}

func TestResolveImplementMergeConfirmSkipHonored(t *testing.T) {
	input := implementInput{
		Target: implementTargetInput{Kind: "ticket", Label: "feature", ScopeLabel: "Phase 1", ScopeSlug: "feature"},
		Facts: implementFactsInput{
			Scope: implementScopeFactsInput{
				Span:    factString{Value: "multi-file", Present: true},
				Surface: factString{Value: "public-interface", Present: true},
			},
		},
		Policy: implementPolicyInput{
			Branch: implementBranchPolicyInput{
				MergeTarget:  factString{Value: "main", Present: true},
				MergeConfirm: factString{Value: "skip", Present: true},
			},
		},
	}
	result := resolveImplement(input, factsFromTicket(input), implementBranchObservation{CurrentBranch: "impl/old", StartCommit: "abc123"})
	if result.Verdict.BranchPlan.MergeConfirm != "skip" {
		t.Fatalf("merge confirm = %q, want skip (explicit skip should be honored)", result.Verdict.BranchPlan.MergeConfirm)
	}
}

func TestResolveImplementMergeConfirmNonSkipStillAsks(t *testing.T) {
	input := implementInput{
		Target: implementTargetInput{Kind: "ticket", Label: "feature", ScopeLabel: "Phase 1", ScopeSlug: "feature"},
		Facts: implementFactsInput{
			Scope: implementScopeFactsInput{
				Span:    factString{Value: "multi-file", Present: true},
				Surface: factString{Value: "public-interface", Present: true},
			},
		},
		Policy: implementPolicyInput{
			Branch: implementBranchPolicyInput{
				MergeTarget:  factString{Value: "main", Present: true},
				MergeConfirm: factString{Value: "ask", Present: true},
			},
		},
	}
	result := resolveImplement(input, factsFromTicket(input), implementBranchObservation{CurrentBranch: "impl/old", StartCommit: "abc123"})
	if result.Verdict.BranchPlan.MergeConfirm != "ask" {
		t.Fatalf("merge confirm = %q, want ask (explicit non-skip value should still ask)", result.Verdict.BranchPlan.MergeConfirm)
	}
}

func TestResolveImplementMergeTargetPolicyIgnoredOutsideImplementBranchWarns(t *testing.T) {
	input := implementInput{
		Target: implementTargetInput{Kind: "inline", Label: "tiny edit", ScopeLabel: "tiny edit", ScopeSlug: "tiny-edit"},
		Facts: implementFactsInput{
			Scope: implementScopeFactsInput{
				Span:            factString{Value: "single-file", Present: true},
				Surface:         factString{Value: "internal", Present: true},
				NewPublicSymbol: factString{Value: "no", Present: true},
				NewTypeContract: factString{Value: "no", Present: true},
				TestSurface:     factString{Value: "none", Present: true},
			},
		},
		Policy: implementPolicyInput{
			Branch: implementBranchPolicyInput{MergeTarget: factString{Value: "master", Present: true}},
		},
	}
	result := resolveImplement(input, factsFromTicket(input), implementBranchObservation{CurrentBranch: "test/wsflow-smoke", StartCommit: "abc123"})
	if result.Verdict.BranchPlan.Action != "create" {
		t.Fatalf("branch action = %q, want create", result.Verdict.BranchPlan.Action)
	}
	if result.Verdict.BranchPlan.MergeTarget != "test/wsflow-smoke" {
		t.Fatalf("merge target = %q, want derived current branch", result.Verdict.BranchPlan.MergeTarget)
	}
	wantWarning := `policy.branch.merge_target "master" ignored (not on an implementation branch: impl/*, or legacy implement/*); derived from current branch "test/wsflow-smoke"`
	if !containsString(result.Warnings, wantWarning) {
		t.Fatalf("warnings missing ignored merge_target note: %v", result.Warnings)
	}
	if !strings.Contains(result.Raw, wantWarning) {
		t.Fatalf("raw missing ignored merge_target note:\n%s", result.Raw)
	}
}

func TestResolveImplementMergeTargetPolicyHonoredOnImplementBranchNoWarning(t *testing.T) {
	input := implementInput{
		Target: implementTargetInput{Kind: "inline", Label: "tiny edit", ScopeLabel: "tiny edit", ScopeSlug: "tiny-edit"},
		Facts: implementFactsInput{
			Scope: implementScopeFactsInput{
				Span:            factString{Value: "single-file", Present: true},
				Surface:         factString{Value: "internal", Present: true},
				NewPublicSymbol: factString{Value: "no", Present: true},
				NewTypeContract: factString{Value: "no", Present: true},
				TestSurface:     factString{Value: "none", Present: true},
			},
		},
		Policy: implementPolicyInput{
			Branch: implementBranchPolicyInput{MergeTarget: factString{Value: "master", Present: true}},
		},
	}
	result := resolveImplement(input, factsFromTicket(input), implementBranchObservation{CurrentBranch: "impl/tiny-edit", StartCommit: "abc123"})
	if result.Verdict.BranchPlan.Action != "continue" {
		t.Fatalf("branch action = %q, want continue", result.Verdict.BranchPlan.Action)
	}
	if result.Verdict.BranchPlan.MergeTarget != "master" {
		t.Fatalf("merge target = %q, want policy value honored", result.Verdict.BranchPlan.MergeTarget)
	}
	for _, w := range result.Warnings {
		if strings.Contains(w, "merge_target") {
			t.Fatalf("unexpected merge_target warning when policy applied: %v", result.Warnings)
		}
	}
}

func TestResolveImplementBranchPlanRules(t *testing.T) {
	base := normalizedImplementFacts{ScopeSlug: "target", MergeTargetPolicy: "feature/base", AllowRename: "no"}
	cases := []struct {
		name             string
		facts            normalizedImplementFacts
		obs              implementBranchObservation
		wantAction       string
		wantReason       string
		wantTargetBranch string
	}{
		{
			name:             "create outside implement branch",
			facts:            base,
			obs:              implementBranchObservation{CurrentBranch: "feature/base", StartCommit: "abc123"},
			wantAction:       "create",
			wantReason:       "not an implementation branch",
			wantTargetBranch: "impl/feature/base/target",
		},
		{
			name:       "stop missing merge target on implement branch",
			facts:      normalizedImplementFacts{ScopeSlug: "target"},
			obs:        implementBranchObservation{CurrentBranch: "implement/old", StartCommit: "abc123"},
			wantAction: "stop",
			wantReason: "merge target required",
		},
		{
			name:             "continue matching branch",
			facts:            base,
			obs:              implementBranchObservation{CurrentBranch: "impl/target", StartCommit: "abc123"},
			wantAction:       "continue",
			wantReason:       "matches target scope",
			wantTargetBranch: "impl/target",
		},
		{
			name:       "rename allowed",
			facts:      normalizedImplementFacts{ScopeSlug: "target", MergeTargetPolicy: "feature/base", AllowRename: "yes"},
			obs:        implementBranchObservation{CurrentBranch: "implement/old", StartCommit: "abc123"},
			wantAction: "rename",
			wantReason: "rename is allowed",
		},
		{
			name:       "stop target exists",
			facts:      normalizedImplementFacts{ScopeSlug: "target", MergeTargetPolicy: "feature/base", AllowRename: "yes"},
			obs:        implementBranchObservation{CurrentBranch: "implement/old", StartCommit: "abc123", TargetExists: true},
			wantAction: "stop",
			wantReason: "already exists",
		},
		{
			name:       "stop upstream ambiguous",
			facts:      normalizedImplementFacts{ScopeSlug: "target", MergeTargetPolicy: "feature/base", AllowRename: "yes"},
			obs:        implementBranchObservation{CurrentBranch: "implement/old", StartCommit: "abc123", Upstream: "origin/old"},
			wantAction: "stop",
			wantReason: "upstream/tracking",
		},
		{
			name:       "legacy implement-prefixed current branch is not misidentified as fresh start",
			facts:      normalizedImplementFacts{ScopeSlug: "target", MergeTargetPolicy: "feature/base", AllowRename: "no"},
			obs:        implementBranchObservation{CurrentBranch: "implement/old", StartCommit: "abc123"},
			wantAction: "stop",
			wantReason: "rename is not allowed",
		},
		{
			name:       "new impl-prefixed current branch is recognized as an implementation branch",
			facts:      normalizedImplementFacts{ScopeSlug: "target", MergeTargetPolicy: "feature/base", AllowRename: "yes"},
			obs:        implementBranchObservation{CurrentBranch: "impl/old", StartCommit: "abc123"},
			wantAction: "rename",
			wantReason: "rename is allowed",
		},
		{
			name:             "target branch name is not truncated when scope slug exceeds 15 characters",
			facts:            normalizedImplementFacts{ScopeSlug: "a-very-long-scope-slug-name", MergeTargetPolicy: "feature/base", AllowRename: "no"},
			obs:              implementBranchObservation{CurrentBranch: "feature/base", StartCommit: "abc123"},
			wantAction:       "create",
			wantReason:       "not an implementation branch",
			wantTargetBranch: "impl/feature/base/a-very-long-scope-slug-name",
		},
		{
			name:             "target branch name trims a trailing dash regardless of length",
			facts:            normalizedImplementFacts{ScopeSlug: "abc-defghijklm-nop-", MergeTargetPolicy: "feature/base", AllowRename: "no"},
			obs:              implementBranchObservation{CurrentBranch: "feature/base", StartCommit: "abc123"},
			wantAction:       "create",
			wantReason:       "not an implementation branch",
			wantTargetBranch: "impl/feature/base/abc-defghijklm-nop",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := deriveImplementBranchPlan(tc.facts, tc.obs)
			if got.Action != tc.wantAction {
				t.Fatalf("action = %q, want %q; plan=%+v", got.Action, tc.wantAction, got)
			}
			if !strings.Contains(got.Reason, tc.wantReason) {
				t.Fatalf("reason = %q, want containing %q", got.Reason, tc.wantReason)
			}
			if tc.wantTargetBranch != "" && got.TargetBranch != tc.wantTargetBranch {
				t.Fatalf("target branch = %q, want %q", got.TargetBranch, tc.wantTargetBranch)
			}
		})
	}
}

func TestResolveImplementMergeRootEncoding(t *testing.T) {
	t.Run("fresh create on non-main current branch encodes merge root", func(t *testing.T) {
		n := normalizedImplementFacts{ScopeSlug: "target"}
		obs := implementBranchObservation{CurrentBranch: "ws-dashboard-dev", StartCommit: "abc123"}
		got := deriveImplementBranchPlan(n, obs)
		if got.Action != "create" {
			t.Fatalf("action = %q, want create; plan=%+v", got.Action, got)
		}
		if got.TargetBranch != "impl/ws-dashboard-dev/target" {
			t.Fatalf("target branch = %q, want impl/ws-dashboard-dev/target", got.TargetBranch)
		}
		if got.MergeTarget != "ws-dashboard-dev" {
			t.Fatalf("merge target = %q, want ws-dashboard-dev", got.MergeTarget)
		}
	})

	t.Run("re-entry on name-rooted branch derives merge root from the branch name", func(t *testing.T) {
		n := normalizedImplementFacts{ScopeSlug: "target", AllowRename: "no"}
		obs := implementBranchObservation{CurrentBranch: "impl/ws-dashboard-dev/target", StartCommit: "abc123"}
		got := deriveImplementBranchPlan(n, obs)
		if got.Action != "continue" {
			t.Fatalf("action = %q, want continue; plan=%+v", got.Action, got)
		}
		if got.MergeTarget != "ws-dashboard-dev" {
			t.Fatalf("merge target = %q, want ws-dashboard-dev (derived from branch name)", got.MergeTarget)
		}
	})

	t.Run("diverging caller merge_target on a name-rooted branch does not silently win", func(t *testing.T) {
		n := normalizedImplementFacts{ScopeSlug: "target", MergeTargetPolicy: "main", AllowRename: "no"}
		obs := implementBranchObservation{CurrentBranch: "impl/ws-dashboard-dev/target", StartCommit: "abc123"}
		got := deriveImplementBranchPlan(n, obs)
		if got.MergeTarget != "ws-dashboard-dev" {
			t.Fatalf("merge target = %q, want name-root ws-dashboard-dev (caller value must not silently win)", got.MergeTarget)
		}
		wantWarning := `policy.branch.merge_target "main" ignored (implementation branch name encodes merge root "ws-dashboard-dev")`
		if !containsString(got.Warnings, wantWarning) {
			t.Fatalf("warnings missing name-root reconcile note: %v", got.Warnings)
		}
	})

	t.Run("slashed scope slug sanitizes to a single-segment stem on create", func(t *testing.T) {
		n := normalizedImplementFacts{ScopeSlug: "feature/evil"}
		obs := implementBranchObservation{CurrentBranch: "main", StartCommit: "abc123"}
		got := deriveImplementBranchPlan(n, obs)
		if got.Action != "create" {
			t.Fatalf("action = %q, want create; plan=%+v", got.Action, got)
		}
		if got.TargetBranch != "impl/main/feature-evil" {
			t.Fatalf("target branch = %q, want impl/main/feature-evil (no accidental extra /)", got.TargetBranch)
		}
	})

	t.Run("merge root ref conflict on create stops with a reason naming the conflict", func(t *testing.T) {
		n := normalizedImplementFacts{ScopeSlug: "target"}
		obs := implementBranchObservation{CurrentBranch: "ws-dashboard-dev", StartCommit: "abc123", MergeRootRefConflict: "impl/ws-dashboard-dev"}
		got := deriveImplementBranchPlan(n, obs)
		if got.Action != "stop" {
			t.Fatalf("action = %q, want stop; plan=%+v", got.Action, got)
		}
		if !strings.Contains(got.Reason, "impl/ws-dashboard-dev") {
			t.Fatalf("reason = %q, want it to name the conflicting ref", got.Reason)
		}
	})
}

func TestDeriveImplementBranchPlanMergeConfirmPassthrough(t *testing.T) {
	cases := []struct {
		name  string
		facts normalizedImplementFacts
		obs   implementBranchObservation
	}{
		{
			name:  "create action carries merge confirm",
			facts: normalizedImplementFacts{ScopeSlug: "target", MergeConfirmPolicy: "ask"},
			obs:   implementBranchObservation{CurrentBranch: "feature/base", StartCommit: "abc123"},
		},
		{
			name:  "continue action carries merge confirm",
			facts: normalizedImplementFacts{ScopeSlug: "target", MergeTargetPolicy: "feature/base", MergeConfirmPolicy: "skip"},
			obs:   implementBranchObservation{CurrentBranch: "impl/target", StartCommit: "abc123"},
		},
		{
			name:  "stop action carries merge confirm",
			facts: normalizedImplementFacts{ScopeSlug: "target", MergeConfirmPolicy: "skip"},
			obs:   implementBranchObservation{CurrentBranch: "implement/old", StartCommit: "abc123"},
		},
		{
			name:  "rename action carries merge confirm",
			facts: normalizedImplementFacts{ScopeSlug: "target", MergeTargetPolicy: "feature/base", AllowRename: "yes", MergeConfirmPolicy: "ask"},
			obs:   implementBranchObservation{CurrentBranch: "implement/old", StartCommit: "abc123"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := deriveImplementBranchPlan(tc.facts, tc.obs)
			if got.MergeConfirm != tc.facts.MergeConfirmPolicy {
				t.Fatalf("merge confirm = %q, want %q (verbatim passthrough regardless of action %q)", got.MergeConfirm, tc.facts.MergeConfirmPolicy, got.Action)
			}
		})
	}
}

func TestResolveImplementDocSkipAndWarnings(t *testing.T) {
	input := implementInput{
		Target: implementTargetInput{
			Kind:       "inline",
			Label:      "docs deferred",
			ScopeLabel: "docs deferred",
			ScopeSlug:  "docs-deferred",
		},
		Facts: implementFactsInput{
			Scope: implementScopeFactsInput{
				Span:            factString{Value: "multi-file", Present: true},
				Surface:         factString{Value: "internal", Present: true},
				NewPublicSymbol: factString{Value: "no", Present: true},
				NewTypeContract: factString{Value: "no", Present: true},
				TestSurface:     factString{Value: "existing", Present: true},
			},
		},
		Policy: implementPolicyInput{
			Docs: implementDocsPolicyInput{
				Mode:   factString{Value: "skip-with-reason", Present: true},
				Reason: factString{Value: "documentation tracked in follow-up", Present: true},
			},
		},
	}
	result := resolveImplement(input, factsFromTicket(input), implementBranchObservation{CurrentBranch: "feature/base", StartCommit: "abc123"})
	if result.Verdict.DocMode != "skipped" || result.Agenda.NeedDoc {
		t.Fatalf("doc mode = %q need_doc=%v, want skipped false", result.Verdict.DocMode, result.Agenda.NeedDoc)
	}
	if result.Agenda.DocReason != "documentation tracked in follow-up" {
		t.Fatalf("doc reason = %q", result.Agenda.DocReason)
	}
	if !containsString(result.Conditions, "doc-mode-policy=skip-with-reason") || !containsString(result.Conditions, "doc-reason=documentation tracked in follow-up") {
		t.Fatalf("conditions missing doc policy/reason: %v", result.Conditions)
	}
	if !strings.Contains(result.Raw, "- doc_reason: documentation tracked in follow-up") {
		t.Fatalf("raw missing doc reason:\n%s", result.Raw)
	}

	input.Policy.Docs.Reason = factString{}
	result = resolveImplement(input, factsFromTicket(input), implementBranchObservation{CurrentBranch: "feature/base", StartCommit: "abc123"})
	if !containsString(result.Warnings, "docs skip requested without reason; normalized to standard") {
		t.Fatalf("warnings missing doc fallback: %v", result.Warnings)
	}
	if result.Verdict.DocMode != "standard" || !result.Agenda.NeedDoc {
		t.Fatalf("doc fallback = %q need_doc=%v, want standard true", result.Verdict.DocMode, result.Agenda.NeedDoc)
	}
}

// ticketPhaseInput builds an implementInput for a ticket target with the
// given ticket stem and scope label, holding the facts/policy shape fixed so
// only the ticket_stem/scope_label vary between calls in the word-key stem
// tests below.
func ticketPhaseInput(ticketStem, scopeLabel string) implementInput {
	return implementInput{
		Target: implementTargetInput{Kind: "ticket", Label: "feature", ScopeLabel: scopeLabel, TicketStem: ticketStem},
		Facts: implementFactsInput{
			Scope: implementScopeFactsInput{
				Span:    factString{Value: "multi-file", Present: true},
				Surface: factString{Value: "public-interface", Present: true},
			},
		},
		Policy: implementPolicyInput{
			Branch: implementBranchPolicyInput{AllowRename: factString{Value: "yes", Present: true}},
		},
	}
}

// TestResolveImplementSameTicketStemAcrossPhasesContinues verifies the core
// ticket contract: entering route.resolve_implement for the same ticket across
// successive phases (same ticket_stem, differing scope_label) derives the
// same impl-branch stem, so the second phase's observation of the first
// phase's branch resolves to "continue" rather than "stop" or "rename".
func TestResolveImplementSameTicketStemAcrossPhasesContinues(t *testing.T) {
	const stem = "260900-feat-x"

	phase1 := ticketPhaseInput(stem, "Phase 1")
	result1 := resolveImplement(phase1, factsFromTicket(phase1), implementBranchObservation{CurrentBranch: "feature/base", StartCommit: "abc123"})
	if result1.Verdict.BranchPlan.Action != "create" {
		t.Fatalf("phase 1 action = %q, want create; plan=%+v", result1.Verdict.BranchPlan.Action, result1.Verdict.BranchPlan)
	}
	targetBranch := result1.Verdict.BranchPlan.TargetBranch
	if targetBranch == "" {
		t.Fatalf("phase 1 produced empty target branch; plan=%+v", result1.Verdict.BranchPlan)
	}

	phase2 := ticketPhaseInput(stem, "Phase 2")
	obs2 := implementBranchObservation{CurrentBranch: targetBranch, StartCommit: "abc123", AheadOfMergeRoot: 3}
	result2 := resolveImplement(phase2, factsFromTicket(phase2), obs2)
	if result2.Verdict.BranchPlan.Action != "continue" {
		t.Fatalf("phase 2 action = %q, want continue (same ticket_stem must resolve the same target branch %q); plan=%+v",
			result2.Verdict.BranchPlan.Action, targetBranch, result2.Verdict.BranchPlan)
	}
}

// TestResolveImplementDifferentTicketStemStillStops verifies the L1 safety
// stop still fires when a different ticket's target lands on a branch that
// already carries another ticket's unmerged work, even though both branches
// share the derivation mechanism.
func TestResolveImplementDifferentTicketStemStillStops(t *testing.T) {
	phase1 := ticketPhaseInput("260900-feat-x", "Phase 1")
	result1 := resolveImplement(phase1, factsFromTicket(phase1), implementBranchObservation{CurrentBranch: "feature/base", StartCommit: "abc123"})
	if result1.Verdict.BranchPlan.Action != "create" {
		t.Fatalf("setup action = %q, want create; plan=%+v", result1.Verdict.BranchPlan.Action, result1.Verdict.BranchPlan)
	}
	occupiedBranch := result1.Verdict.BranchPlan.TargetBranch

	other := ticketPhaseInput("260900-feat-y", "Phase 1")
	obs := implementBranchObservation{CurrentBranch: occupiedBranch, StartCommit: "abc123", AheadOfMergeRoot: 2}
	result := resolveImplement(other, factsFromTicket(other), obs)
	if result.Verdict.BranchPlan.Action != "stop" {
		t.Fatalf("different ticket_stem action = %q, want stop even with allow_rename=yes; plan=%+v",
			result.Verdict.BranchPlan.Action, result.Verdict.BranchPlan)
	}
}

// TestNormalizeImplementFactsTicketStemOverridesCallerScopeSlug verifies that
// a non-empty target.ticket_stem is authoritative over a caller-supplied
// target.scope_slug, not just the empty-scope_slug fallback: the derived
// word-key wins and a warning explains the override.
func TestNormalizeImplementFactsTicketStemOverridesCallerScopeSlug(t *testing.T) {
	input := implementInput{
		Target: implementTargetInput{Kind: "ticket", TicketStem: "x", ScopeSlug: "caller-supplied"},
	}
	n, warnings := normalizeImplementFacts(input)
	want := wskey.Derive("x", 3)
	if n.ScopeSlug != want {
		t.Fatalf("ScopeSlug = %q, want derived word-key %q", n.ScopeSlug, want)
	}
	if n.ScopeSlug == "caller-supplied" {
		t.Fatalf("ScopeSlug retained caller-supplied value %q, want it overridden", n.ScopeSlug)
	}
	if !containsString(warnings, "target.scope_slug ignored for ticket target; branch stem derived deterministically from ticket_stem") {
		t.Fatalf("warnings missing scope_slug-override notice: %v", warnings)
	}
}

// TestNormalizeImplementFactsInlineUnaffected verifies inline targets (no
// ticket_stem) keep the existing slugifyImplementScope label-derived path,
// both when scope_slug is caller-supplied and when it is missing.
func TestNormalizeImplementFactsInlineUnaffected(t *testing.T) {
	t.Run("caller-supplied scope_slug is preserved", func(t *testing.T) {
		input := implementInput{
			Target: implementTargetInput{Kind: "inline", Label: "tiny edit", ScopeLabel: "tiny edit", ScopeSlug: "tiny-edit"},
		}
		n, warnings := normalizeImplementFacts(input)
		if n.ScopeSlug != "tiny-edit" {
			t.Fatalf("ScopeSlug = %q, want unchanged caller-supplied %q", n.ScopeSlug, "tiny-edit")
		}
		if containsString(warnings, "target.scope_slug missing; derived from target label") {
			t.Fatalf("unexpected missing-scope_slug warning: %v", warnings)
		}
	})

	t.Run("missing scope_slug falls back to label slug", func(t *testing.T) {
		input := implementInput{
			Target: implementTargetInput{Kind: "inline", Label: "tiny edit", ScopeLabel: "tiny edit"},
		}
		n, warnings := normalizeImplementFacts(input)
		want := slugifyImplementScope("tiny edit")
		if n.ScopeSlug != want {
			t.Fatalf("ScopeSlug = %q, want label-derived %q", n.ScopeSlug, want)
		}
		if !containsString(warnings, "target.scope_slug missing; derived from target label") {
			t.Fatalf("warnings missing label-fallback notice: %v", warnings)
		}
	})
}
