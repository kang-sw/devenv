package wsmailbox

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wskey"
)

func TestReplyStoreRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mailbox-replyids.json")
	replyID := ReplyID("amber-tide-fox")
	err := WithReplyLock(path, func(store *ReplyStore) error {
		store.Entries[replyID] = ReplyEntry{LastSeen: "2026-09-13T00:00:00Z"}
		store.Queues = AppendQueue(store.Queues, replyID, Envelope{Content: "reply", SentAt: "2026-09-13T00:00:00Z"})
		return nil
	})
	if err != nil {
		t.Fatalf("WithReplyLock: %v", err)
	}
	store, err := LoadReplyStore(path)
	if err != nil {
		t.Fatalf("LoadReplyStore: %v", err)
	}
	if store.Entries[replyID].LastSeen == "" {
		t.Fatalf("Entries[%s] missing after round trip", replyID)
	}
	if len(store.Queues[replyID]) != 1 {
		t.Fatalf("Queues[%s] = %#v, want 1 entry", replyID, store.Queues[replyID])
	}
}

func TestReplyIDDeterministicWithoutPersistence(t *testing.T) {
	cache := filepath.Join(t.TempDir(), "uncreated")
	t.Setenv("WS_CACHE_HOME", cache)
	for key, want := range map[string]string{
		"amber-tide-fox":    "headlessroutineremodelerpassage",
		"other-session-key": "staunchmarriedungluetrapezoid",
	} {
		got := ReplyID(key)
		if got != want || got != ReplyID(key) || got != strings.ReplaceAll(wskey.DeriveFull(key, 4), "-", "") || !IsValidReplyID(got) {
			t.Fatalf("ReplyID(%q) = %q", key, got)
		}
	}
	if _, err := os.Stat(cache); !os.IsNotExist(err) {
		t.Fatalf("pure derivation touched cache: %v", err)
	}
}
