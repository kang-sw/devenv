// Package wsmailbox implements the storage half of the cross-session
// mailbox core (260913-feat-cross-session-mailbox-core): name-keyed
// presence + per-name message queues over the three scope tiers the
// project's other note/config storage already uses (machine/worktree/
// clone), plus the machine-tier reply-id registry that backs the
// env-less universal-send return channel. Storage reuses existing roots:
// each scope tier's mailbox.json lives beside that tier's wsnote store
// (notes.json), one directory up from the note file itself, so a scope's
// mailbox store and its note store always co-locate without introducing a
// new root-resolution path.
//
// This package is a pure storage layer: it knows how to resolve paths and
// perform locked read-modify-write on the JSON file, but nothing about
// session keys, ferrule, or MCP tool wiring. internal/mcp's mailbox_*.go
// files own that policy layer, mirroring wsnote/internal-mcp's own split.
package wsmailbox

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/gofrs/flock"

	"github.com/kang-sw/devenv/internal/wsconfig"
	"github.com/kang-sw/devenv/internal/wsnote"
)

// Scope identifies which storage tier a mailbox name lives in. Unlike
// wsnote.Layer, there is no "repo" scope: a mailbox address is a live
// coordination handle, never git-tracked content.
type Scope string

const (
	ScopeMachine  Scope = "machine"
	ScopeWorktree Scope = "worktree"
	ScopeClone    Scope = "clone"
)

// IsValidScope reports whether raw names one of the three mailbox scopes.
func IsValidScope(raw string) bool {
	switch Scope(raw) {
	case ScopeMachine, ScopeWorktree, ScopeClone:
		return true
	default:
		return false
	}
}

// storeFileName is the mailbox store's filename, sibling to each scope
// tier's "notes.json".
const storeFileName = "mailbox.json"

const schemaVersion = 1

// Presence is one name's registered identity: the durable/discoverable
// half of the mailbox contract. Owner is the rebindable internal pointer
// (Decision 3/4) — the session_key currently authorized against the named
// inbox — and is empty until the first parent-less ferrule binds it.
// Harness/Cwd/StartedAt are descriptive metadata only (Decision 10), never
// consulted for gating.
type Presence struct {
	Name      string `json:"name"`
	Scope     Scope  `json:"scope"`
	Owner     string `json:"owner,omitempty"`
	Harness   string `json:"harness,omitempty"`
	Cwd       string `json:"cwd,omitempty"`
	StartedAt string `json:"started_at,omitempty"`
	LastSeen  string `json:"last_seen"`
	PID       int    `json:"pid,omitempty"`
	Auto      bool   `json:"auto,omitempty"`
	Conflict  bool   `json:"conflict,omitempty"`
}

// Envelope is one queued message. Exactly one of From/ReplyTo is set on
// delivery (Decision 12): From when the recipient shares a layer where the
// sender published a slug, ReplyTo (an "id:<reply-id>" handle) otherwise.
type Envelope struct {
	From    string `json:"from,omitempty"`
	ReplyTo string `json:"reply_to,omitempty"`
	Content string `json:"content"`
	SentAt  string `json:"sent_at"`
}

// StoreFile is one scope tier's on-disk mailbox store: presence records and
// per-name message queues, both keyed by the bare name (no "@scope"
// suffix — the file itself is already scope-specific).
type StoreFile struct {
	SchemaVersion int                   `json:"schema_version"`
	Presence      map[string]Presence   `json:"presence,omitempty"`
	Queues        map[string][]Envelope `json:"queues,omitempty"`
}

// MaxQueueLen bounds a single name's queue: a never-draining name's queue
// is trimmed from the front (oldest first) rather than growing unbounded.
// Implementation-chosen default, not a cross-ticket contract.
const MaxQueueLen = 200

// LockTimeout bounds how long a store RMW waits to acquire its flock,
// mirroring wsnote/store.go's lockTimeout.
const LockTimeout = 2 * time.Second

// MachinePath resolves the machine-scope mailbox store path: sibling of
// the machine-layer note store (typically ~/.ws/mailbox.json).
func MachinePath() (string, error) {
	notesPath, err := wsnote.MachinePath(wsconfig.Options{})
	if err != nil {
		return "", err
	}
	return sibling(notesPath), nil
}

// WorktreePath resolves the worktree-scope mailbox store path for root:
// sibling of that worktree's note store.
func WorktreePath(root string) (string, error) {
	notesPath, err := wsnote.WorktreePath(root)
	if err != nil {
		return "", err
	}
	return sibling(notesPath), nil
}

// ClonePath resolves the clone-scope mailbox store path for root: sibling
// of that project's clone-layer note store (shared across every worktree
// of the project).
func ClonePath(root string) (string, error) {
	notesPath, err := wsnote.ClonePath(root)
	if err != nil {
		return "", err
	}
	return sibling(notesPath), nil
}

func sibling(notesPath string) string {
	return filepath.Join(filepath.Dir(notesPath), storeFileName)
}

// PathForScope resolves the store path for scope, given the caller's own
// root (used only for worktree/clone; ignored for machine). This is the
// single dispatch point every mailbox_runtime.go call site uses instead of
// switching on Scope itself.
func PathForScope(scope Scope, root string) (string, error) {
	switch scope {
	case ScopeMachine:
		return MachinePath()
	case ScopeWorktree:
		return WorktreePath(root)
	case ScopeClone:
		return ClonePath(root)
	default:
		return "", fmt.Errorf("wsmailbox: invalid scope %q", scope)
	}
}

// Load reads the store file at path. A missing file returns an empty,
// non-nil StoreFile, mirroring wsnote.Load's "no file yet" contract. Load
// never locks: writes go through temp+atomic-rename (see WithLock), so a
// concurrent reader observes either the old or the new whole file, never a
// partial one.
func Load(path string) (StoreFile, error) {
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return emptyStore(), nil
	}
	if err != nil {
		return StoreFile{}, fmt.Errorf("read mailbox store %s: %w", path, err)
	}
	return decodeStore(path, raw)
}

func emptyStore() StoreFile {
	return StoreFile{SchemaVersion: schemaVersion, Presence: map[string]Presence{}, Queues: map[string][]Envelope{}}
}

func decodeStore(path string, raw []byte) (StoreFile, error) {
	var store StoreFile
	if err := json.Unmarshal(raw, &store); err != nil {
		return StoreFile{}, fmt.Errorf("parse mailbox store %s: %w", path, err)
	}
	if store.Presence == nil {
		store.Presence = map[string]Presence{}
	}
	if store.Queues == nil {
		store.Queues = map[string][]Envelope{}
	}
	return store, nil
}

// WithLock performs an flock-serialized read-modify-write on the store file
// at path: load the current content, run fn against it, and atomically
// persist the result via temp-write + rename. This is the sole mutation
// path for presence self-registration/heartbeat, owner rebind, send
// (append to a queue), and recv (drain a queue). Mirrors wsnote/store.go's
// rmw, generalized to the typed StoreFile shape.
func WithLock(path string, fn func(*StoreFile) error) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create mailbox store dir: %w", err)
	}

	lockPath := path + ".lock"
	fl := flock.New(lockPath)
	ctx, cancel := context.WithTimeout(context.Background(), LockTimeout)
	defer cancel()

	locked, err := fl.TryLockContext(ctx, 50*time.Millisecond)
	if err != nil {
		return fmt.Errorf("acquire mailbox store lock: %w", err)
	}
	if !locked {
		return fmt.Errorf("timed out waiting for mailbox store lock: %s", lockPath)
	}
	defer fl.Unlock() //nolint:errcheck

	current, err := Load(path)
	if err != nil {
		return err
	}
	if err := fn(&current); err != nil {
		return err
	}

	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+"-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp mailbox store: %w", err)
	}
	tmpName := tmp.Name()
	if current.SchemaVersion == 0 {
		current.SchemaVersion = schemaVersion
	}
	payload, err := json.MarshalIndent(current, "", "  ")
	if err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("encode mailbox store: %w", err)
	}
	payload = append(payload, '\n')
	if _, werr := tmp.Write(payload); werr != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("write temp mailbox store: %w", werr)
	}
	if cerr := tmp.Close(); cerr != nil {
		os.Remove(tmpName)
		return fmt.Errorf("close temp mailbox store: %w", cerr)
	}
	if rerr := os.Rename(tmpName, path); rerr != nil {
		os.Remove(tmpName)
		return fmt.Errorf("atomic rename mailbox store: %w", rerr)
	}
	return nil
}

// AppendQueue appends msg to queues[name], trimming from the front when the
// result would exceed MaxQueueLen (bounded size, Decision 2).
func AppendQueue(queues map[string][]Envelope, name string, msg Envelope) map[string][]Envelope {
	if queues == nil {
		queues = map[string][]Envelope{}
	}
	list := append(queues[name], msg)
	if len(list) > MaxQueueLen {
		list = list[len(list)-MaxQueueLen:]
	}
	queues[name] = list
	return queues
}
