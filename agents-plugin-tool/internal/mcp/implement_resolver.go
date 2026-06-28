package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kang-sw/devenv/internal/wsgit"
)

type implementInput struct {
	Target implementTargetInput `json:"target"`
	Facts  implementFactsInput  `json:"facts,omitempty"`
	Policy implementPolicyInput `json:"policy,omitempty"`
	Format string               `json:"format,omitempty"`
}

type implementTargetInput struct {
	Kind       string `json:"kind,omitempty"`
	Label      string `json:"label,omitempty"`
	TicketStem string `json:"ticket_stem,omitempty"`
	TicketPath string `json:"ticket_path,omitempty"`
	ScopeLabel string `json:"scope_label,omitempty"`
	ScopeSlug  string `json:"scope_slug,omitempty"`
}

type implementFactsInput struct {
	Scope      implementScopeFactsInput      `json:"scope,omitempty"`
	Complexity implementComplexityFactsInput `json:"complexity,omitempty"`
	Risk       implementRiskFactsInput       `json:"risk,omitempty"`
}

type implementScopeFactsInput struct {
	Span                      factString `json:"span,omitempty"`
	Surface                   factString `json:"surface,omitempty"`
	NewPublicSymbol           factString `json:"new_public_symbol,omitempty"`
	NewTypeContract           factString `json:"new_type_contract,omitempty"`
	TestSurface               factString `json:"test_surface,omitempty"`
	ExplicitDelegationRequest factString `json:"explicit_delegation_request,omitempty"`
}

type implementComplexityFactsInput struct {
	ChangePoints   factString `json:"change_points,omitempty"`
	ReusePoints    factString `json:"reuse_points,omitempty"`
	StrategyShape  factString `json:"strategy_shape,omitempty"`
	SideEffectRisk factString `json:"side_effect_risk,omitempty"`
	ColdContext    factString `json:"cold_context,omitempty"`
}

type implementRiskFactsInput struct {
	Correctness        factString `json:"correctness,omitempty"`
	Fit                factString `json:"fit,omitempty"`
	Test               factString `json:"test,omitempty"`
	SecurityOrContract factString `json:"security_or_contract,omitempty"`
}

type implementPolicyInput struct {
	Branch implementBranchPolicyInput `json:"branch,omitempty"`
	Review implementReviewPolicyInput `json:"review,omitempty"`
	Docs   implementDocsPolicyInput   `json:"docs,omitempty"`
}

type implementBranchPolicyInput struct {
	MergeTarget factString `json:"merge_target,omitempty"`
	AllowRename factString `json:"allow_rename,omitempty"`
}

type implementReviewPolicyInput struct {
	Override factString `json:"override,omitempty"`
}

type implementDocsPolicyInput struct {
	Mode   factString `json:"mode,omitempty"`
	Reason factString `json:"reason,omitempty"`
}

type implementResult struct {
	Verdict         implementVerdict      `json:"verdict"`
	NextInstruction string                `json:"next_instruction"`
	Target          implementResultTarget `json:"target"`
	Scope           string                `json:"scope"`
	Reason          string                `json:"reason"`
	Conditions      []string              `json:"conditions"`
	Warnings        []string              `json:"warnings"`
	Agenda          implementAgenda       `json:"agenda"`
	TodoReplaced    bool                  `json:"todo_replaced"`
	Raw             string                `json:"raw"`
}

type implementResultTarget struct {
	Kind       string `json:"kind,omitempty"`
	Label      string `json:"label,omitempty"`
	TicketStem string `json:"ticket_stem,omitempty"`
	TicketPath string `json:"ticket_path,omitempty"`
	ScopeLabel string `json:"scope_label,omitempty"`
	ScopeSlug  string `json:"scope_slug,omitempty"`
}

type implementVerdict struct {
	Delegation  string              `json:"delegation"`
	BranchPlan  implementBranchPlan `json:"branch_plan"`
	PlanDepth   string              `json:"plan_depth"`
	ReviewAlloc string              `json:"review_alloc"`
	NeedReview  bool                `json:"need_review"`
	DocMode     string              `json:"doc_mode"`
}

type implementBranchPlan struct {
	Action        string   `json:"action"`
	CurrentBranch string   `json:"current_branch"`
	TargetBranch  string   `json:"target_branch,omitempty"`
	MergeTarget   string   `json:"merge_target,omitempty"`
	StartCommit   string   `json:"start_commit,omitempty"`
	Reason        string   `json:"reason"`
	Warnings      []string `json:"warnings"`
}

type implementAgenda struct {
	Delegation  string                `json:"delegation"`
	BranchPlan  implementBranchPlan   `json:"branch_plan"`
	PlanDepth   string                `json:"plan_depth"`
	ReviewAlloc string                `json:"review_alloc"`
	NeedReview  bool                  `json:"need_review"`
	DocMode     string                `json:"doc_mode"`
	DocReason   string                `json:"doc_reason,omitempty"`
	NeedDoc     bool                  `json:"need_doc"`
	Target      implementResultTarget `json:"target"`
	Scope       string                `json:"scope"`
	Conditions  []string              `json:"conditions"`
	Warnings    []string              `json:"warnings"`
}

type implementBranchObservation struct {
	CurrentBranch string
	StartCommit   string
	Upstream      string
	Ahead         int
	Behind        int
	TargetExists  bool
}

type normalizedImplementFacts struct {
	Span                      string
	Surface                   string
	NewPublicSymbol           string
	NewTypeContract           string
	TestSurface               string
	ExplicitDelegationRequest string
	ChangePoints              string
	ReusePoints               string
	StrategyShape             string
	SideEffectRisk            string
	ColdContext               string
	CorrectnessRisk           string
	FitRisk                   string
	TestRisk                  string
	SecurityOrContractRisk    string
	ReviewOverride            string
	DocModePolicy             string
	DocReason                 string
	MergeTargetPolicy         string
	AllowRename               string
	ScopeSlug                 string
}

func parseImplementInput(args map[string]any) (implementInput, error) {
	format, err := parseProceedFormat(args["format"])
	if err != nil {
		return implementInput{}, err
	}
	targetMap, ok := args["target"].(map[string]any)
	if !ok {
		if _, exists := args["target"]; !exists {
			return implementInput{}, fmt.Errorf("target is required")
		}
		return implementInput{}, fmt.Errorf("target must be an object")
	}
	target, err := parseImplementTarget(targetMap)
	if err != nil {
		return implementInput{}, err
	}
	facts, err := parseImplementFacts(args["facts"])
	if err != nil {
		return implementInput{}, err
	}
	policy, err := parseImplementPolicy(args["policy"])
	if err != nil {
		return implementInput{}, err
	}
	return implementInput{Target: target, Facts: facts, Policy: policy, Format: format}, nil
}

func parseImplementTarget(m map[string]any) (implementTargetInput, error) {
	kindFact, err := parseObjectString(m, "kind")
	if err != nil {
		return implementTargetInput{}, fmt.Errorf("target.%w", err)
	}
	kind := normalizeToken(kindFact.Value)
	switch kind {
	case "", "unknown":
		kind = "unknown"
	case "ticket", "inline":
	default:
		return implementTargetInput{}, fmt.Errorf("invalid target.kind %q: want one of ticket, inline, unknown", kind)
	}
	label, err := parseObjectString(m, "label")
	if err != nil {
		return implementTargetInput{}, fmt.Errorf("target.%w", err)
	}
	stem, err := parseObjectString(m, "ticket_stem")
	if err != nil {
		return implementTargetInput{}, fmt.Errorf("target.%w", err)
	}
	path, err := parseObjectString(m, "ticket_path")
	if err != nil {
		return implementTargetInput{}, fmt.Errorf("target.%w", err)
	}
	scopeLabel, err := parseObjectString(m, "scope_label")
	if err != nil {
		return implementTargetInput{}, fmt.Errorf("target.%w", err)
	}
	scopeSlug, err := parseObjectString(m, "scope_slug")
	if err != nil {
		return implementTargetInput{}, fmt.Errorf("target.%w", err)
	}
	out := implementTargetInput{
		Kind:       kind,
		Label:      strings.TrimSpace(label.Value),
		TicketStem: strings.TrimSpace(stem.Value),
		TicketPath: strings.TrimSpace(path.Value),
		ScopeLabel: strings.TrimSpace(scopeLabel.Value),
		ScopeSlug:  strings.TrimSpace(scopeSlug.Value),
	}
	if out.Label == "" {
		out.Label = firstNonEmpty(out.TicketPath, out.TicketStem, out.Kind)
	}
	return out, nil
}

func parseImplementFacts(raw any) (implementFactsInput, error) {
	if raw == nil {
		return implementFactsInput{}, nil
	}
	m, ok := raw.(map[string]any)
	if !ok {
		return implementFactsInput{}, fmt.Errorf("facts must be an object")
	}
	var out implementFactsInput
	if group, ok := m["scope"]; ok && group != nil {
		gm, ok := group.(map[string]any)
		if !ok {
			return out, fmt.Errorf("facts.scope must be an object")
		}
		scope, err := parseImplementScopeFacts(gm)
		if err != nil {
			return out, err
		}
		out.Scope = scope
	}
	if group, ok := m["complexity"]; ok && group != nil {
		gm, ok := group.(map[string]any)
		if !ok {
			return out, fmt.Errorf("facts.complexity must be an object")
		}
		complexity, err := parseImplementComplexityFacts(gm)
		if err != nil {
			return out, err
		}
		out.Complexity = complexity
	}
	if group, ok := m["risk"]; ok && group != nil {
		gm, ok := group.(map[string]any)
		if !ok {
			return out, fmt.Errorf("facts.risk must be an object")
		}
		risk, err := parseImplementRiskFacts(gm)
		if err != nil {
			return out, err
		}
		out.Risk = risk
	}
	return out, nil
}

func parseImplementScopeFacts(m map[string]any) (implementScopeFactsInput, error) {
	var out implementScopeFactsInput
	var err error
	if out.Span, err = parseEnumFact(m, "span", []string{"single-file", "multi-file", "unknown"}); err != nil {
		return out, fmt.Errorf("facts.scope.%w", err)
	}
	if out.Surface, err = parseEnumFact(m, "surface", []string{"internal", "public-interface", "cross-module", "unknown"}); err != nil {
		return out, fmt.Errorf("facts.scope.%w", err)
	}
	if out.NewPublicSymbol, err = parseEnumFact(m, "new_public_symbol", []string{"yes", "no", "unknown"}); err != nil {
		return out, fmt.Errorf("facts.scope.%w", err)
	}
	if out.NewTypeContract, err = parseEnumFact(m, "new_type_contract", []string{"yes", "no", "unknown"}); err != nil {
		return out, fmt.Errorf("facts.scope.%w", err)
	}
	if out.TestSurface, err = parseEnumFact(m, "test_surface", []string{"none", "existing", "new-files", "unknown"}); err != nil {
		return out, fmt.Errorf("facts.scope.%w", err)
	}
	if out.ExplicitDelegationRequest, err = parseEnumFact(m, "explicit_delegation_request", []string{"yes", "no", "unknown"}); err != nil {
		return out, fmt.Errorf("facts.scope.%w", err)
	}
	return out, nil
}

func parseImplementComplexityFacts(m map[string]any) (implementComplexityFactsInput, error) {
	var out implementComplexityFactsInput
	var err error
	if out.ChangePoints, err = parseEnumFact(m, "change_points", []string{"clear", "partially-known", "unknown"}); err != nil {
		return out, fmt.Errorf("facts.complexity.%w", err)
	}
	if out.ReusePoints, err = parseEnumFact(m, "reuse_points", []string{"confirmed", "unconfirmed", "not-applicable", "unknown"}); err != nil {
		return out, fmt.Errorf("facts.complexity.%w", err)
	}
	if out.StrategyShape, err = parseEnumFact(m, "strategy_shape", []string{"single-obvious", "multiple-viable", "unknown"}); err != nil {
		return out, fmt.Errorf("facts.complexity.%w", err)
	}
	if out.SideEffectRisk, err = parseEnumFact(m, "side_effect_risk", []string{"low", "moderate", "high", "unknown"}); err != nil {
		return out, fmt.Errorf("facts.complexity.%w", err)
	}
	if out.ColdContext, err = parseEnumFact(m, "cold_context", []string{"yes", "no", "unknown"}); err != nil {
		return out, fmt.Errorf("facts.complexity.%w", err)
	}
	return out, nil
}

func parseImplementRiskFacts(m map[string]any) (implementRiskFactsInput, error) {
	var out implementRiskFactsInput
	var err error
	for _, field := range []struct {
		name string
		dest *factString
	}{
		{"correctness", &out.Correctness},
		{"fit", &out.Fit},
		{"test", &out.Test},
		{"security_or_contract", &out.SecurityOrContract},
	} {
		if *field.dest, err = parseEnumFact(m, field.name, []string{"low", "moderate", "high", "unknown"}); err != nil {
			return out, fmt.Errorf("facts.risk.%w", err)
		}
	}
	return out, nil
}

func parseImplementPolicy(raw any) (implementPolicyInput, error) {
	if raw == nil {
		return implementPolicyInput{}, nil
	}
	m, ok := raw.(map[string]any)
	if !ok {
		return implementPolicyInput{}, fmt.Errorf("policy must be an object")
	}
	var out implementPolicyInput
	if group, ok := m["branch"]; ok && group != nil {
		gm, ok := group.(map[string]any)
		if !ok {
			return out, fmt.Errorf("policy.branch must be an object")
		}
		var err error
		if out.Branch.MergeTarget, err = parseObjectString(gm, "merge_target"); err != nil {
			return out, fmt.Errorf("policy.branch.%w", err)
		}
		if out.Branch.AllowRename, err = parseEnumFact(gm, "allow_rename", []string{"yes", "no", "unknown"}); err != nil {
			return out, fmt.Errorf("policy.branch.%w", err)
		}
	}
	if group, ok := m["review"]; ok && group != nil {
		gm, ok := group.(map[string]any)
		if !ok {
			return out, fmt.Errorf("policy.review must be an object")
		}
		var err error
		if out.Review.Override, err = parseEnumFact(gm, "override", []string{"auto", "lead-only", "single", "partitioned"}); err != nil {
			return out, fmt.Errorf("policy.review.%w", err)
		}
	}
	if group, ok := m["docs"]; ok && group != nil {
		gm, ok := group.(map[string]any)
		if !ok {
			return out, fmt.Errorf("policy.docs must be an object")
		}
		var err error
		if out.Docs.Mode, err = parseEnumFact(gm, "mode", []string{"standard", "skip-with-reason", "unknown"}); err != nil {
			return out, fmt.Errorf("policy.docs.%w", err)
		}
		if out.Docs.Reason, err = parseObjectString(gm, "reason"); err != nil {
			return out, fmt.Errorf("policy.docs.%w", err)
		}
	}
	return out, nil
}

func observeImplementBranch(root string, targetBranch string) (implementBranchObservation, error) {
	client := wsgit.NewClient()
	status, err := client.Status(context.Background(), root)
	if err != nil {
		return implementBranchObservation{}, err
	}
	obs := implementBranchObservation{
		CurrentBranch: status.Branch.Head,
		StartCommit:   status.Branch.OID,
		Upstream:      status.Branch.Upstream,
		Ahead:         status.Branch.Ahead,
		Behind:        status.Branch.Behind,
	}
	if targetBranch != "" {
		if _, err := (wsgit.ExecRunner{}).RunGit(context.Background(), root, "rev-parse", "--verify", "--quiet", "refs/heads/"+targetBranch); err == nil {
			obs.TargetExists = true
		}
	}
	return obs, nil
}

func resolveImplement(input implementInput, obs implementBranchObservation) implementResult {
	n, warnings := normalizeImplementFacts(input)
	target := implementResultTarget{
		Kind:       input.Target.Kind,
		Label:      input.Target.Label,
		TicketStem: input.Target.TicketStem,
		TicketPath: input.Target.TicketPath,
		ScopeLabel: input.Target.ScopeLabel,
		ScopeSlug:  n.ScopeSlug,
	}
	branchPlan := deriveImplementBranchPlan(n, obs)
	warnings = append(warnings, branchPlan.Warnings...)
	delegation := deriveImplementDelegation(n)
	planDepth := deriveImplementPlanDepth(n, delegation)
	reviewAlloc := deriveImplementReviewAlloc(n, delegation)
	docMode := deriveImplementDocMode(n)
	needReview := reviewAlloc != "lead-only"
	conditions := implementConditions(n)
	reason := implementReason(n, delegation, planDepth, reviewAlloc)
	verdict := implementVerdict{
		Delegation:  delegation,
		BranchPlan:  branchPlan,
		PlanDepth:   planDepth,
		ReviewAlloc: reviewAlloc,
		NeedReview:  needReview,
		DocMode:     docMode,
	}
	agenda := implementAgenda{
		Delegation:  delegation,
		BranchPlan:  branchPlan,
		PlanDepth:   planDepth,
		ReviewAlloc: reviewAlloc,
		NeedReview:  needReview,
		DocMode:     docMode,
		DocReason:   n.DocReason,
		NeedDoc:     docMode == "standard",
		Target:      target,
		Scope:       firstNonEmpty(input.Target.ScopeLabel, "unknown"),
		Conditions:  conditions,
		Warnings:    warnings,
	}
	result := implementResult{
		Verdict:         verdict,
		NextInstruction: implementNextInstruction(verdict),
		Target:          target,
		Scope:           agenda.Scope,
		Reason:          reason,
		Conditions:      conditions,
		Warnings:        warnings,
		Agenda:          agenda,
		TodoReplaced:    true,
	}
	result.Raw = renderImplementRaw(result)
	return result
}

func normalizeImplementFacts(input implementInput) (normalizedImplementFacts, []string) {
	warnings := []string{}
	scope := input.Facts.Scope
	complexity := input.Facts.Complexity
	risk := input.Facts.Risk
	policy := input.Policy
	n := normalizedImplementFacts{
		Span:                      factOr(scope.Span, "unknown"),
		Surface:                   factOr(scope.Surface, "unknown"),
		NewPublicSymbol:           factOr(scope.NewPublicSymbol, "unknown"),
		NewTypeContract:           factOr(scope.NewTypeContract, "unknown"),
		TestSurface:               factOr(scope.TestSurface, "unknown"),
		ExplicitDelegationRequest: factOr(scope.ExplicitDelegationRequest, "unknown"),
		ChangePoints:              factOr(complexity.ChangePoints, "unknown"),
		ReusePoints:               factOr(complexity.ReusePoints, "unknown"),
		StrategyShape:             factOr(complexity.StrategyShape, "unknown"),
		SideEffectRisk:            factOr(complexity.SideEffectRisk, "unknown"),
		ColdContext:               factOr(complexity.ColdContext, "unknown"),
		CorrectnessRisk:           factOr(risk.Correctness, "unknown"),
		FitRisk:                   factOr(risk.Fit, "unknown"),
		TestRisk:                  factOr(risk.Test, "unknown"),
		SecurityOrContractRisk:    factOr(risk.SecurityOrContract, "unknown"),
		ReviewOverride:            factOr(policy.Review.Override, "auto"),
		DocModePolicy:             factOr(policy.Docs.Mode, "standard"),
		DocReason:                 strings.TrimSpace(policy.Docs.Reason.Value),
		MergeTargetPolicy:         strings.TrimSpace(policy.Branch.MergeTarget.Value),
		AllowRename:               factOr(policy.Branch.AllowRename, "unknown"),
		ScopeSlug:                 strings.TrimSpace(input.Target.ScopeSlug),
	}
	if n.ScopeSlug == "" {
		n.ScopeSlug = slugifyImplementScope(firstNonEmpty(input.Target.ScopeLabel, input.Target.TicketStem, input.Target.Label, "implementation"))
		warnings = append(warnings, "target.scope_slug missing; derived from target label")
	}
	if n.DocModePolicy == "skip-with-reason" && n.DocReason == "" {
		warnings = append(warnings, "docs skip requested without reason; normalized to standard")
		n.DocModePolicy = "standard"
	}
	if n.ReviewOverride == "" {
		n.ReviewOverride = "auto"
	}
	return n, warnings
}

func deriveImplementDelegation(n normalizedImplementFacts) string {
	if n.Span == "single-file" &&
		n.Surface == "internal" &&
		n.NewPublicSymbol == "no" &&
		n.NewTypeContract == "no" &&
		n.TestSurface != "new-files" &&
		n.ExplicitDelegationRequest == "no" {
		return "direct-edit"
	}
	return "delegated"
}

func deriveImplementPlanDepth(n normalizedImplementFacts, delegation string) string {
	if delegation == "delegated" {
		return "survey"
	}
	if n.ChangePoints == "clear" && n.SideEffectRisk == "low" {
		return "none"
	}
	return "none"
}

func deriveImplementReviewAlloc(n normalizedImplementFacts, delegation string) string {
	if n.ReviewOverride != "" && n.ReviewOverride != "auto" {
		switch n.ReviewOverride {
		case "partitioned":
			return partitionedReviewAlloc(n)
		default:
			return n.ReviewOverride
		}
	}
	if delegation == "direct-edit" && n.CorrectnessRisk == "low" && n.FitRisk == "low" && n.TestRisk == "low" && n.SecurityOrContractRisk == "low" {
		return "lead-only"
	}
	if delegation == "direct-edit" && n.Surface == "internal" && n.SecurityOrContractRisk != "high" {
		return "single"
	}
	return partitionedReviewAlloc(n)
}

func partitionedReviewAlloc(n normalizedImplementFacts) string {
	parts := []string{}
	if materialRisk(n.CorrectnessRisk) || materialRisk(n.SecurityOrContractRisk) || n.NewTypeContract == "yes" || n.NewPublicSymbol == "yes" {
		parts = append(parts, "correctness")
	}
	if materialRisk(n.FitRisk) || n.Surface == "public-interface" || n.Surface == "cross-module" || n.ReusePoints == "unconfirmed" || n.ReusePoints == "unknown" {
		parts = append(parts, "fit")
	}
	if materialRisk(n.TestRisk) || n.TestSurface == "existing" || n.TestSurface == "new-files" || n.TestSurface == "unknown" {
		parts = append(parts, "test")
	}
	if len(parts) == 0 {
		parts = append(parts, "correctness")
	}
	return "partitioned: " + strings.Join(parts, ", ")
}

func materialRisk(value string) bool {
	return value == "moderate" || value == "high" || value == "unknown"
}

func deriveImplementDocMode(n normalizedImplementFacts) string {
	if n.DocModePolicy == "skip-with-reason" && n.DocReason != "" {
		return "skipped"
	}
	return "standard"
}

func deriveImplementBranchPlan(n normalizedImplementFacts, obs implementBranchObservation) implementBranchPlan {
	targetBranch := "implement/" + n.ScopeSlug
	plan := implementBranchPlan{
		CurrentBranch: obs.CurrentBranch,
		TargetBranch:  targetBranch,
		StartCommit:   obs.StartCommit,
		MergeTarget:   n.MergeTargetPolicy,
	}
	if !strings.HasPrefix(obs.CurrentBranch, "implement/") {
		plan.Action = "create"
		plan.MergeTarget = obs.CurrentBranch
		plan.Reason = "current branch is not an implementation branch"
		return plan
	}
	if plan.MergeTarget == "" {
		plan.Action = "stop"
		plan.Reason = "merge target required while already on an implementation branch"
		return plan
	}
	if obs.CurrentBranch == targetBranch {
		plan.Action = "continue"
		plan.Reason = "current implementation branch matches target scope"
		return plan
	}
	if n.AllowRename != "yes" {
		plan.Action = "stop"
		plan.Reason = "current implementation branch differs from target scope and rename is not allowed"
		return plan
	}
	if obs.TargetExists {
		plan.Action = "stop"
		plan.Reason = "target implementation branch already exists"
		return plan
	}
	if obs.Upstream != "" || obs.Ahead != 0 || obs.Behind != 0 {
		plan.Action = "stop"
		plan.Reason = "current implementation branch has upstream/tracking state; rename is ambiguous"
		return plan
	}
	plan.Action = "rename"
	plan.Reason = "current implementation branch differs from target scope and rename is allowed"
	return plan
}

func implementNextInstruction(verdict implementVerdict) string {
	nextAfterBranch := implementNextAfterBranch(verdict)
	switch verdict.BranchPlan.Action {
	case "stop":
		return "Stop before source edits. Report the branch safety blocker in Branch Action and ask for the missing policy or branch cleanup."
	case "create":
		return fmt.Sprintf("Create %s from %s before source edits, then %s", verdict.BranchPlan.TargetBranch, verdict.BranchPlan.MergeTarget, nextAfterBranch)
	case "rename":
		return fmt.Sprintf("Rename the current branch to %s before source edits, then %s", verdict.BranchPlan.TargetBranch, nextAfterBranch)
	case "continue":
		return fmt.Sprintf("Continue on %s, then %s", verdict.BranchPlan.CurrentBranch, nextAfterBranch)
	default:
		return "Stop before source edits. Report that the branch action is unrecognized."
	}
}

func implementNextAfterBranch(verdict implementVerdict) string {
	if verdict.Delegation == "direct-edit" {
		return fmt.Sprintf("run prep guardrails, apply direct edits in the lead context, run %s review, and complete %s documentation gates.", verdict.ReviewAlloc, verdict.DocMode)
	}
	return fmt.Sprintf("call ws.path.generate(kind: \"plan\"), render plan-populator-survey with ticket_path, selected_phase, and plan_path, dispatch it to write the light plan, render plan-populator-research on the same plan path only if survey returns [escalate-to-research] for low confidence or strategic uncertainty, then render implementer with PlanPath before %s review and %s documentation gates.", verdict.ReviewAlloc, verdict.DocMode)
}

func implementConditions(n normalizedImplementFacts) []string {
	conditions := []string{
		"span=" + n.Span,
		"surface=" + n.Surface,
		"new-public-symbol=" + n.NewPublicSymbol,
		"new-type-contract=" + n.NewTypeContract,
		"test-surface=" + n.TestSurface,
		"explicit-delegation-request=" + n.ExplicitDelegationRequest,
		"change-points=" + n.ChangePoints,
		"reuse-points=" + n.ReusePoints,
		"strategy-shape=" + n.StrategyShape,
		"side-effect-risk=" + n.SideEffectRisk,
		"correctness-risk=" + n.CorrectnessRisk,
		"fit-risk=" + n.FitRisk,
		"test-risk=" + n.TestRisk,
		"security-or-contract-risk=" + n.SecurityOrContractRisk,
		"review-override=" + n.ReviewOverride,
		"doc-mode-policy=" + n.DocModePolicy,
	}
	if n.DocModePolicy == "skip-with-reason" {
		conditions = append(conditions, "doc-reason="+n.DocReason)
	}
	return conditions
}

func implementReason(n normalizedImplementFacts, delegation, planDepth, reviewAlloc string) string {
	return fmt.Sprintf("delegation=%s; plan-depth=%s; review=%s; surface=%s; span=%s; side-effect-risk=%s", delegation, planDepth, reviewAlloc, n.Surface, n.Span, n.SideEffectRisk)
}

func renderImplementRaw(result implementResult) string {
	var b strings.Builder
	v := result.Verdict
	fmt.Fprintf(&b, "Implementation Verdict\n")
	fmt.Fprintf(&b, "Mode: %s\n", v.Delegation)
	if v.BranchPlan.Action == "stop" {
		fmt.Fprintf(&b, "Branch Action: stop - %s\n", v.BranchPlan.Reason)
	} else {
		fmt.Fprintf(&b, "Branch Action: %s %s\n", v.BranchPlan.Action, firstNonEmpty(v.BranchPlan.TargetBranch, v.BranchPlan.CurrentBranch))
	}
	fmt.Fprintf(&b, "Merge Target: %s\n", firstNonEmpty(v.BranchPlan.MergeTarget, "n/a"))
	fmt.Fprintf(&b, "Plan Depth: %s\n", v.PlanDepth)
	fmt.Fprintf(&b, "Review Allocation: %s\n", v.ReviewAlloc)
	fmt.Fprintf(&b, "Doc Mode: %s\n\n", v.DocMode)
	fmt.Fprintf(&b, "Next: %s\n\n", result.NextInstruction)
	fmt.Fprintf(&b, "Target: %s\n", firstNonEmpty(result.Target.Label, result.Target.TicketStem, result.Target.TicketPath, "n/a"))
	fmt.Fprintf(&b, "Scope: %s\n", result.Scope)
	fmt.Fprintf(&b, "Reason: %s\n\n", result.Reason)
	b.WriteString("Conditions:\n")
	for _, condition := range result.Conditions {
		fmt.Fprintf(&b, "- %s\n", condition)
	}
	b.WriteString("\nWarnings:\n")
	if len(result.Warnings) == 0 {
		b.WriteString("- none\n")
	} else {
		for _, warning := range result.Warnings {
			fmt.Fprintf(&b, "- %s\n", warning)
		}
	}
	b.WriteString("\nAgenda:\n")
	fmt.Fprintf(&b, "- delegation: %s\n", result.Agenda.Delegation)
	fmt.Fprintf(&b, "- branch_plan.action: %s\n", result.Agenda.BranchPlan.Action)
	fmt.Fprintf(&b, "- branch_plan.target_branch: %s\n", firstNonEmpty(result.Agenda.BranchPlan.TargetBranch, "n/a"))
	fmt.Fprintf(&b, "- merge_target: %s\n", firstNonEmpty(result.Agenda.BranchPlan.MergeTarget, "n/a"))
	fmt.Fprintf(&b, "- plan_depth: %s\n", result.Agenda.PlanDepth)
	fmt.Fprintf(&b, "- review_alloc: %s\n", result.Agenda.ReviewAlloc)
	fmt.Fprintf(&b, "- need_review: %t\n", result.Agenda.NeedReview)
	fmt.Fprintf(&b, "- doc_mode: %s\n", result.Agenda.DocMode)
	if result.Agenda.DocMode == "skipped" {
		fmt.Fprintf(&b, "- doc_reason: %s\n", result.Agenda.DocReason)
	}
	return b.String()
}

func implementResultJSON(result implementResult) (string, error) {
	raw, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return "", err
	}
	return string(raw) + "\n", nil
}

func slugifyImplementScope(raw string) string {
	s := strings.ToLower(strings.TrimSpace(raw))
	var b strings.Builder
	lastDash := false
	for _, r := range s {
		keep := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if keep {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "implementation"
	}
	return out
}
