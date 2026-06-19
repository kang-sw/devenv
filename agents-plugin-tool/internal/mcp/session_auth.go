package mcp

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sync"

	"github.com/kang-sw/devenv/internal/wskey"
	"github.com/kang-sw/devenv/internal/wsstate"
)

// sessionEntry associates a canonical repository root and a capability scope
// with an ephemeral session key minted by the session-bootstrap tool.
type sessionEntry struct {
	root            string
	scope           toolRole
	preferMercenary bool
}

// sessionRecord is the on-disk JSON shape of a session entry. It is versioned so
// the format can grow (render lineage, permission/capability metadata) without a
// migration; unknown future fields are simply ignored by older readers.
type sessionRecord struct {
	SchemaVersion   int               `json:"schema_version"`
	Root            string            `json:"root"`
	Scope           string            `json:"scope"`
	PreferMercenary bool              `json:"prefer_mercenary"`
	// Overrides is the session-scope generic config overlay. Keys are item
	// identifiers; values are string-encoded config values. Added as an additive
	// field; existing records without it parse with a nil map.
	Overrides       map[string]string `json:"overrides,omitempty"`
}

const sessionRecordSchemaVersion = 1

// sessionKeyFilenamePattern bounds which key strings may become a file path. It
// is deliberately a path-safety guard (no separators, no dots, lowercase alnum +
// hyphen only), not an exact word-chain format check, so the store tolerates
// future key-format evolution while still rejecting traversal attempts like
// "../../etc/passwd" handed to lookup by an untrusted caller.
var sessionKeyFilenamePattern = regexp.MustCompile(`^[a-z0-9-]{1,128}$`)

// sessionStore maps ephemeral session keys to session entries using one JSON
// file per key under a flat keys/ directory in the ws cache root. The file is
// the source of truth, not the process: a fresh MCP server instance (or a lead
// that restarted mid-delegation) resolves a key by reading its file, so session
// continuity no longer depends on a shared in-memory registry. There is no
// eviction or logout; deleting the file is the only physical removal.
//
// The flat layout (keys/<key>.json) is required by the access pattern: a caller
// presents only the opaque key, never its root, so the file path must be
// derivable from the key plus the globally-deterministic cache root.
type sessionStore struct {
	// mu serializes same-process read-modify-write (setPreferMercenary) and the
	// mint claim loop. Cross-process safety rests on filesystem primitives:
	// O_EXCL create for the unique mint claim, atomic temp+rename for updates.
	mu sync.Mutex
}

func newSessionStore() *sessionStore {
	return &sessionStore{}
}

// keysDir resolves and creates the flat per-session key directory. The cache
// root honors WS_CACHE_HOME (and otherwise ~/.cache/ws@...), the same seam every
// other ws cache artifact uses, so all server instances agree on the location.
func (s *sessionStore) keysDir() (string, error) {
	cacheRoot, err := wsstate.CacheRoot(wsstate.Options{})
	if err != nil {
		return "", err
	}
	dir := filepath.Join(cacheRoot, "keys")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create session keys dir: %w", err)
	}
	return dir, nil
}

func (s *sessionStore) keyPath(dir, key string) string {
	return filepath.Join(dir, key+".json")
}

// mint generates a unique session key and writes its record file.
//
// Uniqueness is claimed at the filesystem level: O_CREATE|O_EXCL creates the
// final file atomically, so two processes (or goroutines) generating the same
// candidate cannot both succeed — the loser sees os.IsExist and retries with a
// fresh candidate. This is the cross-process analogue of the previous in-memory
// check-and-insert and has no TOCTOU window.
func (s *sessionStore) mint(root string, scope toolRole) (string, error) {
	dir, err := s.keysDir()
	if err != nil {
		return "", err
	}
	record := sessionRecord{
		SchemaVersion: sessionRecordSchemaVersion,
		Root:          root,
		Scope:         string(scope),
	}
	payload, err := json.Marshal(record)
	if err != nil {
		return "", err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	const maxAttempts = 64
	for i := 0; i < maxAttempts; i++ {
		candidate, err := wskey.Generate()
		if err != nil {
			return "", err
		}
		f, err := os.OpenFile(s.keyPath(dir, candidate), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err != nil {
			if os.IsExist(err) {
				continue // rare collision; generate a fresh candidate and retry
			}
			return "", fmt.Errorf("create session key file: %w", err)
		}
		if _, werr := f.Write(payload); werr != nil {
			f.Close()
			os.Remove(s.keyPath(dir, candidate)) // do not leave a half-written claim behind
			return "", fmt.Errorf("write session key file: %w", werr)
		}
		if cerr := f.Close(); cerr != nil {
			os.Remove(s.keyPath(dir, candidate))
			return "", fmt.Errorf("close session key file: %w", cerr)
		}
		return candidate, nil
	}
	return "", errors.New("mcp: could not mint a unique session key after many attempts")
}

// lookup returns the session entry for the given key and whether it was found.
// A malformed key (path-unsafe, or simply unknown) returns (zero, false), which
// callers translate into the unknown_session re-login contract.
func (s *sessionStore) lookup(key string) (sessionEntry, bool) {
	dir, err := s.keysDir()
	if err != nil {
		return sessionEntry{}, false
	}
	record, ok := s.readRecord(dir, key)
	if !ok {
		return sessionEntry{}, false
	}
	return sessionEntry{
		root:            record.Root,
		scope:           toolRole(record.Scope),
		preferMercenary: record.PreferMercenary,
	}, true
}

// setPreferMercenary flips the preferMercenary flag for the given key via an
// atomic read-modify-write. Returns true if the key was found and updated.
func (s *sessionStore) setPreferMercenary(key string) bool {
	dir, err := s.keysDir()
	if err != nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.readRecord(dir, key)
	if !ok {
		return false
	}
	record.PreferMercenary = true
	return s.writeRecordAtomic(dir, key, record) == nil
}

// getOverride returns the Overrides entry for the given item key in the session
// record identified by sessionKey. Returns ("", false) when the session is not
// found, the key is path-unsafe, or the item is absent.
//
// s.mu is held for the duration of the read to match the mutex discipline of
// setOverride, preventing a data race when another goroutine is concurrently
// writing to the same session record.
func (s *sessionStore) getOverride(sessionKey, itemKey string) (string, bool) {
	dir, err := s.keysDir()
	if err != nil {
		return "", false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.readRecord(dir, sessionKey)
	if !ok {
		return "", false
	}
	if record.Overrides == nil {
		return "", false
	}
	v, ok := record.Overrides[itemKey]
	return v, ok
}

// listOverrideKeys returns all item keys present in the Overrides map of the
// session record identified by sessionKey. Returns nil when the session is not
// found or the Overrides map is empty.
func (s *sessionStore) listOverrideKeys(sessionKey string) []string {
	dir, err := s.keysDir()
	if err != nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.readRecord(dir, sessionKey)
	if !ok || len(record.Overrides) == 0 {
		return nil
	}
	keys := make([]string, 0, len(record.Overrides))
	for k := range record.Overrides {
		keys = append(keys, k)
	}
	return keys
}

// setOverride writes an Overrides entry for the given item key/value into the
// session record identified by sessionKey via atomic read-modify-write.
// Returns an error if the session key is not found or the write fails.
func (s *sessionStore) setOverride(sessionKey, itemKey, value string) error {
	dir, err := s.keysDir()
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.readRecord(dir, sessionKey)
	if !ok {
		return fmt.Errorf("session key not found: %s", sessionKey)
	}
	if record.Overrides == nil {
		record.Overrides = map[string]string{}
	}
	record.Overrides[itemKey] = value
	return s.writeRecordAtomic(dir, sessionKey, record)
}

func (s *sessionStore) readRecord(dir, key string) (sessionRecord, bool) {
	if !sessionKeyFilenamePattern.MatchString(key) {
		return sessionRecord{}, false
	}
	data, err := os.ReadFile(s.keyPath(dir, key))
	if err != nil {
		return sessionRecord{}, false
	}
	var record sessionRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return sessionRecord{}, false
	}
	return record, true
}

// writeRecordAtomic replaces an existing key file via temp-write + rename so a
// concurrent reader never observes a partial record. The caller holds s.mu and
// has already validated the key via readRecord.
func (s *sessionStore) writeRecordAtomic(dir, key string, record sessionRecord) error {
	payload, err := json.Marshal(record)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, key+"-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(payload); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, s.keyPath(dir, key)); err != nil {
		os.Remove(tmpName)
		return err
	}
	return nil
}
