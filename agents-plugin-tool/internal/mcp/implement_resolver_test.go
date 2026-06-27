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
}

func TestResolveImplementBranchPlanRules(t *testing.T) {
	base := normalizedImplementFacts{ScopeSlug: "target", MergeTargetPolicy: "feature/base", AllowRename: "no"}
	cases := []struct {
		name       string
		facts      normalizedImplementFacts
		obs        implementBranchObservation
		wantAction string
		wantReason string
	}{
		{
			name:       "create outside implement branch",
			facts:      base,
			obs:        implementBranchObservation{CurrentBranch: "feature/base", StartCommit: "abc123"},
			wantAction: "create",
			wantReason: "not an implementation branch",
		},
		{
			name:       "stop missing merge target on implement branch",
			facts:      normalizedImplementFacts{ScopeSlug: "target"},
			obs:        implementBranchObservation{CurrentBranch: "implement/old", StartCommit: "abc123"},
			wantAction: "stop",
			wantReason: "merge target required",
		},
		{
			name:       "continue matching branch",
			facts:      base,
			obs:        implementBranchObservation{CurrentBranch: "implement/target", StartCommit: "abc123"},
			wantAction: "continue",
			wantReason: "matches target scope",
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
