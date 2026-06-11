package mcp

import (
	"errors"
	"sync"

	"github.com/kang-sw/devenv/internal/wskey"
)

// sessionEntry associates a canonical repository root and a capability scope
// with an ephemeral session key minted by ws.lead.login.
type sessionEntry struct {
	root            string
	scope           toolRole
	preferMercenary bool
}

// sessionRegistry is a concurrency-safe in-memory store mapping session keys to
// session entries. There is no eviction, logout, or persistence.
type sessionRegistry struct {
	mu      sync.RWMutex
	entries map[string]sessionEntry
}

func newSessionRegistry() *sessionRegistry {
	return &sessionRegistry{entries: make(map[string]sessionEntry)}
}

// mint generates a unique session key and atomically inserts the entry.
//
// It calls wskey.Generate directly rather than wskey.GenerateUnique because the
// registry needs an atomic check-and-insert: generate the candidate OUTSIDE the
// lock, acquire the write lock, check membership, insert if free, release —
// looping on the rare collision. wskey.GenerateUnique calls the exists predicate
// OUTSIDE any lock (intentional for its general use case), which would introduce
// a TOCTOU window between the predicate check and the subsequent write-lock
// insert. The inline loop eliminates that window without any lock-ordering issue.
// wskey.GenerateUnique remains public API for callers that do not need atomic
// check-and-insert (e.g. the planned word-chain id generalization follow-up).
func (r *sessionRegistry) mint(root string, scope toolRole) (string, error) {
	const maxAttempts = 64
	for i := 0; i < maxAttempts; i++ {
		candidate, err := wskey.Generate()
		if err != nil {
			return "", err
		}
		r.mu.Lock()
		_, taken := r.entries[candidate]
		if !taken {
			r.entries[candidate] = sessionEntry{root: root, scope: scope}
			r.mu.Unlock()
			return candidate, nil
		}
		r.mu.Unlock()
		// rare collision; generate a fresh candidate and retry
	}
	return "", errors.New("mcp: could not mint a unique session key after many attempts")
}

// lookup returns the session entry for the given key and whether it was found.
func (r *sessionRegistry) lookup(key string) (sessionEntry, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	entry, ok := r.entries[key]
	return entry, ok
}

// setPreferMercenary flips the preferMercenary flag for the given key under the write lock.
// Returns true if the key was found and updated, false if the key was not found.
func (r *sessionRegistry) setPreferMercenary(key string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	entry, ok := r.entries[key]
	if !ok {
		return false
	}
	entry.preferMercenary = true
	r.entries[key] = entry
	return true
}
