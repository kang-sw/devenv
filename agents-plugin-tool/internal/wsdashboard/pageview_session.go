// Session view projection builds live agent and page state.

package wsdashboard

import (
	"html/template"
	"path/filepath"
	"sort"
	"time"

	"github.com/kang-sw/devenv/internal/wsinbox"
	"github.com/kang-sw/devenv/internal/wslive"
	"github.com/kang-sw/devenv/internal/wsmailbox"
	"github.com/kang-sw/devenv/internal/wswhiteboard"
)

// subagentCounts counts the running agents of self's snapshot agents by
// depth: one whose parent is in the list is Deep, any other Direct (so the
// running child of an evicted parent counts as Direct; the sum stays right,
// since retention keeps every running agent). A running agent whose
// ReplyID lookup resolves to a session with a live adapter adds that
// session's running agents, at every depth and through its own delegates,
// to Deep; a session already counted (self included) is not counted again.
func subagentCounts(self string, agents []wslive.Agent, lookup sessionLookup, now time.Time) SubagentCount {
	c, _ := subagentProjection(self, agents, lookup, now)
	return c
}

// subagentProjection keeps the compact roster and its badge counts in lockstep.
func subagentProjection(self string, agents []wslive.Agent, lookup sessionLookup, now time.Time) (SubagentCount, []AgentNode) {
	var c SubagentCount
	var rows []AgentNode
	visited := map[string]bool{self: true}
	var linked func(agents []wslive.Agent)
	linked = func(agents []wslive.Agent) {
		for _, a := range agents {
			if a.Status != wslive.StatusRunning || a.ReplyID == "" || visited[a.ReplyID] {
				continue
			}
			visited[a.ReplyID] = true
			if _, ok := lookup.get(a.ReplyID); !ok {
				continue
			}
			if snap := lookup.snap(a.ReplyID); adapterPresent(snap, now) {
				for _, b := range snap.Agents {
					if b.Status == wslive.StatusRunning {
						c.Deep++
						rows = append(rows, AgentNode{Agent: b})
					}
				}
				linked(snap.Agents)
			}
		}
	}
	known := map[string]bool{}
	for _, a := range agents {
		known[a.ID] = true
	}
	for _, a := range agents {
		if a.Status != wslive.StatusRunning {
			continue
		}
		rows = append(rows, AgentNode{Agent: a})
		if a.ParentID != "" && a.ParentID != a.ID && known[a.ParentID] {
			c.Deep++
		} else {
			c.Direct++
		}
	}
	linked(agents)
	return c, rows
}

// ConvEntry is one conversation ring entry as the page shows it.
type ConvEntry struct {
	wslive.RingEntry
	// BoardTargets is render-only attribution from the current in-memory board.
	BoardTargets map[string]string
	// Key is the ring entry's file name, the element key the page keeps an
	// expanded mail line open by.
	Key string
	// At is the entry's time (epoch ms).
	At int64
	// UserBlocks keeps per-envelope bubbles under one stable ring/stream key.
	// EnvelopeAt is the original offset-bearing timestamp of one such bubble.
	UserBlocks []ConvEntry
	EnvelopeAt string
	// HTML is a lead reply's text rendered as markdown with its inline HTML
	// sanitized; user messages, mail bodies, and turn lines stay plain text,
	// raw HTML included. It is set only while the entry renders on an entry
	// cache miss.
	HTML template.HTML
	// Images are a user message's attached images, taken out of its text.
	Images []AttachmentView
	// Content keeps grouped source/memo/image display in message order;
	// nil retains the legacy Text rendering path.
	Content []SelectionContentPart
	// Answers are the lines of the answer block a user message starts with,
	// taken out of its text; the bubble shows them as capsules.
	Answers []wsinbox.AnswerLine
	// Mentions are the mention elements after that block (or opening the
	// message), taken out of its text and shown as capsules after the
	// answer capsules.
	Mentions      []wsinbox.Mention
	CloseRequests []wsinbox.CloseRequest
	// Picks and Consoles are the web card's pick and console elements after
	// the mentions, taken out of its text and shown as capsules after them.
	Picks    []wsinbox.Pick
	Consoles []wsinbox.Console
	// Rendered is the entry's whole element, from the entry cache.
	Rendered template.HTML
	// Body is a mail line's full message from the mailbox history; HasBody
	// is false once the message has left that ring, and the line then shows
	// only its first line.
	Body    string
	HasBody bool
	// MailWakeSuppressed leaves only a hidden keyed marker when this mail is
	// shown at its associated wake or nudge run. Keeping the key preserves cursors.
	MailWakeSuppressed bool
	// BoardNudgeMails holds each raw envelope once on the first row of a
	// collapsed run. Ring text and all absorbed entry keys remain unchanged.
	BoardNudgeMails []string
	// PeerReplyID links a mail line's peer to its page when PeerLinked.
	PeerReplyID string
	PeerLinked  bool
	// PeerLabel is the display label of a served peer the line names by its
	// id: address, shown in place of Peer (which stays the tooltip); empty
	// when the peer has none, is not served, or is a named address.
	PeerLabel string
	// Lang is the language the entry renders in; the entry cache keys on it.
	Lang string
}

// WhiteboardChanges decodes only the supported carrier; values remain plain
// strings for html/template to escape, never markdown or panel HTML.
func (e ConvEntry) WhiteboardChanges() []wswhiteboard.Effect {
	effects, _ := wslive.DecodeWhiteboardEffects(e.RingEntry)
	return effects
}

func (e ConvEntry) WhiteboardFallback() string {
	return wslive.WhiteboardEffectsFallback(e.RingEntry)
}

// pageInputs is what buildSessionView reads.
type pageInputs struct {
	Session Session
	Page    PageContent
	// Snapshot is the session's live snapshot; nil when it has none.
	Snapshot *wslive.Snapshot
	Ring     []wslive.RingRecord
	// Held and Drained are the session's board-answer inbox: the messages
	// sent from the page and not yet received, and the newest received ones.
	Held, Drained []wsinbox.Record
	// AttachDir is the session's absolute inbox attachments directory: an
	// image path inside it shows as a thumbnail.
	AttachDir string
	Now       time.Time
	Version   string
	// HandedOff reports that the session's agent record is a lead's that
	// was handed to a terminal.
	HandedOff bool
	// IsChild marks a spawned worker or card-bound subordinate session.
	IsChild                                   bool
	ChildCloseKnown, ChildClosed, ChildActive bool
	// Model and Effort are the session's configured spawn/selected values.
	Model, Effort string
	// Lookup resolves the other sessions this daemon serves that the view
	// names.
	Lookup sessionLookup
	// Parents resolves the session's owners for the head's parent jump; the
	// zero value finds none beyond the registration's own ParentReplyID.
	Parents parentSource
}

// sessionLookup resolves the sessions a page view names: session reads the
// index alone, and snapshot reads a session's live snapshot (nil when it has
// none), so a caller that uses only whether a session is served, or whether
// it has a board, reads no snapshot. The zero value serves none.
type sessionLookup struct {
	session  func(replyID string) (Session, bool)
	snapshot func(replyID string) *wslive.Snapshot
	// observed is needed only by non-compact peer child hovers. Counts and
	// compact peer rows use snapshot, which need not sweep retained history.
	observed func(replyID string) *wslive.Snapshot
	// piDeleted checks revival only for settled Pi rows, never native rows.
	piDeleted func(replyID string) bool
}

// get returns a session this daemon serves.
func (l sessionLookup) get(replyID string) (Session, bool) {
	if l.session == nil {
		return Session{}, false
	}
	return l.session(replyID)
}

// snap returns a served session's live snapshot; nil when it has none.
func (l sessionLookup) snap(replyID string) *wslive.Snapshot {
	if l.snapshot == nil {
		return nil
	}
	return l.snapshot(replyID)
}

func (l sessionLookup) observedSnap(replyID string) *wslive.Snapshot {
	if l.observed != nil {
		return l.observed(replyID)
	}
	return l.snap(replyID)
}

// adapterPresent is wslive.AdapterPresent on an already read snapshot.
func adapterPresent(snap *wslive.Snapshot, now time.Time) bool {
	return snap != nil && snap.Adapter != "" && !snap.Stale(now)
}

// harnessRunning uses the hosted harness PID, not the holder heartbeat. A
// terminal session has only its registration heartbeat as a liveness signal.
func harnessRunning(reg Registration, now time.Time) bool {
	if reg.Hosted {
		return !reg.HostStopped && reg.HostPID > 0 && processAlive(reg.HostPID)
	}
	return reg.Live(now)
}

func buildSessionView(in pageInputs) SessionView {
	reg := in.Session.Registration
	v := SessionView{
		ReplyID:          reg.ReplyID,
		Parent:           resolveParent(in.Session, in.Parents),
		Label:            in.Session.Label,
		Project:          reg.Project,
		ProjectName:      filepath.Base(reg.Project),
		Harness:          reg.Harness,
		Live:             reg.Live(in.Now),
		ProcessRunning:   harnessRunning(reg, in.Now),
		Adapter:          adapterPresent(in.Snapshot, in.Now),
		Hosted:           reg.Hosted,
		HandedOff:        in.HandedOff && !reg.Hosted && !reg.Live(in.Now),
		IsChild:          in.IsChild || reg.ParentReplyID != "",
		ChildCloseKnown:  in.ChildCloseKnown,
		ChildClosed:      in.ChildClosed,
		ChildActive:      in.ChildActive,
		Model:            in.Model,
		Effort:           in.Effort,
		Version:          in.Version,
		HasBoard:         in.Page.HasBoard,
		Seq:              in.Page.Seq,
		BoardIncarnation: in.Page.BoardIncarnation,
		CountPolicy:      in.Page.Board.CountPolicy,
		CountAcks:        in.Page.Board.CountAcks,
		Board:            in.Page.Board.Board,
		BoardUnits:       in.Page.BoardUnits,
		Panels:           in.Page.Board.Panels,
		Mermaid:          in.Page.Board.Mermaid,
	}
	if in.Snapshot != nil {
		v.ActivityAt = in.Snapshot.UpdatedAt
		if o := in.Snapshot.Lead.Observed; o != nil {
			v.ModelHistory = append(v.ModelHistory, ModelHistoryRow{Observed: o})
			if v.Model == "" {
				v.Model = o.Current.Model
			}
			if v.Effort == "" {
				v.Effort = o.Current.Effort
			}
		}
		histories := map[string]ModelHistoryRow{}
		for id, o := range in.Snapshot.ObservedAgents {
			if o != nil {
				histories[id] = ModelHistoryRow{ID: id, Observed: o}
			}
		}
		for _, a := range in.Snapshot.Agents {
			if a.Observed != nil {
				histories[a.ID] = ModelHistoryRow{ID: a.ID, Label: a.Label, Observed: a.Observed}
			}
		}
		var ids []string
		for id := range histories {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			v.ModelHistory = append(v.ModelHistory, histories[id])
		}
	}
	v.CanEditSettings = v.Hosted && !v.IsChild
	if !v.Adapter {
		// The registry/page source already applies serving eligibility. Do not
		// promote a runner child to a lead, or use stale snapshot activity.
		if !reg.Agent && (len(in.Ring) > 0 || len(v.ModelHistory) > 0) {
			v.SavedConversation = true
			contacts := resolvedContacts(in.Page.Contacts, reg.ReplyID)
			v.Conversation = conversation(wslive.Lead{}, in.Ring, in.Page.MailBodies, contacts, in.Lookup, replyMarkdown(reg.Project, reg.ReplyID), reg.ReplyID)
			splitUserEntries(v.Conversation.Entries, in.AttachDir, reg.ReplyID)
			attributeBoardNavigation(&v.Conversation, in.Page.BoardTargets)
		}
		return v
	}
	lookup := in.Lookup
	v.Lead = leadView(in.Snapshot.Lead)
	v.Band = bandView(in.Snapshot)
	v.Commands = in.Snapshot.Commands
	v.Host = in.Snapshot.Host
	v.Background, v.Monitors = taskBadgeCounts(in.Snapshot)
	v.Tree.Agents = agentTree(in.Snapshot.Agents, lookup)
	for _, a := range in.Snapshot.Agents {
		switch a.Status {
		case wslive.StatusRunning:
			v.Tree.Running++
		case wslive.StatusFailed:
			v.Tree.Failed++
		default:
			v.Tree.Finished++
		}
	}
	contacts := resolvedContacts(in.Page.Contacts, reg.ReplyID)
	v.Tree.Peers = peerGroup(contacts, reg.Project, lookup, in.Now)
	v.Conversation = conversation(in.Snapshot.Lead, in.Ring, in.Page.MailBodies, contacts, lookup, replyMarkdown(reg.Project, reg.ReplyID), reg.ReplyID)
	splitUserEntries(v.Conversation.Entries, in.AttachDir, reg.ReplyID)
	v.Conversation.Held = heldViews(in.Held, in.AttachDir, reg.ReplyID)
	attributeBoardNavigation(&v.Conversation, in.Page.BoardTargets)
	v.Conversation.Sent = sentMarks(in.Drained, in.Held, in.Page.Reasked, settledTurnStart(in.Ring))
	v.Tree.Subagents, v.Tree.CompactAgents = subagentProjection(reg.ReplyID, in.Snapshot.Agents, lookup, in.Now)
	v.Tree.HUD = hudProjection(reg.ReplyID, v.Label, v.Harness, in.Snapshot.Agents, len(v.Tree.Peers.Live), lookup, in.Now)
	v.Conversation.Subagents = v.Tree.Subagents
	return v
}

// leadView splits the lead's batches into finished ones and its live row.
func leadView(lead wslive.Lead) LeadView {
	lv := LeadView{Lead: lead}
	lv.Finished, lv.Running, lv.Generating = splitBatches(lead.Batches)
	return lv
}

// splitBatches separates finished batches from the running call and the
// message still generating.
func splitBatches(batches []wslive.BatchView) (finished []wslive.BatchView, running *wslive.RunningView, generating *wslive.GeneratingView) {
	for i := range batches {
		b := batches[i]
		switch {
		case b.Running != nil:
			running = b.Running
		case b.Generating != nil:
			generating = b.Generating
		default:
			finished = append(finished, b)
		}
	}
	return finished, running, generating
}

// liveSlots picks a running agent's liveSlotCount rows: the latest entries of
// its finished batches, then the running call, then a tool call generating its
// arguments, oldest first. A pending permission prompt takes the slot in place
// of the newest entry. Thinking and replying add no entry, so the slot keeps
// the latest call; a placeholder fills it when there is none.
func liveSlots(perm *wslive.Permission, finished []wslive.BatchView, running *wslive.RunningView, generating *wslive.GeneratingView) []LiveSlot {
	var entries []LiveSlot
	for i := range finished {
		entries = append(entries, LiveSlot{Finished: &finished[i]})
	}
	if running != nil {
		entries = append(entries, LiveSlot{Running: running})
	}
	if generating != nil && generating.Kind == wslive.PhaseTool {
		entries = append(entries, LiveSlot{Generating: generating})
	}
	keep := liveSlotCount
	var slots []LiveSlot
	if perm != nil {
		slots = append(slots, LiveSlot{Permission: perm})
		keep--
	}
	if len(entries) > keep {
		entries = entries[len(entries)-keep:]
	}
	slots = append(slots, entries...)
	for len(slots) < liveSlotCount {
		slots = append(slots, LiveSlot{})
	}
	return slots
}

// agentTree nests the snapshot's flat agent list by parent id, keeping the
// snapshot's order among siblings. An agent whose parent is not in the list
// hangs from the root, so retention never hides a running grandchild.
func agentTree(agents []wslive.Agent, lookup sessionLookup) []AgentNode {
	known := map[string]bool{}
	for _, a := range agents {
		known[a.ID] = true
	}
	children := map[string][]wslive.Agent{}
	var roots []wslive.Agent
	for _, a := range agents {
		if a.ParentID != "" && a.ParentID != a.ID && known[a.ParentID] {
			children[a.ParentID] = append(children[a.ParentID], a)
		} else {
			roots = append(roots, a)
		}
	}
	seen := map[string]bool{}
	var build func(list []wslive.Agent) []AgentNode
	build = func(list []wslive.Agent) []AgentNode {
		var out []AgentNode
		for _, a := range list {
			if seen[a.ID] {
				continue // a parent cycle in a malformed snapshot
			}
			seen[a.ID] = true
			node := AgentNode{Agent: a}
			node.Finished, node.Running, node.Generating = splitBatches(a.Batches)
			if node.IsRunning() {
				node.Slots = liveSlots(node.Permission, node.Finished, node.Running, node.Generating)
			}
			if a.ReplyID != "" {
				if a.AgentKind == wslive.AgentKindPi && a.Status == wslive.StatusDone && lookup.piDeleted != nil {
					node.PiDeleted = lookup.piDeleted(a.ReplyID)
				}
				// A conversation the agent runner hosts (a spawned Pi
				// subagent) has a page and no board.
				if s, ok := lookup.get(a.ReplyID); ok && (s.HasBoard || s.Agent) {
					node.Linked = true
				}
			}
			node.Children = build(children[a.ID])
			out = append(out, node)
		}
		return out
	}
	return build(roots)
}

// resolvedContacts drops the session's own address and orders the rest by
// last exchange, newest first.
func resolvedContacts(contacts []Contact, self string) []Contact {
	out := make([]Contact, 0, len(contacts))
	for _, c := range contacts {
		if c.ReplyID == self || c.Address == wsmailbox.ReplyIDPrefix+self {
			continue
		}
		out = append(out, c)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].LastExchange.After(out[j].LastExchange) })
	return out
}
