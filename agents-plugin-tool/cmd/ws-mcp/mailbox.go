package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/kang-sw/devenv/internal/wsmailbox"
)

// mailbox.go implements the host-neutral wake CLI
// (260913-feat-cross-session-mailbox-wake Phase 1, Decision 6): a blocking
// `ws-mcp mailbox wait` subcommand, never an MCP tool, so a harness can arm
// it as a background task and re-invoke the agent on exit. The blocking/peek
// logic itself lives in internal/wsmailbox (host-neutral, unit-tested); this
// file is the thin CLI-argument/exit-code/output layer over it, mirroring
// the git/tickets subcommand dispatch pattern above.

func mailboxCommand(args []string) {
	if len(args) < 1 {
		mailboxUsage()
		os.Exit(2)
	}
	switch args[0] {
	case "wait":
		mailboxWait(args[1:])
	default:
		mailboxUsage()
		os.Exit(2)
	}
}

func mailboxUsage() {
	fmt.Fprintln(os.Stderr, "usage: ws-mcp mailbox <wait>")
}

// mailboxWaitExitTimeout is the distinguishable exit code for "the wait
// deadline passed with no unread mail" — distinct from 0 (mail found) and 1
// (a genuine error), so a harness adapter or script can tell the three
// outcomes apart without parsing stdout.
const mailboxWaitExitTimeout = 3

// mailboxWaitExitInterrupted is the exit code for a SIGINT/SIGTERM-terminated
// wait: the listening marker is cleared before exit, but the process ended
// before a deadline or mail arrival, so this is neither the mailboxWaitExit-
// Timeout nor a normal success/failure outcome.
const mailboxWaitExitInterrupted = 130

func mailboxWait(args []string) {
	fs := flag.NewFlagSet("mailbox wait", flag.ExitOnError)
	root := fs.String("root", ".", "caller's own worktree/clone root; only needed to resolve a worktree/clone-scope --slug")
	sessionKey := fs.String("session-key", "", "caller's own session_key (required): computes the reply-id queue and the named-inbox owner check")
	slug := fs.String("slug", "", `optional explicit "name@scope" to also check the named inbox for (owner-gated); omit for reply-id-only (env-less) wait`)
	timeout := fs.Duration("timeout", 0, "maximum time to block (e.g. 5m); 0 blocks until mail arrives")
	format := fs.String("format", "", `output format: text or json`)
	_ = fs.Parse(args)

	*sessionKey = strings.TrimSpace(*sessionKey)
	if *sessionKey == "" {
		fatal("mailbox wait", fmt.Errorf("--session-key is required"))
	}
	if *timeout < 0 {
		fatal("mailbox wait", fmt.Errorf("--timeout must be >= 0 (0 blocks until mail arrives)"))
	}

	target := wsmailbox.WaitTarget{SessionKey: *sessionKey, Root: defaultRoot(*root), Slug: strings.TrimSpace(*slug)}

	// One-time startup diagnostic (never repeated in the poll loop): warn
	// when an explicit --slug cannot actually be reached, so a wait that
	// silently degrades to reply-id-only does not also silently time out
	// with no explanation while the named inbox it names fills up.
	if target.Slug != "" {
		if present, owned, err := wsmailbox.NamedInboxStatus(target); err != nil {
			fmt.Fprintf(os.Stderr, "ws-mcp mailbox wait: warning: could not resolve --slug %s: %v\n", target.Slug, err)
		} else if !owned {
			reason := "has no presence record yet"
			if present {
				reason = "is not currently owned by this --session-key"
			}
			fmt.Fprintf(os.Stderr, "ws-mcp mailbox wait: warning: named inbox %s %s; falling back to a reply-id-only wait\n", target.Slug, reason)
		}
	}

	nowStr := time.Now().UTC().Format(time.RFC3339)
	marker := wsmailbox.ListeningMarker{SessionKey: *sessionKey, Slug: target.Slug, PID: os.Getpid(), StartedAt: nowStr}
	if *timeout > 0 {
		marker.TimeoutAt = time.Now().UTC().Add(*timeout).Format(time.RFC3339)
	}
	if err := wsmailbox.WriteListeningMarker(marker); err != nil {
		fatal("mailbox wait", err)
	}
	// Every exit path below calls os.Exit (directly, or via fatal/
	// emitMailboxWaitResult): os.Exit never runs deferred functions, so the
	// marker clear cannot live behind `defer` here — each branch clears it
	// explicitly before its own terminal exit.
	clearMarker := func() { _ = wsmailbox.ClearListeningMarker(*sessionKey) }

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	type outcome struct {
		result wsmailbox.WaitResult
		err    error
	}
	done := make(chan outcome, 1)
	go func() {
		result, err := wsmailbox.Wait(target, wsmailbox.WaitOptions{Timeout: *timeout})
		done <- outcome{result, err}
	}()

	select {
	case out := <-done:
		clearMarker()
		if out.err != nil {
			fatal("mailbox wait", out.err)
		}
		emitMailboxWaitResult(out.result, *format)
	case <-ctx.Done():
		clearMarker()
		fmt.Fprintln(os.Stderr, "ws-mcp mailbox wait: interrupted")
		os.Exit(mailboxWaitExitInterrupted)
	}
}

func emitMailboxWaitResult(result wsmailbox.WaitResult, format string) {
	if result.TimedOut {
		if outputJSON(format) {
			printJSONOrFatal("mailbox wait", map[string]any{
				"timed_out": true, "unread": 0,
				"named": []wsmailbox.Envelope{}, "reply": []wsmailbox.Envelope{},
			}, nil)
		} else {
			fmt.Println("timeout: no unread mail")
		}
		os.Exit(mailboxWaitExitTimeout)
	}

	total := result.Total()
	if outputJSON(format) {
		// Normalize a nil (empty) slice to "[]" rather than "null": this
		// JSON shape is a declared new type contract for future adapters,
		// so every consumer gets one array shape per field, never a null
		// case to special-case.
		named, reply := result.Named, result.Reply
		if named == nil {
			named = []wsmailbox.Envelope{}
		}
		if reply == nil {
			reply = []wsmailbox.Envelope{}
		}
		printJSONOrFatal("mailbox wait", map[string]any{
			"timed_out": false,
			"unread":    total,
			"named":     named,
			"reply":     reply,
		}, nil)
		return
	}
	fmt.Printf("unread %d\n", total)
	for _, m := range result.Named {
		printMailboxEnvelope(m)
	}
	for _, m := range result.Reply {
		printMailboxEnvelope(m)
	}
}

func printMailboxEnvelope(m wsmailbox.Envelope) {
	handle := m.From
	if handle == "" {
		handle = m.ReplyTo
	}
	fmt.Printf("[%s] %s: %s\n", m.SentAt, handle, m.Content)
}
