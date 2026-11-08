package wsagent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kang-sw/devenv/internal/wslive"
)

// PiBackend runs Pi in RPC mode, one process per conversation, on a
// session file kept in the conversation's session directory, so a restart
// resumes the same Pi session. It keeps Pi's own system prompt and appends
// the composed instruction file; ambient extensions, skills, prompt
// templates, context files, and trust-gated project configuration stay
// off, and --tools is the whole tool surface.
type PiBackend struct {
	// Command is the pi executable; empty looks up "pi" on PATH.
	Command string
}

// piBridgeFile is the extension the runner writes into a conversation's Pi
// directory when the profile allows MCP servers (ws-mcp included): it
// registers exactly those servers through Pi's built-in MCP support.
const piBridgeFile = "ws-agent-mcp.ts"

// piDir is where a conversation's Pi session lives.
func piDir(dir string) string { return filepath.Join(dir, "pi") }

func piSessionFile(dir string) string { return filepath.Join(piDir(dir), "session.jsonl") }

var piNameUnsafe = regexp.MustCompile(`[^A-Za-z0-9_]`)

// piMCPToolName is Pi's name for an MCP server's tool: mcp__<server>__<tool>
// with every character outside [A-Za-z0-9_] as an underscore.
func piMCPToolName(server, tool string) string {
	return "mcp__" + piNameUnsafe.ReplaceAllString(server, "_") + "__" + piNameUnsafe.ReplaceAllString(tool, "_")
}

// piSurface is the tool surface a Pi child may declare: the --tools entries,
// and the names and MCP-server prefixes the tool check accepts.
type piSurface struct {
	entries  []string
	exact    map[string]bool
	prefixes []string
}

func piToolSurface(spec StartSpec) piSurface {
	s := piSurface{exact: map[string]bool{}}
	add := func(name string) {
		if !s.exact[name] {
			s.exact[name] = true
			s.entries = append(s.entries, name)
		}
	}
	for _, entry := range spec.Profile.Tools {
		add(ToolBase(entry))
	}
	if spec.WSMCP != nil {
		for _, name := range spec.Profile.WSTools {
			add(piMCPToolName(ReservedMCPServer, name))
		}
	}
	for _, name := range spec.Profile.MCPServerNames() {
		prefix := "mcp__" + piNameUnsafe.ReplaceAllString(name, "_") + "__"
		s.prefixes = append(s.prefixes, prefix)
		s.entries = append(s.entries, prefix+"*")
	}
	return s
}

func (s piSurface) allows(tool string) bool {
	if s.exact[tool] {
		return true
	}
	for _, prefix := range s.prefixes {
		if strings.HasPrefix(tool, prefix) {
			return true
		}
	}
	return false
}

// checkPiTools refuses a tool loadout that declares a tool outside the
// profile. Pi reports its loadout in system messages: the first request's
// carries every declared tool, later ones the tools added since.
func checkPiTools(added []string, s piSurface) error {
	var extra []string
	for _, name := range added {
		if !s.allows(name) {
			extra = append(extra, name)
		}
	}
	if len(extra) == 0 {
		return nil
	}
	sort.Strings(extra)
	return fmt.Errorf("launch refused: pi reports tools outside the profile: %s", strings.Join(extra, ", "))
}

// piMCPServers are the MCP server registrations for spec: the profile's own
// servers (direct exposure unless the server object sets one) and ws-mcp.
func piMCPServers(spec StartSpec) (map[string]any, error) {
	servers := map[string]any{}
	for name, raw := range spec.Profile.MCPServers {
		var obj map[string]any
		if err := json.Unmarshal(raw, &obj); err != nil || obj == nil {
			return nil, fmt.Errorf("agent profile: mcp server %q is not an object", name)
		}
		if _, ok := obj["exposure"]; !ok {
			obj["exposure"] = "direct"
		}
		servers[name] = obj
	}
	if spec.WSMCP != nil {
		servers[ReservedMCPServer] = map[string]any{
			"command":  spec.WSMCP.Command,
			"args":     spec.WSMCP.Args,
			"env":      spec.WSMCP.Env,
			"cwd":      spec.Root,
			"exposure": "direct",
		}
	}
	return servers, nil
}

// writePiBridge writes the MCP bridge extension for servers and returns its
// path.
func writePiBridge(dir string, servers map[string]any) (string, error) {
	raw, err := json.Marshal(servers)
	if err != nil {
		return "", err
	}
	src := "// Written by the ws agent runner at every start: registers the MCP servers\n" +
		"// this conversation's agent profile allows, and nothing else.\n" +
		"const servers: Record<string, Record<string, unknown>> = " + string(raw) + ";\n\n" +
		"export default function (pi: { registerMcpServer(name: string, config: Record<string, unknown>): void }) {\n" +
		"  for (const [name, config] of Object.entries(servers)) pi.registerMcpServer(name, config);\n" +
		"}\n"
	path := filepath.Join(dir, piBridgeFile)
	return path, writeFileAtomic(path, []byte(src), 0o600)
}

// piArgs is the pi command line for spec, without the executable; bridge
// is the MCP bridge extension's path, or "" when no MCP server is allowed.
func piArgs(spec StartSpec, bridge string) []string {
	p := spec.Profile
	args := []string{
		"--mode", "rpc",
		"--session", piSessionFile(spec.Dir),
		"--session-dir", piDir(spec.Dir),
		"--no-extensions",
	}
	if bridge != "" {
		args = append(args, "--extension", "builtin:mcp", "--extension", bridge)
	}
	for _, path := range p.PluginPaths(spec.Root) {
		args = append(args, "--extension", path)
	}
	if !p.Skills {
		args = append(args, "--no-skills", "--no-prompt-templates")
	}
	// An explicit append (even empty) also keeps the user's
	// APPEND_SYSTEM.md out.
	args = append(args, "--no-context-files", "--no-approve", "--append-system-prompt", spec.Instructions)
	if surface := piToolSurface(spec); len(surface.entries) > 0 {
		args = append(args, "--tools", strings.Join(surface.entries, ","))
	} else {
		args = append(args, "--no-tools")
	}
	if spec.Model != "" {
		args = append(args, "--model", spec.Model)
	}
	if spec.Effort != "" {
		args = append(args, "--thinking", spec.Effort)
	}
	return args
}

// piEnv is the child's environment: env without the ws Pi package's
// process-role and bootstrap variables a parent Pi session leaves behind,
// and without a mailbox identity.
func piEnv(env []string) []string {
	return harnessEnv(env, []string{"WS_PI_", "WS_MAILBOX"}, nil, map[string]string{"PI_SKIP_VERSION_CHECK": "1"})
}

// checkPiToolEntries refuses permission rules: pi's --tools allows whole
// tools, so a rule such as "bash(git status)" would allow all of bash.
func checkPiToolEntries(p Profile) error {
	for _, entry := range p.Tools {
		if entry != ToolBase(entry) {
			return fmt.Errorf("pi backend: tool entry %q is a permission rule; pi allows whole tools only", entry)
		}
	}
	return nil
}

// Start launches the pi process for spec.
func (b PiBackend) Start(ctx context.Context, spec StartSpec) (Process, error) {
	if err := checkPiToolEntries(spec.Profile); err != nil {
		return nil, err
	}
	command := b.Command
	if command == "" {
		path, err := lookPath("pi")
		if err != nil {
			return nil, fmt.Errorf("pi backend: %w", err)
		}
		command = path
	}
	if err := os.MkdirAll(piDir(spec.Dir), 0o700); err != nil {
		return nil, err
	}
	servers, err := piMCPServers(spec)
	if err != nil {
		return nil, err
	}
	bridge := ""
	if len(servers) > 0 {
		if bridge, err = writePiBridge(piDir(spec.Dir), servers); err != nil {
			return nil, err
		}
	}
	cmd := exec.Command(command, piArgs(spec, bridge)...)
	cmd.Dir = spec.Root
	cmd.Env = piEnv(spec.Env)
	m := newPiMapper(spec)
	p, err := startStreamProcess(cmd, m, piProtocol{})
	if err != nil {
		return nil, err
	}
	return &piProcess{streamProcess: p, mapper: m, effort: spec.Effort}, nil
}

// piProtocol writes Pi's RPC commands.
type piProtocol struct{}

func (piProtocol) userLine(text string) any {
	return map[string]any{"type": "prompt", "message": text}
}

func (piProtocol) requestLine(id string, request map[string]any) any {
	out := map[string]any{"id": id}
	for k, v := range request {
		out[k] = v
	}
	return out
}

// piProcess adds Pi's RPC commands to a stream process.
type piProcess struct {
	*streamProcess
	mapper *piMapper

	mu     sync.Mutex
	effort string
}

func (p *piProcess) Send(text string) error {
	p.mapper.aborted.Store(false)
	return p.streamProcess.Send(text)
}

// Interrupt aborts the running prompt. Pi answers abort only once the run
// settles, so it is sent without waiting; the turn ends at agent_settled.
func (p *piProcess) Interrupt() error {
	p.mapper.aborted.Store(true)
	return p.writeLine(map[string]any{"type": "abort"})
}

// SetModel uses Pi's set_model command for an exact provider/model id; a
// fuzzy pattern, a thinking suffix, or a return to the default needs the
// --model of a restart. set_model resets the thinking level, so a set
// effort is applied again.
func (p *piProcess) SetModel(model string) error {
	provider, id, ok := strings.Cut(model, "/")
	if !ok || provider == "" || id == "" || strings.Contains(id, ":") {
		return ErrUnsupported
	}
	if err := p.request("set_model", map[string]any{"type": "set_model", "provider": provider, "modelId": id}); err != nil {
		return err
	}
	p.mu.Lock()
	effort := p.effort
	p.mu.Unlock()
	if effort == "" {
		return nil
	}
	return p.request("set_thinking_level", map[string]any{"type": "set_thinking_level", "level": effort})
}

// SetEffort uses Pi's set_thinking_level command; a return to the default
// level needs a restart.
func (p *piProcess) SetEffort(effort string) error {
	if effort == "" {
		return ErrUnsupported
	}
	if err := p.request("set_thinking_level", map[string]any{"type": "set_thinking_level", "level": effort}); err != nil {
		return err
	}
	p.mu.Lock()
	p.effort = effort
	p.mu.Unlock()
	return nil
}

// piMapper maps Pi's RPC output to runner events: the tool loadout of each
// system message to the tool check, assistant text to final replies, tool
// calls to the lead's batch (one per assistant message), compaction to
// compact events, agent_settled to the turn's end, and session statistics
// (asked for after each turn) to usage and the session id.
type piMapper struct {
	spec    StartSpec
	surface piSurface
	now     func() time.Time
	batches batchTracker
	// aborted is set by Interrupt; an aborted run's error is the interrupt
	// asked for, not a failure.
	aborted atomic.Bool

	messages int
	turnErr  string
}

func newPiMapper(spec StartSpec) *piMapper {
	return &piMapper{spec: spec, surface: piToolSurface(spec), now: time.Now, batches: newBatchTracker("p")}
}

type piLine struct {
	Type       string          `json:"type"`
	ID         string          `json:"id"`
	Command    string          `json:"command"`
	Success    bool            `json:"success"`
	Error      string          `json:"error"`
	Data       json.RawMessage `json:"data"`
	Message    json.RawMessage `json:"message"`
	ToolCallID string          `json:"toolCallId"`
	IsError    bool            `json:"isError"`
	Reason     string          `json:"reason"`
	Method     string          `json:"method"`
}

type piMessage struct {
	Role       string          `json:"role"`
	Content    json.RawMessage `json:"content"`
	ToolsAdded []struct {
		Name string `json:"name"`
	} `json:"toolsAdded"`
	StopReason   string `json:"stopReason"`
	ErrorMessage string `json:"errorMessage"`
}

type piBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text"`
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

type piStats struct {
	SessionID    string   `json:"sessionId"`
	Cost         *float64 `json:"cost"`
	ContextUsage *struct {
		Tokens        *int64   `json:"tokens"`
		ContextWindow int64    `json:"contextWindow"`
		Percent       *float64 `json:"percent"`
	} `json:"contextUsage"`
}

func (m *piMapper) mapLine(line []byte, replies lineReplies) []Event {
	var l piLine
	if json.Unmarshal(line, &l) != nil {
		return nil
	}
	now := m.now().UnixMilli()
	switch l.Type {
	case "response":
		if l.ID != "" {
			replies.answer(l.ID, controlAnswer{ok: l.Success, err: l.Error})
		}
		switch l.Command {
		case "prompt":
			// A refused or handled prompt starts no run, so no
			// agent_settled will end the turn.
			if !l.Success {
				return m.endTurn(now, "prompt refused: "+l.Error)
			}
			var d struct {
				Disposition string `json:"disposition"`
			}
			if json.Unmarshal(l.Data, &d) == nil && d.Disposition == "handled" {
				return m.endTurn(now, "")
			}
		case "get_session_stats":
			if l.Success {
				return m.stats(l.Data)
			}
		}
	case "message_end":
		return m.message(l.Message, now)
	case "tool_execution_end":
		return m.batches.endCall(l.ToolCallID, l.IsError, now)
	case "compaction_start", "compaction_end":
		trigger := ""
		switch l.Reason {
		case "manual":
			trigger = wslive.TriggerManual
		case "threshold", "overflow":
			trigger = wslive.TriggerAuto
		}
		phase := wslive.CompactStart
		if l.Type == "compaction_end" {
			phase = wslive.CompactEnd
		}
		return []Event{{Live: &wslive.Event{Kind: wslive.KindCompact, Phase: phase, Trigger: trigger}}}
	case "agent_settled":
		replies.send(map[string]any{"type": "get_session_stats"})
		err := m.turnErr
		if m.aborted.Load() {
			err = ""
		}
		return m.endTurn(now, err)
	case "extension_ui_request":
		// No one can answer a dialog in a headless conversation; a waiting
		// dialog would hold the run forever.
		switch l.Method {
		case "select", "confirm", "input", "editor":
			replies.send(map[string]any{"type": "extension_ui_response", "id": l.ID, "cancelled": true})
		}
	}
	return nil
}

func (m *piMapper) endTurn(now int64, turnErr string) []Event {
	var out []Event
	if ev := m.batches.finish(now, true); ev != nil {
		out = append(out, *ev)
	}
	m.turnErr = ""
	return append(out, Event{TurnEnd: true, TurnError: turnErr})
}

func (m *piMapper) message(raw json.RawMessage, now int64) []Event {
	var msg piMessage
	if json.Unmarshal(raw, &msg) != nil {
		return nil
	}
	switch msg.Role {
	case "system":
		var added []string
		for _, t := range msg.ToolsAdded {
			added = append(added, t.Name)
		}
		if err := checkPiTools(added, m.surface); err != nil {
			return []Event{{Fatal: err}}
		}
	case "assistant":
		m.messages++
		group := strconv.Itoa(m.messages)
		m.turnErr = ""
		if msg.StopReason == "error" {
			m.turnErr = strings.TrimSpace(msg.ErrorMessage)
			if m.turnErr == "" {
				m.turnErr = "turn failed"
			}
		}
		var blocks []piBlock
		if json.Unmarshal(msg.Content, &blocks) != nil {
			return nil
		}
		var out []Event
		for _, b := range blocks {
			switch b.Type {
			case "text":
				if strings.TrimSpace(b.Text) != "" {
					out = append(out, Event{Live: &wslive.Event{Kind: wslive.KindReply, Text: b.Text, Final: true}})
				}
			case "toolCall":
				out = append(out, m.batches.addCall(group, b.ID, b.Name, argsPreview(b.Arguments), now)...)
			}
		}
		return out
	}
	return nil
}

func (m *piMapper) stats(raw json.RawMessage) []Event {
	var s piStats
	if json.Unmarshal(raw, &s) != nil {
		return nil
	}
	var out []Event
	if s.SessionID != "" {
		out = append(out, Event{SessionID: s.SessionID})
	}
	ev := wslive.Event{Kind: wslive.KindStatus}
	if s.Cost != nil {
		ev.Cost = &wslive.UsageCost{USD: *s.Cost, Estimate: true}
	}
	if c := s.ContextUsage; c != nil {
		ev.Context = &wslive.UsageContext{Tokens: c.Tokens, Window: c.ContextWindow, Percent: c.Percent}
	}
	if ev.Cost == nil && ev.Context == nil {
		return out
	}
	return append(out, Event{Live: &ev})
}
