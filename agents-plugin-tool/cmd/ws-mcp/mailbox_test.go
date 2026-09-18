package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsmailbox"
)

// buildWsMCPMailboxTestBin builds the ws-mcp binary once for this file's
// mailbox CLI tests, mirroring the go-build-then-exec pattern main_test.go
// already uses for every other subcommand.
func buildWsMCPMailboxTestBin(t *testing.T) string {
	t.Helper()
	bin := wsMCPTestBin(t)
	build := exec.Command("go", "build", "-o", bin, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build failed: %v\n%s", err, string(out))
	}
	return bin
}

// mailboxTestEnv returns an isolated WS_CONFIG_HOME/WS_CACHE_HOME env so a
// test's mailbox store/secret/listening-marker files never touch the real
// machine-global cache.
func mailboxTestEnv(t *testing.T) []string {
	t.Helper()
	return append(os.Environ(),
		"WS_CONFIG_HOME="+filepath.Join(t.TempDir(), "config"),
		"WS_CACHE_HOME="+filepath.Join(t.TempDir(), "cache"),
	)
}

// cacheHomeFromEnv extracts WS_CACHE_HOME from an env slice built by
// mailboxTestEnv, so an in-process helper (seeding via internal/wsmailbox
// directly, or resolving a marker path) reads/writes the exact same store
// files the subprocess under test uses.
func cacheHomeFromEnv(t *testing.T, env []string) string {
	t.Helper()
	return envValue(t, env, "WS_CACHE_HOME")
}

// configHomeFromEnv extracts WS_CONFIG_HOME from an env slice built by
// mailboxTestEnv: MachinePath (used to seed a machine-scope named inbox)
// resolves through wsconfig.GlobalPath, which is keyed on WS_CONFIG_HOME,
// not WS_CACHE_HOME — the two stores live under different roots.
func configHomeFromEnv(t *testing.T, env []string) string {
	t.Helper()
	return envValue(t, env, "WS_CONFIG_HOME")
}

func envValue(t *testing.T, env []string, key string) string {
	t.Helper()
	for _, kv := range env {
		if v, ok := strings.CutPrefix(kv, key+"="); ok {
			return v
		}
	}
	t.Fatalf("%s missing from test env", key)
	return ""
}

func TestMailboxWaitRequiresSessionKey(t *testing.T) {
	bin := buildWsMCPMailboxTestBin(t)
	cmd := exec.Command(bin, "mailbox", "wait", "--timeout", "1s")
	cmd.Env = mailboxTestEnv(t)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("mailbox wait without --session-key unexpectedly succeeded: %s", out)
	}
	if !strings.Contains(string(out), "--session-key is required") {
		t.Fatalf("mailbox wait error = %q, want a --session-key is required message", out)
	}
}

func TestMailboxWaitReturnsImmediatelyOnUnreadReplyMail(t *testing.T) {
	bin := buildWsMCPMailboxTestBin(t)
	env := mailboxTestEnv(t)

	const sessionKey = "amber-tide-fox"
	seedReplyMail(t, env, sessionKey, "run ticket X")

	cmd := exec.Command(bin, "mailbox", "wait", "--session-key", sessionKey, "--timeout", "10s")
	cmd.Env = env
	start := time.Now()
	out, err := cmd.CombinedOutput()
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("mailbox wait failed: %v\n%s", err, out)
	}
	if elapsed > 5*time.Second {
		t.Fatalf("mailbox wait took %s for already-unread mail, want a near-immediate level-triggered return", elapsed)
	}
	text := string(out)
	if !strings.Contains(text, "unread 1") || !strings.Contains(text, "run ticket X") {
		t.Fatalf("mailbox wait output = %q, want it to report the pre-seeded mail", text)
	}
}

func TestMailboxWaitTimesOutCleanlyWithDistinguishableExitCode(t *testing.T) {
	bin := buildWsMCPMailboxTestBin(t)
	env := mailboxTestEnv(t)

	cmd := exec.Command(bin, "mailbox", "wait", "--session-key", "amber-tide-fox", "--timeout", "1s")
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("mailbox wait on an empty queue exited without an ExitError: err=%v out=%s", err, out)
	}
	if exitErr.ExitCode() != mailboxWaitExitTimeout {
		t.Fatalf("mailbox wait exit code = %d, want %d (timeout)", exitErr.ExitCode(), mailboxWaitExitTimeout)
	}
	if !strings.Contains(string(out), "timeout") {
		t.Fatalf("mailbox wait timeout output = %q, want it to mention the timeout", out)
	}
}

func TestMailboxWaitJSONFormat(t *testing.T) {
	bin := buildWsMCPMailboxTestBin(t)
	env := mailboxTestEnv(t)

	const sessionKey = "amber-tide-fox"
	seedReplyMail(t, env, sessionKey, "hello")

	cmd := exec.Command(bin, "mailbox", "wait", "--session-key", sessionKey, "--timeout", "10s", "--format", "json")
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("mailbox wait --format json failed: %v\n%s", err, out)
	}
	var got struct {
		TimedOut bool                 `json:"timed_out"`
		Unread   int                  `json:"unread"`
		Reply    []wsmailbox.Envelope `json:"reply"`
		Named    []wsmailbox.Envelope `json:"named"`
	}
	if uerr := json.Unmarshal(bytes.TrimSpace(out), &got); uerr != nil {
		t.Fatalf("invalid mailbox wait JSON: %v\n%s", uerr, out)
	}
	if got.TimedOut || got.Unread != 1 || len(got.Reply) != 1 || got.Reply[0].Content != "hello" {
		t.Fatalf("mailbox wait JSON = %#v", got)
	}
}

// TestMailboxWaitListeningMarkerLifecycle exercises the discoverable armed-
// wait marker end to end: written while the CLI blocks on an empty queue,
// then cleared once it exits at the timeout deadline (Phase 1's own
// verification bullet: "written while armed and cleared on exit").
func TestMailboxWaitListeningMarkerLifecycle(t *testing.T) {
	bin := buildWsMCPMailboxTestBin(t)
	env := mailboxTestEnv(t)
	t.Setenv("WS_CACHE_HOME", cacheHomeFromEnv(t, env))

	const sessionKey = "amber-tide-fox"
	markerPath, err := wsmailbox.ListeningMarkerPath(sessionKey)
	if err != nil {
		t.Fatalf("ListeningMarkerPath: %v", err)
	}

	cmd := exec.Command(bin, "mailbox", "wait", "--session-key", sessionKey, "--timeout", "3s")
	cmd.Env = env
	if err := cmd.Start(); err != nil {
		t.Fatalf("start mailbox wait: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		if _, statErr := os.Stat(markerPath); statErr == nil {
			break
		}
		if time.Now().After(deadline) {
			_ = cmd.Process.Kill()
			t.Fatalf("listening marker not written within 2s at %s", markerPath)
		}
		time.Sleep(20 * time.Millisecond)
	}

	waitErr := cmd.Wait()
	exitErr, ok := waitErr.(*exec.ExitError)
	if !ok || exitErr.ExitCode() != mailboxWaitExitTimeout {
		t.Fatalf("mailbox wait ended unexpectedly (want timeout exit %d): err=%v", mailboxWaitExitTimeout, waitErr)
	}

	if _, statErr := os.Stat(markerPath); !os.IsNotExist(statErr) {
		t.Fatalf("listening marker still present after mailbox wait exited: err=%v", statErr)
	}
}

// TestMailboxWaitSlugEndToEndThroughCLI exercises the --slug flag through the
// actual CLI binary (flag parsing -> WaitTarget -> owner-gated named-inbox
// peek), not just internal/wsmailbox's function-level tests: a flag-name
// typo or field-mapping bug in mailbox.go's CLI layer would otherwise go
// undetected (round-1 test review finding).
func TestMailboxWaitSlugEndToEndThroughCLI(t *testing.T) {
	bin := buildWsMCPMailboxTestBin(t)
	env := mailboxTestEnv(t)

	const sessionKey = "amber-tide-fox"
	seedNamedInbox(t, env, "alice", sessionKey, "run ticket X")

	cmd := exec.Command(bin, "mailbox", "wait", "--session-key", sessionKey, "--slug", "alice@machine", "--timeout", "10s")
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("mailbox wait --slug failed: %v\n%s", err, out)
	}
	text := string(out)
	if !strings.Contains(text, "unread 1") || !strings.Contains(text, "run ticket X") {
		t.Fatalf("mailbox wait --slug output = %q, want it to report the named-inbox mail", text)
	}
}

// TestMailboxWaitWarnsWhenSlugIsNotOwned covers the round-1 correctness
// finding that an unreachable --slug (no presence yet, or owned by a
// different session_key) must not degrade to a silent reply-id-only wait:
// the CLI emits a one-time stderr diagnostic before it ever blocks.
func TestMailboxWaitWarnsWhenSlugIsNotOwned(t *testing.T) {
	bin := buildWsMCPMailboxTestBin(t)
	env := mailboxTestEnv(t)

	seedNamedInbox(t, env, "alice", "someone-else-key", "not for you")

	cmd := exec.Command(bin, "mailbox", "wait", "--session-key", "amber-tide-fox", "--slug", "alice@machine", "--timeout", "1s")
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	if _, ok := err.(*exec.ExitError); !ok {
		t.Fatalf("mailbox wait with an unowned --slug did not time out as expected: err=%v out=%s", err, out)
	}
	if !strings.Contains(string(out), "not currently owned") {
		t.Fatalf("mailbox wait output = %q, want a stderr warning that the named inbox is not owned by this session", out)
	}
}

// TestMailboxWaitRealSubprocessBlocksThenReturnsOnArrival is the one CLI-level
// test that exercises the actual select/goroutine/polling wiring in
// mailbox.go against a real subprocess: start on an empty queue, deposit
// mail while it is genuinely blocked, and confirm it exits promptly with the
// mail rather than only at the timeout deadline. The unit-level equivalent
// (TestWaitBlocksThenReturnsOnArrivalDuringSleep) proves the same control
// flow with a fake clock; this proves the CLI wiring around it end to end
// (round-1 test review finding: the "arrival mid-block" case had no
// subprocess coverage).
func TestMailboxWaitRealSubprocessBlocksThenReturnsOnArrival(t *testing.T) {
	bin := buildWsMCPMailboxTestBin(t)
	env := mailboxTestEnv(t)

	const sessionKey = "amber-tide-fox"
	cmd := exec.Command(bin, "mailbox", "wait", "--session-key", sessionKey, "--timeout", "10s")
	cmd.Env = env
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	start := time.Now()
	if err := cmd.Start(); err != nil {
		t.Fatalf("start mailbox wait: %v", err)
	}

	// Give the subprocess time to perform its immediate (empty) peek and
	// enter its first poll sleep before depositing mail, so this genuinely
	// exercises "arrives while blocked" rather than the level-triggered
	// immediate-return path already covered elsewhere.
	time.Sleep(150 * time.Millisecond)
	seedReplyMail(t, env, sessionKey, "arrived mid-block")

	waitErr := cmd.Wait()
	elapsed := time.Since(start)
	if waitErr != nil {
		t.Fatalf("mailbox wait failed: %v\n%s", waitErr, stdout.String())
	}
	if elapsed > 5*time.Second {
		t.Fatalf("mailbox wait took %s to notice mail deposited mid-block, want well under its 10s timeout", elapsed)
	}
	text := stdout.String()
	if !strings.Contains(text, "unread 1") || !strings.Contains(text, "arrived mid-block") {
		t.Fatalf("mailbox wait output = %q, want it to report the mail deposited while blocked", text)
	}
}

// TestMailboxWaitRejectsNegativeTimeout covers the round-1 correctness
// finding that a negative --timeout was silently accepted as "block
// forever" instead of being rejected.
func TestMailboxWaitRejectsNegativeTimeout(t *testing.T) {
	bin := buildWsMCPMailboxTestBin(t)
	cmd := exec.Command(bin, "mailbox", "wait", "--session-key", "amber-tide-fox", "--timeout", "-5s")
	cmd.Env = mailboxTestEnv(t)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("mailbox wait with a negative --timeout unexpectedly succeeded: %s", out)
	}
	if !strings.Contains(string(out), "--timeout must be >= 0") {
		t.Fatalf("mailbox wait error = %q, want a --timeout must be >= 0 message", out)
	}
}

// TestMailboxWaitFailsWithGenericErrorExitCodeOnInvalidSessionKey drives the
// CLI's exit-code-1 "genuine error" path (round-1 test review finding: this
// had no coverage anywhere in the diff), using the same invalid session_key
// input the round-1 correctness fix now rejects at the marker-write step.
func TestMailboxWaitFailsWithGenericErrorExitCodeOnInvalidSessionKey(t *testing.T) {
	bin := buildWsMCPMailboxTestBin(t)
	cmd := exec.Command(bin, "mailbox", "wait", "--session-key", "../../escaped/pwned", "--timeout", "1s")
	cmd.Env = mailboxTestEnv(t)
	out, err := cmd.CombinedOutput()
	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("mailbox wait with an invalid session_key exited without an ExitError: err=%v out=%s", err, out)
	}
	if exitErr.ExitCode() != 1 {
		t.Fatalf("mailbox wait exit code = %d, want 1 (generic error)", exitErr.ExitCode())
	}
	if !strings.Contains(string(out), "invalid session_key") {
		t.Fatalf("mailbox wait error output = %q, want it to mention the invalid session_key", out)
	}
}

// TestMailboxWaitRearmReminderOnMailExit asserts Phase 1 of 260917: when the
// wait returns because mail arrived, the text output appends a runnable re-arm
// command (its own binary + the wait-scoping flags it received) and the
// plain-language nudge, so the "one wait covers one wake" reminder is delivered
// at fire time rather than only when the arming guidance was first read.
func TestMailboxWaitRearmReminderOnMailExit(t *testing.T) {
	bin := buildWsMCPMailboxTestBin(t)
	env := mailboxTestEnv(t)

	const sessionKey = "amber-tide-fox"
	seedReplyMail(t, env, sessionKey, "run ticket X")

	cmd := exec.Command(bin, "mailbox", "wait", "--session-key", sessionKey, "--timeout", "10s")
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("mailbox wait failed: %v\n%s", err, out)
	}
	text := string(out)
	if !strings.Contains(text, "re-arm:") {
		t.Fatalf("mailbox wait output = %q, want a re-arm: line on the mail exit", text)
	}
	// The re-arm command reprints the binary + the flags it was invoked with.
	if !strings.Contains(text, "mailbox wait --session-key "+sessionKey) || !strings.Contains(text, "--timeout 10s") {
		t.Fatalf("mailbox wait output = %q, want a runnable re-arm command carrying --session-key and --timeout", text)
	}
	if !strings.Contains(text, "re-run the re-arm command") {
		t.Fatalf("mailbox wait output = %q, want the re-arm nudge string", text)
	}
}

// TestMailboxWaitRearmReminderOnTimeoutExit asserts the same re-arm reminder is
// delivered on the timeout exit path, not only the mail-found path.
func TestMailboxWaitRearmReminderOnTimeoutExit(t *testing.T) {
	bin := buildWsMCPMailboxTestBin(t)
	env := mailboxTestEnv(t)

	const sessionKey = "amber-tide-fox"
	cmd := exec.Command(bin, "mailbox", "wait", "--session-key", sessionKey, "--slug", "alice@machine", "--timeout", "1s")
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	if exitErr, ok := err.(*exec.ExitError); !ok || exitErr.ExitCode() != mailboxWaitExitTimeout {
		t.Fatalf("mailbox wait did not time out as expected: err=%v out=%s", err, out)
	}
	text := string(out)
	if !strings.Contains(text, "re-arm:") || !strings.Contains(text, "re-run the re-arm command") {
		t.Fatalf("mailbox wait timeout output = %q, want the re-arm command and nudge", text)
	}
	// An explicit --slug is reprinted in the re-arm command so re-running it
	// re-arms an identical (named-inbox-covering) wait.
	if !strings.Contains(text, "--slug alice@machine") || !strings.Contains(text, "--timeout 1s") {
		t.Fatalf("mailbox wait timeout output = %q, want the re-arm command to reprint --slug and --timeout", text)
	}
}

// TestMailboxWaitJSONCarriesRearmField asserts the --format json path gains the
// additive `rearm` object (command + nudge) without disturbing the existing
// fields, so machine callers see one new field rather than a changed shape.
func TestMailboxWaitJSONCarriesRearmField(t *testing.T) {
	bin := buildWsMCPMailboxTestBin(t)
	env := mailboxTestEnv(t)

	const sessionKey = "amber-tide-fox"
	seedReplyMail(t, env, sessionKey, "hello")

	cmd := exec.Command(bin, "mailbox", "wait", "--session-key", sessionKey, "--timeout", "10s", "--format", "json")
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("mailbox wait --format json failed: %v\n%s", err, out)
	}
	var got struct {
		TimedOut bool                 `json:"timed_out"`
		Unread   int                  `json:"unread"`
		Reply    []wsmailbox.Envelope `json:"reply"`
		Named    []wsmailbox.Envelope `json:"named"`
		Rearm    struct {
			Command string `json:"command"`
			Nudge   string `json:"nudge"`
		} `json:"rearm"`
	}
	if uerr := json.Unmarshal(bytes.TrimSpace(out), &got); uerr != nil {
		t.Fatalf("invalid mailbox wait JSON: %v\n%s", uerr, out)
	}
	// Existing fields stay structurally stable.
	if got.TimedOut || got.Unread != 1 || len(got.Reply) != 1 || got.Reply[0].Content != "hello" {
		t.Fatalf("mailbox wait JSON existing fields changed: %#v", got)
	}
	if !strings.Contains(got.Rearm.Command, "mailbox wait --session-key "+sessionKey) || got.Rearm.Nudge == "" {
		t.Fatalf("mailbox wait JSON rearm = %#v, want a runnable command and a non-empty nudge", got.Rearm)
	}
}

// TestMailboxWaitJSONTimeoutCarriesRearmField asserts the timeout JSON exit
// also carries the additive `rearm` field.
func TestMailboxWaitJSONTimeoutCarriesRearmField(t *testing.T) {
	bin := buildWsMCPMailboxTestBin(t)
	env := mailboxTestEnv(t)

	cmd := exec.Command(bin, "mailbox", "wait", "--session-key", "amber-tide-fox", "--timeout", "1s", "--format", "json")
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	if exitErr, ok := err.(*exec.ExitError); !ok || exitErr.ExitCode() != mailboxWaitExitTimeout {
		t.Fatalf("mailbox wait --format json did not time out as expected: err=%v out=%s", err, out)
	}
	var got struct {
		TimedOut bool `json:"timed_out"`
		Unread   int  `json:"unread"`
		Rearm    struct {
			Command string `json:"command"`
			Nudge   string `json:"nudge"`
		} `json:"rearm"`
	}
	if uerr := json.Unmarshal(bytes.TrimSpace(out), &got); uerr != nil {
		t.Fatalf("invalid mailbox wait JSON: %v\n%s", uerr, out)
	}
	if !got.TimedOut || got.Unread != 0 {
		t.Fatalf("mailbox wait timeout JSON = %#v, want timed_out=true unread=0", got)
	}
	if got.Rearm.Command == "" || got.Rearm.Nudge == "" {
		t.Fatalf("mailbox wait timeout JSON rearm = %#v, want a command and nudge", got.Rearm)
	}
}

// seedNamedInbox seeds a machine-scope presence record (owned by ownerKey)
// plus one queued envelope for name, directly via internal/wsmailbox.
func seedNamedInbox(t *testing.T, env []string, name, ownerKey, content string) {
	t.Helper()
	t.Setenv("WS_CACHE_HOME", cacheHomeFromEnv(t, env))
	t.Setenv("WS_CONFIG_HOME", configHomeFromEnv(t, env))

	path, err := wsmailbox.MachinePath()
	if err != nil {
		t.Fatalf("MachinePath: %v", err)
	}
	if err := wsmailbox.WithLock(path, func(store *wsmailbox.StoreFile) error {
		store.Presence[name] = wsmailbox.Presence{Name: name, Scope: wsmailbox.ScopeMachine, Owner: ownerKey, LastSeen: "2026-09-13T00:00:00Z"}
		store.Queues = wsmailbox.AppendQueue(store.Queues, name, wsmailbox.Envelope{Content: content, SentAt: "2026-09-13T00:00:00Z"})
		return nil
	}); err != nil {
		t.Fatalf("seed named inbox: %v", err)
	}
}

// seedReplyMail deposits one envelope directly into sessionKey's reply-id
// queue via internal/wsmailbox, independent of the mailbox.send MCP tool
// (260913-feat-cross-session-mailbox-core, out of this ticket's scope) so
// this test only ever exercises the wait CLI under test.
func seedReplyMail(t *testing.T, env []string, sessionKey, content string) {
	t.Helper()
	t.Setenv("WS_CACHE_HOME", cacheHomeFromEnv(t, env))

	secret, err := wsmailbox.EnsureMachineSecret()
	if err != nil {
		t.Fatalf("EnsureMachineSecret: %v", err)
	}
	replyID := wsmailbox.ReplyID(secret, sessionKey)
	path, err := wsmailbox.ReplyRegistryPath()
	if err != nil {
		t.Fatalf("ReplyRegistryPath: %v", err)
	}
	if err := wsmailbox.WithReplyLock(path, func(store *wsmailbox.ReplyStore) error {
		store.Queues = wsmailbox.AppendQueue(store.Queues, replyID, wsmailbox.Envelope{Content: content, SentAt: "2026-09-13T00:00:00Z"})
		return nil
	}); err != nil {
		t.Fatalf("seed reply queue: %v", err)
	}
}
