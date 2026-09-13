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

// mailboxPresenceLive's restart-false-positive fix delegates its actual
// process-liveness syscall logic to wsstate.ProcessAlive (see
// mailbox_runtime.go's doc comment on mailboxPresenceLive for why: a
// hand-rolled probe here previously had a platform-specific bug this
// package cannot re-introduce without noticing). That primitive's own
// liveness behavior — including the exited-PID case this fix depends on —
// is exercised directly in internal/wsstate's
// TestProcessAliveDetectsLiveAndExitedProcess, not duplicated here.
