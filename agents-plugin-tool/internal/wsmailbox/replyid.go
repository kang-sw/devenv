package wsmailbox

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofrs/flock"

	"github.com/kang-sw/devenv/internal/wskey"
	"github.com/kang-sw/devenv/internal/wsstate"
)

// The always-on reply channel uses a deterministic session-derived handle.
// Its flat registry and queues live at the machine-global session-key cache
// root (wsstate.CacheRoot, sibling to keys/), independently of named scopes.
const replyRegistryName = "mailbox-replyids.json"

// ReplyID concatenates four deterministic full-pool words. Handles are local
// routable identifiers, not collision-free or cryptographic capabilities.
func ReplyID(callerSessionKey string) string {
	return strings.ReplaceAll(wskey.DeriveFull(callerSessionKey, 4), "-", "")
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
// per reply-id, keyed by the readable ReplyID string.
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
	if rerr := replaceFile(tmpName, path); rerr != nil {
		os.Remove(tmpName)
		return fmt.Errorf("atomic rename mailbox reply registry: %w", rerr)
	}
	return nil
}
