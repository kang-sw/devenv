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
	cacheHome := ""
	for _, kv := range env {
		if v, ok := strings.CutPrefix(kv, "WS_CACHE_HOME="); ok {
			cacheHome = v
		}
	}
	if cacheHome == "" {
		t.Fatal("WS_CACHE_HOME missing from test env")
	}
	t.Setenv("WS_CACHE_HOME", cacheHome)

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

	if err := cmd.Wait(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); !ok || exitErr.ExitCode() != mailboxWaitExitTimeout {
			t.Fatalf("mailbox wait ended unexpectedly: %v", err)
		}
	}

	if _, statErr := os.Stat(markerPath); !os.IsNotExist(statErr) {
		t.Fatalf("listening marker still present after mailbox wait exited: err=%v", statErr)
	}
}

// seedReplyMail deposits one envelope directly into sessionKey's reply-id
// queue via internal/wsmailbox, independent of the mailbox.send MCP tool
// (260913-feat-cross-session-mailbox-core, out of this ticket's scope) so
// this test only ever exercises the wait CLI under test.
func seedReplyMail(t *testing.T, env []string, sessionKey, content string) {
	t.Helper()
	cacheHome := ""
	for _, kv := range env {
		if v, ok := strings.CutPrefix(kv, "WS_CACHE_HOME="); ok {
			cacheHome = v
		}
	}
	if cacheHome == "" {
		t.Fatal("WS_CACHE_HOME missing from test env")
	}
	t.Setenv("WS_CACHE_HOME", cacheHome)

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
