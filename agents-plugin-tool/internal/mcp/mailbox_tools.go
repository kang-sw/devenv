package mcp

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/kang-sw/devenv/internal/wsmailbox"
)

// mailbox_tools.go implements the three host-neutral MCP tools
// (mailbox.send/mailbox.recv/mailbox.lookup_peers, Decision 5) and the
// central response-piggyback wrapper (Decision: piggyback badge). Identity,
// registration, gating, and stamping policy live in mailbox_runtime.go;
// this file is the thin MCP-argument/response layer over it.

// mailboxUnknownSessionError mirrors resolveToolRoot/requireLeadSessionKey's
// unknown_session error text, so a stale/expired session_key produces the
// same re-login guidance from every ws.* tool.
func mailboxUnknownSessionError(tool string) error {
	return fmt.Errorf("%s: unknown_session: session key not found; "+
		"if you are the lead, re-bootstrap your session per ws:workflow-manual with your known root and retry the call", tool)
}

func (s *Server) handleMailboxSend(id json.RawMessage, args map[string]any) response {
	const tool = "mailbox.send"
	sessionKey, err := sessionStateKey(tool, args)
	if err != nil {
		return toolTextResponse(id, "", err)
	}
	entry, found := s.sessions.lookup(sessionKey)
	if !found {
		return toolTextResponse(id, "", mailboxUnknownSessionError(tool))
	}
	to, err := stringArg(tool, "to", args)
	if err != nil {
		return toolTextResponse(id, "", err)
	}
	content, err := stringArg(tool, "content", args)
	if err != nil {
		return toolTextResponse(id, "", err)
	}
	addr, err := wsmailbox.ParseAddress(to)
	if err != nil {
		return toolTextResponse(id, "", fmt.Errorf("%s: %w", tool, err))
	}

	// Universal send + always-on reply-id (Decision 11): every send opens/
	// refreshes the sender's own return channel, regardless of whether it
	// also holds a durable slug.
	replyID, err := s.publishReplyID(sessionKey)
	if err != nil {
		return toolTextResponse(id, "", fmt.Errorf("%s: %w", tool, err))
	}
	s.markMailboxReplyOpened(sessionKey)

	now := mailboxNowString()

	// The from: stamp claims this process's WS_MAILBOX/WS_MAILBOX_AUTO
	// identity on the sender's behalf, so it is itself gated by the
	// caller==owner check (Decision 3: "governs the named slug inbox (send
	// to it, recv from it, and its piggyback badge)") — a non-owner caller
	// sharing this process must not be able to claim the owner's slug
	// identity when sending, only fall back to its own reply-id. An
	// unbound owner pointer (no ferrule yet) also means nobody may claim it.
	var from string
	if isOwner, identity, oerr := s.mailboxOwnerCheck(sessionKey, entry.root); oerr == nil && isOwner {
		switch addr.Kind {
		case wsmailbox.AddressSlug:
			from = mailboxFromStamp(identity, addr.Scope, false)
		case wsmailbox.AddressReplyID:
			from = mailboxFromStamp(identity, "", true)
		}
	}
	replyTo := ""
	if from == "" {
		replyTo = wsmailbox.ReplyIDPrefix + replyID
	}
	envelope := wsmailbox.Envelope{From: from, ReplyTo: replyTo, Content: content, SentAt: now}

	switch addr.Kind {
	case wsmailbox.AddressSlug:
		path, perr := wsmailbox.PathForScope(addr.Scope, entry.root)
		if perr != nil {
			return toolTextResponse(id, "", fmt.Errorf("%s: %w", tool, perr))
		}
		if werr := wsmailbox.WithLock(path, func(store *wsmailbox.StoreFile) error {
			store.Queues = wsmailbox.AppendQueue(store.Queues, addr.Name, envelope)
			return nil
		}); werr != nil {
			return toolTextResponse(id, "", fmt.Errorf("%s: %w", tool, werr))
		}
	case wsmailbox.AddressReplyID:
		path, perr := wsmailbox.ReplyRegistryPath()
		if perr != nil {
			return toolTextResponse(id, "", fmt.Errorf("%s: %w", tool, perr))
		}
		if werr := wsmailbox.WithReplyLock(path, func(store *wsmailbox.ReplyStore) error {
			store.Queues = wsmailbox.AppendQueue(store.Queues, addr.ReplyID, envelope)
			return nil
		}); werr != nil {
			return toolTextResponse(id, "", fmt.Errorf("%s: %w", tool, werr))
		}
	}

	stampLabel, stampValue := "reply_to", replyTo
	if from != "" {
		stampLabel, stampValue = "from", from
	}
	result := map[string]any{
		"to": to, "sent_at": now, "your_reply_id": wsmailbox.ReplyIDPrefix + replyID,
		"stamp": stampLabel, "stamp_value": stampValue,
	}
	if wantsJSON(args) {
		return toolJSONResponse(id, result, nil)
	}
	return toolTextResponse(id, fmt.Sprintf("sent to %s\n%s: %s\nyour_reply_id: %s%s\n", to, stampLabel, stampValue, wsmailbox.ReplyIDPrefix, replyID), nil)
}

func (s *Server) handleMailboxRecv(id json.RawMessage, args map[string]any) response {
	const tool = "mailbox.recv"
	sessionKey, err := sessionStateKey(tool, args)
	if err != nil {
		return toolTextResponse(id, "", err)
	}
	entry, found := s.sessions.lookup(sessionKey)
	if !found {
		return toolTextResponse(id, "", mailboxUnknownSessionError(tool))
	}

	var drained []wsmailbox.Envelope
	var readErrs []error

	// Named-inbox drain: only when this session currently holds the owner
	// pointer (server-layer caller==owner gate, Decision 3). The tentative
	// drain is accumulated in a block-local slice and only merged into the
	// result after WithLock's write (temp+rename) has actually succeeded:
	// merging it beforehand — while the closure return value is still
	// pending the real disk write — risks reporting a message as delivered
	// while it is still sitting in the on-disk queue, causing a duplicate
	// delivery on the next recv.
	if isOwner, identity, oerr := s.mailboxOwnerCheck(sessionKey, entry.root); oerr != nil {
		readErrs = append(readErrs, fmt.Errorf("named-inbox owner check: %w", oerr))
	} else if isOwner {
		if path, perr := wsmailbox.PathForScope(identity.Scope, entry.root); perr != nil {
			readErrs = append(readErrs, fmt.Errorf("named-inbox path: %w", perr))
		} else {
			var ownerDrained []wsmailbox.Envelope
			now := mailboxNowString()
			werr := wsmailbox.WithLock(path, func(store *wsmailbox.StoreFile) error {
				ownerDrained = append(ownerDrained, store.Queues[identity.Name]...)
				delete(store.Queues, identity.Name)
				if p, ok := store.Presence[identity.Name]; ok {
					p.LastSeen = now
					store.Presence[identity.Name] = p
				}
				return nil
			})
			if werr != nil {
				readErrs = append(readErrs, fmt.Errorf("named-inbox drain: %w", werr))
			} else {
				drained = append(drained, ownerDrained...)
			}
		}
	}

	// Own reply-id queue: always drained, authorized intrinsically by the
	// caller's own session_key (Decision 3's carve-out), independent of
	// owner status. Same drain-after-confirmed-write discipline as above.
	if replyID, rerr := s.mailboxReplyID(sessionKey); rerr != nil {
		readErrs = append(readErrs, fmt.Errorf("reply-id: %w", rerr))
	} else if path, perr := wsmailbox.ReplyRegistryPath(); perr != nil {
		readErrs = append(readErrs, fmt.Errorf("reply-id registry path: %w", perr))
	} else {
		var replyDrained []wsmailbox.Envelope
		werr := wsmailbox.WithReplyLock(path, func(store *wsmailbox.ReplyStore) error {
			replyDrained = append(replyDrained, store.Queues[replyID]...)
			delete(store.Queues, replyID)
			return nil
		})
		if werr != nil {
			readErrs = append(readErrs, fmt.Errorf("reply-id drain: %w", werr))
		} else {
			drained = append(drained, replyDrained...)
		}
	}

	// A genuine storage error must not be reported as "no unread mail": that
	// would silently hide mail the caller cannot currently reach behind an
	// indistinguishable empty-inbox response. Partial success (one queue
	// drained, the other errored) still returns what was actually drained,
	// with the error only logged — failing the whole call would discard
	// mail that *was* successfully retrieved.
	if len(readErrs) > 0 {
		if len(drained) == 0 {
			return toolTextResponse(id, "", fmt.Errorf("%s: %w", tool, readErrs[0]))
		}
		for _, rerr := range readErrs {
			appendDebugEvent("mailbox.recv_partial_error", map[string]any{"error": rerr.Error()})
		}
	}

	sort.SliceStable(drained, func(i, j int) bool { return drained[i].SentAt < drained[j].SentAt })

	if wantsJSON(args) {
		return toolJSONResponse(id, drained, nil)
	}
	if len(drained) == 0 {
		return toolTextResponse(id, "no unread mail\n", nil)
	}
	var b strings.Builder
	for _, m := range drained {
		handle := m.From
		if handle == "" {
			handle = m.ReplyTo
		}
		fmt.Fprintf(&b, "[%s] %s: %s\n", m.SentAt, handle, m.Content)
	}
	return toolTextResponse(id, b.String(), nil)
}

// mailboxPeerOut is lookup_peers' per-peer JSON/text shape.
type mailboxPeerOut struct {
	Address   string `json:"address"`
	Harness   string `json:"harness,omitempty"`
	Cwd       string `json:"cwd,omitempty"`
	StartedAt string `json:"started_at,omitempty"`
	Conflict  bool   `json:"conflict,omitempty"`
}

func (s *Server) handleMailboxLookupPeers(id json.RawMessage, args map[string]any) response {
	const tool = "mailbox.lookup_peers"
	sessionKey, err := sessionStateKey(tool, args)
	if err != nil {
		return toolTextResponse(id, "", err)
	}
	entry, found := s.sessions.lookup(sessionKey)
	if !found {
		return toolTextResponse(id, "", mailboxUnknownSessionError(tool))
	}
	scopeRaw, err := stringArg(tool, "scope", args)
	if err != nil {
		return toolTextResponse(id, "", err)
	}
	if !wsmailbox.IsValidScope(scopeRaw) {
		return toolTextResponse(id, "", fmt.Errorf("%s: scope must be machine, worktree, or clone", tool))
	}
	scope := wsmailbox.Scope(scopeRaw)

	path, perr := wsmailbox.PathForScope(scope, entry.root)
	if perr != nil {
		return toolTextResponse(id, "", fmt.Errorf("%s: %w", tool, perr))
	}
	store, lerr := wsmailbox.Load(path)
	if lerr != nil {
		return toolTextResponse(id, "", fmt.Errorf("%s: %w", tool, lerr))
	}

	// isOwner mirrors the exact condition self["address"] gets populated
	// under, so the peer-enumeration loop below can drop the caller's own
	// live entry with the same predicate. mailboxOwnerCheck already
	// resolves and Active-guards identity internally (file idiom, see
	// mailboxFromStamp/handleMailboxSend/mailboxHasConflict call sites), so
	// its returned identity is reused here rather than re-resolving it via
	// a second mailboxIdentityResolved() call.
	isOwner, identity, ownerErr := s.mailboxOwnerCheck(sessionKey, entry.root)
	if ownerErr != nil {
		isOwner = false
	}

	now := mailboxNow()
	names := make([]string, 0, len(store.Presence))
	for name := range store.Presence {
		names = append(names, name)
	}
	sort.Strings(names)
	var peers []mailboxPeerOut
	selfConflict := false
	for _, name := range names {
		p := store.Presence[name]
		if !mailboxPresenceLive(p, now) {
			continue // lookup_peers filters dead peers (Decision: liveness)
		}
		// Self-exclusion (260913-bug-mailbox-lookup-peers-self-leak): the
		// caller's own registered named inbox must surface only under
		// self, never inside peers[]. A caller's own reply-id never faces
		// this because it lives in an entirely separate store (the reply-id
		// registry, not this scope's Presence map); a named inbox shares
		// this same Presence map with every other name in scope, so it
		// needs an explicit exclusion by owned identity (name+scope)
		// instead. This only ever drops the caller's own live entry — a
		// genuinely distinct second name (or a conflicted duplicate of a
		// name this session does not own) is untouched.
		if isOwner && identity.Scope == scope && name == identity.Name {
			// Presence is name-keyed, so a contesting second live process
			// claiming this same name flags Conflict on THIS, the
			// rightful owner's, own record (mailbox_runtime.go's
			// ensureMailboxRegistered) rather than creating a second
			// entry. Dropping this entry must not swallow that signal —
			// the owner is the party that most needs the warning — so
			// relocate it into self instead (repair of the self-leak
			// hotfix, which dropped the entry without relocating this).
			selfConflict = p.Conflict
			continue
		}
		peers = append(peers, mailboxPeerOut{
			Address: name + "@" + string(scope), Harness: p.Harness, Cwd: p.Cwd, StartedAt: p.StartedAt, Conflict: p.Conflict,
		})
	}

	self := map[string]any{}
	if isOwner {
		s.refreshMailboxPresenceHeartbeat(entry.root)
		self["address"] = identity.address()
		self["auto"] = identity.Auto
		if selfConflict {
			self["conflict"] = true
		}
	}
	if _, hasAddress := self["address"]; !hasAddress {
		// Decision 5: an env-less self-lookup — or a non-owner session
		// sharing this process's identity, which cannot recv from the named
		// inbox either — publishes the caller's reply-id (the same
		// channel-open a send performs) rather than returning an address
		// this particular session cannot actually use.
		if replyID, rerr := s.publishReplyID(sessionKey); rerr == nil {
			s.markMailboxReplyOpened(sessionKey)
			self["reply_id"] = wsmailbox.ReplyIDPrefix + replyID
		}
	}

	if wantsJSON(args) {
		return toolJSONResponse(id, map[string]any{"self": self, "peers": peers}, nil)
	}
	var b strings.Builder
	if addr, ok := self["address"].(string); ok {
		autoTag := ""
		if v, _ := self["auto"].(bool); v {
			autoTag = " (auto-identity)"
		}
		conflictTag := ""
		if v, _ := self["conflict"].(bool); v {
			conflictTag = " CONFLICT: another live process also claims this name"
		}
		fmt.Fprintf(&b, "self: %s%s%s\n", addr, autoTag, conflictTag)
	} else if rid, ok := self["reply_id"].(string); ok {
		fmt.Fprintf(&b, "self: %s (reply-id only; no durable inbox)\n", rid)
	}
	if len(peers) == 0 {
		b.WriteString("no live peers\n")
	}
	for _, p := range peers {
		line := p.Address
		if p.Harness != "" {
			line += " harness=" + p.Harness
		}
		if p.Cwd != "" {
			line += " cwd=" + p.Cwd
		}
		if p.StartedAt != "" {
			line += " started_at=" + p.StartedAt
		}
		if p.Conflict {
			line += " CONFLICT: multiple live processes claim this name"
		}
		b.WriteString(line + "\n")
	}
	return toolTextResponse(id, b.String(), nil)
}

// --- central piggyback wrapper ------------------------------------------

// applyMailboxPiggyback appends an unread-mail badge to resp's text content
// when sessionKey has unread mail, per the single central dispatch wrapper
// decision. It is a no-op passthrough for any response shape it does not
// recognize (an error response, or one with no text content), and for a
// session with nothing to report.
//
// jsonFormat must be true when the original call requested format:"json"
// (wantsJSON(params.Arguments)): toolJSONResponse builds the exact same
// content[0]["text"] shape as a text response, just filled with marshalled
// JSON — concatenating badge text onto it would corrupt that JSON for any
// caller parsing it structurally. Rather than trying to detect this from
// the response shape alone (indistinguishable from plain text) or thread a
// sibling field through every arbitrarily-shaped JSON result (some of
// which marshal to a bare array, with no object to attach a field to), the
// badge is simply suppressed for JSON-format callers: automation consuming
// format:"json" is not the ambient-badge audience Decision 1's piggyback
// exists for.
func (s *Server) applyMailboxPiggyback(resp response, sessionKey string, jsonFormat bool) response {
	if jsonFormat {
		return resp
	}
	badge := s.mailboxPiggybackBadge(sessionKey)
	if badge == "" {
		return resp
	}
	result, ok := resp.Result.(map[string]any)
	if !ok {
		return resp
	}
	if isErr, _ := result["isError"].(bool); isErr {
		return resp
	}
	content, ok := result["content"].([]map[string]string)
	if !ok || len(content) == 0 {
		return resp
	}
	content[0]["text"] = content[0]["text"] + badge
	return resp
}

// mailboxPiggybackBadge computes the unread badge text for sessionKey, or
// "" when there is nothing to show. It is the single point that decides
// whether to touch mailbox storage at all: a session that carries no
// active process identity AND has never opened its own reply-id channel
// costs exactly one cheap in-memory-cost session-record read and no
// mailbox-store I/O (Decision 1's zero-overhead contract).
func (s *Server) mailboxPiggybackBadge(sessionKey string) string {
	sessionKey = strings.TrimSpace(sessionKey)
	if sessionKey == "" {
		return ""
	}
	identity := s.mailboxIdentityResolved()
	record, found := s.sessions.readState(sessionKey)
	if !found {
		return ""
	}
	checkReply := record.MailboxReplyOpened
	if !identity.Active && !checkReply {
		return ""
	}

	var unread int
	var senders []string

	if identity.Active {
		s.refreshMailboxPresenceHeartbeat(record.Root)
		if isOwner, _, oerr := s.mailboxOwnerCheck(sessionKey, record.Root); oerr == nil && isOwner {
			if path, perr := wsmailbox.PathForScope(identity.Scope, record.Root); perr == nil {
				if store, lerr := wsmailbox.Load(path); lerr == nil {
					if queue := store.Queues[identity.Name]; len(queue) > 0 {
						unread += len(queue)
						senders = append(senders, mailboxSenderLabels(queue)...)
					}
				}
			}
		}
	}

	if checkReply {
		if replyID, rerr := s.mailboxReplyID(sessionKey); rerr == nil {
			if path, perr := wsmailbox.ReplyRegistryPath(); perr == nil {
				if store, lerr := wsmailbox.LoadReplyStore(path); lerr == nil {
					if queue := store.Queues[replyID]; len(queue) > 0 {
						unread += len(queue)
						senders = append(senders, mailboxSenderLabels(queue)...)
					}
				}
			}
		}
	}

	if unread == 0 {
		return ""
	}
	return formatMailboxBadge(unread, senders)
}

func mailboxSenderLabels(queue []wsmailbox.Envelope) []string {
	out := make([]string, 0, len(queue))
	for _, m := range queue {
		handle := m.From
		if handle == "" {
			handle = m.ReplyTo
		}
		out = append(out, handle)
	}
	return out
}

func dedupMailboxSenders(raw []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(raw))
	for _, v := range raw {
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
}

// formatMailboxBadge renders the "unread N: from X -> call mailbox.recv"
// badge text, capped to 3 distinct sender handles.
func formatMailboxBadge(unread int, senders []string) string {
	labels := dedupMailboxSenders(senders)
	if len(labels) == 0 {
		return fmt.Sprintf("\nunread %d: call mailbox.recv\n", unread)
	}
	const capN = 3
	shown := labels
	suffix := ""
	if len(shown) > capN {
		shown = shown[:capN]
		suffix = ", …"
	}
	return fmt.Sprintf("\nunread %d: from %s%s -> call mailbox.recv\n", unread, strings.Join(shown, ", "), suffix)
}
