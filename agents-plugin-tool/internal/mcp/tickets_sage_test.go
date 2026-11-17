package mcp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsdoc"
)

func TestParseSageVerdicts(t *testing.T) {
	// Happy path: a fully specified verdict with one issue round-trips.
	got, err := parseSageVerdicts([]any{
		map[string]any{
			"reviewer": "design",
			"verdict":  "block",
			"issues": []any{
				map[string]any{"title": "T", "severity": "high", "resolution": "missing"},
			},
		},
	})
	if err != nil {
		t.Fatalf("parseSageVerdicts happy: %v", err)
	}
	if len(got) != 1 || got[0].Reviewer != "design" || got[0].Verdict != "block" {
		t.Fatalf("parsed verdict = %+v", got)
	}
	if len(got[0].Issues) != 1 || got[0].Issues[0].Title != "T" || got[0].Issues[0].Severity != "high" || got[0].Issues[0].Resolution != "missing" {
		t.Fatalf("parsed issue = %+v", got[0].Issues)
	}

	// The four error branches.
	if _, err := parseSageVerdicts(nil); err == nil {
		t.Error("nil verdicts: expected error")
	}
	if _, err := parseSageVerdicts("not-an-array"); err == nil {
		t.Error("non-array verdicts: expected error")
	}
	if _, err := parseSageVerdicts([]any{"not-an-object"}); err == nil {
		t.Error("non-object verdict item: expected error")
	}
	if _, err := parseSageVerdicts([]any{map[string]any{"issues": []any{"not-an-object"}}}); err == nil {
		t.Error("non-object issue item: expected error")
	}
}

func TestFormatSageGateRoundTrip(t *testing.T) {
	// A combined-mode run (the state a completeness decline lands in: design
	// still runs). formatSageGate must render only the gate decision — no
	// commit title, no commit paths, no ws/git.commit call handed to the
	// caller. The posture write rides the caller's own next ordinary commit.
	out := formatSageGate(wsdoc.SageGateResult{
		Action:    "run",
		Reviewers: []string{"design", "completeness"},
		Mode:      "combined",
	})
	for _, want := range []string{
		"action: run", "reviewers: design, completeness", "mode: combined",
		"next_instruction:", "stage=combined",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("formatSageGate missing %q in:\n%s", want, out)
		}
	}
	if strings.Contains(out, "commit: ") {
		t.Fatalf("formatSageGate must not claim a commit happened:\n%s", out)
	}

	// ask action carries the prompt and the relay instruction.
	askOut := formatSageGate(wsdoc.SageGateResult{Action: "ask", AskPrompt: "prompt-token"})
	if !strings.Contains(askOut, "ask_prompt: prompt-token") || !strings.Contains(askOut, "answer=yes|no") {
		t.Fatalf("formatSageGate ask output:\n%s", askOut)
	}

	// No gate action may propose a commit for the posture flip. A
	// regression that reintroduces a canonical title, a pending_commit_* key
	// family, or a ready-to-paste ws/git.commit call on any branch fails here.
	// stop_blocked is included because sageGateCombined can persist a design
	// posture before reaching the completeness blocked branch, so it is not a
	// write-free action.
	skipOut := formatSageGate(wsdoc.SageGateResult{Action: "skip"})
	blockedOut := formatSageGate(wsdoc.SageGateResult{Action: "stop_blocked"})
	for name, text := range map[string]string{"skip": skipOut, "ask": askOut, "run": out, "stop_blocked": blockedOut} {
		if strings.Contains(text, "ws/git.commit") || strings.Contains(text, "git.commit(") {
			t.Fatalf("formatSageGate %s must not hand the caller a commit call:\n%s", name, text)
		}
		if strings.Contains(text, "pending_commit") || strings.Contains(text, "chore(sage)") {
			t.Fatalf("formatSageGate %s must not carry commit metadata:\n%s", name, text)
		}
		if !strings.Contains(text, "next_instruction:") {
			t.Fatalf("formatSageGate %s missing next_instruction:\n%s", name, text)
		}
	}

	// The advisory is returned as a field when supplied.
	runOut := formatSageGate(wsdoc.SageGateResult{
		Action:    "run",
		Reviewers: []string{"design"},
		Mode:      "standalone",
		Advisory:  "advisory-token",
	})
	if !strings.Contains(runOut, "advisory: Advisory-token") {
		t.Fatalf("formatSageGate missing advisory field:\n%s", runOut)
	}

	checkOut := formatSageGate(wsdoc.SageGateResult{
		Action:            "check_review_required",
		FreshnessStages:   []string{"design", "completeness"},
		ReviewBaseline:    "abc12345",
		ReviewInstruction: "instruction-token",
	})
	for _, want := range []string{
		"action: check_review_required",
		"freshness_stages: design, completeness",
		"review_baseline: abc12345",
		"review_instruction: instruction-token",
	} {
		if !strings.Contains(checkOut, want) {
			t.Fatalf("formatSageGate freshness output missing %q:\n%s", want, checkOut)
		}
	}
}

func TestFormatSageRecordRoundTrip(t *testing.T) {
	out := formatSageRecord(wsdoc.SageRecordResult{
		Verdict:        "block",
		Posture:        map[string]string{"sage-review-design": "blocked", "sage-review-completeness": "blocked"},
		BlockedSection: "## Blocked (2026-07-20)",
	})
	for _, want := range []string{"verdict: block", "sage-review-design: blocked", "sage-review-completeness: blocked", "blocked_section: appended"} {
		if !strings.Contains(out, want) {
			t.Fatalf("formatSageRecord missing %q in:\n%s", want, out)
		}
	}
	if strings.Contains(out, "commit:") {
		t.Fatalf("formatSageRecord should not claim a commit:\n%s", out)
	}

	// concern surfaces the manual-escalation instruction and routes the lead to
	// its own commit rather than claiming one happened. The escalation sentence
	// is about a missing decision, so it is gated on one being present —
	// TestFormatSageRecordConcernWithoutMissing pins the other side of the gate.
	concernOut := formatSageRecord(wsdoc.SageRecordResult{
		Verdict: "concern",
		Posture: map[string]string{"sage-review-design": "completed"},
		Missing: 1,
	})
	if !strings.Contains(concernOut, "verdict: concern") || !strings.Contains(concernOut, "missing_issues: 1") || !strings.Contains(concernOut, "next_instruction:") {
		t.Fatalf("formatSageRecord concern output:\n%s", concernOut)
	}

	// pass surfaces the same no-commit routing.
	passOut := formatSageRecord(wsdoc.SageRecordResult{
		Verdict: "pass",
		Posture: map[string]string{"sage-review-design": "completed"},
	})
	// A clean pass carries no issues, so it must not grow a routing clause.
	if strings.Contains(passOut, "autonomous_issues:") || strings.Contains(passOut, "missing_issues:") {
		t.Fatalf("formatSageRecord clean pass must stay terse:\n%s", passOut)
	}
}

// The blocked gate result carries no reviewers field.
func TestFormatSageGateBlockedRecovery(t *testing.T) {
	out := formatSageGate(wsdoc.SageGateResult{Action: "stop_blocked"})
	// The gate carries no stage field, so the text must not imply the caller can
	// read the blocked stage off this result.
	if strings.Contains(out, "reviewers:") {
		t.Fatalf("stop_blocked must not render a reviewers line:\n%s", out)
	}
}

// A block reports its posture, issue count, and recovery tool.
func TestFormatSageRecordBlockRecovery(t *testing.T) {
	out := formatSageRecord(wsdoc.SageRecordResult{
		Verdict:        "block",
		Posture:        map[string]string{"sage-review-design": "blocked"},
		BlockedSection: "## Blocked (2026-07-29)",
		Missing:        1,
	})
	for _, want := range []string{"verdict: block", "sage-review-design: blocked", "blocked_section: appended", "missing_issues: 1", "next_instruction:", "ws/tickets.sage_stamp"} {
		if !strings.Contains(out, want) {
			t.Fatalf("formatSageRecord block output missing %q:\n%s", want, out)
		}
	}
}

// A concern can carry autonomous issues without missing issues.
func TestFormatSageRecordConcernWithoutMissing(t *testing.T) {
	out := formatSageRecord(wsdoc.SageRecordResult{
		Verdict:    "concern",
		Posture:    map[string]string{"sage-review-design": "completed"},
		Autonomous: 2,
	})
	if !strings.Contains(out, "verdict: concern") || !strings.Contains(out, "autonomous_issues: 2") || strings.Contains(out, "missing_issues:") {
		t.Fatalf("concern issue fields:\n%s", out)
	}
}

// TestFormatSageRecordIssueRouting covers the consumer of the reviewers'
// `resolution` field. Both reviewer playbooks emit autonomous/missing, but until
// now nothing routed on it and the caller improvised.
func TestFormatSageRecordIssueRouting(t *testing.T) {
	both := formatSageRecord(wsdoc.SageRecordResult{
		Verdict:    "concern",
		Posture:    map[string]string{"sage-review-design": "completed"},
		Autonomous: 2,
		Missing:    1,
	})
	for _, want := range []string{"autonomous_issues: 2", "missing_issues: 1", "next_instruction:", "ws/tickets.sage_stamp"} {
		if !strings.Contains(both, want) {
			t.Fatalf("formatSageRecord mixed routing missing %q in:\n%s", want, both)
		}
	}

	autonomousOnly := formatSageRecord(wsdoc.SageRecordResult{
		Verdict:    "pass",
		Posture:    map[string]string{"sage-review-completeness": "completed"},
		Autonomous: 3,
	})
	if !strings.Contains(autonomousOnly, "autonomous_issues: 3") || strings.Contains(autonomousOnly, "missing_issues:") || !strings.Contains(autonomousOnly, "ws/tickets.sage_stamp") {
		t.Fatalf("formatSageRecord autonomous-only fields:\n%s", autonomousOnly)
	}

	missingOnly := formatSageRecord(wsdoc.SageRecordResult{
		Verdict: "concern",
		Posture: map[string]string{"sage-review-design": "completed"},
		Missing: 1,
	})
	if !strings.Contains(missingOnly, "missing_issues: 1") || strings.Contains(missingOnly, "autonomous_issues:") || !strings.Contains(missingOnly, "next_instruction:") {
		t.Fatalf("formatSageRecord missing-only fields:\n%s", missingOnly)
	}
}

// Autonomous issues name the stamp tool; a clean pass does not.
func TestFormatSageRecordAutonomousOrdersRestamp(t *testing.T) {
	const restamp = "ws/tickets.sage_stamp"

	for name, result := range map[string]wsdoc.SageRecordResult{
		"autonomous-only": {
			Verdict:    "pass",
			Posture:    map[string]string{"sage-review-design": "completed"},
			Autonomous: 3,
		},
		"autonomous-and-missing": {
			Verdict:    "concern",
			Posture:    map[string]string{"sage-review-design": "completed"},
			Autonomous: 2,
			Missing:    1,
		},
	} {
		out := formatSageRecord(result)
		restampIdx := strings.Index(out, restamp)
		if restampIdx == -1 {
			t.Fatalf("%s routing must order a re-stamp so the digest covers the fixed body:\n%s", name, out)
		}
	}

	clean := formatSageRecord(wsdoc.SageRecordResult{
		Verdict: "pass",
		Posture: map[string]string{"sage-review-design": "completed"},
	})
	if strings.Contains(clean, restamp) {
		t.Fatalf("a stage with no issues edits nothing and must stay terse:\n%s", clean)
	}
}

// Block outputs retain their posture fields and name the stamp tool.
func TestFormatSageRecordBlockOrdersDeltaReview(t *testing.T) {
	design := map[string]string{"sage-review-design": "blocked"}
	combined := map[string]string{"sage-review-design": "blocked", "sage-review-completeness": "blocked"}

	for name, result := range map[string]wsdoc.SageRecordResult{
		"autonomous-only": {Verdict: "block", Posture: design, BlockedSection: "## Blocked", Autonomous: 2},
		"mixed":           {Verdict: "block", Posture: design, BlockedSection: "## Blocked", Autonomous: 2, Missing: 1},
		"missing-only":    {Verdict: "block", Posture: map[string]string{"sage-review-completeness": "blocked"}, BlockedSection: "## Blocked", Missing: 1},
		"combined-mixed":  {Verdict: "block", Posture: combined, BlockedSection: "## Blocked", Autonomous: 1, Missing: 1},
		"no-issues":       {Verdict: "block", Posture: design, BlockedSection: "## Blocked"},
	} {
		out := formatSageRecord(result)
		for _, want := range []string{"verdict: block", "blocked_section: appended", "next_instruction:", "ws/tickets.sage_stamp"} {
			if !strings.Contains(out, want) {
				t.Fatalf("%s: block output missing %q:\n%s", name, want, out)
			}
		}
		for field := range result.Posture {
			if !strings.Contains(out, field+": blocked") {
				t.Fatalf("%s: block output missing %s:\n%s", name, field, out)
			}
		}
	}

	// A mixed concern is not blocked: its same-verdict re-stamp is what refreshes
	// the digest over the fixed body, so it keeps that direction.
	concern := formatSageRecord(wsdoc.SageRecordResult{
		Verdict:    "concern",
		Posture:    map[string]string{"sage-review-design": "completed"},
		Autonomous: 1,
		Missing:    1,
	})
	if !strings.Contains(concern, "verdict: concern") || !strings.Contains(concern, "ws/tickets.sage_stamp") {
		t.Fatalf("mixed concern output:\n%s", concern)
	}
}

func TestServeStdioSageGateDispatch(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	mustWrite(t, root, filepath.Join("ai-docs", "tickets", "todo", "260101-epic-sg.md"),
		"---\ntitle: Sage\nsage-review-design: required\n---\n\n## Route Facts\n\n| fact | value |\n|---|---|\n| scope.span | single-file |\n\nBody.\n")
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))
	t.Setenv("WS_MCP_NAMESPACE", "wsflow")

	server := NewServer(root, "test")
	key, _ := parseLoginResponse(t, callLogin(t, server, 9701, root, nil))

	resp := callToolWithKey(t, server, 9702, key, "tickets.sage_gate", map[string]any{
		"stem":    "260101-epic-sg",
		"landing": "todo",
	})
	if !strings.Contains(resp, "action: run") || !strings.Contains(resp, "reviewers: design") || !strings.Contains(resp, "mode: standalone") {
		t.Fatalf("sage_gate dispatch response:\n%s", resp)
	}
	if !strings.Contains(resp, "wsflow/tickets.sage_stamp") || strings.Contains(resp, "ws/tickets.sage_stamp") || strings.Contains(resp, "ws/config.list") {
		t.Fatalf("sage_gate dispatch used the wrong namespace:\n%s", resp)
	}
	// C5: the required->run advisory (non-waivable statement + review-scope
	// line) must reach the actual MCP dispatch response, not just the
	// wsdoc.SageGateResult struct field.
	if !strings.Contains(resp, "advisory:") {
		t.Fatalf("sage_gate dispatch response missing advisory line:\n%s", resp)
	}
}

// TestServeStdioOpenDecisionQueueRefusals pins that the pending-queue refusal
// reaches the caller through both MCP tools: sage_gate's structured stop with
// its settle-point-and-delete instruction (and no posture-uncommitted note,
// since the stop writes nothing), and tickets.move's error, with the ticket
// left in todo/.
func TestServeStdioOpenDecisionQueueRefusals(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	stem := "260101-feat-odq"
	ticketRel := filepath.Join("ai-docs", "tickets", "todo", stem+".md")
	mustWrite(t, root, ticketRel,
		"---\ntitle: Sage\n---\n\n## Open Decision Queue\n\n(1) [open] Pick a bound.\n\n## Route Facts\n\n| fact | value |\n|---|---|\n| scope.span | single-file |\n\nBody.\n")
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))

	server := NewServer(root, "test")
	key, _ := parseLoginResponse(t, callLogin(t, server, 9711, root, nil))

	gate := callToolWithKey(t, server, 9712, key, "tickets.sage_gate", map[string]any{"stem": stem, "landing": "ready"})
	for _, want := range []string{"action: stop_open_decision_queue", "## Open Decision Queue"} {
		if !strings.Contains(gate, want) {
			t.Fatalf("sage_gate response missing %q:\n%s", want, gate)
		}
	}

	move := callToolWithKey(t, server, 9713, key, "tickets.move", map[string]any{"stem": stem, "to": "ready"})
	if !strings.Contains(move, "## Open Decision Queue") {
		t.Fatalf("tickets.move response missing the queue refusal:\n%s", move)
	}
	if _, err := os.Stat(filepath.Join(root, ticketRel)); err != nil {
		t.Fatalf("refused move must leave the ticket in todo/: %v", err)
	}
}

func TestServeStdioSageGateDetectsStaleCompletedReview(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	ticketRel := filepath.Join("ai-docs", "tickets", "ready", "260101-feat-stale.md")
	mustWrite(t, root, ticketRel,
		"---\ntitle: Sage\nsage-review-design: completed\nsage-review-completeness: completed\n---\n\n## Route Facts\n\n| fact | value |\n|---|---|\n| scope.span | single-file |\n\nBody.\n")
	initGit(t, root)
	runGit(t, root, "add", ticketRel)
	runGit(t, root, "commit", "-m", "stamp review")
	mustWrite(t, root, ticketRel,
		"---\ntitle: Sage\nsage-review-design: completed\nsage-review-completeness: completed\n---\n\n## Route Facts\n\n| fact | value |\n|---|---|\n| scope.span | single-file |\n\nChanged body.\n")
	runGit(t, root, "add", ticketRel)
	runGit(t, root, "commit", "-m", "edit after review")
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))

	server := NewServer(root, "test")
	key, _ := parseLoginResponse(t, callLogin(t, server, 9707, root, nil))

	resp := callToolWithKey(t, server, 9708, key, "tickets.sage_gate", map[string]any{
		"stem":    "260101-feat-stale",
		"landing": "ready",
	})
	for _, want := range []string{
		"action: check_review_required",
		"freshness_stages: design, completeness",
		"review_baseline:",
		"review_instruction:",
	} {
		if !strings.Contains(resp, want) {
			t.Fatalf("sage_gate stale response missing %q:\n%s", want, resp)
		}
	}
}

// TestServeStdioSageGateDeclineDoesNotAutoCommit is the C2 regression test:
// the ask-decline path (recommended posture + answer=="no") must write and
// persist the "skipped" posture, then stop. It must neither commit it (the
// original defect: a nil-Verifier wsgit.NewClient() commit bypassing the
// ready-sage-posture guardrail chokepoint) nor propose a separate commit for
// it (the cycle-1 relocation of the same defect: a canonically-titled
// ws/git.commit call handed to the caller, whose `-A` staging sweeps the whole
// uncommitted ticket into a commit describing only the posture flip).
func TestServeStdioSageGateDeclineDoesNotAutoCommit(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	ticketRel := filepath.Join("ai-docs", "tickets", "todo", "260101-feat-decline.md")
	mustWrite(t, root, ticketRel,
		"---\ntitle: Sage\nsage-review-design: skipped\nsage-review-completeness: recommended\n---\n\n## Route Facts\n\n| fact | value |\n|---|---|\n| scope.span | single-file |\n\nBody.\n")
	initGit(t, root)
	runGit(t, root, "add", ticketRel)
	runGit(t, root, "commit", "-m", "initial ticket")
	logBefore := runGitOutput(t, root, "log", "--oneline")
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))

	server := NewServer(root, "test")
	key, _ := parseLoginResponse(t, callLogin(t, server, 9705, root, nil))

	resp := callToolWithKey(t, server, 9706, key, "tickets.sage_gate", map[string]any{
		"stem":    "260101-feat-decline",
		"landing": "ready",
		"answer":  "no",
	})
	if !strings.Contains(resp, "action: skip") {
		t.Fatalf("sage_gate decline response should resolve to skip:\n%s", resp)
	}
	// The decline proposes no commit of its own — not an automatic one and not
	// a suggested one. A canonical "chore(sage): skip ... review" title over a
	// ticket file swallows the co-located real edits (260725,
	// {#260720-wsdoc-commit-boundary}), so the whole payload is gone.
	for _, forbidden := range []string{"pending_commit", "chore(sage)", "ws/git.commit", "git.commit("} {
		if strings.Contains(resp, forbidden) {
			t.Fatalf("sage_gate decline response must not carry %q:\n%s", forbidden, resp)
		}
	}
	if strings.Contains(resp, "commit: ") {
		t.Fatalf("sage_gate decline response must not claim a commit happened:\n%s", resp)
	}

	logAfter := runGitOutput(t, root, "log", "--oneline")
	if string(logBefore) != string(logAfter) {
		t.Fatalf("sage_gate decline dispatch must not create a commit: before=%q after=%q", logBefore, logAfter)
	}
	body, err := os.ReadFile(filepath.Join(root, ticketRel))
	if err != nil {
		t.Fatalf("read ticket: %v", err)
	}
	if !strings.Contains(string(body), "sage-review-completeness: skipped") {
		t.Fatalf("sage_gate decline must still write the skipped posture:\n%s", body)
	}
	status := strings.TrimRight(string(runGitOutput(t, root, "status", "--porcelain", ticketRel)), "\n")
	if status != " M "+filepath.ToSlash(ticketRel) {
		t.Fatalf("sage_gate decline dispatch should leave the ticket modified-but-unstaged, got status %q", status)
	}
}

func TestServeStdioSageStampDispatch(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	ticketRel := filepath.Join("ai-docs", "tickets", "todo", "260101-feat-sr.md")
	mustWrite(t, root, ticketRel,
		"---\ntitle: Sage\nsage-review-design: required\n---\n\n## Route Facts\n\n| fact | value |\n|---|---|\n| scope.span | single-file |\n\nBody.\n")
	initGit(t, root)
	// Commit the ticket file first so the subsequent posture write can be
	// distinguished as modified-but-unstaged rather than merely untracked —
	// this is the shape the ticket's swallow bug depends on (an already
	// committed ticket file with further uncommitted edits at sage_stamp time).
	runGit(t, root, "add", ticketRel)
	runGit(t, root, "commit", "-m", "initial ticket")
	logBefore := runGitOutput(t, root, "log", "--oneline")
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))

	server := NewServer(root, "test")
	key, _ := parseLoginResponse(t, callLogin(t, server, 9801, root, nil))

	resp := callToolWithKey(t, server, 9802, key, "tickets.sage_stamp", map[string]any{
		"stem":  "260101-feat-sr",
		"stage": "design",
		"verdicts": []any{
			map[string]any{"reviewer": "design", "verdict": "pass"},
		},
	})
	if !strings.Contains(resp, "verdict: pass") || !strings.Contains(resp, "sage-review-design: completed") {
		t.Fatalf("sage_stamp dispatch response:\n%s", resp)
	}
	if strings.Contains(resp, "commit:") {
		t.Fatalf("sage_stamp dispatch should not report a commit:\n%s", resp)
	}
	logAfter := runGitOutput(t, root, "log", "--oneline")
	if string(logBefore) != string(logAfter) {
		t.Fatalf("sage_stamp dispatch must not create a commit: before=%q after=%q", logBefore, logAfter)
	}
	status := strings.TrimRight(string(runGitOutput(t, root, "status", "--porcelain", ticketRel)), "\n")
	if status != " M "+filepath.ToSlash(ticketRel) {
		t.Fatalf("sage_stamp dispatch should leave the ticket modified-but-unstaged, got status %q", status)
	}

	// Missing verdict for the requested stage must surface as a tool error, not a
	// silent completed write.
	errResp := callToolWithKey(t, server, 9803, key, "tickets.sage_stamp", map[string]any{
		"stem":     "260101-feat-sr",
		"stage":    "combined",
		"verdicts": []any{map[string]any{"reviewer": "design", "verdict": "pass"}},
	})
	if !strings.Contains(errResp, "both design and completeness") {
		t.Fatalf("expected missing-verdict error, got:\n%s", errResp)
	}
}

// TestServeStdioSageStampDelegateKeyBlocked confirms the new lead-only gate
// (260723 Phase 2): a delegate-scoped session key must be rejected at the
// keyed capability gate, mirroring TestWorkflowManualDelegateKeyBlocked. This
// is genuinely new enforcement — the pre-rename tickets.sage_record carried
// no isLeadOnlyTool entry, so a delegate key could reach it.
func TestServeStdioSageStampDelegateKeyBlocked(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	mustWrite(t, root, filepath.Join("ai-docs", "tickets", "todo", "260101-feat-blocked.md"),
		"---\ntitle: Sage\nsage-review-design: required\n---\n\n## Route Facts\n\n| fact | value |\n|---|---|\n| scope.span | single-file |\n\nBody.\n")
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))

	server := NewServer(root, "test")
	delegateKey, err := server.sessions.mint(root, roleDelegate, "")
	if err != nil {
		t.Fatalf("mint delegate key: %v", err)
	}

	rawResp := callToolOnce(t, server, 9804, "tickets.sage_stamp", map[string]any{
		"session_key": delegateKey,
		"stem":        "260101-feat-blocked",
		"stage":       "design",
		"verdicts": []any{
			map[string]any{"reviewer": "design", "verdict": "pass"},
		},
	})
	if !strings.Contains(rawResp, "tool not available in current") {
		t.Errorf("delegate key: expected lead-only rejection, got:\n%s", rawResp)
	}
	if !strings.Contains(rawResp, "-32601") {
		t.Errorf("delegate key: expected JSON-RPC error code -32601, got:\n%s", rawResp)
	}
	// The rejection must be a pure gate error, not a partial write.
	body, readErr := os.ReadFile(filepath.Join(root, "ai-docs", "tickets", "todo", "260101-feat-blocked.md"))
	if readErr != nil {
		t.Fatalf("read ticket: %v", readErr)
	}
	if strings.Contains(string(body), "sage-review-design: completed") {
		t.Errorf("delegate key: rejected call must not have written frontmatter:\n%s", body)
	}
}

func TestServeStdioActionableTodoGateRejects(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))
	server := NewServer(root, "test")
	key, _ := parseLoginResponse(t, callLogin(t, server, 9800, root, nil))
	for _, category := range []string{"feat", "bug", "refactor", "chore"} {
		stem := "260101-" + category + "-todo"
		rel := filepath.Join("ai-docs", "tickets", "todo", stem+".md")
		before := "---\ntitle: Backlog\n---\n\nBody.\n"
		mustWrite(t, root, rel, before)
		resp := callToolWithKey(t, server, 9801, key, "tickets.sage_gate", map[string]any{"stem": stem, "landing": "todo"})
		if !strings.Contains(resp, "actionable tickets run sage review at ready promotion") || strings.Contains(resp, "action: run") {
			t.Fatalf("todo did not fail loudly: %s", resp)
		}
		raw, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil || string(raw) != before {
			t.Fatalf("rejected call mutated body: %s %v", raw, err)
		}
	}
}
