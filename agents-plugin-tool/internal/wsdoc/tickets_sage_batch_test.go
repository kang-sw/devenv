package wsdoc

import (
	"os"
	"strings"
	"testing"
)

// Batch orchestration maps model output to existing per-ticket stamps. Exercise
// that boundary with already-mapped outcomes; no batch API or policy is added.
func TestSageMappedBatchOutcomes(t *testing.T) {
	for _, tc := range []struct {
		name    string
		blocked []string
		skipped bool
	}{
		{"clean batch", nil, false},
		{"one ticket failure", []string{"260101-feat-alpha"}, false},
		{"cross conflict affects two", []string{"260101-feat-alpha", "260101-feat-beta"}, false},
		{"skipped member is context only", []string{"260101-feat-alpha"}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			for _, stem := range []string{"260101-feat-alpha", "260101-feat-beta", "260101-feat-gamma"} {
				fields := map[string]string{"sage-review-design": "required", "sage-review-completeness": "required"}
				skipped := tc.skipped && stem == "260101-feat-beta"
				if skipped {
					fields["sage-review-design"] = "skipped"
				}
				path := writeSageTicket(t, root, stem, fields)
				gate, err := SageGate(root, SageGateOptions{TicketStem: stem, Landing: "ready"}, "required")
				if err != nil || gate.Action != "run" {
					t.Fatalf("gate on todo body: %+v, %v", gate, err)
				}
				stage := "combined"
				verdicts := []SageVerdict{{Reviewer: "design", Verdict: "pass"}, {Reviewer: "completeness", Verdict: "pass"}}
				blocked := false
				for _, affected := range tc.blocked {
					blocked = blocked || affected == stem
				}
				if blocked {
					verdicts[0] = SageVerdict{Reviewer: "design", Verdict: "block", Issues: []SageIssue{{Title: "Cross-ticket [" + strings.Join(tc.blocked, ", ") + "]: incompatible ownership", Severity: "critical", Resolution: "autonomous"}}}
				}
				if skipped {
					stage, verdicts = "completeness", verdicts[1:]
					if strings.Join(gate.Reviewers, ",") != "completeness" {
						t.Fatalf("skipped design dispatched: %+v", gate)
					}
				}
				result, err := SageRecord(root, SageRecordOptions{TicketStem: stem, Stage: stage, Verdicts: verdicts, Today: "2026-01-01"})
				if err != nil {
					t.Fatal(err)
				}
				fm := frontmatter(path)
				if skipped {
					_, hasDesignStamp := fm["sage-review-design-reviewed"]
					if fm["sage-review-design"] != "skipped" || hasDesignStamp {
						t.Fatalf("context-only design stamp mutated: %+v", fm)
					}
				} else if blocked {
					if fm["sage-review-design"] != "blocked" || !strings.Contains(result.BlockedSection, "Cross-ticket [") || !strings.Contains(result.BlockedSection, "### Completeness Reviewer — pass") {
						t.Fatalf("mapped conflict diagnostics lost: %+v", result)
					}
				} else {
					digest, err := sageReviewCurrentBodyDigest(path)
					if err != nil || fm["sage-review-design"] != "completed" || fm["sage-review-design-reviewed"] != digest {
						t.Fatalf("unaffected pass not stamped to reviewed body: %+v, %v", fm, err)
					}
				}
				if _, err := os.Stat(path); err != nil {
					t.Fatalf("review moved a batch member before landing: %v", err)
				}
			}
		})
	}
}
