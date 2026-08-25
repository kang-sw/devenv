package mcp

import (
	"strings"
	"testing"
)

func TestResolveImplementStrategyRules(t *testing.T) {
	input := implementInput{
		Target: implementTargetInput{Kind: "inline", Label: "tiny edit", ScopeLabel: "tiny edit", ScopeSlug: "tiny-edit"},
		Facts: implementFactsInput{
			Scope: implementScopeFactsInput{
				Span:                      factString{Value: "single-file", Present: true},
				Surface:                   factString{Value: "internal", Present: true},
				NewPublicSymbol:           factString{Value: "no", Present: true},
				NewTypeContract:           factString{Value: "no", Present: true},
				TestSurface:               factString{Value: "none", Present: true},
				ExplicitDelegationRequest: factString{Value: "no", Present: true},
			},
			Complexity: implementComplexityFactsInput{
				ChangePoints:   factString{Value: "clear", Present: true},
				ReusePoints:    factString{Value: "not-applicable", Present: true},
				StrategyShape:  factString{Value: "single-obvious", Present: true},
				SideEffectRisk: factString{Value: "low", Present: true},
				ColdContext:    factString{Value: "no", Present: true},
			},
			Risk: implementRiskFactsInput{
				Correctness:        factString{Value: "low", Present: true},
				Fit:                factString{Value: "low", Present: true},
				Test:               factString{Value: "low", Present: true},
				SecurityOrContract: factString{Value: "low", Present: true},
			},
		},
	}
	result := resolveImplement(input, implementBranchObservation{CurrentBranch: "feature/demo", StartCommit: "abc123"})
	if result.Verdict.Delegation != "direct-edit" {
		t.Fatalf("delegation = %q, want direct-edit", result.Verdict.Delegation)
	}
	if result.Verdict.PlanDepth != "none" {
		t.Fatalf("plan depth = %q, want none", result.Verdict.PlanDepth)
	}
	if result.Verdict.ReviewAlloc != "lead-only" || result.Verdict.NeedReview {
		t.Fatalf("review = %q need=%v, want lead-only false", result.Verdict.ReviewAlloc, result.Verdict.NeedReview)
	}
	if strings.Contains(result.NextInstruction, "plan-populator") || strings.Contains(result.NextInstruction, "path.generate") {
		t.Fatalf("direct-edit next instruction mentioned planner actions: %q", result.NextInstruction)
	}
	if strings.Contains(result.Raw, "Plan Depth: brief") {
		t.Fatalf("direct-edit raw exposed brief plan depth:\n%s", result.Raw)
	}
}

func TestResolveImplementCurrentBranchCompletion(t *testing.T) {
	input := lowCeremonyImplementInput()
	result := resolveImplement(input, implementBranchObservation{CurrentBranch: "feature/demo", StartCommit: "abc123"})
	plan := result.Verdict.BranchPlan
	if plan.Action != "current" || plan.CurrentBranch != "feature/demo" {
		t.Fatalf("branch plan = %+v, want retained feature/demo", plan)
	}
	if plan.TargetBranch != "" || plan.MergeTarget != "" {
		t.Fatalf("current-branch plan retained merge metadata: %+v", plan)
	}
	if !strings.Contains(result.NextInstruction, "Keep the current branch feature/demo") || !strings.Contains(result.NextInstruction, "omit merge work") {
		t.Fatalf("next instruction missing current-branch completion: %q", result.NextInstruction)
	}
	if !strings.Contains(result.Raw, "Merge Confirm: n/a") || !strings.Contains(result.Raw, "- merge_confirm: n/a") {
		t.Fatalf("current-branch text did not mark merge confirmation inapplicable:\n%s", result.Raw)
	}
	if !containsString(result.Conditions, "merge-confirm=n/a") || containsString(result.Conditions, "merge-confirm=ask") {
		t.Fatalf("current-branch conditions retained applicable merge confirmation: %v", result.Conditions)
	}
	if !containsString(result.Conditions, "low-ceremony-if-safe=yes") {
		t.Fatalf("current-branch conditions omitted normalized low-ceremony preference: %v", result.Conditions)
	}
}

func TestResolveImplementCurrentBranchPreferenceGate(t *testing.T) {
	for _, tc := range []struct {
		name    string
		policy  factString
		wantVal string
	}{
		{name: "no", policy: factString{Value: "no", Present: true}, wantVal: "no"},
		{name: "unknown", policy: factString{Value: "unknown", Present: true}, wantVal: "unknown"},
		{name: "missing", wantVal: "unknown"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := lowCeremonyImplementInput()
			input.Policy.LowCeremonyIfSafe = tc.policy
			result := resolveImplement(input, implementBranchObservation{CurrentBranch: "feature/demo", StartCommit: "abc123"})
			if result.Verdict.BranchPlan.Action != "create" {
				t.Fatalf("branch action = %q, want standard create", result.Verdict.BranchPlan.Action)
			}
			if !containsString(result.Conditions, "low-ceremony-if-safe="+tc.wantVal) {
				t.Fatalf("conditions omitted normalized preference %q: %v", tc.wantVal, result.Conditions)
			}
			if containsString(result.Warnings, "policy.low_ceremony_if_safe=yes not applicable; continuing with standard branch path") {
				t.Fatalf("non-yes preference emitted rejected-request warning: %v", result.Warnings)
			}
		})
	}
}

func TestResolveImplementRejectedLowCeremonyPreferenceWarnsWithoutChangingVerdicts(t *testing.T) {
	input := lowCeremonyImplementInput()
	input.Facts.Scope.Span = factString{Value: "multi-file", Present: true}
	input.Facts.Scope.Surface = factString{Value: "cross-module", Present: true}
	input.Facts.Scope.TestSurface = factString{Value: "existing", Present: true}
	input.Facts.Risk.Correctness = factString{Value: "moderate", Present: true}
	input.Facts.Risk.Fit = factString{Value: "moderate", Present: true}
	input.Facts.Risk.Test = factString{Value: "moderate", Present: true}

	result := resolveImplement(input, implementBranchObservation{CurrentBranch: "feature/demo", StartCommit: "abc123"})
	if result.Verdict.BranchPlan.Action != "create" || result.Verdict.Delegation != "delegated" || result.Verdict.ReviewAlloc != "partitioned: correctness, fit, test" || result.Verdict.DocMode != "skipped" {
		t.Fatalf("rejected preference changed independent verdicts: %+v", result.Verdict)
	}
	warning := "policy.low_ceremony_if_safe=yes not applicable; continuing with standard branch path"
	if !containsString(result.Warnings, warning) || !containsString(result.Agenda.Warnings, warning) || !strings.Contains(result.Raw, warning) {
		t.Fatalf("rejected preference warning missing from result, agenda, or raw output: %+v", result)
	}
}

func TestResolveImplementCurrentBranchCompletionNearMisses(t *testing.T) {
	base := normalizedImplementFacts{
		Span: "single-file", Surface: "internal", NewPublicSymbol: "no", NewTypeContract: "no", TestSurface: "none",
		ExplicitDelegationRequest: "no", CorrectnessRisk: "low", FitRisk: "low", TestRisk: "low", SecurityOrContractRisk: "low",
		LowCeremonyIfSafe: "yes", ReviewOverride: "auto", DocModePolicy: "skip-with-reason", DocReason: "docs unaffected",
	}
	baseObs := implementBranchObservation{CurrentBranch: "feature/demo", StartCommit: "abc123"}
	cases := []struct {
		name       string
		kind       string
		facts      normalizedImplementFacts
		obs        implementBranchObservation
		wantAction string
	}{
		{name: "ticket target", kind: "ticket", facts: base, obs: baseObs, wantAction: "create"},
		{name: "impl branch", kind: "inline", facts: base, obs: implementBranchObservation{CurrentBranch: "impl/demo"}, wantAction: "stop"},
		{name: "legacy implement branch", kind: "inline", facts: base, obs: implementBranchObservation{CurrentBranch: "implement/demo"}, wantAction: "stop"},
		{name: "detached head", kind: "inline", facts: base, obs: implementBranchObservation{CurrentBranch: "(detached)"}, wantAction: "stop"},
		{name: "empty head", kind: "inline", facts: base, obs: implementBranchObservation{}, wantAction: "stop"},
		{name: "empty start commit", kind: "inline", facts: base, obs: implementBranchObservation{CurrentBranch: "feature/demo"}, wantAction: "create"},
		{name: "git unborn marker", kind: "inline", facts: base, obs: implementBranchObservation{CurrentBranch: "feature/demo", StartCommit: "(initial)"}, wantAction: "create"},
		{name: "explicit delegation", kind: "inline", facts: mutateLowCeremonyFacts(base, func(n *normalizedImplementFacts) { n.ExplicitDelegationRequest = "yes" }), obs: baseObs},
		{name: "lead-only override", kind: "inline", facts: mutateLowCeremonyFacts(base, func(n *normalizedImplementFacts) { n.ReviewOverride = "lead-only" }), obs: baseObs},
		{name: "missing docs reason", kind: "inline", facts: mutateLowCeremonyFacts(base, func(n *normalizedImplementFacts) { n.DocReason = "" }), obs: baseObs},
		{name: "standard docs", kind: "inline", facts: mutateLowCeremonyFacts(base, func(n *normalizedImplementFacts) { n.DocModePolicy = "standard" }), obs: baseObs},
		{name: "span unknown", kind: "inline", facts: mutateLowCeremonyFacts(base, func(n *normalizedImplementFacts) { n.Span = "unknown" }), obs: baseObs},
		{name: "span failed", kind: "inline", facts: mutateLowCeremonyFacts(base, func(n *normalizedImplementFacts) { n.Span = "multi-file" }), obs: baseObs},
		{name: "surface unknown", kind: "inline", facts: mutateLowCeremonyFacts(base, func(n *normalizedImplementFacts) { n.Surface = "unknown" }), obs: baseObs},
		{name: "surface failed", kind: "inline", facts: mutateLowCeremonyFacts(base, func(n *normalizedImplementFacts) { n.Surface = "cross-module" }), obs: baseObs},
		{name: "public symbol unknown", kind: "inline", facts: mutateLowCeremonyFacts(base, func(n *normalizedImplementFacts) { n.NewPublicSymbol = "unknown" }), obs: baseObs},
		{name: "public symbol failed", kind: "inline", facts: mutateLowCeremonyFacts(base, func(n *normalizedImplementFacts) { n.NewPublicSymbol = "yes" }), obs: baseObs},
		{name: "type contract unknown", kind: "inline", facts: mutateLowCeremonyFacts(base, func(n *normalizedImplementFacts) { n.NewTypeContract = "unknown" }), obs: baseObs},
		{name: "type contract failed", kind: "inline", facts: mutateLowCeremonyFacts(base, func(n *normalizedImplementFacts) { n.NewTypeContract = "yes" }), obs: baseObs},
		{name: "test surface unknown", kind: "inline", facts: mutateLowCeremonyFacts(base, func(n *normalizedImplementFacts) { n.TestSurface = "unknown" }), obs: baseObs},
		{name: "test surface failed", kind: "inline", facts: mutateLowCeremonyFacts(base, func(n *normalizedImplementFacts) { n.TestSurface = "new-files" }), obs: baseObs},
		{name: "correctness unknown", kind: "inline", facts: mutateLowCeremonyFacts(base, func(n *normalizedImplementFacts) { n.CorrectnessRisk = "unknown" }), obs: baseObs},
		{name: "correctness failed", kind: "inline", facts: mutateLowCeremonyFacts(base, func(n *normalizedImplementFacts) { n.CorrectnessRisk = "moderate" }), obs: baseObs},
		{name: "fit unknown", kind: "inline", facts: mutateLowCeremonyFacts(base, func(n *normalizedImplementFacts) { n.FitRisk = "unknown" }), obs: baseObs},
		{name: "fit failed", kind: "inline", facts: mutateLowCeremonyFacts(base, func(n *normalizedImplementFacts) { n.FitRisk = "moderate" }), obs: baseObs},
		{name: "test risk unknown", kind: "inline", facts: mutateLowCeremonyFacts(base, func(n *normalizedImplementFacts) { n.TestRisk = "unknown" }), obs: baseObs},
		{name: "test risk failed", kind: "inline", facts: mutateLowCeremonyFacts(base, func(n *normalizedImplementFacts) { n.TestRisk = "moderate" }), obs: baseObs},
		{name: "contract risk unknown", kind: "inline", facts: mutateLowCeremonyFacts(base, func(n *normalizedImplementFacts) { n.SecurityOrContractRisk = "unknown" }), obs: baseObs},
		{name: "contract risk failed", kind: "inline", facts: mutateLowCeremonyFacts(base, func(n *normalizedImplementFacts) { n.SecurityOrContractRisk = "moderate" }), obs: baseObs},
		{name: "explicit direct override cannot rescue unsafe scope", kind: "inline", facts: mutateLowCeremonyFacts(base, func(n *normalizedImplementFacts) { n.ExplicitDirectEditRequest = "yes"; n.Span = "multi-file" }), obs: baseObs},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := deriveResolvedImplementBranchPlan(tc.kind, tc.facts, tc.obs)
			wantAction := tc.wantAction
			if wantAction == "" {
				wantAction = "create"
			}
			if got.Action != wantAction {
				t.Fatalf("near miss branch action = %q, want unchanged standard action %q; plan=%+v", got.Action, wantAction, got)
			}
		})
	}
}

func lowCeremonyImplementInput() implementInput {
	return implementInput{
		Target: implementTargetInput{Kind: "inline", Label: "tiny edit", ScopeLabel: "tiny edit", ScopeSlug: "tiny-edit"},
		Facts: implementFactsInput{
			Scope: implementScopeFactsInput{
				Span: factString{Value: "single-file", Present: true}, Surface: factString{Value: "internal", Present: true},
				NewPublicSymbol: factString{Value: "no", Present: true}, NewTypeContract: factString{Value: "no", Present: true},
				TestSurface: factString{Value: "none", Present: true}, ExplicitDelegationRequest: factString{Value: "no", Present: true},
			},
			Risk: implementRiskFactsInput{
				Correctness: factString{Value: "low", Present: true}, Fit: factString{Value: "low", Present: true},
				Test: factString{Value: "low", Present: true}, SecurityOrContract: factString{Value: "low", Present: true},
			},
		},
		Policy: implementPolicyInput{
			LowCeremonyIfSafe: factString{Value: "yes", Present: true},
			Review:            implementReviewPolicyInput{Override: factString{Value: "auto", Present: true}},
			Docs:              implementDocsPolicyInput{Mode: factString{Value: "skip-with-reason", Present: true}, Reason: factString{Value: "docs unaffected", Present: true}},
		},
	}
}

func mutateLowCeremonyFacts(base normalizedImplementFacts, mutate func(*normalizedImplementFacts)) normalizedImplementFacts {
	mutate(&base)
	return base
}

func TestResolveImplementDelegatedDefaultsToSurveyPlan(t *testing.T) {
	input := implementInput{
		Target: implementTargetInput{Kind: "ticket", Label: "feature", TicketPath: "ai-docs/tickets/ready/feature.md", ScopeLabel: "Phase 1", ScopeSlug: "feature"},
		Facts: implementFactsInput{
			Scope: implementScopeFactsInput{
				Span:                      factString{Value: "multi-file", Present: true},
				Surface:                   factString{Value: "public-interface", Present: true},
				NewPublicSymbol:           factString{Value: "no", Present: true},
				NewTypeContract:           factString{Value: "no", Present: true},
				TestSurface:               factString{Value: "existing", Present: true},
				ExplicitDelegationRequest: factString{Value: "no", Present: true},
			},
			Complexity: implementComplexityFactsInput{
				ChangePoints:   factString{Value: "clear", Present: true},
				ReusePoints:    factString{Value: "confirmed", Present: true},
				StrategyShape:  factString{Value: "single-obvious", Present: true},
				SideEffectRisk: factString{Value: "moderate", Present: true},
				ColdContext:    factString{Value: "no", Present: true},
			},
			Risk: implementRiskFactsInput{
				Correctness:        factString{Value: "moderate", Present: true},
				Fit:                factString{Value: "moderate", Present: true},
				Test:               factString{Value: "moderate", Present: true},
				SecurityOrContract: factString{Value: "moderate", Present: true},
			},
		},
	}
	result := resolveImplement(input, implementBranchObservation{CurrentBranch: "feature/base", StartCommit: "abc123"})
	if result.Verdict.Delegation != "delegated" {
		t.Fatalf("delegation = %q, want delegated", result.Verdict.Delegation)
	}
	if result.Verdict.PlanDepth != "survey" {
		t.Fatalf("plan depth = %q, want survey", result.Verdict.PlanDepth)
	}
	for _, want := range []string{"installed delegated Prep and Edit todos", "partitioned: correctness, fit, test review", "standard documentation gates"} {
		if !strings.Contains(result.NextInstruction, want) {
			t.Fatalf("delegated next instruction missing %q: %q", want, result.NextInstruction)
		}
	}
	if strings.Contains(result.Raw, "brief") {
		t.Fatalf("delegated raw exposed old brief path:\n%s", result.Raw)
	}
}

func TestResolveImplementInlineDelegatedNextDefersPlannerAuthorityToPrep(t *testing.T) {
	input := implementInput{
		Target: implementTargetInput{Kind: "inline", Label: "bounded multi-file edit", ScopeLabel: "bounded edit", ScopeSlug: "bounded-edit"},
		Facts: implementFactsInput{Scope: implementScopeFactsInput{
			Span: factString{Value: "multi-file", Present: true}, Surface: factString{Value: "internal", Present: true},
			TestSurface: factString{Value: "existing", Present: true}, ExplicitDelegationRequest: factString{Value: "no", Present: true},
		}},
	}
	result := resolveImplement(input, implementBranchObservation{CurrentBranch: "feature/base", StartCommit: "abc123"})
	for _, want := range []string{"installed delegated Prep and Edit todos", result.Verdict.ReviewAlloc + " review", "standard documentation gates"} {
		if !strings.Contains(result.NextInstruction, want) {
			t.Fatalf("inline delegated next instruction missing %q: %q", want, result.NextInstruction)
		}
	}
	for _, forbidden := range []string{"target_kind", "ticket_path", "selected_phase", "inline_contract", "plan-populator"} {
		if strings.Contains(result.NextInstruction, forbidden) {
			t.Fatalf("inline delegated next instruction duplicates Prep detail %q: %q", forbidden, result.NextInstruction)
		}
	}
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
			if got := deriveImplementReviewAlloc(tc.facts, "delegated"); got != tc.want {
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

func TestAutomaticLeadOnlyReviewEligibleRequiresGenuineLow(t *testing.T) {
	genuineLow := normalizedImplementFacts{
		CorrectnessRisk: "low", FitRisk: "low", TestRisk: "low", SecurityOrContractRisk: "low",
	}
	if !automaticLeadOnlyReviewEligible(genuineLow, "direct-edit") {
		t.Fatalf("genuine all-low fact set should qualify for lead-only review")
	}

	allUnknown := normalizedImplementFacts{
		CorrectnessRisk: "unknown", FitRisk: "unknown", TestRisk: "unknown", SecurityOrContractRisk: "unknown",
	}
	if automaticLeadOnlyReviewEligible(allUnknown, "direct-edit") {
		t.Fatalf("all-unknown fact set must not qualify for lead-only review")
	}
	if got := deriveImplementReviewAlloc(allUnknown, "direct-edit"); got != "single" {
		t.Fatalf("all-unknown review alloc = %q, want single (not lead-only, not partitioned)", got)
	}
}

func TestResolveImplementExplicitDirectEditOverridesMultiFileScope(t *testing.T) {
	// explicit_direct_edit_request=yes overrides all other scope facts to direct-edit.
	input := implementInput{
		Target: implementTargetInput{Kind: "inline", Label: "force direct", ScopeLabel: "force direct", ScopeSlug: "force-direct"},
		Facts: implementFactsInput{
			Scope: implementScopeFactsInput{
				Span:                      factString{Value: "multi-file", Present: true},
				Surface:                   factString{Value: "cross-module", Present: true},
				NewPublicSymbol:           factString{Value: "yes", Present: true},
				ExplicitDirectEditRequest: factString{Value: "yes", Present: true},
			},
		},
	}
	result := resolveImplement(input, implementBranchObservation{CurrentBranch: "feature/demo", StartCommit: "abc123"})
	if result.Verdict.Delegation != "direct-edit" {
		t.Fatalf("delegation = %q, want direct-edit when explicit_direct_edit_request=yes overrides scope", result.Verdict.Delegation)
	}
	if !containsString(result.Conditions, "explicit-direct-edit-request=yes") {
		t.Fatalf("conditions missing explicit-direct-edit-request=yes: %v", result.Conditions)
	}
}

func TestResolveImplementSurveyEscalatesResearchFromSurveySignal(t *testing.T) {
	input := implementInput{
		Target: implementTargetInput{Kind: "ticket", Label: "risky feature", ScopeLabel: "Phase 2", ScopeSlug: "risky-feature"},
		Facts: implementFactsInput{
			Scope: implementScopeFactsInput{
				Span:                      factString{Value: "multi-file", Present: true},
				Surface:                   factString{Value: "cross-module", Present: true},
				TestSurface:               factString{Value: "new-files", Present: true},
				ExplicitDelegationRequest: factString{Value: "yes", Present: true},
			},
			Complexity: implementComplexityFactsInput{
				StrategyShape:  factString{Value: "multiple-viable", Present: true},
				SideEffectRisk: factString{Value: "high", Present: true},
				ReusePoints:    factString{Value: "unconfirmed", Present: true},
				ColdContext:    factString{Value: "yes", Present: true},
			},
		},
	}
	result := resolveImplement(input, implementBranchObservation{CurrentBranch: "feature/base", StartCommit: "abc123"})
	if result.Verdict.PlanDepth != "survey" {
		t.Fatalf("plan depth = %q, want survey even for risky delegated prep", result.Verdict.PlanDepth)
	}
	if !strings.Contains(result.NextInstruction, "installed delegated Prep and Edit todos") {
		t.Fatalf("delegated next instruction does not route to installed todos: %q", result.NextInstruction)
	}
	prep := implementPrepInstruction(implementTodoVerdict{
		TargetKind:  "ticket",
		Delegation:  result.Verdict.Delegation,
		BranchPlan:  result.Verdict.BranchPlan,
		PlanDepth:   result.Verdict.PlanDepth,
		ReviewAlloc: result.Verdict.ReviewAlloc,
		DocMode:     result.Verdict.DocMode,
	})
	for _, want := range []string{"[escalate-to-research]", "low confidence", "strategic uncertainty", "plan-populator-research"} {
		if !strings.Contains(prep, want) {
			t.Fatalf("survey escalation Prep instruction missing %q: %q", want, prep)
		}
	}
	if strings.Contains(result.Raw, "Plan Depth: research") {
		t.Fatalf("resolver preselected research instead of survey escalation:\n%s", result.Raw)
	}
}

func TestResolveImplementBranchStopOmitsPlannerInstructions(t *testing.T) {
	input := implementInput{
		Target: implementTargetInput{Kind: "ticket", Label: "feature", ScopeLabel: "Phase 1", ScopeSlug: "feature"},
		Facts: implementFactsInput{
			Scope: implementScopeFactsInput{
				Span:                      factString{Value: "multi-file", Present: true},
				Surface:                   factString{Value: "public-interface", Present: true},
				ExplicitDelegationRequest: factString{Value: "yes", Present: true},
			},
		},
		Policy: implementPolicyInput{
			Branch: implementBranchPolicyInput{MergeTarget: factString{Value: "main", Present: true}, AllowRename: factString{Value: "no", Present: true}},
		},
	}
	result := resolveImplement(input, implementBranchObservation{CurrentBranch: "implement/old", StartCommit: "abc123"})
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
				Span:                      factString{Value: "multi-file", Present: true},
				Surface:                   factString{Value: "public-interface", Present: true},
				ExplicitDelegationRequest: factString{Value: "yes", Present: true},
			},
		},
		Policy: implementPolicyInput{
			Branch: implementBranchPolicyInput{MergeTarget: factString{Value: "main", Present: true}},
		},
	}
	result := resolveImplement(input, implementBranchObservation{CurrentBranch: "impl/old", StartCommit: "abc123"})
	if result.Verdict.BranchPlan.Action != "rename" {
		t.Fatalf("branch action = %q, want rename (allow_rename absent should default to yes)", result.Verdict.BranchPlan.Action)
	}
}

func TestResolveImplementMergeConfirmDefaultsToAskWhenUnset(t *testing.T) {
	input := implementInput{
		Target: implementTargetInput{Kind: "ticket", Label: "feature", ScopeLabel: "Phase 1", ScopeSlug: "feature"},
		Facts: implementFactsInput{
			Scope: implementScopeFactsInput{
				Span:                      factString{Value: "multi-file", Present: true},
				Surface:                   factString{Value: "public-interface", Present: true},
				ExplicitDelegationRequest: factString{Value: "yes", Present: true},
			},
		},
		Policy: implementPolicyInput{
			Branch: implementBranchPolicyInput{MergeTarget: factString{Value: "main", Present: true}},
		},
	}
	result := resolveImplement(input, implementBranchObservation{CurrentBranch: "impl/old", StartCommit: "abc123"})
	if result.Verdict.BranchPlan.MergeConfirm != "ask" {
		t.Fatalf("merge confirm = %q, want ask (absent should default to ask)", result.Verdict.BranchPlan.MergeConfirm)
	}
}

func TestResolveImplementMergeConfirmSkipHonored(t *testing.T) {
	input := implementInput{
		Target: implementTargetInput{Kind: "ticket", Label: "feature", ScopeLabel: "Phase 1", ScopeSlug: "feature"},
		Facts: implementFactsInput{
			Scope: implementScopeFactsInput{
				Span:                      factString{Value: "multi-file", Present: true},
				Surface:                   factString{Value: "public-interface", Present: true},
				ExplicitDelegationRequest: factString{Value: "yes", Present: true},
			},
		},
		Policy: implementPolicyInput{
			Branch: implementBranchPolicyInput{
				MergeTarget:  factString{Value: "main", Present: true},
				MergeConfirm: factString{Value: "skip", Present: true},
			},
		},
	}
	result := resolveImplement(input, implementBranchObservation{CurrentBranch: "impl/old", StartCommit: "abc123"})
	if result.Verdict.BranchPlan.MergeConfirm != "skip" {
		t.Fatalf("merge confirm = %q, want skip (explicit skip should be honored)", result.Verdict.BranchPlan.MergeConfirm)
	}
}

func TestResolveImplementMergeConfirmNonSkipStillAsks(t *testing.T) {
	input := implementInput{
		Target: implementTargetInput{Kind: "ticket", Label: "feature", ScopeLabel: "Phase 1", ScopeSlug: "feature"},
		Facts: implementFactsInput{
			Scope: implementScopeFactsInput{
				Span:                      factString{Value: "multi-file", Present: true},
				Surface:                   factString{Value: "public-interface", Present: true},
				ExplicitDelegationRequest: factString{Value: "yes", Present: true},
			},
		},
		Policy: implementPolicyInput{
			Branch: implementBranchPolicyInput{
				MergeTarget:  factString{Value: "main", Present: true},
				MergeConfirm: factString{Value: "ask", Present: true},
			},
		},
	}
	result := resolveImplement(input, implementBranchObservation{CurrentBranch: "impl/old", StartCommit: "abc123"})
	if result.Verdict.BranchPlan.MergeConfirm != "ask" {
		t.Fatalf("merge confirm = %q, want ask (explicit non-skip value should still ask)", result.Verdict.BranchPlan.MergeConfirm)
	}
}

func TestResolveImplementMergeTargetPolicyIgnoredOutsideImplementBranchWarns(t *testing.T) {
	input := implementInput{
		Target: implementTargetInput{Kind: "inline", Label: "tiny edit", ScopeLabel: "tiny edit", ScopeSlug: "tiny-edit"},
		Facts: implementFactsInput{
			Scope: implementScopeFactsInput{
				Span:                      factString{Value: "single-file", Present: true},
				Surface:                   factString{Value: "internal", Present: true},
				NewPublicSymbol:           factString{Value: "no", Present: true},
				NewTypeContract:           factString{Value: "no", Present: true},
				TestSurface:               factString{Value: "none", Present: true},
				ExplicitDelegationRequest: factString{Value: "no", Present: true},
			},
		},
		Policy: implementPolicyInput{
			Branch: implementBranchPolicyInput{MergeTarget: factString{Value: "master", Present: true}},
		},
	}
	result := resolveImplement(input, implementBranchObservation{CurrentBranch: "test/wsflow-smoke", StartCommit: "abc123"})
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
				Span:                      factString{Value: "single-file", Present: true},
				Surface:                   factString{Value: "internal", Present: true},
				NewPublicSymbol:           factString{Value: "no", Present: true},
				NewTypeContract:           factString{Value: "no", Present: true},
				TestSurface:               factString{Value: "none", Present: true},
				ExplicitDelegationRequest: factString{Value: "no", Present: true},
			},
		},
		Policy: implementPolicyInput{
			Branch: implementBranchPolicyInput{MergeTarget: factString{Value: "master", Present: true}},
		},
	}
	result := resolveImplement(input, implementBranchObservation{CurrentBranch: "impl/tiny-edit", StartCommit: "abc123"})
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
				Span:                      factString{Value: "multi-file", Present: true},
				Surface:                   factString{Value: "internal", Present: true},
				NewPublicSymbol:           factString{Value: "no", Present: true},
				NewTypeContract:           factString{Value: "no", Present: true},
				TestSurface:               factString{Value: "existing", Present: true},
				ExplicitDelegationRequest: factString{Value: "no", Present: true},
			},
		},
		Policy: implementPolicyInput{
			Docs: implementDocsPolicyInput{
				Mode:   factString{Value: "skip-with-reason", Present: true},
				Reason: factString{Value: "documentation tracked in follow-up", Present: true},
			},
		},
	}
	result := resolveImplement(input, implementBranchObservation{CurrentBranch: "feature/base", StartCommit: "abc123"})
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
	result = resolveImplement(input, implementBranchObservation{CurrentBranch: "feature/base", StartCommit: "abc123"})
	if !containsString(result.Warnings, "docs skip requested without reason; normalized to standard") {
		t.Fatalf("warnings missing doc fallback: %v", result.Warnings)
	}
	if result.Verdict.DocMode != "standard" || !result.Agenda.NeedDoc {
		t.Fatalf("doc fallback = %q need_doc=%v, want standard true", result.Verdict.DocMode, result.Agenda.NeedDoc)
	}
}
