package wsmailbox

import "time"

// wait.go implements the host-neutral half of the wake path
// (260913-feat-cross-session-mailbox-wake Phase 1, Decision 6): a
// level-triggered blocking check over the durable queues a caller is
// entitled to drain — the owner's name-keyed inbox when a slug is set, and
// always the caller's own reply-id queue — with no MCP tool call and no
// polling at the tool-call layer (Cross-Child Decision 16). The internal
// recheck loop below is the CLI process blocking on itself, which Decision 6
// explicitly permits ("blocking lives in a CLI process... never an MCP
// tool"); it is not the "poll loop" Decision 16 forbids, which is about a
// caller repeatedly re-invoking a tool.
//
// Wait never drains: draining is mailbox.recv's job (server-gated,
// session-state-aware). This file only peeks and reports, so re-invoking the
// agent after Wait returns always goes through the normal recv path with no
// risk of double-consuming a message this CLI process already "saw".

// WaitTarget identifies which durable queues a Wait call should check.
type WaitTarget struct {
	// SessionKey is the caller's own session_key: always used to compute the
	// caller's reply-id queue, and compared against the named inbox's Owner
	// pointer when Slug is set.
	SessionKey string
	// Root is the caller's own canonicalized worktree/clone root, needed to
	// resolve a worktree/clone-scope Slug's store path (see
	// ai-docs/manuals/ws-mcp.md on why process cwd is unreliable). Ignored
	// for a machine-scope Slug or when Slug is empty.
	Root string
	// Slug is an optional explicit "name@scope" to also check the named
	// inbox for. Empty means reply-id-only (the env-less path, wake half of
	// Decision 14): Wait never re-derives a slug from WS_MAILBOX/
	// WS_MAILBOX_AUTO itself, because a fresh CLI process re-reading
	// WS_MAILBOX_AUTO would mint a different random stem than the
	// already-registered server process — the caller passes the address it
	// already knows (from the workflow ambient block or lookup_peers' self
	// entry) instead.
	Slug string
}

// WaitResult is one Wait call's outcome: the envelopes found in each queue
// (never drained), or TimedOut when neither held anything before the
// deadline.
type WaitResult struct {
	TimedOut bool
	Named    []Envelope // from the named inbox, when Slug was set and owned
	Reply    []Envelope // from the caller's own reply-id queue
}

// Total returns the combined unread count across both queues.
func (r WaitResult) Total() int { return len(r.Named) + len(r.Reply) }

// DefaultWaitPoll is Wait's internal recheck interval when WaitOptions.Poll
// is unset. Implementation-chosen default, not a cross-ticket contract.
const DefaultWaitPoll = 500 * time.Millisecond

// WaitOptions configures one Wait call. Now/Sleep are the injectable
// clock/sleep (mirroring mailbox_runtime.go's mailboxNow convention) so
// tests exercise the full blocking-then-wakes-on-arrival path without real
// wall-clock delay.
type WaitOptions struct {
	// Timeout bounds how long Wait blocks with nothing to report. Zero means
	// block until mail arrives (no deadline).
	Timeout time.Duration
	// Poll is the internal recheck interval; <= 0 defaults to
	// DefaultWaitPoll.
	Poll time.Duration
	// Now defaults to time.Now.
	Now func() time.Time
	// Sleep defaults to time.Sleep.
	Sleep func(time.Duration)
}

// Wait performs target's level-triggered check: an immediate peek, returned
// at once if either queue already holds unread mail (Decision 6's lost-
// wakeup avoidance — the arm/drain gap is covered because the store is
// durable and this is a synchronous read on startup, not a wait on a bare
// arrival event), otherwise blocking in Poll-sized increments until mail
// arrives or Timeout elapses.
func Wait(target WaitTarget, opts WaitOptions) (WaitResult, error) {
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	sleep := opts.Sleep
	if sleep == nil {
		sleep = time.Sleep
	}
	poll := opts.Poll
	if poll <= 0 {
		poll = DefaultWaitPoll
	}

	hasDeadline := opts.Timeout > 0
	var deadline time.Time
	if hasDeadline {
		deadline = now().Add(opts.Timeout)
	}

	for {
		result, err := peekWaitTarget(target)
		if err != nil {
			return WaitResult{}, err
		}
		if result.Total() > 0 {
			return result, nil
		}
		if !hasDeadline {
			sleep(poll)
			continue
		}
		remaining := deadline.Sub(now())
		if remaining <= 0 {
			return WaitResult{TimedOut: true}, nil
		}
		if remaining < poll {
			sleep(remaining)
		} else {
			sleep(poll)
		}
	}
}

// peekWaitTarget reads both queues once, without draining either.
func peekWaitTarget(target WaitTarget) (WaitResult, error) {
	named, err := peekNamedInbox(target)
	if err != nil {
		return WaitResult{}, err
	}
	reply, err := peekReplyQueue(target.SessionKey)
	if err != nil {
		return WaitResult{}, err
	}
	return WaitResult{Named: named, Reply: reply}, nil
}

// peekNamedInbox reads target.Slug's queue, gated on target.SessionKey
// currently holding that name's Owner pointer — a pure disk read mirroring
// internal/mcp's server-layer caller==owner check (mailboxOwnerCheck), but
// against an already-loaded store rather than live session state, since a
// standalone CLI process has none. Returns (nil, nil) when Slug is empty,
// the name has no presence yet, or SessionKey is not the current owner.
func peekNamedInbox(target WaitTarget) ([]Envelope, error) {
	if target.Slug == "" {
		return nil, nil
	}
	name, scope, err := ParseSlugScope(target.Slug)
	if err != nil {
		return nil, err
	}
	path, err := PathForScope(scope, target.Root)
	if err != nil {
		return nil, err
	}
	store, err := Load(path)
	if err != nil {
		return nil, err
	}
	p, ok := store.Presence[name]
	if !ok || p.Owner == "" || p.Owner != target.SessionKey {
		return nil, nil
	}
	return store.Queues[name], nil
}

// peekReplyQueue reads sessionKey's own reply-id queue (always checked,
// Decision 6's "always the caller's own reply-id queue" — authorized
// intrinsically by the caller's own session_key, same carve-out
// mailbox.recv applies).
func peekReplyQueue(sessionKey string) ([]Envelope, error) {
	secret, err := EnsureMachineSecret()
	if err != nil {
		return nil, err
	}
	replyID := ReplyID(secret, sessionKey)
	path, err := ReplyRegistryPath()
	if err != nil {
		return nil, err
	}
	store, err := LoadReplyStore(path)
	if err != nil {
		return nil, err
	}
	return store.Queues[replyID], nil
}
