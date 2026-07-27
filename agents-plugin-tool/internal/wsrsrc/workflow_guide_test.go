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

// implementedOnlyRuleTokens anchor the bullet that makes a spec verification
// pass destructive. Two tokens rather than one byte-exact sentence: the rule
// reads "spec entries describe implemented behavior only" today, but "only
// implemented behavior" is the same correct document, and the bullet is
// hard-wrapped, so any reflow can drop a newline into the middle of a longer
// needle. Both tokens are required and the match must be unique, so a loosened
// anchor cannot silently land on a neighbouring bullet and leave the exception
// check asserting against the wrong text.
var implementedOnlyRuleTokens = []string{"spec entries", "implemented behavior"}

// normalizedBullets splits markdown into top-level bullets, lowercased with
// whitespace runs collapsed so a hard wrap inside a phrase does not hide it.
// Bullets are split on "\n- ", so indented continuation lines stay with their
// own bullet.
func normalizedBullets(text string) []string {
	var out []string
	for _, bullet := range strings.Split(text, "\n- ") {
		out = append(out, strings.Join(strings.Fields(strings.ToLower(bullet)), " "))
	}
	return out
}

func containsAllTokens(text string, tokens []string) bool {
	for _, token := range tokens {
		if !strings.Contains(text, token) {
			return false
		}
	}
	return true
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
// byte-exact text, and case-insensitively, so ordinary rewording does not break
// it. This guard covers exactly this one sentence and says nothing about the
// rest of the shared content; see 260728-research-parallel-workflow-guide-divergence.
//
// Every copy is checked in one pass: a simultaneous loss in two of the three
// must not report only the first, or the operator fixes one file and reruns
// into a second red.
func TestWorkflowGuidesKeepImplementationGapException(t *testing.T) {
	for _, path := range workflowGuides() {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("read %s: %v", path, err)
			continue
		}
		var matched []string
		for _, bullet := range normalizedBullets(string(raw)) {
			if containsAllTokens(bullet, implementedOnlyRuleTokens) {
				matched = append(matched, bullet)
			}
		}
		if len(matched) != 1 {
			t.Errorf("%s: want exactly one bullet stating the implemented-behavior-only rule (all of %q), got %d",
				path, implementedOnlyRuleTokens, len(matched))
			continue
		}
		bullet := matched[0]
		if !strings.Contains(bullet, "implementation gap") {
			t.Errorf("%s: the implemented-behavior-only rule does not name the Implementation Gap Callout as its exception: %q", path, bullet)
			continue
		}
		if !strings.Contains(bullet, "exception") {
			t.Errorf("%s: the implemented-behavior-only rule names Implementation Gap but not as an exception: %q", path, bullet)
		}
	}
}
