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

	if err := os.MkdirAll(filepath.Dir(destAbs), 0o755); err != nil {
		return TicketCreateResult{}, err
	}

	stub := "---\ntitle: \"\"\n"
	if state == "todo" || state == "ready" {
		stub += "sage-review: " + ResolvedSageReviewPosture(opts.SageReview) + "\n"
	}
	stub += "---\n"

	if err := os.WriteFile(destAbs, []byte(stub), 0o644); err != nil {
		return TicketCreateResult{}, err
	}

	tip := "sage review posture: " + ResolvedSageReviewPosture(opts.SageReview) + "."
	if state == "idea" {
		tip = "promoting to 'todo/' stamps the resolved sage-review posture."
	}

	return TicketCreateResult{Path: relPath, Tip: tip}, nil
}
