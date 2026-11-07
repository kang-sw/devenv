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
	// Project is the session root; ProjectName its base name.
	Project, ProjectName string
	Harness              string
	// Live is the served-session liveness rule (Registration.Live).
	Live bool
	// Adapter reports a live host adapter (a fresh snapshot naming one):
	// the page shows the lead bar, the subagent tree, the conversation, the
	// composer, and the layout control. Without one the page is board-first.
	Adapter bool
	Version string

	HasBoard bool
	Seq      int
	// CountPolicy, CountAssumed, and CountAcks are the board's open items.
	CountPolicy, CountAssumed, CountAcks int
	Board, Panels                        template.HTML
	Mermaid                              bool

	Lead         LeadView
	Tree         TreeView
	Conversation ConversationView
}

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
	Children []AgentNode
}

// IsRunning reports a running agent.
func (a AgentNode) IsRunning() bool { return a.Status == wslive.StatusRunning }

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
	// ReplyID is the peer session; empty for a named address with no owner.
	ReplyID string
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
	// phase alone.
	Compact bool
	Lead    LeadView
	// Agents is the peer's own subagent tree, one level deep: a peer's peer
	// group never appears. AgentCount counts every agent it holds.
	Agents     []AgentNode
	AgentCount int
}

// ConversationView is the conversation stream.
type ConversationView struct {
	Entries []ConvEntry
	// Reply is the lead's reply text still streaming; nil when none.
	// ReplyHTML is its text rendered as markdown.
	Reply     *wslive.Reply
	ReplyHTML template.HTML
	// Phase and Permission drive the trailing activity or approval line.
	Phase      string
	Permission *wslive.Permission
	Running    *wslive.RunningView
	Generating *wslive.GeneratingView
	// Users and Mails count the user messages and mail lines.
	Users, Mails int
	// Held lists the messages sent from the page that the session has not
	// received yet, oldest first; Sent is each board item's latest sent
	// answer, which the page marks on the item's card.
	Held []HeldView
	Sent []SentMark
	// markdown renders a lead reply; an entry runs it only when the entry
	// cache misses (renderEntries).
	markdown func(string) template.HTML
}

// ConvEntry is one conversation ring entry as the page shows it.
type ConvEntry struct {
	wslive.RingEntry
	// Key is the ring entry's file name, the element key the page keeps an
	// expanded mail line open by.
	Key string
	// At is the entry's time (epoch ms).
	At int64
	// HTML is a lead reply's text rendered as markdown; user messages, mail
	// bodies, and turn lines stay plain text. It is set only while the entry
	// renders on an entry cache miss.
	HTML template.HTML
	// Images are a user message's attached images, taken out of its text.
	Images []AttachmentView
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
	// Lookup returns a session this daemon serves and its snapshot (nil when
	// it has none).
	Lookup func(replyID string) (Session, *wslive.Snapshot, bool)
}

// adapterPresent is wslive.AdapterPresent on an already read snapshot.
func adapterPresent(snap *wslive.Snapshot, now time.Time) bool {
	return snap != nil && snap.Adapter != "" && !snap.Stale(now)
}

func buildSessionView(in pageInputs) SessionView {
	reg := in.Session.Registration
	v := SessionView{
		ReplyID:      reg.ReplyID,
		Project:      reg.Project,
		ProjectName:  filepath.Base(reg.Project),
		Harness:      reg.Harness,
		Live:         reg.Live(in.Now),
		Adapter:      adapterPresent(in.Snapshot, in.Now),
		Version:      in.Version,
		HasBoard:     in.Page.HasBoard,
		Seq:          in.Page.Seq,
		CountPolicy:  in.Page.Board.CountPolicy,
		CountAssumed: in.Page.Board.CountAssumed,
		CountAcks:    in.Page.Board.CountAcks,
		Board:        in.Page.Board.Board,
		Panels:       in.Page.Board.Panels,
		Mermaid:      in.Page.Board.Mermaid,
	}
	if !v.Adapter {
		return v
	}
	lookup := in.Lookup
	if lookup == nil {
		lookup = func(string) (Session, *wslive.Snapshot, bool) { return Session{}, nil, false }
	}
	v.Lead = leadView(in.Snapshot.Lead)
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
	userEntryImages(v.Conversation.Entries, in.AttachDir, reg.ReplyID)
	v.Conversation.Held = heldViews(in.Held, in.AttachDir, reg.ReplyID)
	v.Conversation.Sent = sentMarks(in.Drained, in.Held)
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

// agentTree nests the snapshot's flat agent list by parent id, keeping the
// snapshot's order among siblings. An agent whose parent is not in the list
// hangs from the root, so retention never hides a running grandchild.
func agentTree(agents []wslive.Agent, lookup func(string) (Session, *wslive.Snapshot, bool)) []AgentNode {
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
			if a.ReplyID != "" {
				if s, _, ok := lookup(a.ReplyID); ok && s.HasBoard {
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
// is never read. Live peers list same-worktree first.
func peerGroup(contacts []Contact, project string, lookup func(string) (Session, *wslive.Snapshot, bool), now time.Time) PeerGroup {
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
	}
	var group PeerGroup
	for _, key := range order {
		m := rows[key]
		row := m.row
		if row.ReplyID != "" {
			if s, snap, ok := lookup(row.ReplyID); ok {
				row.Served = true
				row.Live = s.Live
				row.Harness = s.Harness
				row.SameWorktree = s.Project != "" && filepath.Clean(s.Project) == filepath.Clean(project)
				row.Compact = s.Live && !row.SameWorktree
				if adapterPresent(snap, now) {
					row.Adapter = true
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

func hasReplyIDPrefix(address string) bool {
	return len(address) >= len(wsmailbox.ReplyIDPrefix) && address[:len(wsmailbox.ReplyIDPrefix)] == wsmailbox.ReplyIDPrefix
}

// replyMarkdown renders a lead reply as markdown under the served page's
// link rule, the board's: a file link or a file image outside root is path
// text, and an image inside root is the image route's URL.
func replyMarkdown(root, replyID string) func(string) template.HTML {
	targets := wswhiteboard.TextTargets{
		FileText: func(path string) string {
			if rel, ok := relativeInside(root, path); ok {
				return filepath.ToSlash(rel)
			}
			return filepath.ToSlash(path)
		},
		ImageSrc: func(path string) string { return ImageSrc(root, replyID, path) },
	}
	return func(s string) template.HTML { return wswhiteboard.RenderText(s, targets) }
}

// conversation turns the ring into the page's stream: each mail line gets
// its body while the mailbox history still holds it and a link to its peer's
// page when this daemon serves the peer. A lead reply's markdown is left to
// the entry render (renderEntries), which caches it.
func conversation(lead wslive.Lead, ring []wslive.RingRecord, bodies map[string]string, contacts []Contact, lookup func(string) (Session, *wslive.Snapshot, bool), render func(string) template.HTML) ConversationView {
	byAddress := map[string]string{}
	for _, c := range contacts {
		if c.ReplyID != "" {
			byAddress[c.Address] = c.ReplyID
		}
	}
	cv := ConversationView{Reply: lead.Reply, Phase: lead.Phase, Permission: lead.Permission, markdown: render}
	if lead.Reply != nil {
		cv.ReplyHTML = render(lead.Reply.Text)
	}
	_, cv.Running, cv.Generating = splitBatches(lead.Batches)
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
				if _, _, ok := lookup(rid); ok {
					e.PeerReplyID, e.PeerLinked = rid, true
				}
			}
		}
		cv.Entries = append(cv.Entries, e)
	}
	return cv
}

// IndexView is what / renders.
type IndexView struct {
	Version  string
	Projects []ProjectGroup
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
	CountAssumed  int
	// LastSeen is the registration's latest stamp (epoch ms).
	LastSeen int64
}

// indexInput is one indexed session with what / shows of it.
type indexInput struct {
	Session  Session
	Snapshot *wslive.Snapshot
	Page     PageContent
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
			ReplyID:      reg.ReplyID,
			Harness:      reg.Harness,
			Live:         reg.Live(now),
			HasBoard:     in.Page.HasBoard,
			CountPolicy:  in.Page.Board.CountPolicy,
			CountAssumed: in.Page.Board.CountAssumed,
			LastSeen:     reg.LastSeen().UnixMilli(),
		}
		if adapterPresent(in.Snapshot, now) {
			row.Adapter = true
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
