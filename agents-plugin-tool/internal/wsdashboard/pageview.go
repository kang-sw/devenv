package wsdashboard

import (
	"html/template"
	"math"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/kang-sw/devenv/internal/wsinbox"
	"github.com/kang-sw/devenv/internal/wslive"
	"github.com/kang-sw/devenv/internal/wsmailbox"
	"github.com/kang-sw/devenv/internal/wswhiteboard"
)

// The session page's view model: everything the page templates read, built
// by pure functions from what the daemon read off disk, so every rule
// (adapter presence, tree retention, peer merging and liveness, mail line
// expansion) is testable without a server. Nothing here carries a session
// key, and nothing depends on the time of rendering except liveness: times
// are carried as epoch milliseconds and the page formats them, so a region's
// HTML changes only when its content does.

const (
	// endedPeerCap is how many ended peers the peer group lists beside every
	// live one.
	endedPeerCap = 5
	// endedSessionWindow is how long / keeps listing an ended session after
	// it was last seen. Older ended sessions stay reachable at /s/<reply-id>
	// while the registry holds them, but leave the index page.
	endedSessionWindow = 24 * time.Hour
)

// Page modes: a session with a live host adapter gets the full page; one
// without gets the board-first page (board and panels only).
const (
	modeLive  = "live"
	modeBoard = "board"
)

// SessionView is what the session page renders.
type SessionView struct {
	ReplyID string
	// Label is the session's display label (Session.Label), which the head
	// and the tab title show in place of ReplyID; empty when it has none.
	Label string
	// Project is the session root; ProjectName its base name.
	Project, ProjectName string
	Harness              string
	// Live is the served-session liveness rule (Registration.Live).
	Live bool
	// Adapter reports a live host adapter (a fresh snapshot naming one):
	// the page shows the lead bar, the subagent tree, the conversation, the
	// composer, and the layout control. Without one the page is board-first.
	Adapter bool
	// Hosted marks a lead session the daemon spawned and feeds
	// (Registration.Hosted): the page answers its tool approvals and
	// carries its process controls.
	Hosted bool
	// HandedOff marks a lead session the daemon hosted and handed to a
	// terminal, while no terminal runs it: the page reads the command that
	// resumes it there from the session's agent state.
	HandedOff bool
	Version   string
	// Lang is the page language (wsi18n); empty renders the default. LangPref
	// is the language the browser chose in the gear menu, empty when it
	// follows the browser. Both are set per request, never by the builders.
	Lang, LangPref string
	// Theme is the theme the gear menu pinned ("light" or "dark"), empty
	// when the page follows the OS. Set per request, like Lang.
	Theme string

	HasBoard bool
	Seq      int
	// CountPolicy and CountAcks are the board's open items.
	CountPolicy, CountAcks int
	Board, Panels          template.HTML
	Mermaid                bool

	Lead         LeadView
	Tree         TreeView
	Conversation ConversationView
	// Band is the status band around the composer.
	Band BandView
	// Host is what the runner of a hosted session reports beyond the
	// adapter's events (background tasks, a typed failure, broken MCP
	// servers, an API retry); nil for an adapter-fed session.
	Host *wslive.Host
	// Git is the drawer's git handle for Project; nil shows none. Set per
	// request from the daemon's own read (gitbadge.go), never by the
	// builders.
	Git *GitBadge
	// Worktrees reports that the daemon reads worktree pools, which adds
	// the drawer's pooled-worktree card. Set per request, like Git.
	Worktrees bool
	// Holders reports that the daemon runs a session holder manager, which
	// adds the drawer's holders card. Set per request, like Git.
	Holders bool
}

// MailboxRef is what a click on the head's name copies: the session's
// mailbox address, marked as one, naming the session's label when it has
// one (mailboxRef).
func (v SessionView) MailboxRef() string { return mailboxRef(v.ReplyID, v.Label) }

// mailboxRef is the mailbox address of replyID marked as one:
// `(mailbox) id:<rid>`, or `(mailbox, label "<label>") id:<rid>` with label
// quoted, a `"` or `\` in it backslash-escaped. dashboard.js mailboxRef builds
// the same text after a rename, before the head re-renders.
func mailboxRef(replyID, label string) string {
	if label == "" {
		return "(mailbox) " + wsmailbox.ReplyIDPrefix + replyID
	}
	return `(mailbox, label "` + labelQuoter.Replace(label) + `") ` + wsmailbox.ReplyIDPrefix + replyID
}

// labelQuoter escapes a label for mailboxRef's quotes.
var labelQuoter = strings.NewReplacer(`\`, `\\`, `"`, `\"`)

// LabelMax is the longest label the label route accepts, in characters,
// which the head's rename input takes as its limit.
func (SessionView) LabelMax() int { return maxLabelRunes }

// ThemedBoard is Board with its topic-body frames pinned to Theme.
func (v SessionView) ThemedBoard() template.HTML { return wswhiteboard.ThemeFrames(v.Board, v.Theme) }

// Mode is the page mode, modeLive or modeBoard.
func (v SessionView) Mode() string {
	if v.Adapter {
		return modeLive
	}
	return modeBoard
}

// LeadView is the lead's live state from the snapshot.
type LeadView struct {
	wslive.Lead
	// Running, Generating, and Permission are the lead's live row: the
	// running tool call, the message still generating, the permission wait.
	Running    *wslive.RunningView
	Generating *wslive.GeneratingView
	// Finished is the lead's finished batches, oldest first.
	Finished []wslive.BatchView
}

// TreeView is the subagent tree: the lead as its root, the subagents under
// it, and the peer lead group.
type TreeView struct {
	Agents []AgentNode
	// Running, Finished, and Failed count the agents by status.
	Running, Finished, Failed int
	Peers                     PeerGroup
}

// AgentNode is one subagent with its children.
type AgentNode struct {
	wslive.Agent
	// Linked reports that the delegate's reply-id is a session this daemon
	// serves with a board, so its label links to that page.
	Linked     bool
	Running    *wslive.RunningView
	Generating *wslive.GeneratingView
	// Finished is the agent's finished batches, oldest first; the page shows
	// them when the node is expanded.
	Finished []wslive.BatchView
	// Slots is a running agent's live rows, always liveSlotCount of them
	// (liveSlots); nil for a finished or failed agent.
	Slots    []LiveSlot
	Children []AgentNode
}

// liveSlotCount is the fixed number of live rows under a running agent, so
// the panel's height changes only when an agent starts or ends.
const liveSlotCount = 1

// LiveSlot is one live row under a running agent. At most one field is set;
// none set is a placeholder row.
type LiveSlot struct {
	Permission *wslive.Permission
	Finished   *wslive.BatchView
	Running    *wslive.RunningView
	// Generating is a tool call whose arguments are streaming.
	Generating *wslive.GeneratingView
}

// IsRunning reports a running agent.
func (a AgentNode) IsRunning() bool { return a.Status == wslive.StatusRunning }

// ModelText is the agent's model and reasoning effort as one stat ("<model>
// <effort>"), either alone when only one is known; empty when neither is.
func (a AgentNode) ModelText() string { return strings.TrimSpace(a.Model + " " + a.Effort) }

// ContextText is the agent's context size in compact form; empty while
// unknown.
func (a AgentNode) ContextText() string {
	if a.ContextTokens <= 0 {
		return ""
	}
	return compactTokens(a.ContextTokens)
}

// Rate is the agent's output rate in whole tokens per second while it runs;
// 0 while unknown and once it ended.
func (a AgentNode) Rate() int64 {
	if !a.IsRunning() || a.OutputTps <= 0 {
		return 0
	}
	return max(int64(math.Round(a.OutputTps)), 1)
}

// HasStats reports whether the row shows any telemetry stat.
func (a AgentNode) HasStats() bool {
	return a.ModelText() != "" || a.ContextText() != "" || a.Rate() > 0
}

// PeerGroup is the lead's peer leads: every live peer and the most recent
// ended ones, each ordered by last exchange, newest first. Live lists the
// same-worktree peers before the others.
type PeerGroup struct {
	Live  []PeerRow
	Ended []PeerRow
	// HiddenEnded counts the ended peers beyond endedPeerCap.
	HiddenEnded int
}

// Rows is Live followed by Ended.
func (g PeerGroup) Rows() []PeerRow { return append(append([]PeerRow{}, g.Live...), g.Ended...) }

// PeerRow is one peer lead: the sessions this lead exchanged mail with that
// resolve to one reply-id, merged.
type PeerRow struct {
	// Address is the row's label: the most recent named address, else the
	// id: address.
	Address string
	// Path is the folder the peer resides in: a served peer's registration
	// Project, else the newest Contact.Cwd over the merged addresses. Empty
	// when neither is known.
	Path string
	// WorktreeMain is the main repository root when Path is a git linked
	// worktree (worktreeMain), else empty; it names the label prefix.
	WorktreeMain string
	// Branch and Detached are a served peer's branch as its snapshot's repo
	// state holds it (the commit id when Detached); empty for an unserved
	// peer, a peer whose root is no repository, and before its ws-mcp's
	// first read. The daemon never runs git for a peer.
	Branch   string
	Detached bool
	// ReplyID is the peer session; empty for a named address with no owner.
	ReplyID string
	// Label is a served peer's display label (Session.Label), which the row
	// shows in place of an id: Address; empty when it has none or Address is
	// a named address.
	Label string
	// Served reports that this daemon indexes the peer, so the row links to
	// its page and shows its live state.
	Served bool
	Live   bool
	// LastExchange is the latest exchange over the merged addresses (epoch ms).
	LastExchange int64
	// Adapter reports that a served peer has a live host adapter; only then
	// do Phase, Lead, and Agents carry its live state.
	Adapter bool
	Harness string
	// SameWorktree reports a served peer whose registration Project is the
	// viewed session's.
	SameWorktree bool
	// Compact marks a live served peer in another worktree: the page draws
	// it as one line (name, harness, phase mark, last mail) with no
	// expansion, so Agents and AgentCount stay empty and Lead carries the
	// phase alone. RunningAgents is still filled: only the count is read,
	// never the tree.
	Compact bool
	Lead    LeadView
	// Agents is the peer's own subagent tree, one level deep: a peer's peer
	// group never appears. AgentCount counts every agent it holds.
	Agents     []AgentNode
	AgentCount int
	// RunningAgents counts the subagents of a live peer with a live adapter
	// that are running (AgentNode.IsRunning's rule), compact rows included;
	// the row shows a spinner while it is above zero. Readers judge a peer
	// busy by whether it has subagents out.
	RunningAgents int
}

// ConversationView is the conversation stream.
type ConversationView struct {
	// Lang is the language the entries render in (SessionView.Lang).
	Lang    string
	Entries []ConvEntry
	// Reply is the lead's reply text still streaming; nil when none.
	// ReplyHTML is its text rendered as markdown with raw HTML escaped.
	Reply     *wslive.Reply
	ReplyHTML template.HTML
	// Phase and Permission drive the trailing activity or approval line.
	Phase      string
	Permission *wslive.Permission
	Running    *wslive.RunningView
	Generating *wslive.GeneratingView
	// ToolGap is the lead's finished tool calls since its last conversation
	// entry, shown as a live tools row until the ring takes it.
	ToolGap *wslive.ToolSummary
	// Users and Mails count the user messages and mail lines.
	Users, Mails int
	// Subagents counts the running subagents for the composer's badge.
	Subagents SubagentCount
	// Held lists the messages sent from the page that the session has not
	// received yet, oldest first; Sent is each board item's latest sent
	// answer, which the page marks on the item's card.
	Held []HeldView
	Sent []SentMark
	// markdown renders a finished lead reply (replyRender.finished); an
	// entry runs it only when the entry cache misses (renderEntries).
	markdown func(string) template.HTML
}

// SubagentCount is the running subagents of a session: Direct are its own
// running children, Deep every running agent below them, including the
// running agents of a running delegate that holds its own ws session.
type SubagentCount struct {
	Direct, Deep int
}

// subagentCounts counts the running agents of self's snapshot agents by
// depth: one whose parent is in the list is Deep, any other Direct (so the
// running child of an evicted parent counts as Direct; the sum stays right,
// since retention keeps every running agent). A running agent whose
// ReplyID lookup resolves to a session with a live adapter adds that
// session's running agents, at every depth and through its own delegates,
// to Deep; a session already counted (self included) is not counted again.
func subagentCounts(self string, agents []wslive.Agent, lookup sessionLookup, now time.Time) SubagentCount {
	var c SubagentCount
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
		if a.ParentID != "" && a.ParentID != a.ID && known[a.ParentID] {
			c.Deep++
		} else {
			c.Direct++
		}
	}
	linked(agents)
	return c
}

// ConvEntry is one conversation ring entry as the page shows it.
type ConvEntry struct {
	wslive.RingEntry
	// Key is the ring entry's file name, the element key the page keeps an
	// expanded mail line open by.
	Key string
	// At is the entry's time (epoch ms).
	At int64
	// HTML is a lead reply's text rendered as markdown with its inline HTML
	// sanitized; user messages, mail bodies, and turn lines stay plain text,
	// raw HTML included. It is set only while the entry renders on an entry
	// cache miss.
	HTML template.HTML
	// Images are a user message's attached images, taken out of its text.
	Images []AttachmentView
	// Answers are the lines of the answer block a user message starts with,
	// taken out of its text; the bubble shows them as capsules.
	Answers []wsinbox.AnswerLine
	// Mentions are the mention elements after that block (or opening the
	// message), taken out of its text and shown as capsules after the
	// answer capsules.
	Mentions []wsinbox.Mention
	// Rendered is the entry's whole element, from the entry cache.
	Rendered template.HTML
	// Body is a mail line's full message from the mailbox history; HasBody
	// is false once the message has left that ring, and the line then shows
	// only its first line.
	Body    string
	HasBody bool
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
	// Lookup resolves the other sessions this daemon serves that the view
	// names.
	Lookup sessionLookup
}

// sessionLookup resolves the sessions a page view names: session reads the
// index alone, and snapshot reads a session's live snapshot (nil when it has
// none), so a caller that uses only whether a session is served, or whether
// it has a board, reads no snapshot. The zero value serves none.
type sessionLookup struct {
	session  func(replyID string) (Session, bool)
	snapshot func(replyID string) *wslive.Snapshot
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

// adapterPresent is wslive.AdapterPresent on an already read snapshot.
func adapterPresent(snap *wslive.Snapshot, now time.Time) bool {
	return snap != nil && snap.Adapter != "" && !snap.Stale(now)
}

func buildSessionView(in pageInputs) SessionView {
	reg := in.Session.Registration
	v := SessionView{
		ReplyID:     reg.ReplyID,
		Label:       in.Session.Label,
		Project:     reg.Project,
		ProjectName: filepath.Base(reg.Project),
		Harness:     reg.Harness,
		Live:        reg.Live(in.Now),
		Adapter:     adapterPresent(in.Snapshot, in.Now),
		Hosted:      reg.Hosted,
		HandedOff:   in.HandedOff && !reg.Hosted && !reg.Live(in.Now),
		Version:     in.Version,
		HasBoard:    in.Page.HasBoard,
		Seq:         in.Page.Seq,
		CountPolicy: in.Page.Board.CountPolicy,
		CountAcks:   in.Page.Board.CountAcks,
		Board:       in.Page.Board.Board,
		Panels:      in.Page.Board.Panels,
		Mermaid:     in.Page.Board.Mermaid,
	}
	if !v.Adapter {
		return v
	}
	lookup := in.Lookup
	v.Lead = leadView(in.Snapshot.Lead)
	v.Band = bandView(in.Snapshot)
	v.Host = in.Snapshot.Host
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
	v.Conversation = conversation(in.Snapshot.Lead, in.Ring, in.Page.MailBodies, contacts, lookup, replyMarkdown(reg.Project, reg.ReplyID))
	splitUserEntries(v.Conversation.Entries, in.AttachDir, reg.ReplyID)
	v.Conversation.Held = heldViews(in.Held, in.AttachDir, reg.ReplyID)
	v.Conversation.Sent = sentMarks(in.Drained, in.Held, in.Page.Reasked)
	v.Conversation.Subagents = subagentCounts(reg.ReplyID, in.Snapshot.Agents, lookup, in.Now)
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
				if s, ok := lookup.get(a.ReplyID); ok && s.HasBoard {
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

// peerGroup merges contacts that resolve to one reply-id into one row and
// applies the retention rule: every live peer and the last endedPeerCap
// ended ones. A served peer's liveness is its own registration; an unserved
// one is live while its presence is (Contact.PresenceLive). project is the
// viewed session's root: only a served peer registered under the same path
// (compared after filepath.Clean, no symlink resolution) keeps its full live
// state; a live served peer elsewhere is a Compact row and its subagent tree
// is never read (only its running count). Live peers list same-worktree first.
func peerGroup(contacts []Contact, project string, lookup sessionLookup, now time.Time) PeerGroup {
	type merged struct {
		row          PeerRow
		named        bool
		presenceLive bool
	}
	var order []string
	rows := map[string]*merged{}
	for _, c := range contacts { // newest first
		key := c.ReplyID
		if key == "" {
			key = "addr:" + c.Address
		}
		m, ok := rows[key]
		if !ok {
			m = &merged{row: PeerRow{Address: c.Address, ReplyID: c.ReplyID}}
			rows[key] = m
			order = append(order, key)
		}
		isNamed := !hasReplyIDPrefix(c.Address)
		if isNamed && !m.named {
			// The most recent named address labels the row.
			m.row.Address, m.named = c.Address, true
		}
		if at := c.LastExchange.UnixMilli(); at > m.row.LastExchange {
			m.row.LastExchange = at
		}
		m.presenceLive = m.presenceLive || c.PresenceLive
		if m.row.Path == "" {
			m.row.Path = c.Cwd
		}
	}
	var group PeerGroup
	for _, key := range order {
		m := rows[key]
		row := m.row
		if row.ReplyID != "" {
			if s, ok := lookup.get(row.ReplyID); ok {
				row.Served = true
				row.Live = s.Live
				if !m.named {
					row.Label = s.Label
				}
				row.Harness = s.Harness
				if s.Project != "" {
					row.Path = s.Project
				}
				row.SameWorktree = s.Project != "" && filepath.Clean(s.Project) == filepath.Clean(project)
				row.Compact = s.Live && !row.SameWorktree
				snap := lookup.snap(row.ReplyID)
				if snap != nil && snap.Repo != nil {
					row.Branch, row.Detached = snap.Repo.Branch, snap.Repo.Detached
				}
				if adapterPresent(snap, now) {
					row.Adapter = true
					if row.Live {
						for _, a := range snap.Agents {
							if (AgentNode{Agent: a}).IsRunning() {
								row.RunningAgents++
							}
						}
					}
					if row.Compact {
						row.Lead = LeadView{Lead: snap.Lead}
					} else {
						row.Lead = leadView(snap.Lead)
						// One level deep: the peer's own agents, never its peers.
						row.Agents = agentTree(snap.Agents, lookup)
						row.AgentCount = len(snap.Agents)
					}
				}
			}
		}
		if !row.Served {
			row.Live = m.presenceLive
		}
		row.WorktreeMain = worktreeMain(row.Path)
		if row.Live {
			group.Live = append(group.Live, row)
		} else {
			group.Ended = append(group.Ended, row)
		}
	}
	byLast := func(list []PeerRow) {
		sort.SliceStable(list, func(i, j int) bool { return list[i].LastExchange > list[j].LastExchange })
	}
	byLast(group.Live)
	byLast(group.Ended)
	sort.SliceStable(group.Live, func(i, j int) bool { return group.Live[i].SameWorktree && !group.Live[j].SameWorktree })
	if len(group.Ended) > endedPeerCap {
		group.HiddenEnded = len(group.Ended) - endedPeerCap
		group.Ended = group.Ended[:endedPeerCap]
	}
	return group
}

// Name is the row's name: Label when set, else Address.
func (r PeerRow) Name() string {
	if r.Label != "" {
		return r.Label
	}
	return r.Address
}

// Title is the row name's tooltip: the id: Address when Label stands in for
// it, then Path, one per line; empty when neither applies.
func (r PeerRow) Title() string {
	var lines []string
	if r.Label != "" {
		lines = append(lines, r.Address)
	}
	if r.Path != "" {
		lines = append(lines, r.Path)
	}
	return strings.Join(lines, "\n")
}

// BranchHandle is the row's branch chip label, as the session page's git
// handle draws a branch (branchHandle); the zero label when Branch is empty.
func (r PeerRow) BranchHandle() GitHandle {
	if r.Branch == "" {
		return GitHandle{}
	}
	return branchHandle(r.Branch, r.Detached)
}

// Folder is the row's label prefix: Path's last element, as
// "<main>(<folder>)" when Path is a linked worktree of the repository at
// WorktreeMain (peerFolderLabel). Empty when Path is.
func (r PeerRow) Folder() string { return peerFolderLabel(r.Path, r.WorktreeMain) }

// peerFolderLabel is "<main>(<folder>)" when main, a worktree's main
// repository root, is known, else path's last element (lastElem).
func peerFolderLabel(path, main string) string {
	folder := lastElem(path)
	if m := lastElem(main); m != "" && folder != "" {
		return m + "(" + folder + ")"
	}
	return folder
}

// lastElem is p's last element, split on "/" and "\" alike (a Windows cwd
// can reach a daemon on another OS), trailing separators ignored.
func lastElem(p string) string {
	p = strings.TrimRight(p, `/\`)
	return p[strings.LastIndexAny(p, `/\`)+1:]
}

func hasReplyIDPrefix(address string) bool {
	return len(address) >= len(wsmailbox.ReplyIDPrefix) && address[:len(wsmailbox.ReplyIDPrefix)] == wsmailbox.ReplyIDPrefix
}

// replyRender renders lead replies as markdown: finished a reply entry,
// streaming the reply still streaming.
type replyRender struct {
	finished, streaming func(string) template.HTML
}

// replyMarkdown renders lead replies under the served page's link rule
// (servedTargets): a file target is an action link for the page's link
// menu, and an image inside root is the image route's URL. A finished
// reply's inline HTML is kept through the reply sanitizer under the same
// rule (wswhiteboard.TextTargets.HTML) and its mermaid fences are drawn by
// the page; the streaming reply shows raw HTML as text and keeps mermaid
// fences code blocks, since it re-renders every tick and a half-closed tag
// would shake the layout and a diagram would redraw with it.
func replyMarkdown(root, replyID string) replyRender {
	return replyRenderUnder(servedTargets(root, replyID))
}

// replyRenderUnder is replyMarkdown under the link rule targets: the
// outside-link route renders the same replies with a recorder on the rule
// (outsideReplyLinks), so both resolve a target the same way.
func replyRenderUnder(targets wswhiteboard.TextTargets) replyRender {
	streaming, finished := targets, targets
	finished.Mermaid = true
	finished.HTML = true
	return replyRender{
		finished:  func(s string) template.HTML { return wswhiteboard.RenderText(s, finished) },
		streaming: func(s string) template.HTML { return wswhiteboard.RenderText(s, streaming) },
	}
}

// conversation turns the ring into the page's stream: each mail line gets
// its body while the mailbox history still holds it and a link to its peer's
// page when this daemon serves the peer. A lead reply's markdown is left to
// the entry render (renderEntries), which caches it.
func conversation(lead wslive.Lead, ring []wslive.RingRecord, bodies map[string]string, contacts []Contact, lookup sessionLookup, render replyRender) ConversationView {
	byAddress := map[string]string{}
	for _, c := range contacts {
		if c.ReplyID != "" {
			byAddress[c.Address] = c.ReplyID
		}
	}
	cv := ConversationView{Reply: lead.Reply, Phase: lead.Phase, Permission: lead.Permission, markdown: render.finished}
	if lead.Reply != nil {
		cv.ReplyHTML = render.streaming(lead.Reply.Text)
	}
	_, cv.Running, cv.Generating = splitBatches(lead.Batches)
	cv.ToolGap = lead.ToolGap
	for _, r := range ring {
		e := ConvEntry{RingEntry: r.RingEntry, Key: r.Name, At: r.At.UnixMilli()}
		switch r.Kind {
		case wslive.RingUser:
			cv.Users++
		case wslive.RingMail:
			cv.Mails++
			if r.HistoryKey != "" {
				e.Body, e.HasBody = bodies[r.HistoryKey]
			}
			rid := byAddress[r.Peer]
			if rid == "" && hasReplyIDPrefix(r.Peer) {
				rid = r.Peer[len(wsmailbox.ReplyIDPrefix):]
			}
			if rid != "" {
				if s, ok := lookup.get(rid); ok {
					e.PeerReplyID, e.PeerLinked = rid, true
					if hasReplyIDPrefix(r.Peer) {
						e.PeerLabel = s.Label
					}
				}
			}
		}
		cv.Entries = append(cv.Entries, e)
	}
	return cv
}

// IndexView is what / renders.
type IndexView struct {
	// Lang, LangPref, and Theme are SessionView's.
	Lang, LangPref, Theme string
	Version               string
	Projects              []ProjectGroup
	// Live and Busy count the live sessions and those not idle.
	Live, Busy int
}

// ProjectGroup is one project root's sessions on /.
type ProjectGroup struct {
	Name, Path string
	// Sessions are the live sessions; Ended the ended ones still inside
	// endedSessionWindow; HiddenEnded counts the older ended ones.
	Sessions    []IndexRow
	Ended       []IndexRow
	HiddenEnded int
}

// IndexRow is one session on /.
type IndexRow struct {
	ReplyID string
	// Label is the session's display label, shown in place of ReplyID;
	// empty when it has none.
	Label   string
	Harness string
	Live    bool
	// Adapter, Phase, and Tool come from a live snapshot; Phase is empty
	// without an adapter.
	Adapter       bool
	Phase         string
	Tool          string
	RunningAgents int
	HasBoard      bool
	CountPolicy   int
	// LastSeen is the registration's latest stamp (epoch ms).
	LastSeen int64
	// Hosted and Approval are the rail row's (RailRow).
	Hosted   bool
	Approval *ApprovalWait
}

// indexInput is one indexed session with what / shows of it.
type indexInput struct {
	Session  Session
	Snapshot *wslive.Snapshot
}

// buildIndexView groups sessions by project. A live session is listed; an
// ended one only while it was last seen within endedSessionWindow.
func buildIndexView(sessions []indexInput, now time.Time, version string) IndexView {
	view := IndexView{Version: version}
	byProject := map[string]*ProjectGroup{}
	var order []string
	for _, in := range sessions {
		reg := in.Session.Registration
		group, ok := byProject[reg.Project]
		if !ok {
			group = &ProjectGroup{Name: filepath.Base(reg.Project), Path: reg.Project}
			byProject[reg.Project] = group
			order = append(order, reg.Project)
		}
		row := IndexRow{
			ReplyID:     reg.ReplyID,
			Label:       in.Session.Label,
			Harness:     reg.Harness,
			Live:        reg.Live(now),
			HasBoard:    in.Session.HasBoard,
			CountPolicy: in.Session.CountPolicy,
			LastSeen:    reg.LastSeen().UnixMilli(),
			Hosted:      reg.Hosted,
		}
		if adapterPresent(in.Snapshot, now) {
			row.Adapter = true
			if row.Live {
				row.Approval = approvalWait(reg, in.Snapshot)
			}
			row.Phase = in.Snapshot.Lead.Phase
			if row.Phase == "" {
				row.Phase = wslive.PhaseIdle
			}
			if _, running, generating := splitBatches(in.Snapshot.Lead.Batches); running != nil {
				row.Tool = running.ToolName
			} else if generating != nil {
				row.Tool = generating.ToolName
			}
			if p := in.Snapshot.Lead.Permission; p != nil {
				row.Tool = p.ToolName
			}
			for _, a := range in.Snapshot.Agents {
				if a.Status == wslive.StatusRunning {
					row.RunningAgents++
				}
			}
		}
		switch {
		case row.Live:
			group.Sessions = append(group.Sessions, row)
			view.Live++
			if row.Adapter && row.Phase != wslive.PhaseIdle {
				view.Busy++
			}
		case now.Sub(reg.LastSeen()) <= endedSessionWindow:
			group.Ended = append(group.Ended, row)
		default:
			group.HiddenEnded++
		}
	}
	sort.Strings(order)
	for _, path := range order {
		group := byProject[path]
		newest := func(list []IndexRow) {
			sort.SliceStable(list, func(i, j int) bool { return list[i].LastSeen > list[j].LastSeen })
		}
		newest(group.Sessions)
		newest(group.Ended)
		if len(group.Sessions) == 0 && len(group.Ended) == 0 {
			continue // only long-ended sessions: the project leaves /
		}
		view.Projects = append(view.Projects, *group)
	}
	return view
}
