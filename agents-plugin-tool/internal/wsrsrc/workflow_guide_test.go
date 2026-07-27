package wsrsrc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// workflowGuides are the three hand-maintained parallel copies of the workflow
// guide: this repo's own guide plus the two lead-bootstrap templates that ship
// downstream. No generator relates them - lead-bootstrap is deliberately absent
// from substitutionMirroredSkills - so they can only drift apart by hand.
func workflowGuides() []string {
	return []string{
		filepath.Join("..", "..", "..", "ai-docs", "WORKFLOW.md"),
		filepath.Join(fullSkillsRoot(), "lead-bootstrap", "WORKFLOW.md"),
		filepath.Join(wsflowSkillsRoot(), "lead-bootstrap", "WORKFLOW.md"),
	}
}

// implementedOnlyRule is the sentence that makes a spec verification pass
// destructive. It is matched as a substring so a reword elsewhere in the bullet
// does not break the lookup.
const implementedOnlyRule = "spec entries describe implemented behavior only"

// markdownBulletContaining returns the one top-level bullet whose text contains
// needle, continuation lines included. Bullets are split on "\n- ", so indented
// continuation lines stay with their own bullet.
func markdownBulletContaining(text, needle string) (string, bool) {
	for _, bullet := range strings.Split(text, "\n- ") {
		if strings.Contains(bullet, needle) {
			return bullet, true
		}
	}
	return "", false
}

// TestWorkflowGuidesKeepImplementationGapException pins the one sentence the
// three copies must not diverge on. Without the exception, "verify the behavior
// exists before writing or keeping its entry" reads as an instruction to delete
// Implementation Gap callouts, which are the sanctioned home for a
// known-but-unscheduled gap. A copy that loses it teaches a verification pass to
// delete the very callouts the rule exists to protect, and nothing else in
// either suite notices.
//
// Asserted on substance - the same bullet names the exception - rather than on
// byte-exact text, so ordinary rewording does not break it. This guard covers
// exactly this one sentence and says nothing about the rest of the shared
// content; see 260728-research-parallel-workflow-guide-divergence.
func TestWorkflowGuidesKeepImplementationGapException(t *testing.T) {
	for _, path := range workflowGuides() {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		bullet, ok := markdownBulletContaining(string(raw), implementedOnlyRule)
		if !ok {
			t.Fatalf("%s: no bullet states the implemented-behavior-only rule (%q)", path, implementedOnlyRule)
		}
		if !strings.Contains(bullet, "Implementation Gap") {
			t.Fatalf("%s: the implemented-behavior-only rule does not name the Implementation Gap Callout as its exception: %q", path, bullet)
		}
		if !strings.Contains(bullet, "exception") {
			t.Fatalf("%s: the implemented-behavior-only rule names Implementation Gap but not as an exception: %q", path, bullet)
		}
	}
}
