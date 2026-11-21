// Git commit options, guarded commit execution, and message formatting.
package wsgit

import (
	"context"
	"fmt"
	"strings"
)

type CommitOptions struct {
	Paths          []string `json:"paths"`
	Title          string   `json:"title"`
	Description    string   `json:"description,omitempty"`
	AIContext      []string `json:"ai_context"`
	UpdatedTickets []string `json:"updated_tickets,omitempty"`
	// SuppressInferredTicketUpdates records an explicit empty MCP list without
	// changing direct Go/CLI callers' zero-value inference. It affects only
	// inferred message summaries, never ticket detection or verification.
	SuppressInferredTicketUpdates bool `json:"-"`
	// Followups are written under "## Follow-ups", one minted id each. The
	// root-lead authorization gate is the caller's (internal/mcp owns sessions).
	Followups []Followup `json:"followups,omitempty"`
	// Resolves are follow-up ids written under "## Resolves".
	Resolves []string `json:"resolves,omitempty"`
	// ExpectedBranch is the branch the caller believes it is checked out on.
	// Commit resolves the actual current branch and refuses (no mutation) when
	// they disagree or when HEAD is detached — the guard against a parallel
	// session switching the shared worktree out from under the caller. It is
	// required: an empty value is rejected by normalizeCommitOptions so a caller
	// that does not know its branch cannot form the call at all. The value must
	// originate from the caller's remembered context, never a fresh HEAD read
	// (which would always match and check nothing).
	ExpectedBranch string `json:"expected_branch"`
	// SparseScopeActive is a caller-computed signal, never a client-supplied
	// field (json:"-"): true only when the caller has already determined a
	// sparse-checkout scope is active for root (see wsdoc.SparseCheckoutActive).
	// It makes stagingCommandsForCommit's `add` staging use `git add --sparse`
	// so an off-topic capture (e.g. a new idea/ ticket) can be staged and then
	// self-hide, instead of failing with "did not match any files". The zero
	// value (false) reproduces today's staging shape byte-for-byte, so every
	// existing caller and test is unaffected.
	SparseScopeActive bool `json:"-"`
}

type CommitResult struct {
	Hash          string         `json:"hash"`
	Paths         []string       `json:"paths"`
	Title         string         `json:"title"`
	TicketChanges []TicketChange `json:"ticket_changes,omitempty"`
	// Followups echoes the recorded follow-ups with their minted ids, in the
	// input order of CommitOptions.Followups.
	Followups []Followup `json:"followups,omitempty"`
	// Warnings carries non-blocking caller-attached notices (for example a
	// resolve of an unknown or already-resolved follow-up id).
	Warnings []string `json:"warnings,omitempty"`
	// Advisories carries non-blocking verifier text lines. Text-mode only;
	// see {#260626-git-commit-todo-reinjection} precedent — never serialized
	// to JSON.
	Advisories []string `json:"-"`
}

func (c Client) Commit(ctx context.Context, root string, opts CommitOptions) (CommitResult, error) {
	opts, err := normalizeCommitOptions(opts)
	if err != nil {
		return CommitResult{}, err
	}
	for _, path := range opts.Paths {
		if err := validateCommitPath(root, path); err != nil {
			return CommitResult{}, err
		}
	}
	runner := c.runner()
	// Branch guard first, before any staging mutation: resolve the actual
	// current branch and refuse if it disagrees with the caller's believed
	// branch (or if HEAD is detached). This is the whole point of the required
	// expected_branch field — a commit that lands on the wrong branch because a
	// parallel session switched the shared worktree is silent and hard to undo,
	// so it must be caught before the commit, not after.
	actualBranch := currentBranch(ctx, runner, root)
	if actualBranch == "" {
		return CommitResult{}, fmt.Errorf("git.commit refused: HEAD is detached, so there is no current branch to confirm against believed branch %q; a commit under a detached HEAD in a shared worktree is exactly the state this guard exists to stop — verify whether you should be committing here", opts.ExpectedBranch)
	}
	if actualBranch != opts.ExpectedBranch {
		refusal := fmt.Sprintf("git.commit refused: believed branch %q does not match the actual checkout %q (a parallel session may have switched this worktree); verify whether you should be committing here before retrying (believed: %s / actual: %s)", opts.ExpectedBranch, actualBranch, opts.ExpectedBranch, actualBranch)
		// When the actual checkout is a worker's impl/ branch, name the sanctioned
		// escape hatch so the reader does not reflexively `git switch` this shared
		// worktree onto their own branch.
		if strings.HasPrefix(actualBranch, "impl/") {
			base := "<the branch this worker was spawned from>"
			if parent := implBranchParent(actualBranch); parent != "" {
				base = fmt.Sprintf("%q", parent)
			}
			refusal += fmt.Sprintf("; the actual checkout is a worker's impl/ branch — housekeeping meanwhile goes through worktree.acquire(base: %s, profile: \"housekeeping\") and its returned key, not this root", base)
		}
		return CommitResult{}, fmt.Errorf("%s", refusal)
	}
	preStatusOut, err := runner.RunGit(ctx, root, StatusArgs()...)
	if err != nil {
		return CommitResult{}, err
	}
	preStatus := ParseStatus(preStatusOut)
	opts.Paths = expandCommitPathsForTicketMoves(preStatus, opts.Paths)
	for _, args := range stagingCommandsForCommit(opts.Paths, preStatus, opts.SparseScopeActive) {
		if _, err := runner.RunGit(ctx, root, args...); err != nil {
			return CommitResult{}, err
		}
	}
	statusOut, err := runner.RunGit(ctx, root, StatusArgs()...)
	if err != nil {
		return CommitResult{}, err
	}
	status := ParseStatus(statusOut)
	var mergePins *commitMergePins
	if statusErr := validateCommitStatus(status, opts.Paths); statusErr != nil {
		if !strings.HasPrefix(statusErr.Error(), "refusing to commit unrelated staged path") {
			return CommitResult{}, statusErr
		}
		admitted, pins, err := admitMergeStaging(ctx, runner, root, opts.Paths)
		if err != nil {
			return CommitResult{}, fmt.Errorf("%w; automatic merge admission unavailable: %v; include the intended staged paths in the existing paths grant explicitly", statusErr, err)
		}
		opts.Paths = append(opts.Paths, admitted...)
		mergePins = pins
		if err := validateCommitStatus(status, opts.Paths); err != nil {
			return CommitResult{}, err
		}
	}
	// Verify runs after staging (so a blocked commit leaves the invalid state
	// staged for the caller to fix and re-verify, not reverted) and before the
	// commit message/`git commit` itself, matching validateCommitStatus's
	// gate position. The verifier only ever sees the delete-side-filtered
	// paths (see filterIndexDeleteSidePaths): a staged ticket rename or
	// deletion leaves its old path unreadable, and wsdoc.TicketVerify's
	// file-exists guardrail would otherwise reject a legitimate status
	// transition. validateCommitStatus above still sees the full expanded
	// opts.Paths, since narrowing what it sees would reopen the
	// unrelated-path guard for the delete-side path itself. See
	// {#260720-wsdoc-commit-boundary} for why this stays a wsgit-side filter
	// rather than a wsdoc.TicketVerify change.
	var advisories []string
	if verifyPaths := filterIndexDeleteSidePaths(status, opts.Paths); len(verifyPaths) > 0 {
		verifyAdvisories, err := c.verifier()(root, verifyPaths)
		if err != nil {
			// A veto (non-nil error) stops Commit here and returns the zero
			// CommitResult, so verifyAdvisories is intentionally discarded:
			// a vetoed commit has nothing to advise on.
			return CommitResult{}, err
		}
		advisories = verifyAdvisories
	}
	ticketChanges := detectTicketChanges(ctx, runner, root)
	if len(opts.UpdatedTickets) == 0 && !opts.SuppressInferredTicketUpdates {
		opts.UpdatedTickets = ticketChangeSummaries(ticketChanges)
	}
	if err := mintFollowupIDs(opts.Followups, c.MintID); err != nil {
		return CommitResult{}, err
	}
	message := CommitMessage(opts)
	if mergePins != nil {
		// Verification and message generation can run caller code. Recheck the
		// pinned heads and the whole staged index (including new outside-grant
		// additions), not just the names that passed the equality proof.
		if err := mergePins.recheck(ctx, runner, root, opts.ExpectedBranch); err != nil {
			return CommitResult{}, err
		}
	}
	// Match Git's non-editing whitespace cleanup while preserving Markdown
	// headings even when the caller configured commit.cleanup=strip.
	if _, err := runner.RunGit(ctx, root, "commit", "-m", message, "--cleanup=whitespace"); err != nil {
		return CommitResult{}, err
	}
	hashOut, err := runner.RunGit(ctx, root, "rev-parse", "HEAD")
	if err != nil {
		return CommitResult{}, err
	}
	return CommitResult{Hash: strings.TrimSpace(string(hashOut)), Paths: opts.Paths, Title: opts.Title, TicketChanges: ticketChanges, Followups: opts.Followups, Advisories: advisories}, nil
}

// currentBranch resolves the short name of the branch HEAD points at, mirroring
// git.merge's resolution (`git symbolic-ref --quiet --short HEAD`). --quiet makes
// a detached HEAD exit non-zero with empty output rather than an error message,
// so any resolution failure is reported as an empty name: the caller then
// refuses (fail-closed) instead of committing unguarded.
func currentBranch(ctx context.Context, runner Runner, root string) string {
	out, err := runner.RunGit(ctx, root, "symbolic-ref", "--quiet", "--short", "HEAD")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// implBranchParent returns the parent (merge root) segment of an
// "impl/<parent>/<stem>" branch, or "" for a rootless "impl/<stem>" or a
// non-impl branch. It is the wsgit-local twin of internal/mcp's
// parseImplBranchRoot: wsgit must not import internal/mcp (the dependency runs
// mcp -> wsgit), so the tiny last-slash split is duplicated rather than shared.
func implBranchParent(branch string) string {
	const prefix = "impl/"
	if !strings.HasPrefix(branch, prefix) {
		return ""
	}
	rest := strings.TrimPrefix(branch, prefix)
	idx := strings.LastIndex(rest, "/")
	if idx <= 0 {
		return ""
	}
	return rest[:idx]
}

func normalizeCommitOptions(opts CommitOptions) (CommitOptions, error) {
	rawAIContext := opts.AIContext
	opts.Title = strings.TrimSpace(opts.Title)
	opts.Description = strings.TrimSpace(opts.Description)
	opts.AIContext = trimStrings(opts.AIContext)
	opts.UpdatedTickets = trimStrings(opts.UpdatedTickets)
	opts.ExpectedBranch = strings.TrimSpace(opts.ExpectedBranch)
	followups, err := normalizeFollowups(opts.Followups)
	if err != nil {
		return CommitOptions{}, err
	}
	opts.Followups = followups
	resolves, err := normalizeResolves(opts.Resolves)
	if err != nil {
		return CommitOptions{}, err
	}
	opts.Resolves = resolves
	if opts.Title == "" {
		return CommitOptions{}, fmt.Errorf("title is required")
	}
	if strings.ContainsAny(opts.Title, "\r\n") {
		return CommitOptions{}, fmt.Errorf("title must be a single line")
	}
	if len(opts.AIContext) == 0 {
		switch {
		case rawAIContext == nil:
			return CommitOptions{}, fmt.Errorf("ai_context is required: no ai_context field was received")
		case len(rawAIContext) == 0:
			return CommitOptions{}, fmt.Errorf("ai_context requires at least one entry: received an empty array")
		default:
			plural := ""
			if len(rawAIContext) != 1 {
				plural = "ies"
			} else {
				plural = "y"
			}
			return CommitOptions{}, fmt.Errorf("ai_context requires at least one non-blank entry: received %d entr%s, all blank", len(rawAIContext), plural)
		}
	}
	if opts.ExpectedBranch == "" {
		return CommitOptions{}, fmt.Errorf("expected_branch is required: assert the branch you believe you are on so the commit is refused if the worktree was switched out from under you; do not read HEAD just to fill this — if you do not know it, verify whether you should be committing here at all")
	}
	paths := trimStrings(opts.Paths)
	if len(paths) == 0 {
		return CommitOptions{}, fmt.Errorf("paths requires at least one path")
	}
	opts.Paths = paths
	return opts, nil
}

// sparseActive threads the caller-computed SparseScopeActive signal into the
// `add` command only. The `rm --cached` branch never gets `--sparse`:
// deletedPathsUnderCommitRoot reads only the pre-staging `git status`, which
// never reports a tracked, skip-worktree, on-disk-absent, unmodified path at
// all (skip-worktree suppresses the worktree-vs-index comparison entirely),
// so that branch can never be asked to stage a merely-hidden path as deleted
// — `--sparse` would be scope creep there, not a guardrail.
func CommitMessage(opts CommitOptions) string {
	var b strings.Builder
	b.WriteString(opts.Title)
	if opts.Description != "" {
		b.WriteString("\n\n")
		b.WriteString(opts.Description)
	}
	b.WriteString("\n\n## AI Context\n")
	for _, item := range opts.AIContext {
		fmt.Fprintf(&b, "- %s\n", item)
	}
	writeCommitSection(&b, "## Ticket Updates", opts.UpdatedTickets)
	followupLines := make([]string, 0, len(opts.Followups))
	for _, f := range opts.Followups {
		followupLines = append(followupLines, FollowupLine(f))
	}
	writeCommitSection(&b, FollowupsHeading, followupLines)
	writeCommitSection(&b, ResolvesHeading, opts.Resolves)
	return strings.TrimRight(b.String(), "\n")
}

func writeCommitSection(b *strings.Builder, heading string, values []string) {
	if len(values) == 0 {
		return
	}
	b.WriteString("\n\n")
	writeCommitHeading(b, heading, values)
}

func writeCommitHeading(b *strings.Builder, heading string, values []string) {
	b.WriteString(heading)
	b.WriteByte('\n')
	for _, value := range values {
		fmt.Fprintf(b, "- %s\n", value)
	}
}

func trimStrings(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}
