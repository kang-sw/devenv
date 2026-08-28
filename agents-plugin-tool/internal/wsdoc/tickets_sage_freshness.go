package wsdoc

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type sageReviewFreshness struct {
	Stages      []string
	Baseline    string
	Instruction string
}

func sageGateFreshnessResult(root, ticketRel string, stages []string) (SageGateResult, error) {
	freshness, err := sageReviewFreshnessCheck(root, ticketRel, stages)
	if err != nil || len(freshness.Stages) == 0 {
		return SageGateResult{}, err
	}
	return SageGateResult{
		Action:            "check_review_required",
		FreshnessStages:   freshness.Stages,
		ReviewBaseline:    freshness.Baseline,
		ReviewInstruction: freshness.Instruction,
	}, nil
}

func sageReviewFreshnessCheck(root, ticketRel string, stages []string) (sageReviewFreshness, error) {
	ticketRel = filepath.ToSlash(filepath.Clean(ticketRel))
	if ticketRel == "." || ticketRel == "" || !strings.HasPrefix(ticketRel, "ai-docs/tickets/") {
		return sageReviewFreshness{}, nil
	}
	if _, err := os.Stat(filepath.Join(root, ".git")); err != nil {
		return sageReviewFreshness{}, nil
	}
	currentRaw, err := sageReviewCurrentTicketContent(root, ticketRel)
	if err != nil {
		return sageReviewFreshness{}, nil
	}
	currentClean := normalizeTicketForSageFreshness(string(currentRaw))
	var affected []string
	var baselines []string
	for _, stage := range uniqueSageStages(stages) {
		baseline, baselineRaw, ok, err := sageReviewStageBaseline(root, ticketRel, stage)
		if err != nil {
			return sageReviewFreshness{}, err
		}
		if !ok {
			continue
		}
		if normalizeTicketForSageFreshness(baselineRaw) == currentClean {
			continue
		}
		affected = append(affected, stage)
		baselines = append(baselines, shortCommit(baseline))
	}
	if len(affected) == 0 {
		return sageReviewFreshness{}, nil
	}
	baseline := strings.Join(uniqueStrings(baselines), ", ")
	return sageReviewFreshness{
		Stages:      affected,
		Baseline:    baseline,
		Instruction: fmt.Sprintf("Inspect the ticket diff against %s and decide whether to rerun sage %s review.", baseline, strings.Join(affected, " and ")),
	}, nil
}

func sageReviewStageBaseline(root, ticketRel, stage string) (commit, text string, ok bool, err error) {
	history, err := sageReviewTicketHistory(root, ticketRel)
	if err != nil {
		return "", "", false, nil
	}
	if len(history) == 0 {
		return "", "", false, nil
	}
	field := "sage-review-" + stage
	var previous string
	for i := len(history) - 1; i >= 0; i-- {
		entry := history[i]
		raw, showErr := sageGitOutput(root, "show", entry.Commit+":"+entry.Path)
		if showErr != nil {
			continue
		}
		design, completeness := effectiveSageReviewPostures(frontmatterFromText(string(raw)))
		current := design
		if field == "sage-review-completeness" {
			current = completeness
		}
		if current == "completed" && previous != "completed" {
			return entry.Commit, string(raw), true, nil
		}
		previous = current
	}
	for _, entry := range history {
		raw, showErr := sageGitOutput(root, "show", entry.Commit+":"+entry.Path)
		if showErr != nil {
			continue
		}
		design, completeness := effectiveSageReviewPostures(frontmatterFromText(string(raw)))
		if (field == "sage-review-design" && design == "completed") ||
			(field == "sage-review-completeness" && completeness == "completed") {
			return entry.Commit, string(raw), true, nil
		}
	}
	return "", "", false, nil
}

type sageReviewHistoryEntry struct {
	Commit string
	Path   string
}

func sageReviewTicketHistory(root, ticketRel string) ([]sageReviewHistoryEntry, error) {
	logOut, err := sageGitOutput(root, "log", "--follow", "--find-renames", "--format=%H", "--name-status", "--", ticketRel)
	if err != nil {
		return nil, err
	}
	var entries []sageReviewHistoryEntry
	currentPath := ticketRel
	var currentCommit string
	var commitPath string
	var olderPath string
	flush := func() {
		if currentCommit == "" {
			return
		}
		if commitPath == "" {
			commitPath = currentPath
		}
		entries = append(entries, sageReviewHistoryEntry{Commit: currentCommit, Path: commitPath})
		if olderPath != "" {
			currentPath = olderPath
		}
	}
	for _, line := range strings.Split(string(logOut), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if isFullCommitHash(line) {
			flush()
			currentCommit = line
			commitPath = ""
			olderPath = ""
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		status := fields[0]
		if (strings.HasPrefix(status, "R") || strings.HasPrefix(status, "C")) && len(fields) >= 3 {
			oldPath := filepath.ToSlash(fields[1])
			newPath := filepath.ToSlash(fields[2])
			if newPath == currentPath {
				commitPath = newPath
				olderPath = oldPath
			}
			continue
		}
		path := filepath.ToSlash(fields[len(fields)-1])
		if path == currentPath {
			commitPath = path
		}
	}
	flush()
	return entries, nil
}

func sageReviewCurrentTicketContent(root, ticketRel string) ([]byte, error) {
	if staged, err := sageGitOutput(root, "diff", "--cached", "--name-only", "--", ticketRel); err == nil && strings.TrimSpace(string(staged)) != "" {
		return sageGitOutput(root, "show", ":"+ticketRel)
	}
	if unstaged, err := sageGitOutput(root, "diff", "--name-only", "--", ticketRel); err == nil && strings.TrimSpace(string(unstaged)) != "" {
		return os.ReadFile(filepath.Join(root, filepath.FromSlash(ticketRel)))
	}
	return os.ReadFile(filepath.Join(root, filepath.FromSlash(ticketRel)))
}

func normalizeTicketForSageFreshness(text string) string {
	lines := strings.Split(text, "\n")
	inFrontmatter := len(lines) > 0 && strings.TrimSpace(lines[0]) == "---"
	for i, line := range lines {
		if i == 0 && inFrontmatter {
			continue
		}
		if inFrontmatter && strings.TrimSpace(line) == "---" {
			inFrontmatter = false
			continue
		}
		if inFrontmatter && !strings.HasPrefix(line, " ") {
			key := strings.TrimSpace(strings.SplitN(line, ":", 2)[0])
			switch key {
			case "sage-review", "sage-review-design", "sage-review-completeness":
				lines[i] = ""
			}
		}
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func sageGitOutput(root string, args ...string) ([]byte, error) {
	cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("git %s failed: %v\n%s", strings.Join(args, " "), err, bytes.TrimSpace(out))
	}
	return out, nil
}

func isFullCommitHash(value string) bool {
	if len(value) != 40 {
		return false
	}
	for _, r := range value {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return true
}

func uniqueSageStages(stages []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, stage := range stages {
		stage = strings.TrimSpace(stage)
		if (stage == "design" || stage == "completeness") && !seen[stage] {
			seen[stage] = true
			out = append(out, stage)
		}
	}
	return out
}

func uniqueStrings(values []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, value := range values {
		if value != "" && !seen[value] {
			seen[value] = true
			out = append(out, value)
		}
	}
	return out
}

func shortCommit(hash string) string {
	if len(hash) > 8 {
		return hash[:8]
	}
	return hash
}
