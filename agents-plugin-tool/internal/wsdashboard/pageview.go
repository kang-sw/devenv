package wsdashboard

import (
	"html/template"
	"math"
	"path/filepath"
	"runtime"
	"strings"
	"time"

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

// Page modes: a fresh host adapter gets the full page; a lead with saved
// entries gets read-only history without one; an empty board stays board-first.
const (
	modeLive    = "live"
	modeBoard   = "board"
	modeHistory = "history"
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
	// ProcessRunning is the harness process state shown in the header.
	ProcessRunning bool
	// Adapter reports a live host adapter (a fresh snapshot naming one):
	// the page shows live activity, the subagent tree, the composer, and
	// the layout control. Saved history does not require an adapter.
	Adapter bool
	// SavedConversation exposes persisted lead history without live controls.
	SavedConversation bool
	// Hosted marks a lead session the daemon spawned and feeds
	// (Registration.Hosted): the page answers its tool approvals and
	// carries its process controls.
	Hosted bool
	// HandedOff marks a lead session the daemon hosted and handed to a
	// terminal, while no terminal runs it: the page reads the command that
	// resumes it there from the session's agent state.
	HandedOff bool
	// IsChild marks a spawned worker or card-bound subordinate session whose
	// composer controls render locked read-only.
	IsChild                                   bool
	ChildCloseKnown, ChildClosed, ChildActive bool
	// CanEditSettings reports whether the viewer may edit model/effort from
	// the composer footer (hosted non-child session viewed from the host).
	CanEditSettings bool
	// Model and Effort are the session's resolved display values for the
	// composer footer strip.
	Model, Effort string
	Version       string
	// Lang is the page language (wsi18n); empty renders the default. LangPref
	// is the language the browser chose in the gear menu, empty when it
	// follows the browser. Both are set per request, never by the builders.
	Lang, LangPref string
	// Theme is the theme the gear menu pinned ("light" or "dark"), empty
	// when the page follows the OS. Set per request, like Lang.
	Theme string
	// ActivityAt is the snapshot time used for stable elapsed fallback text.
	ActivityAt int64

	HasBoard         bool
	Seq              int
	BoardIncarnation string
	// CountPolicy and CountAcks are the board's open items: policy and
	// check items, then technical and announced ones.
	CountPolicy, CountAcks int
	Board, Panels          template.HTML
	BoardUnits             *BoardUnits
	Mermaid                bool

	// Parent is the session that owns this one (the head's parent jump);
	// nil for a session nobody owns.
	Parent *ParentView

	Lead         LeadView
	Tree         TreeView
	Conversation ConversationView
	// ModelHistory is retained response metadata, independent of live activity.
	ModelHistory []ModelHistoryRow
	// Band is the status band around the composer.
	Band BandView
	// Commands are the slash commands the session's adapter runs from a
	// composer command entry (the snapshot's list); none offers no picker.
	Commands []wslive.SlashCommand
	// Host is what the runner of a hosted session reports beyond the
	// adapter's events (background tasks, a typed failure, broken MCP
	// servers, an API retry); nil for an adapter-fed session.
	Host *wslive.Host
	// Background and Monitors are the separately reported badge counts.
	Background, Monitors int
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

// MailboxOpen is the copy tag's opening attributes, also used after a rename.
func (v SessionView) MailboxOpen() string {
	var root, branch string
	if v.Git != nil {
		root = v.Git.TopLevel
		if !v.Git.Detached {
			branch = v.Git.Branch
		}
	}
	return mailboxOpen(v.ReplyID, v.Project, root, branch, v.Harness)
}

// MailboxRef is the tag copied by the head's name.
func (v SessionView) MailboxRef() string { return mailboxRef(v.MailboxOpen(), v.Label) }

// mailboxOpen is a session's copy tag up to its closing: the head's name and
// a rail row's copy button both build it here, so the two copy one string.
// root is the worktree's top level, written only when it is not project;
// branch is empty for a detached HEAD or no repository.
func mailboxOpen(replyID, project, root, branch, harness string) string {
	attr := func(s string) string { return mailboxAttrEscaper.Replace(s) }
	open := `<mailbox id="` + attr(wsmailbox.ReplyIDPrefix+replyID) + `" dir="` + attr(project) + `"`
	a, b := filepath.Clean(root), filepath.Clean(project)
	if root != "" && !(a == b || runtime.GOOS == "windows" && strings.EqualFold(a, b)) {
		open += ` root="` + attr(root) + `"`
	}
	if branch != "" {
		open += ` branch="` + attr(branch) + `"`
	}
	if harness != "" {
		open += ` harness="` + attr(harness) + `"`
	}
	return open
}

// mailboxRef closes a copy tag around label, or self-closes it without one.
func mailboxRef(open, label string) string {
	if label == "" {
		return open + `/>`
	}
	return open + `>` + mailboxTextEscaper.Replace(label) + `</mailbox>`
}

var mailboxAttrEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;")
var mailboxTextEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")

// LabelMax is the longest label the label route accepts, in characters,
// which the head's rename input takes as its limit.
func (SessionView) LabelMax() int { return maxLabelRunes }

// ThemedBoard is Board with its topic-body frames pinned to Theme.
func (v SessionView) ThemedBoard() template.HTML { return wswhiteboard.ThemeFrames(v.Board, v.Theme) }

// Mode is the live, saved-history, or board-only page layout.
func (v SessionView) Mode() string {
	if v.Adapter {
		return modeLive
	}
	if v.SavedConversation {
		return modeHistory
	}
	return modeBoard
}

type ModelHistoryRow struct {
	ID, Label string
	Observed  *wslive.ObservedModel
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
	CompactAgents             []AgentNode
	Subagents                 SubagentCount
	HUD                       HUDView
}

// RunningAgents and EndedAgents split the top-level subagents into the
// agents popover's groups, each in tree order; a child stays under its
// parent whatever its own status.
func (t TreeView) RunningAgents() []AgentNode { return t.agentsWhere(true) }
func (t TreeView) EndedAgents() []AgentNode   { return t.agentsWhere(false) }

// Tasks is the background tasks and monitors together, the agents popover's
// background group count.
func (v SessionView) Tasks() int { return v.Background + v.Monitors }

func (t TreeView) agentsWhere(running bool) []AgentNode {
	var out []AgentNode
	for _, a := range t.Agents {
		if a.IsRunning() == running {
			out = append(out, a)
		}
	}
	return out
}

// AgentNode is one subagent with its children.
type AgentNode struct {
	wslive.Agent
	// Linked reports that the delegate's reply-id is a session this daemon
	// serves with a page (a board, or an agent runner's conversation), so
	// its label links to that page.
	Linked bool
	// PiDeleted is display-only; the live snapshot keeps its legacy status.
	PiDeleted  bool
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

// EndedStatus is the localized status key suffix for an ended row. A row with
// running background work shows "background"; Pi children otherwise sleep
// between turns; native Claude/Codex rows finish done.
func (a AgentNode) EndedStatus() string {
	if a.Status == wslive.StatusDone && a.BackgroundTasks > 0 {
		return "background"
	}
	if a.AgentKind == wslive.AgentKindPi && a.Status == wslive.StatusDone {
		if a.PiDeleted {
			return "deleted"
		}
		return "sleep"
	}
	if a.Status == wslive.StatusFailed {
		return "failed"
	}
	return "done"
}

// ModelText is the agent's model and reasoning effort joined by a space
// ("<model> <effort>"), either alone when only one is known; empty when
// neither is. The row renders them as separate stats (template astat).
func (a AgentNode) ModelText() string { return strings.TrimSpace(a.DisplayModel() + " " + a.Effort) }

// DisplayModel prefers this agent's response evidence. Legacy adapter telemetry
// remains separately labelled in the hover, and never establishes observation.
func (a AgentNode) DisplayModel() string {
	if a.Observed != nil {
		return a.Observed.Current.Model
	}
	return a.Model
}

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

// ActiveSince is the shifted ticker base for the current working segment.
// A running row without a turn start can be held but idle; it must not tick.
func (a AgentNode) ActiveSince() int64 {
	if !a.IsRunning() || a.TurnStartedAt <= 0 {
		return 0
	}
	return a.TurnStartedAt - max(a.ActiveMs, 0)
}

// HasStats reports whether the row shows telemetry or a finished timestamp.
func (a AgentNode) HasStats() bool {
	return a.ModelText() != "" || a.ContextText() != "" || a.Rate() > 0 || (!a.IsRunning() && a.EndedAt > 0)
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
	Summary   *SummaryView
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
