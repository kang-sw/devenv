package mcp

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsconfig"
)

// The prompts orchestrate native reviewers. These tests check the delivered
// contract, not a model's ability to judge a particular ticket pair.
func TestBatchPromotionRenderedContract(t *testing.T) {
	for _, namespace := range []string{"ws", "wsflow"} {
		t.Run(namespace, func(t *testing.T) {
			useLeadProfile(t)
			t.Setenv(envNamespace, namespace)
			t.Setenv(envNoAgent, "0")
			pkg := "agents-plugin"
			if namespace == "wsflow" {
				t.Setenv(envNoAgent, "1")
				pkg = "agents-plugin-wsflow"
			}
			root := filepath.Join("..", "..", "..", pkg, "rsrc")
			t.Setenv("WS_RSRC_ROOT", root)
			t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
			t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))
			s := newTestServerWithHarness(t, "codex")
			lead := toolText(t, callToolOnce(t, s, 1, "playbook.read", map[string]any{"name": "lead-ticket"}))
			design, _, err := renderPlaybookBody(s, root, "ticket-reviewer-design", nil, wsconfig.Options{}, "", "", "", nil)
			if err != nil {
				t.Fatal(err)
			}
			for name, tc := range map[string]struct {
				body  string
				wants []string
			}{
				"single ticket":     {lead, []string{"A single-ticket promotion uses the existing single-ticket reviewer path"}},
				"complete batch":    {lead, []string{"before any reviewer reads the batch", "Render `ticket-reviewer-design` once for the batch", "Completeness review remains per ticket", "only after all reviews settle"}},
				"coherence mapping": {lead, []string{"validate exact stem coverage", "subset of eligible stems before stamping", "only the coherence issues naming it", "Cross-ticket [<affected_stems>]:", "whole batch at its original statuses"}},
				"skip posture":      {lead, []string{"Resolve recommendations before dispatch", "they receive no design verdict or stamp", "If every design stage is skipped, omit design dispatch", "preserve skipped stages by excluding their verdicts and stamps"}},
				"delta dispatch":    {lead, []string{"`changed_stems`", "`previously_passed_stems`", "prior report alongside the current paths", "Include changed context-only members", "Reject a reversal lacking that evidence", "carry still-unresolved findings forward"}},
				"batch findings":    {design, []string{"contradictions, duplicated scope, dependency mistakes, and overlapping implementation surfaces", "one row per `review_eligible_stems` entry", "`coherence`", "nonempty `affected_stems` naming only eligible members", "Completeness findings belong to the per-ticket completeness reviewer"}},
				"pass preservation": {design, []string{"previously passed tickets are accepted baseline context, not fresh targets", "Carry unresolved prior findings forward and check fixes", "changed ticket or relation", "citing a concrete premise", "explaining how the change invalidates it", "`follow_up_findings` and do not alter the current verdict", "This boundary also applies to comparisons with ready inventory and parent epics"}},
			} {
				t.Run(name, func(t *testing.T) {
					body := strings.Join(strings.Fields(tc.body), " ")
					for _, want := range tc.wants {
						if !strings.Contains(body, want) {
							t.Errorf("rendered contract missing %q", want)
						}
					}
				})
			}
		})
	}
}
