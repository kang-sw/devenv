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

	"github.com/kang-sw/devenv/internal/wsinbox"
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

// piEfforts are pi's thinking levels (--thinking), lowest first.
var piEfforts = []string{"off", "minimal", "low", "medium", "high", "xhigh", "max"}

// runListing runs a harness's model listing and returns its stdout; a test
// replaces it with a fixture.
var runListing = func(ctx context.Context, command string, args []string, env []string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, command, args...)
	cmd.Env = env
	return cmd.Output()
}

// Executable is the pi command availability is checked against.
func (b PiBackend) Executable() string {
	if b.Command != "" {
		return b.Command
	}
	return "pi"
}

// Choices lists the models pi --list-models reports, offline and with
// project-local files ignored, and pi's thinking levels. The levels come
// back even when the listing fails.
func (b PiBackend) Choices(ctx context.Context) ([]ModelChoice, []string, error) {
	efforts := append([]string(nil), piEfforts...)
	command, err := lookPath(b.Executable())
	if err != nil {
		return nil, efforts, fmt.Errorf("pi backend: %w", err)
	}
	out, err := runListing(ctx, command, []string{"--offline", "--no-approve", "--list-models"}, piEnv(environ()))
	if err != nil {
		return nil, efforts, fmt.Errorf("pi --list-models: %w", err)
	}
	models, err := parsePiModels(out)
	return models, efforts, err
}

// parsePiModels reads pi --list-models' table: a header naming the
// columns (provider, model, ..., thinking, ...) and one whitespace-separated
// row per model. A model's id is provider/model, as --model takes it; a row
// listed twice is kept once, and an id the runner would refuse is skipped.
func parsePiModels(out []byte) ([]ModelChoice, error) {
	var cols map[string]int
	models := []ModelChoice{}
	seen := map[string]bool{}
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		if cols == nil {
			if fields[0] != "provider" {
				continue
			}
			cols = map[string]int{}
			for i, name := range fields {
				cols[name] = i
			}
			if _, ok := cols["model"]; !ok {
				return nil, fmt.Errorf("pi --list-models header has no model column: %q", strings.TrimSpace(line))
			}
			continue
		}
		if len(fields) <= cols["model"] {
			continue
		}
		id := fields[cols["provider"]] + "/" + fields[cols["model"]]
		if seen[id] || !modelPattern.MatchString(id) {
			continue
		}
		seen[id] = true
		m := ModelChoice{ID: id}
		if i, ok := cols["thinking"]; ok && i < len(fields) {
			thinking := fields[i] == "yes"
			m.Thinking = &thinking
		}
		models = append(models, m)
	}
	if cols == nil && strings.TrimSpace(string(out)) != "" {
		first, _, _ := strings.Cut(strings.TrimSpace(string(out)), "\n")
		return nil, fmt.Errorf("pi --list-models printed no model table: %q", first)
	}
	return models, nil
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
	if spec.Lead != nil {
		return piLeadArgs(spec)
	}
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

// piLeadArgs is the pi command line of a lead conversation: RPC mode on the
// conversation's own session file, with the user's extensions, skills, and
// context files as an ordinary pi session loads them. A session taken over
// from a terminal runs on that session's file instead (spec.SessionFile),
// in pi's own session directory for it.
func piLeadArgs(spec StartSpec) []string {
	args := []string{"--mode", "rpc"}
	if own := piSessionFile(spec.Dir); spec.SessionFile != "" && spec.SessionFile != own {
		args = append(args, "--session", spec.SessionFile)
	} else {
		args = append(args, "--session", own, "--session-dir", piDir(spec.Dir))
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

// piLeadEnv is a lead conversation's environment: piEnv without the ws
// variables of the session that started the daemon (leadDropEnv), plus the
// hosted-session variables.
func piLeadEnv(spec StartSpec) []string {
	set := map[string]string{"PI_SKIP_VERSION_CHECK": "1"}
	for name, value := range spec.Lead.Env {
		set[name] = value
	}
	return harnessEnv(spec.Env, append([]string{"WS_PI_"}, leadDropEnv...), nil, set)
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
	if spec.Lead != nil {
		return b.startLead(spec)
	}
	if spec.Holder != nil && spec.Holder.id != "" {
		// Rejoin transports the already-running child. Do not resolve an
		// executable or rewrite its MCP bridge/instructions on a new daemon.
		m := newPiMapper(spec)
		p := newStreamProcess(m, piProtocol{})
		if err := spec.Holder.start(p, &exec.Cmd{Path: "pi"}); err != nil {
			return nil, err
		}
		return &piProcess{streamProcess: p, mapper: m, effort: spec.Effort}, nil
	}
	if err := checkPiToolEntries(spec.Profile); err != nil {
		return nil, err
	}
	command, pre, err := harnessCommand(b.Command, "pi")
	if err != nil {
		return nil, fmt.Errorf("pi backend: %w", err)
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
	cmd := exec.Command(command, append(pre, piArgs(spec, bridge)...)...)
	cmd.Dir = spec.workDir()
	cmd.Env = piEnv(spec.Env)
	m := newPiMapper(spec)
	p, err := startChild(spec, cmd, m, piProtocol{})
	if err != nil {
		return nil, err
	}
	return &piProcess{streamProcess: p, mapper: m, effort: spec.Effort}, nil
}

// startLead launches (or, with a holder to attach to, rejoins) the pi
// process of a lead conversation.
func (b PiBackend) startLead(spec StartSpec) (Process, error) {
	command, pre, err := harnessCommand(b.Command, "pi")
	if err != nil {
		return nil, fmt.Errorf("pi backend: %w", err)
	}
	if err := os.MkdirAll(piDir(spec.Dir), 0o700); err != nil {
		return nil, err
	}
	cmd := exec.Command(command, append(pre, piLeadArgs(spec)...)...)
	cmd.Dir = spec.workDir()
	cmd.Env = piLeadEnv(spec)
	m := newPiMapper(spec)
	attach := spec.Holder != nil && spec.Holder.id != ""
	p, err := startChild(spec, cmd, m, piProtocol{})
	if err != nil {
		return nil, err
	}
	if !attach {
		// The command list the dashboard's composer offers; each turn's
		// end asks again (piMapper), so commands an extension registers
		// later show too. A rejoin's replay rebuilds the list it had.
		p.send(piGetCommands)
	}
	return &piProcess{streamProcess: p, mapper: m, effort: spec.Effort, lead: true}, nil
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
	// lead marks a lead conversation's process.
	lead bool
}

func (p *piProcess) Send(text string) error {
	p.mapper.aborted.Store(false)
	if rest, ok := piCompactCommand(text); p.lead && ok {
		// A typed /compact is no RPC built-in: it is the compact command.
		return p.compact(rest)
	}
	return p.streamProcess.Send(text)
}

// RunCommand runs a lead's dashboard slash command: /compact is the compact
// command, with its arguments as the instructions; any other command is a
// prompt of its typed line, which pi expands (an extension command, a
// prompt template, a skill). It is sent at a turn's end only, never as a
// steer or follow-up, which refuse extension commands.
func (p *piProcess) RunCommand(cmd wsinbox.Command) error {
	if p.lead && cmd.Name == "compact" {
		p.mapper.aborted.Store(false)
		return p.compact(cmd.Args)
	}
	return p.Send(cmd.Line())
}

// compact writes the compact command, with instructions when there are any.
func (p *piProcess) compact(instructions string) error {
	line := map[string]any{"type": "compact"}
	if instructions = strings.TrimSpace(instructions); instructions != "" {
		line["customInstructions"] = instructions
	}
	return p.writeLine(line)
}

// piCompactCommand reports whether text is a typed /compact, and returns
// the instructions after it.
func piCompactCommand(text string) (string, bool) {
	text = strings.TrimSpace(text)
	if text == "/compact" {
		return "", true
	}
	if rest, ok := strings.CutPrefix(text, "/compact "); ok && !strings.Contains(rest, "\n") {
		return strings.TrimSpace(rest), true
	}
	return "", false
}

// Steer is pi's steer command: the running agent takes the message after
// its current tool calls, before its next model request.
func (p *piProcess) Steer(text string) error {
	return p.writeLine(map[string]any{"type": "steer", "message": text})
}

// ClearSteered is pi's clear_queue command: it drops the steered messages
// pi queued and no run took, once the run that would have taken them
// ended.
func (p *piProcess) ClearSteered() error {
	return p.request("clear_queue", map[string]any{"type": "clear_queue"})
}

// piUserText is a user message's text, as pi matches a queued message to
// the one its run took: a string content, or its text blocks joined.
func piUserText(raw json.RawMessage) string {
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return text
	}
	var blocks []piBlock
	if json.Unmarshal(raw, &blocks) != nil {
		return ""
	}
	var parts []string
	for _, b := range blocks {
		if b.Type == "text" {
			parts = append(parts, b.Text)
		}
	}
	return strings.Join(parts, "")
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

// TurnRunning asks pi's get_state whether a run is under way: isStreaming
// stays set until agent_settled (a retry, a compaction, and a queued
// continuation included), and isCompacting covers a compact command, whose
// answer ends the runner's turn. An answer without isStreaming is an
// error, not an idle harness.
func (p *piProcess) TurnRunning() (bool, error) {
	body, err := p.requestBody("get_state", map[string]any{"type": "get_state"}, controlTimeout)
	if err != nil {
		return false, err
	}
	var d struct {
		IsStreaming  *bool `json:"isStreaming"`
		IsCompacting bool  `json:"isCompacting"`
	}
	if json.Unmarshal(body, &d) != nil || d.IsStreaming == nil {
		return false, fmt.Errorf("pi's get_state answer reports no isStreaming: %.200s", body)
	}
	return *d.IsStreaming || d.IsCompacting, nil
}

// piMapper maps Pi's RPC output to runner events: the tool loadout of each
// system message to the tool check, assistant text to final replies, tool
// calls to the lead's batch (one per assistant message), compaction to
// compact events, agent_settled to the turn's end, session statistics
// (asked for after each turn) to usage and the session id, and the hosted
// lead's subagent widget (piAgentsWidgetKey) to subagent rows.
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
	ToolName   string          `json:"toolName"`
	// Result is a tool_execution_end's result, read only for a card or UI call:
	// left raw so no other line's result shape can fail the line's decode.
	Result  json.RawMessage `json:"result"`
	IsError bool            `json:"isError"`
	Reason  string          `json:"reason"`
	Method  string          `json:"method"`
	// WidgetKey and WidgetLines are a setWidget request's. The lines stay
	// raw so a widget of another shape cannot fail the line's decode.
	WidgetKey   string          `json:"widgetKey"`
	WidgetLines json.RawMessage `json:"widgetLines"`
}

type piMessage struct {
	Role       string          `json:"role"`
	Content    json.RawMessage `json:"content"`
	ToolsAdded []struct {
		Name string `json:"name"`
	} `json:"toolsAdded"`
	StopReason   string `json:"stopReason"`
	ErrorMessage string `json:"errorMessage"`
	Timestamp    int64  `json:"timestamp"`
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
			replies.answer(l.ID, controlAnswer{ok: l.Success, err: l.Error, body: l.Data})
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
		case "get_commands":
			if l.Success {
				return piCommands(l.Data)
			}
		case "compact":
			// A lead's typed /compact ran as this command: its answer ends
			// the turn the send began.
			if m.spec.Lead != nil {
				errText := ""
				if !l.Success {
					errText = "compact refused: " + l.Error
				}
				return m.endTurn(now, errText)
			}
		case "steer":
			// A non-lead process steers a child's report, which the runner
			// sends as a turn instead when pi refuses it (children.go).
			if m.spec.Lead == nil {
				answer := SteerAnswer{}
				if !l.Success {
					answer.Refusal = l.Error
					if answer.Refusal == "" {
						answer.Refusal = "refused"
					}
				}
				return []Event{{SteerAnswer: &answer}}
			}
			// A lead's message was taken from the inbox already; a refusal
			// is all that is left of it.
			if !l.Success {
				return []Event{{Notice: "pi refused a steered message: " + l.Error}}
			}
		}
	case "agent_start":
		// Workers need the same boundary when rebuilding a held RPC stream.
		return []Event{{TurnStart: true}}
	case "message_end":
		return m.message(l.Message, now)
	case "tool_execution_end":
		out := m.batches.endCall(l.ToolCallID, l.IsError, now)
		var result struct {
			Content []piBlock `json:"content"`
		}
		tool := resultTool(l.ToolName, piMCPToolName)
		if l.IsError || tool == "" {
			return out
		}
		// A result that does not decode reads as empty text: a tracker
		// write still counts, and no card or UI line parses.
		var parts []string
		if json.Unmarshal(l.Result, &result) == nil {
			for _, b := range result.Content {
				if b.Type == "text" {
					parts = append(parts, b.Text)
				}
			}
		}
		return append(out, resultEvents(tool, strings.Join(parts, "\n"))...)
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
		if m.spec.Lead != nil {
			replies.send(piGetCommands)
		}
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
		case "setWidget":
			if l.WidgetKey == piEventsWidgetKey && m.spec.Lead != nil {
				return piHostedEvents(l.WidgetLines)
			}
			if l.WidgetKey == piAgentsWidgetKey {
				return piAgentEvents(l.WidgetLines)
			}
		}
	}
	return nil
}

// recoveryReply reads Pi's saved RPC messages only after get_state says idle.
// The final message must be an assistant result, not an unfinished tool call.
func (p *piProcess) recoveryReply(startedAt int64) (string, string, []string, error) {
	body, err := p.requestBody("get_messages", map[string]any{"type": "get_messages"}, controlTimeout)
	if err != nil {
		return "", "", nil, err
	}
	var data struct {
		Messages []piMessage `json:"messages"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		return "", "", nil, err
	}
	if len(data.Messages) == 0 {
		return "", "", nil, fmt.Errorf("no saved final assistant message after a replay gap")
	}
	last := data.Messages[len(data.Messages)-1]
	if startedAt == 0 || last.Timestamp < startedAt || last.Role != "assistant" || (last.StopReason != "stop" && last.StopReason != "error" && last.StopReason != "aborted") {
		return "", "", nil, fmt.Errorf("saved messages do not identify a final assistant result after a replay gap")
	}
	var taken []string
	for _, message := range data.Messages {
		if message.Role == "user" && message.Timestamp >= startedAt {
			taken = append(taken, piUserText(message.Content))
		}
	}
	turnErr := ""
	if last.StopReason == "error" {
		turnErr = strings.TrimSpace(last.ErrorMessage)
		if turnErr == "" {
			turnErr = "turn failed"
		}
	}
	return piUserText(last.Content), turnErr, taken, nil
}

// piGetCommands asks a lead's pi for its slash commands.
var piGetCommands = map[string]any{"type": "get_commands"}

// piCompact is the compact command as the command list shows it: pi's
// get_commands leaves its built-in commands out, and compact is the one
// the runner maps (RunCommand).
var piCompact = wslive.SlashCommand{Name: "compact", Description: "Manually compact the session context", Source: "builtin"}

// piCommands maps a get_commands answer to a commands event: each command
// with its description and its source (extension, prompt, skill) as pi
// names it, which tells where it comes from, not who may invoke it, so no
// layer is set; compact is added when pi does not list it. An unreadable
// answer changes nothing.
func piCommands(raw json.RawMessage) []Event {
	var d struct {
		Commands []struct {
			Name        string `json:"name"`
			Description string `json:"description"`
			Source      string `json:"source"`
		} `json:"commands"`
	}
	if json.Unmarshal(raw, &d) != nil {
		return nil
	}
	list := []wslive.SlashCommand{}
	compact := false
	for _, c := range d.Commands {
		compact = compact || c.Name == piCompact.Name
		list = append(list, wslive.SlashCommand{Name: c.Name, Description: c.Description, Source: c.Source})
	}
	if !compact {
		list = append([]wslive.SlashCommand{piCompact}, list...)
	}
	return []Event{{Live: &wslive.Event{Kind: wslive.KindCommands, Commands: list}}}
}

// piAgentsWidgetKey is the widget a hosted Pi lead's ws extension sets to
// report its native subagents: each line is one JSON live event, the
// agent and permission events its dashboard feed sends outside a hosted
// session. RPC mode carries an extension's output to stdout only as UI
// requests, and a widget passes string lines through unchanged.
const piAgentsWidgetKey = "ws-dashboard-agents"

// piAgentEvents decodes the subagent widget's lines. Only agent and
// permission events naming a subagent are taken: the lead, its turns, and
// a reset stay the runner's own. A line that does not decode is skipped.
func piAgentEvents(raw json.RawMessage) []Event {
	var lines []string
	if json.Unmarshal(raw, &lines) != nil {
		return nil
	}
	var out []Event
	for _, line := range lines {
		var ev wslive.Event
		if json.Unmarshal([]byte(line), &ev) != nil || ev.AgentID == "" {
			continue
		}
		if ev.Kind == wslive.KindAgent || ev.Kind == wslive.KindPermission {
			out = append(out, Event{Live: &ev})
		}
	}
	return out
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
	case "user":
		// A non-lead run took this message: a steered child's report it
		// took is not sent again (children.go).
		if m.spec.Lead == nil {
			if text := piUserText(msg.Content); text != "" {
				return []Event{{UserTaken: text}}
			}
		}
	case "system":
		var added []string
		for _, t := range msg.ToolsAdded {
			added = append(added, t.Name)
		}
		// A lead runs the user's own loadout; only a profile's is checked.
		if m.spec.Lead == nil {
			if err := checkPiTools(added, m.surface); err != nil {
				return []Event{{Fatal: err}}
			}
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

// piVersionPattern finds a dotted version in `pi --version` output.
var piVersionPattern = regexp.MustCompile(`(\d+)\.(\d+)(?:\.\d+)?`)

// PiVersion runs `pi --version` (pi looked up on PATH, as PiBackend does
// with no Command) and returns its first dotted version, such as "1.0.4".
// It fails when pi is missing, does not exit within the context, or prints
// no version.
func PiVersion(ctx context.Context) (string, error) {
	command, pre, err := harnessCommand("", "pi")
	if err != nil {
		return "", err
	}
	cmd := exec.CommandContext(ctx, command, append(pre, "--version")...)
	cmd.Env = piEnv(environ())
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("pi --version: %w", err)
	}
	version := piVersionPattern.FindString(string(out))
	if version == "" {
		return "", fmt.Errorf("pi --version printed no version: %q", strings.TrimSpace(string(out)))
	}
	return version, nil
}

// VersionAtLeast reports whether version's major and minor numbers (its
// first dotted pair) are at least major.minor; a version it cannot read is
// not.
func VersionAtLeast(version string, major, minor int) bool {
	m := piVersionPattern.FindStringSubmatch(version)
	if m == nil {
		return false
	}
	gotMajor, _ := strconv.Atoi(m[1])
	gotMinor, _ := strconv.Atoi(m[2])
	return gotMajor > major || (gotMajor == major && gotMinor >= minor)
}
