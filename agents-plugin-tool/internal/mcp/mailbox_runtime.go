package mcp

import (
	"context"
	"fmt"
	"os"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/kang-sw/devenv/internal/wskey"
	"github.com/kang-sw/devenv/internal/wsmailbox"
	"github.com/kang-sw/devenv/internal/wsstate"
)

// mailbox_runtime.go implements the cross-session mailbox core's
// process-level policy (260913-feat-cross-session-mailbox-core): identity
// resolution from WS_MAILBOX / WS_MAILBOX_AUTO (Decisions 1, 10),
// self-registration + duplicate-live-name detection (Decision 2), owner
// binding at a parent-less lead ferrule (Decision 3), the caller==owner gate,
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

	// registeredMu guards registeredRoot/registered, written by the
	// registerOnce body and by each successful lead rebind on request
	// goroutines, and read by the background presence ticker goroutine.
	// registeredRoot is the root whose store holds this process's owned
	// record: the registration root until a lead rebind binds under another.
	registeredMu   sync.Mutex
	registered     bool
	registeredRoot string

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
		// wskey.Generate's word-pool output is expected to already satisfy
		// IsValidName (lowercase letters/digits/hyphens), but that is an
		// invariant of a sibling package's word list, not something this
		// package controls — validate rather than silently trusting it, the
		// same defensive posture ParseSlugScope already applies to an
		// explicit WS_MAILBOX value.
		if !wsmailbox.IsValidName(stem) {
			appendDebugEvent("mailbox.identity_invalid", map[string]any{"source": envMailboxAuto, "value": stem, "error": "auto-minted stem is not a valid mailbox name"})
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
// a malformed record). A presence record whose PID is definitively no
// longer running is always dead regardless of LastSeen recency — this
// closes the "ordinary restart" false-positive-conflict gap (a crashed
// process's own recent heartbeat must not block its own restart from
// reclaiming the name for up to mailboxLivenessThreshold). It intentionally
// does NOT treat "PID still running" as sufficient for liveness on its own:
// recency stays the liveness signal, and an idle-but-alive holder keeps it
// fresh through the background presence ticker (runMailboxPresenceTicker),
// which ServeStdio runs for the server's lifetime. That ticker is a
// goroutine independent of the request loop, so what the threshold detects
// is a stopped or suspended serving process, not a wedged request loop: a
// process whose handlers hang but whose ticker still runs stays live.
//
// Liveness itself delegates to wsstate.ProcessAlive rather than a
// hand-rolled probe: that helper is this repo's own established,
// cross-platform (unix + windows, both CI-exercised via windows-smoke)
// same-machine process-liveness primitive, already used by this package's
// own orchestrator lock. A bespoke probe here risked exactly the kind of
// platform-specific gap (os.Process.Signal's ESRCH-masking on unix;
// os.FindProcess's inverted error meaning on windows) that a
// previously-reviewed version of this function actually had.
func mailboxPresenceLive(p wsmailbox.Presence, now time.Time) bool {
	if !wsstate.ProcessAlive(p.PID) {
		return false
	}
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
//
// root must be the CALLING SESSION's own canonicalized root
// (sessionEntry.root / sessionRecord.Root), never the process-level
// Server.root: per ai-docs/manuals/ws-mcp.md, the MCP process's own cwd is
// unreliable (Codex normalizes it to the installed plugin cache), so a
// worktree/clone-scope identity cannot resolve its store path from process
// state. A machine-scope identity needs no root at all.
//
// The registration attempt is deferred — never consumed — until a call
// carries a real root for a worktree/clone identity: the guard sits before
// registerOnce.Do so an early root-less call (e.g. "initialize") does not
// permanently skip registration once a real root becomes available on a
// later call (e.g. the owning ferrule login).
func (s *Server) ensureMailboxRegistered(root string) {
	identity := s.mailboxIdentityResolved()
	if !identity.Active {
		return
	}
	if identity.Scope != wsmailbox.ScopeMachine && strings.TrimSpace(root) == "" {
		return
	}
	s.mailbox.registerOnce.Do(func() {
		// Retained for the background presence ticker, which has no calling
		// session and must heartbeat the store this identity registered in.
		s.mailbox.registeredMu.Lock()
		s.mailbox.registered = true
		s.mailbox.registeredRoot = root
		s.mailbox.registeredMu.Unlock()
		path, err := wsmailbox.PathForScope(identity.Scope, root)
		if err != nil {
			appendDebugEvent("mailbox.register_error", map[string]any{"error": err.Error()})
			return
		}
		now := mailboxNow()
		pid := os.Getpid()
		fresh := s.ownedMailboxPresence(identity, root, now.Format(time.RFC3339))
		werr := wsmailbox.WithLock(path, func(store *wsmailbox.StoreFile) error {
			if existing, ok := store.Presence[identity.Name]; ok && existing.PID != pid && mailboxPresenceLive(existing, now) {
				s.setMailboxConflict(true)
				existing.Conflict = true
				store.Presence[identity.Name] = existing
				return nil
			}
			store.Presence[identity.Name] = fresh
			return nil
		})
		if werr != nil {
			appendDebugEvent("mailbox.register_error", map[string]any{"error": werr.Error()})
		}
	})
}

// ownedMailboxPresence builds a fresh presence record for identity held by
// this process, registered under root at nowStr: the record
// ensureMailboxRegistered writes, and the one a lead rebind rebuilds when it
// takes over a missing or dead-PID record.
func (s *Server) ownedMailboxPresence(identity mailboxIdentity, root, nowStr string) wsmailbox.Presence {
	return wsmailbox.Presence{
		Name: identity.Name, Scope: identity.Scope,
		Harness: s.currentHarness(), Cwd: root, StartedAt: nowStr, LastSeen: nowStr,
		PID: os.Getpid(), Auto: identity.Auto,
	}
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
// resurrect a name another process now legitimately holds). root must be
// the calling session's own canonicalized root (see ensureMailboxRegistered);
// a root-less call is a no-op for a worktree/clone identity, but the
// throttle window is still claimed for this call, matching the existing
// per-process throttle contract (a machine-scope identity is unaffected,
// needing no root).
func (s *Server) refreshMailboxPresenceHeartbeat(root string) {
	identity := s.mailboxIdentityResolved()
	if !identity.Active || !s.mailboxHeartbeatDue() {
		return
	}
	s.writeMailboxPresenceLastSeen(identity, root)
}

// writeMailboxPresenceLastSeen stamps LastSeen on identity's presence record
// in root's store, only while that record's PID is this process: a record
// another process holds (PID mismatch, including a conflict this process
// lost) or a missing record is never written, so a name is never
// resurrected. A no-op for a worktree/clone identity with no root.
func (s *Server) writeMailboxPresenceLastSeen(identity mailboxIdentity, root string) {
	if identity.Scope != wsmailbox.ScopeMachine && strings.TrimSpace(root) == "" {
		return
	}
	path, err := wsmailbox.PathForScope(identity.Scope, root)
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

// newMailboxPresenceTicker is the injectable tick source for the background
// presence ticker; tests replace it with a manual channel. The interval
// matches mailboxHeartbeatThrottle, well inside mailboxLivenessThreshold.
var newMailboxPresenceTicker = func() (<-chan time.Time, func()) {
	ticker := time.NewTicker(mailboxHeartbeatThrottle)
	return ticker.C, ticker.Stop
}

// startMailboxPresenceTicker starts the background presence heartbeat for
// this process's active mailbox identity and returns a channel closed once
// the ticker goroutine has exited (after ctx is cancelled). A mailbox-inert
// process starts no goroutine and gets an already-closed channel.
func (s *Server) startMailboxPresenceTicker(ctx context.Context) <-chan struct{} {
	done := make(chan struct{})
	if !s.mailboxIdentityResolved().Active {
		close(done)
		return done
	}
	tick, stop := newMailboxPresenceTicker()
	go func() {
		defer close(done)
		defer stop()
		// Request goroutines recover their panics (ServeStdio); this
		// goroutine does the same so a failed heartbeat write never takes
		// down the serve process. The heartbeat simply ends.
		defer func() {
			if r := recover(); r != nil {
				recordPanic("mailbox.presence_ticker", "", r, debug.Stack())
			}
		}()
		s.runMailboxPresenceTicker(ctx, tick)
	}()
	return done
}

// runMailboxPresenceTicker refreshes this process's presence LastSeen on
// every tick until ctx is cancelled, so an idle owner — one making no
// mailbox tool calls, e.g. blocked in an armed mailbox wait — stays live to
// lookup_peers and its name stays unreclaimable. Each tick follows whatever
// record the process holds right now by name and PID in the store of
// registeredRoot (the registration root, or the root of the most recent
// successful lead rebind); before registration (a worktree/clone identity whose owning
// login has not arrived yet) a tick is a no-op.
func (s *Server) runMailboxPresenceTicker(ctx context.Context, tick <-chan time.Time) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick:
			s.mailbox.registeredMu.Lock()
			registered, root := s.mailbox.registered, s.mailbox.registeredRoot
			s.mailbox.registeredMu.Unlock()
			if registered {
				s.writeMailboxPresenceLastSeen(s.mailboxIdentityResolved(), root)
			}
		}
	}
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

// mailboxReplyIDRetention bounds how long an idle, empty-queue reply-id
// entry survives in the machine-tier registry before lazy reaping deletes
// it (Decision 11's lazy-expiry reaping), mirroring session_auth.go's
// keyRetentionAge order of magnitude. Reaping only ever removes an entry
// whose queue is already empty: undelivered mail is never dropped by this
// pass, only the bookkeeping Entries record for a channel nobody has
// touched in a long time.
const mailboxReplyIDRetention = 30 * 24 * time.Hour

// reapStaleReplyIDs deletes Entries whose LastSeen exceeds
// mailboxReplyIDRetention and whose Queues slot is empty, bounding the
// registry's otherwise-unbounded growth from one-off callers that never
// return. Folded into every publishReplyID write rather than a background
// sweep (background mechanisms are 260913-feat-cross-session-mailbox-wake's
// territory, Decisions 6/7/9).
func reapStaleReplyIDs(store *wsmailbox.ReplyStore, now time.Time) {
	for id, entry := range store.Entries {
		if len(store.Queues[id]) > 0 {
			continue
		}
		last, err := time.Parse(time.RFC3339, entry.LastSeen)
		if err != nil || now.Sub(last) > mailboxReplyIDRetention {
			delete(store.Entries, id)
		}
	}
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
		reapStaleReplyIDs(store, mailboxNow())
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
// ferrule call is a parent-less lead-capability mint (Decision 3: top-lead
// login / re-login recovery). A parent-carrying mint never binds, and neither
// does a parent-less mint whose capability resolves to delegate or leaf: a
// harness can launch a child without threading the lead's key through, and
// that child's MCP server may share this process's WS_MAILBOX identity, so a
// missing parent alone does not prove a top-lead login. An omitted capability
// resolves to lead (parseCapabilityScope) and still binds. The address itself
// (identity.Name/Scope) is never re-minted here.
//
// It refuses to bind when this process has lost (or is contesting) the
// duplicate-live-name race: s.mailbox.conflict, set by
// ensureMailboxRegistered on a collision, is checked first, and the
// presence record is re-verified for a live different-PID holder under the
// same lock the write would use, so a race that resolves against this
// process AFTER its own registration is still caught. Binding ownership
// over a name another live process legitimately holds would let this
// session hijack that process's identity (Critical: owner-rebind ignoring
// conflict state).
//
// A missing record, or one left behind by a different, no-longer-live PID, is
// rebuilt as this process's record (ownedMailboxPresence): the heartbeat
// writers only refresh a record whose PID is this process, so keeping the
// stale PID would leave the reclaimed inbox looking dead to every peer, and
// the previous holder's Harness/Cwd and Conflict flag would misdescribe the
// live owner. A successful bind also points the presence ticker at root, so
// a lead that re-logs in under another root keeps that store's record fresh.
func (s *Server) rebindMailboxOwnerAtFerrule(newSessionKey, parentKey string, scope toolRole, root string) {
	if strings.TrimSpace(parentKey) != "" || scope != roleLead {
		return
	}
	identity := s.mailboxIdentityResolved()
	if !identity.Active {
		return
	}
	s.ensureMailboxRegistered(root)
	if s.mailboxHasConflict() {
		appendDebugEvent("mailbox.rebind_skipped_conflict", map[string]any{"name": identity.Name, "scope": string(identity.Scope)})
		return
	}
	path, err := wsmailbox.PathForScope(identity.Scope, root)
	if err != nil {
		return
	}
	now := mailboxNow()
	nowStr := now.Format(time.RFC3339)
	pid := os.Getpid()
	bound := false
	werr := wsmailbox.WithLock(path, func(store *wsmailbox.StoreFile) error {
		p, ok := store.Presence[identity.Name]
		if ok && p.PID != pid && mailboxPresenceLive(p, now) {
			// A different, still-live process now legitimately holds this
			// name (a race resolved after our own registration); refuse to
			// steal its ownership pointer.
			return nil
		}
		if !ok || p.PID != pid {
			// Registration was skipped or lost (e.g. a startup conflict), or
			// the record belongs to a dead process: rebuild it as ours.
			p = s.ownedMailboxPresence(identity, root, nowStr)
		}
		p.Owner = newSessionKey
		p.LastSeen = nowStr
		store.Presence[identity.Name] = p
		bound = true
		return nil
	})
	if werr == nil && bound {
		s.mailbox.registeredMu.Lock()
		s.mailbox.registered = true
		s.mailbox.registeredRoot = root
		s.mailbox.registeredMu.Unlock()
	}
}

// mailboxHasConflict reports whether ensureMailboxRegistered flagged a
// duplicate-live-name collision for this process's identity.
func (s *Server) mailboxHasConflict() bool {
	s.mailbox.conflictMu.Lock()
	defer s.mailbox.conflictMu.Unlock()
	return s.mailbox.conflict
}

// mailboxOwnerCheck reports whether callerSessionKey currently holds the
// named-inbox owner pointer for this process's active identity. ok=false
// with err=nil means "not the owner" (including: no active identity, or no
// owner bound yet); a non-nil err means the store could not be read. root
// must be the calling session's own canonicalized root (see
// ensureMailboxRegistered); a root-less call against a worktree/clone
// identity reports "not the owner" rather than erroring, since no store
// can be resolved yet.
func (s *Server) mailboxOwnerCheck(callerSessionKey string, root string) (isOwner bool, identity mailboxIdentity, err error) {
	identity = s.mailboxIdentityResolved()
	if !identity.Active {
		return false, identity, nil
	}
	if identity.Scope != wsmailbox.ScopeMachine && strings.TrimSpace(root) == "" {
		return false, identity, nil
	}
	path, perr := wsmailbox.PathForScope(identity.Scope, root)
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
// process carries no active WS_MAILBOX/WS_MAILBOX_AUTO identity (the
// common, mailbox-inert case, which must stay silent per Decision 1) or
// when sessionKey is not the confirmed owner of that identity — a
// non-owner session sharing this process (e.g. a parent-carrying
// worker/delegate mint) cannot actually recv from the named inbox, so
// announcing an address it cannot use would be a leak, not a service.
// Also opportunistically refreshes this process's own presence heartbeat,
// internally throttled so a busy bootstrap/continue loop does not rewrite
// presence on every call.
func mailboxAddressAnnouncement(s *Server, sessionKey, root string) string {
	identity := s.mailboxIdentityResolved()
	if !identity.Active {
		return ""
	}
	isOwner, _, err := s.mailboxOwnerCheck(sessionKey, root)
	if err != nil || !isOwner {
		return ""
	}
	s.refreshMailboxPresenceHeartbeat(root)
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
