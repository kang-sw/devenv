package wsmailbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/gofrs/flock"

	"github.com/kang-sw/devenv/internal/wskey"
	"github.com/kang-sw/devenv/internal/wsstate"
)

// The always-on reply channel uses a deterministic session-derived handle.
// Its flat registry and queues live at the machine-global session-key cache
// root (wsstate.CacheRoot, sibling to sessions/), independently of named scopes.
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
//
// Harness/Cwd describe the registering session (descriptive only, never
// gating), so a correspondent that knows this reply-id can tell sessions
// apart. Contacts is this session's own correspondence history, keyed by
// the literal counterparty address ("id:<reply-id>" or "slug@scope"); it is
// reaped together with the entry and bounded by MaxContacts.
type ReplyEntry struct {
	LastSeen string             `json:"last_seen"`
	Harness  string             `json:"harness,omitempty"`
	Cwd      string             `json:"cwd,omitempty"`
	Contacts map[string]Contact `json:"contacts,omitempty"`
}

// Contact is one counterparty the owning session exchanged mail with.
// LastExchange is the owning session's own send/recv time, not the
// envelope's sent_at; Harness/Cwd are the counterparty's metadata as known
// at that exchange (empty when unknown).
type Contact struct {
	LastExchange string `json:"last_exchange"`
	Harness      string `json:"harness,omitempty"`
	Cwd          string `json:"cwd,omitempty"`
}

// MaxContacts bounds one entry's correspondence history: a long-lived
// session's entry is never reaped, so the history is trimmed to the most
// recent MaxContacts on every write.
const MaxContacts = 20

// ContactRecord is one Contacts entry with its address key, as returned by
// SortedContacts.
type ContactRecord struct {
	Address string
	Contact
}

// SortedContacts returns contacts ordered by LastExchange newest first, ties
// broken by address for determinism. LastExchange values are RFC3339 UTC
// stamps, so lexical order is chronological order.
func SortedContacts(contacts map[string]Contact) []ContactRecord {
	out := make([]ContactRecord, 0, len(contacts))
	for addr, c := range contacts {
		out = append(out, ContactRecord{Address: addr, Contact: c})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].LastExchange != out[j].LastExchange {
			return out[i].LastExchange > out[j].LastExchange
		}
		return out[i].Address < out[j].Address
	})
	return out
}

// RecordContact upserts address into e.Contacts and trims the history to
// the MaxContacts most recent entries.
//
// The map is replaced, never written in place: the memory store shares an
// entry's Contacts map between its committed state and a transaction's
// working copy (memstore.go), so an in-place write would leak into both.
func (e *ReplyEntry) RecordContact(address string, c Contact) {
	contacts := make(map[string]Contact, len(e.Contacts)+1)
	maps.Copy(contacts, e.Contacts)
	e.Contacts = contacts
	e.Contacts[address] = c
	if len(e.Contacts) <= MaxContacts {
		return
	}
	for _, rec := range SortedContacts(e.Contacts)[MaxContacts:] {
		delete(e.Contacts, rec.Address)
	}
}

// ReplyStore is the machine-tier reply-id registry: one entry + one queue
// per reply-id, keyed by the readable ReplyID string.
type ReplyStore struct {
	SchemaVersion int `json:"schema_version"`
	// ExportMark: see StoreFile.ExportMark.
	ExportMark string                `json:"memory_store_export,omitempty"`
	Entries    map[string]ReplyEntry `json:"entries,omitempty"`
	Queues     map[string][]Envelope `json:"queues,omitempty"`
}

// ReplyRegistry resolves the machine-tier reply-id registry file path
// under p's cache root.
func (p Paths) ReplyRegistry() (string, error) {
	root, err := wsstate.CacheRoot(p.State)
	if err != nil {
		return "", err
	}
	return filepath.Join(root, replyRegistryName), nil
}

// ReplyRegistryPath is Paths{}.ReplyRegistry: this process's environment.
func ReplyRegistryPath() (string, error) { return Paths{}.ReplyRegistry() }

// LoadReplyFile reads the legacy reply-id registry file. A missing file
// returns an empty, non-nil ReplyStore. Live reads go through Store.
func LoadReplyFile(path string) (ReplyStore, error) {
	raw, err := readFile(path)
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

// SystemLabel identifies daemon-originated mail without providing a reply address.
const SystemLabel = "system"

// postSystem appends within an existing store transaction.
func postSystem(queues map[string][]Envelope, name, content string, sentAt time.Time) map[string][]Envelope {
	return AppendQueue(queues, name, Envelope{Label: SystemLabel, Content: content, SentAt: sentAt.UTC().Format(time.RFC3339)})
}

// WithReplyFileLock performs an flock-serialized read-modify-write on the
// legacy reply-id registry file at path, mirroring WithFileLock. Live writes
// go through Store; this serves the import, the export, and fixtures.
func WithReplyFileLock(path string, fn func(*ReplyStore) error) error {
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

	current, err := LoadReplyFile(path)
	if err != nil {
		return err
	}
	if err := fn(&current); err != nil {
		if errors.Is(err, errSkipWrite) {
			return nil
		}
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

// ReplyRetention bounds how long an idle, empty-queue reply-id entry
// survives before reaping deletes it (Decision 11's lazy expiry).
const ReplyRetention = 30 * 24 * time.Hour

// ReapReplyEntries deletes Entries whose LastSeen exceeds ReplyRetention and
// whose queue is empty: undelivered mail is never dropped, only the
// bookkeeping of a channel nobody touched in a long time.
func ReapReplyEntries(store *ReplyStore, now time.Time) {
	for id, entry := range store.Entries {
		if len(store.Queues[id]) > 0 {
			continue
		}
		last, err := time.Parse(time.RFC3339, entry.LastSeen)
		if err != nil || now.Sub(last) > ReplyRetention {
			delete(store.Entries, id)
		}
	}
}
