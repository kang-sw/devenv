package main

import (
	"bytes"
	"encoding/json"
	"os/exec"
	"strings"
	"testing"
)

// mailbox_codex_hook_test.go covers the Codex Stop-hook adapter subcommand
// (260913-feat-cross-session-mailbox-wake Phase 2): a non-blocking,
// event-driven level check that emits Codex's `{"decision":"block",
// "reason":...}` Stop-hook response only when the baked-in --slug's named
// inbox has unread mail, and otherwise fails open/silent so a malformed
// payload, an unresolvable slug, or an empty --slug never breaks the
// harness's turn-conclude step.

func runMailboxCodexStopHook(t *testing.T, bin string, env []string, stdin string, args ...string) (string, int) {
	t.Helper()
	cmd := exec.Command(bin, append([]string{"mailbox", "codex-stop-hook"}, args...)...)
	cmd.Env = env
	cmd.Stdin = strings.NewReader(stdin)
	out, err := cmd.CombinedOutput()
	exitCode := 0
	if err != nil {
		exitErr, ok := err.(*exec.ExitError)
		if !ok {
			t.Fatalf("mailbox codex-stop-hook did not exit cleanly: %v\n%s", err, out)
		}
		exitCode = exitErr.ExitCode()
	}
	return string(out), exitCode
}

func TestMailboxCodexStopHookBlocksWhenNamedInboxHasUnreadMail(t *testing.T) {
	bin := buildWsMCPMailboxTestBin(t)
	env := mailboxTestEnv(t)
	seedNamedInbox(t, env, "lead", "owner-key", "run ticket X")

	out, exitCode := runMailboxCodexStopHook(t, bin, env, `{"hook_event_name":"Stop","stop_hook_active":false}`, "--slug", "lead@machine")
	if exitCode != 0 {
		t.Fatalf("mailbox codex-stop-hook exit code = %d, want 0\n%s", exitCode, out)
	}
	var got struct {
		Decision string `json:"decision"`
		Reason   string `json:"reason"`
	}
	if err := json.Unmarshal(bytes.TrimSpace([]byte(out)), &got); err != nil {
		t.Fatalf("invalid mailbox codex-stop-hook JSON: %v\n%s", err, out)
	}
	if got.Decision != "block" || got.Reason == "" {
		t.Fatalf("mailbox codex-stop-hook output = %#v, want decision=block with a non-empty reason", got)
	}
}

func TestMailboxCodexStopHookSilentWhenQueueEmpty(t *testing.T) {
	bin := buildWsMCPMailboxTestBin(t)
	env := mailboxTestEnv(t)

	out, exitCode := runMailboxCodexStopHook(t, bin, env, `{"hook_event_name":"Stop","stop_hook_active":false}`, "--slug", "lead@machine")
	if exitCode != 0 || strings.TrimSpace(out) != "" {
		t.Fatalf("mailbox codex-stop-hook on an empty queue = (exit %d, out %q), want (0, \"\")", exitCode, out)
	}
}

func TestMailboxCodexStopHookGuardsAgainstStopHookActiveLoop(t *testing.T) {
	bin := buildWsMCPMailboxTestBin(t)
	env := mailboxTestEnv(t)
	seedNamedInbox(t, env, "lead", "owner-key", "run ticket X")

	// stop_hook_active: true is the re-entry Stop from a prior block; this
	// must not block again even though the mail is still queued.
	out, exitCode := runMailboxCodexStopHook(t, bin, env, `{"hook_event_name":"Stop","stop_hook_active":true}`, "--slug", "lead@machine")
	if exitCode != 0 || strings.TrimSpace(out) != "" {
		t.Fatalf("mailbox codex-stop-hook with stop_hook_active=true = (exit %d, out %q), want (0, \"\") — loop guard must win over pending mail", exitCode, out)
	}
}

func TestMailboxCodexStopHookNoOpWithoutSlug(t *testing.T) {
	bin := buildWsMCPMailboxTestBin(t)
	env := mailboxTestEnv(t)

	out, exitCode := runMailboxCodexStopHook(t, bin, env, `{"hook_event_name":"Stop","stop_hook_active":false}`)
	if exitCode != 0 || strings.TrimSpace(out) != "" {
		t.Fatalf("mailbox codex-stop-hook with no --slug = (exit %d, out %q), want (0, \"\")", exitCode, out)
	}
}

func TestMailboxCodexStopHookFailsOpenOnMalformedPayload(t *testing.T) {
	bin := buildWsMCPMailboxTestBin(t)
	env := mailboxTestEnv(t)
	seedNamedInbox(t, env, "lead", "owner-key", "run ticket X")

	out, exitCode := runMailboxCodexStopHook(t, bin, env, `{not valid json`, "--slug", "lead@machine")
	if exitCode != 0 {
		t.Fatalf("mailbox codex-stop-hook on malformed stdin exit code = %d, want 0 (fail open)\n%s", exitCode, out)
	}
	if !strings.Contains(out, "warning") {
		t.Fatalf("mailbox codex-stop-hook malformed-payload output = %q, want a stderr warning", out)
	}
}

func TestMailboxCodexStopHookFailsOpenOnUnresolvableSlug(t *testing.T) {
	bin := buildWsMCPMailboxTestBin(t)
	env := mailboxTestEnv(t)

	out, exitCode := runMailboxCodexStopHook(t, bin, env, `{"hook_event_name":"Stop","stop_hook_active":false}`, "--slug", "not-a-slug")
	if exitCode != 0 {
		t.Fatalf("mailbox codex-stop-hook with an invalid --slug exit code = %d, want 0 (fail open)\n%s", exitCode, out)
	}
	if !strings.Contains(out, "warning") {
		t.Fatalf("mailbox codex-stop-hook invalid-slug output = %q, want a stderr warning", out)
	}
}

func TestMailboxCodexStopHookAcceptsEmptyStdin(t *testing.T) {
	bin := buildWsMCPMailboxTestBin(t)
	env := mailboxTestEnv(t)
	seedNamedInbox(t, env, "lead", "owner-key", "run ticket X")

	// A completely empty stdin (no JSON at all) must decode to
	// stop_hook_active=false rather than erroring, so it still blocks.
	out, exitCode := runMailboxCodexStopHook(t, bin, env, "", "--slug", "lead@machine")
	if exitCode != 0 {
		t.Fatalf("mailbox codex-stop-hook with empty stdin exit code = %d, want 0\n%s", exitCode, out)
	}
	if !strings.Contains(out, `"decision":"block"`) {
		t.Fatalf("mailbox codex-stop-hook with empty stdin output = %q, want a decision:block response", out)
	}
}
