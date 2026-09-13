package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
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
	case "codex-stop-hook":
		mailboxCodexStopHook(args[1:])
	default:
		mailboxUsage()
		os.Exit(2)
	}
}

func mailboxUsage() {
	fmt.Fprintln(os.Stderr, "usage: ws-mcp mailbox <wait|codex-stop-hook>")
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

// mailboxCodexStopHook implements the Codex Stop-hook adapter's level check
// (260913-feat-cross-session-mailbox-wake Phase 2, Decisions 6/7/9): a
// turn-boundary, event-driven read of one slug's named-inbox queue, never a
// poll loop (Cross-Child Decision 16 — this fires once per Stop event, it
// does not sleep-and-recheck). It emits Codex's Stop-hook
// `{"decision":"block","reason":...}` response only when unread mail is
// pending; otherwise it is silent and exits 0 so the turn concludes
// normally.
//
// This is adapter code, not the host-neutral contract: Phase 1's
// `mailbox wait`/listening marker back the blocking CLI wait the env-less
// and Claude paths use; Codex's Stop hook instead re-fires on every turn
// conclusion, so it only ever needs one non-blocking peek per firing, over
// internal/wsmailbox's PeekNamedInboxUnread rather than Wait.
//
// --slug is baked into the hook command's args at arm time from the
// harness process's own inherited WS_MAILBOX/WS_MAILBOX_AUTO env — this
// process has no ws session_key to derive one itself, and per Decision 6
// the durable, hook-backed wake path requires that slug (env). Every
// failure path here fails open (exit 0, stderr warning only): a hook that
// crashes or blocks the harness's turn-conclude step would be worse than a
// missed wake, and Decision 7 frames this whole adapter as best-effort,
// never the contract.
func mailboxCodexStopHook(args []string) {
	fs := flag.NewFlagSet("mailbox codex-stop-hook", flag.ExitOnError)
	root := fs.String("root", ".", "caller's own worktree/clone root; only needed to resolve a worktree/clone-scope --slug")
	slug := fs.String("slug", "", `mailbox slug "name@scope" to check, baked into the hook command at arm time from the harness's own inherited WS_MAILBOX/WS_MAILBOX_AUTO; empty means nothing to check`)
	reason := fs.String("reason", defaultCodexStopHookReason, "instruction text returned to the model when unread mail is pending")
	_ = fs.Parse(args)

	*slug = strings.TrimSpace(*slug)
	if *slug == "" {
		// No durable inbox to check (env-less session): this hook has
		// nothing to do. The env-less wake path is the model's own
		// background-wait registration at piggyback time, not this hook.
		os.Exit(0)
	}

	payload, err := readCodexStopHookPayload(os.Stdin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ws-mcp mailbox codex-stop-hook: warning: could not read Stop hook payload: %v\n", err)
		os.Exit(0)
	}
	if payload.StopHookActive {
		// Loop guard: this is the re-entry Stop from a prior block. Blocking
		// again here would loop regardless of whether the agent drained the
		// mail on its one injected instruction cycle.
		os.Exit(0)
	}

	unread, err := wsmailbox.PeekNamedInboxUnread(*slug, defaultRoot(*root))
	if err != nil {
		fmt.Fprintf(os.Stderr, "ws-mcp mailbox codex-stop-hook: warning: could not check %s: %v\n", *slug, err)
		os.Exit(0)
	}
	if unread == 0 {
		os.Exit(0)
	}

	printJSONOrFatal("mailbox codex-stop-hook", map[string]any{
		"decision": "block",
		"reason":   *reason,
	}, nil)
}

// defaultCodexStopHookReason is the instruction handed back to the model
// through Stop's decision:block response. It must stay host-neutral (no
// this-repository ticket/path references, per shipped-surface-boundary.md):
// downstream projects install this same plugin text verbatim.
const defaultCodexStopHookReason = "Unread mail is waiting in your mailbox. Call the mailbox recv tool now, then act on what it returns."

// codexStopHookPayload is the subset of Codex's Stop hook stdin payload
// this adapter reads. The full payload (per ai-docs/manuals/
// codex-integration.md's 2026-09-13 re-probe) also carries session_id,
// turn_id, transcript_path, cwd, hook_event_name, model, permission_mode,
// and last_assistant_message; none of those are needed to decide whether to
// block.
type codexStopHookPayload struct {
	StopHookActive bool `json:"stop_hook_active"`
}

// readCodexStopHookPayload decodes r's JSON body, tolerating an empty body
// (no error, zero-value payload) so an ad-hoc or malformed invocation still
// fails open rather than erroring on a merely-empty stdin.
func readCodexStopHookPayload(r io.Reader) (codexStopHookPayload, error) {
	raw, err := io.ReadAll(r)
	if err != nil {
		return codexStopHookPayload{}, fmt.Errorf("read stop hook payload: %w", err)
	}
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return codexStopHookPayload{}, nil
	}
	var payload codexStopHookPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return codexStopHookPayload{}, fmt.Errorf("parse stop hook payload: %w", err)
	}
	return payload, nil
}
