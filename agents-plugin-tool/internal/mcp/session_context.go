package mcp

import (
	"errors"
	"fmt"
	"strings"
)

// sessionContext is an in-process projection, not a new stored or wire shape.
// Authority and lineage come only from the key store. Display nesting and a
// conversation's ParentRef never participate in selection.
type sessionContext struct {
	key            string
	scope          toolRole
	parent         string
	canonicalRoot  string
	root           string
	hosted         bool
	harness        string
	harnessSession string
	harnessCwd     string
	found          bool
	transient      bool // cached activity inspection could not read the record
}

type sessionAccess struct {
	mode   string // identity, intake, active, mint-identity, lookup, bootstrap, relogin, manual, or mint
	key    string
	root   string // already canonicalized by the caller, preserving its error order
	scope  toolRole
	parent string
}

// resolveSession is the shared adoption core. It deliberately owns no storage
// selector, delivery loop, or render side effects, so a card bootstrap can wrap
// it without acquiring a second authority source. Mint callers retain their
// existing timing (manual renders and validates before it mints).
func (s *Server) resolveSession(in sessionAccess) (sessionContext, error) {
	// Already-keyed consumers retain exact matching and lookup's activity touch.
	// Unlike adoption, they neither normalize the key nor inspect hosted entry
	// or harness metadata. In particular, a missing key never adopts the host.
	switch in.mode {
	case "mint-identity":
		// Existing identity-only mint consumers already supply the root and
		// lineage. Preserve the store-only operation: no hosted probe, extra
		// record read, or adoption normalization belongs to this path.
		key, err := s.sessions.mint(in.root, in.scope, in.parent)
		if err != nil {
			return sessionContext{}, err
		}
		return contextForEntry(key, sessionEntry{root: in.root, scope: in.scope, parent: in.parent}, true), nil
	case "identity":
		entry, found := s.sessions.lookup(in.key)
		return contextForEntry(in.key, entry, found), nil
	case "intake":
		entry, found := s.sessions.intakeLookup(in.key)
		return contextForEntry(in.key, entry, found), nil
	case "active":
		entry, found, transient := s.sessions.intakeInspect(in.key)
		out := contextForEntry(in.key, entry, found)
		out.transient = transient
		return out, nil
	}
	in.key, in.parent = strings.TrimSpace(in.key), strings.TrimSpace(in.parent)
	hosted := s.hostedSessionKey() // validates registration.host_pid once
	switch in.mode {
	case "bootstrap":
		if hosted != "" && in.parent == "" && in.scope == roleLead {
			if reason := s.leadKeyRefusal(hosted, in.root); reason != "" {
				return sessionContext{}, fmt.Errorf("session bootstrap: this session is hosted and its session key is fixed, but %s %q %s", envHostedSessionKey, hosted, reason)
			}
			in.key = hosted
		} else {
			if in.parent != "" {
				if _, ok := s.sessions.lookup(in.parent); !ok {
					return sessionContext{}, fmt.Errorf("session bootstrap: parent_session_key %q is not a known session key", in.parent)
				}
			}
			key, err := s.sessions.mint(in.root, in.scope, in.parent)
			if err != nil {
				return sessionContext{}, err
			}
			in.key = key
		}
	case "relogin":
		if hosted != "" && in.key != hosted {
			return sessionContext{}, hostedKeyRefusal("session bootstrap", hosted, in.key)
		}
		if in.parent != "" {
			return sessionContext{}, errors.New("session bootstrap: relogin_session_key cannot be combined with parent_session_key")
		}
		if in.scope != roleLead {
			return sessionContext{}, errors.New("session bootstrap: relogin_session_key requires lead capability")
		}
		if reason := s.leadKeyRefusal(in.key, in.root); reason != "" {
			return sessionContext{}, fmt.Errorf("session bootstrap: relogin_session_key %q %s", in.key, reason)
		}
		if err := s.hostedLoginGuard(in.key, in.root); err != nil {
			return sessionContext{}, err
		}
	case "manual":
		if hosted != "" && in.key != hosted {
			if entry, ok := s.sessions.lookup(in.key); !ok || entry.parent == "" {
				return sessionContext{}, hostedKeyRefusal("workflow_manual", hosted, in.key)
			}
		}
	case "mint":
		key, err := s.sessions.mint(in.root, in.scope, in.parent)
		if err != nil {
			return sessionContext{}, err
		}
		in.key = key
	}
	out := sessionContext{key: in.key, hosted: hosted != "" && in.key == hosted}
	if entry, ok := s.sessions.lookup(in.key); ok {
		out.found = true
		out.root, out.canonicalRoot, out.scope, out.parent = entry.root, entry.root, entry.scope, entry.parent
		if rec, err := s.sessions.readStateWithError(in.key, partKey); err == nil {
			out.harness, out.harnessSession, out.harnessCwd = rec.Harness, rec.HarnessSession, rec.HarnessCwd
		}
	}
	return out, nil
}

func contextForEntry(key string, entry sessionEntry, found bool) sessionContext {
	return sessionContext{key: key, root: entry.root, canonicalRoot: entry.root, scope: entry.scope, parent: entry.parent, found: found}
}

// lookupSession adapts existing identity-only consumers to the shared resolver
// without widening their reads or changing their refusal and error ordering.
func (s *Server) lookupSession(key string) (sessionEntry, bool) {
	resolved, _ := s.resolveSession(sessionAccess{mode: "identity", key: key})
	return resolved.entry(), resolved.found
}

func (c sessionContext) entry() sessionEntry {
	return sessionEntry{root: c.root, scope: c.scope, parent: c.parent}
}

// intakeSession preserves the adapter-only cache and its bounded revocation.
func (s *Server) intakeSession(key string) (sessionEntry, bool) {
	resolved, _ := s.resolveSession(sessionAccess{mode: "intake", key: key})
	return resolved.entry(), resolved.found
}

// activeSession is the timer's non-touching view of the same cache. A transient
// read is distinct from revocation so it cannot discard an in-flight turn.
func (s *Server) activeSession(key string) (sessionEntry, bool, bool) {
	resolved, _ := s.resolveSession(sessionAccess{mode: "active", key: key})
	return resolved.entry(), resolved.found, resolved.transient
}
