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

	designRequired, _ := sageReviewStageRequirement(fullStem)
	resolved := ResolvedSageReviewPosture(opts.SageReview)

	// Never-skippable design-review invariant: a ticket created directly at
	// ready has no "from" state that could have already run a design-review
	// gate against it, so a fresh, non-terminal resolved posture must block
	// creation here the same way prepareSageReviewForUpwardMove blocks a
	// tickets.move promotion — otherwise tickets.create(status: "ready")
	// would be a silent bypass of the invariant.
	if state == "ready" && designRequired && resolved != "completed" && resolved != "skipped" {
		return TicketCreateResult{}, sageReviewStageError("sage-review-design", resolved)
	}

	if err := os.MkdirAll(filepath.Dir(destAbs), 0o755); err != nil {
		return TicketCreateResult{}, err
	}

	stub := "---\ntitle: \"\"\n"
	if (state == "todo" || state == "ready") && designRequired {
		stub += "sage-review-design: " + resolved + "\n"
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
	default:
		tip = "sage review posture: design " + resolved + "."
	}

	return TicketCreateResult{Path: relPath, Tip: tip}, nil
}
