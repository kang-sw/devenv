package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsmailbox"
)

// initGitFixtureForHookTest creates a throwaway git repo under t.TempDir(),
// mirroring internal/wsmailbox's own initGitFixture test helper: a real
// commit is needed for `git rev-parse --show-toplevel` (which
// wsstate.Manager.Resolve shells out to) to succeed.
func initGitFixtureForHookTest(t *testing.T) string {
	t.Helper()
	repo := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	runGitForHookTest(t, repo, "init")
	runGitForHookTest(t, repo, "config", "user.email", "test@example.invalid")
	runGitForHookTest(t, repo, "config", "user.name", "Test User")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("# Test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGitForHookTest(t, repo, "add", "README.md")
	runGitForHookTest(t, repo, "commit", "-m", "init")
	return repo
}

func runGitForHookTest(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// seedWorktreeInbox seeds root's own worktree-scope mailbox store (as
// opposed to seedNamedInbox's machine scope) with one queued envelope for
// name, directly via internal/wsmailbox — mirrors seedNamedInbox's
// temporarily-scoped-env pattern so the same isolated cache/config home the
// CLI subprocess uses is the one this in-process seed call resolves
// against.
func seedWorktreeInbox(t *testing.T, env []string, root, name, ownerKey, content string) {
	t.Helper()
	t.Setenv("WS_CACHE_HOME", cacheHomeFromEnv(t, env))
	t.Setenv("WS_CONFIG_HOME", configHomeFromEnv(t, env))

	path, err := wsmailbox.WorktreePath(root)
	if err != nil {
		t.Fatalf("WorktreePath(%s): %v", root, err)
	}
	if err := wsmailbox.WithLock(path, func(store *wsmailbox.StoreFile) error {
		store.Presence[name] = wsmailbox.Presence{Name: name, Scope: wsmailbox.ScopeWorktree, Owner: ownerKey, LastSeen: "2026-09-13T00:00:00Z"}
		store.Queues = wsmailbox.AppendQueue(store.Queues, name, wsmailbox.Envelope{Content: content, SentAt: "2026-09-13T00:00:00Z"})
		return nil
	}); err != nil {
		t.Fatalf("seed worktree inbox %s at %s: %v", name, root, err)
	}
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
	return runMailboxCodexStopHookInDir(t, bin, env, "", stdin, args...)
}

// runMailboxCodexStopHookInDir is runMailboxCodexStopHook with an explicit
// subprocess working directory, needed only by the payload-cwd test below
// to make the OS-level process cwd deliberately diverge from the Stop
// payload's own "cwd" field — proving which one --root's default actually
// resolves against.
func runMailboxCodexStopHookInDir(t *testing.T, bin string, env []string, dir string, stdin string, args ...string) (string, int) {
	t.Helper()
	cmd := exec.Command(bin, append([]string{"mailbox", "codex-stop-hook"}, args...)...)
	cmd.Env = env
	cmd.Dir = dir
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
// worktree/clone-scope slug.
//
// Round-2 (test-partition, fix-verification) review flagged the original
// version of this test as too weak to actually catch a regression: it only
// asserted the ABSENCE of a warning against the test binary's own ambient
// process cwd (which happened to already be a valid worktree root), so it
// would have passed identically whether payload.cwd was wired in or
// silently ignored. This version deliberately makes the OS-level process
// cwd (via cmd.Dir) diverge from the payload's own cwd field, and seeds
// unread mail ONLY at the payload-cwd fixture's worktree-scope store: a
// decision:block response is possible only if the CLI actually resolved
// --root's default against payload.Cwd, not the subprocess's own OS cwd.
func TestMailboxCodexStopHookUsesPayloadCwdForWorktreeSlug(t *testing.T) {
	bin := buildWsMCPMailboxTestBin(t)
	env := mailboxTestEnv(t)

	mailFixture := initGitFixtureForHookTest(t)       // has unread "lead@worktree" mail
	processCwdFixture := initGitFixtureForHookTest(t) // deliberately does not
	seedWorktreeInbox(t, env, mailFixture, "lead", "owner-key", "run ticket X")

	payload := `{"hook_event_name":"Stop","stop_hook_active":false,"cwd":"` + strings.ReplaceAll(mailFixture, `\`, `\\`) + `"}`
	out, exitCode := runMailboxCodexStopHookInDir(t, bin, withMailboxEnv(env, "lead@worktree"), processCwdFixture, payload)
	if exitCode != 0 || !strings.Contains(out, `"decision":"block"`) {
		t.Fatalf("mailbox codex-stop-hook with payload.cwd pointing at the seeded fixture = (exit %d, out %q), want a decision:block response — --root's default must resolve against payload.cwd, not the OS process cwd", exitCode, out)
	}

	// Negative control: the exact same slug and the exact same OS process
	// cwd, but this time cwd is absent from the payload. --root's default
	// then resolves against processCwdFixture (which has no seeded mail),
	// so this must go silent — proving the prior block truly depended on
	// payload.cwd, not on some other ambient signal.
	noCwdPayload := `{"hook_event_name":"Stop","stop_hook_active":false}`
	out2, exit2 := runMailboxCodexStopHookInDir(t, bin, withMailboxEnv(env, "lead@worktree"), processCwdFixture, noCwdPayload)
	if exit2 != 0 || strings.TrimSpace(out2) != "" {
		t.Fatalf("mailbox codex-stop-hook without payload.cwd, OS cwd at the unseeded fixture = (exit %d, out %q), want a clean (0, \"\")", exit2, out2)
	}
}

// TestMailboxCodexStopHookHonorsExplicitReason closes the round-1
// test-partition Minor finding (still open per round-2 verification): no
// test previously exercised --reason, so a regression breaking that flag
// entirely (e.g. a typo dropping it from the flag set) would have gone
// unnoticed by every other test here, which all leave it at its default.
func TestMailboxCodexStopHookHonorsExplicitReason(t *testing.T) {
	bin := buildWsMCPMailboxTestBin(t)
	env := mailboxTestEnv(t)
	seedNamedInbox(t, env, "lead", "owner-key", "run ticket X")

	const customReason = "custom drain instruction for this test"
	out, exitCode := runMailboxCodexStopHook(t, bin, withMailboxEnv(env, "lead@machine"), `{"hook_event_name":"Stop","stop_hook_active":false}`, "--reason", customReason)
	if exitCode != 0 {
		t.Fatalf("mailbox codex-stop-hook --reason exit code = %d, want 0\n%s", exitCode, out)
	}
	var got struct {
		Decision string `json:"decision"`
		Reason   string `json:"reason"`
	}
	if err := json.Unmarshal(bytes.TrimSpace([]byte(out)), &got); err != nil {
		t.Fatalf("invalid mailbox codex-stop-hook JSON: %v\n%s", err, out)
	}
	if got.Decision != "block" || got.Reason != customReason {
		t.Fatalf("mailbox codex-stop-hook --reason output = %#v, want decision=block with reason=%q", got, customReason)
	}
}
