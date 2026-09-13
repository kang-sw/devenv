package wsmailbox

// hook_peek.go implements the Codex Stop-hook adapter's coarse,
// ownership-agnostic mailbox check (260913-feat-cross-session-mailbox-wake
// Phase 2, Decision 9). A Codex Stop hook runs as a bare OS subprocess: it
// carries a slug (baked into the hook command's args from the harness
// process's own inherited WS_MAILBOX/WS_MAILBOX_AUTO env at arm time) but no
// ws session_key, so it cannot perform wait.go's owner-matched check the
// way a `mailbox wait --session-key` CLI invocation can.
//
// This is deliberately best-effort: it answers only "does this slug's queue
// currently hold anything," never "is the caller entitled to read it." The
// real caller==owner enforcement remains mailbox.recv's server-side gate,
// exercised once the woken agent actually drains — this function only
// decides whether the Stop hook has a reason to ask the model to do that.

// PeekNamedInboxUnread reports how many messages are currently queued for
// slug's named inbox, regardless of which session (if any) presence
// currently records as its owner. root resolves a worktree/clone-scope
// slug; ignored for machine scope.
func PeekNamedInboxUnread(slug, root string) (int, error) {
	name, scope, err := ParseSlugScope(slug)
	if err != nil {
		return 0, err
	}
	path, err := PathForScope(scope, root)
	if err != nil {
		return 0, err
	}
	store, err := Load(path)
	if err != nil {
		return 0, err
	}
	return len(store.Queues[name]), nil
}
