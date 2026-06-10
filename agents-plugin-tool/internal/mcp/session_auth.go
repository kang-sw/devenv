package mcp

import (
	"errors"
	"sync"

	"github.com/kang-sw/devenv/internal/wskey"
)

// sessionEntry associates a canonical repository root and a capability scope
// with an ephemeral session key minted by ws.lead.login.
type sessionEntry struct {
	root  string
	scope toolRole
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
// It uses wskey.Generate to produce candidates outside the write lock, then
// performs check-and-insert under the write lock, looping on collision. This
// avoids any lock-ordering deadlock: the generator never calls back into the
// registry while the registry holds a lock.
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
