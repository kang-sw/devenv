package mcp

import (
	"errors"
	"fmt"
	"time"

	"github.com/kang-sw/devenv/internal/wsxmlcmd"
)

// The tracker tools: tracker.update applies one batch of tk: commands (the
// shared command language, package wsxmlcmd) to the machine's tracker
// stores through wstrackerhost, and tracker.query reads them. Both are
// always listed and work whatever workflow.tracker_enabled says; a
// successful call turns that knob on at session scope, which adds the
// tracker section to the workflow manual. The command table and rules live
// in the tracker playbook, which the manual section, the refusals, and
// playbook.read all quote. tracker.show (tracker_show.go) and tracker.ui
// (tracker_ui.go) are listed only where a support conversation's profile
// allows them; the same gate admits it
// for that conversation alone, and trackerLeadOnlyDenial refuses it to any
// other spawned session.

const (
	trackerPlaybookName = "tracker"
	trackerPrefix       = "tk:"
	// trackerRegisterTimeout bounds a tk:register: a clone of the remote,
	// the join, and its push. Sync bounds itself (wstrackerhost).
	trackerRegisterTimeout = 3 * time.Minute
)

func isTrackerTool(name string) bool {
	return name == "tracker.update" || name == "tracker.query"
}

func trackerLeadOnlyDenial(name string) string {
	if name == trackerShowTool {
		return fmt.Sprintf("%s is not available to a spawned session: only a tracker store's support conversation draws item cards, in that store's chat.", name)
	}
	if name == trackerUITool {
		return fmt.Sprintf("%s is not available to a spawned session: only a tracker store's support conversation moves the tracker view, from that store's chat.", name)
	}
	return fmt.Sprintf("%s is not available to a spawned session: the tracker records the user's work through the top-level lead. Put progress in your final report; your lead records it.", name)
}

// trackerSessionKey returns the caller's session key when it names a
// top-level lead, or a store's support conversation whose profile names
// tool; for the latter it also returns the conversation's scope, whose
// store pins its writes. The call gate refuses a spawned key only when it
// finds the key, so a key the store cannot find (made up, stale, expired)
// would pass it; this refuses one before any registry read or write.
func (s *Server) trackerSessionKey(tool string, args map[string]any) (string, *trackerSupportScope, error) {
	key, err := sessionStateKey(tool, args)
	if err != nil {
		return "", nil, err
	}
	entry, found := s.lookupSession(key)
	if !found {
		return "", nil, mailboxUnknownSessionError(tool)
	}
	if entry.isTopLevelLead() {
		return key, nil, nil
	}
	if scope, ok := s.trackerSupport(key, entry); ok && scope.tools[tool] {
		return key, &scope, nil
	}
	return "", nil, errors.New(trackerLeadOnlyDenial(tool))
}

// supportPinProblems records what a support conversation's batch may not
// do: send a store command (register, sync, unregister, relabel, hide, or
// unhide; the conversation runs in a store's clone, so it must not remove
// or rename one), or write to a store other than the conversation's own.
func supportPinProblems(p *wsxmlcmd.Problems, b *trackerBatch, store string) {
	if b.storeCmd != nil {
		p.Add("%s: a store's support conversation cannot register, sync, unregister, relabel, hide, or unhide a store; the user does that from the dashboard's tracker", b.storeCmd.Label())
	}
	// A batch writes to the one store its <tk:store> group names (b.store);
	// a second group is already refused by the parse.
	if b.store != "" && b.store != store && len(b.events) > 0 {
		at := b.events[0].cmd.Label()
		if sc := b.events[0].cmd.Scope; sc != nil {
			at = sc.At()
		}
		p.Add("%s: %q; this conversation writes to its own store %q only, and every other store is read-only here", at, b.store, store)
	}
}

// trackerOrigins are the origins a tracker.update write may carry: every
// write through this tool is an agent's, so human is not among them, and
