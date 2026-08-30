package wsdoc

import (
	"os"
	"os/exec"
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
	}{
		{name: "skipped", field: "skipped", wantAction: "skip"},
		{name: "completed", field: "completed", wantAction: "skip"},
		{name: "blocked", field: "blocked", wantAction: "stop_blocked"},
		{name: "recommended-ask", field: "recommended", wantAction: "ask"},
		{name: "recommended-accept", field: "recommended", answer: "yes", wantAction: "run", wantMode: "standalone"},
		{name: "recommended-decline", field: "recommended", answer: "no", wantAction: "skip"},
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
		})
	}
}

// TestSageGateRequiredAndRecommendedCarryNonWaivableAdvisory covers
// verification item 4: the non-waivable statement + review-scope line must
// ride the ordinary path an agent actually reaches — posture `required`'s
// `run` result, and posture `recommended`'s `ask` prompt — not only an
// answer=="no" decline path (which `required` never reaches, since
// `required` never asks). Also confirms the advisory reaches the combined
// (both-stages) mode, not just the standalone path.
func TestSageGateRequiredAndRecommendedCarryNonWaivableAdvisory(t *testing.T) {
	stem := "260101-feat-sample"

	t.Run("required-run-standalone", func(t *testing.T) {
		root := t.TempDir()
		writeSageTicket(t, root, stem, map[string]string{"sage-review-design": "required"})
		res, err := SageGate(root, SageGateOptions{TicketStem: stem, Landing: "todo"}, "auto")
		if err != nil {
			t.Fatalf("SageGate: %v", err)
		}
		if res.Action != "run" {
			t.Fatalf("action = %q, want run", res.Action)
		}
		if res.Advisory == "" {
			t.Fatalf("Advisory is empty, want the non-waivable statement on the ordinary required->run path")
		}
		if !strings.Contains(res.Advisory, "not waivable") {
			t.Fatalf("Advisory = %q, want the non-waivable statement", res.Advisory)
		}
		if !strings.Contains(res.Advisory, "coherence") || !strings.Contains(res.Advisory, "structure, fields, and clarity") {
			t.Fatalf("Advisory = %q, want the design-vs-completeness review-scope line", res.Advisory)
		}
	})

	t.Run("recommended-ask-standalone", func(t *testing.T) {
		root := t.TempDir()
		writeSageTicket(t, root, stem, map[string]string{"sage-review-design": "recommended"})
		res, err := SageGate(root, SageGateOptions{TicketStem: stem, Landing: "todo"}, "ask")
		if err != nil {
			t.Fatalf("SageGate: %v", err)
		}
		if res.Action != "ask" {
			t.Fatalf("action = %q, want ask", res.Action)
		}
		if res.Advisory == "" {
			t.Fatalf("Advisory is empty, want it kept on the recommended ask prompt too (per ticket decision)")
		}
	})

	// C8: resolveStage's `recommended` + answer=="yes" -> `run` branch must
	// carry the advisory too, symmetric with sageGateCombined's equivalent
	// accepted-recommended-design branch (which already attached it) — a
	// "run" result is a run result regardless of which posture produced it.
	t.Run("recommended-accept-run-standalone", func(t *testing.T) {
		root := t.TempDir()
		writeSageTicket(t, root, stem, map[string]string{"sage-review-design": "recommended"})
		res, err := SageGate(root, SageGateOptions{TicketStem: stem, Landing: "todo", Answer: "yes"}, "ask")
		if err != nil {
			t.Fatalf("SageGate: %v", err)
		}
		if res.Action != "run" {
			t.Fatalf("action = %q, want run", res.Action)
		}
		if res.Advisory == "" {
			t.Fatalf("Advisory is empty, want it carried on the accepted-recommended run result too (symmetric with sageGateCombined)")
		}
	})

	t.Run("required-run-combined", func(t *testing.T) {
		root := t.TempDir()
		writeSageTicket(t, root, stem, map[string]string{
			"sage-review-design":       "required",
			"sage-review-completeness": "required",
		})
		res, err := SageGate(root, SageGateOptions{TicketStem: stem, Landing: "ready"}, "auto")
		if err != nil {
			t.Fatalf("SageGate: %v", err)
		}
		if res.Action != "run" || res.Mode != "combined" {
			t.Fatalf("action/mode = %q/%q, want run/combined", res.Action, res.Mode)
		}
		if res.Advisory == "" {
			t.Fatalf("Advisory is empty, want it carried into combined mode too, not just the standalone path")
		}
	})
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
	if res.Action != "skip" {
		t.Fatalf("decline = %+v, want skip", res)
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

func initSageFreshnessRepo(t *testing.T, root string) {
	t.Helper()
	runGit(t, root, "init", "-q")
	runGit(t, root, "config", "user.email", "test@example.com")
	runGit(t, root, "config", "user.name", "Test User")
}

func commitSageFreshnessRepo(t *testing.T, root, message string) string {
	t.Helper()
	runGit(t, root, "add", "-A")
	runGit(t, root, "commit", "-q", "-m", message)
	return strings.TrimSpace(string(runGitOutputSage(t, root, "rev-parse", "HEAD")))
}

func runGitOutputSage(t *testing.T, root string, args ...string) []byte {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, string(out))
	}
	return out
}

func TestSageGateWarnsWhenCompletedReviewIsStale(t *testing.T) {
	root := t.TempDir()
	stem := "260101-feat-stale"
	path := writeSageTicket(t, root, stem, map[string]string{
		"sage-review-design":       "completed",
		"sage-review-completeness": "completed",
	})
	initSageFreshnessRepo(t, root)
	baseline := commitSageFreshnessRepo(t, root, "stamp review")
	if err := os.WriteFile(path, []byte("---\ntitle: Sample\nsage-review-design: completed\nsage-review-completeness: completed\n---\n\n# Sample\n\nBody text changed.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	commitSageFreshnessRepo(t, root, "edit after review")

	res, err := SageGate(root, SageGateOptions{TicketStem: stem, Landing: "ready"}, "auto")
	if err != nil {
		t.Fatalf("SageGate: %v", err)
	}
	if res.Action != "check_review_required" {
		t.Fatalf("action = %q, want check_review_required", res.Action)
	}
	if strings.Join(res.FreshnessStages, ",") != "design,completeness" {
		t.Fatalf("freshness stages = %v, want design+completeness", res.FreshnessStages)
	}
	if res.ReviewBaseline != shortCommit(baseline) {
		t.Fatalf("baseline = %q, want %q", res.ReviewBaseline, shortCommit(baseline))
	}
	if !strings.Contains(res.ReviewInstruction, "Inspect the ticket diff") {
		t.Fatalf("instruction = %q, want diff inspection guidance", res.ReviewInstruction)
	}
}

func TestSageGateWarnsOnUncommittedPostStampEdit(t *testing.T) {
	root := t.TempDir()
	stem := "260101-feat-uncommitted"
	path := writeSageTicket(t, root, stem, map[string]string{"sage-review-design": "completed"})
	initSageFreshnessRepo(t, root)
	commitSageFreshnessRepo(t, root, "stamp review")
	if err := os.WriteFile(path, []byte("---\ntitle: Sample\nsage-review-design: completed\n---\n\n# Sample\n\nUncommitted change.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := SageGate(root, SageGateOptions{TicketStem: stem, Landing: "todo"}, "auto")
	if err != nil {
		t.Fatalf("SageGate: %v", err)
	}
	if res.Action != "check_review_required" || strings.Join(res.FreshnessStages, ",") != "design" {
		t.Fatalf("result = %+v, want design freshness check", res)
	}
}

func TestSageGateWarnsOnStagedOnlyPostStampEdit(t *testing.T) {
	root := t.TempDir()
	stem := "260101-feat-staged"
	rel := filepath.Join("ai-docs", "tickets", "todo", stem+".md")
	path := writeSageTicket(t, root, stem, map[string]string{"sage-review-design": "completed"})
	initSageFreshnessRepo(t, root)
	commitSageFreshnessRepo(t, root, "stamp review")
	staged := "---\ntitle: Sample\nsage-review-design: completed\n---\n\n# Sample\n\nStaged change.\n"
	if err := os.WriteFile(path, []byte(staged), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "add", rel)
	runGit(t, root, "restore", "--worktree", rel)

	res, err := SageGate(root, SageGateOptions{TicketStem: stem, Landing: "todo"}, "auto")
	if err != nil {
		t.Fatalf("SageGate: %v", err)
	}
	if res.Action != "check_review_required" || strings.Join(res.FreshnessStages, ",") != "design" {
		t.Fatalf("result = %+v, want staged design freshness check", res)
	}
}

func TestSageGateFreshnessIsStageSpecific(t *testing.T) {
	root := t.TempDir()
	stem := "260101-feat-stage"
	path := writeSageTicket(t, root, stem, map[string]string{
		"sage-review-design":       "completed",
		"sage-review-completeness": "required",
	})
	initSageFreshnessRepo(t, root)
	commitSageFreshnessRepo(t, root, "design completed")
	if err := os.WriteFile(path, []byte("---\ntitle: Sample\nsage-review-design: completed\nsage-review-completeness: required\n---\n\n# Sample\n\nChanged before completeness.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := SageGate(root, SageGateOptions{TicketStem: stem, Landing: "ready"}, "auto")
	if err != nil {
		t.Fatalf("SageGate: %v", err)
	}
	if res.Action != "check_review_required" || strings.Join(res.FreshnessStages, ",") != "design" {
		t.Fatalf("result = %+v, want only design freshness check", res)
	}
}

func TestSageGateFreshnessIgnoresSageOnlyAndStatusOnlyChanges(t *testing.T) {
	t.Run("sage-posture-only", func(t *testing.T) {
		root := t.TempDir()
		stem := "260101-feat-sageonly"
		path := writeSageTicket(t, root, stem, map[string]string{"sage-review-design": "required"})
		initSageFreshnessRepo(t, root)
		commitSageFreshnessRepo(t, root, "ticket before review")
		if err := os.WriteFile(path, []byte("---\ntitle: Sample\nsage-review-design: completed\n---\n\n# Sample\n\nBody text.\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		commitSageFreshnessRepo(t, root, "stamp review")

		res, err := SageGate(root, SageGateOptions{TicketStem: stem, Landing: "todo"}, "auto")
		if err != nil {
			t.Fatalf("SageGate: %v", err)
		}
		if res.Action != "skip" {
			t.Fatalf("result = %+v, want skip", res)
		}
	})

	t.Run("status-move-only", func(t *testing.T) {
		root := t.TempDir()
		stem := "260101-feat-moveonly"
		writeSageTicket(t, root, stem, map[string]string{
			"sage-review-design":       "completed",
			"sage-review-completeness": "completed",
		})
		initSageFreshnessRepo(t, root)
		commitSageFreshnessRepo(t, root, "stamp review")
		oldRel := filepath.Join("ai-docs", "tickets", "todo", stem+".md")
		newRel := filepath.Join("ai-docs", "tickets", "ready", stem+".md")
		if err := os.MkdirAll(filepath.Join(root, filepath.Dir(newRel)), 0o755); err != nil {
			t.Fatal(err)
		}
		runGit(t, root, "mv", oldRel, newRel)
		commitSageFreshnessRepo(t, root, "move to ready")

		res, err := SageGate(root, SageGateOptions{TicketStem: stem, Landing: "ready"}, "auto")
		if err != nil {
			t.Fatalf("SageGate: %v", err)
		}
		if res.Action != "skip" {
			t.Fatalf("result = %+v, want skip for pure status move", res)
		}
	})
}

func TestSageGateFreshnessFollowsStatusMoveThenContentEdit(t *testing.T) {
	root := t.TempDir()
	stem := "260101-feat-moveedit"
	writeSageTicket(t, root, stem, map[string]string{
		"sage-review-design":       "completed",
		"sage-review-completeness": "completed",
	})
	initSageFreshnessRepo(t, root)
	baseline := commitSageFreshnessRepo(t, root, "stamp review")
	oldRel := filepath.Join("ai-docs", "tickets", "todo", stem+".md")
	newRel := filepath.Join("ai-docs", "tickets", "ready", stem+".md")
	if err := os.MkdirAll(filepath.Join(root, filepath.Dir(newRel)), 0o755); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "mv", oldRel, newRel)
	if err := os.WriteFile(filepath.Join(root, newRel), []byte("---\ntitle: Sample\nsage-review-design: completed\nsage-review-completeness: completed\n---\n\n# Sample\n\nBody changed in move.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	commitSageFreshnessRepo(t, root, "move and edit")

	res, err := SageGate(root, SageGateOptions{TicketStem: stem, Landing: "ready"}, "auto")
	if err != nil {
		t.Fatalf("SageGate: %v", err)
	}
	if res.Action != "check_review_required" {
		t.Fatalf("action = %q, want check_review_required", res.Action)
	}
	if strings.Join(res.FreshnessStages, ",") != "design,completeness" {
		t.Fatalf("freshness stages = %v, want design+completeness", res.FreshnessStages)
	}
	if res.ReviewBaseline != shortCommit(baseline) {
		t.Fatalf("baseline = %q, want pre-move baseline %q", res.ReviewBaseline, shortCommit(baseline))
	}
}

// TestSageGateDigestRestampClearsFreshness covers the ticket's verification
// bullet 1: a digest re-stamp (completed -> completed, no pending dip)
// clears freshness immediately (no git walk needed, since a recorded digest
// is now compared directly), and a later body edit without a further
// re-stamp goes stale again.
func TestSageGateDigestRestampClearsFreshness(t *testing.T) {
	root := t.TempDir()
	stem := "260101-feat-digest"
	path := writeSageTicket(t, root, stem, nil)
	initSageFreshnessRepo(t, root)
	commitSageFreshnessRepo(t, root, "initial")

	if _, err := SageRecord(root, SageRecordOptions{
		TicketStem: stem,
		Stage:      "design",
		Today:      "2026-07-29",
		Verdicts:   []SageVerdict{{Reviewer: "design", Verdict: "pass"}},
	}); err != nil {
		t.Fatalf("SageRecord (first stamp): %v", err)
	}
	commitSageFreshnessRepo(t, root, "stamp review")

	stamped := readFileString(t, path)
	if !strings.Contains(stamped, "sage-review-design-reviewed:") {
		t.Fatalf("stamp did not record a digest:\n%s", stamped)
	}

	// Body edit followed by a re-stamp on the new body: completed ->
	// completed directly, no pending dip, new digest recorded.
	edited := strings.Replace(stamped, "Body text.", "Body text changed for restamp.", 1)
	if err := os.WriteFile(path, []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}
	commitSageFreshnessRepo(t, root, "edit before restamp")
	if _, err := SageRecord(root, SageRecordOptions{
		TicketStem: stem,
		Stage:      "design",
		Today:      "2026-07-30",
		Verdicts:   []SageVerdict{{Reviewer: "design", Verdict: "pass"}},
	}); err != nil {
		t.Fatalf("SageRecord (re-stamp): %v", err)
	}
	commitSageFreshnessRepo(t, root, "re-stamp review")

	res, err := SageGate(root, SageGateOptions{TicketStem: stem, Landing: "todo"}, "auto")
	if err != nil {
		t.Fatalf("SageGate: %v", err)
	}
	if res.Action == "check_review_required" {
		t.Fatalf("result = %+v, want fresh immediately after digest re-stamp", res)
	}

	// A further edit without re-stamping must go stale again.
	restamped := readFileString(t, path)
	final := strings.Replace(restamped, "restamp.", "restamp, then edited again.", 1)
	if err := os.WriteFile(path, []byte(final), 0o644); err != nil {
		t.Fatal(err)
	}
	commitSageFreshnessRepo(t, root, "edit after restamp")

	res, err = SageGate(root, SageGateOptions{TicketStem: stem, Landing: "todo"}, "auto")
	if err != nil {
		t.Fatalf("SageGate: %v", err)
	}
	if res.Action != "check_review_required" || strings.Join(res.FreshnessStages, ",") != "design" {
		t.Fatalf("result = %+v, want stale design freshness check after post-restamp edit", res)
	}
	if res.ReviewBaseline != "the last recorded digest" {
		t.Fatalf("baseline = %q, want the digest sentinel", res.ReviewBaseline)
	}
	if !strings.Contains(res.ReviewInstruction, "Inspect the ticket diff") {
		t.Fatalf("instruction = %q, want diff inspection guidance", res.ReviewInstruction)
	}
}

// TestSageGateLegacyFallbackFollowsLatestTransition covers the ticket's
// verification bullet 2: a legacy ticket (no recorded digest) whose commit
// history contains a reset -> re-stamp (two completed-transitions) reads
// fresh via the corrected latest-transition fallback, since the current body
// matches the second (latest) stamp rather than the first. A later edit
// after that latest stamp reads stale.
func TestSageGateLegacyFallbackFollowsLatestTransition(t *testing.T) {
	root := t.TempDir()
	stem := "260101-feat-legacy-latest"
	rel := filepath.Join("ai-docs", "tickets", "todo", stem+".md")

	// c1: initial non-completed state.
	mustWrite(t, root, rel, "---\ntitle: Sample\nsage-review-design: required\n---\n\n# Sample\n\nBody v1.\n")
	initSageFreshnessRepo(t, root)
	commitSageFreshnessRepo(t, root, "initial")

	// c2: first completed stamp (transition A), on body v1.
	mustWrite(t, root, rel, "---\ntitle: Sample\nsage-review-design: completed\n---\n\n# Sample\n\nBody v1.\n")
	commitSageFreshnessRepo(t, root, "stamp A")

	// c3: reset to a non-terminal posture with a body change.
	mustWrite(t, root, rel, "---\ntitle: Sample\nsage-review-design: required\n---\n\n# Sample\n\nBody v2.\n")
	commitSageFreshnessRepo(t, root, "reset")

	// c4: second completed stamp (transition B, latest), on the new body.
	mustWrite(t, root, rel, "---\ntitle: Sample\nsage-review-design: completed\n---\n\n# Sample\n\nBody v2.\n")
	commitSageFreshnessRepo(t, root, "stamp B")

	// Current body (v2) matches transition B, not transition A (v1). Under
	// the old earliest-transition fallback this would false-positive as
	// stale; the latest-transition fix must read fresh.
	res, err := SageGate(root, SageGateOptions{TicketStem: stem, Landing: "todo"}, "auto")
	if err != nil {
		t.Fatalf("SageGate: %v", err)
	}
	if res.Action == "check_review_required" {
		t.Fatalf("result = %+v, want fresh via latest-transition fallback", res)
	}

	// A later edit after transition B without a further stamp must go stale.
	mustWrite(t, root, rel, "---\ntitle: Sample\nsage-review-design: completed\n---\n\n# Sample\n\nBody v3.\n")
	commitSageFreshnessRepo(t, root, "edit after B")

	res, err = SageGate(root, SageGateOptions{TicketStem: stem, Landing: "todo"}, "auto")
	if err != nil {
		t.Fatalf("SageGate: %v", err)
	}
	if res.Action != "check_review_required" || strings.Join(res.FreshnessStages, ",") != "design" {
		t.Fatalf("result = %+v, want stale design freshness check after post-B edit", res)
	}
}

// TestSageGateFrontmatterOnlyEditStaysFresh covers the ticket's verification
// bullet 3: a frontmatter-only edit (title:) after a stamp does not trigger
// staleness, now that normalization excludes the whole frontmatter block
// (not just sage-review* keys) from both the digest and the fallback
// body-only comparison.
func TestSageGateFrontmatterOnlyEditStaysFresh(t *testing.T) {
	root := t.TempDir()
	stem := "260101-feat-fmonly"
	path := writeSageTicket(t, root, stem, nil)
	initSageFreshnessRepo(t, root)
	commitSageFreshnessRepo(t, root, "initial")

	if _, err := SageRecord(root, SageRecordOptions{
		TicketStem: stem,
		Stage:      "design",
		Today:      "2026-07-29",
		Verdicts:   []SageVerdict{{Reviewer: "design", Verdict: "pass"}},
	}); err != nil {
		t.Fatalf("SageRecord: %v", err)
	}
	commitSageFreshnessRepo(t, root, "stamp review")

	stamped := readFileString(t, path)
	edited := strings.Replace(stamped, "title: Sample", "title: Sample Renamed", 1)
	if edited == stamped {
		t.Fatalf("fixture did not contain expected title line:\n%s", stamped)
	}
	if err := os.WriteFile(path, []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}
	commitSageFreshnessRepo(t, root, "frontmatter-only edit")

	res, err := SageGate(root, SageGateOptions{TicketStem: stem, Landing: "todo"}, "auto")
	if err != nil {
		t.Fatalf("SageGate: %v", err)
	}
	if res.Action == "check_review_required" {
		t.Fatalf("result = %+v, want fresh after frontmatter-only edit", res)
	}
}

// TestSageRecordCountsIssueResolutions pins the autonomous/missing tally that
// feeds the dispatch layer's issue routing. The dispatch-layer tests build a
// SageRecordResult by hand, so without this the counting could be wrong in
// either direction and the suite would stay green.
func TestSageRecordCountsIssueResolutions(t *testing.T) {
	// Standalone counts only the resolved stage's verdict. A stray verdict for
	// the other reviewer must not leak into the tally.
	root := t.TempDir()
	writeSageTicket(t, root, "260101-feat-c1", map[string]string{"sage-review-design": "required"})
	res, err := SageRecord(root, SageRecordOptions{
		TicketStem: "260101-feat-c1",
		Stage:      "design",
		Today:      "2026-07-29",
		Verdicts: []SageVerdict{
			{Reviewer: "design", Verdict: "concern", Issues: []SageIssue{
				{Title: "A", Severity: "important", Resolution: "autonomous"},
				{Title: "B", Severity: "minor", Resolution: "MISSING"},
				// Absent resolution counts as autonomous: an unroutable issue is
				// worse than one the lead tries and fails to fix itself.
				{Title: "C", Severity: "minor"},
			}},
			{Reviewer: "completeness", Verdict: "block", Issues: []SageIssue{
				{Title: "leak", Severity: "critical", Resolution: "missing"},
			}},
		},
	})
	if err != nil {
		t.Fatalf("SageRecord standalone: %v", err)
	}
	if res.Autonomous != 2 || res.Missing != 1 {
		t.Fatalf("standalone tally = %d autonomous / %d missing, want 2/1 (the completeness verdict must not count)", res.Autonomous, res.Missing)
	}

	// Combined counts both consumed verdicts and nothing else.
	root2 := t.TempDir()
	writeSageTicket(t, root2, "260101-feat-c2", map[string]string{"sage-review-design": "required", "sage-review-completeness": "required"})
	res2, err := SageRecord(root2, SageRecordOptions{
		TicketStem: "260101-feat-c2",
		Stage:      "combined",
		Today:      "2026-07-29",
		Verdicts: []SageVerdict{
			{Reviewer: "design", Verdict: "pass", Issues: []SageIssue{{Title: "A", Severity: "minor", Resolution: "autonomous"}}},
			{Reviewer: "completeness", Verdict: "concern", Issues: []SageIssue{
				{Title: "B", Severity: "important", Resolution: "autonomous"},
				{Title: "C", Severity: "important", Resolution: "missing"},
			}},
		},
	})
	if err != nil {
		t.Fatalf("SageRecord combined: %v", err)
	}
	if res2.Autonomous != 2 || res2.Missing != 1 {
		t.Fatalf("combined tally = %d autonomous / %d missing, want 2/1", res2.Autonomous, res2.Missing)
	}

	// A clean pass records no issues, so the routing clause stays absent.
	root3 := t.TempDir()
	writeSageTicket(t, root3, "260101-feat-c3", map[string]string{"sage-review-design": "required"})
	res3, err := SageRecord(root3, SageRecordOptions{
		TicketStem: "260101-feat-c3",
		Stage:      "design",
		Today:      "2026-07-29",
		Verdicts:   []SageVerdict{{Reviewer: "design", Verdict: "pass"}},
	})
	if err != nil {
		t.Fatalf("SageRecord clean pass: %v", err)
	}
	if res3.Autonomous != 0 || res3.Missing != 0 {
		t.Fatalf("clean-pass tally = %d/%d, want 0/0", res3.Autonomous, res3.Missing)
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
	if res2.Posture["sage-review-design"] != "completed" || res2.BlockedSection != "" {
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
	if res.Posture["sage-review-completeness"] != "blocked" {
		t.Fatalf("posture = %v, want blocked", res.Posture)
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
	if res.Verdict != "block" {
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
	if res3.Posture["sage-review-design"] != "completed" || res3.Posture["sage-review-completeness"] != "completed" {
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
	if res4.Verdict != "pass" || res4.Posture["sage-review-design"] != "completed" {
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

// TestSageGateCombinedSeparateAsks pins FIX 1: in combined mode each recommended
// stage gets its own ask (design first) and the supplied answer never leaks from
// one stage to the other.
// TestAppendOrReplaceBlockedSectionExcisesOnlyItsOwnSection pins that excising a
// prior Blocked section never removes the content that follows it. The
// "later-sections-survive" case is shaped like
// ai-docs/tickets/ready/260726-bug-inline-playbook-invocation-commit-ownership.md,
// where a lead recorded two "## " sections after the Blocked section.
func TestAppendOrReplaceBlockedSectionExcisesOnlyItsOwnSection(t *testing.T) {
	const section = "## Blocked (2026-07-29)\n\n### Design Reviewer — block\n\n| # | Title | Severity |\n|---|-------|----------|\n| 1 | fresh | high |"
	const header = "---\ntitle: Sample\n---\n\n# Sample\n\nBody text.\n"
	const prior = "## Blocked (2026-07-27)\n\n### Design Reviewer — block\n\n| # | Title | Severity |\n|---|-------|----------|\n| 1 | stale | high |\n"
	const later = "## Landing-order inversion (2026-07-28)\n\nLater note one.\n\n## Category C dissolved (2026-07-28)\n\nLater note two.\n"

	cases := []struct {
		name  string
		start string
		want  string
	}{
		{
			name:  "no-prior-blocked-appends",
			start: header,
			want:  header + "\n" + section + "\n",
		},
		{
			name:  "no-prior-blocked-keeps-trailing-newline-normalization",
			start: header + "\n\n\n",
			want:  header + "\n" + section + "\n",
		},
		{
			name:  "prior-blocked-last-is-replaced",
			start: header + "\n" + prior,
			want:  header + "\n" + section + "\n",
		},
		{
			name:  "later-sections-survive",
			start: header + "\n" + prior + "\n" + later,
			want:  header + "\n" + later + "\n" + section + "\n",
		},
		{
			name:  "multiple-prior-blocked-sections-all-excised",
			start: header + "\n" + prior + "\n" + later + "\n" + prior,
			want:  header + "\n" + later + "\n" + section + "\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			rel := filepath.Join("ai-docs", "tickets", "ready", "260101-bug-sample.md")
			mustWrite(t, root, rel, tc.start)
			path := filepath.Join(root, rel)
			if err := appendOrReplaceBlockedSection(path, section); err != nil {
				t.Fatalf("appendOrReplaceBlockedSection: %v", err)
			}
			if got := readFileString(t, path); got != tc.want {
				t.Fatalf("body mismatch:\ngot:\n%q\nwant:\n%q", got, tc.want)
			}
		})
	}
}

// TestSageRecordSecondBlockKeepsLaterSections is the end-to-end shape of the
// data loss: a second block verdict on a ticket whose Blocked section is not
// last must not delete the sections written after it.
func TestSageRecordSecondBlockKeepsLaterSections(t *testing.T) {
	root := t.TempDir()
	path := writeSageTicket(t, root, "260101-bug-later", map[string]string{"sage-review-design": "required"})
	record := func(title string) {
		t.Helper()
		if _, err := SageRecord(root, SageRecordOptions{
			TicketStem: "260101-bug-later", Stage: "design", Today: "2026-07-29",
			Verdicts: []SageVerdict{{Reviewer: "design", Verdict: "block", Issues: []SageIssue{{Title: title, Severity: "high", Resolution: "missing"}}}},
		}); err != nil {
			t.Fatalf("SageRecord %s: %v", title, err)
		}
	}
	record("first")

	const later = "## Landing-order inversion (2026-07-28)\n\nLater note one.\n\n## Category C dissolved (2026-07-28)\n\nLater note two.\n"
	mustWrite(t, root, filepath.Join("ai-docs", "tickets", "todo", "260101-bug-later.md"), readFileString(t, path)+"\n"+later)

	record("second")

	body := readFileString(t, path)
	if !strings.Contains(body, later) {
		t.Fatalf("later sections did not survive verbatim:\n%s", body)
	}
	if strings.Count(body, "## Blocked (") != 1 || strings.Contains(body, "first") || !strings.Contains(body, "second") {
		t.Fatalf("prior Blocked section not replaced exactly once:\n%s", body)
	}
	if strings.Index(body, later) > strings.Index(body, "## Blocked (") {
		t.Fatalf("new Blocked section should be appended after the later sections:\n%s", body)
	}
}

func TestSageGateCombinedSeparateAsks(t *testing.T) {
	newTicket := func(t *testing.T) (string, string) {
		root := t.TempDir()
		stem := "260101-feat-comb"
		path := writeSageTicket(t, root, stem, map[string]string{
			"sage-review-design":       "recommended",
			"sage-review-completeness": "recommended",
		})
		return root, path
	}

	// Both recommended, no answer -> ask design first.
	t.Run("design-ask-first", func(t *testing.T) {
		root, _ := newTicket(t)
		res, err := SageGate(root, SageGateOptions{TicketStem: "260101-feat-comb", Landing: "ready"}, "ask")
		if err != nil {
			t.Fatal(err)
		}
		if res.Action != "ask" || res.AskPrompt != "Run design review for this ticket?" {
			t.Fatalf("first ask = %+v, want ask design", res)
		}
	})

	// Design declined: design persisted skipped (written, uncommitted, and with
	// no proposed commit of its own), and completeness is ASKED (not silently
	// skipped) — the answer must not leak.
	t.Run("design-decline-then-completeness-ask", func(t *testing.T) {
		root, path := newTicket(t)
		res, err := SageGate(root, SageGateOptions{TicketStem: "260101-feat-comb", Landing: "ready", Answer: "no"}, "ask")
		if err != nil {
			t.Fatal(err)
		}
		if res.Action != "ask" || res.AskPrompt != "Run completeness review for this ticket?" {
			t.Fatalf("after design decline = %+v, want completeness ask", res)
		}
		body := readFileString(t, path)
		if !strings.Contains(body, "sage-review-design: skipped") {
			t.Fatalf("design not persisted skipped:\n%s", body)
		}
		// The leak guard: completeness must still be recommended, NOT skipped.
		if strings.Contains(body, "sage-review-completeness: skipped") {
			t.Fatalf("design's 'no' leaked into completeness:\n%s", body)
		}
		if !strings.Contains(body, "sage-review-completeness: recommended") {
			t.Fatalf("completeness posture unexpectedly changed:\n%s", body)
		}
	})

	// Design accepted: persisted required, then completeness is asked.
	t.Run("design-accept-then-completeness-ask", func(t *testing.T) {
		root, path := newTicket(t)
		res, err := SageGate(root, SageGateOptions{TicketStem: "260101-feat-comb", Landing: "ready", Answer: "yes"}, "ask")
		if err != nil {
			t.Fatal(err)
		}
		if res.Action != "ask" || res.AskPrompt != "Run completeness review for this ticket?" {
			t.Fatalf("after design accept = %+v, want completeness ask", res)
		}
		if !strings.Contains(readFileString(t, path), "sage-review-design: required") {
			t.Fatalf("design accept not persisted as required:\n%s", readFileString(t, path))
		}
	})

	// Design already required, completeness recommended, no answer -> ask
	// completeness (design's run deferred; completeness asked separately).
	t.Run("completeness-ask-after-design-runs", func(t *testing.T) {
		root := t.TempDir()
		writeSageTicket(t, root, "260101-feat-comb2", map[string]string{
			"sage-review-design":       "required",
			"sage-review-completeness": "recommended",
		})
		res, err := SageGate(root, SageGateOptions{TicketStem: "260101-feat-comb2", Landing: "ready"}, "ask")
		if err != nil {
			t.Fatal(err)
		}
		if res.Action != "ask" || res.AskPrompt != "Run completeness review for this ticket?" {
			t.Fatalf("= %+v, want completeness ask", res)
		}
	})

	// Completeness accepted after design runs -> both run combined.
	t.Run("completeness-accept-both-run", func(t *testing.T) {
		root := t.TempDir()
		writeSageTicket(t, root, "260101-feat-comb3", map[string]string{
			"sage-review-design":       "required",
			"sage-review-completeness": "recommended",
		})
		res, err := SageGate(root, SageGateOptions{TicketStem: "260101-feat-comb3", Landing: "ready", Answer: "yes"}, "ask")
		if err != nil {
			t.Fatal(err)
		}
		if res.Action != "run" || res.Mode != "combined" || len(res.Reviewers) != 2 {
			t.Fatalf("= %+v, want run combined [design completeness]", res)
		}
	})

	// Completeness declined after design runs -> design runs standalone and the
	// completeness decline is persisted as skipped, uncommitted.
	t.Run("completeness-decline-design-only", func(t *testing.T) {
		root := t.TempDir()
		path := writeSageTicket(t, root, "260101-feat-comb4", map[string]string{
			"sage-review-design":       "required",
			"sage-review-completeness": "recommended",
		})
		res, err := SageGate(root, SageGateOptions{TicketStem: "260101-feat-comb4", Landing: "ready", Answer: "no"}, "ask")
		if err != nil {
			t.Fatal(err)
		}
		if res.Action != "run" || res.Mode != "standalone" || len(res.Reviewers) != 1 || res.Reviewers[0] != "design" {
			t.Fatalf("= %+v, want run standalone [design]", res)
		}
		if !strings.Contains(readFileString(t, path), "sage-review-completeness: skipped") {
			t.Fatalf("completeness decline not persisted skipped:\n%s", readFileString(t, path))
		}
	})
}

// TestSageGateCombinedDegradesToDesignStandalone pins FIX 5: a combined-eligible
// ticket (design non-terminal) whose completeness is already terminal degrades
// to a design-only standalone run. This is an intentional divergence from the
// pre-diff prose for that anomalous state.
func TestSageGateCombinedDegradesToDesignStandalone(t *testing.T) {
	root := t.TempDir()
	writeSageTicket(t, root, "260101-feat-degr", map[string]string{
		"sage-review-design":       "required",
		"sage-review-completeness": "completed",
	})
	res, err := SageGate(root, SageGateOptions{TicketStem: "260101-feat-degr", Landing: "ready"}, "auto")
	if err != nil {
		t.Fatal(err)
	}
	if res.Action != "run" || res.Mode != "standalone" || len(res.Reviewers) != 1 || res.Reviewers[0] != "design" {
		t.Fatalf("degraded = %+v, want run standalone [design]", res)
	}
}

// TestSageMissingTicketErrors pins FIX 3(a): a well-formed but nonexistent stem
// hits the findTicketPath not-found branch for both tools.
func TestSageMissingTicketErrors(t *testing.T) {
	root := t.TempDir()
	writeSageTicket(t, root, "260101-feat-present", nil)
	if _, err := SageGate(root, SageGateOptions{TicketStem: "260101-feat-ghost", Landing: "todo"}, "auto"); err == nil {
		t.Error("SageGate: expected not-found error for nonexistent stem")
	}
	if _, err := SageRecord(root, SageRecordOptions{
		TicketStem: "260101-feat-ghost", Stage: "design",
		Verdicts: []SageVerdict{{Reviewer: "design", Verdict: "pass"}},
	}); err == nil {
		t.Error("SageRecord: expected not-found error for nonexistent stem")
	}
}

// TestSageRecordValidationAndEmptyVerdicts pins FIX 3(b)+(c): SageRecord's own
// stem-format validation, and the requirement that a missing reviewer verdict is
// an error rather than a silent completed write.
func TestSageRecordValidationAndEmptyVerdicts(t *testing.T) {
	root := t.TempDir()
	path := writeSageTicket(t, root, "260101-feat-ev", map[string]string{
		"sage-review-design":       "required",
		"sage-review-completeness": "required",
	})

	// (b) bad stem format.
	if _, err := SageRecord(root, SageRecordOptions{TicketStem: "not-a-stem", Stage: "design"}); err == nil {
		t.Error("SageRecord: expected stem-format error")
	}

	// (c) empty verdicts must error for every stage, never silently write completed.
	for _, stage := range []string{"design", "completeness", "combined"} {
		if _, err := SageRecord(root, SageRecordOptions{TicketStem: "260101-feat-ev", Stage: stage, Today: "2026-07-20"}); err == nil {
			t.Errorf("SageRecord stage=%s with empty verdicts: expected error", stage)
		}
	}
	// Nothing should have been written to disk by the erroring calls.
	if strings.Contains(readFileString(t, path), "completed") {
		t.Fatalf("empty-verdict error path must not write completed:\n%s", readFileString(t, path))
	}

	// combined with only one reviewer tagged must also error (no positional leak).
	if _, err := SageRecord(root, SageRecordOptions{
		TicketStem: "260101-feat-ev", Stage: "combined", Today: "2026-07-20",
		Verdicts: []SageVerdict{{Reviewer: "design", Verdict: "pass"}},
	}); err == nil {
		t.Error("SageRecord combined with only design verdict: expected error")
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
