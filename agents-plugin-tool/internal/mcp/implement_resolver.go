package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/kang-sw/devenv/internal/wsgit"
	"github.com/kang-sw/devenv/internal/wskey"
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
	ExplicitDirectEditRequest factString `json:"explicit_direct_edit_request,omitempty"`
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
	LowCeremonyIfSafe factString                 `json:"low_ceremony_if_safe,omitempty"`
	Branch            implementBranchPolicyInput `json:"branch,omitempty"`
	Review            implementReviewPolicyInput `json:"review,omitempty"`
	Docs              implementDocsPolicyInput   `json:"docs,omitempty"`
}

type implementBranchPolicyInput struct {
	MergeTarget  factString `json:"merge_target,omitempty"`
	AllowRename  factString `json:"allow_rename,omitempty"`
	MergeConfirm factString `json:"merge_confirm,omitempty"`
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
	Action             string   `json:"action"`
	CurrentBranch      string   `json:"current_branch"`
	TargetBranch       string   `json:"target_branch,omitempty"`
	MergeTarget        string   `json:"merge_target,omitempty"`
	MergeConfirm       string   `json:"merge_confirm,omitempty"`
	StartCommit        string   `json:"start_commit,omitempty"`
	Reason             string   `json:"reason"`
	Warnings           []string `json:"warnings"`
	SuspectedOwnerStem string   `json:"suspected_owner_stem,omitempty"`
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
	CurrentBranch        string
	StartCommit          string
	Upstream             string
	Ahead                int
	Behind               int
	TargetExists         bool
	MergeRootRefConflict string
	AheadOfMergeRoot     int
}

type normalizedImplementFacts struct {
	Span                      string
	Surface                   string
	NewPublicSymbol           string
	NewTypeContract           string
	TestSurface               string
	ExplicitDelegationRequest string
	ExplicitDirectEditRequest string
	ChangePoints              string
	ReusePoints               string
	StrategyShape             string
	SideEffectRisk            string
	ColdContext               string
	CorrectnessRisk           string
	FitRisk                   string
	TestRisk                  string
	SecurityOrContractRisk    string
	LowCeremonyIfSafe         string
	ReviewOverride            string
	DocModePolicy             string
	DocReason                 string
	MergeTargetPolicy         string
	AllowRename               string
	MergeConfirmPolicy        string
	ScopeSlug                 string
	TicketStem                string
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
	if out.ExplicitDirectEditRequest, err = parseEnumFact(m, "explicit_direct_edit_request", []string{"yes", "no", "unknown"}); err != nil {
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
	var err error
	if out.LowCeremonyIfSafe, err = parseEnumFact(m, "low_ceremony_if_safe", []string{"yes", "no", "unknown"}); err != nil {
		return out, fmt.Errorf("policy.%w", err)
	}
	if group, ok := m["branch"]; ok && group != nil {
		gm, ok := group.(map[string]any)
		if !ok {
			return out, fmt.Errorf("policy.branch must be an object")
		}
		if out.Branch.MergeTarget, err = parseObjectString(gm, "merge_target"); err != nil {
			return out, fmt.Errorf("policy.branch.%w", err)
		}
		if out.Branch.AllowRename, err = parseEnumFact(gm, "allow_rename", []string{"yes", "no", "unknown"}); err != nil {
			return out, fmt.Errorf("policy.branch.%w", err)
		}
		if out.Branch.MergeConfirm, err = parseEnumFact(gm, "merge_confirm", []string{"skip", "ask", "unknown"}); err != nil {
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

// aheadOfMergeRootCount returns the number of commits currentBranch carries
// ahead of mergeRoot's merge-base with currentBranch. It fails open to 0 on
// any git error (unresolvable ref, unrelated histories): an infra failure
// here is out of the ticket's test matrix, not a normal false-negative risk
// case, consistent with the existing err == nil truthy pattern this file
// already uses for TargetExists/MergeRootRefConflict.
func aheadOfMergeRootCount(root, mergeRoot, currentBranch string) int {
	result, err := wsgit.NewClient().MergeBase(context.Background(), root, mergeRoot, currentBranch)
	if err != nil {
		return 0
	}
	out, err := (wsgit.ExecRunner{}).RunGit(context.Background(), root, "rev-list", "--count", result.MergeBase+".."+currentBranch)
	if err != nil {
		return 0
	}
	count, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil {
		return 0
	}
	return count
}

// implementCloseMergeReviewNudge computes the tickets.close merge-review
// advisory: closing a ticket while the current branch is an unmerged
// impl/<root>/<stem> branch leaves that branch's work unreviewed-and-merged,
// so the close response nudges the lead to review-and-merge it into <root>
// after the close-move commit lands. tickets.close itself never merges or
// commits (see {#260620-ticket-close-tool}), so this is advisory text only,
// computed from the pre-close-commit git state via observeImplementBranch's
// existing AheadOfMergeRoot observation (Phase 1) — no new git-observation
// code, and no marker/schema/code path for the ticket-declared stop-gate
// exception, which stays ordinary lead judgment outside this hook. Failing
// open to "" on any git error, a non-impl branch, or a merged/clean impl
// branch keeps this from ever blocking or erroring the close call, and
// leaves room for epic 260824's later review-watermark hook to compose
// without rework.
func implementCloseMergeReviewNudge(root string) string {
	obs, err := observeImplementBranch(root, "")
	if err != nil {
		return ""
	}
	mergeRoot, stem, ok := parseImplBranchRoot(obs.CurrentBranch)
	if !ok || obs.AheadOfMergeRoot <= 0 {
		return ""
	}
	return fmt.Sprintf("This tool performed no merge. After the close-move commit for this ticket lands, review and merge %s into %s.", "impl/"+mergeRoot+"/"+stem, mergeRoot)
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
	if validObservedBranch(obs.CurrentBranch) {
		mergeRoot := implementMergeRootFor(obs.CurrentBranch)
		if mergeRoot != "" && mergeRoot != obs.CurrentBranch {
			obs.AheadOfMergeRoot = aheadOfMergeRootCount(root, mergeRoot, obs.CurrentBranch)
		}
	}
	if targetBranch != "" {
		if _, err := (wsgit.ExecRunner{}).RunGit(context.Background(), root, "rev-parse", "--verify", "--quiet", "refs/heads/"+targetBranch); err == nil {
			obs.TargetExists = true
		}
		segments := strings.Split(targetBranch, "/")
		for i := 1; i < len(segments); i++ {
			ancestor := strings.Join(segments[:i], "/")
			if _, err := (wsgit.ExecRunner{}).RunGit(context.Background(), root, "rev-parse", "--verify", "--quiet", "refs/heads/"+ancestor); err == nil {
				obs.MergeRootRefConflict = ancestor
				break
			}
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
	delegation := deriveImplementDelegation(n)
	planDepth := deriveImplementPlanDepth(n, delegation)
	reviewAlloc := deriveImplementReviewAlloc(n, delegation)
	docMode := deriveImplementDocMode(n)
	branchPlan := deriveResolvedImplementBranchPlan(input.Target.Kind, n, obs)
	warnings = append(warnings, branchPlan.Warnings...)
	if n.LowCeremonyIfSafe == "yes" && branchPlan.Action != "current" {
		warnings = append(warnings, "policy.low_ceremony_if_safe=yes not applicable; continuing with standard branch path")
	}
	if branchPlan.Action == "create" && n.MergeTargetPolicy != "" {
		warnings = append(warnings, fmt.Sprintf("policy.branch.merge_target %q ignored (not on an implementation branch: impl/*, or legacy implement/*); derived from current branch %q", n.MergeTargetPolicy, branchPlan.MergeTarget))
	}
	needReview := reviewAlloc != "lead-only"
	conditions := implementConditions(n, branchPlan.Action)
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
		ExplicitDirectEditRequest: factOr(scope.ExplicitDirectEditRequest, "unknown"),
		ChangePoints:              factOr(complexity.ChangePoints, "unknown"),
		ReusePoints:               factOr(complexity.ReusePoints, "unknown"),
		StrategyShape:             factOr(complexity.StrategyShape, "unknown"),
		SideEffectRisk:            factOr(complexity.SideEffectRisk, "unknown"),
		ColdContext:               factOr(complexity.ColdContext, "unknown"),
		CorrectnessRisk:           factOr(risk.Correctness, "unknown"),
		FitRisk:                   factOr(risk.Fit, "unknown"),
		TestRisk:                  factOr(risk.Test, "unknown"),
		SecurityOrContractRisk:    factOr(risk.SecurityOrContract, "unknown"),
		LowCeremonyIfSafe:         factOr(policy.LowCeremonyIfSafe, "unknown"),
		ReviewOverride:            factOr(policy.Review.Override, "auto"),
		DocModePolicy:             factOr(policy.Docs.Mode, "standard"),
		DocReason:                 strings.TrimSpace(policy.Docs.Reason.Value),
		MergeTargetPolicy:         strings.TrimSpace(policy.Branch.MergeTarget.Value),
		AllowRename:               factOr(policy.Branch.AllowRename, "yes"),
		MergeConfirmPolicy:        factOr(policy.Branch.MergeConfirm, "ask"),
		ScopeSlug:                 strings.TrimSpace(input.Target.ScopeSlug),
		TicketStem:                strings.TrimSpace(input.Target.TicketStem),
	}
	if n.TicketStem != "" {
		if n.ScopeSlug != "" {
			warnings = append(warnings, "target.scope_slug ignored for ticket target; branch stem derived deterministically from ticket_stem")
		}
		n.ScopeSlug = wskey.Derive(n.TicketStem, 3)
	} else if n.ScopeSlug == "" {
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
	if n.ExplicitDirectEditRequest == "yes" {
		return "direct-edit"
	}
	if n.ExplicitDelegationRequest == "yes" {
		return "delegated"
	}
	if automaticDirectEditEligible(n) {
		return "direct-edit"
	}
	return "delegated"
}

func automaticDirectEditEligible(n normalizedImplementFacts) bool {
	return n.Span == "single-file" &&
		n.Surface == "internal" &&
		n.NewPublicSymbol == "no" &&
		n.NewTypeContract == "no" &&
		n.TestSurface != "new-files"
}

func automaticLeadOnlyReviewEligible(n normalizedImplementFacts, delegation string) bool {
	return delegation == "direct-edit" &&
		n.CorrectnessRisk == "low" &&
		n.FitRisk == "low" &&
		n.TestRisk == "low" &&
		n.SecurityOrContractRisk == "low"
}

func currentBranchImplementEligible(targetKind string, n normalizedImplementFacts, obs implementBranchObservation) bool {
	return targetKind == "inline" &&
		n.LowCeremonyIfSafe == "yes" &&
		validObservedBranch(obs.CurrentBranch) &&
		validObservedStartCommit(obs.StartCommit) &&
		!strings.HasPrefix(obs.CurrentBranch, "impl/") &&
		!strings.HasPrefix(obs.CurrentBranch, "implement/") &&
		n.ExplicitDelegationRequest != "yes" &&
		automaticDirectEditEligible(n) &&
		n.TestSurface != "unknown" &&
		n.ReviewOverride == "auto" &&
		automaticLeadOnlyReviewEligible(n, "direct-edit") &&
		n.DocModePolicy == "skip-with-reason" &&
		n.DocReason != ""
}

func validObservedStartCommit(commit string) bool {
	commit = strings.TrimSpace(commit)
	return commit != "" && commit != "(initial)"
}

func validObservedBranch(branch string) bool {
	branch = strings.TrimSpace(branch)
	return branch != "" && branch != "(detached)"
}

func deriveResolvedImplementBranchPlan(targetKind string, n normalizedImplementFacts, obs implementBranchObservation) implementBranchPlan {
	if !currentBranchImplementEligible(targetKind, n, obs) {
		return deriveImplementBranchPlan(n, obs)
	}
	return implementBranchPlan{
		Action:        "current",
		CurrentBranch: obs.CurrentBranch,
		StartCommit:   obs.StartCommit,
		Reason:        "inline target independently qualifies for current-branch completion",
	}
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
	if automaticLeadOnlyReviewEligible(n, delegation) {
		return "lead-only"
	}
	parts := implementReviewPartitions(n)
	if len(parts) <= 1 {
		return "single"
	}
	return "partitioned: " + strings.Join(parts, ", ")
}

func implementReviewPartitions(n normalizedImplementFacts) []string {
	parts := []string{}
	if materialRisk(n.CorrectnessRisk) || materialRisk(n.SecurityOrContractRisk) || n.NewTypeContract == "yes" || n.NewPublicSymbol == "yes" {
		parts = append(parts, "correctness")
	}
	if materialRisk(n.FitRisk) || n.Surface == "cross-module" || n.ReusePoints == "unconfirmed" {
		parts = append(parts, "fit")
	}
	if materialRisk(n.TestRisk) || n.TestSurface == "new-files" {
		parts = append(parts, "test")
	}
	return parts
}

func partitionedReviewAlloc(n normalizedImplementFacts) string {
	parts := implementReviewPartitions(n)
	if len(parts) == 0 {
		parts = append(parts, "correctness")
	}
	return "partitioned: " + strings.Join(parts, ", ")
}

func materialRisk(value string) bool {
	return value == "moderate" || value == "high"
}

func deriveImplementDocMode(n normalizedImplementFacts) string {
	if n.DocModePolicy == "skip-with-reason" && n.DocReason != "" {
		return "skipped"
	}
	return "standard"
}

// implementTargetBranchName builds the canonical implementation branch name
// for a given merge root and scope slug: "impl/" followed by the merge root
// (if any) and the slug (<=15 characters recommended, not enforced), with any
// "/" in scopeSlug sanitized to "-" (the stem must stay single-segment so
// parseImplBranchRoot's split-on-last-slash parse is never ambiguous) and any
// trailing "-" trimmed. Both branch-plan derivation and enter-implement
// observation must use this single helper so the two never construct
// diverging target-branch names.
func implementTargetBranchName(mergeRoot, scopeSlug string) string {
	stem := strings.ReplaceAll(scopeSlug, "/", "-")
	stem = strings.TrimRight(stem, "-")
	if mergeRoot == "" {
		return "impl/" + stem
	}
	return "impl/" + mergeRoot + "/" + stem
}

// parseImplBranchRoot parses an "impl/"-prefixed branch name into its merge
// root and stem. ok is true only when branch has the "impl/" prefix and the
// remainder contains at least one "/" (i.e. a merge-root segment is present);
// the split happens on the LAST "/", so a merge root that itself contains "/"
// (e.g. a nested branch name) is preserved intact.
func parseImplBranchRoot(branch string) (root, stem string, ok bool) {
	const prefix = "impl/"
	if !strings.HasPrefix(branch, prefix) {
		return "", "", false
	}
	rest := strings.TrimPrefix(branch, prefix)
	idx := strings.LastIndex(rest, "/")
	if idx < 0 {
		return "", "", false
	}
	return rest[:idx], rest[idx+1:], true
}

// implementMergeRootFor derives the merge root for a given current branch
// using the same 3-way rule deriveImplementBranchPlan applies: a fresh
// (non-impl/-, non-implement/-prefixed) branch is its own merge root; a
// name-rooted "impl/<root>/<stem>" branch yields the parsed root; a rootless
// "impl/<stem>" or any "implement/<stem>" branch yields "" (legacy path,
// merge target comes solely from caller policy). Used by enter.implement's
// preflight to build the correct target branch name before the real
// observation is taken.
func implementMergeRootFor(currentBranch string) string {
	if !strings.HasPrefix(currentBranch, "impl/") && !strings.HasPrefix(currentBranch, "implement/") {
		return currentBranch
	}
	if root, _, ok := parseImplBranchRoot(currentBranch); ok {
		return root
	}
	return ""
}

// deriveImplementBranchPlan implements a 3-way split on the observed current
// branch:
//  1. Not "impl/"- or "implement/"-prefixed -> create: merge root is the
//     current branch itself (may contain "/"), the D/F ref-conflict guard
//     applies only here since this is the only path that mints a brand-new
//     nested ref.
//  2. "impl/<root>/<stem>" (root present) -> name-rooted: the branch NAME is
//     the authoritative merge-root source, never the caller's
//     policy.branch.merge_target; a diverging caller value is reconciled to
//     the name-root with a warning rather than silently honored.
//  3. Rootless "impl/<stem>" or any "implement/<stem>" -> unchanged legacy
//     path: merge target comes solely from policy.branch.merge_target, empty
//     still stops-and-asks. This path's behavior is byte-for-byte identical
//     to the pre-encoding resolver.
func deriveImplementBranchPlan(n normalizedImplementFacts, obs implementBranchObservation) implementBranchPlan {
	if !validObservedBranch(obs.CurrentBranch) {
		return implementBranchPlan{
			CurrentBranch: obs.CurrentBranch,
			StartCommit:   obs.StartCommit,
			MergeConfirm:  n.MergeConfirmPolicy,
			Action:        "stop",
			Reason:        "current branch is unavailable or detached",
		}
	}

	isImpl := strings.HasPrefix(obs.CurrentBranch, "impl/")
	isImplement := strings.HasPrefix(obs.CurrentBranch, "implement/")

	if !isImpl && !isImplement {
		mergeRoot := obs.CurrentBranch
		targetBranch := implementTargetBranchName(mergeRoot, n.ScopeSlug)
		plan := implementBranchPlan{
			CurrentBranch: obs.CurrentBranch,
			TargetBranch:  targetBranch,
			StartCommit:   obs.StartCommit,
			MergeTarget:   mergeRoot,
			MergeConfirm:  n.MergeConfirmPolicy,
		}
		if obs.MergeRootRefConflict != "" {
			plan.Action = "stop"
			plan.Reason = fmt.Sprintf("existing branch %q conflicts with creating %q", obs.MergeRootRefConflict, targetBranch)
			return plan
		}
		plan.Action = "create"
		plan.Reason = "current branch is not an implementation branch"
		return plan
	}

	if root, _, ok := parseImplBranchRoot(obs.CurrentBranch); ok {
		mergeRoot := root
		targetBranch := implementTargetBranchName(mergeRoot, n.ScopeSlug)
		plan := implementBranchPlan{
			CurrentBranch: obs.CurrentBranch,
			TargetBranch:  targetBranch,
			StartCommit:   obs.StartCommit,
			MergeTarget:   mergeRoot,
			MergeConfirm:  n.MergeConfirmPolicy,
		}
		if n.MergeTargetPolicy != "" && n.MergeTargetPolicy != mergeRoot {
			plan.Warnings = append(plan.Warnings, fmt.Sprintf(
				"policy.branch.merge_target %q ignored (implementation branch name encodes merge root %q)",
				n.MergeTargetPolicy, mergeRoot))
		}
		return finishImplementBranchPlanTail(plan, n, obs, targetBranch)
	}

	// Rootless "impl/<stem>" or any "implement/<stem>": unchanged legacy path.
	targetBranch := implementTargetBranchName("", n.ScopeSlug)
	plan := implementBranchPlan{
		CurrentBranch: obs.CurrentBranch,
		TargetBranch:  targetBranch,
		StartCommit:   obs.StartCommit,
		MergeTarget:   n.MergeTargetPolicy,
		MergeConfirm:  n.MergeConfirmPolicy,
	}
	if plan.MergeTarget == "" {
		plan.Action = "stop"
		plan.Reason = "merge target required while already on an implementation branch"
		return plan
	}
	return finishImplementBranchPlanTail(plan, n, obs, targetBranch)
}

// finishImplementBranchPlanTail applies the shared continue/rename/stop
// comparison logic once a branch's mergeRoot and targetBranch are known,
// regardless of which of the 3 deriveImplementBranchPlan paths produced them.
func finishImplementBranchPlanTail(plan implementBranchPlan, n normalizedImplementFacts, obs implementBranchObservation, targetBranch string) implementBranchPlan {
	if obs.CurrentBranch == targetBranch {
		plan.Action = "continue"
		plan.Reason = "current implementation branch matches target scope"
		return plan
	}
	if obs.AheadOfMergeRoot > 0 {
		_, suspectedStem, _ := parseImplBranchRoot(obs.CurrentBranch)
		plan.Action = "stop"
		plan.SuspectedOwnerStem = firstNonEmpty(suspectedStem, "unknown")
		plan.Reason = fmt.Sprintf(
			"current implementation branch has %d unmerged commit(s) ahead of merge root %q and target scope %q differs from suspected prior work %q; starting here would mix ticket work (not overridable by allow_rename)",
			obs.AheadOfMergeRoot, plan.MergeTarget, firstNonEmpty(n.TicketStem, "unspecified"), plan.SuspectedOwnerStem)
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
		if verdict.BranchPlan.SuspectedOwnerStem != "" {
			return fmt.Sprintf(
				"Stop before source edits. Do not rename over unmerged work. Resolve branch identity from session context, or dispatch an explore comparing %s's commit history to the target ticket, then re-invoke enter.implement. Suspected prior owner (branch-name encoded, best-effort): %s.",
				verdict.BranchPlan.CurrentBranch, verdict.BranchPlan.SuspectedOwnerStem)
		}
		return "Stop before source edits. Report the branch safety blocker in Branch Action and ask for the missing policy or branch cleanup."
	case "create":
		return fmt.Sprintf("Create %s from %s before source edits, then %s", verdict.BranchPlan.TargetBranch, verdict.BranchPlan.MergeTarget, nextAfterBranch)
	case "rename":
		return fmt.Sprintf("Rename the current branch to %s before source edits, then %s", verdict.BranchPlan.TargetBranch, nextAfterBranch)
	case "continue":
		return fmt.Sprintf("Continue on %s, then %s", verdict.BranchPlan.CurrentBranch, nextAfterBranch)
	case "current":
		return fmt.Sprintf("Keep the current branch %s, omit merge work, then %s", verdict.BranchPlan.CurrentBranch, nextAfterBranch)
	default:
		return "Stop before source edits. Report that the branch action is unrecognized."
	}
}

func implementNextAfterBranch(verdict implementVerdict) string {
	if verdict.Delegation == "direct-edit" {
		return fmt.Sprintf("run prep guardrails, apply direct edits in the lead context, run %s review, and complete %s documentation gates.", verdict.ReviewAlloc, verdict.DocMode)
	}
	return fmt.Sprintf("execute the installed delegated Prep and Edit todos, %s review, and %s documentation gates in order.", verdict.ReviewAlloc, verdict.DocMode)
}

func plannerAuthorityInputs(targetKind string) string {
	if strings.EqualFold(strings.TrimSpace(targetKind), "inline") {
		return `target_kind=inline, ticket_path="", selected_phase="", inline_contract, and plan_path`
	}
	return `target_kind=ticket, ticket_path, selected_phase, inline_contract="", and plan_path`
}

func implementConditions(n normalizedImplementFacts, branchAction string) []string {
	conditions := []string{
		"span=" + n.Span,
		"surface=" + n.Surface,
		"new-public-symbol=" + n.NewPublicSymbol,
		"new-type-contract=" + n.NewTypeContract,
		"test-surface=" + n.TestSurface,
		"explicit-delegation-request=" + n.ExplicitDelegationRequest,
		"explicit-direct-edit-request=" + n.ExplicitDirectEditRequest,
		"change-points=" + n.ChangePoints,
		"reuse-points=" + n.ReusePoints,
		"strategy-shape=" + n.StrategyShape,
		"side-effect-risk=" + n.SideEffectRisk,
		"correctness-risk=" + n.CorrectnessRisk,
		"fit-risk=" + n.FitRisk,
		"test-risk=" + n.TestRisk,
		"security-or-contract-risk=" + n.SecurityOrContractRisk,
		"low-ceremony-if-safe=" + n.LowCeremonyIfSafe,
		"review-override=" + n.ReviewOverride,
		"doc-mode-policy=" + n.DocModePolicy,
	}
	if branchAction == "current" {
		conditions = append(conditions, "merge-confirm=n/a")
	} else {
		conditions = append(conditions, "merge-confirm="+n.MergeConfirmPolicy)
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
	fmt.Fprintf(&b, "Merge Confirm: %s\n", implementMergeConfirmText(v.BranchPlan))
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
	fmt.Fprintf(&b, "- merge_confirm: %s\n", implementMergeConfirmText(result.Agenda.BranchPlan))
	fmt.Fprintf(&b, "- plan_depth: %s\n", result.Agenda.PlanDepth)
	fmt.Fprintf(&b, "- review_alloc: %s\n", result.Agenda.ReviewAlloc)
	fmt.Fprintf(&b, "- need_review: %t\n", result.Agenda.NeedReview)
	fmt.Fprintf(&b, "- doc_mode: %s\n", result.Agenda.DocMode)
	if result.Agenda.DocMode == "skipped" {
		fmt.Fprintf(&b, "- doc_reason: %s\n", result.Agenda.DocReason)
	}
	return b.String()
}

func implementMergeConfirmText(plan implementBranchPlan) string {
	if plan.Action == "current" {
		return "n/a"
	}
	return firstNonEmpty(plan.MergeConfirm, "ask")
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
