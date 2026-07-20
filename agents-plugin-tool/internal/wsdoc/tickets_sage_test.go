package wsdoc

import (
	"path/filepath"
	"strings"
	"testing"
)

// Literal Blocked Section Templates captured verbatim from
// agents-plugin/rsrc/lead-write-ticket/lead-write-ticket.md (the three "Blocked
// Section Template" fences) BEFORE they were deleted from the playbook. These
// are the byte-identical regression fixtures for renderBlockedSection: the tool
// must reproduce them exactly when rendered with the templates' placeholder
// inputs. The "—" is U+2014 (em dash), matching the source.
const blockedTemplateDesignOnly = "## Blocked (YYYY-MM-DD)\n" +
	"\n" +
	"### Design Reviewer — <verdict>\n" +
	"\n" +
	"| # | Title | Severity | Resolution |\n" +
	"|---|-------|----------|------------|\n" +
	"| 1 | <title> | <severity> | <resolution> |"

const blockedTemplateCompletenessOnly = "## Blocked (YYYY-MM-DD)\n" +
	"\n" +
	"### Completeness Reviewer — <verdict>\n" +
	"\n" +
	"| # | Title | Severity |\n" +
	"|---|-------|----------|\n" +
	"| 1 | <title> | <severity> |"

const blockedTemplateCombined = "## Blocked (YYYY-MM-DD)\n" +
	"\n" +
	"### Design Reviewer — <verdict>\n" +
	"\n" +
	"| # | Title | Severity | Resolution |\n" +
	"|---|-------|----------|------------|\n" +
	"| 1 | <title> | <severity> | <resolution> |\n" +
	"\n" +
	"### Completeness Reviewer — <verdict>\n" +
	"\n" +
	"| # | Title | Severity |\n" +
	"|---|-------|----------|\n" +
	"| 1 | <title> | <severity> |"

func placeholderIssue() []SageIssue {
	return []SageIssue{{Title: "<title>", Severity: "<severity>", Resolution: "<resolution>"}}
}

func TestRenderBlockedSectionByteIdentical(t *testing.T) {
	gotDesign := renderBlockedSection("YYYY-MM-DD", []blockedReviewerSection{
		{Heading: "Design Reviewer", Verdict: "<verdict>", Issues: placeholderIssue(), WithResolution: true},
	})
	if gotDesign != blockedTemplateDesignOnly {
		t.Fatalf("design-only render not byte-identical:\ngot:\n%q\nwant:\n%q", gotDesign, blockedTemplateDesignOnly)
	}

	gotCompleteness := renderBlockedSection("YYYY-MM-DD", []blockedReviewerSection{
		{Heading: "Completeness Reviewer", Verdict: "<verdict>", Issues: placeholderIssue(), WithResolution: false},
	})
	if gotCompleteness != blockedTemplateCompletenessOnly {
		t.Fatalf("completeness-only render not byte-identical:\ngot:\n%q\nwant:\n%q", gotCompleteness, blockedTemplateCompletenessOnly)
	}

	gotCombined := renderBlockedSection("YYYY-MM-DD", []blockedReviewerSection{
		{Heading: "Design Reviewer", Verdict: "<verdict>", Issues: placeholderIssue(), WithResolution: true},
		{Heading: "Completeness Reviewer", Verdict: "<verdict>", Issues: placeholderIssue(), WithResolution: false},
	})
	if gotCombined != blockedTemplateCombined {
		t.Fatalf("combined render not byte-identical:\ngot:\n%q\nwant:\n%q", gotCombined, blockedTemplateCombined)
	}
}

// writeSageTicket writes a ticket fixture into todo/ with the given sage
// frontmatter fields and returns its absolute path.
func writeSageTicket(t *testing.T, root, stem string, fields map[string]string) string {
	t.Helper()
	var b strings.Builder
	b.WriteString("---\ntitle: Sample\n")
	for k, v := range fields {
		b.WriteString(k + ": " + v + "\n")
	}
	b.WriteString("---\n\n# Sample\n\nBody text.\n")
	rel := filepath.Join("ai-docs", "tickets", "todo", stem+".md")
	mustWrite(t, root, rel, b.String())
	return filepath.Join(root, rel)
}

func TestSageGateIdeaSkips(t *testing.T) {
	root := t.TempDir()
	writeSageTicket(t, root, "260101-feat-sample", nil)
	res, err := SageGate(root, SageGateOptions{TicketStem: "260101-feat-sample", Landing: "idea"}, "auto")
	if err != nil {
		t.Fatalf("SageGate: %v", err)
	}
	if res.Action != "skip" {
		t.Fatalf("idea landing action = %q, want skip", res.Action)
	}
}

func TestSageGateTodoPostures(t *testing.T) {
	stem := "260101-feat-sample"
	cases := []struct {
		name       string
		field      string
		config     string
		answer     string
		wantAction string
		wantMode   string
		wantCommit string
	}{
		{name: "skipped", field: "skipped", wantAction: "skip"},
		{name: "completed", field: "completed", wantAction: "skip"},
		{name: "blocked", field: "blocked", wantAction: "stop_blocked"},
		{name: "recommended-ask", field: "recommended", wantAction: "ask"},
		{name: "recommended-accept", field: "recommended", answer: "yes", wantAction: "run", wantMode: "standalone"},
		{name: "recommended-decline", field: "recommended", answer: "no", wantAction: "skip", wantCommit: "chore(sage): skip design review"},
		{name: "required", field: "required", wantAction: "run", wantMode: "standalone"},
		{name: "missing-config-off", field: "", config: "off", wantAction: "skip"},
		{name: "missing-config-ask", field: "", config: "ask", wantAction: "ask"},
		{name: "missing-config-auto", field: "", config: "auto", wantAction: "run", wantMode: "standalone"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			fields := map[string]string{}
			if tc.field != "" {
				fields["sage-review-design"] = tc.field
			}
			writeSageTicket(t, root, stem, fields)
			res, err := SageGate(root, SageGateOptions{TicketStem: stem, Landing: "todo", Answer: tc.answer}, tc.config)
			if err != nil {
				t.Fatalf("SageGate: %v", err)
			}
			if res.Action != tc.wantAction {
				t.Fatalf("action = %q, want %q", res.Action, tc.wantAction)
			}
			if tc.wantMode != "" && res.Mode != tc.wantMode {
				t.Fatalf("mode = %q, want %q", res.Mode, tc.wantMode)
			}
			if tc.wantAction == "run" && (len(res.Reviewers) != 1 || res.Reviewers[0] != "design") {
				t.Fatalf("reviewers = %v, want [design]", res.Reviewers)
			}
			if tc.wantCommit != "" && res.CommitTitle != tc.wantCommit {
				t.Fatalf("commit title = %q, want %q", res.CommitTitle, tc.wantCommit)
			}
		})
	}
}

func TestSageGateCategoryMatrix(t *testing.T) {
	for _, stem := range []string{"260101-research-topic", "260101-workset-board"} {
		root := t.TempDir()
		writeSageTicket(t, root, stem, map[string]string{"sage-review-design": "required"})
		for _, landing := range []string{"todo", "ready"} {
			res, err := SageGate(root, SageGateOptions{TicketStem: stem, Landing: landing}, "auto")
			if err != nil {
				t.Fatalf("SageGate(%s,%s): %v", stem, landing, err)
			}
			if res.Action != "skip" {
				t.Fatalf("%s %s action = %q, want skip (exempt)", stem, landing, res.Action)
			}
		}
	}

	// epic: design-only. ready with non-terminal design runs design standalone.
	root := t.TempDir()
	writeSageTicket(t, root, "260101-epic-board", map[string]string{"sage-review-design": "required"})
	res, err := SageGate(root, SageGateOptions{TicketStem: "260101-epic-board", Landing: "ready"}, "auto")
	if err != nil {
		t.Fatalf("SageGate epic: %v", err)
	}
	if res.Action != "run" || res.Mode != "standalone" || len(res.Reviewers) != 1 || res.Reviewers[0] != "design" {
		t.Fatalf("epic ready = %+v, want run/standalone/[design]", res)
	}

	// epic: terminal design skips (no completeness stage ever).
	root2 := t.TempDir()
	writeSageTicket(t, root2, "260101-epic-done", map[string]string{"sage-review-design": "completed"})
	res2, err := SageGate(root2, SageGateOptions{TicketStem: "260101-epic-done", Landing: "ready"}, "auto")
	if err != nil {
		t.Fatalf("SageGate epic done: %v", err)
	}
	if res2.Action != "skip" {
		t.Fatalf("epic terminal design ready action = %q, want skip", res2.Action)
	}
}

func TestSageGateReadyBothStages(t *testing.T) {
	// Design terminal -> completeness stands alone.
	root := t.TempDir()
	writeSageTicket(t, root, "260101-feat-a", map[string]string{
		"sage-review-design":       "completed",
		"sage-review-completeness": "required",
	})
	res, err := SageGate(root, SageGateOptions{TicketStem: "260101-feat-a", Landing: "ready"}, "auto")
	if err != nil {
		t.Fatalf("SageGate: %v", err)
	}
	if res.Action != "run" || res.Mode != "standalone" || len(res.Reviewers) != 1 || res.Reviewers[0] != "completeness" {
		t.Fatalf("design-terminal ready = %+v, want run/standalone/[completeness]", res)
	}

	// Design non-terminal (both required) -> combined mode, both reviewers.
	root2 := t.TempDir()
	writeSageTicket(t, root2, "260101-feat-b", map[string]string{
		"sage-review-design":       "required",
		"sage-review-completeness": "required",
	})
	res2, err := SageGate(root2, SageGateOptions{TicketStem: "260101-feat-b", Landing: "ready"}, "auto")
	if err != nil {
		t.Fatalf("SageGate combined: %v", err)
	}
	if res2.Action != "run" || res2.Mode != "combined" {
		t.Fatalf("combined action/mode = %q/%q, want run/combined", res2.Action, res2.Mode)
	}
	if len(res2.Reviewers) != 2 || res2.Reviewers[0] != "design" || res2.Reviewers[1] != "completeness" {
		t.Fatalf("combined reviewers = %v, want [design completeness]", res2.Reviewers)
	}

	// Design blocked -> stop_blocked (never-skippable design invariant).
	root3 := t.TempDir()
	writeSageTicket(t, root3, "260101-feat-c", map[string]string{
		"sage-review-design":       "blocked",
		"sage-review-completeness": "required",
	})
	res3, err := SageGate(root3, SageGateOptions{TicketStem: "260101-feat-c", Landing: "ready"}, "auto")
	if err != nil {
		t.Fatalf("SageGate blocked: %v", err)
	}
	if res3.Action != "stop_blocked" {
		t.Fatalf("design-blocked ready action = %q, want stop_blocked", res3.Action)
	}
}

func TestSageGateLegacyMigration(t *testing.T) {
	// Legacy sage-review: completed maps to both fields completed -> ready skips.
	root := t.TempDir()
	writeSageTicket(t, root, "260101-feat-legacy", map[string]string{"sage-review": "completed"})
	res, err := SageGate(root, SageGateOptions{TicketStem: "260101-feat-legacy", Landing: "ready"}, "auto")
	if err != nil {
		t.Fatalf("SageGate legacy: %v", err)
	}
	if res.Action != "skip" {
		t.Fatalf("legacy completed ready action = %q, want skip", res.Action)
	}

	// Legacy blocked -> stop_blocked at todo design gate.
	root2 := t.TempDir()
	writeSageTicket(t, root2, "260101-feat-legb", map[string]string{"sage-review": "blocked"})
	res2, err := SageGate(root2, SageGateOptions{TicketStem: "260101-feat-legb", Landing: "todo"}, "auto")
	if err != nil {
		t.Fatalf("SageGate legacy blocked: %v", err)
	}
	if res2.Action != "stop_blocked" {
		t.Fatalf("legacy blocked todo action = %q, want stop_blocked", res2.Action)
	}
}

func TestSageGateDeclinePersistsSkipped(t *testing.T) {
	root := t.TempDir()
	path := writeSageTicket(t, root, "260101-feat-decl", map[string]string{"sage-review-design": "recommended"})
	res, err := SageGate(root, SageGateOptions{TicketStem: "260101-feat-decl", Landing: "todo", Answer: "no"}, "ask")
	if err != nil {
		t.Fatalf("SageGate decline: %v", err)
	}
	if res.Action != "skip" || res.CommitTitle != "chore(sage): skip design review" {
		t.Fatalf("decline = %+v, want skip + skip-design commit", res)
	}
	body := readFileString(t, path)
	if !strings.Contains(body, "sage-review-design: skipped") {
		t.Fatalf("decline did not persist skipped:\n%s", body)
	}
}

func TestSageGateMissingPersistsResolvedPosture(t *testing.T) {
	root := t.TempDir()
	path := writeSageTicket(t, root, "260101-feat-miss", nil)
	if _, err := SageGate(root, SageGateOptions{TicketStem: "260101-feat-miss", Landing: "todo"}, "auto"); err != nil {
		t.Fatalf("SageGate: %v", err)
	}
	body := readFileString(t, path)
	if !strings.Contains(body, "sage-review-design: required") {
		t.Fatalf("config-fallback did not persist required posture:\n%s", body)
	}
}

func TestSageRecordDesignStandalone(t *testing.T) {
	// block: writes blocked, appends Blocked section, block commit title.
	root := t.TempDir()
	path := writeSageTicket(t, root, "260101-feat-d1", map[string]string{"sage-review-design": "required"})
	res, err := SageRecord(root, SageRecordOptions{
		TicketStem: "260101-feat-d1",
		Stage:      "design",
		Today:      "2026-07-20",
		Verdicts:   []SageVerdict{{Reviewer: "design", Verdict: "block", Issues: []SageIssue{{Title: "T", Severity: "high", Resolution: "missing"}}}},
	})
	if err != nil {
		t.Fatalf("SageRecord block: %v", err)
	}
	if res.Posture["sage-review-design"] != "blocked" {
		t.Fatalf("posture = %v, want blocked", res.Posture)
	}
	if res.CommitTitle != "docs(sage): block ticket on design review" {
		t.Fatalf("commit title = %q", res.CommitTitle)
	}
	body := readFileString(t, path)
	wantSection := "## Blocked (2026-07-20)\n\n### Design Reviewer — block\n\n| # | Title | Severity | Resolution |\n|---|-------|----------|------------|\n| 1 | T | high | missing |"
	if !strings.Contains(body, wantSection) {
		t.Fatalf("blocked section not found verbatim in body:\n%s", body)
	}
	if !strings.Contains(body, "sage-review-design: blocked") {
		t.Fatalf("frontmatter not updated to blocked:\n%s", body)
	}

	// pass: writes completed, no Blocked section.
	root2 := t.TempDir()
	path2 := writeSageTicket(t, root2, "260101-feat-d2", map[string]string{"sage-review-design": "required"})
	res2, err := SageRecord(root2, SageRecordOptions{
		TicketStem: "260101-feat-d2",
		Stage:      "design",
		Today:      "2026-07-20",
		Verdicts:   []SageVerdict{{Reviewer: "design", Verdict: "pass"}},
	})
	if err != nil {
		t.Fatalf("SageRecord pass: %v", err)
	}
	if res2.CommitTitle != "docs(sage): mark design review completed" || res2.BlockedSection != "" {
		t.Fatalf("pass result = %+v", res2)
	}
	if strings.Contains(readFileString(t, path2), "## Blocked") {
		t.Fatalf("pass should not append Blocked section")
	}
}

func TestSageRecordCompletenessStandalone(t *testing.T) {
	root := t.TempDir()
	path := writeSageTicket(t, root, "260101-feat-c1", map[string]string{"sage-review-completeness": "required"})
	res, err := SageRecord(root, SageRecordOptions{
		TicketStem: "260101-feat-c1",
		Stage:      "completeness",
		Today:      "2026-07-20",
		Verdicts:   []SageVerdict{{Reviewer: "completeness", Verdict: "block", Issues: []SageIssue{{Title: "T", Severity: "med"}}}},
	})
	if err != nil {
		t.Fatalf("SageRecord: %v", err)
	}
	if res.CommitTitle != "docs(sage): block ticket on completeness review" {
		t.Fatalf("commit title = %q", res.CommitTitle)
	}
	// Completeness table omits the Resolution column.
	wantSection := "### Completeness Reviewer — block\n\n| # | Title | Severity |\n|---|-------|----------|\n| 1 | T | med |"
	if !strings.Contains(readFileString(t, path), wantSection) {
		t.Fatalf("completeness blocked section not verbatim:\n%s", readFileString(t, path))
	}
}

func TestSageRecordCombinedAggregation(t *testing.T) {
	// design block dominates -> block.
	root := t.TempDir()
	writeSageTicket(t, root, "260101-feat-agg1", map[string]string{"sage-review-design": "required", "sage-review-completeness": "required"})
	res, err := SageRecord(root, SageRecordOptions{
		TicketStem: "260101-feat-agg1",
		Stage:      "combined",
		Today:      "2026-07-20",
		Verdicts: []SageVerdict{
			{Reviewer: "design", Verdict: "block", Issues: []SageIssue{{Title: "D", Severity: "high", Resolution: "missing"}}},
			{Reviewer: "completeness", Verdict: "pass"},
		},
	})
	if err != nil {
		t.Fatalf("SageRecord: %v", err)
	}
	if res.Verdict != "block" || res.CommitTitle != "docs(sage): block ticket on sage review" {
		t.Fatalf("design-block aggregate = %+v", res)
	}
	if res.Posture["sage-review-design"] != "blocked" || res.Posture["sage-review-completeness"] != "blocked" {
		t.Fatalf("both fields not blocked: %v", res.Posture)
	}

	// completeness block dominates -> block.
	root2 := t.TempDir()
	writeSageTicket(t, root2, "260101-feat-agg2", map[string]string{"sage-review-design": "required", "sage-review-completeness": "required"})
	res2, err := SageRecord(root2, SageRecordOptions{
		TicketStem: "260101-feat-agg2", Stage: "combined", Today: "2026-07-20",
		Verdicts: []SageVerdict{{Reviewer: "design", Verdict: "pass"}, {Reviewer: "completeness", Verdict: "block", Issues: []SageIssue{{Title: "C", Severity: "high"}}}},
	})
	if err != nil {
		t.Fatalf("SageRecord: %v", err)
	}
	if res2.Verdict != "block" {
		t.Fatalf("completeness-block aggregate = %q, want block", res2.Verdict)
	}

	// concern + resolution:missing -> concern, but writes completed.
	root3 := t.TempDir()
	writeSageTicket(t, root3, "260101-feat-agg3", map[string]string{"sage-review-design": "required", "sage-review-completeness": "required"})
	res3, err := SageRecord(root3, SageRecordOptions{
		TicketStem: "260101-feat-agg3", Stage: "combined", Today: "2026-07-20",
		Verdicts: []SageVerdict{
			{Reviewer: "design", Verdict: "concern", Issues: []SageIssue{{Title: "D", Severity: "low", Resolution: "missing"}}},
			{Reviewer: "completeness", Verdict: "pass"},
		},
	})
	if err != nil {
		t.Fatalf("SageRecord: %v", err)
	}
	if res3.Verdict != "concern" {
		t.Fatalf("missing-resolution aggregate = %q, want concern", res3.Verdict)
	}
	if res3.Posture["sage-review-design"] != "completed" || res3.CommitTitle != "docs(sage): mark sage review completed" {
		t.Fatalf("concern should still write completed: %+v", res3)
	}

	// all pass -> pass, completed.
	root4 := t.TempDir()
	writeSageTicket(t, root4, "260101-feat-agg4", map[string]string{"sage-review-design": "required", "sage-review-completeness": "required"})
	res4, err := SageRecord(root4, SageRecordOptions{
		TicketStem: "260101-feat-agg4", Stage: "combined", Today: "2026-07-20",
		Verdicts: []SageVerdict{
			{Reviewer: "design", Verdict: "concern", Issues: []SageIssue{{Title: "D", Severity: "low", Resolution: "addressed"}}},
			{Reviewer: "completeness", Verdict: "pass"},
		},
	})
	if err != nil {
		t.Fatalf("SageRecord: %v", err)
	}
	if res4.Verdict != "pass" || res4.CommitTitle != "docs(sage): mark sage review completed" {
		t.Fatalf("all-pass aggregate = %+v", res4)
	}
}

func TestSageRecordBlockedSectionReplacedOnSecondCycle(t *testing.T) {
	root := t.TempDir()
	path := writeSageTicket(t, root, "260101-feat-cyc", map[string]string{"sage-review-design": "required"})
	for _, sev := range []string{"first", "second"} {
		if _, err := SageRecord(root, SageRecordOptions{
			TicketStem: "260101-feat-cyc", Stage: "design", Today: "2026-07-20",
			Verdicts: []SageVerdict{{Reviewer: "design", Verdict: "block", Issues: []SageIssue{{Title: sev, Severity: "high", Resolution: "missing"}}}},
		}); err != nil {
			t.Fatalf("SageRecord %s: %v", sev, err)
		}
	}
	body := readFileString(t, path)
	if strings.Count(body, "## Blocked (") != 1 {
		t.Fatalf("expected exactly one Blocked section after two cycles:\n%s", body)
	}
	if strings.Contains(body, "first") || !strings.Contains(body, "second") {
		t.Fatalf("second cycle should replace the first Blocked section:\n%s", body)
	}
}

func TestSageGateInvalidInputs(t *testing.T) {
	root := t.TempDir()
	writeSageTicket(t, root, "260101-feat-x", nil)
	if _, err := SageGate(root, SageGateOptions{TicketStem: "not-a-stem", Landing: "todo"}, "auto"); err == nil {
		t.Error("expected error for bad stem")
	}
	if _, err := SageGate(root, SageGateOptions{TicketStem: "260101-feat-x", Landing: "bogus"}, "auto"); err == nil {
		t.Error("expected error for bad landing")
	}
	if _, err := SageGate(root, SageGateOptions{TicketStem: "260101-feat-x", Landing: "todo", Answer: "maybe"}, "auto"); err == nil {
		t.Error("expected error for bad answer")
	}
	if _, err := SageRecord(root, SageRecordOptions{TicketStem: "260101-feat-x", Stage: "bogus"}); err == nil {
		t.Error("expected error for bad stage")
	}
}
