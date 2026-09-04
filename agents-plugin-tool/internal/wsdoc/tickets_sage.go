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
// Commit boundary: like tickets_mutate.go these functions never import wsgit,
// and neither of them produces a commit or commit metadata of any kind.
// SageGate's ask-decline path writes the "skipped" posture into the ticket
// frontmatter and returns; SageRecord writes the resolved posture and any
// Blocked section and returns. Both leave the write uncommitted for the
// caller's own next ordinary commit, which carries the posture together with
// whatever other edits the caller already holds on the same ticket file.
//
// Neither result struct carries a commit title/paths/ai_context. 260725
// established why for tickets.sage_stamp ({#260720-wsdoc-commit-boundary}): a
// canonical, bland title over a ticket file silently swallows the co-located
// real edits into a commit whose message describes only the posture flip. The
// decline path is the same shape — the posture flip is never the only
// uncommitted change on the ticket at that moment — so it gets the same
// treatment, and no separate commit for the posture flip is proposed at all.

// SageGateOptions carries the sage-review gate inputs.
type SageGateOptions struct {
	TicketStem string
	Landing    string // "idea" | "todo" | "ready"
	Answer     string // optional ask follow-up: "yes" | "no"
}

// SageGateResult is the gate decision. Action is the primary control value the
// caller follows; the remaining fields are populated per action.
type SageGateResult struct {
	Action    string   // "skip" | "stop_blocked" | "ask" | "run" | "check_review_required"
	AskPrompt string   // populated when Action == "ask"
	Reviewers []string // populated when Action == "run"; subset of {"design","completeness"}
	Mode      string   // "standalone" | "combined"; populated when Action == "run"

	// Advisory carries the non-waivable statement + review-scope line
	// (sageReviewNonWaivableAdvisory, tickets_mutate.go) on the ordinary
	// path an agent actually reaches: every "run" result (required's direct
	// run and a recommended stage's accepted "yes" run alike) and a
	// recommended stage's "ask" prompt, per ticket decision — not only on an
	// answer=="no" decline path, which posture "required" never reaches
	// (required never asks). A "run" result is a run result regardless of
	// which posture produced it, so the text is attached uniformly rather
	// than only on the "required" branch.
	Advisory string

	FreshnessStages   []string // populated when Action == "check_review_required"
	ReviewBaseline    string   // populated when Action == "check_review_required"
	ReviewInstruction string   // populated when Action == "check_review_required"
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

// SageRecordResult is the applied outcome. It writes the frontmatter posture
// (and any Blocked section) directly; it does not stage or commit — the MCP
// dispatch layer returns it as-is and the caller commits separately via its
// own ws/git.commit.
type SageRecordResult struct {
	Verdict        string            // aggregate: "pass" | "concern" | "block"
	Posture        map[string]string // frontmatter fields written
	BlockedSection string            // rendered Blocked section, empty when not blocked

	// Autonomous/Missing count the recorded issues by their `resolution` field
	// across the verdicts this stage actually consumed. Both reviewer playbooks
	// emit `resolution` precisely so the lead can split "I fix this myself" from
	// "a user has to decide this", but nothing consumed the field: the caller's
	// procedure never branched on it and the only prior reader was
	// anyIssueResolutionMissing's pass->concern escalation. Surfacing the counts
	// here lets the dispatch layer's next_instruction route them, which is where
	// post-call branch handling belongs (ai-docs/manuals/skill-authoring.md Layer 2)
	// rather than in restated playbook prose.
	Autonomous int
	Missing    int
}

// stageOutcome is the internal per-stage resolution used to compose the gate.
type stageOutcome struct {
	action    string // "run" | "ask" | "stop_blocked" | "skip"
	askPrompt string
	// advisory carries sageReviewNonWaivableAdvisory on every action=="run"
	// (posture "required", or posture "recommended" accepted via
	// answer=="yes") and on action=="ask" via posture "recommended" — a "run"
	// result carries the text regardless of which posture produced it, and
	// "ask" carries it per the ticket decision.
	advisory string
}

// SageGate resolves the sage-review gate for a landing, porting the
// lead-write-ticket gate + per-stage posture prose. resolvedSageReviewConfig is
// the config.list sage_review value resolved by the caller (used only for the
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

	// nil scope: the sage tools write frontmatter to the ticket file, so a
	// path they cannot open is no more actionable than a missing one. Leaving
	// them index-unaware keeps their behavior identical to today.
	ticketRel, _, _, err := findTicketPath(root, nil, stem)
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
		if design == "completed" {
			if result, err := sageGateFreshnessResult(root, ticketRel, []string{"design"}); err != nil {
				return SageGateResult{}, err
			} else if result.Action != "" {
				return result, nil
			}
		}
		return sageGateStandalone(ticketAbs, "design", "sage-review-design", design, resolvedSageReviewConfig, answer)
	}

	// landing == "ready" (including a requested todo/ -> ready/ promotion).
	if !designRequired && !completenessRequired {
		return SageGateResult{Action: "skip"}, nil
	}
	design, completeness := effectiveSageReviewPostures(frontmatter(ticketAbs))

	if designRequired && !completenessRequired {
		// epic: design-only. Skip when design posture is already terminal.
		if design == "completed" || design == "skipped" {
			if design == "completed" {
				if result, err := sageGateFreshnessResult(root, ticketRel, []string{"design"}); err != nil {
					return SageGateResult{}, err
				} else if result.Action != "" {
					return result, nil
				}
			}
			return SageGateResult{Action: "skip"}, nil
		}
		return sageGateStandalone(ticketAbs, "design", "sage-review-design", design, resolvedSageReviewConfig, answer)
	}

	// Both stages required.
	if design == "completed" || design == "skipped" {
		// Design already terminal: completeness stage stands alone.
		var completed []string
		if design == "completed" {
			completed = append(completed, "design")
		}
		if completeness == "completed" {
			completed = append(completed, "completeness")
		}
		if len(completed) > 0 {
			if result, err := sageGateFreshnessResult(root, ticketRel, completed); err != nil {
				return SageGateResult{}, err
			} else if result.Action != "" {
				return result, nil
			}
		}
		return sageGateStandalone(ticketAbs, "completeness", "sage-review-completeness", completeness, resolvedSageReviewConfig, answer)
	}
	// Design not yet terminal: the never-skippable design invariant fires for a
	// ticket that reached ready without a prior todo design pass. Run design +
	// completeness in combined mode.
	return sageGateCombined(ticketAbs, design, completeness, resolvedSageReviewConfig, answer)
}

// resolveConcretePosture returns the effective posture for a stage, applying the
// missing/pending config.list fallback and persisting the resolved value.
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
func resolveStage(ticketAbs, reviewer, field, posture, resolvedConfig, answer string) (stageOutcome, error) {
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
			return stageOutcome{action: "run", advisory: sageReviewNonWaivableAdvisory}, nil
		case "no":
			// Decline: persist "skipped" and return. No commit and no commit
			// metadata — the write rides the caller's next ordinary commit of
			// the ticket path (see the file header's commit-boundary note).
			if err := writeFrontmatterField(ticketAbs, map[string]string{field: "skipped"}); err != nil {
				return stageOutcome{}, err
			}
			return stageOutcome{action: "skip"}, nil
		default:
			return stageOutcome{action: "ask", askPrompt: "Run " + reviewer + " review for this ticket?", advisory: sageReviewNonWaivableAdvisory}, nil
		}
	case "required":
		return stageOutcome{action: "run", advisory: sageReviewNonWaivableAdvisory}, nil
	default:
		// Unknown/unexpected posture value: treat as a no-op skip rather than
		// inventing behavior; the caller (lead) can inspect the frontmatter.
		return stageOutcome{action: "skip"}, nil
	}
}

// sageGateStandalone resolves a single stage and maps it to a gate result.
func sageGateStandalone(ticketAbs, reviewer, field, posture, resolvedConfig, answer string) (SageGateResult, error) {
	out, err := resolveStage(ticketAbs, reviewer, field, posture, resolvedConfig, answer)
	if err != nil {
		return SageGateResult{}, err
	}
	return gateResultFromStage(out, reviewer, "standalone"), nil
}

func gateResultFromStage(out stageOutcome, reviewer, mode string) SageGateResult {
	switch out.action {
	case "run":
		return SageGateResult{Action: "run", Reviewers: []string{reviewer}, Mode: mode, Advisory: out.advisory}
	case "ask":
		return SageGateResult{Action: "ask", AskPrompt: out.askPrompt, Advisory: out.advisory}
	case "stop_blocked":
		return SageGateResult{Action: "stop_blocked"}
	default: // skip
		return SageGateResult{Action: "skip"}
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
func sageGateCombined(ticketAbs, design, completeness, resolvedConfig, answer string) (SageGateResult, error) {
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
		return sageGateStandalone(ticketAbs, "completeness", "sage-review-completeness", completeness, resolvedConfig, answer)
	}

	// Design is non-terminal (recommended | required). Resolve its ask first so a
	// recommended completeness is never resolved by design's answer.
	if dp == "recommended" {
		switch answer {
		case "":
			return SageGateResult{Action: "ask", AskPrompt: "Run design review for this ticket?", Advisory: sageReviewNonWaivableAdvisory}, nil
		case "no":
			if err := writeFrontmatterField(ticketAbs, map[string]string{"sage-review-design": "skipped"}); err != nil {
				return SageGateResult{}, err
			}
			// Design declined -> terminal, written and left uncommitted.
			// Completeness stands alone with its own fresh ask (the answer was
			// consumed by design, so pass "").
			return sageGateStandalone(ticketAbs, "completeness", "sage-review-completeness", completeness, resolvedConfig, "")
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
		return SageGateResult{Action: "run", Reviewers: []string{"design"}, Mode: "standalone", Advisory: sageReviewNonWaivableAdvisory}, nil
	case "recommended":
		switch answer {
		case "":
			return SageGateResult{Action: "ask", AskPrompt: "Run completeness review for this ticket?", Advisory: sageReviewNonWaivableAdvisory}, nil
		case "no":
			if err := writeFrontmatterField(ticketAbs, map[string]string{"sage-review-completeness": "skipped"}); err != nil {
				return SageGateResult{}, err
			}
			return SageGateResult{Action: "run", Reviewers: []string{"design"}, Mode: "standalone", Advisory: sageReviewNonWaivableAdvisory}, nil
		default: // yes
			return SageGateResult{Action: "run", Reviewers: []string{"design", "completeness"}, Mode: "combined", Advisory: sageReviewNonWaivableAdvisory}, nil
		}
	default: // required
		return SageGateResult{Action: "run", Reviewers: []string{"design", "completeness"}, Mode: "combined", Advisory: sageReviewNonWaivableAdvisory}, nil
	}
}

// SageRecord aggregates reviewer verdicts, writes the resolved posture(s), and
// renders any Blocked section. It commits nothing and returns no commit
// metadata (260725, {#260720-wsdoc-commit-boundary}).
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

	// nil scope: the sage tools write frontmatter to the ticket file, so a
	// path they cannot open is no more actionable than a missing one. Leaving
	// them index-unaware keeps their behavior identical to today.
	ticketRel, _, _, err := findTicketPath(root, nil, stem)
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
	res := SageRecordResult{Verdict: verdict, Posture: map[string]string{}}
	res.Autonomous, res.Missing = countIssueResolutions(v)

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
		return res, nil
	}

	// pass or concern resolved to pass.
	res.Posture[field] = "completed"
	digest, err := sageReviewCurrentBodyDigest(ticketAbs)
	if err != nil {
		return SageRecordResult{}, err
	}
	if err := writeFrontmatterField(ticketAbs, map[string]string{field: "completed", field + "-reviewed": digest}); err != nil {
		return SageRecordResult{}, err
	}
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

	res := SageRecordResult{Verdict: final, Posture: map[string]string{}}
	res.Autonomous, res.Missing = countIssueResolutions(d, c)

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
		return res, nil
	}

	res.Posture["sage-review-design"] = "completed"
	res.Posture["sage-review-completeness"] = "completed"
	digest, err := sageReviewCurrentBodyDigest(ticketAbs)
	if err != nil {
		return SageRecordResult{}, err
	}
	if err := writeFrontmatterField(ticketAbs, map[string]string{
		"sage-review-design":                "completed",
		"sage-review-completeness":          "completed",
		"sage-review-design-reviewed":       digest,
		"sage-review-completeness-reviewed": digest,
	}); err != nil {
		return SageRecordResult{}, err
	}
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

// countIssueResolutions tallies issues by `resolution` across the verdicts a
// stage consumed. An issue whose resolution is absent or unrecognized counts as
// autonomous: the lead attempting a fix it cannot make is recoverable, whereas
// silently dropping the issue from both buckets would hide it from routing.
func countIssueResolutions(verdicts ...SageVerdict) (autonomous, missing int) {
	for _, v := range verdicts {
		for _, issue := range v.Issues {
			if strings.EqualFold(strings.TrimSpace(issue.Resolution), "missing") {
				missing++
				continue
			}
			autonomous++
		}
	}
	return autonomous, missing
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
// of the ticket body, first excising any prior "## Blocked (" section. A prior
// section is not reliably last: a lead records what changed while the ticket
// waited by adding "## " sections after it, so excision runs from the Blocked
// heading only up to the next "## " heading (or EOF when it is genuinely last).
// Truncating to EOF instead would delete that later, unrelated content. Every
// prior Blocked section is excised rather than only the first, so a hand-placed
// duplicate cannot survive as a stale blocker beside the fresh one.
func appendOrReplaceBlockedSection(path, section string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lines := strings.Split(string(raw), "\n")
	kept := make([]string, 0, len(lines))
	for i := 0; i < len(lines); i++ {
		if !strings.HasPrefix(lines[i], "## Blocked (") {
			kept = append(kept, lines[i])
			continue
		}
		for i+1 < len(lines) && !strings.HasPrefix(lines[i+1], "## ") {
			i++
		}
	}
	body := strings.TrimRight(strings.Join(kept, "\n"), "\n")
	return os.WriteFile(path, []byte(body+"\n\n"+section+"\n"), 0o644)
}
