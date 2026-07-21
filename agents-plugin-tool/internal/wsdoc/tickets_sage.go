package wsdoc

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// tickets_sage.go relocates the lead-write-ticket "On: Sage Review Gate" /
// "Design Review Stage" / "Completeness Review Stage" / "Ready-promotion
// Aggregation" prose state-machine and its three Blocked Section Templates into
// two Go tools: SageGate (posture resolution + gate decision) and SageRecord
// (verdict aggregation + frontmatter/Blocked-section write). Reuses the posture
// helpers already living in tickets_mutate.go (sageReviewStageRequirement,
// effectiveSageReviewPostures, ResolvedSageReviewPosture, writeFrontmatterField)
// so the two mechanisms cannot drift.
//
// Commit boundary: like tickets_mutate.go these functions never import wsgit;
// they compute the frontmatter write, the Blocked-section render, and the commit
// title/paths/ai_context, and return them for the MCP dispatch layer to actually
// commit via wsgit.NewClient().Commit(...). This keeps commit output byte-for-byte
// identical to today's git.commit-produced commits.

// SageGateOptions carries the sage-review gate inputs.
type SageGateOptions struct {
	TicketStem string
	Landing    string // "idea" | "todo" | "ready"
	Answer     string // optional ask follow-up: "yes" | "no"
}

// SageGateResult is the gate decision. Action is the primary control value the
// caller follows; the remaining fields are populated per action.
type SageGateResult struct {
	Action    string   // "skip" | "stop_blocked" | "ask" | "run"
	AskPrompt string   // populated when Action == "ask"
	Reviewers []string // populated when Action == "run"; subset of {"design","completeness"}
	Mode      string   // "standalone" | "combined"; populated when Action == "run"

	// Commit metadata is populated only on the ask-decline path (recommended
	// posture + Answer=="no"), where the legacy prose persisted `skipped` and
	// committed a small standalone commit. Empty CommitTitle means no commit.
	CommitTitle string
	CommitPaths []string
	AIContext   []string
}

// SageIssue is one reviewer-reported issue row.
type SageIssue struct {
	Title      string `json:"title"`
	Severity   string `json:"severity"`
	Resolution string `json:"resolution"`
}

// SageVerdict is a single reviewer's result.
type SageVerdict struct {
	Reviewer string      `json:"reviewer"`
	Verdict  string      `json:"verdict"`
	Issues   []SageIssue `json:"issues"`
}

// SageRecordOptions carries the post-review aggregation inputs.
type SageRecordOptions struct {
	TicketStem string
	Stage      string // "design" | "completeness" | "combined"
	Verdicts   []SageVerdict
	Today      string // YYYY-MM-DD, caller-supplied for testability
}

// SageRecordResult is the applied outcome. The MCP layer commits CommitPaths
// under CommitTitle/AIContext.
type SageRecordResult struct {
	Verdict        string            // aggregate: "pass" | "concern" | "block"
	Posture        map[string]string // frontmatter fields written
	BlockedSection string            // rendered Blocked section, empty when not blocked
	CommitTitle    string
	CommitPaths    []string
	AIContext      []string
}

// stageOutcome is the internal per-stage resolution used to compose the gate.
type stageOutcome struct {
	action    string // "run" | "ask" | "stop_blocked" | "skip"
	askPrompt string
	// commit* set only when action=="skip" via an ask-decline.
	commitTitle string
	commitPaths []string
	aiContext   []string
}

// SageGate resolves the sage-review gate for a landing, porting the
// lead-write-ticket gate + per-stage posture prose. resolvedSageReviewConfig is
// the config.show sage_review value resolved by the caller (used only for the
// missing/pending config-fallback branch).
func SageGate(root string, opts SageGateOptions, resolvedSageReviewConfig string) (SageGateResult, error) {
	stem := strings.TrimSpace(opts.TicketStem)
	if !ticketStemRE.MatchString(stem) {
		return SageGateResult{}, fmt.Errorf("stem must be a ticket stem")
	}
	landing := strings.TrimSpace(opts.Landing)
	switch landing {
	case "idea", "todo", "ready":
	default:
		return SageGateResult{}, fmt.Errorf("landing must be idea, todo, or ready")
	}
	answer := strings.ToLower(strings.TrimSpace(opts.Answer))
	switch answer {
	case "", "yes", "no":
	default:
		return SageGateResult{}, fmt.Errorf("answer must be yes or no")
	}

	// idea/ landing: skip the gate entirely.
	if landing == "idea" {
		return SageGateResult{Action: "skip"}, nil
	}

	designRequired, completenessRequired := sageReviewStageRequirement(stem)

	ticketRel, _, err := findTicketPath(root, stem)
	if err != nil {
		return SageGateResult{}, err
	}
	ticketAbs := filepath.Join(root, filepath.FromSlash(ticketRel))

	if landing == "todo" {
		// Design-stage-exempt categories (research/workset) skip entirely.
		if !designRequired {
			return SageGateResult{Action: "skip"}, nil
		}
		design, _ := effectiveSageReviewPostures(frontmatter(ticketAbs))
		return sageGateStandalone(ticketAbs, ticketRel, "design", "sage-review-design", design, resolvedSageReviewConfig, answer)
	}

	// landing == "ready" (including a requested todo/ -> ready/ promotion).
	if !designRequired && !completenessRequired {
		return SageGateResult{Action: "skip"}, nil
	}
	design, completeness := effectiveSageReviewPostures(frontmatter(ticketAbs))

	if designRequired && !completenessRequired {
		// epic: design-only. Skip when design posture is already terminal.
		if design == "completed" || design == "skipped" {
			return SageGateResult{Action: "skip"}, nil
		}
		return sageGateStandalone(ticketAbs, ticketRel, "design", "sage-review-design", design, resolvedSageReviewConfig, answer)
	}

	// Both stages required.
	if design == "completed" || design == "skipped" {
		// Design already terminal: completeness stage stands alone.
		return sageGateStandalone(ticketAbs, ticketRel, "completeness", "sage-review-completeness", completeness, resolvedSageReviewConfig, answer)
	}
	// Design not yet terminal: the never-skippable design invariant fires for a
	// ticket that reached ready without a prior todo design pass. Run design +
	// completeness in combined mode.
	return sageGateCombined(ticketAbs, ticketRel, design, completeness, resolvedSageReviewConfig, answer)
}

// resolveConcretePosture returns the effective posture for a stage, applying the
// missing/pending config.show fallback and persisting the resolved value.
func resolveConcretePosture(ticketAbs, field, posture, resolvedConfig string) (string, error) {
	p := strings.TrimSpace(posture)
	if p == "" || p == "pending" {
		p = ResolvedSageReviewPosture(resolvedConfig)
		if err := writeFrontmatterField(ticketAbs, map[string]string{field: p}); err != nil {
			return "", err
		}
	}
	return p, nil
}

// resolveStage ports the shared per-stage posture branches (Design/Completeness
// Review Stage steps 1-7): config-fallback resolve+persist for missing/pending,
// terminal no-op, blocked stop, recommended ask/accept/decline, required run.
func resolveStage(ticketAbs, ticketRel, reviewer, field, posture, resolvedConfig, answer string) (stageOutcome, error) {
	p, err := resolveConcretePosture(ticketAbs, field, posture, resolvedConfig)
	if err != nil {
		return stageOutcome{}, err
	}
	switch p {
	case "skipped", "completed":
		return stageOutcome{action: "skip"}, nil
	case "blocked":
		return stageOutcome{action: "stop_blocked"}, nil
	case "recommended":
		switch answer {
		case "yes":
			return stageOutcome{action: "run"}, nil
		case "no":
			if err := writeFrontmatterField(ticketAbs, map[string]string{field: "skipped"}); err != nil {
				return stageOutcome{}, err
			}
			return stageOutcome{
				action:      "skip",
				commitTitle: "chore(sage): skip " + reviewer + " review",
				commitPaths: []string{ticketRel},
				aiContext:   []string{"user declined " + reviewer + " review in ask mode"},
			}, nil
		default:
			return stageOutcome{action: "ask", askPrompt: "Run " + reviewer + " review for this ticket?"}, nil
		}
	case "required":
		return stageOutcome{action: "run"}, nil
	default:
		// Unknown/unexpected posture value: treat as a no-op skip rather than
		// inventing behavior; the caller (lead) can inspect the frontmatter.
		return stageOutcome{action: "skip"}, nil
	}
}

// sageGateStandalone resolves a single stage and maps it to a gate result.
func sageGateStandalone(ticketAbs, ticketRel, reviewer, field, posture, resolvedConfig, answer string) (SageGateResult, error) {
	out, err := resolveStage(ticketAbs, ticketRel, reviewer, field, posture, resolvedConfig, answer)
	if err != nil {
		return SageGateResult{}, err
	}
	return gateResultFromStage(out, reviewer, "standalone"), nil
}

func gateResultFromStage(out stageOutcome, reviewer, mode string) SageGateResult {
	switch out.action {
	case "run":
		return SageGateResult{Action: "run", Reviewers: []string{reviewer}, Mode: mode}
	case "ask":
		return SageGateResult{Action: "ask", AskPrompt: out.askPrompt}
	case "stop_blocked":
		return SageGateResult{Action: "stop_blocked"}
	default: // skip
		return SageGateResult{Action: "skip", CommitTitle: out.commitTitle, CommitPaths: out.commitPaths, AIContext: out.aiContext}
	}
}

// sageGateCombined resolves the ready/ both-stage combined case (design not yet
// terminal). Each recommended stage gets its own ask, design first: the supplied
// Answer resolves only the first still-pending stage, and a re-entry call
// resolves the next. An accepted design recommendation is persisted as
// `required` so the re-entry that asks completeness does not re-ask design; that
// transient marker is overwritten to completed/blocked by sage_record.
//
// The ask/decline round-trip in combined mode is underspecified by the source
// prose; this resolution keeps each stage's decision independent (the deleted
// prose asked each stage separately) so a design answer never silently resolves
// the completeness stage.
func sageGateCombined(ticketAbs, ticketRel, design, completeness, resolvedConfig, answer string) (SageGateResult, error) {
	dp, err := resolveConcretePosture(ticketAbs, "sage-review-design", design, resolvedConfig)
	if err != nil {
		return SageGateResult{}, err
	}
	if dp == "blocked" {
		return SageGateResult{Action: "stop_blocked"}, nil
	}
	if dp == "completed" || dp == "skipped" {
		// Design resolved to terminal via the config fallback: completeness
		// stands alone and the supplied answer belongs to it.
		return sageGateStandalone(ticketAbs, ticketRel, "completeness", "sage-review-completeness", completeness, resolvedConfig, answer)
	}

	// Design is non-terminal (recommended | required). Resolve its ask first so a
	// recommended completeness is never resolved by design's answer.
	var designDecline stageOutcome
	if dp == "recommended" {
		switch answer {
		case "":
			return SageGateResult{Action: "ask", AskPrompt: "Run design review for this ticket?"}, nil
		case "no":
			if err := writeFrontmatterField(ticketAbs, map[string]string{"sage-review-design": "skipped"}); err != nil {
				return SageGateResult{}, err
			}
			designDecline = stageOutcome{
				commitTitle: "chore(sage): skip design review",
				commitPaths: []string{ticketRel},
				aiContext:   []string{"user declined design review in ask mode"},
			}
			// Design declined -> terminal. Completeness stands alone with its own
			// fresh ask (the answer was consumed by design, so pass "").
			res, err := sageGateStandalone(ticketAbs, ticketRel, "completeness", "sage-review-completeness", completeness, resolvedConfig, "")
			if err != nil {
				return SageGateResult{}, err
			}
			return mergeGateCommit(res, designDecline), nil
		default: // yes
			if err := writeFrontmatterField(ticketAbs, map[string]string{"sage-review-design": "required"}); err != nil {
				return SageGateResult{}, err
			}
			dp = "required"
			answer = "" // consumed by design; completeness asks separately
		}
	}

	// dp == "required": design will run. The (possibly reset) answer now belongs
	// to the completeness stage.
	cp, err := resolveConcretePosture(ticketAbs, "sage-review-completeness", completeness, resolvedConfig)
	if err != nil {
		return SageGateResult{}, err
	}
	switch cp {
	case "blocked":
		return SageGateResult{Action: "stop_blocked"}, nil
	case "completed", "skipped":
		// Intentional divergence from the pre-diff prose: when design is
		// non-terminal but completeness is already terminal, only design runs
		// (standalone) rather than always-combined. This state cannot arise under
		// the never-skippable-design invariant in normal flow; the standalone
		// path is the more correct outcome for the anomaly.
		return SageGateResult{Action: "run", Reviewers: []string{"design"}, Mode: "standalone"}, nil
	case "recommended":
		switch answer {
		case "":
			return SageGateResult{Action: "ask", AskPrompt: "Run completeness review for this ticket?"}, nil
		case "no":
			if err := writeFrontmatterField(ticketAbs, map[string]string{"sage-review-completeness": "skipped"}); err != nil {
				return SageGateResult{}, err
			}
			res := SageGateResult{Action: "run", Reviewers: []string{"design"}, Mode: "standalone"}
			return mergeGateCommit(res, stageOutcome{
				commitTitle: "chore(sage): skip completeness review",
				commitPaths: []string{ticketRel},
				aiContext:   []string{"user declined completeness review in ask mode"},
			}), nil
		default: // yes
			return SageGateResult{Action: "run", Reviewers: []string{"design", "completeness"}, Mode: "combined"}, nil
		}
	default: // required
		return SageGateResult{Action: "run", Reviewers: []string{"design", "completeness"}, Mode: "combined"}, nil
	}
}

// mergeGateCommit folds a declined-stage commit (from `extra`) into a gate
// result. The single-commit path is the reachable one: after FIX 1 each stage's
// decline consumes its own answer round-trip, so SageGate never produces two
// decline commits in one call. The dual-commit branch is a defensive guard (both
// stages carrying commit metadata) kept for callers that combine independently
// declined outcomes; it is exercised directly by unit test.
func mergeGateCommit(res SageGateResult, extra stageOutcome) SageGateResult {
	if extra.commitTitle == "" {
		return res
	}
	if res.CommitTitle == "" {
		res.CommitTitle = extra.commitTitle
		res.CommitPaths = extra.commitPaths
		res.AIContext = extra.aiContext
		return res
	}
	res.CommitTitle = "chore(sage): skip sage review"
	res.AIContext = append(append([]string{}, extra.aiContext...), res.AIContext...)
	if len(res.CommitPaths) == 0 {
		res.CommitPaths = extra.commitPaths
	}
	return res
}

// SageRecord aggregates reviewer verdicts, writes the resolved posture(s),
// renders any Blocked section, and returns the commit metadata for the MCP layer.
func SageRecord(root string, opts SageRecordOptions) (SageRecordResult, error) {
	stem := strings.TrimSpace(opts.TicketStem)
	if !ticketStemRE.MatchString(stem) {
		return SageRecordResult{}, fmt.Errorf("stem must be a ticket stem")
	}
	stage := strings.TrimSpace(opts.Stage)
	switch stage {
	case "design", "completeness", "combined":
	default:
		return SageRecordResult{}, fmt.Errorf("stage must be design, completeness, or combined")
	}

	ticketRel, _, err := findTicketPath(root, stem)
	if err != nil {
		return SageRecordResult{}, err
	}
	ticketAbs := filepath.Join(root, filepath.FromSlash(ticketRel))

	switch stage {
	case "design":
		return sageRecordSingle(ticketAbs, ticketRel, opts.Today, "design", "sage-review-design", "Design Reviewer", true, opts.Verdicts)
	case "completeness":
		return sageRecordSingle(ticketAbs, ticketRel, opts.Today, "completeness", "sage-review-completeness", "Completeness Reviewer", false, opts.Verdicts)
	default:
		return sageRecordCombined(ticketAbs, ticketRel, opts.Today, opts.Verdicts)
	}
}

// sageRecordSingle handles the standalone design or completeness stage.
func sageRecordSingle(ticketAbs, ticketRel, today, reviewer, field, heading string, withResolution bool, verdicts []SageVerdict) (SageRecordResult, error) {
	v, ok := findVerdict(verdicts, reviewer)
	if !ok {
		// A single verdict for a single-stage record needs no reviewer tag; any
		// other absence is an error rather than a silent "pass" that would mark a
		// review completed that never ran.
		if len(verdicts) == 1 {
			v = verdicts[0]
		} else {
			return SageRecordResult{}, fmt.Errorf("no %s reviewer verdict supplied", reviewer)
		}
	}
	verdict := normalizeVerdict(v.Verdict)
	res := SageRecordResult{Verdict: verdict, Posture: map[string]string{}, CommitPaths: []string{ticketRel}}

	if verdict == "block" {
		section := renderBlockedSection(today, []blockedReviewerSection{
			{Heading: heading, Verdict: v.Verdict, Issues: v.Issues, WithResolution: withResolution},
		})
		if err := appendOrReplaceBlockedSection(ticketAbs, section); err != nil {
			return SageRecordResult{}, err
		}
		res.BlockedSection = section
		res.Posture[field] = "blocked"
		if err := writeFrontmatterField(ticketAbs, map[string]string{field: "blocked"}); err != nil {
			return SageRecordResult{}, err
		}
		res.CommitTitle = "docs(sage): block ticket on " + reviewer + " review"
		res.AIContext = []string{reviewer + " review blocked"}
		return res, nil
	}

	// pass or concern resolved to pass.
	res.Posture[field] = "completed"
	if err := writeFrontmatterField(ticketAbs, map[string]string{field: "completed"}); err != nil {
		return SageRecordResult{}, err
	}
	res.CommitTitle = "docs(sage): mark " + reviewer + " review completed"
	res.AIContext = []string{reviewer + " review passed"}
	return res, nil
}

// sageRecordCombined handles the combined ready-promotion aggregation across
// both reviewer verdicts (Ready-promotion Aggregation prose).
func sageRecordCombined(ticketAbs, ticketRel, today string, verdicts []SageVerdict) (SageRecordResult, error) {
	d, dok := findVerdict(verdicts, "design")
	c, cok := findVerdict(verdicts, "completeness")
	if !dok || !cok {
		return SageRecordResult{}, fmt.Errorf("combined stage requires both design and completeness reviewer verdicts")
	}
	dv := normalizeVerdict(d.Verdict)
	cv := normalizeVerdict(c.Verdict)

	final := "pass"
	switch {
	case dv == "block" || cv == "block":
		final = "block"
	case anyIssueResolutionMissing(d.Issues) || anyIssueResolutionMissing(c.Issues):
		// concern+concern/pass with a missing decision escalates to concern; the
		// write path still marks completed (concern resolved to pass), and the
		// concern is surfaced so the lead may escalate to block manually.
		final = "concern"
	default:
		final = "pass"
	}

	res := SageRecordResult{Verdict: final, Posture: map[string]string{}, CommitPaths: []string{ticketRel}}

	if final == "block" {
		section := renderBlockedSection(today, []blockedReviewerSection{
			{Heading: "Design Reviewer", Verdict: d.Verdict, Issues: d.Issues, WithResolution: true},
			{Heading: "Completeness Reviewer", Verdict: c.Verdict, Issues: c.Issues, WithResolution: false},
		})
		if err := appendOrReplaceBlockedSection(ticketAbs, section); err != nil {
			return SageRecordResult{}, err
		}
		res.BlockedSection = section
		res.Posture["sage-review-design"] = "blocked"
		res.Posture["sage-review-completeness"] = "blocked"
		if err := writeFrontmatterField(ticketAbs, map[string]string{"sage-review-design": "blocked", "sage-review-completeness": "blocked"}); err != nil {
			return SageRecordResult{}, err
		}
		res.CommitTitle = "docs(sage): block ticket on sage review"
		res.AIContext = []string{"sage review blocked: design and/or completeness issues"}
		return res, nil
	}

	res.Posture["sage-review-design"] = "completed"
	res.Posture["sage-review-completeness"] = "completed"
	if err := writeFrontmatterField(ticketAbs, map[string]string{"sage-review-design": "completed", "sage-review-completeness": "completed"}); err != nil {
		return SageRecordResult{}, err
	}
	res.CommitTitle = "docs(sage): mark sage review completed"
	res.AIContext = []string{"sage review passed"}
	return res, nil
}

// findVerdict returns the verdict tagged for the named reviewer. Matching is by
// reviewer name only (no positional fallback) so a mistagged or absent verdict
// is never silently substituted for another stage's.
func findVerdict(verdicts []SageVerdict, reviewer string) (SageVerdict, bool) {
	for _, v := range verdicts {
		if strings.Contains(strings.ToLower(strings.TrimSpace(v.Reviewer)), reviewer) {
			return v, true
		}
	}
	return SageVerdict{}, false
}

func normalizeVerdict(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "block":
		return "block"
	case "concern":
		return "concern"
	default:
		return "pass"
	}
}

func anyIssueResolutionMissing(issues []SageIssue) bool {
	for _, issue := range issues {
		if strings.EqualFold(strings.TrimSpace(issue.Resolution), "missing") {
			return true
		}
	}
	return false
}

// blockedReviewerSection is one reviewer subsection of a Blocked section.
type blockedReviewerSection struct {
	Heading        string
	Verdict        string
	Issues         []SageIssue
	WithResolution bool
}

// renderBlockedSection reproduces the three relocated Blocked Section Templates
// byte-for-byte. The Design reviewer table carries a Resolution column; the
// Completeness reviewer table does not. Multiple sections (combined) are joined
// by a single blank line.
func renderBlockedSection(today string, sections []blockedReviewerSection) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Blocked (%s)\n", today)
	for _, sec := range sections {
		b.WriteString("\n")
		fmt.Fprintf(&b, "### %s — %s\n\n", sec.Heading, sec.Verdict)
		if sec.WithResolution {
			b.WriteString("| # | Title | Severity | Resolution |\n")
			b.WriteString("|---|-------|----------|------------|\n")
			for i, issue := range sec.Issues {
				fmt.Fprintf(&b, "| %d | %s | %s | %s |\n", i+1, issue.Title, issue.Severity, issue.Resolution)
			}
		} else {
			b.WriteString("| # | Title | Severity |\n")
			b.WriteString("|---|-------|----------|\n")
			for i, issue := range sec.Issues {
				fmt.Fprintf(&b, "| %d | %s | %s |\n", i+1, issue.Title, issue.Severity)
			}
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// appendOrReplaceBlockedSection appends the rendered Blocked section at the end
// of the ticket body, replacing any prior "## Blocked (" section (they are
// always appended last, so a prior one is truncated from its heading to EOF).
func appendOrReplaceBlockedSection(path, section string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	text := string(raw)
	lines := strings.Split(text, "\n")
	cut := -1
	for i, line := range lines {
		if strings.HasPrefix(line, "## Blocked (") {
			cut = i
			break
		}
	}
	if cut >= 0 {
		text = strings.Join(lines[:cut], "\n")
	}
	body := strings.TrimRight(text, "\n")
	return os.WriteFile(path, []byte(body+"\n\n"+section+"\n"), 0o644)
}
