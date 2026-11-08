package mcp

import (
	"encoding/json"
	"fmt"
)

// AgentKeyStore mints and keeps the ws session keys of the dashboard
// daemon's agent conversations (internal/wsagent). Each key is a
// delegate-scope key with no parent, rooted at the conversation's working
// root: lead-only tools, session.*, and config writes stay refused, the
// dashboard never registers it (only a top-level lead registers), and the
// attached ws-mcp's tool allowlist narrows it further.
type AgentKeyStore struct {
	store *sessionStore
}

// NewAgentKeyStore returns a store over this machine's key directory.
func NewAgentKeyStore() *AgentKeyStore {
	return &AgentKeyStore{store: newSessionStore()}
}

// Mint creates a new agent key rooted at root.
func (a *AgentKeyStore) Mint(root string) (string, error) {
	return a.store.mint(root, roleDelegate, "")
}

// Ensure keeps key usable for root: an existing record (either layout) rooted
// at root is touched (key records are pruned by age), a missing one is
// written again with the same key in the split layout, and a record rooted
// elsewhere or of another scope is an error.
func (a *AgentKeyStore) Ensure(root, key string) error {
	if !sessionKeyFilenamePattern.MatchString(key) {
		return fmt.Errorf("malformed session key")
	}
	d, err := a.store.dirs()
	if err != nil {
		return err
	}
	if record, ok := a.store.readRecord(d, key, partKey); ok {
		if record.Root != root || record.Scope != string(roleDelegate) || record.Parent != "" {
			return fmt.Errorf("session key record is not this conversation's (root %s, scope %s)", record.Root, record.Scope)
		}
		a.store.touch(d, key)
		return nil
	}
	payload, err := json.Marshal(sessionKeyRecord{SchemaVersion: sessionRecordSchemaVersion, Root: root, Scope: string(roleDelegate)})
	if err != nil {
		return err
	}
	claimed, err := a.store.claimKey(d, key, payload)
	if err != nil {
		return err
	}
	if !claimed {
		return fmt.Errorf("session key record exists but is unreadable")
	}
	return nil
}
