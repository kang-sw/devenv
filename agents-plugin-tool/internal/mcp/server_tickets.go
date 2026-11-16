package mcp

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/kang-sw/devenv/internal/wsconfig"
	"github.com/kang-sw/devenv/internal/wsdoc"
	"github.com/kang-sw/devenv/internal/wsgit"
)

// assigneeFeature resolves the two inputs the ticket-assignee-awareness feature
// depends on: the committed ticket-assignee-aware project flag (repo scope,
// anchored at the session's worktree root) and, when that flag is on, the
// caller's current git user.email. It is the single reader of both so
// tickets.query and tickets.create_empty agree on when the feature is active and
// on which identity it compares against. Flag off (the default) ⇒ aware=false
// and an empty email, and every caller treats the feature as inert. A config
// load error is returned rather than read as "off": a malformed committed file
// must not silently drop the ownership gate.
func (s *Server) assigneeFeature(root, sessionKey string) (aware bool, currentEmail string, err error) {
	r := s.sessionResolver(sessionKey, builtinConfigDefaults())
	resolved, err := r.Get(sessionKey, wsconfig.ItemTicketAssigneeAware)
	if err != nil {
		return false, "", err
	}
	if strings.TrimSpace(resolved.Value) != "on" {
		return false, "", nil
	}
	return true, wsgit.CurrentUserEmail(context.Background(), s.gitRunner(), root), nil
}

// applyAssigneeGate attaches the computed ownership gate to every ticket in the
// projection, using the caller's current identity. Called only when the feature
// is on, so an absent AssigneeGate on any projection means the feature is off.
func applyAssigneeGate(tickets []wsdoc.TicketInfo, currentEmail string) {
	for i := range tickets {
		gate := wsdoc.AssigneeGateFor(tickets[i].Assignee, currentEmail)
		tickets[i].AssigneeGate = &gate
	}
}

// resolveSetAssignee turns tickets.create_empty's set_assignee argument into the
// assignee list to stamp into the new ticket. The argument is
// true | false | string[], default true. The whole behavior is subordinate to
// the ticket-assignee-aware flag: aware=false stamps nothing. When aware: true
// (or absent) stamps the caller's current git email — nothing when that email
// is empty (CI/bot ⇒ assign-any); false stamps nothing; a string[] stamps those
// emails verbatim (blank entries dropped by the create layer).
func resolveSetAssignee(raw any, present, aware bool, currentEmail string) []string {
	if !aware {
		return nil
	}
	stampSelf := func() []string {
		if strings.TrimSpace(currentEmail) == "" {
			return nil
		}
		return []string{currentEmail}
	}
	if !present {
		return stampSelf()
	}
	switch v := raw.(type) {
	case bool:
		if v {
			return stampSelf()
		}
		return nil
	default:
		return stringList(raw)
	}
}

// formatTicketCreate's next_instruction line carries the acceptance-check
// caveat verbatim ("valid empty skeleton + applicable initial posture") so a caller
// never mistakes tickets.create_empty for a full mutation orchestrator —
// tickets.template owns the body skeleton, and this tool never renders one.
func formatTicketCreate(res wsdoc.TicketCreateResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Created %s\n", res.Path)
	if res.Warning != "" {
		fmt.Fprintf(&b, "warning: %s\n", res.Warning)
	}
	fmt.Fprintf(&b, "Tip: %s\n", res.Tip)
	b.WriteString("next_instruction: This is a valid empty skeleton + applicable initial posture only, not a full mutation orchestrator; call tickets.template for the body skeleton before treating this ticket as populated.\n")
	return b.String()
}

func formatTicketMutate(verb string, result wsdoc.TicketMutateResult) string {
	stem := strings.TrimSuffix(filepath.Base(result.NewPath), ".md")
	var b strings.Builder
	fmt.Fprintf(&b, "%s: %s\n  %s -> %s\n", verb, stem, result.OldPath, result.NewPath)
	if result.Tip != "" {
		fmt.Fprintf(&b, "tip: %s\n", result.Tip)
	}
	if instruction := ticketMutateNextInstruction(verb, result); instruction != "" {
		fmt.Fprintf(&b, "next_instruction: %s\n", instruction)
	}
	return b.String()
}

// ticketMutateNextInstruction surfaces the action-time follow-on for a
// tickets.close/tickets.move result, replacing front-loaded playbook prose
// that used to restate these obligations up front (260723 Phase 2). It
// returns "" when there is no must-know follow-on beyond the plain tip.
func ticketMutateNextInstruction(verb string, result wsdoc.TicketMutateResult) string {
	switch {
	case verb == "moved" && strings.Contains(filepath.ToSlash(result.NewPath), "/ready/"):
		return "Call tickets.sage_gate(stem, landing) next to resolve the sage-review gate for this promotion."
	case verb == "closed" && strings.Contains(result.Tip, "unresolved phase"):
		return "This is a soft warning only (no block); resolve or explicitly accept the unresolved phase(s) before treating this ticket as truly closed."
	default:
		return ""
	}
}

// verifyAdapter adapts wsdoc.TicketVerify's richer VerifyResult into the
// wsgit.Client.Verifier shape, so wsgit.Commit can veto a commit without
// importing internal/wsdoc directly (see {#260720-wsdoc-commit-boundary}).
// Wired into both the git.commit dispatch case below and the CLI gitCommit
// handler (via VerifyAdapter) so every ws.git.commit entry point is gated
// identically. On the pass branch, result.Warnings is formatted into the
// advisories return value using the same "WARN [%s] %s: %s" text shape as
// formatTicketVerify, so a warning reads identically whether seen via
// tickets.verify or via git.commit.
//
// result.Advisories (the cross-file ticket-graph pass) rides the same channel
// verbatim, with one commit-path-only presentation detail: an advisory whose
// Kind is a mechanical remedy gets the amend recipe appended, because at this
// layer the commit already exists. The recipe is deliberately not part of the
// check text, so standalone ws/tickets.verify — where nothing has been
// committed and an amend instruction would be nonsense — omits it. The verdict
// and the advisory set are identical on both paths, which is what
// {#260723-tickets-verify-tool}'s identical-verdict guarantee requires.
func verifyAdapter(root string, paths []string) ([]string, error) {
	return verifyAdapterEnv(nil, root, paths)
}

// verifyAdapter is verifyAdapter with git run on this server's caller
// environment.
func (s *Server) verifyAdapter(root string, paths []string) ([]string, error) {
	return verifyAdapterEnv(s.environ(), root, paths)
}

func verifyAdapterEnv(env []string, root string, paths []string) ([]string, error) {
	result, err := wsdoc.TicketVerifyEnv(root, paths, env)
	if err != nil {
		return nil, err
	}
	if !result.OK {
		var b strings.Builder
		b.WriteString("ticket verify failed:\n")
		for _, finding := range result.Findings {
			fmt.Fprintf(&b, "- [%s] %s: %s\n", finding.Guardrail, finding.Path, finding.Message)
		}
		return nil, fmt.Errorf("%s", strings.TrimRight(b.String(), "\n"))
	}
	if len(result.Warnings) == 0 && len(result.Advisories) == 0 {
		return nil, nil
	}
	advisories := make([]string, 0, len(result.Warnings)+len(result.Advisories))
	for _, warning := range result.Warnings {
		advisories = append(advisories, fmt.Sprintf("WARN [%s] %s: %s", warning.Guardrail, warning.Path, warning.Message))
	}
	for _, advisory := range result.Advisories {
		text := advisory.Text
		if advisory.Kind == wsdoc.AdvisoryKindFix {
			// 7-space continuation, matching the FIX:/CHECK: hanging indent.
			text += "\n       Then git commit --amend --no-edit."
		}
		advisories = append(advisories, text)
	}
	return advisories, nil
}

// formatTicketVerify renders a wsdoc.VerifyResult for the standalone
// tickets.verify tool: an overall PASS/FAIL line followed by one bullet per
// finding (hard, hyphenated FAIL) and warning (soft, WARN) — mirroring
// formatTicketMutate/formatSageGate's plain-text style — then the ticket-graph
// advisories, which render identically here and on the commit path except that
// the commit path appends the amend recipe.
func formatTicketVerify(result wsdoc.VerifyResult) string {
	var b strings.Builder
	if result.OK {
		b.WriteString("verify: PASS\n")
	} else {
		b.WriteString("verify: FAIL\n")
	}
	for _, finding := range result.Findings {
		fmt.Fprintf(&b, "  FAIL [%s] %s: %s\n", finding.Guardrail, finding.Path, finding.Message)
	}
	for _, warning := range result.Warnings {
		fmt.Fprintf(&b, "  WARN [%s] %s: %s\n", warning.Guardrail, warning.Path, warning.Message)
	}
	// Advisories render verbatim: the board block carries its own 4-space row
	// indent, and the commit path's formatGitCommit adds its own 2-space pass.
	// They deliberately do not feed the next_instruction switch below — an
	// ancestor note is explicitly no-action-needed, so "should be addressed or
	// explicitly accepted" would be wrong.
	for _, advisory := range result.Advisories {
		fmt.Fprintf(&b, "\n%s\n", advisory.Text)
	}
	switch {
	case !result.OK:
		b.WriteString("next_instruction: Fix every FAIL finding above; these are the same hard guardrails git.commit enforces and will block a commit.\n")
	case len(result.Warnings) > 0:
		b.WriteString("next_instruction: PASS with warnings above; they do not block a commit but should be addressed or explicitly accepted.\n")
	}
	return b.String()
}

func formatSageGate(result wsdoc.SageGateResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "action: %s\n", result.Action)
	if result.AskPrompt != "" {
		fmt.Fprintf(&b, "ask_prompt: %s\n", result.AskPrompt)
	}
	if len(result.Reviewers) > 0 {
		fmt.Fprintf(&b, "reviewers: %s\n", strings.Join(result.Reviewers, ", "))
	}
	if result.Mode != "" {
		fmt.Fprintf(&b, "mode: %s\n", result.Mode)
	}
	if result.Advisory != "" {
		fmt.Fprintf(&b, "advisory: %s\n", capitalizeFirst(result.Advisory))
	}
	if len(result.FreshnessStages) > 0 {
		fmt.Fprintf(&b, "freshness_stages: %s\n", strings.Join(result.FreshnessStages, ", "))
	}
	if result.ReviewBaseline != "" {
		fmt.Fprintf(&b, "review_baseline: %s\n", result.ReviewBaseline)
	}
	if result.ReviewInstruction != "" {
		fmt.Fprintf(&b, "review_instruction: %s\n", result.ReviewInstruction)
	}
	b.WriteString(sageGateNextInstruction(result))
	return b.String()
}

// sageGatePostureUncommittedNote is the single sentence every gate action
// carries about the posture write. Sharing one sentence is the point: the
// prior code said three different things about one condition ("skip" handed over
// a ready-to-paste commit call, "run" appended the same call, "ask" said
// nothing).
const sageGatePostureUncommittedNote = " Any sage-review posture this call wrote is left uncommitted; it rides your next ordinary commit of the ticket path, so do not commit it separately."

// sageGateNextInstruction names no commit title and hands the caller no
// ws/git.commit call. Any posture this gate wrote (a config-fallback resolve, or
// an ask-decline's "skipped") is left in the working tree for the caller's next
// ordinary commit of the ticket path, so the posture lands in one commit with
// the ticket's real edits and real ## AI Context instead of a separate
// canonically-titled commit that would swallow them (260725,
// {#260720-wsdoc-commit-boundary}).
func sageGateNextInstruction(result wsdoc.SageGateResult) string {
	switch result.Action {
	case "skip":
		return "next_instruction: Sage review gate resolved with no further review required; proceed to handoff." + sageGatePostureUncommittedNote
	case "stop_blocked":
		// The recovery sentence exists because the bare "stop" read as a dead
		// end while the ticket skill's reviewer-spawn step covers "each stage
		// the gate reports blocked whose blocker this invocation's edits
		// address" — a clause that looks unreachable if the gate is the only
		// path to a reviewer. It is not: resolveStage returns stop_blocked
		// forever, but sage_stamp is callable without the gate naming anyone,
		// which formatSageRecord's block branch already states. Saying it here
		// too keeps one condition from being described two ways.
		//
		// stop_blocked carries the note too: sageGateCombined can persist a
		// design posture (design recommended + answer "yes") before the
		// completeness stage hits its blocked branch, so this action is not
		// guaranteed to be write-free. The note's hedged "Any sage-review
		// posture this call wrote" stays true on the branches that wrote
		// nothing, which is why it can be attached unconditionally.
		return "next_instruction: A blocked sage review must be addressed before promotion; stop and report the blocker. This gate returns stop_blocked while the posture is blocked and never names a reviewer again, so a later invocation whose edits address the blocker spawns that stage's reviewer via On: Reviewer Spawn and clears the posture with ws/tickets.sage_stamp carrying fresh verdicts; read which stage is blocked from the ticket's sage-review-* frontmatter, which this result does not carry." + sageGatePostureUncommittedNote
	case "stop_missing_route_facts":
		return "next_instruction: This ticket has no ## Route Facts section, and the implementation route reads its facts from there, so it cannot be routed as promoted. Render the `ticket-fact-populator` playbook on this ticket, apply what it returns, then call tickets.sage_gate again with the same stem/landing; the section's values are the completeness reviewer's subject, so no review runs until it exists." + sageGatePostureUncommittedNote
	case "stop_open_decision_queue":
		// No posture note: this stop precedes every posture write on both
		// landings, so the call wrote nothing.
		return "next_instruction: This ticket still has a ## Open Decision Queue section, whose items are unsettled working state rather than decisions, so it cannot be reviewed or promoted yet. Settle the queue with the user (announced defaults and technical decisions acknowledged, every policy question settled), move each settled item into its home section, delete the section, then call tickets.sage_gate again with the same stem/landing."
	case "ask":
		return "next_instruction: Relay ask_prompt to the user, then call tickets.sage_gate again with the same stem/landing plus answer=yes|no." + sageGatePostureUncommittedNote
	case "run":
		return "next_instruction: Spawn the listed reviewer(s) via On: Reviewer Spawn, then call tickets.sage_stamp(stem, stage, verdicts) with stage=" + sageStageForReviewers(result) + "." + sageGatePostureUncommittedNote
	case "check_review_required":
		return "next_instruction: Inspect the ticket diff against review_baseline and decide whether to rerun the listed sage review stage(s), then call tickets.sage_gate again with the same stem/landing plus answer=yes|no; do not treat the prior completed review as current until that decision is made." + sageGatePostureUncommittedNote
	default:
		return "next_instruction: Unrecognized action; stop and report."
	}
}

func sageStageForReviewers(result wsdoc.SageGateResult) string {
	if result.Mode == "combined" {
		return "combined"
	}
	if len(result.Reviewers) == 1 {
		return result.Reviewers[0]
	}
	return result.Mode
}

func formatSageRecord(result wsdoc.SageRecordResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "verdict: %s\n", result.Verdict)
	for _, field := range []string{"sage-review-design", "sage-review-completeness"} {
		if value, ok := result.Posture[field]; ok {
			fmt.Fprintf(&b, "%s: %s\n", field, value)
		}
	}
	if result.BlockedSection != "" {
		b.WriteString("blocked_section: appended\n")
	}
	if result.Autonomous > 0 || result.Missing > 0 {
		fmt.Fprintf(&b, "autonomous_issues: %d\nmissing_issues: %d\n", result.Autonomous, result.Missing)
	}
	// Issue routing is emitted before the verdict's own direction so the caller
	// reads "fix these, then commit" in that order; appending it after "commit,
	// then proceed to handoff" put the fixes behind the commit.
	b.WriteString("next_instruction:")
	b.WriteString(sageRecordIssueRouting(result))
	switch result.Verdict {
	case "block":
		// A blocked posture previously returned the same "commit, then proceed to
		// handoff" text as a pass, so a block at a todo/ landing was recorded and
		// then dropped: the caller's only block branch covers the ready/ landing.
		// This says stop, matching sageGateNextInstruction's stop_blocked branch
		// and the caller's ready/ branch. resolveStage returns stop_blocked for a
		// blocked posture forever, so sage_gate never names a reviewer again: the
		// one recovery is the caller rerunning the blocked stage's reviewer itself
		// (a delta review) and stamping those fresh verdicts. A same-verdict
		// re-stamp is not a recovery: the block comes from the reviewer verdict
		// field, so it records the same block and repeats this text. The text is
		// silent on committing versus reverting, which is landing-dependent and
		// the caller's; it only forbids committing as if the block were cleared.
		fmt.Fprintf(&b, " Blocked posture recorded but not committed. A blocked sage review must be addressed before promotion: do not commit or hand off as if it were cleared. ws/tickets.sage_gate returns stop_blocked while the posture is blocked and will not name a reviewer again, so only a later ws/tickets.sage_stamp carrying fresh verdicts clears it: once the ticket is revised, rerun %s on it yourself (a delta review) and stamp that fresh verdict set. If you cannot address the block now, stop and report the blocker.", sageBlockedReviewers(result))
	case "concern":
		// The missing-decision sentence is gated on an actual missing issue. A
		// standalone stage records `concern` straight from the reviewer verdict
		// with no missing issue required, so the ungated text told the caller to
		// weigh the criticality of a decision the same message reported as absent.
		if result.Missing > 0 {
			b.WriteString(" Recorded but not committed; commit this posture change via ws/git.commit before proceeding. The concern with a missing decision is surfaced — escalate to block manually only if the missing decision is judged critical.")
		} else {
			b.WriteString(" Recorded but not committed; commit this posture change via ws/git.commit before proceeding.")
		}
	default:
		b.WriteString(" Sage review posture recorded but not committed; commit this change via ws/git.commit, then proceed to handoff.")
	}
	return b.String()
}

// sageRestampAfterFixes closes every non-block autonomous-issue routing clause: apply the
// fixes, re-stamp with the same verdicts so the recorded digest covers the fixed
// body, and only then commit.
const sageRestampAfterFixes = " Once those edits are in the ticket, call ws/tickets.sage_stamp again with the same verdicts: the digest recorded now covers the body as it stands at this stamp, and only the re-stamp makes it cover the fixed body. Commit after that second stamp, not before it."

// sageRecordIssueRouting renders the per-resolution routing clause that leads
// every next_instruction carrying issues. Both reviewer playbooks classify each
// issue as `autonomous` (planning or implementation can settle it) or `missing`
// (a policy choice those stages cannot make); this is the consumer of that
// split. Empty when the stage recorded no issues, so a clean pass stays terse.
//
// An autonomous fix edits the ticket body after this stamp took its digest, so
// every non-block autonomous branch also orders a re-stamp: without it the digest this
// call recorded is stale the moment the fixes land, tickets.verify warns, and
// the next sage_gate asks to rerun the reviewers whose findings were applied.
//
// A block is the exception: its recorded verdict comes from the reviewer
// verdict field, not from the issue counts, so a re-stamp with the same
// verdicts records the same block whatever mix of issues it carries. A block
// therefore closes every branch with sageNoSameVerdictRestamp instead, and the
// block direction in formatSageRecord names the delta review that clears it.
// The branch keys on the recorded verdict, not on Missing > 0: an
// autonomous-only block loops the same way, and a mixed concern or pass is not
// blocked and still needs the re-stamp to refresh its digest.
func sageRecordIssueRouting(result wsdoc.SageRecordResult) string {
	closing := sageRestampAfterFixes
	missingClosing := ""
	if result.Verdict == "block" {
		closing = sageNoSameVerdictRestamp
		missingClosing = sageNoSameVerdictRestamp
	}
	switch {
	case result.Autonomous > 0 && result.Missing > 0:
		return fmt.Sprintf(" Route the recorded issues first: fix the %d autonomous issue(s) in the ticket yourself, and take the %d missing issue(s) through the Open Decision Queue, each classed by the same rules as any other item.%s", result.Autonomous, result.Missing, closing)
	case result.Autonomous > 0:
		return fmt.Sprintf(" Fix the %d autonomous issue(s) in the ticket yourself first; none of them need a user decision.%s", result.Autonomous, closing)
	case result.Missing > 0:
		return fmt.Sprintf(" Take the %d missing issue(s) through the Open Decision Queue first, each classed by the same rules as any other item.%s", result.Missing, missingClosing)
	}
	return ""
}

// sageNoSameVerdictRestamp closes a block's routing clause in place of
// sageRestampAfterFixes.
const sageNoSameVerdictRestamp = " Do not call ws/tickets.sage_stamp again with these same verdicts after those revisions: it records the same block."

// sageBlockedReviewers names the reviewer(s) whose fresh verdicts clear this
// block, read off the posture fields the stamp set to blocked: one field for a
// standalone stage, both after a combined block.
func sageBlockedReviewers(result wsdoc.SageRecordResult) string {
	design := result.Posture["sage-review-design"] == "blocked"
	completeness := result.Posture["sage-review-completeness"] == "blocked"
	switch {
	case design && completeness:
		return "both the design and the completeness reviewers"
	case design:
		return "the design reviewer"
	case completeness:
		return "the completeness reviewer"
	}
	return "the blocked stage's reviewer"
}

func parseSageVerdicts(raw any) ([]wsdoc.SageVerdict, error) {
	if raw == nil {
		return nil, fmt.Errorf("verdicts is required")
	}
	list, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("verdicts must be an array")
	}
	verdicts := make([]wsdoc.SageVerdict, 0, len(list))
	for _, item := range list {
		m, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("each verdict must be an object")
		}
		v := wsdoc.SageVerdict{}
		v.Reviewer, _ = m["reviewer"].(string)
		v.Verdict, _ = m["verdict"].(string)
		if issuesRaw, ok := m["issues"].([]any); ok {
			for _, ir := range issuesRaw {
				im, ok := ir.(map[string]any)
				if !ok {
					return nil, fmt.Errorf("each issue must be an object")
				}
				issue := wsdoc.SageIssue{}
				issue.Title, _ = im["title"].(string)
				issue.Severity, _ = im["severity"].(string)
				issue.Resolution, _ = im["resolution"].(string)
				v.Issues = append(v.Issues, issue)
			}
		}
		verdicts = append(verdicts, v)
	}
	return verdicts, nil
}

// ticketScopeAnnotation is the aggregate hidden-count line the discovery
// surfaces carry when this worktree's sparse-checkout hides part of the board.
// It is text-mode only, following CommitResult.Advisories' `json:"-"`
// precedent ({#260626-git-commit-todo-reinjection}): a non-blocking annotation
// never changes a tool's JSON contract, and every response this server produces
// is a single content block. It runs a path-only index enumeration and never
// routes through loadTicketGraph, so a discovery call still pays no
// .done/.dropped body reads. Any error degrades to no annotation.
func ticketScopeAnnotation(root string, statuses []string) string {
	if len(statuses) == 0 {
		// The caller's filters selected no status at all, so the listing is
		// empty for a reason that has nothing to do with the scope. Blaming the
		// scope here would invert the annotation's purpose, which is exactly to
		// keep "filtered" and "hidden" distinguishable.
		return ""
	}
	info, err := wsdoc.TicketScope(root, statuses)
	if err != nil || !info.Active || info.Hidden == 0 {
		return ""
	}
	return fmt.Sprintf(
		"scope: %d ticket(s) hidden by this worktree's sparse-checkout scope (core.sparseCheckout); "+
			"they remain in the index and resolvable by stem.\n", info.Hidden)
}

// effectiveTicketStatuses decodes the tool arguments and defers to
// wsdoc.EffectiveTicketStatuses so the hidden count covers exactly the statuses
// the accompanying listing covered — including its archive gating, which drops
// an explicitly requested .done/.dropped unless the matching include flag is
// set. Without that half, tickets.query(statuses: ["done"]) with include_done
// unset would render an empty listing and then blame the scope for it. The rule
// itself lives in wsdoc rather than being mirrored here: mcp already imports
// wsdoc freely, so there is no import-boundary reason to keep a second copy that
// can drift.
func effectiveTicketStatuses(args map[string]any) []string {
	return wsdoc.EffectiveTicketStatuses(
		stringList(args["statuses"]),
		boolArgument(args["include_done"]),
		boolArgument(args["include_dropped"]),
	)
}

func formatTickets(tickets []wsdoc.TicketInfo) string {
	var b strings.Builder
	for _, ticket := range tickets {
		fmt.Fprintf(&b, "[%s] %s", ticket.Status, ticket.Stem)
		if ticket.Title != "" {
			fmt.Fprintf(&b, " - %s", ticket.Title)
		}
		if ticket.Path != "" {
			fmt.Fprintf(&b, " (%s)", ticket.Path)
		}
		flags := []string{}
		if ticket.ResultPresent {
			flags = append(flags, "result")
		}
		if ticket.MentionsTicketStem {
			flags = append(flags, "mentions_ticket_stem")
		}
		if ticket.Hidden {
			flags = append(flags, "hidden")
		}
		if ticket.Parent != "" {
			flags = append(flags, "parent="+ticket.Parent)
		}
		if len(flags) > 0 {
			fmt.Fprintf(&b, " [%s]", strings.Join(flags, " "))
		}
		b.WriteString("\n")
		if ticket.DispatchBlocked != nil {
			fmt.Fprintf(&b, "  dispatch_blocked: %s - %s\n", ticket.DispatchBlocked.BlockingStem, ticket.DispatchBlocked.Reason)
		}
		// Ownership line: emitted only when the feature is on (AssigneeGate set)
		// and the ticket actually names an assignee. A mismatch carries the fixed
		// warning token; a self-assignment prints the ownership plainly; an
		// assign-any ticket prints nothing so the common case stays quiet.
		if g := ticket.AssigneeGate; g != nil && !g.AssignAny && len(ticket.Assignee) > 0 {
			line := "  assignee: " + strings.Join(ticket.Assignee, ", ")
			if g.Warning != "" {
				line += "   # " + g.Warning
			}
			b.WriteString(line + "\n")
		}
		// Advisory body marker, kept on its own line and prefix so it is never
		// confused with the typed dispatch_blocked gate above. Emitted on every
		// projection because the queue selector reads it from the discovery
		// inventory; the heading is verbatim, its currency is the caller's to judge.
		for _, heading := range ticket.BlockedHeadings {
			fmt.Fprintf(&b, "  blocked_marker: %s\n", heading)
		}
		writeIndentedLines(&b, "  unresolved: ", ticket.UnresolvedPhases)
		writeIndentedLines(&b, "  snippet: ", ticket.MatchingSnippets)
	}
	return b.String()
}

func ticketDiscoveryPagination(arguments map[string]any) (int, int, error) {
	offset, err := exactIntegerArgument(arguments, "offset", 0)
	if err != nil {
		return 0, 0, err
	}
	if offset < 0 {
		return 0, 0, fmt.Errorf("offset must be at least 0")
	}
	limit, err := exactIntegerArgument(arguments, "limit", 50)
	if err != nil {
		return 0, 0, err
	}
	if limit < 1 || limit > 200 {
		return 0, 0, fmt.Errorf("limit must be between 1 and 200")
	}
	return offset, limit, nil
}

func hasTicketOnlyArgument(arguments map[string]any) bool {
	_, ok := arguments["mentions_ticket_stem"]
	return ok
}
