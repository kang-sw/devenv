package wsdoc

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type TicketCreateOptions struct {
	Stem         string // semantic stem (no date prefix)
	InitialState string // "idea" | "todo" | "ready"
	SageReview   string // sage_review config value ("" | "off" | "auto" | "ask")
	Today        string // YYMMDD; if empty, use time.Now().Format("060102")
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

	state := strings.TrimSpace(opts.InitialState)
	switch state {
	case "idea", "todo", "ready":
	default:
		return TicketCreateResult{}, fmt.Errorf("initial_state must be idea, todo, or ready")
	}

	today := strings.TrimSpace(opts.Today)
	if today == "" {
		today = time.Now().Format("060102")
	}

	fullStem := today + "-" + stem
	relPath := ticketRelPath(statusDirs[state], fullStem)
	destAbs := filepath.Join(root, filepath.FromSlash(relPath))

	if _, err := os.Stat(destAbs); err == nil {
		return TicketCreateResult{}, fmt.Errorf("ticket already exists: %s", relPath)
	} else if !os.IsNotExist(err) {
		return TicketCreateResult{}, err
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
	if (state == "todo" || state == "ready") && designRequired {
		stub += "sage-review-design: " + resolved + "\n"
	}
	if state == "ready" && completenessRequired {
		stub += "sage-review-completeness: " + resolved + "\n"
	}
	stub += "---\n"

	if err := os.WriteFile(destAbs, []byte(stub), 0o644); err != nil {
		return TicketCreateResult{}, err
	}

	var tip string
	switch {
	case state == "idea":
		tip = "promoting to 'todo/' stamps the resolved sage-review-design posture."
	case !designRequired:
		tip = "sage review is exempt for this ticket category."
	case readyWarning != "":
		tip = readyWarning
	case state == "ready" && completenessRequired:
		tip = sageReviewPostureTip(sageReviewPostures{Design: resolved, Completeness: resolved})
	default:
		tip = "sage review posture: design " + resolved + "."
	}

	return TicketCreateResult{Path: relPath, Tip: tip}, nil
}
