package mcp

import (
	"bytes"
	"strings"
	"testing"
)

// TestTicketsVerifyGitCommitCallSiteParityBlocksInvalidTicket is the ticket's
// explicit acceptance check: the standalone tickets.verify call and the
// git.commit commit-gate must return the same verdict (same guardrail, same
// path) for an identical invalid ticket fixture, and the blocked commit must
// never land.
func TestTicketsVerifyGitCommitCallSiteParityBlocksInvalidTicket(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	mustWrite(t, root, "keep.txt", "placeholder\n")
	runGit(t, root, "add", "keep.txt")
	runGit(t, root, "commit", "-m", "initial")
	initialHead := strings.TrimSpace(string(runGitOutput(t, root, "rev-parse", "HEAD")))

	badPath := "ai-docs/tickets/todo/not-a-valid-stem.md"
	mustWrite(t, root, badPath, "---\ntitle: Bad stem\n---\n\nBody.\n")

	server := NewServer(root, "test")
	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"tickets.verify","arguments":{"paths":["` + badPath + `"]}}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"git.commit","arguments":{"paths":["` + badPath + `"],"title":"test: blocked commit","ai_context":["User intent: prove the commit gate matches tickets.verify."]}}}`,
	}, "\n") + "\n"

	var out bytes.Buffer
	if err := serveStdioWithSession(t, server, root, input, &out); err != nil {
		t.Fatalf("ServeStdio returned error: %v", err)
	}
	byID := responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))

	verifyText := toolText(t, byID["1"])
	if !strings.Contains(verifyText, "verify: FAIL") {
		t.Fatalf("tickets.verify text = %q, want a FAIL summary", verifyText)
	}
	if !strings.Contains(verifyText, "[stem]") || !strings.Contains(verifyText, badPath) {
		t.Fatalf("tickets.verify text = %q, want a stem guardrail finding for %s", verifyText, badPath)
	}

	if !toolIsError(t, byID["2"]) {
		t.Fatalf("git.commit was not blocked by the same invalid ticket: %s", byID["2"])
	}
	commitText := toolText(t, byID["2"])
	if !strings.Contains(commitText, "[stem]") || !strings.Contains(commitText, badPath) {
		t.Fatalf("git.commit blocked text = %q, want the same stem finding tickets.verify reported", commitText)
	}

	// The blocked commit must never have landed.
	headAfter := strings.TrimSpace(string(runGitOutput(t, root, "rev-parse", "HEAD")))
	if headAfter != initialHead {
		t.Fatalf("HEAD moved to %s despite a blocked commit (want unchanged %s)", headAfter, initialHead)
	}
}

// TestTicketsVerifyGitCommitCallSiteParityAllowsValidTicket is the passing
// mirror of the parity check above: a ticket that tickets.verify reports as
// PASS must also commit cleanly through git.commit, and a ready ticket
// missing only spec addressing must warn (not block) in both.
func TestTicketsVerifyGitCommitCallSiteParityAllowsValidTicket(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)

	goodPath := "ai-docs/tickets/todo/260723-feat-valid-parity.md"
	mustWrite(t, root, goodPath, "---\ntitle: Valid ticket\n---\n\nBody.\n")

	server := NewServer(root, "test")
	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"tickets.verify","arguments":{"paths":["` + goodPath + `"]}}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"git.commit","arguments":{"paths":["` + goodPath + `"],"title":"test: parity pass","ai_context":["User intent: prove a passing verify never blocks the commit."]}}}`,
	}, "\n") + "\n"

	var out bytes.Buffer
	if err := serveStdioWithSession(t, server, root, input, &out); err != nil {
		t.Fatalf("ServeStdio returned error: %v", err)
	}
	byID := responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))

	verifyText := toolText(t, byID["1"])
	if !strings.Contains(verifyText, "verify: PASS") {
		t.Fatalf("tickets.verify text = %q, want PASS", verifyText)
	}

	if toolIsError(t, byID["2"]) {
		t.Fatalf("git.commit unexpectedly blocked a passing-verify ticket: %s", byID["2"])
	}
	commitText := toolText(t, byID["2"])
	if !strings.Contains(commitText, "commit: ") {
		t.Fatalf("git.commit text = %q, want a successful commit summary", commitText)
	}
}

// TestTicketsVerifySpecAddressWarningDoesNotBlockCommit confirms the
// ticket's spec-address soft-warn posture end to end: a ready/ ticket with a
// terminal sage-review posture but no spec addressing must be reported only
// as a tickets.verify warning, and git.commit must still succeed.
func TestTicketsVerifySpecAddressWarningDoesNotBlockCommit(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)

	path := "ai-docs/tickets/ready/260723-feat-no-spec-parity.md"
	body := "---\n" +
		"title: No spec addressing\n" +
		"sage-review-design: completed\n" +
		"sage-review-completeness: completed\n" +
		"---\n\nBody.\n"
	mustWrite(t, root, path, body)

	server := NewServer(root, "test")
	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"tickets.verify","arguments":{"paths":["` + path + `"]}}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"git.commit","arguments":{"paths":["` + path + `"],"title":"test: spec-address warns only","ai_context":["User intent: prove spec-address never blocks a commit."]}}}`,
	}, "\n") + "\n"

	var out bytes.Buffer
	if err := serveStdioWithSession(t, server, root, input, &out); err != nil {
		t.Fatalf("ServeStdio returned error: %v", err)
	}
	byID := responseLinesByID(t, strings.Split(strings.TrimSpace(out.String()), "\n"))

	verifyText := toolText(t, byID["1"])
	if !strings.Contains(verifyText, "verify: PASS") {
		t.Fatalf("tickets.verify text = %q, want PASS (spec-address is a warning, not a finding)", verifyText)
	}
	if !strings.Contains(verifyText, "WARN [spec-address]") {
		t.Fatalf("tickets.verify text = %q, want a spec-address warning line", verifyText)
	}

	if toolIsError(t, byID["2"]) {
		t.Fatalf("git.commit blocked a ticket whose only problem is the soft-warn spec-address guardrail: %s", byID["2"])
	}
}
