package mcp

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/kang-sw/devenv/internal/wskey"
	"github.com/kang-sw/devenv/internal/wsmailbox"
)

// mailbox_runtime.go implements the cross-session mailbox core's
// process-level policy (260913-feat-cross-session-mailbox-core): identity
// resolution from WS_MAILBOX / WS_MAILBOX_AUTO (Decisions 1, 10),
// self-registration + duplicate-live-name detection (Decision 2), owner
// binding at a parent-less ferrule (Decision 3), the caller==owner gate,
// the always-on reply-id return channel (Decision 11), and envelope
// stamping (Decision 12). mailbox_tools.go layers the three MCP handlers
// and the central piggyback wrapper on top of this file; wsmailbox owns
// pure storage.

const (
	envMailbox     = "WS_MAILBOX"
	envMailboxAuto = "WS_MAILBOX_AUTO"

	// mailboxHeartbeatThrottle bounds how often a live process rewrites its
	// own presence LastSeen: at most once per this window, mirroring
	// sessionStore's touchGuardWindow throttle. Implementation-chosen
	// default (Decision 2: "implementation-chosen with stated defaults").
	mailboxHeartbeatThrottle = time.Minute

	// mailboxLivenessThreshold is the missed-heartbeat window past which a
	// presence record is treated as dead: eligible for lookup_peers to hide
	// and for a fresh process to reclaim the name outright. Implementation-
	// chosen default.
	mailboxLivenessThreshold = 10 * time.Minute
)

// mailboxNow is the injectable clock, mirroring note_tools.go's noteNow so
// tests can control exact instants without depending on wall-clock gaps.
var mailboxNow = func() time.Time { return time.Now().UTC() }

func mailboxNowString() string { return mailboxNow().Format(time.RFC3339) }

// mailboxIdentity is the resolved, process-lifetime-stable identity
// WS_MAILBOX / WS_MAILBOX_AUTO establish for this MCP server process. It is
// resolved once (Server.mailboxOnce): an AUTO stem must stay stable across
// every call this process serves — regenerated only by a full process
// restart, never per call — and an explicit WS_MAILBOX is static env
// anyway.
type mailboxIdentity struct {
	Active bool
	Name   string
	Scope  wsmailbox.Scope
	Auto   bool
}

// mailboxRuntimeState holds every piece of Server state mailbox_runtime.go
// and mailbox_tools.go need, grouped into one embeddable struct so
// server.go's Server literal stays a single field rather than six.
type mailboxRuntimeState struct {
	identityOnce  sync.Once
	identityValue mailboxIdentity

	registerOnce sync.Once

	secretOnce  sync.Once
	secretValue []byte
	secretErr   error

	heartbeatMu sync.Mutex
	heartbeatAt time.Time

	conflictMu sync.Mutex
	conflict   bool
}

func (s *Server) mailboxIdentityResolved() mailboxIdentity {
	s.mailbox.identityOnce.Do(func() {
		s.mailbox.identityValue = computeMailboxIdentity()
	})
	return s.mailbox.identityValue
}

func computeMailboxIdentity() mailboxIdentity {
	if raw := strings.TrimSpace(os.Getenv(envMailbox)); raw != "" {
		name, scope, err := wsmailbox.ParseSlugScope(raw)
		if err != nil {
			appendDebugEvent("mailbox.identity_invalid", map[string]any{"source": envMailbox, "value": raw, "error": err.Error()})
			return mailboxIdentity{}
		}
		return mailboxIdentity{Active: true, Name: name, Scope: scope}
	}
	if raw := strings.TrimSpace(os.Getenv(envMailboxAuto)); raw != "" {
		scopeRaw := strings.ToLower(raw)
		if !wsmailbox.IsValidScope(scopeRaw) {
			appendDebugEvent("mailbox.identity_invalid", map[string]any{"source": envMailboxAuto, "value": raw})
			return mailboxIdentity{}
		}
		stem, err := wskey.Generate()
		if err != nil {
			appendDebugEvent("mailbox.identity_invalid", map[string]any{"source": envMailboxAuto, "error": err.Error()})
			return mailboxIdentity{}
		}
		return mailboxIdentity{Active: true, Name: stem, Scope: wsmailbox.Scope(scopeRaw), Auto: true}
	}
	return mailboxIdentity{}
}

// mailboxAddress renders identity as its wire "name@scope" form. Callers
// must check Active first; an inert identity renders as "".
func (identity mailboxIdentity) address() string {
	if !identity.Active {
		return ""
	}
	return identity.Name + "@" + string(identity.Scope)
}

// mailboxPresenceLive reports whether p's LastSeen falls within
// mailboxLivenessThreshold of now. An unparsable LastSeen is treated as
// dead (safe default: allows reclaim rather than wedging a name forever on
// a malformed record).
func mailboxPresenceLive(p wsmailbox.Presence, now time.Time) bool {
	last, err := time.Parse(time.RFC3339, p.LastSeen)
	if err != nil {
		return false
	}
	return now.Sub(last) < mailboxLivenessThreshold
}

func (s *Server) setMailboxConflict(v bool) {
	s.mailbox.conflictMu.Lock()
	s.mailbox.conflict = v
	s.mailbox.conflictMu.Unlock()
}

// ensureMailboxRegistered performs the once-per-process self-registration
// (Decision 2): claim identity.Name in its scope's store, unless a
// still-live different process already holds it, in which case the
// existing record is flagged Conflict and left untouched (never
// clobbered) — the second process's own mailbox effectively fails to bind
// a named inbox (send/recv/piggyback on it never find an Owner pointing at
// this process's sessions), while the conflict itself surfaces through
// lookup_peers. A no-op when this process carries no active identity.
func (s *Server) ensureMailboxRegistered() {
	s.mailbox.registerOnce.Do(func() {
		identity := s.mailboxIdentityResolved()
		if !identity.Active {
			return
		}
		path, err := wsmailbox.PathForScope(identity.Scope, s.root)
		if err != nil {
			appendDebugEvent("mailbox.register_error", map[string]any{"error": err.Error()})
			return
		}
		now := mailboxNow()
		nowStr := now.Format(time.RFC3339)
		pid := os.Getpid()
		harness := s.currentHarness()
		root := s.root
		werr := wsmailbox.WithLock(path, func(store *wsmailbox.StoreFile) error {
			if existing, ok := store.Presence[identity.Name]; ok && existing.PID != pid && mailboxPresenceLive(existing, now) {
				s.setMailboxConflict(true)
				existing.Conflict = true
				store.Presence[identity.Name] = existing
				return nil
			}
			store.Presence[identity.Name] = wsmailbox.Presence{
				Name: identity.Name, Scope: identity.Scope,
				Harness: harness, Cwd: root, StartedAt: nowStr, LastSeen: nowStr,
				PID: pid, Auto: identity.Auto,
			}
			return nil
		})
		if werr != nil {
			appendDebugEvent("mailbox.register_error", map[string]any{"error": werr.Error()})
		}
	})
}

// mailboxHeartbeatDue reports whether enough time has passed since the last
// heartbeat write to justify another one, and if so atomically claims the
// window. Shared by the presence heartbeat refresh across every mailbox
// call site so a busy session does not rewrite its own presence record on
// every single tool call.
func (s *Server) mailboxHeartbeatDue() bool {
	s.mailbox.heartbeatMu.Lock()
	defer s.mailbox.heartbeatMu.Unlock()
	now := mailboxNow()
	if !s.mailbox.heartbeatAt.IsZero() && now.Sub(s.mailbox.heartbeatAt) < mailboxHeartbeatThrottle {
		return false
	}
	s.mailbox.heartbeatAt = now
	return true
}

// refreshMailboxPresenceHeartbeat opportunistically refreshes this
// process's own presence LastSeen, throttled by mailboxHeartbeatDue. A
// no-op when inert, when the throttle window has not elapsed, or when this
// process has lost the name to a live conflict winner (PID mismatch: never
// resurrect a name another process now legitimately holds).
func (s *Server) refreshMailboxPresenceHeartbeat() {
	identity := s.mailboxIdentityResolved()
	if !identity.Active || !s.mailboxHeartbeatDue() {
		return
	}
	path, err := wsmailbox.PathForScope(identity.Scope, s.root)
	if err != nil {
		return
	}
	nowStr := mailboxNowString()
	pid := os.Getpid()
	_ = wsmailbox.WithLock(path, func(store *wsmailbox.StoreFile) error {
		p, ok := store.Presence[identity.Name]
		if !ok || p.PID != pid {
			return nil
		}
		p.LastSeen = nowStr
		store.Presence[identity.Name] = p
		return nil
	})
}

// mailboxSecret returns the process-cached, once-per-machine HMAC secret
// (Decision 11).
func (s *Server) mailboxSecret() ([]byte, error) {
	s.mailbox.secretOnce.Do(func() {
		s.mailbox.secretValue, s.mailbox.secretErr = wsmailbox.EnsureMachineSecret()
	})
	return s.mailbox.secretValue, s.mailbox.secretErr
}

// mailboxReplyID computes callerSessionKey's deterministic reply-id.
func (s *Server) mailboxReplyID(callerSessionKey string) (string, error) {
	secret, err := s.mailboxSecret()
	if err != nil {
		return "", err
	}
	return wsmailbox.ReplyID(secret, callerSessionKey), nil
}

// publishReplyID publishes/refreshes callerSessionKey's reply-id entry in
// the machine-tier registry (the channel-open act of Decision 11 send, and
// Decision 5's env-less self-lookup) and returns the reply-id.
func (s *Server) publishReplyID(callerSessionKey string) (string, error) {
	replyID, err := s.mailboxReplyID(callerSessionKey)
	if err != nil {
		return "", err
	}
	path, err := wsmailbox.ReplyRegistryPath()
	if err != nil {
		return "", err
	}
	nowStr := mailboxNowString()
	if err := wsmailbox.WithReplyLock(path, func(store *wsmailbox.ReplyStore) error {
		store.Entries[replyID] = wsmailbox.ReplyEntry{LastSeen: nowStr}
		return nil
	}); err != nil {
		return "", err
	}
	return replyID, nil
}

// markMailboxReplyOpened stamps the session record's MailboxReplyOpened
// flag (session_auth.go), the cheap in-memory-cost gate the piggyback
// wrapper checks before ever touching the reply-id registry file for an
// otherwise-unrelated session (Decision 1's zero-overhead contract). A
// no-op when already set, avoiding a redundant disk write on every send.
func (s *Server) markMailboxReplyOpened(sessionKey string) {
	rec, ok := s.sessions.readState(sessionKey)
	if !ok || rec.MailboxReplyOpened {
		return
	}
	_ = s.sessions.mutateRecord(sessionKey, func(r *sessionRecord) error {
		r.MailboxReplyOpened = true
		return nil
	})
}

// rebindMailboxOwnerAtFerrule rebinds the named-inbox owner pointer to
// newSessionKey when this process holds an active mailbox identity and the
// ferrule call is parent-less (Decision 3: top-lead login / re-login
// recovery, never a worker/delegate mint, which always carries a parent).
// The address itself (identity.Name/Scope) is never re-minted here.
func (s *Server) rebindMailboxOwnerAtFerrule(newSessionKey, parentKey string) {
	if strings.TrimSpace(parentKey) != "" {
		return
	}
	identity := s.mailboxIdentityResolved()
	if !identity.Active {
		return
	}
	s.ensureMailboxRegistered()
	path, err := wsmailbox.PathForScope(identity.Scope, s.root)
	if err != nil {
		return
	}
	nowStr := mailboxNowString()
	_ = wsmailbox.WithLock(path, func(store *wsmailbox.StoreFile) error {
		p, ok := store.Presence[identity.Name]
		if !ok {
			// Registration was skipped or lost (e.g. a startup conflict);
			// rebuild a minimal record so ownership still binds. A later
			// heartbeat refresh backfills descriptive metadata.
			p = wsmailbox.Presence{Name: identity.Name, Scope: identity.Scope, PID: os.Getpid(), StartedAt: nowStr}
		}
		p.Owner = newSessionKey
		p.LastSeen = nowStr
		store.Presence[identity.Name] = p
		return nil
	})
}

// mailboxOwnerCheck reports whether callerSessionKey currently holds the
// named-inbox owner pointer for this process's active identity. ok=false
// with err=nil means "not the owner" (including: no active identity, or no
// owner bound yet); a non-nil err means the store could not be read.
func (s *Server) mailboxOwnerCheck(callerSessionKey string) (isOwner bool, identity mailboxIdentity, err error) {
	identity = s.mailboxIdentityResolved()
	if !identity.Active {
		return false, identity, nil
	}
	path, perr := wsmailbox.PathForScope(identity.Scope, s.root)
	if perr != nil {
		return false, identity, perr
	}
	store, lerr := wsmailbox.Load(path)
	if lerr != nil {
		return false, identity, lerr
	}
	p, ok := store.Presence[identity.Name]
	if !ok || p.Owner == "" {
		return false, identity, nil
	}
	return p.Owner == callerSessionKey, identity, nil
}

// mailboxAddressAnnouncement computes the workflow_manual ambient
// self-address line (Decision: self-address surface), or "" when this
// process carries no active WS_MAILBOX/WS_MAILBOX_AUTO identity — the
// common, mailbox-inert case, which must stay silent (Decision 1). Also
// opportunistically refreshes this process's own presence heartbeat,
// internally throttled so a busy bootstrap/continue loop does not rewrite
// presence on every call.
func mailboxAddressAnnouncement(s *Server) string {
	identity := s.mailboxIdentityResolved()
	if !identity.Active {
		return ""
	}
	s.refreshMailboxPresenceHeartbeat()
	autoTag := ""
	if identity.Auto {
		autoTag = " (auto-minted)"
	}
	return fmt.Sprintf("> **Mailbox address:** `%s`%s. Reachable via mailbox.send; call mailbox.recv to drain unread mail.", identity.address(), autoTag)
}

// mailboxFromStamp decides the sender's server-stamped "from:" handle
// (Decision 12): the sender's own slug when the recipient shares a layer
// the sender's slug reaches (a machine-scope slug reaches any recipient; a
// clone/worktree slug reaches only a same-scope, non-reply-id recipient),
// otherwise "" (the caller falls back to a reply-to: id: stamp instead).
func mailboxFromStamp(identity mailboxIdentity, targetScope wsmailbox.Scope, targetIsReplyID bool) string {
	if !identity.Active {
		return ""
	}
	if identity.Scope == wsmailbox.ScopeMachine {
		return identity.address()
	}
	if !targetIsReplyID && identity.Scope == targetScope {
		return identity.address()
	}
	return ""
}
