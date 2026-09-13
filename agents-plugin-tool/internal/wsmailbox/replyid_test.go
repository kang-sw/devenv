package wsmailbox

import (
	"os"
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

// TestEnsureMachineSecretRepairsTruncatedFile verifies the doc-claimed recovery
// in EnsureMachineSecret/readValidSecret: a truncated / wrong-length secret file
// (e.g. a partial write) is repaired by regenerating a fresh valid-length secret
// and persisting it, rather than propagating the bad bytes as if they were a
// real key.
func TestEnsureMachineSecretRepairsTruncatedFile(t *testing.T) {
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	path, err := MachineSecretPath()
	if err != nil {
		t.Fatalf("MachineSecretPath: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	bad := []byte("too-short")
	if len(bad) == secretByteLen {
		t.Fatalf("test fixture is accidentally a valid-length secret")
	}
	if err := os.WriteFile(path, bad, 0o600); err != nil {
		t.Fatal(err)
	}

	secret, err := EnsureMachineSecret()
	if err != nil {
		t.Fatalf("EnsureMachineSecret over a truncated file: %v", err)
	}
	if len(secret) != secretByteLen {
		t.Fatalf("EnsureMachineSecret returned a %d-byte secret, want a repaired %d-byte secret", len(secret), secretByteLen)
	}
	if string(secret) == string(bad) {
		t.Fatalf("EnsureMachineSecret propagated the truncated bytes instead of repairing")
	}

	// The repair is persisted, not just returned: a fresh read sees the valid
	// secret, so the next process does not re-repair.
	reread, err := readValidSecret(path)
	if err != nil {
		t.Fatalf("readValidSecret after repair: %v", err)
	}
	if len(reread) != secretByteLen || string(reread) != string(secret) {
		t.Fatalf("repaired secret was not persisted: reread %d bytes, want the returned %d-byte secret", len(reread), secretByteLen)
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
