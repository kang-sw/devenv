package wsagent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/kang-sw/devenv/internal/claudeagents"
	"github.com/kang-sw/devenv/internal/wslive"
)

// ClaudeBackend runs Claude Code headless: print mode with stream-json in
// and out and stdin held open, one process per conversation. It keeps
// Claude Code's own system prompt and appends the composed instruction file
// (--append-system-prompt-file); it never passes --bare (which drops the
// subscription login) or --system-prompt.
type ClaudeBackend struct {
	// Command is the claude executable; empty looks up "claude" on PATH.
	Command string
}

// claudeModelAliases are the models the claude backend offers: the CLI's
// version-less aliases, passed verbatim to --model so each always loads the
// latest model of its family. The CLI has no model-list command, and a
// versioned name would go stale.
var claudeModelAliases = []string{"opus", "sonnet", "haiku", "fable"}

// Executable is the claude command availability is checked against.
func (b ClaudeBackend) Executable() string {
	if b.Command != "" {
		return b.Command
	}
	return "claude"
}

// Choices lists the model aliases and the effort levels.
func (ClaudeBackend) Choices(context.Context) ([]ModelChoice, []string, error) {
	models := make([]ModelChoice, 0, len(claudeModelAliases))
	for _, id := range claudeModelAliases {
		models = append(models, ModelChoice{ID: id})
	}
	return models, append([]string(nil), claudeagents.Levels...), nil
}

// claudeDisabledPlugins are Claude Code's built-in plugins that settings can
// turn off; the remaining built-ins (telemetry, privacy) cannot be.
var claudeDisabledPlugins = []string{"cc-plugin-agents-md@builtin", "cc-plugin-plugin-authoring@builtin"}

// claudeKeptEnv are CLAUDE* variables a child keeps: the user's config
// directory and credentials. Every other CLAUDE* variable belongs to the
// parent harness session that started the daemon.
var claudeKeptEnv = map[string]bool{
	"CLAUDE_CONFIG_DIR":       true,
	"CLAUDE_CODE_OAUTH_TOKEN": true,
	"CLAUDE_CODE_USE_BEDROCK": true,
	"CLAUDE_CODE_USE_VERTEX":  true,
}

// claudeEnv is the child's environment: env without the parent harness
// session's CLAUDE* variables (claudeKeptEnv aside) and without a mailbox
// identity, which the child's ws-mcp would otherwise claim.
func claudeEnv(env []string) []string {
	return harnessEnv(env, []string{"CLAUDE", "WS_MAILBOX"}, claudeKeptEnv, nil)
}

// leadDropEnv are the variable prefixes a lead conversation's child starts
// without, beyond its harness's own: the daemon's environment is that of
// whichever session's launcher started it, and its mailbox identity, its
// ws-mcp launcher state (project root, runtime binary, resource tree), and
// its own hosted-session variables must not reach another session. The
// user's state locations (WS_CACHE_HOME, WS_CONFIG_HOME) stay.
var leadDropEnv = []string{"WS_MAILBOX", "WS_MCP_", "WS_RSRC", "WS_HOSTED_"}

// claudeLeadEnv is a lead conversation's environment: env without the
// parent harness session's CLAUDE* variables and leadDropEnv, plus the
// hosted-session variables.
func claudeLeadEnv(spec StartSpec) []string {
	// The session-state lines are what tell a turn nobody sent from an idle
	// harness (session_state_changed in mapLine).
	set := map[string]string{"CLAUDE_CODE_EMIT_SESSION_STATE_EVENTS": "1"}
	for name, value := range spec.Lead.Env {
		set[name] = value
	}
	return harnessEnv(spec.Env, append([]string{"CLAUDE"}, leadDropEnv...), claudeKeptEnv, set)
}

// claudeLeadArgs is the command line of a lead conversation: print mode
// with stream-json both ways and nothing held back, so user and project
// settings, plugins, hooks, memory, and MCP servers load as in the user's
// own session. Tool permission prompts come to the daemon
// (--permission-prompt-tool stdio) as can_use_tool control requests.
func claudeLeadArgs(spec StartSpec) []string {
	mode := spec.Lead.PermissionMode
	if mode == "" {
		mode = DefaultLeadPermissionMode
	}
	args := []string{
		"-p",
		"--input-format", "stream-json",
		"--output-format", "stream-json",
		"--verbose",
		"--permission-mode", mode,
		"--permission-prompt-tool", "stdio",
	}
	if spec.Model != "" {
		args = append(args, "--model", spec.Model)
	}
	if spec.Effort != "" {
		args = append(args, "--effort", spec.Effort)
	}
	if spec.SessionID != "" {
		args = append(args, "--resume", spec.SessionID)
	}
	return args
}

// claudeArgs is the claude command line for spec, without the executable.
func claudeArgs(spec StartSpec) ([]string, error) {
	if spec.Lead != nil {
		return claudeLeadArgs(spec), nil
	}
	p := spec.Profile
	mcpConfig, err := claudeMCPConfig(spec)
	if err != nil {
		return nil, err
	}
	disabled := map[string]bool{}
	for _, id := range claudeDisabledPlugins {
		disabled[id] = false
	}
	settings, _ := json.Marshal(map[string]any{"enabledPlugins": disabled})
	var bases []string
	seen := map[string]bool{}
	for _, entry := range p.Tools {
		if base := ToolBase(entry); !seen[base] {
			seen[base] = true
			bases = append(bases, base)
		}
	}
	args := []string{
		"-p",
		"--input-format", "stream-json",
		"--output-format", "stream-json",
		"--verbose",
		// No settings file: user, project, and local settings (their
		// permission rules, hooks, plugins, and memory files) stay out.
		"--setting-sources", "",
		"--settings", string(settings),
		"--strict-mcp-config", "--mcp-config", mcpConfig,
		"--tools", strings.Join(bases, ","),
		"--permission-mode", "dontAsk",
	}
	if !p.Skills {
		args = append(args, "--disable-slash-commands")
	}
	for _, dir := range p.PluginPaths(spec.Root) {
		args = append(args, "--plugin-dir", dir)
	}
	if spec.Instructions != "" {
		args = append(args, "--append-system-prompt-file", spec.Instructions)
	}
	if spec.Model != "" {
		args = append(args, "--model", spec.Model)
	}
	if spec.Effort != "" {
		args = append(args, "--effort", spec.Effort)
	}
	if spec.SessionID != "" {
		args = append(args, "--resume", spec.SessionID)
	}
	// The allowlist goes last: the option takes every following argument.
	if allowed := claudeAllowedRules(spec); len(allowed) > 0 {
		args = append(args, "--allowedTools")
		args = append(args, allowed...)
	}
	return args, nil
}

// claudeAllowedRules are the permission rules the child runs without a
// prompt: the profile's tool entries, its ws tools, and every tool of its
// own MCP servers. dontAsk denies everything else.
func claudeAllowedRules(spec StartSpec) []string {
	rules := append([]string{}, spec.Profile.Tools...)
	if spec.WSMCP != nil {
		for _, name := range spec.Profile.WSTools {
			rules = append(rules, claudeMCPToolName(ReservedMCPServer, name))
		}
	}
	for _, name := range spec.Profile.MCPServerNames() {
		rules = append(rules, "mcp__"+claudeNamePart(name))
	}
	return rules
}

func claudeMCPConfig(spec StartSpec) (string, error) {
	servers := map[string]any{}
	for name, raw := range spec.Profile.MCPServers {
		servers[name] = raw
	}
	if spec.WSMCP != nil {
		servers[ReservedMCPServer] = map[string]any{"type": "stdio", "command": spec.WSMCP.Command, "args": spec.WSMCP.Args, "env": spec.WSMCP.Env}
	}
	raw, err := json.Marshal(map[string]any{"mcpServers": servers})
	return string(raw), err
}

var claudeNameUnsafe = regexp.MustCompile(`[^A-Za-z0-9_-]`)

func claudeNamePart(s string) string { return claudeNameUnsafe.ReplaceAllString(s, "_") }

// claudeMCPToolName is Claude Code's name for an MCP server's tool:
// mcp__<server>__<tool>, with characters outside [A-Za-z0-9_-] (a ws tool
// name's dots) as underscores.
func claudeMCPToolName(server, tool string) string {
	return "mcp__" + claudeNamePart(server) + "__" + claudeNamePart(tool)
}

// claudeInit is the part of the system/init message the init check reads.
type claudeInit struct {
	SessionID      string   `json:"session_id"`
	Model          string   `json:"model"`
	Tools          []string `json:"tools"`
	PermissionMode string   `json:"permissionMode"`
	MCPServers     []struct {
		Name   string `json:"name"`
		Status string `json:"status"`
	} `json:"mcp_servers"`
	Plugins []struct {
		Name   string `json:"name"`
		Path   string `json:"path"`
		Source string `json:"source"`
	} `json:"plugins"`
	// SlashCommands are the names of the commands a message may run
	// (absent from an older harness's init). The init's
	// terminal_slash_commands are left out: they need a terminal, and a
	// headless session has none.
	SlashCommands *[]string `json:"slash_commands"`
}

// claudeInitToolNames maps a built-in tool name to the older name Claude's
// system/init may report for it: Claude 2.1.319 launched with --tools Agent
// lists the tool as "Task" in init while its calls are named "Agent".
var claudeInitToolNames = map[string]string{"Agent": "Task"}

// checkClaudeInit verifies that the child sees only what spec permits: its
// tools are the profile's built-in tools, ws tools, and own servers' tools;
// its MCP servers are ws (when attached) and the profile's; every plugin
// is a built-in or one of the profile's plugin directories; and permissions
// run in dontAsk mode.
func checkClaudeInit(init claudeInit, spec StartSpec) error {
	allowed := map[string]bool{}
	for _, entry := range spec.Profile.Tools {
		base := ToolBase(entry)
		allowed[base] = true
		if legacy, ok := claudeInitToolNames[base]; ok {
			allowed[legacy] = true
		}
	}
	servers := map[string]bool{}
	var prefixes []string
	if spec.WSMCP != nil {
		servers[ReservedMCPServer] = true
		for _, name := range spec.Profile.WSTools {
			allowed[claudeMCPToolName(ReservedMCPServer, name)] = true
		}
	}
	for _, name := range spec.Profile.MCPServerNames() {
		servers[name] = true
		prefixes = append(prefixes, "mcp__"+claudeNamePart(name)+"__")
	}
	var extra []string
	for _, tool := range init.Tools {
		ok := allowed[tool]
		for _, prefix := range prefixes {
			ok = ok || strings.HasPrefix(tool, prefix)
		}
		if !ok {
			extra = append(extra, tool)
		}
	}
	if len(extra) > 0 {
		sort.Strings(extra)
		return fmt.Errorf("launch refused: claude reports tools outside the profile: %s", strings.Join(extra, ", "))
	}
	for _, s := range init.MCPServers {
		if !servers[s.Name] {
			return fmt.Errorf("launch refused: claude reports MCP server %q outside the profile", s.Name)
		}
	}
	dirs := map[string]bool{}
	for _, dir := range spec.Profile.PluginPaths(spec.Root) {
		dirs[filepath.Clean(dir)] = true
	}
	for _, pl := range init.Plugins {
		if strings.HasSuffix(pl.Source, "@builtin") || dirs[filepath.Clean(pl.Path)] {
			continue
		}
		return fmt.Errorf("launch refused: claude loaded plugin %q (%s) outside the profile", pl.Name, pl.Path)
	}
	if init.PermissionMode != "dontAsk" {
		return fmt.Errorf("launch refused: claude runs in permission mode %q, not dontAsk", init.PermissionMode)
	}
	return nil
}

// Start launches the claude process for spec.
func (b ClaudeBackend) Start(ctx context.Context, spec StartSpec) (Process, error) {
	command, pre, err := harnessCommand(b.Command, "claude")
	if err != nil {
		return nil, fmt.Errorf("claude backend: %w", err)
	}
	args, err := claudeArgs(spec)
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(command, append(pre, args...)...)
	cmd.Dir = spec.workDir()
	cmd.Env = claudeEnv(spec.Env)
	if spec.Lead != nil {
		cmd.Env = claudeLeadEnv(spec)
	}
	// Read before the start, which fills the holder id for a spawn too.
	attach := spec.Holder != nil && spec.Holder.id != ""
	p, err := startChild(spec, cmd, newClaudeMapper(spec), claudeProtocol{})
	if err != nil {
		return nil, err
	}
	if spec.Lead != nil && !attach {
		// The command list the dashboard's composer offers: an init, which
		// brings it, comes only with a turn, so a session nobody has
		// written to would offer no commands. A rejoin's replay rebuilds
		// the list it had.
		p.send(claudeProtocol{}.requestLine(claudeCommandsAsk, map[string]any{"subtype": "initialize"}))
	}
	return claudeProcess{streamProcess: p, lead: spec.Lead != nil}, nil
}

// claudeProtocol writes Claude Code's stream-json input lines.
type claudeProtocol struct{}

func (claudeProtocol) userLine(text string) any {
	return map[string]any{"type": "user", "message": map[string]any{"role": "user", "content": text}}
}

func (claudeProtocol) requestLine(id string, request map[string]any) any {
	return map[string]any{"type": "control_request", "request_id": id, "request": request}
}

// claudeProcess adds Claude Code's control requests to a stream process.
type claudeProcess struct {
	*streamProcess
	// lead marks a lead conversation's process.
	lead bool
}

func (p claudeProcess) Interrupt() error {
	return p.request("interrupt", map[string]any{"subtype": "interrupt"})
}

// Steer writes text as a user message while a turn runs. Claude Code queues
// it and takes it up after the running tool call, inside the same turn:
// one result follows for both messages (seen on 2.1.322).
func (p claudeProcess) Steer(text string) error { return p.Send(text) }

// SetModel uses Claude Code's set_model control, which applies from the
// next turn. A set_model that names no model returns to the default one:
// the model a launch with no --model runs, whatever this launch named
// (seen on 2.1.322).
func (p claudeProcess) SetModel(model string) error {
	req := map[string]any{"subtype": "set_model"}
	if model != "" {
		req["model"] = model
	}
	return p.request("set_model", req)
}

// SetEffort changes the effort with Claude Code's apply_flag_settings
// control (2.1.322 answers it with a bare success), which applies from the
// next turn. The control merges into the settings the launch's --settings
// gave, so a profile's process keeps those. A null level clears the
// override and so returns to the default; the harness then runs the
// model's own default level until its next launch, which reads the user's
// settings again.
func (p claudeProcess) SetEffort(effort string) error {
	var level any
	if effort != "" {
		level = effort
	}
	return p.request("apply_flag_settings", map[string]any{"subtype": "apply_flag_settings", "settings": map[string]any{"effortLevel": level}})
}

// SetPermissionMode changes a lead's permission mode with Claude Code's
// set_permission_mode control. The mode the harness then runs in comes back
// on its stream (a status line's permissionMode).
func (p claudeProcess) SetPermissionMode(mode string) error {
	if !p.lead || mode == "" {
		return ErrUnsupported
	}
	return p.request("set_permission_mode", map[string]any{"subtype": "set_permission_mode", "mode": mode})
}

// mcpReconnectTimeout bounds the wait for an mcp_reconnect's answer, which
// comes once the server has started again.
var mcpReconnectTimeout = time.Minute

// MCPStatus asks a lead's harness for its MCP servers and their status
// (Claude Code's mcp_status control).
func (p claudeProcess) MCPStatus() ([]MCPServer, error) {
	if !p.lead {
		return nil, ErrUnsupported
	}
	body, err := p.requestBody("mcp_status", map[string]any{"subtype": "mcp_status"}, controlTimeout)
	if err != nil {
		return nil, err
	}
	var d struct {
		MCPServers []MCPServer `json:"mcpServers"`
	}
	if len(body) > 0 && json.Unmarshal(body, &d) != nil {
		return nil, errors.New("unreadable mcp_status answer")
	}
	return d.MCPServers, nil
}

// ReconnectMCP reconnects one of a lead's MCP servers in-stream (Claude
// Code's mcp_reconnect control, which answers once the server is back or
// failed again). The /mcp reconnect command is refused under print mode;
// the control is not.
func (p claudeProcess) ReconnectMCP(server string) error {
	if !p.lead {
		return ErrUnsupported
	}
	_, err := p.requestBody("mcp_reconnect", map[string]any{"subtype": "mcp_reconnect", "serverName": server}, mcpReconnectTimeout)
	return err
}

// AnswerApproval answers a can_use_tool request on the child's input: allow
// hands the tool call's input back unchanged, deny carries the message the
// model reads.
func (p claudeProcess) AnswerApproval(req ApprovalRequest, allow bool, message string) error {
	return p.writeLine(claudeApprovalResponse(req, allow, message))
}

// claudeApprovalResponse is the control_response line that answers req.
func claudeApprovalResponse(req ApprovalRequest, allow bool, message string) any {
	answer := map[string]any{"behavior": "deny", "message": message}
	if allow {
		input := req.Input
		if len(input) == 0 {
			input = json.RawMessage("{}")
		}
		answer = map[string]any{"behavior": "allow", "updatedInput": input}
	} else if strings.TrimSpace(message) == "" {
		answer["message"] = "The user denied this tool call."
	}
	return map[string]any{"type": "control_response", "response": map[string]any{"subtype": "success", "request_id": req.ID, "response": answer}}
}

// claudeControlError is the control_response line that refuses the control
// request id with message.
func claudeControlError(id, message string) any {
	return map[string]any{"type": "control_response", "response": map[string]any{"subtype": "error", "request_id": id, "error": message}}
}

// A tool whose prompt is a question to the user, not a permission, is
// denied with a message that tells the model what to do instead: an allow
// without the user's answers breaks such a tool, and the dashboard renders
// no form for it. claudeChatOnlyDenial is for a question (AskUserQuestion,
// or any request flagged as needing user interaction); claudePlanDenial is
// ExitPlanMode's, since only the user's own mode change leaves plan mode.
const (
	claudeChatOnlyDenial  = "This session has no interactive form for this tool. Put the question in your reply as plain text and end your turn; the user answers in the chat."
	claudePlanDenial      = "This session has no interactive form for approving a plan. Present the plan in your reply as plain text and end your turn. The session stays in plan mode until the user changes the permission mode from the session's menu; do not call ExitPlanMode again before the user says the mode changed."
	claudeMalformedDenial = "The permission request was malformed (no tool name, tool call id, or input), so it was denied."
)

// claudeMapper maps Claude Code's stream-json output to runner events: the
// init message to the session id (after the init check), assistant text to
// final replies, tool calls to the lead's batch (one per assistant
// message), rate-limit and result figures to usage, and the result to the
// turn's end, and the subagent tasks the Agent tool starts to subagents.
// A line a subagent wrote (parent_tool_use_id set) is not the lead's: its
// text and tool calls stay out of the lead's reply and batches.
type claudeMapper struct {
	spec    StartSpec
	now     func() time.Time
	batches batchTracker
	// tasks are the subagent task ids started and not yet ended.
	tasks map[string]bool
	// wakeTasks retains task identity past task_updated's terminal state:
	// task_notification follows it and carries the delivered summary.
	wakeTasks map[string]claudeWakeTask

	context *wslive.UsageContext
	limits  map[string]wslive.UsageLimit
	cost    *wslive.UsageCost
	// window is the largest modelUsage.contextWindow a result reported: the
	// model's window, the fallback for compactWindow.
	window int64
	// model is the model the lead's last init named. compactWindow is the
	// window the harness compacts against (get_context_usage's
	// rawMaxTokens, which honours CLAUDE_CODE_AUTO_COMPACT_WINDOW), as
	// answered for compactModel; it counts only while that is model.
	// askModel is the model the last ask was sent under, and asks numbers
	// them.
	model         string
	compactWindow int64
	compactModel  string
	askModel      string
	asks          int
	// described are the descriptions the initialize answer gave the
	// commands, by name: a turn's init lists names only, and its list
	// replaces the answer's.
	described map[string]string

	// compacted is set from a compact_boundary to the next result: the
	// stream re-emits the last assistant message in between, with no replay
	// flag but under the line uuid it had before, and it is dropped. uuids
	// are the recent assistant lines' (claudeSeenUUIDs of them), so a real
	// message after an automatic compaction mid-turn is kept.
	compacted bool
	uuids     []string
	// states is set once the stream carried a session_state_changed line
	// (a harness that emits them): from then on the idle state ends a
	// lead's turn, not the result, which an idle follows and whose error
	// turnErr holds until then. Ending the turn at the result too would
	// let that trailing idle end the next turn, which a held message
	// submitted at the result had already begun.
	//
	// The exception is a result while background tasks run (background,
	// the last background_tasks_changed count): the harness then stays
	// running and sends no idle until every task ended and the turn their
	// completion starts is done, also across later prompts (seen on
	// 2.1.323, claude-lead-background-agent-states.jsonl), so that result
	// ends the turn. ended marks such a result until the next turn start
	// is mapped: an idle in between belongs to the turn already ended and
	// ends nothing.
	states     bool
	turnErr    string
	background int
	ended      bool
	// approvals are the can_use_tool requests not seen settled, request id
	// by tool call id.
	approvals map[string]string

	// retrying is set from an api_retry line until a response or the turn's
	// end settles it.
	retrying bool
	// rejected are the usage limits the last rate-limit lines reported as
	// refusing requests, each with its reset time (epoch ms; 0 unknown).
	rejected map[string]int64
	// spawns are the lead's Agent calls not yet started as a task (their
	// requested model and effort), and subs the running subagent tasks'
	// telemetry; both by the Agent call's tool call id.
	spawns map[string]claudeSpawn
	subs   map[string]*claudeSub
}

// claudeSpawn is what a lead's Agent call asked of its subagent: the model
// (the call's model parameter) and the effort its subagent type carries.
type claudeSpawn struct {
	model, effort string
}

func newClaudeMapper(spec StartSpec) *claudeMapper {
	return &claudeMapper{spec: spec, now: time.Now, batches: newBatchTracker("c"), limits: map[string]wslive.UsageLimit{}, tasks: map[string]bool{}, approvals: map[string]string{},
		rejected: map[string]int64{}, spawns: map[string]claudeSpawn{}, subs: map[string]*claudeSub{}, wakeTasks: map[string]claudeWakeTask{}}
}

// lead reports a lead conversation's mapper.
func (m *claudeMapper) lead() bool { return m.spec.Lead != nil }

type claudeLine struct {
	Type    string          `json:"type"`
	Subtype string          `json:"subtype"`
	Message json.RawMessage `json:"message"`
	// UUID is the line's own id; a re-emitted line keeps it.
	UUID string `json:"uuid"`
	// RequestID and Request are a control_request's (can_use_tool) and a
	// control_cancel_request's.
	RequestID string          `json:"request_id"`
	Request   json.RawMessage `json:"request"`
	// State is a session_state_changed line's: running, requires_action,
	// or idle. PermissionMode is a status line's, after a mode change.
	State          string `json:"state"`
	PermissionMode string `json:"permissionMode"`
	// Tasks is a background_tasks_changed line's whole current list.
	Tasks *[]json.RawMessage `json:"tasks"`
	// ToolUseID and SubagentType are a task_started line's: the Agent call
	// that started it, and the subagent type it asked for.
	ToolUseID    string `json:"tool_use_id"`
	SubagentType string `json:"subagent_type"`
	// CompactResult and CompactError report a compaction that failed.
	CompactResult string `json:"compact_result"`
	CompactError  string `json:"compact_error"`
	// Error, APIError, and APIErrorParams are a failed request's typed
	// error: on an assistant line (authentication_failed, model_not_found,
	// rate_limit; usage_limit_reached) and on an api_retry line, with that
	// line's Attempt, MaxRetries, and ErrorStatus. Kept raw: a shape this
	// mapper does not expect must not cost the whole line.
	Error          json.RawMessage `json:"error"`
	APIError       json.RawMessage `json:"api_error"`
	APIErrorParams *struct {
		RateLimit *claudeRateLimit `json:"rate_limit_info"`
	} `json:"api_error_params"`
	Attempt     json.RawMessage `json:"attempt"`
	MaxRetries  json.RawMessage `json:"max_retries"`
	ErrorStatus json.RawMessage `json:"error_status"`
	// ParentToolUseID names the Agent call whose subagent wrote the line.
	ParentToolUseID string `json:"parent_tool_use_id"`
	// TaskID, TaskType, Description, Status, and Patch describe a
	// background task's system lines (task_started, task_updated,
	// task_notification).
	TaskID          string `json:"task_id"`
	TaskType        string `json:"task_type"`
	Description     string `json:"description"`
	Summary         string `json:"summary"`
	IsBackgrounded  *bool  `json:"is_backgrounded"`
	OwnedBySubagent bool   `json:"owned_by_subagent"`
	Status          string `json:"status"`
	Patch           struct {
		Status string `json:"status"`
	} `json:"patch"`
	Response struct {
		Subtype   string          `json:"subtype"`
		RequestID string          `json:"request_id"`
		Error     string          `json:"error"`
		Response  json.RawMessage `json:"response"`
	} `json:"response"`
	RateLimit *claudeRateLimit `json:"rate_limit_info"`
	IsError       bool     `json:"is_error"`
	Result        string   `json:"result"`
	Errors        []string `json:"errors"`
	TerminalState string   `json:"terminal_reason"`
	TotalCostUSD  *float64 `json:"total_cost_usd"`
	ModelUsage    map[string]struct {
		ContextWindow int64 `json:"contextWindow"`
	} `json:"modelUsage"`
	CompactMetadata *struct {
		Trigger string `json:"trigger"`
	} `json:"compact_metadata"`
}

// claudeRateLimit is a rate-limit report: a rate_limit_event's, or the one
// a usage-limit refusal carries.
type claudeRateLimit struct {
	Status         string `json:"status"`
	RateLimitType  string `json:"rateLimitType"`
	ResetsAt       int64  `json:"resetsAt"`
	OverageStatus  string `json:"overageStatus"`
	IsUsingOverage bool   `json:"isUsingOverage"`
	UnifiedWindows map[string]struct {
		Utilization *float64 `json:"utilization"`
		ResetsAt    int64    `json:"resetsAt"`
	} `json:"unifiedWindows"`
}

type claudeMessage struct {
	ID      string          `json:"id"`
	Model   string          `json:"model"`
	Content json.RawMessage `json:"content"`
	Usage   *struct {
		InputTokens              int64 `json:"input_tokens"`
		CacheReadInputTokens     int64 `json:"cache_read_input_tokens"`
		CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
		OutputTokens             int64 `json:"output_tokens"`
	} `json:"usage"`
}

type claudeBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text"`
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Input     json.RawMessage `json:"input"`
	ToolUseID string          `json:"tool_use_id"`
	IsError   bool            `json:"is_error"`
	// Content is a tool_result's content: a string, or text blocks.
	Content json.RawMessage `json:"content"`
}

// claudeResultText is a tool_result's content as text: the string itself,
// or its text blocks joined.
func claudeResultText(raw json.RawMessage) string {
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return text
	}
	var blocks []claudeBlock
	if json.Unmarshal(raw, &blocks) != nil {
		return ""
	}
	var parts []string
	for _, b := range blocks {
		if b.Type == "text" {
			parts = append(parts, b.Text)
		}
	}
	return strings.Join(parts, "\n")
}

func (m *claudeMapper) mapLine(line []byte, replies lineReplies) []Event {
	evs := m.mapEvents(line, replies)
	if slices.ContainsFunc(evs, func(ev Event) bool { return ev.TurnStart }) {
		m.ended = false
	}
	return evs
}

func (m *claudeMapper) mapEvents(line []byte, replies lineReplies) []Event {
	var l claudeLine
	if json.Unmarshal(line, &l) != nil {
		return nil
	}
	now := m.now().UnixMilli()
	switch l.Type {
	case "control_response":
		replies.answer(l.Response.RequestID, controlAnswer{ok: l.Response.Subtype == "success", err: l.Response.Error, body: l.Response.Response})
		if strings.HasPrefix(l.Response.RequestID, claudeContextAsk) {
			return m.contextUsage(l)
		}
		if l.Response.RequestID == claudeCommandsAsk && m.lead() {
			return m.initialized(l)
		}
	case "control_request":
		if m.lead() {
			return m.approval(l, now, replies)
		}
	case "control_cancel_request":
		// The harness withdrew a prompt: it is settled with no answer.
		if m.lead() && l.RequestID != "" {
			for toolUse, id := range m.approvals {
				if id == l.RequestID {
					delete(m.approvals, toolUse)
				}
			}
			return []Event{{ApprovalDone: l.RequestID}}
		}
	case "system":
		if l.CompactResult == "failed" {
			// A compaction that failed: it settles as a failed compaction
			// line carrying the harness's own error text.
			m.compacted = false
			return []Event{{Live: &wslive.Event{Kind: wslive.KindCompact, Phase: wslive.CompactFailed, Text: l.CompactError}}}
		}
		switch l.Subtype {
		case "api_retry":
			if !m.lead() {
				return nil
			}
			m.retrying = true
			r := wslive.Retry{Attempt: rawInt(l.Attempt), MaxRetries: rawInt(l.MaxRetries), Error: rawString(l.Error), Status: rawInt(l.ErrorStatus)}
			if r.MaxRetries < r.Attempt {
				r.MaxRetries = 0
			}
			return []Event{{Retry: &r}}
		case "init":
			var init claudeInit
			if json.Unmarshal(line, &init) != nil {
				return []Event{{Fatal: errors.New("launch refused: unreadable claude init message")}}
			}
			if m.lead() {
				return m.leadInit(init, replies)
			}
			if err := checkClaudeInit(init, m.spec); err != nil {
				return []Event{{Fatal: err}}
			}
			if init.SessionID != "" {
				return []Event{{SessionID: init.SessionID}}
			}
		case "session_state_changed":
			// running opens a turn whether or not a send began it (a
			// background task's result, a pushed message), and again after
			// a prompt (requires_action) inside one, which the runner takes
			// as the turn it already runs; idle ends the turn.
			if !m.lead() {
				return nil
			}
			switch l.State {
			case HarnessRunning:
				m.states = true
				return []Event{{HarnessState: l.State}, {TurnStart: true}}
			case HarnessWaiting:
				m.states = true
				return []Event{{HarnessState: l.State}}
			case HarnessIdle:
				m.states = true
				if m.ended {
					// The result before it ended the turn (states).
					m.ended = false
					return []Event{{HarnessState: l.State}}
				}
				out := append([]Event{{HarnessState: l.State}}, m.closeTurn(now)...)
				out = append(out, m.status(), Event{TurnEnd: true, TurnError: m.turnErr})
				m.turnErr = ""
				return out
			}
		case "status":
			if !m.lead() {
				return nil
			}
			var out []Event
			if l.PermissionMode != "" {
				out = append(out, Event{PermissionMode: l.PermissionMode})
			}
			// A compaction is a turn of its own when no send started it.
			if l.Status == "compacting" {
				out = append(out, Event{TurnStart: true}, Event{Live: &wslive.Event{Kind: wslive.KindCompact, Phase: wslive.CompactStart}})
			}
			return out
		case "background_tasks_changed":
			if m.lead() && l.Tasks != nil {
				return []Event{m.backgroundCounts(*l.Tasks)}
			}
		case "compact_boundary":
			m.compacted = true
			trigger := ""
			if l.CompactMetadata != nil && (l.CompactMetadata.Trigger == wslive.TriggerAuto || l.CompactMetadata.Trigger == wslive.TriggerManual) {
				trigger = l.CompactMetadata.Trigger
			}
			return []Event{{Live: &wslive.Event{Kind: wslive.KindCompact, Phase: wslive.CompactEnd, Trigger: trigger}}}
		case "task_started":
			if m.lead() && l.TaskID != "" {
				m.rememberWakeTask(l, false)
			}
			if l.TaskType == claudeAgentTask && l.TaskID != "" {
				m.tasks[l.TaskID] = true
				ev := wslive.Event{Kind: wslive.KindAgent, AgentID: l.TaskID, Status: wslive.StatusRunning, Label: l.Description}
				if m.lead() && l.ToolUseID != "" {
					spawn, ok := m.spawns[l.ToolUseID]
					if !ok {
						typ, _ := json.Marshal(map[string]string{"subagent_type": l.SubagentType})
						spawn = claudeSpawnOf(typ)
					}
					delete(m.spawns, l.ToolUseID)
					m.subs[l.ToolUseID] = &claudeSub{task: l.TaskID}
					ev.Model, ev.Effort = spawn.model, spawn.effort
				}
				return []Event{{Live: &ev}}
			}
		case "task_updated", "task_notification":
			var out []Event
			if m.lead() && l.Subtype == "task_notification" {
				out = m.taskWake(l)
			}
			status := l.Status
			if l.Subtype == "task_updated" {
				status = l.Patch.Status
			}
			if end := claudeTaskEnd(status); end != "" && m.tasks[l.TaskID] {
				delete(m.tasks, l.TaskID)
				for toolUse, sub := range m.subs {
					if sub.task == l.TaskID {
						delete(m.subs, toolUse)
					}
				}
				out = append(out, Event{Live: &wslive.Event{Kind: wslive.KindAgent, AgentID: l.TaskID, Status: end}})
			}
			return out
		}
	case "assistant":
		if l.ParentToolUseID != "" {
			return m.subagentReply(l, now)
		}
		if l.UUID != "" {
			if slices.Contains(m.uuids, l.UUID) {
				if m.compacted {
					return nil
				}
			} else {
				m.uuids = append(m.uuids, l.UUID)
				if len(m.uuids) > claudeSeenUUIDs {
					m.uuids = m.uuids[len(m.uuids)-claudeSeenUUIDs:]
				}
			}
		}
		alert := m.alert(l)
		out := m.assistant(l.Message, now, alert != nil)
		if alert != nil {
			out = append(out, Event{Alert: alert})
		}
		if m.lead() {
			if m.retrying {
				m.retrying = false
				out = append([]Event{{RetryEnd: true}}, out...)
			}
			// The harness works: a turn runs, whether or not a send began it.
			out = append([]Event{{TurnStart: true}}, out...)
		}
		return out
	case "user":
		settled := m.settled(l.Message)
		if l.ParentToolUseID != "" {
			return settled
		}
		return append(settled, m.toolResults(l.Message, now)...)
	case "rate_limit_event":
		if l.RateLimit != nil {
			m.rateLimit(*l.RateLimit)
			for kind, w := range l.RateLimit.UnifiedWindows {
				if (kind == wslive.LimitFiveHour || kind == wslive.LimitSevenDay) && w.Utilization != nil {
					m.limits[kind] = wslive.UsageLimit{Kind: kind, Percent: *w.Utilization * 100, ResetsAt: w.ResetsAt * 1000}
				}
			}
			return []Event{m.status()}
		}
	case "result":
		out := m.closeTurn(now)
		if m.retrying {
			m.retrying = false
			out = append(out, Event{RetryEnd: true})
		}
		for _, mu := range l.ModelUsage {
			if mu.ContextWindow > m.window {
				m.window = mu.ContextWindow
			}
		}
		if l.TotalCostUSD != nil {
			m.cost = &wslive.UsageCost{USD: *l.TotalCostUSD, Estimate: true}
		}
		out = append(out, m.status())
		if m.states && m.background == 0 {
			// The idle state that follows ends the turn.
			m.turnErr = claudeResultError(l)
			return out
		}
		m.ended, m.turnErr = m.states, ""
		return append(out, Event{TurnEnd: true, TurnError: claudeResultError(l)})
	}
	return nil
}

// claudeResultError is the error a result line ends its turn with, or
// empty. An interrupted turn ends as error_during_execution with an
// aborted_* terminal reason (aborted_streaming, aborted_tools); that is the
// interrupt asked for, not a failure.
func claudeResultError(l claudeLine) string {
	if (!l.IsError && l.Subtype == "success") || strings.HasPrefix(l.TerminalState, "aborted") {
		return ""
	}
	if msg := strings.TrimSpace(l.Result); msg != "" {
		return msg
	}
	if msg := strings.Join(l.Errors, "; "); msg != "" {
		return msg
	}
	return "turn failed: " + l.Subtype
}

// closeTurn is what a result and any end of a turn settle: a prompt still
// open will never be answered, the open tool batch is done, and a
// compaction's re-emitted message has passed.
func (m *claudeMapper) closeTurn(now int64) []Event {
	var out []Event
	m.compacted = false
	for toolUse, id := range m.approvals {
		delete(m.approvals, toolUse)
		out = append(out, Event{ApprovalDone: id})
	}
	if b := m.batches.finish(now, true); b != nil {
		out = append(out, *b)
	}
	return out
}

// leadInit maps a lead's init message, which every user turn brings: the
// session id, the turn's start, the permission mode the harness reports
// (it may differ from the one asked for), its MCP servers' status, and its
// slash commands (names only; the state keeps the list unchanged when a
// later init repeats it). It
// also asks for the compaction window when none is known for the init's
// model (askContextUsage).
func (m *claudeMapper) leadInit(init claudeInit, replies lineReplies) []Event {
	m.model = init.Model
	m.askContextUsage(replies)
	var out []Event
	if init.SessionID != "" {
		out = append(out, Event{SessionID: init.SessionID})
	}
	out = append(out, Event{TurnStart: true})
	if init.PermissionMode != "" {
		out = append(out, Event{PermissionMode: init.PermissionMode})
	}
	if len(init.MCPServers) > 0 {
		servers := make([]MCPServer, 0, len(init.MCPServers))
		for _, s := range init.MCPServers {
			servers = append(servers, MCPServer{Name: s.Name, Status: s.Status})
		}
		out = append(out, Event{MCPServers: servers})
	}
	if init.SlashCommands != nil {
		list := make([]wslive.SlashCommand, 0, len(*init.SlashCommands))
		for _, name := range *init.SlashCommands {
			list = append(list, wslive.SlashCommand{Name: name, Description: m.described[name]})
		}
		out = append(out, Event{Live: &wslive.Event{Kind: wslive.KindCommands, Commands: list}})
	}
	return out
}

// claudeCommandsAsk is the request id of the initialize control a lead's
// launch sends for its slash commands (the control answers in print mode
// without a model turn, seen on 2.1.323).
const claudeCommandsAsk = "ws-commands"

// initialized reads the initialize answer's commands: the page's list until
// the first turn's init, which names the commands a message may run and
// replaces it. A name the harness hides (a leading "__") stays out.
func (m *claudeMapper) initialized(l claudeLine) []Event {
	var d struct {
		Commands []struct {
			Name        string `json:"name"`
			Description string `json:"description"`
		} `json:"commands"`
	}
	if l.Response.Subtype != "success" || json.Unmarshal(l.Response.Response, &d) != nil || len(d.Commands) == 0 {
		return nil
	}
	m.described = map[string]string{}
	list := make([]wslive.SlashCommand, 0, len(d.Commands))
	for _, c := range d.Commands {
		if c.Name == "" || strings.HasPrefix(c.Name, "__") {
			continue
		}
		m.described[c.Name] = c.Description
		list = append(list, wslive.SlashCommand{Name: c.Name, Description: c.Description})
	}
	return []Event{{Live: &wslive.Event{Kind: wslive.KindCommands, Commands: list}}}
}

// claudeContextAsk prefixes the request id of a get_context_usage ask the
// mapper sends; its answer is told from other control responses by it, also
// when replayed under a later mapper whose ask count differs.
const claudeContextAsk = "ws-context-"

// askContextUsage asks the harness for its context usage (Claude Code's
// get_context_usage control, answered in print mode without a model turn,
// seen on 2.1.322) unless a compaction window is known for the current
// model. A set_model applies from the next turn, whose init names the new
// model, so a model change asks again; an unanswered or refused ask is
// repeated at the next init. A compaction does not change the window and
// asks nothing.
func (m *claudeMapper) askContextUsage(replies lineReplies) {
	if m.compactWindow > 0 && m.compactModel == m.model {
		return
	}
	m.asks++
	m.askModel = m.model
	replies.send(claudeProtocol{}.requestLine(claudeContextAsk+strconv.Itoa(m.asks), map[string]any{"subtype": "get_context_usage"}))
}

// contextUsage reads a get_context_usage answer's compaction window
// (rawMaxTokens, the figure the terminal band shows) and reports the new
// window when a context figure is already shown.
func (m *claudeMapper) contextUsage(l claudeLine) []Event {
	var d struct {
		RawMaxTokens int64 `json:"rawMaxTokens"`
	}
	if l.Response.Subtype != "success" || json.Unmarshal(l.Response.Response, &d) != nil || d.RawMaxTokens <= 0 {
		return nil
	}
	changed := d.RawMaxTokens != m.compactWindow || m.askModel != m.compactModel
	m.compactWindow, m.compactModel = d.RawMaxTokens, m.askModel
	if !changed || m.context == nil {
		return nil
	}
	return []Event{m.status()}
}

// claudeCanUseTool is a can_use_tool control request's body.
type claudeCanUseTool struct {
	Subtype     string          `json:"subtype"`
	ToolName    string          `json:"tool_name"`
	Input       json.RawMessage `json:"input"`
	Description string          `json:"description"`
	ToolUseID   string          `json:"tool_use_id"`
	AgentID     string          `json:"agent_id"`
	// RequiresUserInteraction marks a tool whose prompt is a question to
	// the user (either spelling).
	RequiresUserInteraction  bool `json:"requires_user_interaction"`
	RequiresUserInteraction2 bool `json:"requiresUserInteraction"`
}

// approval maps a lead's control request. A can_use_tool request becomes a
// waiting prompt; one that names no tool, tool call, or input is denied at
// once rather than left hanging, and so is a tool whose prompt is a
// question to the user. Any other control request has no handler here: it
// is refused with an error answer, so the harness does not wait on it, and
// noted for the log.
func (m *claudeMapper) approval(l claudeLine, now int64, replies lineReplies) []Event {
	if l.RequestID == "" {
		return nil
	}
	var req claudeCanUseTool
	if json.Unmarshal(l.Request, &req) != nil || req.Subtype != "can_use_tool" {
		replies.send(claudeControlError(l.RequestID, fmt.Sprintf("this session's host does not handle the control request %q", req.Subtype)))
		return []Event{{Notice: fmt.Sprintf("the harness sent a control request this session does not handle (%q); it was answered with an error", req.Subtype)}}
	}
	ar := ApprovalRequest{ID: l.RequestID, Tool: req.ToolName, Input: req.Input, Description: req.Description, ToolUseID: req.ToolUseID, AgentID: req.AgentID, StartedAt: now}
	input := bytes.TrimSpace(req.Input)
	switch {
	case req.ToolName == "" || req.ToolUseID == "" || len(input) == 0 || input[0] != '{':
		replies.send(claudeApprovalResponse(ar, false, claudeMalformedDenial))
		return nil
	case req.ToolName == "ExitPlanMode":
		replies.send(claudeApprovalResponse(ar, false, claudePlanDenial))
		return nil
	case req.ToolName == "AskUserQuestion" || req.RequiresUserInteraction || req.RequiresUserInteraction2:
		replies.send(claudeApprovalResponse(ar, false, claudeChatOnlyDenial))
		return nil
	}
	m.approvals[req.ToolUseID] = l.RequestID
	return []Event{{Approval: &ar}}
}

// settled returns an ApprovalDone for each waiting prompt whose tool call a
// user message brings the result of: the prompt was answered. A subagent's
// tool results count too.
func (m *claudeMapper) settled(raw json.RawMessage) []Event {
	if len(m.approvals) == 0 {
		return nil
	}
	var msg claudeMessage
	if json.Unmarshal(raw, &msg) != nil {
		return nil
	}
	var blocks []claudeBlock
	if json.Unmarshal(msg.Content, &blocks) != nil {
		return nil
	}
	var out []Event
	for _, b := range blocks {
		if id, ok := m.approvals[b.ToolUseID]; ok && b.Type == "tool_result" {
			delete(m.approvals, b.ToolUseID)
			out = append(out, Event{ApprovalDone: id})
		}
	}
	return out
}

// claudeSeenUUIDs is how many recent assistant line uuids the mapper keeps
// to recognise a compaction's re-emitted message.
const claudeSeenUUIDs = 64

// claudeAgentTask is the task type of a subagent the Agent tool started.
// Claude 2.1.319 runs it in the background: the Agent call's tool_result
// comes at once, and system task lines carry the subagent's start and end.
const claudeAgentTask = "local_agent"

// claudeTaskEnd maps a task status to the subagent's final status: done for
// completed, failed for any other terminal one, "" while it runs.
func claudeTaskEnd(status string) string {
	switch status {
	case "", "pending", "running":
		return ""
	case "completed":
		return wslive.StatusDone
	}
	return wslive.StatusFailed
}

// assistant maps the lead's assistant message: its text (left out when
// alerted, since the alert's line carries it) and its tool calls. A lead's
// message whose usage changes the context figure reports it (a status) at
// once, so the meter follows each model call rather than the turn's end; a
// message is often split over several lines carrying the same usage, and
// those report it once.
func (m *claudeMapper) assistant(raw json.RawMessage, now int64, alerted bool) []Event {
	var msg claudeMessage
	if json.Unmarshal(raw, &msg) != nil {
		return nil
	}
	var out []Event
	if msg.Usage != nil && !alerted {
		// An alert's message is the harness's own, with no real usage.
		tokens := msg.Usage.InputTokens + msg.Usage.CacheReadInputTokens + msg.Usage.CacheCreationInputTokens + msg.Usage.OutputTokens
		changed := m.context == nil || m.context.Tokens == nil || *m.context.Tokens != tokens
		m.context = &wslive.UsageContext{Tokens: &tokens}
		if changed && m.lead() {
			out = append(out, m.status())
		}
	}
	var blocks []claudeBlock
	if json.Unmarshal(msg.Content, &blocks) != nil {
		return out
	}
	for _, b := range blocks {
		switch b.Type {
		case "text":
			if strings.TrimSpace(b.Text) != "" && !alerted {
				out = append(out, Event{Live: &wslive.Event{Kind: wslive.KindReply, Text: b.Text, Final: true}})
			}
		case "tool_use":
			out = append(out, m.batches.addCall(msg.ID, b.ID, b.Name, argsPreview(b.Input), now)...)
			if !m.lead() {
				continue
			}
			if b.Name == "Agent" || b.Name == "Task" {
				m.spawns[b.ID] = claudeSpawnOf(b.Input)
			}
			if strings.HasSuffix(b.Name, claudeReconnectLever) {
				// The lead asks for an MCP reconnect: a server it needs is
				// gone, whatever the harness last reported.
				out = append(out, Event{MCPCheck: true})
			}
		}
	}
	if u := msg.Usage; u != nil && !alerted {
		out = append(out, m.batches.setUsage(msg.ID, &wslive.TokenUsage{
			Input: u.InputTokens, Output: u.OutputTokens,
			CacheRead: u.CacheReadInputTokens, CacheWrite: u.CacheCreationInputTokens,
		})...)
	}
	return out
}

func (m *claudeMapper) toolResults(raw json.RawMessage, now int64) []Event {
	var msg claudeMessage
	if json.Unmarshal(raw, &msg) != nil {
		return nil
	}
	var blocks []claudeBlock
	if json.Unmarshal(msg.Content, &blocks) != nil {
		return nil
	}
	var out []Event
	for _, b := range blocks {
		if b.Type == "tool_result" {
			name := m.batches.callName(b.ToolUseID)
			out = append(out, m.batches.endCall(b.ToolUseID, b.IsError, now)...)
			if m.lead() && b.IsError && strings.HasPrefix(name, "mcp__") {
				// A failed MCP tool call may be a server that went away.
				out = append(out, Event{MCPCheck: true})
			}
			if tool := resultTool(name, claudeMCPToolName); !b.IsError && tool != "" {
				out = append(out, resultEvents(tool, claudeResultText(b.Content))...)
			}
		}
	}
	return out
}

func (m *claudeMapper) status() Event {
	ev := wslive.Event{Kind: wslive.KindStatus, Cost: m.cost}
	if m.context != nil {
		c := *m.context
		c.Window = m.window
		if m.compactWindow > 0 && m.compactModel == m.model {
			c.Window = m.compactWindow
		}
		ev.Context = &c
	}
	for _, kind := range []string{wslive.LimitFiveHour, wslive.LimitSevenDay} {
		if l, ok := m.limits[kind]; ok {
			ev.Limits = append(ev.Limits, l)
		}
	}
	return Event{Live: &ev}
}
