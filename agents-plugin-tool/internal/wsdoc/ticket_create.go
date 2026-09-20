package wsdoc

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// stemDatePrefixRe matches a leading YYMMDD- date prefix on a caller-supplied
// stem — the shape "stem" takes everywhere else in the workflow (ticket
// filenames, git log --grep, and every ticket reference), but not the
// dateless-semantic shape TicketCreate contractually expects. No ticket
// category is numeric (feat/bug/refactor/chore/research/epic/...), so a
// leading 6-digit prefix never misclassifies a valid semantic stem.
var stemDatePrefixRe = regexp.MustCompile(`^(\d{6})-(.+)$`)

// splitDatePrefix reports whether stem carries a leading YYMMDD- prefix and,
// if so, returns the embedded date and the remaining dateless stem.
func splitDatePrefix(stem string) (embedded, rest string, ok bool) {
	m := stemDatePrefixRe.FindStringSubmatch(stem)
	if m == nil {
		return "", "", false
	}
	return m[1], m[2], true
}

type TicketCreateOptions struct {
	Stem         string // semantic stem (no date prefix)
	InitialState string // "idea" | "todo" | "ready"
	SageReview   string // sage_review config value ("" | "off" | "auto" | "ask")
	Today        string // YYMMDD; if empty, use time.Now().Format("060102")
	// Assignee, when non-empty, stamps an `assignee:` YAML sequence into the
	// created stub's frontmatter — the explicit ownership act at creation. The
	// MCP layer resolves it from set_assignee (and only when the
	// ticket-assignee-aware flag is on); an empty slice leaves the ticket
	// unassigned (assign-any). Emails are emitted verbatim (case preserved);
	// blank entries are skipped.
	Assignee []string
}

type TicketCreateResult struct {
	Path string
	Tip  string
}

func TicketCreate(root string, opts TicketCreateOptions) (TicketCreateResult, error) {
	stem := strings.TrimSpace(opts.Stem)
	if stem == "" {
		return TicketCreateResult{}, fmt.Errorf("stem must not be empty")
	}

	today := strings.TrimSpace(opts.Today)
	if today == "" {
		today = time.Now().Format("060102")
	}

	// stem is contractually dateless; TicketCreate prepends today's date
	// below. A caller passing the dated form "stem" takes everywhere else
	// (filenames, git log --grep, ticket references) would otherwise double
	// the prefix silently. A prefix equal to today is a harmless duplicate —
	// strip and proceed; a prefix that differs from today is ambiguous
	// between an intentional backdate and a mistake, so it is rejected
	// rather than silently honored or silently re-dated.
	if embedded, rest, ok := splitDatePrefix(stem); ok {
		if embedded != today {
			return TicketCreateResult{}, fmt.Errorf(
				"stem carries date prefix %q but today is %s; pass a dateless semantic stem (the date is prepended automatically)",
				embedded+"-", today)
		}
		stem = rest
	}

	category, _, _ := strings.Cut(stem, "-")
	state := strings.TrimSpace(opts.InitialState)
	if state == "ready" && nonImplementationCategories[category] {
		return TicketCreateResult{}, fmt.Errorf("%s tickets never enter ready/: they are board artifacts rather than execution targets", category)
	}

	// Retired authoring must not disable historical stem recognition.
	if category == "workset" {
		_, err := TicketTemplate(category)
		return TicketCreateResult{}, err
	}

	switch state {
	case "idea", "todo", "ready":
	default:
		return TicketCreateResult{}, fmt.Errorf("initial_state must be idea, todo, or ready")
	}

	fullStem := today + "-" + stem
	relPath := ticketRelPath(statusDirs[state], fullStem)
	destAbs := filepath.Join(root, filepath.FromSlash(relPath))

	if _, err := os.Stat(destAbs); err == nil {
		return TicketCreateResult{}, fmt.Errorf("ticket already exists: %s", relPath)
	} else if !os.IsNotExist(err) {
		return TicketCreateResult{}, err
	}
	// The stat says nothing about a destination this worktree's sparse-checkout
	// scope hides: the index entry would still be there, and writing over it
	// would silently clobber a ticket the caller cannot see. The check stays
	// destination-path-only — a same-stem ticket in a *different* status
	// directory does not block creation today either.
	if scope := newTicketScope(root); scope != nil {
		exists, err := scope.hasIndexPath(relPath)
		if err != nil {
			return TicketCreateResult{}, err
		}
		if exists {
			return TicketCreateResult{}, fmt.Errorf(
				"ticket already exists outside this worktree's sparse-checkout scope (core.sparseCheckout): %s "+
					"is in the index but not checked out here; widen the scope (`git sparse-checkout add %s`) to inspect it",
				relPath, relPath)
		}
	}

	designRequired, completenessRequired := sageReviewStageRequirement(fullStem)
	resolved := ResolvedSageReviewPosture(opts.SageReview)

	// Never-skippable design-review invariant: a ticket created directly at
	// ready has no "from" state that could have already run a design-review
	// gate against it. ws/git.commit's ready-sage-posture guardrail is the
	// sole HARD enforcement point (single chokepoint); tickets.create_empty
	// no longer blocks on a non-terminal resolved posture, it warns instead
	// (readyWarning below, carried on TicketCreateResult.Tip). TicketCreate
	// never has a blocked case here: resolved only ever comes from
	// ResolvedSageReviewPosture, whose outputs are recommended/required/
	// skipped, never blocked — a brand-new ticket has no prior posture to be
	// blocked from. Built from readyPostureProblems over *both* required
	// stages (mirroring prepareSageReviewForUpwardMove/TicketsMove) so the
	// warning names exactly what ws/tickets.verify will fail on — a
	// design-only warning left create_empty(ready) and move(to: "ready")
	// disagreeing about a category that also requires completeness.
	var readyWarning string
	if state == "ready" {
		readyWarning = readySagePostureWarning(readyPostureProblems(designRequired, resolved, completenessRequired, resolved))
	}

	if err := os.MkdirAll(filepath.Dir(destAbs), 0o755); err != nil {
		return TicketCreateResult{}, err
	}

	stub := "---\ntitle: \"\"\n"
	if (state == "ready" || (state == "todo" && !completenessRequired)) && designRequired {
		stub += "sage-review-design: " + resolved + "\n"
	}
	if state == "ready" && completenessRequired {
		stub += "sage-review-completeness: " + resolved + "\n"
	}
	if assigneeBlock := assigneeFrontmatter(opts.Assignee); assigneeBlock != "" {
		stub += assigneeBlock
	}
	stub += "---\n"

	if err := os.WriteFile(destAbs, []byte(stub), 0o644); err != nil {
		return TicketCreateResult{}, err
	}

	var tip string
	switch {
	case !designRequired:
		tip = "sage review is exempt for this ticket category."
	case completenessRequired && state != "ready":
		tip = "Populate facts and run design and completeness review at ready promotion; todo authoring is ungated."
	case state == "idea":
		tip = "Explicit epic settlement at todo promotion populates facts and runs design review."
	case readyWarning != "":
		tip = readyWarning
	case state == "ready" && completenessRequired:
		tip = sageReviewPostureTip(sageReviewPostures{Design: resolved, Completeness: resolved})
	default:
		tip = "sage review posture: design " + resolved + "."
	}

	return TicketCreateResult{Path: relPath, Tip: tip}, nil
}

// assigneeFrontmatter renders an `assignee:` YAML sequence for the create stub,
// one email per `- item` line, matching the list shape the frontmatter parser
// reads back into []string. Returns "" when no non-blank email is given, so an
// unassigned ticket carries no assignee key at all (assign-any).
func assigneeFrontmatter(assignee []string) string {
	var b strings.Builder
	for _, email := range assignee {
		email = strings.TrimSpace(email)
		if email == "" {
			continue
		}
		if b.Len() == 0 {
			b.WriteString("assignee:\n")
		}
		b.WriteString("  - " + email + "\n")
	}
	return b.String()
}
