package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/kang-sw/devenv/internal/mcp"
	"github.com/kang-sw/devenv/internal/wsconfig"
	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wsinbox"
	"github.com/kang-sw/devenv/internal/wsmailbox"
	"github.com/kang-sw/devenv/internal/wsstate"
	"github.com/kang-sw/devenv/internal/wswake"
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

// mailboxWait is the CLI entry: the wait runs in this process, owned by
// its PID, ended by a signal or (on Windows) the parent's death.
func mailboxWait(args []string) {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	// Parent death joins the signal path rather than exiting on its own, so
	// the one interrupted branch clears the marker and sets the exit code
	// for both. On Windows a host that stops this wait terminates only its
	// direct parent (the launcher), not this process; the watch is
	// Windows-only (a no-op on POSIX, where the launcher execs). It arms for
	// every Windows wait, whoever started it, so any wait — including one a
	// shell started in the background — now ends when its direct parent
	// exits. A parent that dies before the watch opens it leaves the watch
	// disarmed, and --timeout remains the backstop.
	ctx, cancelOnParentExit := context.WithCancel(ctx)
	defer cancelOnParentExit()
	startParentDeathWatch(func(ppid int) {
		mcp.RecordLifecycleEvent("process.parent_exited", map[string]any{
			"ppid":    ppid,
			"command": "mailbox wait",
			"action":  "cancel_wait",
		})
		cancelOnParentExit()
	})
	os.Exit(runMailboxWait(ctx, args, os.Stdout, os.Stderr, "", os.Getpid(), nil, os.Getenv, nil))
}

// runMailboxWait is `mailbox wait` as a function: the flags in args, the
// result on stdout, diagnostics on stderr, the exit code returned. cwd is
// the caller's working directory, against which a relative --root resolves
// (empty means this process's own); ownerPID owns the listening marker (the
// launcher's PID when the dashboard daemon runs the wait for a launcher,
// this process's otherwise), environ is the caller's environment the
// stores and their git resolve under (nil means this process's), and getenv
// reads it. A ctx that ends cancels the wait at once and clears the marker,
// as a signal does. wake is the hub of a wait the dashboard daemon runs,
// whose mailbox and inbox writes and nudges signal it, so the wait rechecks
// on those and polls only as a backstop; nil (a standalone process, which
// receives no such signal) keeps the short poll.
func runMailboxWait(ctx context.Context, args []string, stdout, stderr io.Writer, cwd string, ownerPID int, environ []string, getenv func(string) string, wake *wswake.Hub) int {
	fs := flag.NewFlagSet("mailbox wait", flag.ContinueOnError)
	fs.SetOutput(stderr)
	root := fs.String("root", ".", "caller's own worktree/clone root; only needed to resolve a worktree/clone-scope --slug")
	sessionKey := fs.String("session-key", "", "caller's own session_key (required): computes the reply-id queue and the named-inbox owner check")
	slug := fs.String("slug", "", `optional explicit "name@scope" to also check the named inbox for (owner-gated); omit for reply-id-only (env-less) wait`)
	timeout := fs.Duration("timeout", 0, "maximum time to block (e.g. 5m); 0 blocks until mail arrives")
	format := fs.String("format", "", `output format: text or json`)
	boardInbox := fs.Bool("board-inbox", false, "also wake on a message the user sent from the dashboard page (held in the session's board-answer inbox); the adapter collects it with the adapter-only dashboard.inbox_drain tool, and this wait only peeks. The result says which kind arrived: mail, board, or both")
	boardSteer := fs.Bool("board-steer", false, "also wake on a dashboard message the user promoted to steering, but not on a held one that was not promoted: the wait an adapter arms during a running turn, when held messages wait for the turn end. Peek only, like --board-inbox, which it cannot be combined with; a steered message reports as board")
	boardInterrupt := fs.Bool("board-interrupt", false, "also wake on an interrupt entry: the user's request from the dashboard page to stop the running turn, held in the session's board-answer inbox. An adapter keeps it armed while dashboard messages are on, idle or during a turn, alone or with --board-inbox or --board-steer; alone it wakes on no held message. Peek only; the adapter collects the entry with dashboard.inbox_drain interrupt_only, and a wake while no turn runs just lets it consume and drop the entry. The result reports it as interrupt, never as board")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	failed := func(err error) int {
		fmt.Fprintf(stderr, "ws-mcp mailbox wait: %v\n", err)
		return 1
	}

	*sessionKey = strings.TrimSpace(*sessionKey)
	if *sessionKey == "" {
		return failed(fmt.Errorf("--session-key is required"))
	}
	if *timeout < 0 {
		return failed(fmt.Errorf("--timeout must be >= 0 (0 blocks until mail arrives)"))
	}
	if *boardInbox && *boardSteer {
		return failed(fmt.Errorf("--board-inbox and --board-steer cannot be combined: --board-inbox already wakes on every held message"))
	}

	waitRoot := defaultRootFrom(*root, getenv)
	if cwd != "" && !filepath.IsAbs(waitRoot) {
		waitRoot = filepath.Join(cwd, waitRoot)
	}
	// The stores follow the caller's environment: the daemon that runs this
	// wait for a launcher has a config and cache home of its own.
	paths := wsmailbox.Paths{
		Config: wsconfig.Options{ConfigHome: getenv(wsconfig.EnvConfigHome), CacheHome: getenv(wsstate.EnvCacheHome)},
		State:  wsstate.Options{CacheHome: getenv(wsstate.EnvCacheHome), Env: environ},
	}
	target := wsmailbox.WaitTarget{SessionKey: *sessionKey, Root: waitRoot, Slug: strings.TrimSpace(*slug), Paths: paths}
	if *boardInbox || *boardSteer || *boardInterrupt {
		stateDir, err := wsdashboard.Dir(getenv(wsstate.EnvCacheHome))
		if err != nil {
			return failed(err)
		}
		target.InboxDir = wsinbox.Dir(wsdashboard.SessionDir(stateDir, wsmailbox.ReplyID(*sessionKey)))
		target.InboxSteerOnly = *boardSteer
		target.InboxInterrupt = *boardInterrupt
		target.InboxInterruptOnly = *boardInterrupt && !*boardInbox && !*boardSteer
	}

	// One-time startup diagnostic (never repeated in the poll loop): warn
	// when an explicit --slug cannot actually be reached, so a wait that
	// silently degrades to reply-id-only does not also silently time out
	// with no explanation while the named inbox it names fills up.
	if target.Slug != "" {
		if present, owned, err := wsmailbox.NamedInboxStatus(target); err != nil {
			fmt.Fprintf(stderr, "ws-mcp mailbox wait: warning: could not resolve --slug %s: %v\n", target.Slug, err)
		} else if !owned {
			reason := "has no presence record yet"
			if present {
				reason = "is not currently owned by this --session-key"
			}
			fmt.Fprintf(stderr, "ws-mcp mailbox wait: warning: named inbox %s %s; falling back to a reply-id-only wait\n", target.Slug, reason)
		}
	}

	nowStr := time.Now().UTC().Format(time.RFC3339)
	marker := wsmailbox.ListeningMarker{SessionKey: *sessionKey, Slug: target.Slug, PID: ownerPID, StartedAt: nowStr}
	if *timeout > 0 {
		marker.TimeoutAt = time.Now().UTC().Add(*timeout).Format(time.RFC3339)
	}
	if err := paths.WriteListeningMarker(marker); err != nil {
		return failed(err)
	}
	// Owner-checked: a replacement wait on the same session_key may already
	// have rewritten the marker, and this exit must not delete it.
	defer func() { _ = paths.ClearListeningMarker(*sessionKey, marker.PID) }()

	// Reprint this invocation's own command name plus the wait-scoping flags
	// it received, so every return path can hand the caller a runnable re-arm
	// command: the arming guidance is read once, but the wait fires much later,
	// so the "re-arm to keep listening" reminder has to arrive at fire time,
	// not only at read time. mcp.CLICommandNameFrom prefers the declared shim
	// name (a wait launched through the shim re-arms through it).
	rearmCmd := buildMailboxWaitRearmCommand(mcp.CLICommandNameFrom(getenv), *sessionKey, target.Slug, *timeout, boardFlags(*boardInbox, *boardSteer, *boardInterrupt))

	result, err := wsmailbox.Wait(target, wsmailbox.WaitOptions{Timeout: *timeout, Done: ctx.Done(), Wake: wake})
	if errors.Is(err, wsmailbox.ErrWaitCancelled) {
		fmt.Fprintln(stderr, "ws-mcp mailbox wait: interrupted")
		return mailboxWaitExitInterrupted
	}
	if err != nil {
		return failed(err)
	}
	return emitMailboxWaitResult(stdout, stderr, result, *format, rearmCmd, *boardInbox || *boardSteer)
}

// emitMailboxWaitResult writes the result to stdout and returns the exit
// code: mailboxWaitExitTimeout on a timeout, 0 on an arrival.
func emitMailboxWaitResult(stdout, stderr io.Writer, result wsmailbox.WaitResult, format, rearmCmd string, boardInbox bool) int {
	// The re-arm reminder rides every return path (mail or timeout): a single
	// wait covers one wake, so the caller must re-run to keep listening. In
	// JSON it is a structured `rearm` object rather than prose, so machine
	// callers see one additive field, not a changed result shape.
	rearm := map[string]string{"command": rearmCmd, "nudge": mailboxRearmNudge}
	writeJSON := func(value any) int {
		encoded, err := json.Marshal(value)
		if err != nil {
			fmt.Fprintf(stderr, "ws-mcp mailbox wait: %v\n", err)
			return 1
		}
		fmt.Fprintln(stdout, string(encoded))
		return -1
	}

	if result.TimedOut {
		if outputJSON(format) {
			if code := writeJSON(map[string]any{
				"timed_out": true, "unread": 0, "board": 0, "interrupt": 0, "kinds": []string{},
				"named": []wsmailbox.Envelope{}, "reply": []wsmailbox.Envelope{},
				"rearm": rearm,
			}); code >= 0 {
				return code
			}
		} else {
			fmt.Fprintln(stdout, "timeout: no unread mail")
			printMailboxWaitRearm(stdout, rearmCmd)
		}
		return mailboxWaitExitTimeout
	}

	total := result.Total()
	// kinds says which kind of arrival woke the wait; a board entry is only
	// peeked here, the adapter drains it.
	kinds := []string{}
	if total > 0 {
		kinds = append(kinds, "mail")
	}
	if result.Board > 0 {
		kinds = append(kinds, "board")
	}
	// An interrupt entry is its own kind, so it is never read as a held
	// board message.
	if result.Interrupt > 0 {
		kinds = append(kinds, "interrupt")
	}
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
		if code := writeJSON(map[string]any{
			"timed_out": false,
			"unread":    total,
			"board":     result.Board,
			"interrupt": result.Interrupt,
			"kinds":     kinds,
			"named":     named,
			"reply":     reply,
			"rearm":     rearm,
		}); code >= 0 {
			return code
		}
		return 0
	}
	fmt.Fprintf(stdout, "unread %d\n", total)
	if boardInbox && result.Board > 0 {
		fmt.Fprintf(stdout, "board %d\n", result.Board)
	}
	if result.Interrupt > 0 {
		fmt.Fprintf(stdout, "interrupt %d\n", result.Interrupt)
	}
	for _, m := range result.Named {
		printMailboxEnvelope(stdout, m)
	}
	for _, m := range result.Reply {
		printMailboxEnvelope(stdout, m)
	}
	printMailboxWaitRearm(stdout, rearmCmd)
	return 0
}

// mailboxRearmNudge is the plain-language reminder printed (text) or carried
// (JSON) on every mailbox wait return: one wait covers a single wake and must
// be re-armed to keep listening. It is delivered when the wait actually
// returns — not only when the arming guidance was first read — because the two
// moments can be far apart.
const mailboxRearmNudge = "one mailbox wait covers a single wake; re-run the re-arm command to keep listening."

// boardFlags are the board-inbox flags a wait was armed with, in a fixed
// order: "--board-inbox" or "--board-steer", then "--board-interrupt".
func boardFlags(boardInbox, boardSteer, boardInterrupt bool) []string {
	var flags []string
	switch {
	case boardInbox:
		flags = append(flags, "--board-inbox")
	case boardSteer:
		flags = append(flags, "--board-steer")
	}
	if boardInterrupt {
		flags = append(flags, "--board-interrupt")
	}
	return flags
}

// buildMailboxWaitRearmCommand reprints the command name (bin, from
// mcp.CLICommandName) plus the wait-scoping flags this invocation received,
// producing a command that re-arms an identical wait. It emits only the
// resolved flags — --session-key (always required), and --slug / --timeout /
// the board flags (boardFlags) when set.
func buildMailboxWaitRearmCommand(bin, sessionKey, slug string, timeout time.Duration, board []string) string {
	parts := []string{bin, "mailbox", "wait", "--session-key", sessionKey}
	if slug != "" {
		parts = append(parts, "--slug", slug)
	}
	if timeout > 0 {
		parts = append(parts, "--timeout", timeout.String())
	}
	parts = append(parts, board...)
	return strings.Join(parts, " ")
}

// printMailboxWaitRearm writes the fire-time re-arm reminder to stdout, after
// the result, on the human/text path only. It rides the same stream as the
// result the harness re-injects into the agent, so the "re-arm to keep
// listening" reminder reaches the woken agent alongside the mail it woke for.
func printMailboxWaitRearm(out io.Writer, rearmCmd string) {
	fmt.Fprintf(out, "\nre-arm: %s\n", rearmCmd)
	fmt.Fprintf(out, "(%s)\n", mailboxRearmNudge)
}

func printMailboxEnvelope(out io.Writer, m wsmailbox.Envelope) {
	handle := m.From
	if handle == "" {
		handle = m.ReplyTo
	}
	fmt.Fprintf(out, "[%s] %s: %s\n", m.SentAt, handle, m.Content)
}

// stopHookMailboxEnv mirrors internal/mcp/mailbox_runtime.go's envMailbox
// constant. Duplicated rather than imported: cmd/ws-mcp does not otherwise
// depend on internal/mcp, and one constant does not justify adding that
// dependency (the same tradeoff internal/wsmailbox/listening.go already
// makes for its session_key pattern). Deliberately WS_MAILBOX only, never
// WS_MAILBOX_AUTO — see the doc comments below. The Codex Stop-hook adapter
// is a bare OS subprocess that inherits the harness-launched environment, so
// it resolves its slug from this variable.
const stopHookMailboxEnv = "WS_MAILBOX"

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
	reason := fs.String("reason", defaultStopHookReason, "instruction text returned to the model when unread mail is pending")
	_ = fs.Parse(args)

	// Always read (and thus drain) stdin before any exit path, including
	// the no-op ones below: exiting first would leave the harness's Stop
	// payload write unread on this end.
	payload, payloadErr := readStopHookPayload[codexStopHookPayload](os.Stdin)

	*slug = strings.TrimSpace(*slug)
	if *slug == "" {
		*slug = strings.TrimSpace(os.Getenv(stopHookMailboxEnv))
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

// defaultStopHookReason is the instruction handed back to the model through
// a Stop hook's decision:block response. It must stay host-neutral (no
// this-repository ticket/path references, per shipped-surface-boundary.md):
// downstream projects install this same plugin text verbatim.
const defaultStopHookReason = "Unread mail is waiting in your mailbox. Call the mailbox recv tool now, then act on what it returns."

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

// readStopHookPayload decodes r's JSON body into a T (a Stop hook payload
// shape), tolerating an empty body (no error, zero-value payload) so an
// ad-hoc or malformed invocation still fails open rather than erroring on a
// merely-empty stdin.
func readStopHookPayload[T any](r io.Reader) (T, error) {
	var payload T
	raw, err := io.ReadAll(r)
	if err != nil {
		return payload, fmt.Errorf("read stop hook payload: %w", err)
	}
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return payload, nil
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return payload, fmt.Errorf("parse stop hook payload: %w", err)
	}
	return payload, nil
}
