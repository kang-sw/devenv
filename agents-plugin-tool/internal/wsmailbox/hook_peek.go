package wsmailbox

// hook_peek.go implements a Stop-hook adapter's coarse, ownership-agnostic
// mailbox check (260913-feat-cross-session-mailbox-wake Phase 2/3,
// Decision 9), shared by both the Codex and Claude adapters
// (cmd/ws-mcp/mailbox.go's mailboxCodexStopHook and mailboxClaudeStopHook).
// A Stop hook runs as a bare OS subprocess on either host: it reads a slug
// from its own inherited WS_MAILBOX environment (the cmd/ws-mcp CLI layer's
// job, not this package's) but carries no ws session_key, so it cannot
// perform wait.go's owner-matched check the way a `mailbox wait
// --session-key` CLI invocation can.
//
// This is deliberately best-effort: it answers only "does this slug's queue
// currently hold anything," never "is the caller entitled to read it." The
// real caller==owner enforcement remains mailbox.recv's server-side gate,
// exercised once the woken agent actually drains — this file only decides
// whether the Stop hook has a reason to ask the model to do that, and bounds
// how often it asks.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

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

// hookNotifyDirName is the on-disk subdirectory, sibling of the mailbox
// store file itself, holding ShouldNotifyNamedInboxUnread's per-name
// watermark files. The name predates the Claude adapter (Phase 2 landed
// first) and is kept as-is rather than renamed: the watermark is genuinely
// shared by both Stop-hook adapters for the same slug (there is only ever
// one queue to debounce notifications against, regardless of which harness
// is asking), and a rename would only churn the on-disk path with no
// behavior change.
const hookNotifyDirName = "mailbox-codex-stop-notified"

// hookNotifyState is one slug's watermark: the queue length as of the last
// time ShouldNotifyNamedInboxUnread returned true for it.
type hookNotifyState struct {
	Count int `json:"count"`
}

// hookNotifyStatePath resolves the watermark file path for name, given
// storeDir — the directory of the SAME resolved store file
// ShouldNotifyNamedInboxUnread just read (filepath.Dir of PathForScope's
// result), not a separate machine-global cache root.
//
// This must key off the actual resolved store directory rather than a
// fixed machine-wide root: for ScopeWorktree/ScopeClone, PathForScope
// resolves a *different* physical store per caller-supplied root (each
// worktree/clone gets its own file — see store.go's WorktreePath/
// ClonePath), so two different worktree roots that both happen to use the
// same mailbox name (e.g. "lead@worktree", the common per-role naming
// pattern this scope exists to support) would otherwise share one
// watermark file despite tracking two independent queues. A prior version
// of this function built the path from a fixed wsstate.CacheRoot()
// instead, keyed only on (scope, name); round-2 review caught that this
// silently reopens the exact "unbounded re-block" failure mode Critical-2
// fixed, just across roots instead of across owners: two same-length,
// never-draining roots would suppress each other's genuine notification,
// or two different-length roots would alternate-fire forever. Deriving the
// watermark path from storeDir instead means it is automatically as
// root-scoped as the store it tracks, with no extra bookkeeping — scope no
// longer needs to appear in the filename either, since storeDir already
// disambiguates every scope (machine's is the single global config
// directory; a worktree/clone's is that root's own per-project cache
// directory) as well as every distinct root within worktree/clone scope.
func hookNotifyStatePath(storeDir, name string) string {
	return filepath.Join(storeDir, hookNotifyDirName, name+".json")
}

// hookNotifyNameSafe re-validates that name alone could not have produced a
// path outside storeDir, defending the same class of bug listening.go's
// listeningKeyPattern guards against even though name is already
// regex-bounded by ParseSlugScope. Cheap and cannot fail for a value
// ParseSlugScope accepted; kept as a named check so a future change to
// namePattern does not silently reopen a traversal.
var hookNotifyNameSafe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)

// ShouldNotifyNamedInboxUnread reports whether a Codex Stop hook should emit
// decision:block for slug right now, and advances slug's on-disk watermark
// exactly when it returns true. True only when the named inbox currently
// holds mail AND its queue length differs from the length as of the last
// time this function returned true for the same slug.
//
// Without this bound, a slug whose owner presence is unset, rebound to
// another session, or simply never drained would get a decision:block on
// every single Stop firing forever — the "worse than a missed wake" outcome
// this whole adapter is supposed to avoid (Decision 7), because the woken
// session may be structurally unable to satisfy the instruction
// (mailbox.recv only drains for the current owner). Bounding to one
// notification per queue-length change still re-notifies on genuinely new
// mail, and clears the watermark once the queue empties so a later refill at
// any count notifies again.
func ShouldNotifyNamedInboxUnread(slug, root string) (bool, int, error) {
	name, scope, err := ParseSlugScope(slug)
	if err != nil {
		return false, 0, err
	}
	path, err := PathForScope(scope, root)
	if err != nil {
		return false, 0, err
	}
	store, err := Load(path)
	if err != nil {
		return false, 0, err
	}
	unread := len(store.Queues[name])

	if !hookNotifyNameSafe.MatchString(name) {
		return false, unread, fmt.Errorf("wsmailbox: invalid mailbox name %q for a notify watermark path", name)
	}
	statePath := hookNotifyStatePath(filepath.Dir(path), name)

	if unread == 0 {
		// Drained (or never had anything): clear the watermark so a future
		// arrival at any count — including the same count as before —
		// notifies again instead of being mistaken for the same unread run.
		if rerr := os.Remove(statePath); rerr != nil && !os.IsNotExist(rerr) {
			return false, 0, fmt.Errorf("clear mailbox codex-stop-hook watermark: %w", rerr)
		}
		return false, 0, nil
	}

	last, ok, err := readHookNotifyState(statePath)
	if err != nil {
		return false, unread, err
	}
	if ok && last.Count == unread {
		return false, unread, nil
	}
	if err := writeHookNotifyState(statePath, hookNotifyState{Count: unread}); err != nil {
		return false, unread, err
	}
	return true, unread, nil
}

func readHookNotifyState(path string) (hookNotifyState, bool, error) {
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return hookNotifyState{}, false, nil
	}
	if err != nil {
		return hookNotifyState{}, false, fmt.Errorf("read mailbox codex-stop-hook watermark: %w", err)
	}
	var state hookNotifyState
	if err := json.Unmarshal(raw, &state); err != nil {
		// A corrupt watermark must not wedge the hook into permanent
		// silence or a crash; treat it as "no watermark yet" so this
		// firing still gets a chance to notify.
		return hookNotifyState{}, false, nil
	}
	return state, true, nil
}

func writeHookNotifyState(path string, state hookNotifyState) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create mailbox codex-stop-hook watermark dir: %w", err)
	}
	payload, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("encode mailbox codex-stop-hook watermark: %w", err)
	}
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+"-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp mailbox codex-stop-hook watermark: %w", err)
	}
	tmpName := tmp.Name()
	if _, werr := tmp.Write(payload); werr != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("write temp mailbox codex-stop-hook watermark: %w", werr)
	}
	if cerr := tmp.Close(); cerr != nil {
		os.Remove(tmpName)
		return fmt.Errorf("close temp mailbox codex-stop-hook watermark: %w", cerr)
	}
	if rerr := os.Rename(tmpName, path); rerr != nil {
		os.Remove(tmpName)
		return fmt.Errorf("atomic rename mailbox codex-stop-hook watermark: %w", rerr)
	}
	return nil
}
