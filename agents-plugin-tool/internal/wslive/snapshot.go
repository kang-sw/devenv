// Package wslive holds a session's live dashboard state on disk: the live
// snapshot (the current activity of the lead and its subagents, overwritten
// in place) and the conversation ring (recent finished conversation
// messages, one file per entry). ws-mcp writes both from host adapter events
// and from its own mailbox path; the dashboard daemon reads them. This
// package owns the shared schema, the pure reducer from adapter events to a
// snapshot, the ring files, and the readers, so the writer and the reader
// cannot drift apart.
//
// Both stores live in the session's dashboard registry directory (the
// caller passes it; see wsdashboard.SessionDir), beside its registration:
//
//	<session dir>/snapshot.json
//	<session dir>/ring/<stamp>-<hex>.json
//
// A session is keyed by its reply-id, never by its session key: a session
// key is tool authority, and everything here reaches a browser.
//
// Version tolerance: readers ignore unknown fields and treat absent ones as
// empty, since a newer daemon reads what older ws-mcp processes still write.
package wslive

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

const (
	// SchemaVersion is the snapshot's schemaVersion. Readers accept any
	// value: fields are added, never repurposed.
	SchemaVersion = 1

	// SnapshotName is the snapshot file inside a session directory.
	SnapshotName = "snapshot.json"
	// RingDirName is the conversation ring folder inside a session
	// directory.
	RingDirName = "ring"

	// BatchCap, ToolNameCap, and ArgsPreviewCap are the batch model's caps:
	// batches kept per agent, distinct tool names per batch, and characters
	// of the running call's argument preview.
	BatchCap       = 5
	ToolNameCap    = 5
	ArgsPreviewCap = 200
	// ToolNameRunes bounds one tool name.
	ToolNameRunes = 64
	// FinishedAgentCap is how many finished agents the tree keeps beside
	// every running one.
	FinishedAgentCap = 5
	// RunningAgentCap is a defensive bound on running agents, so an adapter
	// that never reports an agent's end cannot grow the snapshot without
	// limit; past it the least recently updated running agent is dropped.
	RunningAgentCap = 64
	// CaptionRunes and LabelRunes bound an agent's caption and label.
	CaptionRunes = 200
	LabelRunes   = 120
	// TextBytes bounds a streamed reply in the snapshot and a user message or
	// reply in the ring; a longer text keeps its head and is marked truncated.
	TextBytes = 64 << 10
	// RingCap is the conversation's live view: the newest ring entries a
	// page draws. Older entries stay on disk for paging (ReadRingPage).
	RingCap = 50
	// RingHistoryCap bounds the ring on disk: past it the oldest entries are
	// trimmed.
	RingHistoryCap = 2000

	// HeartbeatInterval is how often a ws-mcp rewrites a session's snapshot
	// while nothing changes, so a reader can tell an idle session from a
	// dead one. StaleAfterBeats heartbeats without a write make a snapshot
	// stale.
	HeartbeatInterval = 30 * time.Second
	StaleAfterBeats   = 3
)

// Phase values of an agent's or the lead's current activity.
const (
	PhaseIdle       = "idle"
	PhaseThinking   = "thinking"
	PhaseReplying   = "replying"
	PhaseTool       = "tool"
	PhasePermission = "permission"
	// PhaseCompacting is the lead's alone: a main-conversation compaction
	// runs.
	PhaseCompacting = "compacting"
)

// Agent status values.
const (
	StatusRunning = "running"
	StatusDone    = "done"
	StatusFailed  = "failed"
)

// Origin values: where a user message came from, or what started a turn.
const (
	OriginTerminal   = "terminal"
	OriginWhiteboard = "whiteboard"
	OriginMail       = "mail"
)

// SnapshotPath is the snapshot file in sessionDir.
func SnapshotPath(sessionDir string) string {
	return filepath.Join(sessionDir, SnapshotName)
}

// RingDir is the conversation ring folder in sessionDir.
func RingDir(sessionDir string) string {
	return filepath.Join(sessionDir, RingDirName)
}

// Snapshot is the snapshot file. Clocks are epoch milliseconds.
type Snapshot struct {
	SchemaVersion int `json:"schemaVersion"`
	// ReplyID names the session.
	ReplyID string `json:"replyId"`
	// Adapter names the host adapter feeding the session ("pi", "claude").
	// A fresh snapshot with an adapter is the signal that the session has
	// one; see AdapterPresent.
	Adapter string `json:"adapter,omitempty"`
	// UpdatedAt is the time of the last write, heartbeats included.
	UpdatedAt int64 `json:"updatedAt"`
	// HeartbeatMs is the writer's heartbeat interval; see Stale.
	HeartbeatMs int64   `json:"heartbeatMs,omitempty"`
	Lead        Lead    `json:"lead"`
	Agents      []Agent `json:"agents,omitempty"`
	// Usage is the session's usage figures as its adapter last reported them
	// (a status event); absent before the first one.
	Usage *Usage `json:"usage,omitempty"`
	// Repo is the session root's folder and branch state, which the writing
	// ws-mcp reads itself; absent until its first read.
	Repo *Repo `json:"repo,omitempty"`
}

// Usage limit kinds: the plan's rolling 5-hour and weekly limits.
const (
	LimitFiveHour = "five_hour"
	LimitSevenDay = "seven_day"
)

// Usage is what a status event reports. Each part is absent when the host
// has no figure for it, and a status event replaces the whole value.
type Usage struct {
	Context *UsageContext `json:"context,omitempty"`
	// Limits holds at most one limit per kind, the 5-hour limit first.
	Limits []UsageLimit `json:"limits,omitempty"`
	Cost   *UsageCost   `json:"cost,omitempty"`
}

// UsageContext is the context window's occupancy.
type UsageContext struct {
	// Tokens is the context's size in tokens; null while the host does not
	// know it (after a compaction, until the next response).
	Tokens *int64 `json:"tokens"`
	// Window is the context window in tokens; 0 when unknown.
	Window int64 `json:"window,omitempty"`
	// Percent is Tokens of Window as the host reports it, else computed;
	// absent while Tokens or Window is unknown.
	Percent *float64 `json:"percent,omitempty"`
	// CacheHit is the last response's prompt cache hit rate in percent:
	// cache reads over uncached input, cache reads, and cache writes.
	CacheHit *float64 `json:"cacheHit,omitempty"`
}

// UsageLimit is one plan usage limit.
type UsageLimit struct {
	// Kind is LimitFiveHour or LimitSevenDay.
	Kind    string  `json:"kind"`
	Percent float64 `json:"percent"`
	// ResetsAt is when the limit's window resets (epoch ms); 0 when unknown.
	ResetsAt int64 `json:"resetsAt,omitempty"`
}

// UsageCost is the session's cost in US dollars.
type UsageCost struct {
	USD float64 `json:"usd"`
	// Estimate marks a figure summed from known prices rather than the
	// host's own; Incomplete marks an estimate that left out calls of
	// unknown price.
	Estimate   bool `json:"estimate,omitempty"`
	Incomplete bool `json:"incomplete,omitempty"`
}

// Repo is the session root's folder and branch state. Branch is empty when
// the root is not a repository; Upstream is empty when the branch tracks
// none, and Ahead and Behind count commits against it as of the last fetch.
// Modified (unstaged), Staged, and Untracked count working-tree entries; all
// zero is a clean tree and also a read that failed, which show alike.
type Repo struct {
	Folder   string `json:"folder"`
	Branch   string `json:"branch,omitempty"`
	Detached bool   `json:"detached,omitempty"`
	Upstream string `json:"upstream,omitempty"`
	Ahead    int    `json:"ahead,omitempty"`
	Behind   int    `json:"behind,omitempty"`

	Modified  int `json:"modified,omitempty"`
	Staged    int `json:"staged,omitempty"`
	Untracked int `json:"untracked,omitempty"`
}

// Activity is what the lead and a subagent share: the batch view and the
// state around it.
type Activity struct {
	// Phase is idle, thinking, replying, tool, permission, or (the lead's
	// only) compacting.
	Phase          string `json:"phase,omitempty"`
	PhaseStartedAt int64  `json:"phaseStartedAt,omitempty"`
	// Done and Failed count finished and failed tool calls: over the current
	// turn for the lead, over the agent's life for a subagent.
	Done    int    `json:"done,omitempty"`
	Failed  int    `json:"failed,omitempty"`
	Caption string `json:"caption,omitempty"`
	// Batches is the newest BatchCap batches, oldest first; an assistant
	// message still generating is the last entry.
	Batches    []BatchView `json:"batches,omitempty"`
	Permission *Permission `json:"permission,omitempty"`
}

// Lead is the snapshot's root entry.
type Lead struct {
	Activity
	// TurnStartedAt is the current turn's start; absent while idle.
	TurnStartedAt int64 `json:"turnStartedAt,omitempty"`
	// TurnOrigin is what started the current turn (terminal, whiteboard,
	// mail), when the adapter knows.
	TurnOrigin string `json:"turnOrigin,omitempty"`
	// PreviousTurnMs is the last finished turn's duration.
	PreviousTurnMs int64 `json:"previousTurnMs,omitempty"`
	// Reply is the lead's reply text still streaming; a finished reply moves
	// to the ring.
	Reply *Reply `json:"reply,omitempty"`
}

// Agent is one subagent in the tree, keyed by the host's native agent id.
type Agent struct {
	Activity
	ID       string `json:"id"`
	ParentID string `json:"parentId,omitempty"`
	Label    string `json:"label,omitempty"`
	// ReplyID labels a delegate that holds a ws session key.
	ReplyID   string `json:"replyId,omitempty"`
	Status    string `json:"status"`
	StartedAt int64  `json:"startedAt,omitempty"`
	EndedAt   int64  `json:"endedAt,omitempty"`
}

// Permission is a tool permission prompt the agent waits on. The approval
// itself stays in the terminal.
type Permission struct {
	ToolName    string `json:"toolName"`
	ArgsPreview string `json:"argsPreview,omitempty"`
	StartedAt   int64  `json:"startedAt,omitempty"`
}

// Reply is reply text in progress.
type Reply struct {
	Text      string `json:"text"`
	StartedAt int64  `json:"startedAt,omitempty"`
	Truncated bool   `json:"truncated,omitempty"`
}

// BatchView is one batch: one assistant message and its tool calls. Exactly
// one of EndedAt, Running, and Generating is present. Field names match the
// host batch model, so an adapter that already builds it passes it through.
type BatchView struct {
	Epoch      string          `json:"epoch,omitempty"`
	Seq        int64           `json:"seq"`
	ToolNames  []string        `json:"toolNames"`
	CallCount  int             `json:"callCount"`
	Failed     int             `json:"failed"`
	StartedAt  int64           `json:"startedAt"`
	EndedAt    int64           `json:"endedAt,omitempty"`
	Running    *RunningView    `json:"running,omitempty"`
	Generating *GeneratingView `json:"generating,omitempty"`
}

// RunningView is a running batch's in-flight summary; the argument preview
// appears on the running call only.
type RunningView struct {
	ToolName      string `json:"toolName"`
	ArgsPreview   string `json:"argsPreview"`
	StartedAt     int64  `json:"startedAt"`
	Done          int    `json:"done"`
	OthersRunning int    `json:"othersRunning"`
}

// GeneratingView is an assistant message still generating: thinking,
// replying, or streaming a tool call's arguments (its tool name and the
// characters so far).
type GeneratingView struct {
	Kind           string `json:"kind"`
	PhaseStartedAt int64  `json:"phaseStartedAt"`
	ToolName       string `json:"toolName,omitempty"`
	Chars          int    `json:"chars,omitempty"`
}

// ReadSnapshot reads a snapshot file. Unknown fields are ignored and absent
// ones read as empty.
func ReadSnapshot(path string) (Snapshot, error) {
	var snap Snapshot
	raw, err := os.ReadFile(path)
	if err != nil {
		return snap, err
	}
	err = json.Unmarshal(raw, &snap)
	return snap, err
}

// Stale reports whether the snapshot's writer has stopped: no write for
// StaleAfterBeats heartbeat intervals. A ws-mcp heartbeats every snapshot it
// holds state for, so a crashed session or a ws-mcp that restarted (and lost
// its in-memory state) leaves a snapshot that turns stale instead of showing
// as live indefinitely. An absent heartbeat reads as HeartbeatInterval.
func (s Snapshot) Stale(now time.Time) bool {
	beat := s.HeartbeatMs
	if beat <= 0 {
		beat = HeartbeatInterval.Milliseconds()
	}
	return now.UnixMilli()-s.UpdatedAt > StaleAfterBeats*beat
}

// AdapterPresent reports whether the session at path has a host adapter: a
// readable snapshot that names its adapter and is not stale. A session
// without one shows only its on-disk stores (board-first page).
func AdapterPresent(path string, now time.Time) bool {
	snap, err := ReadSnapshot(path)
	return err == nil && snap.Adapter != "" && !snap.Stale(now)
}

// RepoRunes bounds a Repo's names; git output reaches a browser.
const RepoRunes = 200

// CleanRepo is r as the snapshot keeps it: each name one display line,
// capped at RepoRunes, and counts never negative.
func CleanRepo(r Repo) Repo {
	r.Folder = cleanLine(r.Folder, RepoRunes)
	r.Branch = cleanLine(r.Branch, RepoRunes)
	r.Upstream = cleanLine(r.Upstream, RepoRunes)
	r.Ahead, r.Behind = max(r.Ahead, 0), max(r.Behind, 0)
	r.Modified, r.Staged, r.Untracked = max(r.Modified, 0), max(r.Staged, 0), max(r.Untracked, 0)
	return r
}
