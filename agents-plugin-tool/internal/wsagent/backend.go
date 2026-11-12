package wsagent

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/kang-sw/devenv/internal/wslive"
)

// Backend starts one harness's headless process. A backend owns the
// harness's flags, its output parsing, and its init check; the runner owns
// the conversation around it (turns, held sends, the session directory).
type Backend interface {
	// Start launches a sustained process for one conversation. A non-empty
	// spec.SessionID resumes that backend session; empty starts a new one,
	// whose id arrives as an Event.SessionID.
	Start(ctx context.Context, spec StartSpec) (Process, error)
}

// StartSpec is what a backend launches a process from.
type StartSpec struct {
	// Root is the conversation's working root: the child's working
	// directory, and the root relative profile paths resolve against.
	Root string
	// Dir is the conversation's session directory, where a backend may keep
	// files it launches with (the composed instruction file is already
	// there).
	Dir     string
	Profile Profile
	// Model and Effort are the conversation's current values; empty leaves
	// the harness default.
	Model  string
	Effort string
	// SessionID is the backend session to resume; empty starts a new one.
	SessionID string
	// Cwd is the child's working directory when it is not Root, and
	// SessionFile the session file a lead conversation runs on in place of
	// its own (Record.Cwd, Record.SessionFile).
	Cwd         string
	SessionFile string
	// Instructions is the composed instruction file to append to the
	// harness's own system prompt; empty appends none.
	Instructions string
	// WSMCP is the ws-mcp server to attach, carrying the profile's ws tool
	// allowlist; nil when the profile allows no ws tool.
	WSMCP *MCPCommand
	// Env is the environment the child starts from; the backend removes the
	// harness variables an inherited parent session leaves behind.
	Env []string
	// Lead is set for a lead conversation (Profile.Lead): the process starts
	// as the user's own session does, under a session holder.
	Lead *LeadSpec
}

// LeadSpec is what a lead conversation's process starts with.
type LeadSpec struct {
	// PermissionMode is the permission mode to launch in (claude).
	PermissionMode string
	// Env is set on the child over the backend's own scrub: the
	// hosted-session variables.
	Env map[string]string
	// Holder runs the child under a session holder; a backend passes the
	// spec to startChild, which uses it.
	Holder *holderChild
}

// MCPCommand is a stdio MCP server's launch command.
type MCPCommand struct {
	Command string            `json:"command"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
}

// Process is one running backend process. Its methods are safe to call from
// one goroutine at a time (the runner's conversation loop).
type Process interface {
	// Events delivers the process's output in order; it is closed after the
	// process exits.
	Events() <-chan Event
	// Send submits one user turn.
	Send(text string) error
	// Interrupt asks the running turn to stop; the turn still ends with a
	// TurnEnd event.
	Interrupt() error
	// SetModel and SetEffort change the value for the next turn in-stream;
	// ErrUnsupported makes the runner restart the process on the same
	// backend session instead.
	SetModel(model string) error
	SetEffort(effort string) error
	// Close ends the process: it closes the child's input, waits a grace
	// period, then terminates and finally kills it. It returns once the
	// child has exited; Events is closed by then.
	Close() error
}

// approvalAnswerer is a Process that takes the answer to a tool permission
// prompt, and modeSetter one whose permission mode changes in-stream
// (ErrUnsupported when it cannot); a lead's claude process is both.
type approvalAnswerer interface {
	AnswerApproval(req ApprovalRequest, allow bool, message string) error
}

type modeSetter interface {
	SetPermissionMode(mode string) error
}

// mcpController is a Process whose harness reports its MCP servers'
// status and reconnects one in-stream; a lead's claude process is one.
type mcpController interface {
	MCPStatus() ([]MCPServer, error)
	ReconnectMCP(server string) error
}

// MCPServer is one MCP server of a harness and its connection status, in
// Claude Code's words (MCPConnected, MCPFailed, and others: pending,
// needs-auth, disabled).
type MCPServer struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

// MCP server statuses the runner acts on.
const (
	MCPConnected = "connected"
	MCPFailed    = "failed"
)

// steerer is a Process that takes a user message while its turn runs: the
// harness works it in at its next step, inside the same turn.
type steerer interface {
	Steer(text string) error
}

// ErrUnsupported is a Process's answer to a control it has no in-stream
// form of.
var ErrUnsupported = errors.New("not supported in-stream")

// ApprovalRequest is a tool permission prompt a lead's harness waits on
// (claude's can_use_tool control request). Field names are the record's
// on-disk format and the route's answer.
type ApprovalRequest struct {
	// ID is the harness's request id: what an answer names.
	ID   string `json:"id"`
	Tool string `json:"tool"`
	// Input is the tool call's input as the harness sent it; an allow hands
	// it back unchanged.
	Input       json.RawMessage `json:"input,omitempty"`
	Description string          `json:"description,omitempty"`
	// ToolUseID is the tool call the prompt is for; its tool result settles
	// the prompt when the answer itself was not seen (a replay).
	ToolUseID string `json:"tool_use_id,omitempty"`
	// AgentID names the subagent whose tool call asks; empty is the lead.
	AgentID   string `json:"agent_id,omitempty"`
	StartedAt int64  `json:"started_at,omitempty"`
}

// Harness states (Event.HarnessState), under the names Claude Code's
// session_state_changed line gives them: a turn runs, the harness waits on
// the user's answer to a prompt, or it waits for input.
const (
	HarnessRunning = "running"
	HarnessWaiting = "requires_action"
	HarnessIdle    = "idle"
)

// Event is one item of a process's output. Exactly one of its parts is set,
// except that a TurnEnd may carry TurnError and that Seq and Replay mark
// any event of a process under a session holder.
type Event struct {
	// Seq is the holder record sequence of the line the event came from;
	// 0 for the daemon's own child. Replay marks a line the runner already
	// applied under an earlier daemon: it rebuilds state and writes
	// nothing. LineEnd (no other part) follows a holder line's last event.
	Seq     uint64
	Replay  bool
	LineEnd bool
	// ReplayDone (no other part but ReplayLost): every record the holder
	// held when the daemon attached again has been delivered. ReplayLost
	// marks a replay whose ring no longer began right after the records the
	// runner applied under an earlier daemon.
	ReplayDone bool
	ReplayLost bool
	// ReplayGap (no other part): records left the holder's ring before
	// they reached this daemon, at this point of the stream.
	ReplayGap bool
	// TurnStart: the harness began a turn. A lead's backend sends it for
	// every turn, since a turn may start with no send (a background task's
	// completion, a compaction, a pushed batch); the runner starts one
	// unless its own send already did.
	TurnStart bool
	// Approval is a tool permission prompt now waiting; ApprovalDone names
	// one (by request id) that is settled: withdrawn by the harness, or its
	// tool call returned.
	Approval     *ApprovalRequest
	ApprovalDone string
	// PermissionMode is the permission mode the harness reports it runs in.
	PermissionMode string
	// HarnessState is the state the harness reports it moved to
	// (HarnessRunning, HarnessWaiting, HarnessIdle).
	HarnessState string
	// Notice is a line for the daemon's log: something the harness did
	// that the conversation does not show.
	Notice string
	// BackgroundTasks is the harness's current background task count, when
	// the line reported the whole list.
	BackgroundTasks *int
	// Alert is a typed failure the harness reported for the lead's request
	// (a login needed, a model unavailable, a usage limit): a conversation
	// line, and the session's failure state until a turn succeeds.
	Alert *wslive.Notice
	// Retry is an API request retry the harness started; RetryEnd that the
	// retrying is over (a response came, or the turn ended).
	Retry    *wslive.Retry
	RetryEnd bool
	// MCPServers is the MCP server list the harness reported with each
	// server's status (an init line).
	MCPServers []MCPServer
	// MCPCheck: something on the stream suggests an MCP server broke (a
	// failed MCP tool call, the lead asking for a reconnect), so the runner
	// asks the harness for its servers' status.
	MCPCheck bool

	// Live is a content event for the conversation's wslive state: a reply,
	// the lead's tool batches, or usage figures. The runner applies turn
	// boundaries and user messages itself; a backend never sends
	// turn_start, turn_end, user, or reset.
	Live *wslive.Event
	// Card is a tracker card the lead showed: a successful tracker.show call
	// of the attached ws-mcp whose result opens with its card line.
	Card *wslive.CardRef
	// UI is a dashboard UI command the lead sent: a successful tracker.ui
	// call of the attached ws-mcp whose result opens with its ui line.
	UI *wslive.UICommand
	// TrackerWrite: a successful tracker.update call of the attached ws-mcp
	// (TrackerWriteTool) returned, so a tracker store may have changed.
	TrackerWrite bool
	// SessionID is the backend session id, once known; the runner persists
	// it for resume.
	SessionID string
	// TurnEnd ends the running turn; TurnError describes a turn that ended
	// in an error (a refusal, an interrupt).
	TurnEnd   bool
	TurnError string
	// Fatal ends the process with an error the conversation reports, such as
	// a failed init check; the process exits after it.
	Fatal error
}

// harnessEnv returns env without the variables a parent harness session
// leaves behind for its own children (session ids, sockets, plugin
// directories), keeping the user's configuration and credentials: a name
// with one of dropPrefixes is dropped unless keep lists it.
func harnessEnv(env []string, dropPrefixes []string, keep map[string]bool, set map[string]string) []string {
	out := make([]string, 0, len(env)+len(set))
	for _, kv := range env {
		name, _, _ := strings.Cut(kv, "=")
		if _, override := set[name]; override {
			continue
		}
		drop := false
		for _, prefix := range dropPrefixes {
			if strings.HasPrefix(name, prefix) && !keep[name] {
				drop = true
				break
			}
		}
		if !drop {
			out = append(out, kv)
		}
	}
	for name, value := range set {
		out = append(out, name+"="+value)
	}
	return out
}

// closeGrace is how long Close waits for the child to exit on its own after
// its input closes, and then after SIGTERM, before it kills it.
var closeGrace = 3 * time.Second

// stopChild ends cmd, whose input stdin is: close stdin (a headless harness
// exits at input EOF), then SIGTERM, then SIGKILL, waiting closeGrace after
// each step. exited is closed once cmd.Wait returned.
func stopChild(cmd *exec.Cmd, closeInput func() error, exited <-chan struct{}) {
	_ = closeInput()
	select {
	case <-exited:
		return
	case <-time.After(closeGrace):
	}
	if cmd.Process != nil {
		_ = cmd.Process.Signal(syscall.SIGTERM)
	}
	select {
	case <-exited:
		return
	case <-time.After(closeGrace):
	}
	if cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
	<-exited
}

// lookPath resolves a harness command; a test replaces it.
var lookPath = exec.LookPath

// environ is the environment a child starts from; a test replaces it.
var environ = os.Environ

// workDir is the child's working directory.
func (s StartSpec) workDir() string {
	if s.Cwd != "" {
		return s.Cwd
	}
	return s.Root
}
