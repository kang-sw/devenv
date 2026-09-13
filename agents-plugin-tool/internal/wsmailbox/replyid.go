package wsmailbox

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/gofrs/flock"

	"github.com/kang-sw/devenv/internal/wsstate"
)

// This file implements Decision 11's always-on reply-id return channel: a
// machine-wide secret plus a deterministic HMAC of the caller's own
// session_key, and the machine-tier registry (presence-less; entries carry
// only a last-seen stamp) + per-reply-id queue that "id:<reply-id>"
// addressing (Decision 12) delivers into.
//
// Both live at the machine-global session-key cache root
// (wsstate.CacheRoot, sibling to keys/), NOT under the wsnote "machine"
// layer root (~/.ws): the reply-id channel is anchored to caller_session_key
// (Decision 13), the same machine-global identity space keys/ already
// owns, and Route Facts for this ticket name that cache root explicitly as
// the machine_secret's home.

const (
	secretFileName    = "mailbox-secret"
	replyRegistryName = "mailbox-replyids.json"
	secretByteLen     = 32
)

// MachineSecretPath resolves the once-per-machine mailbox HMAC secret file
// path.
func MachineSecretPath() (string, error) {
	root, err := wsstate.CacheRoot(wsstate.Options{})
	if err != nil {
		return "", err
	}
	return filepath.Join(root, secretFileName), nil
}

// EnsureMachineSecret reads the persisted machine secret, minting it on
// first use. Minting (and recovery from a truncated/corrupt file) is
// serialized with the same flock convention WithLock/WithReplyLock use: an
// O_EXCL create can only ever claim a MISSING file, so a second process
// racing against a truncated/corrupt existing file would fall through to
// the "file already exists" branch and re-read + return the very same bad
// bytes, unvalidated. flock covers both cases uniformly, and the actual
// write is a temp-file + atomic rename, matching the package's own
// StoreFile/ReplyStore write discipline instead of a bare in-place write.
func EnsureMachineSecret() ([]byte, error) {
	path, err := MachineSecretPath()
	if err != nil {
		return nil, err
	}
	if raw, err := readValidSecret(path); raw != nil || err != nil {
		return raw, err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create mailbox secret dir: %w", err)
	}
	lockPath := path + ".lock"
	fl := flock.New(lockPath)
	ctx, cancel := context.WithTimeout(context.Background(), LockTimeout)
	defer cancel()
	locked, err := fl.TryLockContext(ctx, 50*time.Millisecond)
	if err != nil {
		return nil, fmt.Errorf("acquire mailbox secret lock: %w", err)
	}
	if !locked {
		return nil, fmt.Errorf("timed out waiting for mailbox secret lock: %s", lockPath)
	}
	defer fl.Unlock() //nolint:errcheck

	// Re-check under the lock: another process may have minted or repaired
	// it while we were waiting to acquire the lock.
	if raw, err := readValidSecret(path); raw != nil || err != nil {
		return raw, err
	}

	secret := make([]byte, secretByteLen)
	if _, err := rand.Read(secret); err != nil {
		return nil, fmt.Errorf("generate mailbox secret: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, secret, 0o600); err != nil {
		return nil, fmt.Errorf("write mailbox secret temp file: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return nil, fmt.Errorf("commit mailbox secret file: %w", err)
	}
	return secret, nil
}

// readValidSecret reads path and returns its contents when present and
// exactly secretByteLen long. A missing file or a wrong-length (truncated/
// corrupt) file both return (nil, nil) — "not valid yet, mint/repair it" —
// distinct from a genuine read error.
func readValidSecret(path string) ([]byte, error) {
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read mailbox secret: %w", err)
	}
	if len(raw) != secretByteLen {
		return nil, nil
	}
	return raw, nil
}

// ReplyID computes the deterministic, unguessable reply-id for
// callerSessionKey: hex(HMAC-SHA256(secret, callerSessionKey)). Same secret
// + same session_key always yields the same reply-id (Decision 11's
// restart-stability: no reply-id itself is ever persisted), and the raw
// session_key — the owner-gate secret — never appears in the output.
func ReplyID(secret []byte, callerSessionKey string) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(callerSessionKey))
	return hex.EncodeToString(mac.Sum(nil))
}

// ReplyEntry is one reply-id's registry record: presence-equivalent, but
// without name/scope/owner (a reply-id has neither) — the machine-tier
// registry knows a reply-id is "live" purely from LastSeen recency,
// refreshed on every send / session-aware call that touches it (Decision
// 11's lazy expiry).
type ReplyEntry struct {
	LastSeen string `json:"last_seen"`
}

// ReplyStore is the machine-tier reply-id registry: one entry + one queue
// per reply-id, keyed by the hex ReplyID string.
type ReplyStore struct {
	SchemaVersion int                   `json:"schema_version"`
	Entries       map[string]ReplyEntry `json:"entries,omitempty"`
	Queues        map[string][]Envelope `json:"queues,omitempty"`
}

// ReplyRegistryPath resolves the machine-tier reply-id registry file path.
func ReplyRegistryPath() (string, error) {
	root, err := wsstate.CacheRoot(wsstate.Options{})
	if err != nil {
		return "", err
	}
	return filepath.Join(root, replyRegistryName), nil
}

// LoadReplyStore reads the reply-id registry. A missing file returns an
// empty, non-nil ReplyStore.
func LoadReplyStore(path string) (ReplyStore, error) {
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return emptyReplyStore(), nil
	}
	if err != nil {
		return ReplyStore{}, fmt.Errorf("read mailbox reply registry %s: %w", path, err)
	}
	var store ReplyStore
	if err := json.Unmarshal(raw, &store); err != nil {
		return ReplyStore{}, fmt.Errorf("parse mailbox reply registry %s: %w", path, err)
	}
	if store.Entries == nil {
		store.Entries = map[string]ReplyEntry{}
	}
	if store.Queues == nil {
		store.Queues = map[string][]Envelope{}
	}
	return store, nil
}

func emptyReplyStore() ReplyStore {
	return ReplyStore{SchemaVersion: schemaVersion, Entries: map[string]ReplyEntry{}, Queues: map[string][]Envelope{}}
}

// WithReplyLock performs an flock-serialized read-modify-write on the
// reply-id registry at path, mirroring WithLock's store.go pattern exactly
// (separate function only because the payload type differs).
func WithReplyLock(path string, fn func(*ReplyStore) error) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create mailbox reply registry dir: %w", err)
	}
	lockPath := path + ".lock"
	fl := flock.New(lockPath)
	ctx, cancel := context.WithTimeout(context.Background(), LockTimeout)
	defer cancel()
	locked, err := fl.TryLockContext(ctx, 50*time.Millisecond)
	if err != nil {
		return fmt.Errorf("acquire mailbox reply registry lock: %w", err)
	}
	if !locked {
		return fmt.Errorf("timed out waiting for mailbox reply registry lock: %s", lockPath)
	}
	defer fl.Unlock() //nolint:errcheck

	current, err := LoadReplyStore(path)
	if err != nil {
		return err
	}
	if err := fn(&current); err != nil {
		return err
	}
	if current.SchemaVersion == 0 {
		current.SchemaVersion = schemaVersion
	}

	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+"-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp mailbox reply registry: %w", err)
	}
	tmpName := tmp.Name()
	payload, err := json.MarshalIndent(current, "", "  ")
	if err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("encode mailbox reply registry: %w", err)
	}
	payload = append(payload, '\n')
	if _, werr := tmp.Write(payload); werr != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("write temp mailbox reply registry: %w", werr)
	}
	if cerr := tmp.Close(); cerr != nil {
		os.Remove(tmpName)
		return fmt.Errorf("close temp mailbox reply registry: %w", cerr)
	}
	if rerr := os.Rename(tmpName, path); rerr != nil {
		os.Remove(tmpName)
		return fmt.Errorf("atomic rename mailbox reply registry: %w", rerr)
	}
	return nil
}
