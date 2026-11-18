package wsagent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kang-sw/devenv/internal/wslive"
)

// agy.go is the Antigravity backend: `agy acp`, the Agent Client Protocol
// (v1: JSON-RPC 2.0 over stdio, one message per line). Hosted leads run
// under a holder; workers are daemon-owned processes using the default
// agent with their instructions appended to its system prompt.
//
// The wire facts it relies on are pinned by agy_fixture_test.go against
// the recorded agy-<version>-*.jsonl fixtures:
//
//   - A lead's process runs under a session holder, which keeps agy's output
//     (never ws's input) in its ring. Every request ws writes therefore
//     carries an id that names its kind (agyID), so a later daemon replaying
//     the ring tells a prompt's response from a control's.
//   - A turn is a session/prompt. A prompt sent while one runs steers it:
//     agy answers the earlier one at once and the run belongs to the later
//     one, so the turn ends with the last outstanding prompt's response.
//   - Permission requests are agent requests whose JSON-RPC id is echoed
//     raw in the answer; the option is picked by kind, never by a fixed id.
//     Nothing withdraws a request on cancel, so the turn's end settles every
//     one still pending.
//   - ws tools arrive over the daemon's HTTP MCP route (mcp_session.go) with
//     the token in the session request, never in argv or the environment. A
//     refused token only drops the tools, with no error, so a start waits for
//     the token to reach the route and fails loudly when it does not.

// AgyBackend runs Antigravity's CLI as an ACP agent, one process per
// conversation (--process-per-session, which agy documents as required for
// HTTP MCP servers a client configures).
type AgyBackend struct {
	// Command is the agy executable; empty looks up "agy" on PATH.
	Command string
}

// AgyPermissionModes are the hosted lead choices. bypass is ws-side intent;
// the process still runs in agy's native default mode.
var AgyPermissionModes = []string{"default", "accept-edits", "auto", "plan", "bypass"}

// agyEfforts are the thought_level values agy 1.5.9 offers; Choices falls
// back to them before any session reported its own.
var agyEfforts = []string{"low", "medium", "high"}

// Timeouts of the requests a start and a control wait on. A session request
// may connect MCP servers before it answers.
var (
	agyInitTimeout    = 30 * time.Second
	agySessionTimeout = 2 * time.Minute
	agyControlTimeout = 30 * time.Second
	// agyMCPConnectWait bounds how long a start waits, after its session
	// opened, for agy to reach the ws MCP route with the session's token.
	// agy connects its MCP servers before it answers the session request
	// (1.5.10), so a live token has resolved by then; the wait only covers
	// the route's own lag.
	agyMCPConnectWait = 10 * time.Second
)

// agyDropEnv are the variable prefixes an agy lead starts without: the
// parent harness sessions' (leadDropEnv names ws's own).
var agyDropEnv = []string{"CLAUDE", "CODEX_", "PI_", "ORCA_"}

// agyRingSkip matches agy's thought chunks, which the mapper drops: a
// lead's holder keeps them out of its ring. A line whose shape this no
// longer matches is kept.
const agyRingSkip = `^\{"jsonrpc":"2\.0","method":"session/update","params":\{"sessionId":"[^"]*","update":\{"sessionUpdate":"agent_thought_chunk"`

// Executable is the agy command availability is checked against.
func (b AgyBackend) Executable() string {
	if b.Command != "" {
		return b.Command
	}
	return "agy"
}

// agyChoices holds the last session's model and thought-level report,
// which wins over the session-less `agy models` listing.
var agyChoices struct {
	mu      sync.Mutex
	models  []ModelChoice
	efforts []string
}

// Choices uses the last session's report, else a session-less listing.
func (b AgyBackend) Choices(ctx context.Context) ([]ModelChoice, []string, error) {
	return b.hostedChoices(ctx)
}

// agyEnv is a lead's environment: the daemon's without the parent harness
// sessions' variables and ws's own launcher state (leadDropEnv, which also
// drops the HTTP MCP credentials: agy takes them in its session request),
// plus the hosted-session variables.
func agyEnv(spec StartSpec) []string {
	set := map[string]string{}
	if spec.Lead != nil {
		for name, value := range spec.Lead.Env {
			set[name] = value
		}
	}
	return harnessEnv(spec.Env, append(slices.Clone(agyDropEnv), leadDropEnv...), nil, set)
}

// Start launches `agy acp` for spec and, unless it rejoins a held process,
// opens the session before it returns: initialize, then session/new or
// session/resume with the ws MCP server, then the conversation's model,
// effort and mode.
func (b AgyBackend) Start(ctx context.Context, spec StartSpec) (Process, error) {
	if spec.Lead != nil && spec.Holder == nil {
		return nil, errors.New("agy backend: a hosted lead requires a session holder")
	}
	if spec.Lead == nil && spec.Holder != nil {
		return nil, errors.New("agy backend: workers are not holder-held")
	}
	attach := spec.Holder != nil && spec.Holder.id != ""
	if !attach && spec.MCPSession == nil {
		return nil, errors.New("agy backend: agy reaches ws tools only over the daemon's HTTP MCP route, and none is available; no process was started without ws tools")
	}
	command, pre, err := harnessCommand(b.Command, "agy")
	if err != nil {
		return nil, fmt.Errorf("agy backend: %w", err)
	}
	// Both roles use the user's agy home and default agent. A rejoin
	// attaches to the running process; startChild ignores cmd.
	cmd := exec.Command(command, append(pre, "acp", "--process-per-session")...)
	cmd.Dir = spec.workDir()
	cmd.Env = agyEnv(spec)
	if spec.Holder != nil {
		spec.Holder.ringSkip = agyRingSkip
	}
	m := newAgyMapper(spec.SessionID)
	sp, err := startChild(spec, cmd, m, agyProtocol{})
	if err != nil {
		return nil, err
	}
	p := &agyProcess{streamProcess: sp, m: m, worker: spec.Lead == nil, epoch: strconv.FormatInt(time.Now().UnixNano(), 36)}
	holder, pid, _ := sp.HolderInfo()
	if cmd.Process != nil {
		pid = cmd.Process.Pid
	}
	if s := spec.MCPSession; s != nil && s.Started != nil {
		s.Started(cmd.Env, pid)
	}
	if attach {
		return p, nil
	}
	// agy connects ws-mcp inside its session request. A lead's row must
	// name this process first; a worker has no hosted-lead registration.
	if spec.Lead != nil && spec.Lead.Spawned != nil {
		spec.Lead.Spawned(holder, pid)
	}
	if err := p.open(spec); err != nil {
		_ = p.Close()
		return nil, fmt.Errorf("agy backend: %w", err)
	}
	return p, nil
}

// agyProtocol writes ACP requests. A user turn needs the session id, so
// agyProcess.Send writes it; userLine is never used.
type agyProtocol struct{}

func (agyProtocol) userLine(string) any { return nil }

func (agyProtocol) requestLine(id string, request map[string]any) any {
	return map[string]any{"jsonrpc": "2.0", "id": id, "method": request["method"], "params": request["params"]}
}

// Request kinds, the second field of agyID.
const (
	agyKindInit    = "init"
	agyKindNew     = "new"
	agyKindResume  = "resume"
	agyKindConfig  = "cfg"
	agyKindMode    = "mode"
	agyKindPrompt  = "p"
	agyRequestMark = "ws"
)

// agyIDKind is the kind of a request id ws wrote ("ws-<kind>-<epoch>-<n>"),
// false for any other id.
func agyIDKind(id string) (string, bool) {
	parts := strings.Split(id, "-")
	if len(parts) != 4 || parts[0] != agyRequestMark {
		return "", false
	}
	return parts[1], true
}

// agyProcess is one `agy acp` process serving one session.
type agyProcess struct {
	*streamProcess
	m *agyMapper
	// epoch keeps this process's request ids apart from an earlier
	// daemon's on the same held process.
	epoch string
	seq   atomic.Int64
	// effort is the thought level the conversation asked for last.
	effort atomic.Pointer[string]
	model  atomic.Pointer[string]
	worker bool
}

func (p *agyProcess) processControls() processControls {
	return processControls{Process: p, approval: p, mode: p, steer: p}
}

func (p *agyProcess) newID(kind string) string {
	return agyRequestMark + "-" + kind + "-" + p.epoch + "-" + strconv.FormatInt(p.seq.Add(1), 10)
}

// call sends one request and waits for its result.
func (p *agyProcess) call(kind, method string, params map[string]any, timeout time.Duration) (json.RawMessage, error) {
	return p.requestBodyID(p.newID(kind), method, map[string]any{"method": method, "params": params}, timeout)
}

// open runs a new process's handshake.
func (p *agyProcess) open(spec StartSpec) error {
	version := ""
	if spec.Lead != nil {
		version = spec.Lead.DaemonVersion
	}
	init, err := p.call(agyKindInit, "initialize", map[string]any{
		"protocolVersion": 1,
		// Without fs or terminal capabilities agy runs its own tools.
		"clientCapabilities": map[string]any{"fs": map[string]any{"readTextFile": false, "writeTextFile": false}, "terminal": false},
		"clientInfo":         map[string]any{"name": "ws", "title": "ws", "version": version},
	}, agyInitTimeout)
	if err != nil {
		return fmt.Errorf("initialize: %w", err)
	}
	var reply struct {
		ProtocolVersion int `json:"protocolVersion"`
	}
	if json.Unmarshal(init, &reply) != nil || reply.ProtocolVersion != 1 {
		return fmt.Errorf("agy answered initialize with ACP protocol version %d; ws speaks version 1", reply.ProtocolVersion)
	}
	params := map[string]any{"cwd": spec.workDir(), "mcpServers": []any{agyHTTPServer(spec.MCPSession)}}
	if spec.Instructions != "" {
		text, err := os.ReadFile(spec.Instructions)
		if err != nil {
			return fmt.Errorf("read the instruction file: %w", err)
		}
		params["_meta"] = map[string]any{"antigravity": map[string]any{"systemPrompt": map[string]any{"append": string(text)}}}
	}
	kind, method := agyKindNew, "session/new"
	if spec.SessionID != "" {
		kind, method = agyKindResume, "session/resume"
		params["sessionId"] = spec.SessionID
	}
	if _, err := p.call(kind, method, params, agySessionTimeout); err != nil {
		return fmt.Errorf("%s: %w", method, err)
	}
	if c := spec.MCPSession.Connected; c != nil {
		select {
		case <-c:
		case <-p.exited:
			return errors.New("agy exited while it opened its session")
		case <-time.After(agyMCPConnectWait):
			return fmt.Errorf("agy did not reach the ws MCP route within %s of opening its session (a refused token only hides the ws tools), so the process was not started without them", agyMCPConnectWait)
		}
	}
	if spec.Effort != "" {
		p.effort.Store(&spec.Effort)
	}
	if spec.Model != "" {
		if err := p.SetModel(spec.Model); err != nil {
			return fmt.Errorf("model: %w", err)
		}
	}
	if spec.Model == "" && spec.Effort != "" {
		if err := p.SetEffort(spec.Effort); err != nil {
			return fmt.Errorf("effort: %w", err)
		}
	}
	if spec.Lead != nil {
		if mode := spec.Lead.PermissionMode; mode != "" && mode != p.m.mode() {
			if err := p.SetPermissionMode(mode); err != nil {
				return fmt.Errorf("permission mode: %w", err)
			}
		}
	}
	return nil
}

// agyHTTPServer is the ws MCP server entry of a session request: the
// daemon's route with the session's bearer token.
func agyHTTPServer(s *MCPSession) map[string]any {
	return map[string]any{
		"type":    "http",
		"name":    ReservedMCPServer,
		"url":     s.URL,
		"headers": []any{map[string]any{"name": "Authorization", "value": "Bearer " + s.Token}},
	}
}

// Send submits a user turn as a session/prompt; sent while a turn runs it
// steers that turn.
func (p *agyProcess) Send(text string) error {
	sid := p.m.session()
	if sid == "" {
		return errors.New("the agy session is not open")
	}
	id := p.newID(agyKindPrompt)
	p.m.promptSent(id)
	err := p.writeLine(map[string]any{"jsonrpc": "2.0", "id": id, "method": "session/prompt", "params": map[string]any{
		"sessionId": sid,
		"prompt":    []any{map[string]any{"type": "text", "text": text}},
	}})
	if err != nil {
		p.m.promptDropped(id)
	}
	return err
}

// Steer writes text as another prompt: agy ends the running prompt and
// carries its run on under this one (seen on 1.5.9).
func (p *agyProcess) Steer(text string) error { return p.Send(text) }

// Interrupt cancels the running turn; its prompt answers "cancelled".
func (p *agyProcess) Interrupt() error {
	return p.writeLine(map[string]any{"jsonrpc": "2.0", "method": "session/cancel", "params": map[string]any{"sessionId": p.m.session()}})
}

// SetModel sets the session's model config option. ACP has no value for
// "the default", so an empty model restarts the process instead. A model
// that offers a thought level gets the conversation's again: agy resets it
// on a switch.
func (p *agyProcess) SetModel(model string) error {
	if model == "" {
		return ErrUnsupported
	}
	base, suffix := agyBaseModel(model)
	if suffix != "" && p.effort.Load() == nil {
		p.effort.Store(&suffix)
	}
	if !p.m.offersValue("model", base) {
		if p.effort.Load() == nil {
			if level := p.m.defaultModelEffort(base); level != "" {
				p.effort.Store(&level)
			}
		}
		if effort := p.effort.Load(); effort != nil {
			if ok, err := p.setModelVariant(base, *effort); ok || err != nil {
				if err == nil {
					p.model.Store(&base)
				}
				return err
			}
		}
	}
	if err := p.setConfig("model", base); err != nil {
		return err
	}
	p.model.Store(&base)
	if effort := p.effort.Load(); effort != nil && *effort != "" {
		return p.SetEffort(*effort)
	}
	return nil
}

// SetEffort uses thought_level when offered, otherwise a suffixed model id.
// A worker fails rather than silently running at another tier's effort. A
// lead keeps an unavailable effort for its next model, as before.
func (p *agyProcess) SetEffort(effort string) error {
	if effort == "" {
		return ErrUnsupported
	}
	p.effort.Store(&effort)
	if p.m.offersValue("thought_level", effort) {
		if err := p.setConfig("thought_level", effort); err != nil {
			return err
		}
		p.emit(Event{Notice: "agy honored effort " + effort + " via thought_level"})
		return nil
	}
	model := p.m.modelName()
	if requested := p.model.Load(); requested != nil {
		model = *requested
	}
	base, _ := agyBaseModel(model)
	if ok, err := p.setModelVariant(base, effort); ok || err != nil {
		return err
	}
	if p.worker {
		return fmt.Errorf("agy offers neither thought_level %q nor model variant %q", effort, base+"-"+effort)
	}
	p.emit(Event{Notice: "agy did not offer effort " + effort + "; retained for the next model"})
	return nil
}

// agyBaseModel separates the thinking axis from agy's model vocabulary.
func agyBaseModel(model string) (string, string) {
	for _, effort := range []string{"xhigh", "high", "medium", "low"} {
		if base, ok := strings.CutSuffix(model, "-"+effort); ok {
			return base, effort
		}
	}
	return model, ""
}

func (p *agyProcess) setModelVariant(base, effort string) (bool, error) {
	variant := base + "-" + effort
	if p.m.offersValue("model", variant) {
		if err := p.setConfig("model", variant); err != nil {
			return true, err
		}
	} else if p.m.hasModel(variant) {
		body, err := p.call(agyKindConfig, "session/set_model", map[string]any{"sessionId": p.m.session(), "modelId": variant}, agyControlTimeout)
		if err != nil {
			return true, err
		}
		var reply struct {
			Models struct {
				CurrentModelID string `json:"currentModelId"`
			} `json:"models"`
		}
		_ = json.Unmarshal(body, &reply)
		if got := reply.Models.CurrentModelID; got != "" && got != variant {
			return true, fmt.Errorf("agy kept model %q after it was set to %q", got, variant)
		}
	} else {
		return false, nil
	}
	p.emit(Event{Notice: "agy honored effort " + effort + " via suffixed model id " + variant})
	return true, nil
}

// setConfig sets one config option and checks that agy took the value: a
// value agy does not apply comes back unchanged rather than refused.
func (p *agyProcess) setConfig(option, value string) error {
	body, err := p.call(agyKindConfig, "session/set_config_option", map[string]any{"sessionId": p.m.session(), "configId": option, "value": value}, agyControlTimeout)
	if err != nil {
		return err
	}
	if got, ok := agyConfigValue(body, option); ok && got != value {
		return fmt.Errorf("agy kept %s %q after it was set to %q", option, got, value)
	}
	return nil
}

// SetPermissionMode sets the session's mode.
func (p *agyProcess) SetPermissionMode(mode string) error {
	if mode == "bypass" || !slices.Contains(AgyPermissionModes, mode) {
		return fmt.Errorf("agy has no permission mode %q; its modes are %s", mode, strings.Join(AgyPermissionModes, ", "))
	}
	_, err := p.call(agyKindMode, "session/set_mode", map[string]any{"sessionId": p.m.session(), "modeId": mode}, agyControlTimeout)
	return err
}

// AnswerApproval answers a permission request with the option of the
// matching kind. ACP carries no denial message.
func (p *agyProcess) AnswerApproval(req ApprovalRequest, allow bool, _ string) error {
	raw, option, err := p.m.answerFor(req.ID, allow)
	if err != nil {
		return err
	}
	return p.writeLine(map[string]any{"jsonrpc": "2.0", "id": raw, "result": map[string]any{
		"outcome": map[string]any{"outcome": "selected", "optionId": option},
	}})
}

// agyConfigValue is option's currentValue in a configOptions answer.
func agyConfigValue(body json.RawMessage, option string) (string, bool) {
	var r struct {
		ConfigOptions []agyConfigOption `json:"configOptions"`
	}
	if json.Unmarshal(body, &r) != nil {
		return "", false
	}
	for _, o := range r.ConfigOptions {
		if o.ID == option {
			return o.CurrentValue, true
		}
	}
	return "", false
}

type agyConfigOption struct {
	ID           string `json:"id"`
	CurrentValue string `json:"currentValue"`
	Options      []struct {
		Value string `json:"value"`
	} `json:"options"`
}

// agyApproval is a pending permission request: the raw id its answer
// echoes and the options it offered.
type agyApproval struct {
	raw        json.RawMessage
	toolCallID string
	options    []agyOption
}

type agyOption struct {
	OptionID string `json:"optionId"`
	Kind     string `json:"kind"`
}

// agyMapper maps agy's output lines. Its state is shared with the process's
// writers (Send registers its prompt before writing it), so it has a lock.
type agyMapper struct {
	mu sync.Mutex

	sessionID string
	curMode   string
	version   string
	// options are the config options the session offers now, by id, with
	// the values each takes.
	options map[string][]string
	// models are the ACP variant ids; model is the current config value.
	models []string
	model  string
	// initDemoted and sessionDemoted are the items the initialize answer
	// and the session's own reports show a mismatched shape for.
	initDemoted    []CapabilityItem
	sessionDemoted []CapabilityItem

	// outstanding are the prompts this process sent and agy has not
	// answered; inTurn is set from a turn's first content to its end.
	outstanding map[string]bool
	inTurn      bool
	turnSeq     int
	group       int
	groupCalls  bool
	reply       strings.Builder
	replyAt     int64
	batches     batchTracker

	// approvals are the pending permission requests by ApprovalRequest.ID,
	// approvalOf their ids by tool call.
	approvals  map[string]*agyApproval
	approvalOf map[string]string
}

func newAgyMapper(sessionID string) *agyMapper {
	return &agyMapper{
		sessionID:   sessionID,
		outstanding: map[string]bool{},
		batches:     newBatchTracker("agy"),
		approvals:   map[string]*agyApproval{},
		approvalOf:  map[string]string{},
	}
}

func (m *agyMapper) session() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.sessionID
}

// offers reports whether the session offers config option id now; before
// a session reported its options, it does.
func (m *agyMapper) offers(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.options[id]
	return m.options == nil || ok
}

// offersValue reports whether option id takes value now; before a session
// reported its options, it does.
func (m *agyMapper) offersValue(id, value string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.options == nil || slices.Contains(m.options[id], value)
}

func (m *agyMapper) modelName() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.model
}

// defaultModelEffort selects an offered variant when the caller chose a
// base model from Choices without an explicit effort. Prefer the current
// variant, otherwise the harness's own offered order.
func (m *agyMapper) defaultModelEffort(base string) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, model := range append(append([]string{m.model}, m.options["model"]...), m.models...) {
		if name, effort := agyBaseModel(model); name == base && effort != "" {
			return effort
		}
	}
	return ""
}

func (m *agyMapper) hasModel(model string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return slices.Contains(m.models, model)
}

func (m *agyMapper) mode() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.curMode
}

func (m *agyMapper) promptSent(id string) {
	m.mu.Lock()
	m.outstanding[id] = true
	m.mu.Unlock()
}

func (m *agyMapper) promptDropped(id string) {
	m.mu.Lock()
	delete(m.outstanding, id)
	m.mu.Unlock()
}

// answerFor is the raw request id and option id that answer approval id.
func (m *agyMapper) answerFor(id string, allow bool) (json.RawMessage, string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	a := m.approvals[id]
	if a == nil {
		return nil, "", fmt.Errorf("agy has no pending permission request %q", id)
	}
	kinds := []string{"reject_once", "reject_always"}
	if allow {
		kinds = []string{"allow_once", "allow_always"}
	}
	for _, kind := range kinds {
		for _, o := range a.options {
			if o.Kind == kind {
				return a.raw, o.OptionID, nil
			}
		}
	}
	return nil, "", fmt.Errorf("agy's permission request %q offers no %s option", id, kinds[0])
}

type agyMessage struct {
	ID     json.RawMessage `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func (m *agyMapper) mapLine(line []byte, replies lineReplies) []Event {
	var msg agyMessage
	if json.Unmarshal(line, &msg) != nil {
		return nil
	}
	now := time.Now().UnixMilli()
	m.mu.Lock()
	defer m.mu.Unlock()
	hasID := len(msg.ID) > 0 && string(msg.ID) != "null"
	switch {
	case msg.Method != "" && hasID:
		return m.request(msg, replies, now)
	case msg.Method == "session/update":
		return m.update(msg.Params, now)
	case msg.Method == "" && hasID:
		return m.response(msg, replies, now)
	}
	return nil
}

// request maps an agent request: a permission request waits for the
// user; any other gets an error answer, so agy never waits on it.
func (m *agyMapper) request(msg agyMessage, replies lineReplies, now int64) []Event {
	if msg.Method != "session/request_permission" {
		replies.send(map[string]any{"jsonrpc": "2.0", "id": msg.ID, "error": map[string]any{"code": -32601, "message": "method not found: " + msg.Method}})
		return []Event{{Notice: "agy asked for " + msg.Method + ", which ws does not serve"}}
	}
	var p struct {
		ToolCall struct {
			ToolCallID string          `json:"toolCallId"`
			Name       string          `json:"name"`
			Title      string          `json:"title"`
			Args       json.RawMessage `json:"args"`
			RawInput   json.RawMessage `json:"rawInput"`
		} `json:"toolCall"`
		Options []agyOption `json:"options"`
	}
	_ = json.Unmarshal(msg.Params, &p)
	raw := compactRaw(msg.ID)
	id := p.ToolCall.ToolCallID
	if id == "" {
		id = "rpc:" + string(raw)
	}
	out := m.startTurn(nil)
	m.approvals[id] = &agyApproval{raw: raw, toolCallID: p.ToolCall.ToolCallID, options: p.Options}
	if p.ToolCall.ToolCallID != "" {
		m.approvalOf[p.ToolCall.ToolCallID] = id
	}
	input := p.ToolCall.Args
	if len(input) == 0 {
		input = p.ToolCall.RawInput
	}
	tool := p.ToolCall.Name
	if tool == "" {
		tool = p.ToolCall.Title
	}
	return append(out, Event{Approval: &ApprovalRequest{ID: id, Tool: tool, Input: input, Description: p.ToolCall.Title, ToolUseID: p.ToolCall.ToolCallID, StartedAt: now}})
}

func compactRaw(raw json.RawMessage) json.RawMessage {
	var b bytes.Buffer
	if json.Compact(&b, raw) != nil {
		return slices.Clone(raw)
	}
	return b.Bytes()
}

// response maps the answer to a request ws wrote: a prompt's ends the turn
// unless a later prompt took the run over; any other answers its waiter
// and reports what the handshake learned.
func (m *agyMapper) response(msg agyMessage, replies lineReplies, now int64) []Event {
	var id string
	if json.Unmarshal(msg.ID, &id) != nil {
		return nil
	}
	kind, ok := agyIDKind(id)
	if !ok {
		return nil
	}
	if kind == agyKindPrompt {
		ours := m.outstanding[id]
		delete(m.outstanding, id)
		if len(m.outstanding) > 0 || (!ours && !m.inTurn) {
			return nil
		}
		return m.endTurn(msg, now)
	}
	a := controlAnswer{ok: msg.Error == nil, body: msg.Result}
	if msg.Error != nil {
		a.err = msg.Error.Message
	}
	replies.answer(id, a)
	if msg.Error != nil {
		return nil
	}
	switch kind {
	case agyKindInit:
		return m.initialized(msg.Result)
	case agyKindNew, agyKindResume:
		return m.opened(msg.Result)
	case agyKindConfig:
		m.configOptions(msg.Result)
	}
	return nil
}

// initialized reads the harness version and the init shape.
//
// Init-field to item mapping that gates demotion (the catalog's
// HarnessReport):
//
//	agentInfo.version              -> the version the profile resolves at
//	sessionCapabilities.resume     -> resume: absent
//	(session answer) configOptions -> model/effort: no model or thought_level option
//	(session answer) modes         -> approval: no available modes
//	available_commands_update      -> compaction: a list without "compact"
//
// Steer, interrupt, follow-up, usage and the other items have no shape to
// check: their grade follows the version alone.
func (m *agyMapper) initialized(result json.RawMessage) []Event {
	var r struct {
		AgentInfo struct {
			Version string `json:"version"`
		} `json:"agentInfo"`
		AgentCapabilities struct {
			SessionCapabilities map[string]json.RawMessage `json:"sessionCapabilities"`
		} `json:"agentCapabilities"`
	}
	_ = json.Unmarshal(result, &r)
	m.version = r.AgentInfo.Version
	m.initDemoted = nil
	if _, ok := r.AgentCapabilities.SessionCapabilities["resume"]; !ok {
		m.initDemoted = append(m.initDemoted, ItemResume)
	}
	return []Event{m.harnessReport()}
}

func (m *agyMapper) harnessReport() Event {
	demoted := slices.Clone(m.initDemoted)
	for _, item := range m.sessionDemoted {
		if !slices.Contains(demoted, item) {
			demoted = append(demoted, item)
		}
	}
	return Event{Harness: &HarnessReport{Version: m.version, Demoted: demoted}}
}

// opened reads a session/new or session/resume answer: the session id,
// the mode, and the session shape.
func (m *agyMapper) opened(result json.RawMessage) []Event {
	var r struct {
		SessionID string `json:"sessionId"`
		Models    struct {
			CurrentModelID  string `json:"currentModelId"`
			AvailableModels []struct {
				ModelID string `json:"modelId"`
			} `json:"availableModels"`
		} `json:"models"`
		Modes *struct {
			CurrentModeID  string            `json:"currentModeId"`
			AvailableModes []json.RawMessage `json:"availableModes"`
		} `json:"modes"`
		ConfigOptions []agyConfigOption `json:"configOptions"`
	}
	if json.Unmarshal(result, &r) != nil {
		return nil
	}
	var out []Event
	if r.SessionID != "" {
		m.sessionID = r.SessionID
		out = append(out, Event{SessionID: r.SessionID})
	}
	m.models = nil
	for _, model := range r.Models.AvailableModels {
		m.models = append(m.models, model.ModelID)
	}
	m.model = r.Models.CurrentModelID
	m.configOptions(result)
	// The thought level is per model (agy drops the option for a model
	// without one), so only a missing model option is a shape mismatch.
	m.sessionDemoted = slices.DeleteFunc(m.sessionDemoted, func(item CapabilityItem) bool {
		return item == ItemModelEffort || item == ItemApproval
	})
	if _, ok := m.options["model"]; !ok {
		m.sessionDemoted = append(m.sessionDemoted, ItemModelEffort)
	}
	if r.Modes == nil || len(r.Modes.AvailableModes) == 0 {
		m.sessionDemoted = append(m.sessionDemoted, ItemApproval)
	} else if r.Modes.CurrentModeID != "" {
		m.curMode = r.Modes.CurrentModeID
		out = append(out, Event{PermissionMode: r.Modes.CurrentModeID})
	}
	return append(out, m.harnessReport())
}

// configOptions keeps the model and thought-level values a session
// reports for Choices.
func (m *agyMapper) configOptions(body json.RawMessage) {
	var r struct {
		ConfigOptions []agyConfigOption `json:"configOptions"`
	}
	if json.Unmarshal(body, &r) != nil || r.ConfigOptions == nil {
		return
	}
	m.options = map[string][]string{}
	for _, o := range r.ConfigOptions {
		values := []string{}
		for _, v := range o.Options {
			values = append(values, v.Value)
		}
		m.options[o.ID] = values
	}
	var models []ModelChoice
	var efforts []string
	seen := map[string]bool{}
	for _, o := range r.ConfigOptions {
		if o.ID == "model" {
			m.model = o.CurrentValue
		}
		for _, v := range o.Options {
			switch o.ID {
			case "model":
				base, _ := agyBaseModel(v.Value)
				if !seen[base] {
					models = append(models, ModelChoice{ID: base})
					seen[base] = true
				}
			case "thought_level":
				efforts = append(efforts, v.Value)
			}
		}
	}
	if len(models) == 0 && len(efforts) == 0 {
		return
	}
	agyChoices.mu.Lock()
	defer agyChoices.mu.Unlock()
	if len(models) > 0 {
		agyChoices.models = models
	}
	if len(efforts) > 0 {
		agyChoices.efforts = efforts
	}
}

// startTurn opens a turn at its first content: a turn ws's own prompt
// began is already running in the runner, and a replayed one is rebuilt
// here.
func (m *agyMapper) startTurn(out []Event) []Event {
	if m.inTurn {
		return out
	}
	m.inTurn = true
	m.turnSeq++
	m.group, m.groupCalls = 0, false
	return append(out, Event{TurnStart: true})
}

// flushReply ends the reply the turn streams so far.
func (m *agyMapper) flushReply(out []Event) []Event {
	text := m.reply.String()
	m.reply.Reset()
	m.replyAt = 0
	if strings.TrimSpace(text) == "" {
		return out
	}
	return append(out, Event{Live: &wslive.Event{Kind: wslive.KindReply, Text: text, Final: true}})
}

func (m *agyMapper) groupID() string {
	return "t" + strconv.Itoa(m.turnSeq) + "g" + strconv.Itoa(m.group)
}

// endTurn settles a turn at its last prompt's answer: the reply, every
// permission request still pending, and the open tool batch end with it.
func (m *agyMapper) endTurn(msg agyMessage, now int64) []Event {
	out := m.flushReply(nil)
	for id := range m.approvals {
		out = append(out, Event{ApprovalDone: id})
	}
	clear(m.approvals)
	clear(m.approvalOf)
	if b := m.batches.finish(now, true); b != nil {
		out = append(out, *b)
	}
	m.inTurn = false
	ev := Event{TurnEnd: true}
	var r struct {
		StopReason string `json:"stopReason"`
	}
	_ = json.Unmarshal(msg.Result, &r)
	switch {
	case msg.Error != nil:
		ev.TurnError = "agy: " + msg.Error.Message
	case r.StopReason == "cancelled":
		ev.TurnReason = wslive.TurnAborted
	case r.StopReason != "" && r.StopReason != "end_turn":
		ev.TurnError = "agy stopped the turn: " + r.StopReason
	}
	return append(out, ev)
}

// agyMCPToolName is a tool's name under agy's MCP naming, which keeps
// the tool's own name whole: the tool "tracker.update" of server "ws" is
// mcp_ws_tracker.update (seen on 1.5.10).
func agyMCPToolName(server, tool string) string {
	return "mcp_" + server + "_" + tool
}

type agyUpdate struct {
	SessionUpdate string `json:"sessionUpdate"`
	// Content is a message chunk's content block, but a tool call
	// update's content list: decoding it as either would fail the other.
	Content           json.RawMessage   `json:"content"`
	ToolCallID        string            `json:"toolCallId"`
	Name              string            `json:"name"`
	Title             string            `json:"title"`
	Status            string            `json:"status"`
	Args              json.RawMessage   `json:"args"`
	Used              *int64            `json:"used"`
	Size              int64             `json:"size"`
	CurrentModeID     string            `json:"currentModeId"`
	AvailableCommands []json.RawMessage `json:"availableCommands"`
}

// update maps a session/update notification.
func (m *agyMapper) update(params json.RawMessage, now int64) []Event {
	var p struct {
		Update agyUpdate `json:"update"`
	}
	if json.Unmarshal(params, &p) != nil {
		return nil
	}
	u := p.Update
	switch u.SessionUpdate {
	case "agent_message_chunk":
		out := m.startTurn(nil)
		var c struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}
		if json.Unmarshal(u.Content, &c) != nil || c.Type != "text" || c.Text == "" {
			return out
		}
		if m.groupCalls {
			// Text after a tool batch is the next message's.
			m.group++
			m.groupCalls = false
		}
		m.reply.WriteString(c.Text)
		if m.replyAt != 0 && now-m.replyAt < claudePartialEvery {
			return out
		}
		m.replyAt = now
		if strings.TrimSpace(m.reply.String()) == "" {
			return out
		}
		return append(out, Event{Live: &wslive.Event{Kind: wslive.KindReply, Text: m.reply.String()}})
	case "agent_thought_chunk":
		return m.startTurn(nil)
	case "tool_call":
		out := m.flushReply(m.startTurn(nil))
		name := u.Name
		if name == "" {
			name = u.Title
		}
		out = append(out, m.batches.addCall(m.groupID(), u.ToolCallID, name, argsPreview(u.Args), now)...)
		m.groupCalls = true
		return append(out, m.toolStatus(u, now)...)
	case "tool_call_update":
		return m.toolStatus(u, now)
	case "usage_update":
		if u.Used == nil {
			return nil
		}
		used := *u.Used
		ctx := &wslive.UsageContext{Tokens: &used, Window: u.Size}
		if u.Size > 0 {
			pct := float64(used) * 100 / float64(u.Size)
			ctx.Percent = &pct
		}
		return []Event{{Live: &wslive.Event{Kind: wslive.KindStatus, Context: ctx}}}
	case "available_commands_update":
		return m.commands(u.AvailableCommands)
	case "current_mode_update":
		if u.CurrentModeID == "" {
			return nil
		}
		m.curMode = u.CurrentModeID
		return []Event{{PermissionMode: u.CurrentModeID}}
	case "config_option_update":
		m.configOptions(params2configOptions(params))
	}
	return nil
}

// params2configOptions lifts a config_option_update's configOptions to the
// shape of a set_config_option answer.
func params2configOptions(params json.RawMessage) json.RawMessage {
	var p struct {
		Update json.RawMessage `json:"update"`
	}
	if json.Unmarshal(params, &p) != nil {
		return nil
	}
	return p.Update
}

// toolStatus ends a tool call that reached a final status: its batch
// entry, its permission request, and a tracker write it made.
func (m *agyMapper) toolStatus(u agyUpdate, now int64) []Event {
	if u.Status != "completed" && u.Status != "failed" {
		return nil
	}
	failed := u.Status == "failed"
	out := m.batches.endCall(u.ToolCallID, failed, now)
	if id, ok := m.approvalOf[u.ToolCallID]; ok {
		delete(m.approvalOf, u.ToolCallID)
		delete(m.approvals, id)
		out = append(out, Event{ApprovalDone: id})
	}
	if !failed && u.Name == agyMCPToolName(ReservedMCPServer, TrackerWriteTool) {
		out = append(out, resultEvents(TrackerWriteTool, "")...)
	}
	return out
}

// commands maps agy's command list (its slash commands and skills); a list
// without "compact" demotes the compaction item.
func (m *agyMapper) commands(raw []json.RawMessage) []Event {
	list := make([]wslive.SlashCommand, 0, len(raw))
	compact := false
	for _, r := range raw {
		var c struct {
			Name        string `json:"name"`
			Description string `json:"description"`
		}
		if json.Unmarshal(r, &c) != nil || c.Name == "" {
			continue
		}
		compact = compact || c.Name == "compact"
		list = append(list, wslive.SlashCommand{Name: c.Name, Description: c.Description})
	}
	out := []Event{{Live: &wslive.Event{Kind: wslive.KindCommands, Commands: list}}}
	had := slices.Contains(m.sessionDemoted, ItemCompaction)
	if compact == had {
		if compact {
			m.sessionDemoted = slices.DeleteFunc(m.sessionDemoted, func(item CapabilityItem) bool { return item == ItemCompaction })
		} else {
			m.sessionDemoted = append(m.sessionDemoted, ItemCompaction)
		}
		out = append([]Event{m.harnessReport()}, out...)
	}
	return out
}
