package wsdoc

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// GitRunner is the minimal git surface the mutation helpers need. It matches
// wsgit.Runner so callers can pass wsgit.ExecRunner{} without wsdoc importing
// wsgit (which would invert the package dependency direction).
type GitRunner interface {
	RunGit(ctx context.Context, root string, args ...string) ([]byte, error)
}

type TicketCloseOptions struct {
	TicketStem string
	Status     string // "done" | "dropped"
	Resolution string // optional
	Today      string // YYYY-MM-DD, caller-supplied for testability
}

type TicketMoveOptions struct {
	TicketStem string
	To         string // "idea" | "todo" | "ready"
	SageReview string // resolved sage_review config value ("" | "off" | "auto" | "ask")
	Today      string // kept for symmetry; unused in the initial implementation
}

type TicketMutateResult struct {
	OldPath string
	NewPath string
	Tip     string
}

// statusDirs maps a status token to its tickets-relative directory name.
var statusDirs = map[string]string{
	"idea":     "idea",
	"todo":     "todo",
	"ready":    "ready",
	".done":    ".done",
	".dropped": ".dropped",
}

func TicketsClose(root string, runner GitRunner, opts TicketCloseOptions) (TicketMutateResult, error) {
	stem := strings.TrimSpace(opts.TicketStem)
	if !ticketStemRE.MatchString(stem) {
		return TicketMutateResult{}, fmt.Errorf("stem must be a ticket stem")
	}

	status := strings.TrimSpace(opts.Status)
	var targetDir, dateField string
	switch status {
	case "done":
		targetDir = ".done"
		dateField = "completed"
	case "dropped":
		targetDir = ".dropped"
		dateField = "dropped"
	default:
		return TicketMutateResult{}, fmt.Errorf("status must be done or dropped")
	}

	oldPath, curStatus, err := findTicketPath(root, stem)
	if err != nil {
		return TicketMutateResult{}, err
	}
	if curStatus == ".done" || curStatus == ".dropped" {
		return TicketMutateResult{}, fmt.Errorf("ticket already closed: %s is in %s", stem, curStatus)
	}

	absOld := filepath.Join(root, filepath.FromSlash(oldPath))
	if err := writeFrontmatterField(absOld, map[string]string{dateField: opts.Today}); err != nil {
		return TicketMutateResult{}, err
	}
	if resolution := strings.TrimSpace(opts.Resolution); resolution != "" {
		if err := appendResolution(absOld, opts.Today, resolution); err != nil {
			return TicketMutateResult{}, err
		}
	}

	newPath := ticketRelPath(targetDir, stem)
	if err := atomicGitMove(root, runner, oldPath, newPath); err != nil {
		return TicketMutateResult{}, err
	}
	return TicketMutateResult{OldPath: oldPath, NewPath: newPath}, nil
}

func TicketsMove(root string, runner GitRunner, opts TicketMoveOptions) (TicketMutateResult, error) {
	stem := strings.TrimSpace(opts.TicketStem)
	if !ticketStemRE.MatchString(stem) {
		return TicketMutateResult{}, fmt.Errorf("stem must be a ticket stem")
	}

	to := strings.TrimSpace(opts.To)
	switch to {
	case "idea", "todo", "ready":
	default:
		return TicketMutateResult{}, fmt.Errorf("to must be idea, todo, or ready")
	}

	oldPath, curStatus, err := findTicketPath(root, stem)
	if err != nil {
		return TicketMutateResult{}, err
	}
	if curStatus == to {
		return TicketMutateResult{}, fmt.Errorf("ticket already at status %s: %s", to, stem)
	}
	if curStatus == ".done" || curStatus == ".dropped" {
		return TicketMutateResult{}, fmt.Errorf("ticket is closed (%s); reopen is out of scope", curStatus)
	}

	if isUpwardMove(curStatus, to) {
		if _, err := prepareSageReviewForUpwardMove(filepath.Join(root, filepath.FromSlash(oldPath)), opts.SageReview, to); err != nil {
			return TicketMutateResult{}, err
		}
	}

	newPath := ticketRelPath(to, stem)
	if err := atomicGitMove(root, runner, oldPath, newPath); err != nil {
		return TicketMutateResult{}, err
	}

	result := TicketMutateResult{OldPath: oldPath, NewPath: newPath}
	if isUpwardMove(curStatus, to) {
		if posture, err := currentSageReviewPosture(filepath.Join(root, filepath.FromSlash(newPath))); err == nil {
			result.Tip = appendTip(result.Tip, sageReviewPostureTip(posture))
		}
	}
	if curStatus == "ready" && (to == "todo" || to == "idea") {
		result.Tip = appendTip(result.Tip, "This ticket had spec entries; clear spec:, spec-remove:, and review ## Spec Impact before re-promoting.")
	}
	if to == "ready" {
		if warning := readyGateWarning(filepath.Join(root, filepath.FromSlash(newPath)), stem); warning != "" {
			result.Tip = appendTip(result.Tip, warning)
		}
	}
	return result, nil
}

// appendTip joins advisory tip messages so multiple warnings from independent
// checks all remain visible in the response instead of overwriting each other.
func appendTip(existing, addition string) string {
	if existing == "" {
		return addition
	}
	if addition == "" {
		return existing
	}
	return existing + " " + addition
}

// ticketCategoryRE extracts the category token from a ticket stem
// (YYMMDD-<category>-<slug>), mirroring the lead-write-ticket convention.
var ticketCategoryRE = regexp.MustCompile(`^\d{6}-([a-z]+)-`)

// exemptReadyGateCategories are ticket categories exempt from the spec-address
// gate enforced by the lead-write-ticket playbook when promoting to ready/.
var exemptReadyGateCategories = map[string]bool{
	"epic":     true,
	"research": true,
	"workset":  true,
}

// readyGateWarning returns a soft, non-blocking warning when a non-exempt
// ticket is moved to ready/ without detected spec addressing (a confirmed
// spec:/spec-remove: frontmatter entry or a ## Spec Impact section). The
// spec-address gate itself is documented and enforced only at the
// lead-write-ticket playbook layer; this primitive-layer warning exists so a
// lead calling tickets_move directly still gets a signal.
func readyGateWarning(ticketAbsPath, stem string) string {
	match := ticketCategoryRE.FindStringSubmatch(stem)
	if len(match) == 2 && exemptReadyGateCategories[match[1]] {
		return ""
	}

	fm := frontmatter(ticketAbsPath)
	if len(scalarList(fm["spec"])) > 0 || len(scalarList(fm["spec-remove"])) > 0 {
		return ""
	}

	raw, err := os.ReadFile(ticketAbsPath)
	if err == nil {
		for _, line := range strings.Split(string(raw), "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), "## Spec Impact") {
				return ""
			}
		}
	}

	return "ready gate is normally enforced by lead-write-ticket; no spec addressing detected."
}

// statusRank orders the active status axis idea < todo < ready so a move toward
// a higher rank counts as upward (promotion).
func statusRank(status string) int {
	switch status {
	case "idea":
		return 0
	case "todo":
		return 1
	case "ready":
		return 2
	default:
		return -1
	}
}

func isUpwardMove(from, to string) bool {
	fr, tr := statusRank(from), statusRank(to)
	if fr < 0 || tr < 0 {
		return false
	}
	return tr > fr
}

func ResolvedSageReviewPosture(sageReview string) string {
	switch strings.ToLower(strings.TrimSpace(sageReview)) {
	case "ask":
		return "recommended"
	case "auto":
		return "required"
	default:
		return "skipped"
	}
}

func prepareSageReviewForUpwardMove(ticketAbsPath, sageReview, to string) (string, error) {
	resolved := ResolvedSageReviewPosture(sageReview)
	fm := frontmatter(ticketAbsPath)
	value, _ := fm["sage-review"].(string)
	posture := strings.TrimSpace(value)
	if posture == "" || posture == "pending" {
		if err := writeFrontmatterField(ticketAbsPath, map[string]string{"sage-review": resolved}); err != nil {
			return "", err
		}
		posture = resolved
	}

	if posture == "blocked" {
		return posture, fmt.Errorf("sage-review: blocked; address blocked review before promoting")
	}
	if to != "ready" {
		return posture, nil
	}

	switch posture {
	case "completed", "skipped":
		return posture, nil
	case "recommended":
		return posture, fmt.Errorf("sage-review: recommended; run sage review or skip recommended review before promoting to ready")
	case "required":
		return posture, fmt.Errorf("sage-review: required; run sage review before promoting to ready")
	default:
		return posture, fmt.Errorf("sage-review: %s; review must complete or be skipped before promoting to ready", posture)
	}
}

func currentSageReviewPosture(ticketAbsPath string) (string, error) {
	fm := frontmatter(ticketAbsPath)
	value, _ := fm["sage-review"].(string)
	posture := strings.TrimSpace(value)
	if posture == "" {
		return "", fmt.Errorf("sage-review missing")
	}
	return posture, nil
}

func sageReviewPostureTip(posture string) string {
	switch posture {
	case "skipped":
		return "sage review posture: skipped."
	case "recommended":
		return "sage review posture: recommended; run sage review or skip it before ready."
	case "required":
		return "sage review posture: required; sage review must run before ready."
	case "completed":
		return "sage review posture: completed."
	default:
		return "sage review posture: " + posture + "."
	}
}

func ticketRelPath(statusDir, stem string) string {
	return strings.Join([]string{"ai-docs", "tickets", statusDir, stem + ".md"}, "/")
}

// findTicketPath scans the five status directories for <stem>.md and returns the
// repo-relative forward-slash path and its status directory token.
func findTicketPath(root, stem string) (path string, status string, err error) {
	if !ticketStemRE.MatchString(strings.TrimSpace(stem)) {
		return "", "", fmt.Errorf("stem must be a ticket stem")
	}
	for _, status := range []string{"idea", "todo", "ready", ".done", ".dropped"} {
		candidate := filepath.Join(root, "ai-docs", "tickets", statusDirs[status], stem+".md")
		if info, statErr := os.Stat(candidate); statErr == nil && !info.IsDir() {
			return ticketRelPath(status, stem), status, nil
		}
	}
	return "", "", fmt.Errorf("ticket not found: %s", stem)
}

// writeFrontmatterField updates or inserts scalar key/value pairs inside the
// leading --- frontmatter fences, preserving all other content verbatim.
func writeFrontmatterField(path string, fields map[string]string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	text := string(raw)
	if !strings.HasPrefix(text, "---") {
		return fmt.Errorf("no frontmatter fences in %s", path)
	}
	lines := strings.Split(text, "\n")
	end := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			end = i
			break
		}
	}
	if end == -1 {
		return fmt.Errorf("no closing frontmatter fence in %s", path)
	}

	for key, value := range fields {
		newLine := key + ": " + value
		replaced := false
		for i := 1; i < end; i++ {
			if !strings.HasPrefix(lines[i], " ") && strings.HasPrefix(lines[i], key+":") {
				lines[i] = newLine
				replaced = true
				break
			}
		}
		if !replaced {
			lines = append(lines[:end], append([]string{newLine}, lines[end:]...)...)
			end++
		}
	}

	return os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644)
}

func appendResolution(path, today, resolution string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	section := "\n\n## Resolution (" + today + ")\n\n" + resolution + "\n"
	return os.WriteFile(path, append(raw, []byte(section)...), 0o644)
}

// atomicGitMove always stages the working-tree edit before the move so a prior
// frontmatter write can never be left unstaged behind the rename.
func atomicGitMove(root string, runner GitRunner, oldPath, newPath string) error {
	ctx := context.Background()
	if _, err := runner.RunGit(ctx, root, "add", oldPath); err != nil {
		return err
	}
	// git mv does not create intermediate destination directories (e.g. .done/
	// on a fresh repo), so ensure the parent exists before the rename.
	if err := os.MkdirAll(filepath.Dir(filepath.Join(root, filepath.FromSlash(newPath))), 0o755); err != nil {
		return err
	}
	if _, err := runner.RunGit(ctx, root, "mv", "--force", oldPath, newPath); err != nil {
		return err
	}
	return nil
}
