package wsmailbox

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

// TestRecordContactTrimsToNewest pins the correspondence-history bound: an
// upsert past MaxContacts keeps only the MaxContacts newest entries, and
// SortedContacts orders newest first.
func TestRecordContactTrimsToNewest(t *testing.T) {
	var entry ReplyEntry
	for i := 0; i < MaxContacts+5; i++ {
		stamp := time.Date(2026, 10, 22, 0, i, 0, 0, time.UTC).Format(time.RFC3339)
		entry.RecordContact(fmt.Sprintf("id:peer%c", 'a'+i), Contact{LastExchange: stamp})
	}
	if len(entry.Contacts) != MaxContacts {
		t.Fatalf("contacts = %d, want %d", len(entry.Contacts), MaxContacts)
	}
	for i := 0; i < 5; i++ {
		if _, ok := entry.Contacts[fmt.Sprintf("id:peer%c", 'a'+i)]; ok {
			t.Fatalf("oldest contact %d survived the trim: %#v", i, entry.Contacts)
		}
	}
	sorted := SortedContacts(entry.Contacts)
	if sorted[0].Address != fmt.Sprintf("id:peer%c", 'a'+MaxContacts+4) || sorted[len(sorted)-1].Address != "id:peerf" {
		t.Fatalf("SortedContacts not newest first: first=%s last=%s", sorted[0].Address, sorted[len(sorted)-1].Address)
	}

	// Re-recording an existing address refreshes it in place.
	newest := time.Date(2026, 10, 23, 0, 0, 0, 0, time.UTC).Format(time.RFC3339)
	entry.RecordContact("id:peerf", Contact{LastExchange: newest, Harness: "codex"})
	if len(entry.Contacts) != MaxContacts || SortedContacts(entry.Contacts)[0].Address != "id:peerf" {
		t.Fatalf("re-recorded contact not refreshed to newest: %#v", SortedContacts(entry.Contacts)[0])
	}
}
