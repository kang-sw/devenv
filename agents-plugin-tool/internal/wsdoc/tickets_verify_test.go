package wsdoc

import (
	"strings"
	"testing"
)

// validReadyTicketBody is a ready/ ticket that satisfies every hard
// guardrail and carries spec addressing, so it also produces no soft warning.
const validReadyTicketBody = "---\n" +
	"title: Valid ready ticket\n" +
	"sage-review-design: completed\n" +
	"sage-review-completeness: completed\n" +
	"spec:\n" +
	"  - 260723-spec-demo\n" +
	"---\n\n" +
	"# Valid ready ticket\n\n" +
	"## Phases\n\n" +
	"### Phase 1: First\n\n" +
	"### Result (abc123) - 2026-07-23\n\n" +
	"Done.\n"

func findingGuardrails(t *testing.T, findings []VerifyFinding) []string {
	t.Helper()
	out := make([]string, 0, len(findings))
	for _, f := range findings {
		out = append(out, f.Guardrail)
	}
	return out
}

func containsGuardrail(guardrails []string, want string) bool {
	for _, g := range guardrails {
		if g == want {
			return true
		}
	}
	return false
}

func TestTicketVerifyPassingFixtureIsOK(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, "ai-docs/tickets/ready/260723-feat-valid.md", validReadyTicketBody)

	result, err := TicketVerify(root, []string{"ai-docs/tickets/ready/260723-feat-valid.md"})
	if err != nil {
		t.Fatalf("TicketVerify returned error: %v", err)
	}
	if !result.OK || len(result.Findings) != 0 {
		t.Fatalf("result = %#v, want OK with no findings", result)
	}
	if len(result.Warnings) != 0 {
		t.Fatalf("result.Warnings = %#v, want none (spec: frontmatter present)", result.Warnings)
	}
}

func TestTicketVerifyRequiresAtLeastOnePath(t *testing.T) {
	if _, err := TicketVerify(t.TempDir(), nil); err == nil {
		t.Fatal("TicketVerify accepted empty paths")
	}
}

func TestTicketVerifySkipsNonTicketPaths(t *testing.T) {
	root := t.TempDir()
	result, err := TicketVerify(root, []string{"src/main.go", "ai-docs/spec/demo.md"})
	if err != nil {
		t.Fatalf("TicketVerify returned error: %v", err)
	}
	if !result.OK || len(result.Findings) != 0 || len(result.Warnings) != 0 {
		t.Fatalf("result = %#v, want a clean no-op for non-ticket paths", result)
	}
}

func TestTicketVerifyBadStemIsHardFinding(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, "ai-docs/tickets/todo/not-a-valid-stem.md", "---\ntitle: Bad stem\n---\n\nBody.\n")

	result, err := TicketVerify(root, []string{"ai-docs/tickets/todo/not-a-valid-stem.md"})
	if err != nil {
		t.Fatalf("TicketVerify returned error: %v", err)
	}
	if result.OK {
		t.Fatalf("result.OK = true, want false for a malformed stem")
	}
	if guardrails := findingGuardrails(t, result.Findings); !containsGuardrail(guardrails, "stem") {
		t.Fatalf("findings = %#v, want a stem guardrail finding", result.Findings)
	}
}

func TestTicketVerifyWrongStatusDirIsHardFinding(t *testing.T) {
	root := t.TempDir()
	// "wip" is accepted by wsgit's legacy ticketStatusStem set but is not one
	// of wsdoc.statusDirs' canonical five directories; verify must follow the
	// canonical set (see {#260720-wsdoc-commit-boundary} plan survey notes).
	mustWrite(t, root, "ai-docs/tickets/wip/260723-feat-demo.md", "---\ntitle: Demo\n---\n\nBody.\n")

	result, err := TicketVerify(root, []string{"ai-docs/tickets/wip/260723-feat-demo.md"})
	if err != nil {
		t.Fatalf("TicketVerify returned error: %v", err)
	}
	if result.OK {
		t.Fatalf("result.OK = true, want false for a wip status directory")
	}
	if guardrails := findingGuardrails(t, result.Findings); !containsGuardrail(guardrails, "status-dir") {
		t.Fatalf("findings = %#v, want a status-dir guardrail finding", result.Findings)
	}
}

func TestTicketVerifyReadySagePostureGuardrail(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
	}{
		{
			name: "missing",
			body: "---\ntitle: Missing posture\n---\n\nBody.\n",
		},
		{
			name: "blocked",
			body: "---\ntitle: Blocked posture\nsage-review-design: blocked\nsage-review-completeness: completed\n---\n\nBody.\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			path := "ai-docs/tickets/ready/260723-feat-" + tc.name + ".md"
			mustWrite(t, root, path, tc.body)

			result, err := TicketVerify(root, []string{path})
			if err != nil {
				t.Fatalf("TicketVerify returned error: %v", err)
			}
			if result.OK {
				t.Fatalf("result.OK = true, want false for %s ready sage-review posture", tc.name)
			}
			if guardrails := findingGuardrails(t, result.Findings); !containsGuardrail(guardrails, "ready-sage-posture") {
				t.Fatalf("findings = %#v, want a ready-sage-posture guardrail finding", result.Findings)
			}
		})
	}
}

func TestTicketVerifyMalformedFrontmatterFenceIsHardFinding(t *testing.T) {
	root := t.TempDir()
	// Opening fence present, closing fence missing.
	mustWrite(t, root, "ai-docs/tickets/todo/260723-feat-fence.md", "---\ntitle: No closing fence\n\n# Body\n")

	result, err := TicketVerify(root, []string{"ai-docs/tickets/todo/260723-feat-fence.md"})
	if err != nil {
		t.Fatalf("TicketVerify returned error: %v", err)
	}
	if result.OK {
		t.Fatalf("result.OK = true, want false for a missing closing frontmatter fence")
	}
	if guardrails := findingGuardrails(t, result.Findings); !containsGuardrail(guardrails, "frontmatter-fence") {
		t.Fatalf("findings = %#v, want a frontmatter-fence guardrail finding", result.Findings)
	}
}

func TestTicketVerifyFencedEmptyBodyIsNotFlagged(t *testing.T) {
	root := t.TempDir()
	// A legitimately fenced-but-empty-body ticket must not be flagged by the
	// frontmatter-fence guardrail (Codebase Findings, frontmatter.go note).
	mustWrite(t, root, "ai-docs/tickets/todo/260723-feat-empty-body.md", "---\ntitle: Empty body\n---\n")

	result, err := TicketVerify(root, []string{"ai-docs/tickets/todo/260723-feat-empty-body.md"})
	if err != nil {
		t.Fatalf("TicketVerify returned error: %v", err)
	}
	if guardrails := findingGuardrails(t, result.Findings); containsGuardrail(guardrails, "frontmatter-fence") {
		t.Fatalf("findings = %#v, frontmatter-fence guardrail must not fire on a fenced empty body", result.Findings)
	}
}

func TestTicketVerifyMalformedPhaseAndResultHeadingsAreHardFindings(t *testing.T) {
	root := t.TempDir()
	body := "---\ntitle: Malformed headings\n---\n\n" +
		"## Phases\n\n" +
		"### Phase First\n\n" + // missing "<n>: "
		"### Result 2026-07-23\n\n" + // missing "(<hash>) - "
		"Body.\n"
	mustWrite(t, root, "ai-docs/tickets/todo/260723-feat-headings.md", body)

	result, err := TicketVerify(root, []string{"ai-docs/tickets/todo/260723-feat-headings.md"})
	if err != nil {
		t.Fatalf("TicketVerify returned error: %v", err)
	}
	if result.OK {
		t.Fatalf("result.OK = true, want false for malformed Phase/Result headings")
	}
	guardrails := findingGuardrails(t, result.Findings)
	count := 0
	for _, g := range guardrails {
		if g == "phase-result-heading" {
			count++
		}
	}
	if count != 2 {
		t.Fatalf("phase-result-heading findings = %d, want 2 (one per malformed heading); findings=%#v", count, result.Findings)
	}
}

func TestTicketVerifyWellFormedPhaseAndResultHeadingsPass(t *testing.T) {
	root := t.TempDir()
	body := "---\ntitle: Well-formed headings\n---\n\n" +
		"## Phases\n\n" +
		"### Phase 1: First\n\n" +
		"### Result (abc123) - 2026-07-23\n\n" +
		"Done.\n\n" +
		"#### Edition (def456) - 2026-07-24\n\n" +
		"Tweak.\n"
	mustWrite(t, root, "ai-docs/tickets/todo/260723-feat-good-headings.md", body)

	result, err := TicketVerify(root, []string{"ai-docs/tickets/todo/260723-feat-good-headings.md"})
	if err != nil {
		t.Fatalf("TicketVerify returned error: %v", err)
	}
	if !result.OK {
		t.Fatalf("result = %#v, want OK for well-formed Phase/Result/Edition headings", result)
	}
}

func TestTicketVerifyCloseDateFieldGuardrail(t *testing.T) {
	for _, tc := range []struct {
		status string
		path   string
	}{
		{status: ".done", path: "ai-docs/tickets/.done/260723-feat-done.md"},
		{status: ".dropped", path: "ai-docs/tickets/.dropped/260723-feat-dropped.md"},
	} {
		t.Run(tc.status, func(t *testing.T) {
			root := t.TempDir()
			mustWrite(t, root, tc.path, "---\ntitle: Closed without date\n---\n\nBody.\n")

			result, err := TicketVerify(root, []string{tc.path})
			if err != nil {
				t.Fatalf("TicketVerify returned error: %v", err)
			}
			if result.OK {
				t.Fatalf("result.OK = true, want false for a missing close date field in %s", tc.status)
			}
			if guardrails := findingGuardrails(t, result.Findings); !containsGuardrail(guardrails, "close-date-field") {
				t.Fatalf("findings = %#v, want a close-date-field guardrail finding", result.Findings)
			}
		})
	}
}

func TestTicketVerifyCloseDateFieldPresentPasses(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, "ai-docs/tickets/.done/260723-feat-done.md", "---\ntitle: Closed\ncompleted: 2026-07-23\n---\n\nBody.\n")

	result, err := TicketVerify(root, []string{"ai-docs/tickets/.done/260723-feat-done.md"})
	if err != nil {
		t.Fatalf("TicketVerify returned error: %v", err)
	}
	if guardrails := findingGuardrails(t, result.Findings); containsGuardrail(guardrails, "close-date-field") {
		t.Fatalf("findings = %#v, close-date-field guardrail must not fire when completed: is set", result.Findings)
	}
}

func TestTicketVerifySpecAddressIsSoftWarnOnly(t *testing.T) {
	root := t.TempDir()
	body := "---\n" +
		"title: No spec addressing\n" +
		"sage-review-design: completed\n" +
		"sage-review-completeness: completed\n" +
		"---\n\nBody.\n"
	mustWrite(t, root, "ai-docs/tickets/ready/260723-feat-nospec.md", body)

	result, err := TicketVerify(root, []string{"ai-docs/tickets/ready/260723-feat-nospec.md"})
	if err != nil {
		t.Fatalf("TicketVerify returned error: %v", err)
	}
	if !result.OK {
		t.Fatalf("result.OK = false, want true: spec-address must warn, not block; findings=%#v", result.Findings)
	}
	if guardrails := findingGuardrails(t, result.Findings); containsGuardrail(guardrails, "spec-address") {
		t.Fatalf("findings = %#v, spec-address must never be a hard finding", result.Findings)
	}
	warningGuardrails := make([]string, 0, len(result.Warnings))
	for _, w := range result.Warnings {
		warningGuardrails = append(warningGuardrails, w.Guardrail)
	}
	if !containsGuardrail(warningGuardrails, "spec-address") {
		t.Fatalf("warnings = %#v, want a spec-address warning", result.Warnings)
	}
}

func TestTicketVerifySpecAddressExemptCategorySkipsWarning(t *testing.T) {
	root := t.TempDir()
	// research/workset/epic categories are exempt from the ready spec-address
	// gate (exemptReadyGateCategories); research is also exempt from both
	// sage-review stages (sageReviewStageRequirement), so this fixture is a
	// clean pass with no findings and no warnings.
	body := "---\ntitle: Research ticket\n---\n\nBody.\n"
	mustWrite(t, root, "ai-docs/tickets/ready/260723-research-nospec.md", body)

	result, err := TicketVerify(root, []string{"ai-docs/tickets/ready/260723-research-nospec.md"})
	if err != nil {
		t.Fatalf("TicketVerify returned error: %v", err)
	}
	if !result.OK {
		t.Fatalf("result = %#v, want OK for an exempt-category ready ticket", result)
	}
	if len(result.Warnings) != 0 {
		t.Fatalf("warnings = %#v, want none for an exempt category", result.Warnings)
	}
}

func TestTicketVerifyReportsPathOnEveryFinding(t *testing.T) {
	root := t.TempDir()
	path := "ai-docs/tickets/todo/bad-stem.md"
	mustWrite(t, root, path, "---\ntitle: Bad\n---\n\nBody.\n")

	result, err := TicketVerify(root, []string{path})
	if err != nil {
		t.Fatalf("TicketVerify returned error: %v", err)
	}
	for _, finding := range result.Findings {
		if finding.Path != path {
			t.Fatalf("finding.Path = %q, want %q; finding=%#v", finding.Path, path, finding)
		}
	}
}

func TestTicketVerifyFileNotFoundIsHardFinding(t *testing.T) {
	root := t.TempDir()
	path := "ai-docs/tickets/todo/260723-feat-missing.md"

	result, err := TicketVerify(root, []string{path})
	if err != nil {
		t.Fatalf("TicketVerify returned error: %v", err)
	}
	if result.OK {
		t.Fatalf("result.OK = true, want false for a ticket path that does not exist on disk")
	}
	if guardrails := findingGuardrails(t, result.Findings); !containsGuardrail(guardrails, "file-exists") {
		t.Fatalf("findings = %#v, want a file-exists guardrail finding", result.Findings)
	}
}

// TestTicketVerifyMessagesAreActionable is a light smoke check that finding
// messages carry enough text to act on, not just a bare guardrail label.
func TestTicketVerifyMessagesAreActionable(t *testing.T) {
	root := t.TempDir()
	path := "ai-docs/tickets/ready/260723-feat-blocked.md"
	mustWrite(t, root, path, "---\ntitle: Blocked\nsage-review-design: blocked\nsage-review-completeness: completed\n---\n\nBody.\n")

	result, err := TicketVerify(root, []string{path})
	if err != nil {
		t.Fatalf("TicketVerify returned error: %v", err)
	}
	found := false
	for _, finding := range result.Findings {
		if finding.Guardrail == "ready-sage-posture" {
			found = true
			if !strings.Contains(finding.Message, "blocked") {
				t.Fatalf("message = %q, want it to mention the blocked posture", finding.Message)
			}
		}
	}
	if !found {
		t.Fatalf("findings = %#v, want a ready-sage-posture finding", result.Findings)
	}
}
