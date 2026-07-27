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
	result := TicketMutateResult{OldPath: oldPath, NewPath: newPath}
	// Soft, non-blocking tip only (must-not-forget SOFT seed) — reuses
	// tickets_verify.go's ticketUnresolvedPhaseWarning so the phase/Result
	// scan logic is not duplicated between tickets.close and tickets.verify's
	// .done/.dropped branch.
	if raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(newPath))); err == nil {
		if warning := ticketUnresolvedPhaseWarning(string(raw)); warning != "" {
			result.Tip = appendTip(result.Tip, warning)
		}
	}
	return result, nil
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

	var readySageWarning string
	if isUpwardMove(curStatus, to) {
		postures, err := prepareSageReviewForUpwardMove(filepath.Join(root, filepath.FromSlash(oldPath)), stem, opts.SageReview)
		if err != nil {
			return TicketMutateResult{}, err
		}
		if to == "ready" {
			// Recompute the ready-landing problem set from the postures
			// prepareSageReviewForUpwardMove already resolved and persisted,
			// rather than have that function build the warning itself — it
			// no longer needs to know the destination status at all (see its
			// doc comment). readyPostureProblems stays the single pure
			// implementation of "which stage(s) are non-terminal", shared
			// with tickets_verify.go's hard guardrail.
			designRequired, completenessRequired := sageReviewStageRequirement(stem)
			readySageWarning = readySagePostureWarning(readyPostureProblems(designRequired, postures.Design, completenessRequired, postures.Completeness))
		} else if err := blockedUpwardMoveError(postures); err != nil {
			// Non-ready upward moves (idea -> todo, or a demote/re-promote
			// round trip re-entering todo) have no ready-sage-posture
			// guardrail downstream to relocate enforcement to —
			// tickets_verify.go's guardrail only runs for status == "ready".
			// Removing this rejection would move enforcement to nowhere, so
			// the pre-existing hard block for a blocked required stage stays
			// here, unlike the ready-landing case (de-blocked, soft warning
			// only, per the single-chokepoint decision).
			return TicketMutateResult{}, err
		}
	}

	newPath := ticketRelPath(to, stem)
	if err := atomicGitMove(root, runner, oldPath, newPath); err != nil {
		return TicketMutateResult{}, err
	}

	result := TicketMutateResult{OldPath: oldPath, NewPath: newPath}
	if isUpwardMove(curStatus, to) {
		postures := currentSageReviewPostures(filepath.Join(root, filepath.FromSlash(newPath)), stem)
		if tip := sageReviewPostureTip(postures); tip != "" {
			result.Tip = appendTip(result.Tip, tip)
		}
	}
	if curStatus == "ready" && (to == "todo" || to == "idea") {
		result.Tip = appendTip(result.Tip, "This ticket had spec entries; clear spec:, spec-remove:, and review ## Spec Impact before re-promoting.")
	}
	if to == "ready" {
		if warning := readyGateWarning(filepath.Join(root, filepath.FromSlash(newPath)), stem); warning != "" {
			result.Tip = appendTip(result.Tip, warning)
		}
		// Soft warning only: ws/tickets.verify / ws/git.commit's
		// ready-sage-posture guardrail is the sole HARD enforcement point
		// (single chokepoint) — this call never blocks the move itself.
		if readySageWarning != "" {
			result.Tip = appendTip(result.Tip, readySageWarning)
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

// sageReviewStageRequirement reports whether a ticket category requires the
// design and/or completeness sage-review stage. It reuses the same
// ticketCategoryRE category-detection mechanism as exemptReadyGateCategories
// rather than inventing a new one: `research`/`workset` are exempt from both
// stages (mirroring their blanket spec-address-gate exemption), `epic` needs
// only design (epics never reach lead-implement so completeness never
// applies), and every other category (the default/actionable categories)
// needs both.
func sageReviewStageRequirement(stem string) (design, completeness bool) {
	category := ""
	if match := ticketCategoryRE.FindStringSubmatch(stem); len(match) == 2 {
		category = match[1]
	}
	switch category {
	case "research", "workset":
		return false, false
	case "epic":
		return true, false
	default:
		return true, true
	}
}

// sageReviewPostures carries the effective per-stage posture for a ticket;
// a stage's field is empty when that stage does not apply to the ticket's
// category.
type sageReviewPostures struct {
	Design       string
	Completeness string
}

// effectiveSageReviewPostures reads the new two-field sage-review state from
// frontmatter. When neither new field is present, it lazily migrates the
// legacy single sage-review: field: a completed/skipped/blocked legacy value
// is authoritative for both new fields (a ticket that already finished sage
// review under the old model had both existing reviewer roles run against
// it); any other legacy value (recommended/required/pending, or missing) is
// treated as absent for both fields so each is resolved fresh, same as new
// ticket stamping. This keeps migration self-healing without a bulk rewrite
// of existing ticket files.
func effectiveSageReviewPostures(fm map[string]any) (design, completeness string) {
	designValue, _ := fm["sage-review-design"].(string)
	completenessValue, _ := fm["sage-review-completeness"].(string)
	design = strings.TrimSpace(designValue)
	completeness = strings.TrimSpace(completenessValue)
	if design != "" || completeness != "" {
		return design, completeness
	}

	legacyValue, _ := fm["sage-review"].(string)
	switch strings.TrimSpace(legacyValue) {
	case "completed":
		return "completed", "completed"
	case "skipped":
		return "skipped", "skipped"
	case "blocked":
		return "blocked", "blocked"
	default:
		return "", ""
	}
}

// sageReviewStageError builds the ready-promotion posture error for a single
// stage field, mirroring the original single-field error wording.
func sageReviewStageError(field, posture string) error {
	switch posture {
	case "recommended":
		return fmt.Errorf("%s: recommended; run sage review or skip recommended review before promoting to ready", field)
	case "required":
		return fmt.Errorf("%s: required; run sage review before promoting to ready", field)
	default:
		return fmt.Errorf("%s: %s; review must complete or be skipped before promoting to ready", field, posture)
	}
}

func sageReviewBlockedError(field string) error {
	return fmt.Errorf("%s: blocked; address blocked review before promoting", field)
}

// blockedUpwardMoveError restores the hard rejection for a non-ready upward
// move (e.g. idea -> todo) that leaves a required sage-review stage blocked,
// design checked before completeness. ready/ landings are exempt from this
// call site: their blocked case is a soft warning instead
// (readySagePostureWarning), because ws/git.commit's ready-sage-posture
// guardrail is the sole HARD enforcement point there. Outside a ready
// landing, tickets_verify.go's guardrail never runs (it is gated on
// status == "ready"), so there is no chokepoint downstream to catch a
// blocked non-ready move; this call site remains the only enforcement.
func blockedUpwardMoveError(postures sageReviewPostures) error {
	if postures.Design == "blocked" {
		return sageReviewBlockedError("sage-review-design")
	}
	if postures.Completeness == "blocked" {
		return sageReviewBlockedError("sage-review-completeness")
	}
	return nil
}

// sageReviewNonWaivableAdvisory is the shared non-waivable statement +
// review-scope line surfaced everywhere a sage-review posture reaches an
// agent on its ordinary path: the ready-landing mutation-time warning
// (tickets.move, tickets.create_empty — see readySagePostureWarning) and
// tickets.sage_gate's `required` -> run result and `recommended` -> ask
// prompt (tickets_sage.go). Kept as a single constant so the two surfaces
// cannot drift in wording.
const sageReviewNonWaivableAdvisory = "sage review is not waivable per ticket (see ws/config.show for the sage_review config); " +
	"design review checks coherence, right-problem framing, and executability; completeness review checks structure, " +
	"fields, and clarity — neither judges whether the underlying research itself is settled."

// prepareSageReviewForUpwardMove resolves and persists up to two frontmatter
// fields (sage-review-design, sage-review-completeness) for an upward move,
// gated by which stages the ticket's category requires. It no longer blocks
// on posture: ws/git.commit's ready-sage-posture guardrail (tickets_verify.go)
// is the single HARD enforcement point for the ready-landing invariant; a
// caller landing at ready with a non-terminal or blocked posture gets a soft
// warning instead (built by the caller via readyPostureProblems +
// readySagePostureWarning, since this function no longer needs to know the
// destination status). The only error this function can still return is a
// genuine I/O failure from the persisted-write below.
func prepareSageReviewForUpwardMove(ticketAbsPath, stem, sageReview string) (sageReviewPostures, error) {
	designRequired, completenessRequired := sageReviewStageRequirement(stem)
	fm := frontmatter(ticketAbsPath)
	design, completeness := effectiveSageReviewPostures(fm)
	resolved := ResolvedSageReviewPosture(sageReview)

	if designRequired && (design == "" || design == "pending") {
		design = resolved
	}
	if completenessRequired && (completeness == "" || completeness == "pending") {
		completeness = resolved
	}

	// Always (re)persist the effective value for each required field in a
	// single writeFrontmatterField call. This is a no-op write when the new
	// field already held that value, and is the self-healing migration write
	// for a ticket that only had the legacy single sage-review: field (see
	// effectiveSageReviewPostures) — the migration replaces the read-time
	// inference with a persisted value on first touch, without a separate
	// bulk-rewrite script. Both fields land in the same os.WriteFile call
	// (writeFrontmatterField builds the full new content in memory before
	// writing once), so there is no partial-write window between them.
	writes := map[string]string{}
	if designRequired {
		writes["sage-review-design"] = design
	}
	if completenessRequired {
		writes["sage-review-completeness"] = completeness
	}
	if len(writes) > 0 {
		if err := writeFrontmatterField(ticketAbsPath, writes); err != nil {
			return sageReviewPostures{}, err
		}
	}
	if !designRequired {
		design = ""
	}
	if !completenessRequired {
		completeness = ""
	}

	return sageReviewPostures{Design: design, Completeness: completeness}, nil
}

// readyPostureProblem describes one required sage-review stage that is not
// yet in a ready-eligible terminal posture (completed, skipped).
type readyPostureProblem struct {
	Field   string // "sage-review-design" | "sage-review-completeness"
	Posture string
	Blocked bool
}

// readyPostureProblems reports, in design-before-completeness order, which
// required sage-review stage(s) block a ticket from landing at ready/. A
// stage is only checked when its *Required flag is true (the category gate
// from sageReviewStageRequirement); an empty posture for a required stage
// means the review was never resolved and is reported as "unset" rather than
// silently treated as not-applicable — deliberately distinct from an
// unrequired stage's empty value, so a hand-authored ready/ ticket that
// never went through TicketsMove's resolved-posture stamping is still
// caught. Pure: no I/O, no writes. This is the single implementation of the
// ready-landing terminal-posture rule, shared by TicketsMove's ready-landing
// warning builder (readySagePostureWarning, below — the soft mutation-time
// path) and TicketVerify's ready-sage-posture guardrail (which wraps the
// result in sageReviewStageError/sageReviewBlockedError for the hard FAIL
// text — the sole HARD enforcement point).
func readyPostureProblems(designRequired bool, design string, completenessRequired bool, completeness string) []readyPostureProblem {
	var problems []readyPostureProblem
	for _, stage := range []struct {
		field    string
		required bool
		posture  string
	}{
		{"sage-review-design", designRequired, design},
		{"sage-review-completeness", completenessRequired, completeness},
	} {
		if !stage.required {
			continue
		}
		switch stage.posture {
		case "completed", "skipped":
		case "blocked":
			problems = append(problems, readyPostureProblem{Field: stage.field, Posture: stage.posture, Blocked: true})
		case "":
			problems = append(problems, readyPostureProblem{Field: stage.field, Posture: "unset"})
		default:
			problems = append(problems, readyPostureProblem{Field: stage.field, Posture: stage.posture})
		}
	}
	return problems
}

// readySagePostureWarning builds the soft, mutation-time advisory for a
// ready-landing move/create that leaves a required sage-review stage in a
// non-terminal posture. Unlike sageReviewStageError/sageReviewBlockedError
// (which build tickets.verify's HARD FAIL text — the sole enforcement
// point), this text never blocks the caller; it states the consequence
// (ws/git.commit will still fail on guardrail ready-sage-posture) and a
// reachable escape. Only the first problem is reported, matching
// readyPostureProblems' design-before-completeness order and the historical
// single-field error precedent. The "unreviewed" and "blocked" variants are
// deliberately distinct sentences (per ticket decision): unreviewed means
// review has not run yet; blocked means a prior review already found
// unresolved issues — the two imply different next actions for the caller.
// Their instruction clauses differ for the same reason: ws/tickets.sage_gate
// resolves an unreviewed posture, but it is a no-op on `blocked` (it returns
// stop_blocked before any resolution logic), so only ws/tickets.sage_stamp
// with a non-block verdict can clear that state. Naming an escape no tool can
// perform would reproduce the dead end this warning exists to remove.
func readySagePostureWarning(problems []readyPostureProblem) string {
	if len(problems) == 0 {
		return ""
	}
	first := problems[0]
	var state, instruction string
	if first.Blocked {
		state = "blocked (a prior review found unresolved issues)"
		instruction = "Address the blocker, then re-run the review and record a non-block verdict via " +
			"ws/tickets.sage_stamp(stem, stage, verdicts) to resolve it — ws/tickets.sage_gate cannot clear a blocked posture"
	} else {
		state = "unreviewed (posture " + first.Posture + "; review has not run yet)"
		instruction = "Call ws/tickets.sage_gate(stem, landing: \"ready\") to resolve it"
	}
	return fmt.Sprintf(
		"%s is %s; ws/git.commit will fail on guardrail ready-sage-posture until this resolves. "+
			"%s; %s",
		first.Field, state, instruction, sageReviewNonWaivableAdvisory,
	)
}

// currentSageReviewPostures returns the effective per-stage posture for a
// ticket after a move, applying the same category gating and legacy
// migration read as prepareSageReviewForUpwardMove so the post-move tip
// reflects the correct value even for a legacy-only ticket.
func currentSageReviewPostures(ticketAbsPath, stem string) sageReviewPostures {
	designRequired, completenessRequired := sageReviewStageRequirement(stem)
	fm := frontmatter(ticketAbsPath)
	design, completeness := effectiveSageReviewPostures(fm)
	if !designRequired {
		design = ""
	}
	if !completenessRequired {
		completeness = ""
	}
	return sageReviewPostures{Design: design, Completeness: completeness}
}

func sageReviewPostureTip(postures sageReviewPostures) string {
	var parts []string
	if postures.Design != "" {
		parts = append(parts, "design "+postures.Design)
	}
	if postures.Completeness != "" {
		parts = append(parts, "completeness "+postures.Completeness)
	}
	if len(parts) == 0 {
		return ""
	}
	return "sage review posture: " + strings.Join(parts, ", ") + "."
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
