package wsdoc

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// VerifyFinding is a single guardrail result (hard failure or soft warning)
// against one ticket path.
type VerifyFinding struct {
	Path      string
	Guardrail string
	Message   string
}

// VerifyResult aggregates every guardrail outcome across the paths passed to
// TicketVerify. OK is true only when Findings is empty; Warnings never affect
// OK (spec-address is soft-warn only, per the ticket's stated posture).
type VerifyResult struct {
	OK       bool
	Findings []VerifyFinding
	Warnings []VerifyFinding
}

var (
	ticketPhaseHeadingRE   = regexp.MustCompile(`^### Phase \d+: .+$`)
	ticketResultHeadingRE  = regexp.MustCompile(`^### Result \(\S+\) - \d{4}-\d{2}-\d{2}$`)
	ticketEditionHeadingRE = regexp.MustCompile(`^#### Edition \(\S+\) - \d{4}-\d{2}-\d{2}$`)
)

// TicketVerify runs the deterministic ticket-write guardrails against every
// ticket-shaped path in paths (ai-docs/tickets/<status>/<stem>.md; other
// paths are silently skipped — non-ticket paths are not verify's concern).
// It never mutates a file. A non-nil error return is reserved for
// caller-input problems (e.g. empty paths); guardrail failures are always
// reported through VerifyResult.Findings, never as a Go error, so a
// commit-gate caller is responsible for turning a non-OK VerifyResult into
// its own blocking error.
func TicketVerify(root string, paths []string) (VerifyResult, error) {
	if len(paths) == 0 {
		return VerifyResult{}, fmt.Errorf("paths requires at least one path")
	}
	result := VerifyResult{}
	for _, path := range paths {
		status, stem, ok := ticketVerifyPathShape(path)
		if !ok {
			continue
		}
		verifyTicketFile(root, path, status, stem, &result)
	}
	result.OK = len(result.Findings) == 0
	return result, nil
}

// ticketVerifyPathShape recognizes ai-docs/tickets/<status>/<stem>.md shaped
// paths without validating status or stem — those are guardrails in their
// own right (see verifyTicketFile) so a malformed status dir or stem still
// reaches the rest of the checks instead of being silently skipped.
func ticketVerifyPathShape(path string) (status, stem string, ok bool) {
	normalized := filepath.ToSlash(filepath.Clean(path))
	const prefix = "ai-docs/tickets/"
	if !strings.HasPrefix(normalized, prefix) || !strings.HasSuffix(normalized, ".md") {
		return "", "", false
	}
	rest := strings.TrimPrefix(normalized, prefix)
	parts := strings.Split(rest, "/")
	if len(parts) != 2 {
		return "", "", false
	}
	stem = strings.TrimSuffix(parts[1], ".md")
	if stem == "" {
		return "", "", false
	}
	return parts[0], stem, true
}

func verifyTicketFile(root, path, status, stem string, result *VerifyResult) {
	addFinding := func(guardrail, message string) {
		result.Findings = append(result.Findings, VerifyFinding{Path: path, Guardrail: guardrail, Message: message})
	}
	addWarning := func(guardrail, message string) {
		result.Warnings = append(result.Warnings, VerifyFinding{Path: path, Guardrail: guardrail, Message: message})
	}

	if !ticketStemRE.MatchString(stem) {
		addFinding("stem", fmt.Sprintf("stem %q does not match the ticket stem pattern (\\d{6}-[\\w-]+)", stem))
	}
	// statusDirs (tickets_mutate.go) is the canonical five-directory set
	// (idea/todo/ready/.done/.dropped); deliberately not wsgit's looser,
	// "wip"-inclusive ticketStatusStem set (see
	// {#260720-wsdoc-commit-boundary} plan survey notes).
	if _, ok := statusDirs[status]; !ok {
		addFinding("status-dir", fmt.Sprintf("status directory %q is not one of idea, todo, ready, .done, .dropped", status))
	}

	absPath := filepath.Join(root, filepath.FromSlash(path))
	raw, err := os.ReadFile(absPath)
	if err != nil {
		addFinding("file-exists", fmt.Sprintf("cannot read ticket file: %v", err))
		return
	}
	text := string(raw)

	if problem := ticketFrontmatterFenceProblem(text); problem != "" {
		addFinding("frontmatter-fence", problem)
	}

	if status == "ready" {
		postures := currentSageReviewPostures(absPath, stem)
		for _, problem := range readyPostureProblems(postures) {
			var err error
			if problem.Blocked {
				err = sageReviewBlockedError(problem.Field)
			} else {
				err = sageReviewStageError(problem.Field, problem.Posture)
			}
			addFinding("ready-sage-posture", err.Error())
		}
	}

	if status == ".done" || status == ".dropped" {
		fm := frontmatter(absPath)
		if problem := ticketCloseDateFieldProblem(fm, status); problem != "" {
			addFinding("close-date-field", problem)
		}
	}

	for _, problem := range ticketPhaseHeadingProblems(text) {
		addFinding("phase-result-heading", problem)
	}

	if status == "ready" {
		if warning := readyGateWarning(absPath, stem); warning != "" {
			addWarning("spec-address", warning)
		}
	}
}

// ticketFrontmatterFenceProblem scans raw ticket text directly for a well
// formed leading frontmatter fence pair, rather than inferring malformity
// from frontmatter()'s nil return — frontmatter() returns nil both for "no
// frontmatter" and "malformed frontmatter", which would misreport a
// legitimately fenced-but-empty-body ticket as malformed.
func ticketFrontmatterFenceProblem(raw string) string {
	if strings.TrimSpace(raw) == "" {
		// An empty file has no fence to be malformed; the stem/status-dir and
		// file-exists guardrails already surface a structural problem here.
		return ""
	}
	lines := strings.Split(raw, "\n")
	if strings.TrimRight(lines[0], "\r") != "---" {
		return "missing opening frontmatter fence (first line must be exactly ---)"
	}
	for i := 1; i < len(lines); i++ {
		if strings.TrimRight(lines[i], "\r") == "---" {
			return ""
		}
	}
	return "missing closing frontmatter fence"
}

// ticketCloseDateFieldProblem checks the dated close field (completed for
// .done, dropped for .dropped) is present and non-empty.
func ticketCloseDateFieldProblem(fm map[string]any, status string) string {
	var field string
	switch status {
	case ".done":
		field = "completed"
	case ".dropped":
		field = "dropped"
	default:
		return ""
	}
	value, _ := fm[field].(string)
	if strings.TrimSpace(value) == "" {
		return fmt.Sprintf("%s: missing or empty date field required for a ticket in %s/", field, status)
	}
	return ""
}

// ticketPhaseHeadingProblems runs a stricter well-formedness pass over
// "### Phase ", "### Result", and "#### Edition" heading lines than
// ticketPhases' lenient prefix-only match: any line with one of these
// prefixes must fully match the AGENTS.md commit-rule heading format.
// ticketPhases itself is left untouched so its existing lenient callers
// (ticket listing) are undisturbed; append-only ordering of phase/Result
// content is a diff-level property and out of scope here.
func ticketPhaseHeadingProblems(raw string) []string {
	var problems []string
	for i, line := range strings.Split(raw, "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, "### Phase "):
			if !ticketPhaseHeadingRE.MatchString(trimmed) {
				problems = append(problems, fmt.Sprintf("line %d: malformed Phase heading (want \"### Phase <n>: <title>\"): %q", i+1, trimmed))
			}
		case strings.HasPrefix(trimmed, "### Result"):
			if !ticketResultHeadingRE.MatchString(trimmed) {
				problems = append(problems, fmt.Sprintf("line %d: malformed Result heading (want \"### Result (<hash>) - YYYY-MM-DD\"): %q", i+1, trimmed))
			}
		case strings.HasPrefix(trimmed, "#### Edition"):
			if !ticketEditionHeadingRE.MatchString(trimmed) {
				problems = append(problems, fmt.Sprintf("line %d: malformed Edition heading (want \"#### Edition (<hash>) - YYYY-MM-DD\"): %q", i+1, trimmed))
			}
		}
	}
	return problems
}
