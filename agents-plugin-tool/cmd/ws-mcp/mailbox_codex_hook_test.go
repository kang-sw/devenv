package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// repoRootForTest returns this test binary's own working-tree root: a real
// git checkout, used only as a WorktreePath-resolvable root for the
// worktree-scope payload-cwd test below. It does not need to be the
// canonical top of the devenv repo — wsstate.Manager.Ensure resolves the
// canonical worktree root from any path inside it.
func repoRootForTest(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd: %v", err)
	}
	return dir
}

// mailbox_codex_hook_test.go covers the Codex Stop-hook adapter subcommand
// (260913-feat-cross-session-mailbox-wake Phase 2): a non-blocking,
// event-driven level check that emits Codex's `{"decision":"block",
// "reason":...}` Stop-hook response only when the queue's length changed
// since the last such notification (internal/wsmailbox's
// ShouldNotifyNamedInboxUnread), and otherwise fails open/silent so a
// malformed payload, an unresolvable slug, or no WS_MAILBOX at all never
// breaks the harness's turn-conclude step.
//
// The shipped hook never passes --slug; it reads WS_MAILBOX from its own
// environment (round-1 review finding: interpolating an untrusted env var
// into the hooks.json command string was an injection surface). These
// tests exercise that same env path via cmd.Env, mirroring how the real
// hook subprocess inherits it from the harness process tree.

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

// withMailboxEnv appends WS_MAILBOX=slug to env, the same environment
// inheritance path the real Stop hook subprocess relies on.
func withMailboxEnv(env []string, slug string) []string {
	return append(append([]string{}, env...), "WS_MAILBOX="+slug)
}

func TestMailboxCodexStopHookBlocksWhenNamedInboxHasUnreadMail(t *testing.T) {
	bin := buildWsMCPMailboxTestBin(t)
	env := mailboxTestEnv(t)
	seedNamedInbox(t, env, "lead", "owner-key", "run ticket X")

	out, exitCode := runMailboxCodexStopHook(t, bin, withMailboxEnv(env, "lead@machine"), `{"hook_event_name":"Stop","stop_hook_active":false}`)
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

func TestMailboxCodexStopHookSlugFlagOverridesEnv(t *testing.T) {
	bin := buildWsMCPMailboxTestBin(t)
	env := mailboxTestEnv(t)
	seedNamedInbox(t, env, "lead", "owner-key", "run ticket X")

	// WS_MAILBOX points at an unseeded slug; --slug overrides it to the
	// seeded one, confirming the explicit override still works for direct
	// invocation/testing even though the shipped hook never passes it.
	out, exitCode := runMailboxCodexStopHook(t, bin, withMailboxEnv(env, "someone-else@machine"), `{"hook_event_name":"Stop","stop_hook_active":false}`, "--slug", "lead@machine")
	if exitCode != 0 || !strings.Contains(out, `"decision":"block"`) {
		t.Fatalf("mailbox codex-stop-hook with --slug override = (exit %d, out %q), want a decision:block response", exitCode, out)
	}
}

func TestMailboxCodexStopHookSilentWhenQueueEmpty(t *testing.T) {
	bin := buildWsMCPMailboxTestBin(t)
	env := mailboxTestEnv(t)

	out, exitCode := runMailboxCodexStopHook(t, bin, withMailboxEnv(env, "lead@machine"), `{"hook_event_name":"Stop","stop_hook_active":false}`)
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
	out, exitCode := runMailboxCodexStopHook(t, bin, withMailboxEnv(env, "lead@machine"), `{"hook_event_name":"Stop","stop_hook_active":true}`)
	if exitCode != 0 || strings.TrimSpace(out) != "" {
		t.Fatalf("mailbox codex-stop-hook with stop_hook_active=true = (exit %d, out %q), want (0, \"\") — loop guard must win over pending mail", exitCode, out)
	}
}

// TestMailboxCodexStopHookBoundsRepeatedFiringAtSameCount is the CLI-level
// counterpart of internal/wsmailbox's watermark unit tests: driving the
// actual subcommand twice against the same unread queue must notify once,
// then go silent, confirming the CLI wiring (not just the package function)
// carries the round-1 "unbounded re-block" fix end to end.
func TestMailboxCodexStopHookBoundsRepeatedFiringAtSameCount(t *testing.T) {
	bin := buildWsMCPMailboxTestBin(t)
	env := mailboxTestEnv(t)
	seedNamedInbox(t, env, "lead", "owner-key", "run ticket X")
	hookEnv := withMailboxEnv(env, "lead@machine")

	out1, exit1 := runMailboxCodexStopHook(t, bin, hookEnv, `{"hook_event_name":"Stop","stop_hook_active":false}`)
	if exit1 != 0 || !strings.Contains(out1, `"decision":"block"`) {
		t.Fatalf("first firing = (exit %d, out %q), want a decision:block response", exit1, out1)
	}

	out2, exit2 := runMailboxCodexStopHook(t, bin, hookEnv, `{"hook_event_name":"Stop","stop_hook_active":false}`)
	if exit2 != 0 || strings.TrimSpace(out2) != "" {
		t.Fatalf("second firing (same unread count) = (exit %d, out %q), want (0, \"\") — the watermark must suppress a repeat notify", exit2, out2)
	}
}

func TestMailboxCodexStopHookNoOpWithoutMailboxIdentity(t *testing.T) {
	bin := buildWsMCPMailboxTestBin(t)
	env := mailboxTestEnv(t)

	out, exitCode := runMailboxCodexStopHook(t, bin, env, `{"hook_event_name":"Stop","stop_hook_active":false}`)
	if exitCode != 0 || strings.TrimSpace(out) != "" {
		t.Fatalf("mailbox codex-stop-hook with no WS_MAILBOX and no --slug = (exit %d, out %q), want (0, \"\")", exitCode, out)
	}
}

func TestMailboxCodexStopHookFailsOpenOnMalformedPayload(t *testing.T) {
	bin := buildWsMCPMailboxTestBin(t)
	env := mailboxTestEnv(t)
	seedNamedInbox(t, env, "lead", "owner-key", "run ticket X")

	out, exitCode := runMailboxCodexStopHook(t, bin, withMailboxEnv(env, "lead@machine"), `{not valid json`)
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
	out, exitCode := runMailboxCodexStopHook(t, bin, withMailboxEnv(env, "lead@machine"), "")
	if exitCode != 0 {
		t.Fatalf("mailbox codex-stop-hook with empty stdin exit code = %d, want 0\n%s", exitCode, out)
	}
	if !strings.Contains(out, `"decision":"block"`) {
		t.Fatalf("mailbox codex-stop-hook with empty stdin output = %q, want a decision:block response", out)
	}
}

// TestMailboxCodexStopHookUsesPayloadCwdForWorktreeSlug covers the round-1
// correctness finding that the Stop payload's own cwd (the only reliable
// signal of the actual session's working directory a bare hook subprocess
// has) was parsed but never used, leaving --root at "." for a
// worktree/clone-scope slug. Machine scope does not need root, so this test
// only confirms cwd flows into the CLI's effective root computation by
// checking a worktree-scope slug resolves without error against a real
// repo checkout at the payload's cwd rather than failing to resolve.
func TestMailboxCodexStopHookUsesPayloadCwdForWorktreeSlug(t *testing.T) {
	bin := buildWsMCPMailboxTestBin(t)
	env := mailboxTestEnv(t)

	// repoRoot: this package's own repo checkout, a valid git worktree root.
	repoRoot := repoRootForTest(t)

	payload := `{"hook_event_name":"Stop","stop_hook_active":false,"cwd":"` + strings.ReplaceAll(repoRoot, `\`, `\\`) + `"}`
	out, exitCode := runMailboxCodexStopHook(t, bin, withMailboxEnv(env, "lead@worktree"), payload)
	// No mail seeded for this worktree-scope slug: expect a clean silent
	// no-op (exit 0, no output), not a resolution error surfaced as a
	// warning — proving cwd resolved the worktree store path successfully.
	if exitCode != 0 || strings.Contains(out, "warning") {
		t.Fatalf("mailbox codex-stop-hook with a worktree-scope slug and payload cwd = (exit %d, out %q), want a clean (0, \"\") with no resolution warning", exitCode, out)
	}
}
