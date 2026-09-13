package wsmailbox

import (
	"path/filepath"
	"testing"
)

func TestEnsureMachineSecretPersistsAndReturnsSameValue(t *testing.T) {
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	first, err := EnsureMachineSecret()
	if err != nil {
		t.Fatalf("EnsureMachineSecret: %v", err)
	}
	if len(first) != secretByteLen {
		t.Fatalf("secret length = %d, want %d", len(first), secretByteLen)
	}
	second, err := EnsureMachineSecret()
	if err != nil {
		t.Fatalf("EnsureMachineSecret (second call): %v", err)
	}
	if string(first) != string(second) {
		t.Fatalf("EnsureMachineSecret returned different values across calls")
	}
}

func TestReplyIDDeterministicAndSensitiveToSessionKey(t *testing.T) {
	secret := []byte("0123456789abcdef0123456789abcdef")

	a1 := ReplyID(secret, "amber-tide-fox")
	a2 := ReplyID(secret, "amber-tide-fox")
	if a1 != a2 {
		t.Fatalf("ReplyID not deterministic: %s != %s", a1, a2)
	}
	if !IsValidReplyID(a1) {
		t.Fatalf("ReplyID output %q does not match IsValidReplyID", a1)
	}

	b := ReplyID(secret, "other-session-key")
	if a1 == b {
		t.Fatalf("ReplyID collided across different session keys")
	}

	otherSecret := []byte("fedcba9876543210fedcba9876543210")
	c := ReplyID(otherSecret, "amber-tide-fox")
	if a1 == c {
		t.Fatalf("ReplyID collided across different secrets")
	}
}

func TestReplyStoreRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mailbox-replyids.json")

	replyID := "a1b2c3"
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
