package wsagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kang-sw/devenv/internal/wsinbox"
	"github.com/kang-sw/devenv/internal/wslive"
)

// CodexBackend drives one sustained `codex app-server` process over stdio.
// Hosted leads and workers run under a holder, which owns their pipes
// across daemon replacement, as for agy and Claude.
type CodexBackend struct {
	// Command is the Codex executable; empty looks up "codex" on PATH.
	Command string
}

// A thread request may connect the user's MCP servers before answering.
var codexRequestTimeout = 2 * time.Minute

func (b CodexBackend) Executable() string {
	if b.Command != "" {
		return b.Command
	}
	return "codex"
}

// codexChoices holds model/list from the last opened process, stamped when
// it arrived. A fresh report takes precedence over the session-less cache
// file listing, and a newer one replaces it; one older than choicesTTL
// answers nothing, so an updated CLI shows through the file again.
var codexChoices struct {
	sync.Mutex
	models  []ModelChoice
	efforts []string
	at      time.Time
}

func (b CodexBackend) Choices(context.Context) ([]ModelChoice, []string, error) {
	return b.hostedChoices()
}

// codexEnv removes parent-session identities, but keeps the harness-owned
// CODEX_HOME and authentication. The HTTP MCP bearer exists only in the
// child's environment, never in argv, the thread config, or its transcript.
func codexEnv(spec StartSpec) []string {
	set := map[string]string{}
	if spec.Lead != nil {
		for name, value := range spec.Lead.Env {
			set[name] = value
		}
	}
	if spec.MCPSession != nil {
		set[EnvMCPSessionToken] = spec.MCPSession.Token
	}
	drop := append([]string{"CLAUDE", "CODEX_", "PI_", "ORCA_", "WS_PI_"}, leadDropEnv...)
	return harnessEnv(spec.Env, drop, map[string]bool{"CODEX_HOME": true}, set)
}

// Start initializes and opens a new process before returning. A holder
// reattach sends nothing: replay rebuilds the mapper, and the runner alone
// decides when to deliver input or reconcile a gap.
func (b CodexBackend) Start(_ context.Context, spec StartSpec) (Process, error) {
	if spec.Lead != nil && spec.Holder == nil {
		return nil, errors.New("codex backend: a hosted lead requires a session holder")
	}
	attach := spec.Holder != nil && spec.Holder.id != ""
	if !attach && spec.MCPSession == nil {
		return nil, errors.New("codex backend: the daemon's HTTP MCP route is required; no session was started without ws tools")
	}
	mode, policy, err := codexStartMode(spec)
	if err != nil {
		return nil, fmt.Errorf("codex backend: %w", err)
	}
	command, pre, err := harnessCommand(b.Command, "codex")
	if err != nil {
		return nil, fmt.Errorf("codex backend: %w", err)
	}
	cmd := exec.Command(command, append(pre, "app-server")...)
	cmd.Dir = spec.workDir()
	cmd.Env = codexEnv(spec)
	mapper := newCodexMapper(spec.SessionID)
	mapper.readOnlyWorker = spec.Lead == nil && policy == "never"
	if attach {
		// The holder may have dropped a reply the earlier daemon already
		// wrote. Read its durable identity once at reattach; steady-state
		// send, mapping and recovery stay entirely in memory.
		page, err := wslive.ReadRingPage(wslive.RingDir(spec.Dir), "", wslive.RingHistoryCap)
		if err != nil {
			return nil, fmt.Errorf("codex backend: read persisted reply identities: %w", err)
		}
		for _, entry := range page.Records {
			if entry.Kind == wslive.RingReply && entry.Source != "" {
				mapper.persisted[entry.Source] = true
			}
		}
	}
	stream, err := startChild(spec, cmd, mapper, codexProtocol{})
	if err != nil {
		return nil, err
	}
	p := &codexProcess{
		streamProcess: stream, m: mapper, epoch: newMCPToken(),
		model: spec.Model, effort: spec.Effort, mode: mode, policy: policy, worker: spec.Lead == nil,
	}
	holder, pid, _ := stream.HolderInfo()
	if p.worker && cmd.Process != nil {
		// A direct worker's MCP hello names the child itself.
		pid = cmd.Process.Pid
	}
	if session := spec.MCPSession; session != nil && session.Started != nil {
		session.Started(cmd.Env, pid)
	}
	if attach {
		return p, nil
	}
	// MCP admission checks the hosted row's physical PID. Publish it before
	// thread/start connects, not after the handshake that waits on it.
	if spec.Lead != nil && spec.Lead.Spawned != nil {
		spec.Lead.Spawned(holder, pid)
	}
	if err := p.open(spec); err != nil {
		_ = p.Close()
		return nil, fmt.Errorf("codex backend: %w", err)
	}
	return p, nil
}

type codexProtocol struct{}

func (codexProtocol) userLine(string) any { return nil }

func (codexProtocol) requestLine(id string, request map[string]any) any {
	return map[string]any{"id": id, "method": request["method"], "params": request["params"]}
}

// Kind-bearing ids let a later daemon interpret responses in the holder
// ring. A random per-attach epoch prevents collisions with in-flight calls
// from the previous daemon. None of these ids is a persisted counter.
type codexProcess struct {
	*streamProcess
	m     *codexMapper
	epoch string
	seq   atomic.Uint64

	// The conversation loop owns these next-turn settings. mode is the
	// sandbox name and policy the approval policy sent with it.
	model  string
	effort string
	mode   string
	policy string
	// worker marks a spawned child, whose sandbox follows its authority.
	worker bool
}

func (p *codexProcess) processControls() processControls {
	return processControls{Process: p, approval: p, mode: p, steer: p, commands: p, turns: p}
}

func (p *codexProcess) call(kind, method string, params map[string]any) (json.RawMessage, error) {
	id := "wsc/" + kind + "/" + p.epoch + "/" + strconv.FormatUint(p.seq.Add(1), 10)
	return p.requestBodyID(id, method, map[string]any{"method": method, "params": params}, codexRequestTimeout)
}

func (p *codexProcess) open(spec StartSpec) error {
	version := codexWorkerClientVersion
	if spec.Lead != nil {
		version = spec.Lead.DaemonVersion
	}
	_, err := p.call("init", "initialize", map[string]any{
		"clientInfo":   map[string]any{"name": "ws", "version": version},
		"capabilities": map[string]any{"experimentalApi": true},
	})
	if err != nil {
		return fmt.Errorf("initialize: %w", err)
	}
	if err := p.writeLine(map[string]any{"method": "initialized"}); err != nil {
		return err
	}

	params := map[string]any{
		"cwd":    spec.workDir(),
		"config": codexThreadConfig(spec),
	}
	if spec.Instructions != "" {
		text, err := os.ReadFile(spec.Instructions)
		if err != nil {
			return fmt.Errorf("read instruction file: %w", err)
		}
		params["developerInstructions"] = string(text)
	}
	if spec.Model != "" {
		params["model"] = spec.Model
	}
	if p.mode != "" {
		params["sandbox"], params["approvalPolicy"] = p.mode, p.policy
	}
	method := "thread/start"
	if spec.SessionID != "" {
		params["threadId"] = spec.SessionID
		method = "thread/resume"
	}
	if _, err := p.call("open", method, params); err != nil {
		return fmt.Errorf("%s: %w", method, err)
	}
	// A listing failure is not an admission failure: the harness default
	// still works. Expose no guessed model names or effort levels.
	if _, err := p.call("models", "model/list", map[string]any{}); err != nil {
		p.deliver(Event{Notice: "codex model/list: " + err.Error()})
	}
	return nil
}

// codexThreadConfig builds the thread/start and thread/resume config override.
// Both distributed plugin server names are shadowed, not the plugins: their
// skills and user-controlled hooks stay loaded. Only ws opens a client: one
// HTTP session per bearer means two clients would collide on request IDs.
// Disabled wsflow still needs a transport for Codex to accept it. For spawned
// workers (spec.Lead == nil), harmless ws tools in ChildSafeWSTools are
// pre-approved via mcp_servers.ws.tools.<name>.approval_mode = "approve" so
// read-only workers (approvalPolicy "never") can call them without widening
// mutating tools; leads stay unchanged.
func codexThreadConfig(spec StartSpec) map[string]any {
	server := map[string]any{
		"url": spec.MCPSession.URL, "bearer_token_env_var": EnvMCPSessionToken, "required": true,
	}
	if spec.Lead == nil {
		if tools := codexChildSafeTools(spec); len(tools) > 0 {
			server["tools"] = tools
		}
	}
	return map[string]any{
		"agents": map[string]any{"enabled": false},
		"mcp_servers": map[string]any{
			"ws":     server,
			"wsflow": map[string]any{"url": spec.MCPSession.URL, "enabled": false},
		},
	}
}

func codexChildSafeTools(spec StartSpec) map[string]any {
	safe := childSafeWSToolsFor(spec.Profile)
	tools := make(map[string]any, len(safe))
	for _, name := range safe {
		tools[name] = map[string]any{"approval_mode": "approve"}
	}
	return tools
}

func codexMode(mode string) (sandbox, policy string, err error) {
	if !slices.Contains(CodexPermissionModes, mode) {
		return "", "", fmt.Errorf("codex has no permission mode %q", mode)
	}
	policy = "on-request"
	if mode == "danger-full-access" {
		policy = "never"
	}
	return mode, policy, nil
}

// codexWorkerClientVersion is the initialize clientInfo version of a worker
// start, which has no lead spec to read the daemon's version from.
const codexWorkerClientVersion = "worker"

// codexWorkerMode maps a spawned child's authority to a sandbox and an
// approval policy. Workdir-write runs in the workspace-write sandbox with
// on-request approvals, which the runner's WorkerAutoApprove answers and
// traces: the confinement is advisory, not enforced. Full authority keeps
// those on-request approvals but uses Codex's danger-full-access sandbox, as
// full already means unrestricted authority. Read-only starts in Codex's
// native read-only sandbox with approval policy never, so Codex refuses an
// escalation itself and no request reaches the auto-allow; codexMode would
// give on-request and let the auto-allow run escalated commands outside the
// sandbox. A worker with no authority is refused rather than started with a
// guess.
func codexWorkerMode(level PermissionLevel) (sandbox, policy string, err error) {
	switch level {
	case PermissionReadOnly:
		return "read-only", "never", nil
	case PermissionWorkdirWrite:
		return "workspace-write", "on-request", nil
	case PermissionFull:
		return "danger-full-access", "on-request", nil
	}
	return "", "", fmt.Errorf("a worker needs an authority (read-only, workdir-write or full); got %q", level)
}

// codexStartMode is the sandbox and approval policy a start opens with: a
// lead's permission mode, a worker's authority. A lead with no mode keeps
// Codex's own settings.
func codexStartMode(spec StartSpec) (sandbox, policy string, err error) {
	if spec.Lead == nil {
		return codexWorkerMode(spec.PermissionLevel)
	}
	if spec.Lead.PermissionMode == "" {
		return "", "", nil
	}
	return codexMode(spec.Lead.PermissionMode)
}

func codexInput(text string) []any {
	return []any{map[string]any{"type": "text", "text": text}}
}

func (p *codexProcess) Send(text string) error {
	session := p.m.session()
	if session == "" {
		return errors.New("the Codex thread is not open")
	}
	params := map[string]any{"threadId": session, "input": codexInput(text)}
	if p.model != "" {
		params["model"] = p.model
	}
	if p.effort != "" {
		params["effort"] = p.effort
	}
	if p.mode != "" {
		kind := map[string]string{
			"read-only": "readOnly", "workspace-write": "workspaceWrite", "danger-full-access": "dangerFullAccess",
		}[p.mode]
		params["sandboxPolicy"] = map[string]any{"type": kind}
		params["approvalPolicy"] = p.policy
	}
	_, err := p.call("turn", "turn/start", params)
	return err
}

func (p *codexProcess) Steer(text string) error {
	session, turn := p.m.active()
	if turn == "" {
		return ErrUnsupported
	}
	_, err := p.call("control", "turn/steer", map[string]any{
		"threadId": session, "expectedTurnId": turn, "input": codexInput(text),
	})
	return err
}

func (p *codexProcess) Interrupt() error {
	session, turn := p.m.active()
	if turn == "" {
		return nil
	}
	_, err := p.call("control", "turn/interrupt", map[string]any{"threadId": session, "turnId": turn})
	return err
}

func (p *codexProcess) SetModel(model string) error {
	if model == "" {
		return ErrUnsupported
	}
	p.model = model
	return nil
}

func (p *codexProcess) SetEffort(effort string) error {
	if effort == "" {
		return ErrUnsupported
	}
	p.effort = effort
	return nil
}

// A mode change applies at the next turn/start, like model and effort; it
// does not reinterpret an approval waiting in the current turn.
func (p *codexProcess) SetPermissionMode(mode string) error {
	if p.worker {
		return errors.New("a worker's sandbox follows its authority, set when it was spawned")
	}
	_, policy, err := codexMode(mode)
	if err != nil {
		return err
	}
	p.mode, p.policy = mode, policy
	return nil
}

func (p *codexProcess) AnswerApproval(request ApprovalRequest, allow bool, _ string) error {
	raw, result, err := p.m.approvalAnswer(request.ID, allow)
	if err != nil {
		return err
	}
	return p.writeLine(map[string]any{"id": raw, "result": result})
}

func (p *codexProcess) RunCommand(command wsinbox.Command) error {
	if command.Name != "compact" {
		return ErrUnsupported
	}
	_, err := p.call("compact", "thread/compact/start", map[string]any{"threadId": p.m.session()})
	return err
}

// TurnRunning reconciles a ring gap in stream order. The mapper emits the
// recovered final answer before TurnEnd. Returning true prevents the runner's
// separate probe callback from settling the turn ahead of those events.
func (p *codexProcess) TurnRunning() (bool, error) {
	_, err := p.call("recover", "thread/read", map[string]any{"threadId": p.m.session(), "includeTurns": true})
	return true, err
}
