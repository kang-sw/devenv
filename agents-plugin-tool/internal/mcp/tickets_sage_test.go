package mcp

import (
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
	// run action with a commit (ask-decline path that also runs the other stage).
	out := formatSageGate(wsdoc.SageGateResult{
		Action:      "run",
		Reviewers:   []string{"design", "completeness"},
		Mode:        "combined",
		CommitTitle: "chore(sage): skip completeness review",
	}, "abc123")
	for _, want := range []string{"action: run", "reviewers: design, completeness", "mode: combined", "commit: abc123", "next_instruction:", "stage=combined"} {
		if !strings.Contains(out, want) {
			t.Fatalf("formatSageGate missing %q in:\n%s", want, out)
		}
	}

	// ask action carries the prompt and the relay instruction.
	askOut := formatSageGate(wsdoc.SageGateResult{Action: "ask", AskPrompt: "Run design review for this ticket?"}, "")
	if !strings.Contains(askOut, "ask_prompt: Run design review for this ticket?") || !strings.Contains(askOut, "answer=yes|no") {
		t.Fatalf("formatSageGate ask output:\n%s", askOut)
	}
}

func TestFormatSageRecordRoundTrip(t *testing.T) {
	out := formatSageRecord(wsdoc.SageRecordResult{
		Verdict:        "block",
		Posture:        map[string]string{"sage-review-design": "blocked", "sage-review-completeness": "blocked"},
		BlockedSection: "## Blocked (2026-07-20)",
		CommitTitle:    "docs(sage): block ticket on sage review",
	}, "deadbeef")
	for _, want := range []string{"verdict: block", "sage-review-design: blocked", "sage-review-completeness: blocked", "blocked_section: appended", "commit: deadbeef"} {
		if !strings.Contains(out, want) {
			t.Fatalf("formatSageRecord missing %q in:\n%s", want, out)
		}
	}

	// concern surfaces the manual-escalation instruction.
	concernOut := formatSageRecord(wsdoc.SageRecordResult{
		Verdict:     "concern",
		Posture:     map[string]string{"sage-review-design": "completed"},
		CommitTitle: "docs(sage): mark sage review completed",
	}, "cafe")
	if !strings.Contains(concernOut, "verdict: concern") || !strings.Contains(concernOut, "escalate to block manually") {
		t.Fatalf("formatSageRecord concern output:\n%s", concernOut)
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
}

func TestServeStdioSageRecordDispatch(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	mustWrite(t, root, filepath.Join("ai-docs", "tickets", "todo", "260101-feat-sr.md"),
		"---\ntitle: Sage\nsage-review-design: required\n---\n\nBody.\n")
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))

	server := NewServer(root, "test")
	key, _ := parseLoginResponse(t, callLogin(t, server, 9801, root, nil))

	resp := callToolWithKey(t, server, 9802, key, "tickets.sage_record", map[string]any{
		"stem":  "260101-feat-sr",
		"stage": "design",
		"verdicts": []any{
			map[string]any{"reviewer": "design", "verdict": "pass"},
		},
	})
	if !strings.Contains(resp, "verdict: pass") || !strings.Contains(resp, "sage-review-design: completed") || !strings.Contains(resp, "commit:") {
		t.Fatalf("sage_record dispatch response:\n%s", resp)
	}

	// Missing verdict for the requested stage must surface as a tool error, not a
	// silent completed write.
	errResp := callToolWithKey(t, server, 9803, key, "tickets.sage_record", map[string]any{
		"stem":     "260101-feat-sr",
		"stage":    "combined",
		"verdicts": []any{map[string]any{"reviewer": "design", "verdict": "pass"}},
	})
	if !strings.Contains(errResp, "both design and completeness") {
		t.Fatalf("expected missing-verdict error, got:\n%s", errResp)
	}
}
