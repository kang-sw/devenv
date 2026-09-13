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

// codexStopHookMailboxEnv mirrors internal/mcp/mailbox_runtime.go's
// envMailbox constant. Duplicated rather than imported: cmd/ws-mcp does not
// otherwise depend on internal/mcp, and one constant does not justify
// adding that dependency (the same tradeoff internal/wsmailbox/listening.go
// already makes for its session_key pattern). Deliberately WS_MAILBOX only,
// never WS_MAILBOX_AUTO — see the doc comment below.
const codexStopHookMailboxEnv = "WS_MAILBOX"

// mailboxCodexStopHook implements the Codex Stop-hook adapter's level check
// (260913-feat-cross-session-mailbox-wake Phase 2, Decisions 6/7/9): a
// turn-boundary, event-driven read of one slug's named-inbox queue, never a
// poll loop (Cross-Child Decision 16 — this fires once per Stop event, it
// does not sleep-and-recheck). It emits Codex's Stop-hook
// `{"decision":"block","reason":...}` response only when unread mail is
// pending *and* the queue's length has changed since the last time this
// hook notified for the same slug (internal/wsmailbox's
// ShouldNotifyNamedInboxUnread — an unbounded per-turn block on a queue the
// woken session can never drain, e.g. an unowned or rebound named inbox,
// would itself be the "worse than a missed wake" outcome Decision 7 warns
// against). Otherwise it is silent and exits 0 so the turn concludes
// normally.
//
// This is adapter code, not the host-neutral contract: Phase 1's
// `mailbox wait`/listening marker back the blocking CLI wait the env-less
// and Claude paths use; Codex's Stop hook instead re-fires on every turn
// conclusion, so it only ever needs one non-blocking peek per firing, over
// internal/wsmailbox's ShouldNotifyNamedInboxUnread rather than Wait.
//
// The slug to check comes from this process's own inherited WS_MAILBOX
// environment variable (read directly here, at fire time — NOT baked as a
// static value into the hook command's args or the plugin manifest, which
// cannot know a per-session value at authoring time): the Stop hook
// subprocess is a plain child of the same harness process tree as the MCP
// server, so it inherits the identical env the session was launched with.
// --slug exists only as an override for direct invocation/testing; the
// shipped hooks.json never passes it. WS_MAILBOX_AUTO is deliberately never
// consulted: its name is minted randomly inside the MCP server process
// (internal/mcp/mailbox_runtime.go's computeMailboxIdentity) and never
// exported anywhere a sibling process can read it, so a bare hook
// subprocess has no way to discover it — durable, hook-backed wake on Codex
// is therefore scoped to an explicit WS_MAILBOX for now.
//
// Every failure path here fails open (exit 0, stderr warning only): a hook
// that crashes or blocks the harness's turn-conclude step would be worse
// than a missed wake, and Decision 7 frames this whole adapter as
// best-effort, never the contract. Stdin is always drained (even on the
// no-op empty-slug path) so the harness writing the Stop payload never sees
// a reader that exited without reading it.
func mailboxCodexStopHook(args []string) {
	fs := flag.NewFlagSet("mailbox codex-stop-hook", flag.ExitOnError)
	root := fs.String("root", ".", "caller's own worktree/clone root; only needed to resolve a worktree/clone-scope slug. Falls back to the Stop payload's own cwd, then WS_MCP_PROJECT_ROOT, when left at its default")
	slug := fs.String("slug", "", `override for the mailbox slug "name@scope" to check; the shipped hook never passes this, reading WS_MAILBOX from its own environment instead. Empty (both) means nothing to check`)
	reason := fs.String("reason", defaultCodexStopHookReason, "instruction text returned to the model when unread mail is pending")
	_ = fs.Parse(args)

	// Always read (and thus drain) stdin before any exit path, including
	// the no-op ones below: exiting first would leave the harness's Stop
	// payload write unread on this end.
	payload, payloadErr := readCodexStopHookPayload(os.Stdin)

	*slug = strings.TrimSpace(*slug)
	if *slug == "" {
		*slug = strings.TrimSpace(os.Getenv(codexStopHookMailboxEnv))
	}
	if *slug == "" {
		// No durable inbox to check (no WS_MAILBOX): this hook has nothing
		// to do. The env-less wake path is the model's own background-wait
		// registration at piggyback time, not this hook.
		os.Exit(0)
	}

	if payloadErr != nil {
		fmt.Fprintf(os.Stderr, "ws-mcp mailbox codex-stop-hook: warning: could not read Stop hook payload: %v\n", payloadErr)
		os.Exit(0)
	}
	if payload.StopHookActive {
		// Loop guard: this is the re-entry Stop from a prior block. Blocking
		// again here would loop regardless of whether the agent drained the
		// mail on its one injected instruction cycle.
		os.Exit(0)
	}

	effectiveRoot := strings.TrimSpace(*root)
	if (effectiveRoot == "" || effectiveRoot == ".") && payload.Cwd != "" {
		// The Stop payload's own cwd is a more direct signal of this turn's
		// working directory than the launcher's own WS_MCP_PROJECT_ROOT
		// inference (which resolves against the hook subprocess's cwd
		// anyway); prefer it for a worktree/clone-scope slug.
		effectiveRoot = payload.Cwd
	}

	notify, _, err := wsmailbox.ShouldNotifyNamedInboxUnread(*slug, defaultRoot(effectiveRoot))
	if err != nil {
		fmt.Fprintf(os.Stderr, "ws-mcp mailbox codex-stop-hook: warning: could not check %s: %v\n", *slug, err)
		os.Exit(0)
	}
	if !notify {
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
// turn_id, transcript_path, hook_event_name, model, permission_mode, and
// last_assistant_message; none of those are needed to decide whether to
// block.
type codexStopHookPayload struct {
	StopHookActive bool   `json:"stop_hook_active"`
	Cwd            string `json:"cwd"`
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
