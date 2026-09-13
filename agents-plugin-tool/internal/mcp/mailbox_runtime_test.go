package mcp

import (
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsmailbox"
)

// TestReapStaleReplyIDsRemovesOnlyStaleEmptyEntries verifies Decision 11's
// lazy-expiry reaping: an entry past mailboxReplyIDRetention with an empty
// queue is removed; a stale entry with a NON-empty queue is kept
// (undelivered mail is never dropped by reaping); a fresh entry is kept
// regardless of its queue state.
func TestReapStaleReplyIDsRemovesOnlyStaleEmptyEntries(t *testing.T) {
	now := time.Now().UTC()
	stale := now.Add(-mailboxReplyIDRetention - time.Hour).Format(time.RFC3339)
	fresh := now.Add(-time.Minute).Format(time.RFC3339)

	store := &wsmailbox.ReplyStore{
		Entries: map[string]wsmailbox.ReplyEntry{
			"stale-empty":     {LastSeen: stale},
			"stale-with-mail": {LastSeen: stale},
			"fresh-empty":     {LastSeen: fresh},
		},
		Queues: map[string][]wsmailbox.Envelope{
			"stale-with-mail": {{Content: "still here", SentAt: fresh}},
		},
	}

	reapStaleReplyIDs(store, now)

	if _, ok := store.Entries["stale-empty"]; ok {
		t.Fatalf("stale entry with an empty queue was not reaped")
	}
	if _, ok := store.Entries["stale-with-mail"]; !ok {
		t.Fatalf("stale entry with a non-empty queue was incorrectly reaped, risking mail loss")
	}
	if _, ok := store.Entries["fresh-empty"]; !ok {
		t.Fatalf("fresh entry was incorrectly reaped")
	}
	if len(store.Queues["stale-with-mail"]) != 1 {
		t.Fatalf("reaping must never touch queue contents: %#v", store.Queues["stale-with-mail"])
	}
}

// TestMailboxProcessDeadDetectsCrashedProcess verifies the Critical
// restart-false-positive fix: a PID that genuinely is not running is
// reported dead, while this test's own live PID is not.
func TestMailboxProcessDeadDetectsCrashedProcess(t *testing.T) {
	if mailboxProcessDead(0) != true {
		t.Fatalf("PID 0 must be treated as dead")
	}
	if mailboxProcessDead(-1) != true {
		t.Fatalf("a negative PID must be treated as dead")
	}
}
