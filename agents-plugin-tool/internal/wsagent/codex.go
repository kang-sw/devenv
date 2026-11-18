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
// It serves hosted leads only; its registration does not admit workers.
// The holder owns its pipes across daemon replacement, as for agy and Claude.
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

// codexChoices holds model/list from the last opened process. Merely opening
// the dashboard must not start a process or create a thread in Codex's home.
var codexChoices struct {
	sync.Mutex
	models  []ModelChoice
	efforts []string
}

func (CodexBackend) Choices(context.Context) ([]ModelChoice, []string, error) {
	codexChoices.Lock()
	defer codexChoices.Unlock()
	return slices.Clone(codexChoices.models), slices.Clone(codexChoices.efforts), nil
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
	if spec.Lead == nil || spec.Holder == nil {
		return nil, errors.New("codex backend: hosted lead sessions only")
	}
	attach := spec.Holder.id != ""
	if !attach && spec.MCPSession == nil {
		return nil, errors.New("codex backend: the daemon's HTTP MCP route is required; no lead was started without ws tools")
	}
	command, pre, err := harnessCommand(b.Command, "codex")
	if err != nil {
		return nil, fmt.Errorf("codex backend: %w", err)
	}
	cmd := exec.Command(command, append(pre, "app-server")...)
	cmd.Dir = spec.workDir()
	cmd.Env = codexEnv(spec)
	mapper := newCodexMapper(spec.SessionID)
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
		model: spec.Model, effort: spec.Effort, mode: spec.Lead.PermissionMode,
	}
	holder, pid, _ := stream.HolderInfo()
	if session := spec.MCPSession; session != nil && session.Started != nil {
		session.Started(cmd.Env, pid)
	}
	if attach {
		return p, nil
	}
	// MCP admission checks the hosted row's physical PID. Publish it before
	// thread/start connects, not after the handshake that waits on it.
	if spec.Lead.Spawned != nil {
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

	// The conversation loop owns these next-turn settings.
	model  string
	effort string
	mode   string
}

func (p *codexProcess) processControls() processControls {
	return processControls{Process: p, approval: p, mode: p, steer: p, commands: p, turns: p}
}

func (p *codexProcess) call(kind, method string, params map[string]any) (json.RawMessage, error) {
	id := "wsc/" + kind + "/" + p.epoch + "/" + strconv.FormatUint(p.seq.Add(1), 10)
	return p.requestBodyID(id, method, map[string]any{"method": method, "params": params}, codexRequestTimeout)
}

func (p *codexProcess) open(spec StartSpec) error {
	_, err := p.call("init", "initialize", map[string]any{
		"clientInfo":   map[string]any{"name": "ws", "version": spec.Lead.DaemonVersion},
		"capabilities": map[string]any{"experimentalApi": true},
	})
	if err != nil {
		return fmt.Errorf("initialize: %w", err)
	}
	if err := p.writeLine(map[string]any{"method": "initialized"}); err != nil {
		return err
	}

	// Shadow both distributed plugin server names, not the plugins: their
	// skills and user-controlled hooks stay loaded. No stdio ws is injected.
	server := map[string]any{
		"url": spec.MCPSession.URL, "bearer_token_env_var": EnvMCPSessionToken, "required": true,
	}
	params := map[string]any{
		"cwd": spec.workDir(),
		"config": map[string]any{
			"agents":      map[string]any{"enabled": false},
			"mcp_servers": map[string]any{"ws": server, "wsflow": server},
		},
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
	if mode := spec.Lead.PermissionMode; mode != "" {
		sandbox, policy, err := codexMode(mode)
		if err != nil {
			return err
		}
		params["sandbox"], params["approvalPolicy"] = sandbox, policy
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
		sandbox, policy, err := codexMode(p.mode)
		if err != nil {
			return err
		}
		kind := map[string]string{
			"read-only": "readOnly", "workspace-write": "workspaceWrite", "danger-full-access": "dangerFullAccess",
		}[sandbox]
		params["sandboxPolicy"] = map[string]any{"type": kind}
		params["approvalPolicy"] = policy
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
	if _, _, err := codexMode(mode); err != nil {
		return err
	}
	p.mode = mode
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
