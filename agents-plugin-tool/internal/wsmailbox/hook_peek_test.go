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

func seedHookPeekQueue(t *testing.T, name string, envelopes ...Envelope) {
	t.Helper()
	path, err := MachinePath()
	if err != nil {
		t.Fatalf("MachinePath: %v", err)
	}
	if err := WithLock(path, func(store *StoreFile) error {
		store.Presence[name] = Presence{Name: name, Scope: ScopeMachine, Owner: "someone-else-key", LastSeen: "2026-09-13T00:00:00Z"}
		for _, e := range envelopes {
			store.Queues = AppendQueue(store.Queues, name, e)
		}
		return nil
	}); err != nil {
		t.Fatalf("seed named inbox: %v", err)
	}
}

func TestPeekNamedInboxUnreadReportsQueueLengthRegardlessOfOwner(t *testing.T) {
	withHookPeekTestCacheHome(t)

	seedHookPeekQueue(t, "alice",
		Envelope{Content: "one", SentAt: "2026-09-13T00:00:00Z"},
		Envelope{Content: "two", SentAt: "2026-09-13T00:00:01Z"},
	)

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

// TestShouldNotifyNamedInboxUnreadFalseOnEmptyQueue covers the common case:
// nothing queued, nothing to notify about.
func TestShouldNotifyNamedInboxUnreadFalseOnEmptyQueue(t *testing.T) {
	withHookPeekTestCacheHome(t)

	notify, unread, err := ShouldNotifyNamedInboxUnread("nobody@machine", "")
	if err != nil {
		t.Fatalf("ShouldNotifyNamedInboxUnread: %v", err)
	}
	if notify || unread != 0 {
		t.Fatalf("ShouldNotifyNamedInboxUnread = (%v, %d), want (false, 0)", notify, unread)
	}
}

// TestShouldNotifyNamedInboxUnreadBoundsRepeatedFiringAtSameCount is the
// direct regression test for the round-1 correctness finding: an inbox the
// woken session can never drain (unowned, rebound, or simply never read)
// must not get decision:block on every single Stop firing forever.
func TestShouldNotifyNamedInboxUnreadBoundsRepeatedFiringAtSameCount(t *testing.T) {
	withHookPeekTestCacheHome(t)
	seedHookPeekQueue(t, "alice", Envelope{Content: "one", SentAt: "2026-09-13T00:00:00Z"})

	notify1, unread1, err := ShouldNotifyNamedInboxUnread("alice@machine", "")
	if err != nil {
		t.Fatalf("first ShouldNotifyNamedInboxUnread: %v", err)
	}
	if !notify1 || unread1 != 1 {
		t.Fatalf("first firing = (%v, %d), want (true, 1)", notify1, unread1)
	}

	// Same queue, unchanged: the second (and any further) firing at the same
	// count must not notify again.
	for i := 0; i < 3; i++ {
		notify, unread, err := ShouldNotifyNamedInboxUnread("alice@machine", "")
		if err != nil {
			t.Fatalf("repeat firing %d: %v", i, err)
		}
		if notify || unread != 1 {
			t.Fatalf("repeat firing %d = (%v, %d), want (false, 1) — the watermark must suppress a repeat notify at the same count", i, notify, unread)
		}
	}
}

// TestShouldNotifyNamedInboxUnreadRenotifiesOnCountIncrease covers new mail
// arriving on top of an already-notified, still-undrained queue: the count
// changed, so it must notify again even though the queue never emptied.
func TestShouldNotifyNamedInboxUnreadRenotifiesOnCountIncrease(t *testing.T) {
	withHookPeekTestCacheHome(t)
	seedHookPeekQueue(t, "alice", Envelope{Content: "one", SentAt: "2026-09-13T00:00:00Z"})

	if notify, _, err := ShouldNotifyNamedInboxUnread("alice@machine", ""); err != nil || !notify {
		t.Fatalf("first firing = (%v, err=%v), want (true, nil)", notify, err)
	}
	if notify, unread, err := ShouldNotifyNamedInboxUnread("alice@machine", ""); err != nil || notify {
		t.Fatalf("second firing (same count) = (%v, %d, err=%v), want (false, 1, nil)", notify, unread, err)
	}

	seedHookPeekQueue(t, "alice", Envelope{Content: "two", SentAt: "2026-09-13T00:00:01Z"})
	notify, unread, err := ShouldNotifyNamedInboxUnread("alice@machine", "")
	if err != nil {
		t.Fatalf("third firing: %v", err)
	}
	if !notify || unread != 2 {
		t.Fatalf("third firing (count increased) = (%v, %d), want (true, 2)", notify, unread)
	}
}

// TestShouldNotifyNamedInboxUnreadResetsAfterDrain covers the full cycle:
// once the queue empties (drained through mailbox.recv, simulated here by
// clearing the store directly), a later refill notifies again even at a
// count the watermark already saw before the drain.
func TestShouldNotifyNamedInboxUnreadResetsAfterDrain(t *testing.T) {
	withHookPeekTestCacheHome(t)
	seedHookPeekQueue(t, "alice", Envelope{Content: "one", SentAt: "2026-09-13T00:00:00Z"})

	if notify, _, err := ShouldNotifyNamedInboxUnread("alice@machine", ""); err != nil || !notify {
		t.Fatalf("first firing = (%v, err=%v), want (true, nil)", notify, err)
	}

	// Drain: queue goes empty.
	path, err := MachinePath()
	if err != nil {
		t.Fatalf("MachinePath: %v", err)
	}
	if err := WithLock(path, func(store *StoreFile) error {
		store.Queues["alice"] = nil
		return nil
	}); err != nil {
		t.Fatalf("drain named inbox: %v", err)
	}
	if notify, unread, err := ShouldNotifyNamedInboxUnread("alice@machine", ""); err != nil || notify || unread != 0 {
		t.Fatalf("firing on drained queue = (%v, %d, err=%v), want (false, 0, nil)", notify, unread, err)
	}

	// Refill at the exact same count (1) the watermark saw before the
	// drain: must notify again, since the watermark was cleared on drain.
	seedHookPeekQueue(t, "alice", Envelope{Content: "one-more", SentAt: "2026-09-13T00:01:00Z"})
	notify, unread, err := ShouldNotifyNamedInboxUnread("alice@machine", "")
	if err != nil {
		t.Fatalf("firing after refill: %v", err)
	}
	if !notify || unread != 1 {
		t.Fatalf("firing after refill = (%v, %d), want (true, 1)", notify, unread)
	}
}

func TestShouldNotifyNamedInboxUnreadRejectsMalformedSlug(t *testing.T) {
	withHookPeekTestCacheHome(t)

	if _, _, err := ShouldNotifyNamedInboxUnread("not-a-slug", ""); err == nil {
		t.Fatal("ShouldNotifyNamedInboxUnread accepted a slug with no @scope suffix")
	}
}
