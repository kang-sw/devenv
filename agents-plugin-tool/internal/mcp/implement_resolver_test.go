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
	for _, want := range []string{"path.generate", "plan-populator-survey", "light plan", "PlanPath"} {
		if !strings.Contains(result.NextInstruction, want) {
			t.Fatalf("delegated next instruction missing %q: %q", want, result.NextInstruction)
		}
	}
	if strings.Contains(result.Raw, "brief") {
		t.Fatalf("delegated raw exposed old brief path:\n%s", result.Raw)
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
	for _, want := range []string{"[escalate-to-research]", "low confidence", "strategic uncertainty", "plan-populator-research"} {
		if !strings.Contains(result.NextInstruction, want) {
			t.Fatalf("survey escalation next instruction missing %q: %q", want, result.NextInstruction)
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
	wantWarning := `policy.branch.merge_target "master" ignored (not on an implement/* branch); derived from current branch "test/wsflow-smoke"`
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
			wantTargetBranch: "impl/target",
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
			name:             "target branch name is truncated to 15 characters",
			facts:            normalizedImplementFacts{ScopeSlug: "a-very-long-scope-slug-name", MergeTargetPolicy: "feature/base", AllowRename: "no"},
			obs:              implementBranchObservation{CurrentBranch: "feature/base", StartCommit: "abc123"},
			wantAction:       "create",
			wantReason:       "not an implementation branch",
			wantTargetBranch: "impl/a-very-long-sco",
		},
		{
			name:             "target branch truncation trims trailing dash",
			facts:            normalizedImplementFacts{ScopeSlug: "abc-defghijklm-nop", MergeTargetPolicy: "feature/base", AllowRename: "no"},
			obs:              implementBranchObservation{CurrentBranch: "feature/base", StartCommit: "abc123"},
			wantAction:       "create",
			wantReason:       "not an implementation branch",
			wantTargetBranch: "impl/abc-defghijklm",
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
