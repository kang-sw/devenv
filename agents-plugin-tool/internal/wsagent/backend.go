package wsagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/kang-sw/devenv/internal/wsinbox"
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

// registrationBackend is the built-in adapter's descriptor provider. Backends
// registered by a caller can still supply a BackendRegistration directly.
type registrationBackend interface {
	Backend
	registration() (string, BackendRegistration)
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
	// MCPSession, when set, attaches the ws-mcp over the daemon's HTTP
	// route (mcp_session.go) in place of WSMCP's command.
	MCPSession *MCPSession
	// Env is the environment the child starts from; the backend removes the
	// harness variables an inherited parent session leaves behind.
	Env []string
	// Lead is set for a lead conversation (Profile.Lead): user settings stay
	// active, while the daemon supplies its ws-mcp and workflow manual.
	Lead *LeadSpec
	// Holder selects durable transport independently of lead privileges.
	Holder *holderChild
	// PermissionLevel is the explicit authority of this launch: a lead's
	// selection, or the level an agent.spawn child was spawned with. Empty
	// keeps legacy native settings. It does not grant tools; a backend reads
	// it only where it maps the level to its own per-run controls (Codex).
	PermissionLevel PermissionLevel
	// RemoveSubagents removes the harness's native subagent tool from the
	// launch whatever the profile or the user's settings allow, so the
	// conversation delegates through agent.spawn (RemovesSubagents). A
	// backend whose harness has no such tool ignores it.
	RemoveSubagents bool
	// RecoveredSteers are the unconsumed worker steers persisted before a
	// holder reattach. A backend may use them to map replayed acknowledgements
	// back to the runner without persisting backend request ids.
	RecoveredSteers []string
}

// LeadSpec is what a lead conversation's process starts with.
type LeadSpec struct {
	// DaemonVersion is the running daemon's version, not the plugin starter's.
	DaemonVersion string
	// PermissionMode is the native permission mode to launch a lead in.
	PermissionMode string
	// PluginIsolation is default (replace ws only) or isolated (bundled only).
	PluginIsolation string
	// PluginWarning records a launch warning on the session; empty clears it.
	PluginWarning func(string)
	// WSMCP resolves the bundled MCP launcher if Pi needs its bridge fallback.
	WSMCP func() (MCPCommand, error)
	// WSMCPErr is why WSMCP gave this start no command, which a Claude
	// lead's refusal names (claudeLeadMCPArgs).
	WSMCPErr error
	// PluginNote is the package fallback warning of the ws-mcp this start
	// resolved, which a Claude lead reports through PluginWarning.
	PluginNote string
	// Manual is the workflow manual the lead reads from its first turn
	// (Options.LeadManual), "" when there is none; Pi's bridge fallback
	// appends it, as the native extension it stands in for would.
	Manual func() string
	// Env is set on the child over the backend's own scrub: the
	// hosted-session variables.
	Env map[string]string
	// Spawned records a new process before Start returns, for a backend
	// whose start waits on its harness reaching ws-mcp (agy's session
	// request): ws-mcp confirms a hosted session by the registration's
	// host_pid, so the row must name the process first.
	Spawned func(holderID string, pid int)
}

// MCPCommand is a stdio MCP server's launch command.
type MCPCommand struct {
	// PluginWarning is a runtime-only package fallback notice for this launch.
	PluginWarning string `json:"-"`
	// Plugin is the hosted bundle the launcher belongs to, when known.
	Plugin  string            `json:"-"`
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

// errNoApprovalAnswer marks an AnswerApproval that wrote nothing because the
// process could not build the answer; any other error is a failed write.
var errNoApprovalAnswer = errors.New("no answer was written")

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

// reportSteerer is a steerer that can take a child's report into its
// running turn (children.go): its events answer each steer (SteerAnswer)
// and tell which user messages its run took (UserTaken), and ClearSteered
// drops the steered messages it queued and no run took, so such a report
// is sent once as a turn instead.
type reportSteerer interface {
	steerer
	ClearSteered() error
}

// commandRunner is a Process that runs a dashboard slash command through a
// mapping of its own (pi's /compact is its compact command); any other
// Process gets the command's bare typed line as a turn.
type commandRunner interface {
	RunCommand(cmd wsinbox.Command) error
}

// turnProber is a Process whose harness answers whether a turn runs (a
// lead's pi process: its get_state). A rejoin whose replay cannot tell asks
// it rather than wait for a turn end an idle harness never sends.
type turnProber interface {
	TurnRunning() (bool, error)
}

// workerRecoverer reconciles a worker's saved messages after a replay gap.
// This is separate from turnProber: knowing a harness is idle alone cannot
// recover a child's final reply or which steered messages it took.
type workerRecoverer interface {
	turnProber
	recoveryReply(startedAt int64) (message, turnErr string, taken []string, err error)
}

// processControls is the runner's control boundary for one launched process.
// Nil optional controls are unavailable; required Process controls still
// decide support from their arguments (notably Pi model/effort changes).
// It owns no input queue or event reader: delivery stays in the conv loop.
type processControls struct {
	Process
	approval approvalAnswerer
	mode     modeSetter
	mcp      mcpController
	steer    steerer
	reports  reportSteerer
	commands commandRunner
	turns    turnProber
	recovery workerRecoverer
}

// processControlProvider narrows a method set when support depends on how
// the process was launched, rather than just which methods its type has.
type processControlProvider interface {
	processControls() processControls
}

func controlsFor(proc Process) processControls {
	if provider, ok := proc.(processControlProvider); ok {
		return provider.processControls()
	}
	// Optional interfaces remain the extension point for other Process
	// implementations, including isolated test processes.
	c := processControls{Process: proc}
	c.approval, _ = proc.(approvalAnswerer)
	c.mode, _ = proc.(modeSetter)
	c.mcp, _ = proc.(mcpController)
	c.steer, _ = proc.(steerer)
	c.reports, _ = proc.(reportSteerer)
	c.commands, _ = proc.(commandRunner)
	c.turns, _ = proc.(turnProber)
	c.recovery, _ = proc.(workerRecoverer)
	return c
}

func (c processControls) runCommand(cmd wsinbox.Command) error {
	if c.commands != nil {
		return c.commands.RunCommand(cmd)
	}
	return c.Send(cmd.Line())
}

func (c processControls) setPermissionMode(mode string) error {
	if c.mode == nil {
		return ErrUnsupported
	}
	return c.mode.SetPermissionMode(mode)
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
// except that a TurnEnd may carry TurnError, a BackgroundTasks update may
// carry known Background and Monitors splits, and Seq and Replay mark
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
	// marks a replay whose ring has dropped records (its first is past 1),
	// so the turn state it rebuilds may miss a turn boundary.
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
	// CommandError reports a refused command without settling the active
	// run. It updates State.Error, not the conversation or typed failure.
	CommandError string
	// BackgroundTasks is the harness's aggregate background task count. A
	// whole-list update also replaces the optional split counts: nil means
	// the stream could not classify every task. The aggregate still includes
	// background subagents and gates idle stop and recovery.
	BackgroundTasks *int
	Background      *int
	Monitors        *int
	// Alert is a typed failure the harness reported for the lead's request
	// (a login needed, a model unavailable, a usage limit): a conversation
	// line, and the session's failure state until a turn succeeds.
	Alert *wslive.Notice
	// Retry is an API request retry the harness started; RetryEnd that the
	// retrying is over (a response came, or the turn ended).
	Retry    *wslive.Retry
	RetryEnd bool
	// Harness is the catalog report of a process's start: its harness
	// version and the items its init shape demotes (catalog.go).
	Harness *HarnessReport
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
	// LiveCoalescesTurnEnd marks a live status immediately followed by this
	// mapper's TurnEnd. File-only recovery can persist both in one snapshot.
	LiveCoalescesTurnEnd bool
	// ReasoningSummary is a validated lead-only widget projection, never an
	// extension-supplied lifecycle/usage/key command.
	ReasoningSummary *wslive.ReasoningSummary
	// summaryCapability is the hosted bundle's explicit process-local opt-in,
	// consumed by piMapper, never stored or inferred from a summary payload.
	summaryCapability *bool
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
	// in an error (a refusal, an interrupt). ModelFailure (with TurnEnd) carries
	// an in-band API/stream failure that remained at the reply tail through the
	// end. ModelFailureUpdate is an internal-only change to that terminal-tail
	// candidate; an empty value clears a candidate after later model output.
	TurnEnd            bool
	TurnError          string
	TurnReason         string
	ModelFailure       string
	ModelFailureUpdate *string
	// ModelActivity is an internal-only signal that a raw stream update arrived
	// during a turn, carrying the count of currently pending tool calls
	// (including daemon commands) so the model-wait watchdog can reset or pause.
	ModelActivity *int
	// CompactRequested (with TurnEnd): the ending turn carried a successful
	// compaction lever call (a *__request_compaction tool call whose result
	// is not an error) and was not interrupted. The runner holds queued
	// input until that compaction starts (leverHoldCap at most), so the
	// compaction runs before the next turn.
	CompactRequested bool
	// Fatal ends the process with an error the conversation reports, such as
	// a failed init check; the process exits after it.
	Fatal error
	// SteerAnswer is a non-lead process's answer to its next unanswered
	// steer command (a child's report, children.go), in the order they
	// were written; UserTaken is the text of a user message its run took
	// (a prompt, or a steered message it worked in).
	SteerAnswer *SteerAnswer
	UserTaken   string
}

// SteerAnswer is how the harness answered a steer command: Refusal is
// empty when it queued the message for its run.
type SteerAnswer struct {
	Refusal string
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

// hostWindows gates the cmd-shim unwrap in harnessCommand; a test
// replaces it.
var hostWindows = runtime.GOOS == "windows"

// harnessCommand resolves the program and leading arguments a harness
// start runs: command when set, else name from PATH. On Windows an npm
// cmd-shim (.cmd/.bat) is unwrapped to node and its script, or to the
// native exe it runs, because Windows runs a .cmd through cmd.exe: the
// started PID would be cmd.exe's, not the harness's, and a hosted session's
// host PID must be the harness process itself (ws-mcp and the Pi extension
// compare it with their own parent / own PID). A shim it will not unwrap is
// refused with a remedy for name. Elsewhere a set command passes through
// untouched.
func harnessCommand(command, name string) (string, []string, error) {
	if command == "" || hostWindows {
		if command == "" {
			command = name
		}
		path, err := lookPath(command)
		if err != nil {
			return "", nil, err
		}
		command = path
	}
	if !hostWindows {
		return command, nil, nil
	}
	switch strings.ToLower(filepath.Ext(command)) {
	case ".cmd", ".bat":
	default:
		return command, nil, nil
	}
	text, err := os.ReadFile(command)
	if err != nil {
		return "", nil, fmt.Errorf("read %s: %w", command, err)
	}
	prog, pre, err := cmdShimArgv(command, string(text), fileExists)
	if errors.Is(err, errShimRefused) {
		return "", nil, fmt.Errorf("%w; %s", err, shimRemedy(command, name))
	}
	return prog, pre, err
}

// shimRemedy tells the user how to get a refused shim out of the way of
// harness name.
func shimRemedy(shim, name string) string {
	if name == "claude" {
		return fmt.Sprintf(`install Claude Code's native build, or put the directory holding the native claude.exe (for example %%USERPROFILE%%\.local\bin) ahead of %s on PATH`, filepath.Dir(shim))
	}
	return fmt.Sprintf("install %s's native build, or put the directory holding its real executable ahead of %s on PATH", name, filepath.Dir(shim))
}

// fileExists reports whether path names an existing file.
func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// cmdShimLine is the launch line of an npm cmd-shim (npm's cmd-shim
// generator): `"%_prog%"  "%dp0%\<script>" %*`, the script relative to the
// shim's directory.
var cmdShimLine = regexp.MustCompile(`(?m)"%_prog%"\s+"%dp0%\\([^"]+)"\s+%\*\s*$`)

// cmdShimNode is the shim's node fallback, which names node as the program;
// a shim for another interpreter does not have it.
var cmdShimNode = regexp.MustCompile(`(?m)^\s*SET "_prog=node"\s*$`)

// cmdShimExe is the launch line of an npm cmd-shim for a native binary
// (e.g. @anthropic-ai/claude-code): `"%dp0%\<path>.exe"   %*`, the exe
// relative to the shim's directory and run with no leading arguments.
var cmdShimExe = regexp.MustCompile(`(?m)^[ \t]*"%dp0%\\([^"]+\.(?i:exe))"[ \t]+%\*[ \t]*\r?$`)

// errShimRefused marks a .cmd/.bat that cmdShimArgv will not unwrap.
var errShimRefused = errors.New("a .cmd would run under cmd.exe, whose PID is not the harness's")

// cmdShimArgv unwraps the npm cmd-shim at shim, whose content is text, to
// the program and leading arguments it runs. A node shim runs node.exe
// beside the shim when exists says it is there, else node from PATH, then
// the script; a native-binary shim runs its exe, which exists must find.
// Any other .cmd is refused (errShimRefused) rather than run through
// cmd.exe.
func cmdShimArgv(shim, text string, exists func(string) bool) (string, []string, error) {
	dir := filepath.Dir(shim)
	m := cmdShimLine.FindStringSubmatch(text)
	if m == nil || !cmdShimNode.MatchString(text) {
		e := cmdShimExe.FindStringSubmatch(text)
		if e == nil {
			return "", nil, fmt.Errorf("%s is not an npm cmd-shim that can be started directly; %w", shim, errShimRefused)
		}
		rel := strings.ReplaceAll(e[1], `\`, "/")
		if strings.HasPrefix(rel, "/") || strings.Contains(rel, ":") || slices.Contains(strings.Split(rel, "/"), "..") {
			return "", nil, fmt.Errorf("%s runs %s, which is not inside the shim's directory; %w", shim, e[1], errShimRefused)
		}
		exe := filepath.Join(dir, filepath.FromSlash(rel))
		if !exists(exe) {
			return "", nil, fmt.Errorf("%s runs %s, which does not exist; %w", shim, exe, errShimRefused)
		}
		return exe, nil, nil
	}
	// %dp0% ends in a separator and the shim adds another; Join cleans both.
	script := filepath.Join(dir, filepath.FromSlash(strings.ReplaceAll(m[1], `\`, "/")))
	node := filepath.Join(dir, "node.exe")
	if !exists(node) {
		path, err := lookPath("node")
		if err != nil {
			return "", nil, fmt.Errorf("%s runs node, which is not beside it or on PATH: %w", shim, err)
		}
		node = path
	}
	return node, []string{script}, nil
}

// environ is the environment a child starts from; a test replaces it.
var environ = os.Environ

// workDir is the child's working directory.
func (s StartSpec) workDir() string {
	if s.Cwd != "" {
		return s.Cwd
	}
	return s.Root
}
