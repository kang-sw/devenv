package wsmailbox

// hook_peek.go implements the Codex Stop-hook adapter's coarse,
// ownership-agnostic mailbox check (260913-feat-cross-session-mailbox-wake
// Phase 2, Decision 9). A Codex Stop hook runs as a bare OS subprocess: it
// reads a slug from its own inherited WS_MAILBOX environment (the cmd/ws-mcp
// CLI layer's job, not this package's) but carries no ws session_key, so it
// cannot perform wait.go's owner-matched check the way a
// `mailbox wait --session-key` CLI invocation can.
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

	"github.com/kang-sw/devenv/internal/wsstate"
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

// hookNotifyDirName is the on-disk directory for ShouldNotifyNamedInboxUnread's
// per-slug watermark files, a sibling of listening.go's marker directory
// under the same cache root.
const hookNotifyDirName = "mailbox-codex-stop-notified"

// hookNotifyState is one slug's watermark: the queue length as of the last
// time ShouldNotifyNamedInboxUnread returned true for it.
type hookNotifyState struct {
	Count int `json:"count"`
}

// hookNotifyStatePath resolves the watermark file path for a validated
// (name, scope) pair. Both come from ParseSlugScope, which already bounds
// name to namePattern and scope to the three-value enum, so the joined
// "<scope>-<name>.json" filename needs no separate path-safety regex the
// way listening.go's caller-supplied session_key does.
func hookNotifyStatePath(name string, scope Scope) (string, error) {
	root, err := wsstate.CacheRoot(wsstate.Options{})
	if err != nil {
		return "", err
	}
	return filepath.Join(root, hookNotifyDirName, string(scope)+"-"+name+".json"), nil
}

// notifyStateFileSafe re-validates that name/scope alone could not have
// produced a path outside the cache root, defending the same class of bug
// listening.go's listeningKeyPattern guards against even though both inputs
// here are already regex-bounded by ParseSlugScope. Cheap and cannot fail
// for a value ParseSlugScope accepted; kept as a named check so a future
// change to namePattern/Scope does not silently reopen a traversal.
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
	statePath, err := hookNotifyStatePath(name, scope)
	if err != nil {
		return false, unread, err
	}

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
