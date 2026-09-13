package wsmailbox

import (
	"path/filepath"
	"testing"
)

func withHookPeekTestCacheHome(t *testing.T) {
	t.Helper()
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))
}

func TestPeekNamedInboxUnreadReportsQueueLengthRegardlessOfOwner(t *testing.T) {
	withHookPeekTestCacheHome(t)

	path, err := MachinePath()
	if err != nil {
		t.Fatalf("MachinePath: %v", err)
	}
	if err := WithLock(path, func(store *StoreFile) error {
		store.Presence["alice"] = Presence{Name: "alice", Scope: ScopeMachine, Owner: "someone-else-key", LastSeen: "2026-09-13T00:00:00Z"}
		store.Queues = AppendQueue(store.Queues, "alice", Envelope{Content: "one", SentAt: "2026-09-13T00:00:00Z"})
		store.Queues = AppendQueue(store.Queues, "alice", Envelope{Content: "two", SentAt: "2026-09-13T00:00:01Z"})
		return nil
	}); err != nil {
		t.Fatalf("seed named inbox: %v", err)
	}

	// Deliberately not the presence Owner: the hook-layer check has no
	// session_key to match against, so it must still see both queued
	// messages instead of returning 0 like the owner-gated Wait/Peek would.
	unread, err := PeekNamedInboxUnread("alice@machine", "")
	if err != nil {
		t.Fatalf("PeekNamedInboxUnread: %v", err)
	}
	if unread != 2 {
		t.Fatalf("PeekNamedInboxUnread = %d, want 2", unread)
	}
}

func TestPeekNamedInboxUnreadZeroOnEmptyQueue(t *testing.T) {
	withHookPeekTestCacheHome(t)

	unread, err := PeekNamedInboxUnread("nobody@machine", "")
	if err != nil {
		t.Fatalf("PeekNamedInboxUnread: %v", err)
	}
	if unread != 0 {
		t.Fatalf("PeekNamedInboxUnread = %d, want 0 on an unseeded slug", unread)
	}
}

func TestPeekNamedInboxUnreadRejectsMalformedSlug(t *testing.T) {
	withHookPeekTestCacheHome(t)

	if _, err := PeekNamedInboxUnread("not-a-slug", ""); err == nil {
		t.Fatal("PeekNamedInboxUnread accepted a slug with no @scope suffix")
	}
}
