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
	askOut := formatSageGate(wsdoc.SageGateResult{Action: "ask", AskPrompt: "Run design review for this ticket?"})
	if !strings.Contains(askOut, "ask_prompt: Run design review for this ticket?") || !strings.Contains(askOut, "answer=yes|no") {
		t.Fatalf("formatSageGate ask output:\n%s", askOut)
	}

	// D1/D3/D4: no gate action may propose a commit for the posture flip, and
	// every action must describe the uncommitted write the same way. A
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
		if !strings.Contains(text, sageGatePostureUncommittedNote) {
			t.Fatalf("formatSageGate %s missing the shared uncommitted-posture note:\n%s", name, text)
		}
	}

	// The advisory line renders as a capitalized sentence (C9), matching its
	// sibling fields (ask_prompt/next_instruction), even though the shared
	// sageReviewNonWaivableAdvisory constant itself stays lowercase-initial
	// for its other embedding inside the mutation-time Tip warning.
	runOut := formatSageGate(wsdoc.SageGateResult{
		Action:    "run",
		Reviewers: []string{"design"},
		Mode:      "standalone",
		Advisory:  "sage review is not waivable per ticket (see ws/config.list for the sage_review config); design review checks coherence.",
	})
	if !strings.Contains(runOut, "advisory: Sage review is not waivable") {
		t.Fatalf("formatSageGate advisory must be capitalized:\n%s", runOut)
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
	if !strings.Contains(concernOut, "verdict: concern") || !strings.Contains(concernOut, "escalate to block manually") {
		t.Fatalf("formatSageRecord concern output:\n%s", concernOut)
	}
	if !strings.Contains(concernOut, "Recorded but not committed") || !strings.Contains(concernOut, "ws/git.commit") {
		t.Fatalf("formatSageRecord concern output should route to ws/git.commit:\n%s", concernOut)
	}

	// pass surfaces the same no-commit routing.
	passOut := formatSageRecord(wsdoc.SageRecordResult{
		Verdict: "pass",
		Posture: map[string]string{"sage-review-design": "completed"},
	})
	if !strings.Contains(passOut, "recorded but not committed") || !strings.Contains(passOut, "ws/git.commit") {
		t.Fatalf("formatSageRecord pass output should route to ws/git.commit:\n%s", passOut)
	}
	// A clean pass carries no issues, so it must not grow a routing clause.
	if strings.Contains(passOut, "autonomous_issues:") || strings.Contains(passOut, "Open Decision Queue") {
		t.Fatalf("formatSageRecord clean pass must stay terse:\n%s", passOut)
	}
}

// TestFormatSageGateBlockedRecovery pins that the gate's stop_blocked branch
// names its recovery route rather than reading as a dead end. Without it, a
// re-entry whose edits address the blocker got a bare "stop" from the gate while
// lead-write-ticket's On: Reviewer Spawn told it to review the blocked stage —
// one condition described two ways, which is what forced a judgement call
// mid-procedure during dogfooding.
func TestFormatSageGateBlockedRecovery(t *testing.T) {
	out := formatSageGate(wsdoc.SageGateResult{Action: "stop_blocked"})
	if !strings.Contains(out, "stop and report the blocker") {
		t.Fatalf("stop_blocked must still tell the caller to stop:\n%s", out)
	}
	for _, want := range []string{"On: Reviewer Spawn", "ws/tickets.sage_stamp", "sage-review-* frontmatter"} {
		if !strings.Contains(out, want) {
			t.Fatalf("stop_blocked must name its recovery route (missing %q):\n%s", want, out)
		}
	}
	// The gate carries no stage field, so the text must not imply the caller can
	// read the blocked stage off this result.
	if strings.Contains(out, "reviewers:") {
		t.Fatalf("stop_blocked must not render a reviewers line:\n%s", out)
	}
}

// TestFormatSageRecordBlockRecovery pins the block branch's recovery route. It
// previously emitted the pass text ("commit, then proceed to handoff"), which
// silently dropped a block at a todo/ landing: the caller's only block branch
// covers the ready/ landing.
func TestFormatSageRecordBlockRecovery(t *testing.T) {
	out := formatSageRecord(wsdoc.SageRecordResult{
		Verdict:        "block",
		Posture:        map[string]string{"sage-review-design": "blocked"},
		BlockedSection: "## Blocked (2026-07-29)",
		Missing:        1,
	})
	if !strings.Contains(out, "stop and report the blocker") {
		t.Fatalf("formatSageRecord block output must tell the caller to stop:\n%s", out)
	}
	// The ready/ landing reverts instead of committing, so the block branch must
	// not prescribe a commit the caller may have to skip.
	if strings.Contains(out, "commit this change via ws/git.commit") {
		t.Fatalf("formatSageRecord block output must leave the commit decision to the caller:\n%s", out)
	}
	// resolveStage returns stop_blocked for a blocked posture forever, so
	// sage_gate never names a reviewer again. The one executable recovery is the
	// sage_stamp route this branch already names; an in-place resolve loop is
	// not, and a landing-specific direction is the caller's own ready/ branch to
	// make.
	for _, forbidden := range []string{"resolve the issues in the appended Blocked section", "Do not land this ticket in ready/"} {
		if strings.Contains(out, forbidden) {
			t.Fatalf("formatSageRecord block output must not prescribe an unexecutable recovery loop or a landing-specific action (found %q):\n%s", forbidden, out)
		}
	}
}

// TestFormatSageRecordConcernWithoutMissing pins the gate on the
// missing-decision sentence. A standalone stage records `concern` straight from
// the reviewer verdict with no missing issue required, so the ungated text asked
// the caller to weigh a decision the same message reported as absent.
func TestFormatSageRecordConcernWithoutMissing(t *testing.T) {
	out := formatSageRecord(wsdoc.SageRecordResult{
		Verdict:    "concern",
		Posture:    map[string]string{"sage-review-design": "completed"},
		Autonomous: 2,
	})
	if strings.Contains(out, "missing decision") {
		t.Fatalf("concern with no missing issue must not mention a missing decision:\n%s", out)
	}
	if !strings.Contains(out, "Fix the 2 autonomous issue(s)") {
		t.Fatalf("concern must still route its autonomous issues:\n%s", out)
	}
}

// TestFormatSageRecordRoutingPrecedesCommit pins the clause order: fixes are
// stated before the commit direction, not after it.
func TestFormatSageRecordRoutingPrecedesCommit(t *testing.T) {
	out := formatSageRecord(wsdoc.SageRecordResult{
		Verdict:    "pass",
		Posture:    map[string]string{"sage-review-design": "completed"},
		Autonomous: 1,
	})
	fixIdx := strings.Index(out, "Fix the 1 autonomous issue(s)")
	commitIdx := strings.Index(out, "commit this change via ws/git.commit")
	if fixIdx == -1 || commitIdx == -1 {
		t.Fatalf("formatSageRecord pass-with-issues output:\n%s", out)
	}
	if fixIdx > commitIdx {
		t.Fatalf("issue routing (offset %d) must precede the commit direction (offset %d):\n%s", fixIdx, commitIdx, out)
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
	for _, want := range []string{"autonomous_issues: 2", "missing_issues: 1", "fix the 2 autonomous issue(s) in the ticket yourself", "take the 1 missing issue(s) through the Open Decision Queue"} {
		if !strings.Contains(both, want) {
			t.Fatalf("formatSageRecord mixed routing missing %q in:\n%s", want, both)
		}
	}

	autonomousOnly := formatSageRecord(wsdoc.SageRecordResult{
		Verdict:    "pass",
		Posture:    map[string]string{"sage-review-completeness": "completed"},
		Autonomous: 3,
	})
	if !strings.Contains(autonomousOnly, "none of them need a user decision") {
		t.Fatalf("formatSageRecord autonomous-only routing:\n%s", autonomousOnly)
	}
	if strings.Contains(autonomousOnly, "Open Decision Queue") {
		t.Fatalf("formatSageRecord must not route autonomous-only issues to the user:\n%s", autonomousOnly)
	}

	missingOnly := formatSageRecord(wsdoc.SageRecordResult{
		Verdict: "concern",
		Posture: map[string]string{"sage-review-design": "completed"},
		Missing: 1,
	})
	if !strings.Contains(missingOnly, "Take the 1 missing issue(s) through the Open Decision Queue") {
		t.Fatalf("formatSageRecord missing-only routing:\n%s", missingOnly)
	}
}

func TestServeStdioSageGateDispatch(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	mustWrite(t, root, filepath.Join("ai-docs", "tickets", "todo", "260101-feat-sg.md"),
		"---\ntitle: Sage\nsage-review-design: required\n---\n\nBody.\n")
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))

	server := NewServer(root, "test")
	key, _ := parseLoginResponse(t, callLogin(t, server, 9701, root, nil))

	resp := callToolWithKey(t, server, 9702, key, "tickets.sage_gate", map[string]any{
		"stem":    "260101-feat-sg",
		"landing": "todo",
	})
	if !strings.Contains(resp, "action: run") || !strings.Contains(resp, "reviewers: design") || !strings.Contains(resp, "mode: standalone") {
		t.Fatalf("sage_gate dispatch response:\n%s", resp)
	}
	// C5: the required->run advisory (non-waivable statement + review-scope
	// line) must reach the actual MCP dispatch response, not just the
	// wsdoc.SageGateResult struct field.
	if !strings.Contains(resp, "advisory: Sage review is not waivable") {
		t.Fatalf("sage_gate dispatch response missing advisory line:\n%s", resp)
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
		"---\ntitle: Sage\nsage-review-design: recommended\n---\n\nBody.\n")
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
		"landing": "todo",
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
	if !strings.Contains(string(body), "sage-review-design: skipped") {
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
		"---\ntitle: Sage\nsage-review-design: required\n---\n\nBody.\n")
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
		"---\ntitle: Sage\nsage-review-design: required\n---\n\nBody.\n")
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
