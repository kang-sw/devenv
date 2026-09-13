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

	// Resolve once, outside the loop: for a worktree/clone Slug,
	// PathForScope resolves through wsnote/wsstate, which shells out to git
	// and rewrites project/worktree metadata (see ai-docs/manuals/ws-mcp.md
	// on why process cwd alone cannot stand in for this). Re-resolving on
	// every poll tick would spawn a git process and rewrite metadata every
	// DefaultWaitPoll for the whole armed-wait lifetime (unbounded with
	// Timeout == 0), and would also mean one transient resolution failure
	// hours into an idle wait kills the wake path outright. Only the cheap
	// Load/LoadReplyStore disk reads repeat per tick below.
	resolved, err := resolveWaitTarget(target)
	if err != nil {
		return WaitResult{}, err
	}

	for {
		result, err := resolved.peek()
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

// resolvedWaitTarget holds the one-time-resolved paths/identity a Wait
// call's recheck loop needs, computed once by resolveWaitTarget.
type resolvedWaitTarget struct {
	sessionKey string
	namedPath  string // "" when target.Slug was empty
	namedName  string
	replyPath  string
	replyID    string
}

// resolveWaitTarget performs every resolution step that only needs to run
// once per Wait call: parsing/resolving an explicit Slug's store path, and
// deriving the caller's reply-id + registry path.
func resolveWaitTarget(target WaitTarget) (resolvedWaitTarget, error) {
	r := resolvedWaitTarget{sessionKey: target.SessionKey}

	if target.Slug != "" {
		name, scope, err := ParseSlugScope(target.Slug)
		if err != nil {
			return resolvedWaitTarget{}, err
		}
		path, err := PathForScope(scope, target.Root)
		if err != nil {
			return resolvedWaitTarget{}, err
		}
		r.namedName = name
		r.namedPath = path
	}

	secret, err := EnsureMachineSecret()
	if err != nil {
		return resolvedWaitTarget{}, err
	}
	r.replyID = ReplyID(secret, target.SessionKey)
	replyPath, err := ReplyRegistryPath()
	if err != nil {
		return resolvedWaitTarget{}, err
	}
	r.replyPath = replyPath
	return r, nil
}

// peek reads both queues once from their already-resolved paths, without
// draining either.
func (r resolvedWaitTarget) peek() (WaitResult, error) {
	var named []Envelope
	if r.namedPath != "" {
		store, err := Load(r.namedPath)
		if err != nil {
			return WaitResult{}, err
		}
		if p, ok := store.Presence[r.namedName]; ok && p.Owner != "" && p.Owner == r.sessionKey {
			named = store.Queues[r.namedName]
		}
	}

	replyStore, err := LoadReplyStore(r.replyPath)
	if err != nil {
		return WaitResult{}, err
	}
	return WaitResult{Named: named, Reply: replyStore.Queues[r.replyID]}, nil
}

// NamedInboxStatus is a one-time startup diagnostic (never part of the
// recheck loop): it reports whether target.Slug currently has a presence
// record and is owned by target.SessionKey, so a CLI caller can warn when an
// armed --slug wait cannot actually reach the named inbox it names instead
// of degrading to a reply-id-only wait with no explanation (an unbound,
// stale, or wrong-root slug otherwise times out silently reporting "no
// unread mail" while the named inbox it was meant to watch fills up).
// Returns present=false, owned=false, err=nil when target.Slug is empty.
func NamedInboxStatus(target WaitTarget) (present, owned bool, err error) {
	if target.Slug == "" {
		return false, false, nil
	}
	name, scope, err := ParseSlugScope(target.Slug)
	if err != nil {
		return false, false, err
	}
	path, err := PathForScope(scope, target.Root)
	if err != nil {
		return false, false, err
	}
	store, err := Load(path)
	if err != nil {
		return false, false, err
	}
	p, ok := store.Presence[name]
	if !ok {
		return false, false, nil
	}
	return true, p.Owner != "" && p.Owner == target.SessionKey, nil
}
