package main

import (
	"bytes"
	"encoding/json"
	"os/exec"
	"strings"
	"testing"
)

// mailbox_claude_hook_test.go covers the Claude Stop-hook adapter subcommand
// (260913-feat-cross-session-mailbox-wake Phase 3): the same non-blocking,
// event-driven level check as the Codex adapter (mailbox_codex_hook_test.go),
// wired to Claude's own Stop hook payload shape and misfire guard instead of
// Codex's. Coverage that is identical in spirit to the Codex suite (env
// resolution, --slug override, empty-queue silence, the stop_hook_active
// loop guard, the watermark's repeat-firing bound, payload-cwd root
// resolution, --reason, fail-open on a malformed payload / unresolvable
// slug) is not re-derived from scratch here in prose; see that file's
// comments for the shared rationale. This file's own comments focus on what
// is different: hook_event_name/agent_id/agent_type gating, which Codex has
// no equivalent of.

func runMailboxClaudeStopHook(t *testing.T, bin string, env []string, stdin string, args ...string) (string, int) {
	t.Helper()
	return runMailboxClaudeStopHookInDir(t, bin, env, "", stdin, args...)
}

// runMailboxClaudeStopHookInDir mirrors runMailboxCodexStopHookInDir, needed
// by the same payload-cwd-vs-OS-cwd divergence test this file also carries
// for the Claude adapter.
func runMailboxClaudeStopHookInDir(t *testing.T, bin string, env []string, dir string, stdin string, args ...string) (string, int) {
	t.Helper()
	cmd := exec.Command(bin, append([]string{"mailbox", "claude-stop-hook"}, args...)...)
	cmd.Env = env
	cmd.Dir = dir
	cmd.Stdin = strings.NewReader(stdin)
	out, err := cmd.CombinedOutput()
	exitCode := 0
	if err != nil {
		exitErr, ok := err.(*exec.ExitError)
		if !ok {
			t.Fatalf("mailbox claude-stop-hook did not exit cleanly: %v\n%s", err, out)
		}
		exitCode = exitErr.ExitCode()
	}
	return string(out), exitCode
}

func TestMailboxClaudeStopHookBlocksWhenNamedInboxHasUnreadMail(t *testing.T) {
	bin := buildWsMCPMailboxTestBin(t)
	env := mailboxTestEnv(t)
	seedNamedInbox(t, env, "lead", "owner-key", "run ticket X")

	out, exitCode := runMailboxClaudeStopHook(t, bin, withMailboxEnv(env, "lead@machine"), `{"hook_event_name":"Stop","stop_hook_active":false}`)
	if exitCode != 0 {
		t.Fatalf("mailbox claude-stop-hook exit code = %d, want 0\n%s", exitCode, out)
	}
	var got struct {
		Decision string `json:"decision"`
		Reason   string `json:"reason"`
	}
	if err := json.Unmarshal(bytes.TrimSpace([]byte(out)), &got); err != nil {
		t.Fatalf("invalid mailbox claude-stop-hook JSON: %v\n%s", err, out)
	}
	if got.Decision != "block" || got.Reason == "" {
		t.Fatalf("mailbox claude-stop-hook output = %#v, want decision=block with a non-empty reason", got)
	}
}

func TestMailboxClaudeStopHookSlugFlagOverridesEnv(t *testing.T) {
	bin := buildWsMCPMailboxTestBin(t)
	env := mailboxTestEnv(t)
	seedNamedInbox(t, env, "lead", "owner-key", "run ticket X")

	out, exitCode := runMailboxClaudeStopHook(t, bin, withMailboxEnv(env, "someone-else@machine"), `{"hook_event_name":"Stop","stop_hook_active":false}`, "--slug", "lead@machine")
	if exitCode != 0 || !strings.Contains(out, `"decision":"block"`) {
		t.Fatalf("mailbox claude-stop-hook with --slug override = (exit %d, out %q), want a decision:block response", exitCode, out)
	}
}

func TestMailboxClaudeStopHookSilentWhenQueueEmpty(t *testing.T) {
	bin := buildWsMCPMailboxTestBin(t)
	env := mailboxTestEnv(t)

	out, exitCode := runMailboxClaudeStopHook(t, bin, withMailboxEnv(env, "lead@machine"), `{"hook_event_name":"Stop","stop_hook_active":false}`)
	if exitCode != 0 || strings.TrimSpace(out) != "" {
		t.Fatalf("mailbox claude-stop-hook on an empty queue = (exit %d, out %q), want (0, \"\")", exitCode, out)
	}
}

func TestMailboxClaudeStopHookGuardsAgainstStopHookActiveLoop(t *testing.T) {
	bin := buildWsMCPMailboxTestBin(t)
	env := mailboxTestEnv(t)
	seedNamedInbox(t, env, "lead", "owner-key", "run ticket X")

	out, exitCode := runMailboxClaudeStopHook(t, bin, withMailboxEnv(env, "lead@machine"), `{"hook_event_name":"Stop","stop_hook_active":true}`)
	if exitCode != 0 || strings.TrimSpace(out) != "" {
		t.Fatalf("mailbox claude-stop-hook with stop_hook_active=true = (exit %d, out %q), want (0, \"\") — loop guard must win over pending mail", exitCode, out)
	}
}

// TestMailboxClaudeStopHookGuardsAgainstSubagentAgentID is the Claude
// adapter's own misfire-guard test: unlike Codex (whose Stop payload has no
// classifier at all), Claude's payload can carry agent_id in a subagent
// context per the research ticket's docs-sourced probe. This must never
// block even though the mail is genuinely pending, because the arm-reminder
// must fire only in the owner/root turn.
func TestMailboxClaudeStopHookGuardsAgainstSubagentAgentID(t *testing.T) {
	bin := buildWsMCPMailboxTestBin(t)
	env := mailboxTestEnv(t)
	seedNamedInbox(t, env, "lead", "owner-key", "run ticket X")

	out, exitCode := runMailboxClaudeStopHook(t, bin, withMailboxEnv(env, "lead@machine"), `{"hook_event_name":"Stop","stop_hook_active":false,"agent_id":"subagent-123"}`)
	if exitCode != 0 || strings.TrimSpace(out) != "" {
		t.Fatalf("mailbox claude-stop-hook with agent_id present = (exit %d, out %q), want (0, \"\") — a subagent-context Stop must never fire the owner arm-reminder", exitCode, out)
	}
}

// TestMailboxClaudeStopHookGuardsAgainstSubagentAgentType mirrors the
// agent_id guard test above for the agent_type field, since the probe found
// either field can signal subagent context.
func TestMailboxClaudeStopHookGuardsAgainstSubagentAgentType(t *testing.T) {
	bin := buildWsMCPMailboxTestBin(t)
	env := mailboxTestEnv(t)
	seedNamedInbox(t, env, "lead", "owner-key", "run ticket X")

	out, exitCode := runMailboxClaudeStopHook(t, bin, withMailboxEnv(env, "lead@machine"), `{"hook_event_name":"Stop","stop_hook_active":false,"agent_type":"explore"}`)
	if exitCode != 0 || strings.TrimSpace(out) != "" {
		t.Fatalf("mailbox claude-stop-hook with agent_type present = (exit %d, out %q), want (0, \"\") — a subagent-context Stop must never fire the owner arm-reminder", exitCode, out)
	}
}

// TestMailboxClaudeStopHookGuardsAgainstSubagentStopEventName covers the
// hook_event_name misfire-guard layer: even though hooks/hooks.json registers
// this command only under "Stop", a payload that explicitly names a
// different event must not be treated as a root-turn Stop.
func TestMailboxClaudeStopHookGuardsAgainstSubagentStopEventName(t *testing.T) {
	bin := buildWsMCPMailboxTestBin(t)
	env := mailboxTestEnv(t)
	seedNamedInbox(t, env, "lead", "owner-key", "run ticket X")

	out, exitCode := runMailboxClaudeStopHook(t, bin, withMailboxEnv(env, "lead@machine"), `{"hook_event_name":"SubagentStop","stop_hook_active":false}`)
	if exitCode != 0 || strings.TrimSpace(out) != "" {
		t.Fatalf("mailbox claude-stop-hook with hook_event_name=SubagentStop = (exit %d, out %q), want (0, \"\")", exitCode, out)
	}
}

// TestMailboxClaudeStopHookBoundsRepeatedFiringAtSameCount is the CLI-level
// counterpart of internal/wsmailbox's watermark unit tests, mirroring the
// Codex adapter's own regression test: driving the actual subcommand twice
// against the same unread queue must notify once, then go silent.
func TestMailboxClaudeStopHookBoundsRepeatedFiringAtSameCount(t *testing.T) {
	bin := buildWsMCPMailboxTestBin(t)
	env := mailboxTestEnv(t)
	seedNamedInbox(t, env, "lead", "owner-key", "run ticket X")
	hookEnv := withMailboxEnv(env, "lead@machine")

	out1, exit1 := runMailboxClaudeStopHook(t, bin, hookEnv, `{"hook_event_name":"Stop","stop_hook_active":false}`)
	if exit1 != 0 || !strings.Contains(out1, `"decision":"block"`) {
		t.Fatalf("first firing = (exit %d, out %q), want a decision:block response", exit1, out1)
	}

	out2, exit2 := runMailboxClaudeStopHook(t, bin, hookEnv, `{"hook_event_name":"Stop","stop_hook_active":false}`)
	if exit2 != 0 || strings.TrimSpace(out2) != "" {
		t.Fatalf("second firing (same unread count) = (exit %d, out %q), want (0, \"\") — the watermark must suppress a repeat notify", exit2, out2)
	}
}

func TestMailboxClaudeStopHookNoOpWithoutMailboxIdentity(t *testing.T) {
	bin := buildWsMCPMailboxTestBin(t)
	env := mailboxTestEnv(t)

	out, exitCode := runMailboxClaudeStopHook(t, bin, env, `{"hook_event_name":"Stop","stop_hook_active":false}`)
	if exitCode != 0 || strings.TrimSpace(out) != "" {
		t.Fatalf("mailbox claude-stop-hook with no WS_MAILBOX and no --slug = (exit %d, out %q), want (0, \"\")", exitCode, out)
	}
}

func TestMailboxClaudeStopHookFailsOpenOnMalformedPayload(t *testing.T) {
	bin := buildWsMCPMailboxTestBin(t)
	env := mailboxTestEnv(t)
	seedNamedInbox(t, env, "lead", "owner-key", "run ticket X")

	out, exitCode := runMailboxClaudeStopHook(t, bin, withMailboxEnv(env, "lead@machine"), `{not valid json`)
	if exitCode != 0 {
		t.Fatalf("mailbox claude-stop-hook on malformed stdin exit code = %d, want 0 (fail open)\n%s", exitCode, out)
	}
	if !strings.Contains(out, "warning") {
		t.Fatalf("mailbox claude-stop-hook malformed-payload output = %q, want a stderr warning", out)
	}
}

func TestMailboxClaudeStopHookFailsOpenOnUnresolvableSlug(t *testing.T) {
	bin := buildWsMCPMailboxTestBin(t)
	env := mailboxTestEnv(t)

	out, exitCode := runMailboxClaudeStopHook(t, bin, env, `{"hook_event_name":"Stop","stop_hook_active":false}`, "--slug", "not-a-slug")
	if exitCode != 0 {
		t.Fatalf("mailbox claude-stop-hook with an invalid --slug exit code = %d, want 0 (fail open)\n%s", exitCode, out)
	}
	if !strings.Contains(out, "warning") {
		t.Fatalf("mailbox claude-stop-hook invalid-slug output = %q, want a stderr warning", out)
	}
}

func TestMailboxClaudeStopHookAcceptsEmptyStdin(t *testing.T) {
	bin := buildWsMCPMailboxTestBin(t)
	env := mailboxTestEnv(t)
	seedNamedInbox(t, env, "lead", "owner-key", "run ticket X")

	// A completely empty stdin (no JSON at all) must decode to
	// stop_hook_active=false and every classifier field empty, so it still
	// blocks rather than erroring.
	out, exitCode := runMailboxClaudeStopHook(t, bin, withMailboxEnv(env, "lead@machine"), "")
	if exitCode != 0 {
		t.Fatalf("mailbox claude-stop-hook with empty stdin exit code = %d, want 0\n%s", exitCode, out)
	}
	if !strings.Contains(out, `"decision":"block"`) {
		t.Fatalf("mailbox claude-stop-hook with empty stdin output = %q, want a decision:block response", out)
	}
}

// TestMailboxClaudeStopHookUsesPayloadCwdForWorktreeSlug mirrors the Codex
// adapter's own payload-cwd test: --root's default must resolve against the
// Stop payload's own cwd field, not the hook subprocess's OS-level cwd.
func TestMailboxClaudeStopHookUsesPayloadCwdForWorktreeSlug(t *testing.T) {
	bin := buildWsMCPMailboxTestBin(t)
	env := mailboxTestEnv(t)

	mailFixture := initGitFixtureForHookTest(t)       // has unread "lead@worktree" mail
	processCwdFixture := initGitFixtureForHookTest(t) // deliberately does not
	seedWorktreeInbox(t, env, mailFixture, "lead", "owner-key", "run ticket X")

	payload := `{"hook_event_name":"Stop","stop_hook_active":false,"cwd":"` + strings.ReplaceAll(mailFixture, `\`, `\\`) + `"}`
	out, exitCode := runMailboxClaudeStopHookInDir(t, bin, withMailboxEnv(env, "lead@worktree"), processCwdFixture, payload)
	if exitCode != 0 || !strings.Contains(out, `"decision":"block"`) {
		t.Fatalf("mailbox claude-stop-hook with payload.cwd pointing at the seeded fixture = (exit %d, out %q), want a decision:block response — --root's default must resolve against payload.cwd, not the OS process cwd", exitCode, out)
	}

	noCwdPayload := `{"hook_event_name":"Stop","stop_hook_active":false}`
	out2, exit2 := runMailboxClaudeStopHookInDir(t, bin, withMailboxEnv(env, "lead@worktree"), processCwdFixture, noCwdPayload)
	if exit2 != 0 || strings.TrimSpace(out2) != "" {
		t.Fatalf("mailbox claude-stop-hook without payload.cwd, OS cwd at the unseeded fixture = (exit %d, out %q), want a clean (0, \"\")", exit2, out2)
	}
}

func TestMailboxClaudeStopHookHonorsExplicitReason(t *testing.T) {
	bin := buildWsMCPMailboxTestBin(t)
	env := mailboxTestEnv(t)
	seedNamedInbox(t, env, "lead", "owner-key", "run ticket X")

	const customReason = "custom drain instruction for this test"
	out, exitCode := runMailboxClaudeStopHook(t, bin, withMailboxEnv(env, "lead@machine"), `{"hook_event_name":"Stop","stop_hook_active":false}`, "--reason", customReason)
	if exitCode != 0 {
		t.Fatalf("mailbox claude-stop-hook --reason exit code = %d, want 0\n%s", exitCode, out)
	}
	var got struct {
		Decision string `json:"decision"`
		Reason   string `json:"reason"`
	}
	if err := json.Unmarshal(bytes.TrimSpace([]byte(out)), &got); err != nil {
		t.Fatalf("invalid mailbox claude-stop-hook JSON: %v\n%s", err, out)
	}
	if got.Decision != "block" || got.Reason != customReason {
		t.Fatalf("mailbox claude-stop-hook --reason output = %#v, want decision=block with reason=%q", got, customReason)
	}
}
