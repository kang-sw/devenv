package mcp

import (
	"encoding/json"
	"fmt"

	"github.com/kang-sw/devenv/internal/wsagent"
	"github.com/kang-sw/devenv/internal/wsgit"
	"github.com/kang-sw/devenv/internal/wsmailbox"
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

// MintLead creates a parent-less lead key at root's canonical git root, the
// record a top-lead ferrule mint writes, so the session's later ferrule
// re-login at the same root accepts it. The daemon mints a hosted lead
// session's key with it and passes it as WS_HOSTED_SESSION_KEY; the key is
// never written into the dashboard registry, which records only its
// reply-id.
func (a *AgentKeyStore) MintLead(root string) (string, error) {
	canonical, err := canonicalSetupRoot(wsgit.ExecRunner{}, root)
	if err != nil {
		return "", err
	}
	return a.store.mint(canonical, roleLead, "")
}

// Ensure keeps key usable for root: an existing record (either layout) rooted
// at root is touched (key records are pruned by age), a missing one is
// written again with the same key in the split layout, and a record rooted
// elsewhere or of another scope is an error.
func (a *AgentKeyStore) Ensure(root, key string) error {
	return a.ensure(root, key, roleDelegate)
}

// LeadRoot is root's canonical git root, the root MintLead records; it
// fails for a folder outside a git worktree.
func (a *AgentKeyStore) LeadRoot(root string) (string, error) {
	return canonicalSetupRoot(wsgit.ExecRunner{}, root)
}

// EnsureLead is Ensure for a hosted lead session's key: canonical is the
// root MintLead recorded (LeadRoot).
func (a *AgentKeyStore) EnsureLead(canonical, key string) error {
	return a.ensure(canonical, key, roleLead)
}

// LeadSession finds the parent-less lead key rooted at root whose reply-id
// is replyID, with the harness conversation its last login recorded
// (recordHarnessSession). It scans the key store's names, as the
// dashboard's session source does: no file maps a reply-id to its key.
func (a *AgentKeyStore) LeadSession(root, replyID string) (wsagent.LeadSession, bool) {
	d, err := a.store.dirs()
	if err != nil {
		return wsagent.LeadSession{}, false
	}
	keys, err := a.store.listKeys(d)
	if err != nil {
		return wsagent.LeadSession{}, false
	}
	for _, key := range keys {
		if wsmailbox.ReplyID(key) != replyID {
			continue
		}
		record, ok := a.store.readRecord(d, key, partKey)
		if !ok || record.Root != root || record.Scope != string(roleLead) || record.Parent != "" {
			continue
		}
		return wsagent.LeadSession{Key: key, Root: record.Root, Harness: record.Harness, Session: record.HarnessSession, Cwd: record.HarnessCwd}, true
	}
	return wsagent.LeadSession{}, false
}

func (a *AgentKeyStore) ensure(root, key string, scope toolRole) error {
	if !sessionKeyFilenamePattern.MatchString(key) {
		return fmt.Errorf("malformed session key")
	}
	d, err := a.store.dirs()
	if err != nil {
		return err
	}
	if record, ok := a.store.readRecord(d, key, partKey); ok {
		if record.Root != root || record.Scope != string(scope) || record.Parent != "" {
			return fmt.Errorf("session key record is not this conversation's (root %s, scope %s)", record.Root, record.Scope)
		}
		a.store.touch(d, key)
		return nil
	}
	payload, err := json.Marshal(sessionKeyRecord{SchemaVersion: sessionRecordSchemaVersion, Root: root, Scope: string(scope)})
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
