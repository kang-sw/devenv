package wsagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

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

// claudeArgs is the claude command line for spec, without the executable.
func claudeArgs(spec StartSpec) ([]string, error) {
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
}

// checkClaudeInit verifies that the child sees only what spec permits: its
// tools are the profile's built-in tools, ws tools, and own servers' tools;
// its MCP servers are ws (when attached) and the profile's; every plugin
// is a built-in or one of the profile's plugin directories; and permissions
// run in dontAsk mode.
func checkClaudeInit(init claudeInit, spec StartSpec) error {
	allowed := map[string]bool{}
	for _, entry := range spec.Profile.Tools {
		allowed[ToolBase(entry)] = true
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
	command := b.Command
	if command == "" {
		path, err := lookPath("claude")
		if err != nil {
			return nil, fmt.Errorf("claude backend: %w", err)
		}
		command = path
	}
	args, err := claudeArgs(spec)
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(command, args...)
	cmd.Dir = spec.Root
	cmd.Env = claudeEnv(spec.Env)
	p, err := startStreamProcess(cmd, newClaudeMapper(spec), claudeProtocol{})
	if err != nil {
		return nil, err
	}
	return claudeProcess{p}, nil
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
type claudeProcess struct{ *streamProcess }

func (p claudeProcess) Interrupt() error {
	return p.request("interrupt", map[string]any{"subtype": "interrupt"})
}

// SetModel uses Claude Code's set_model control, which applies from the
// next turn. Returning to the default model has no in-stream form.
func (p claudeProcess) SetModel(model string) error {
	if model == "" {
		return ErrUnsupported
	}
	return p.request("set_model", map[string]any{"subtype": "set_model", "model": model})
}

// SetEffort has no in-stream form in Claude Code (2.1.319 answers
// set_effort with "Unsupported control request subtype"), so an effort
// change restarts the process with --effort on the same session.
func (claudeProcess) SetEffort(string) error { return ErrUnsupported }

// claudeMapper maps Claude Code's stream-json output to runner events: the
// init message to the session id (after the init check), assistant text to
// final replies, tool calls to the lead's batch (one per assistant
// message), rate-limit and result figures to usage, and the result to the
// turn's end.
type claudeMapper struct {
	spec    StartSpec
	now     func() time.Time
	batches batchTracker

	context *wslive.UsageContext
	limits  map[string]wslive.UsageLimit
	cost    *wslive.UsageCost
	window  int64
}

func newClaudeMapper(spec StartSpec) *claudeMapper {
	return &claudeMapper{spec: spec, now: time.Now, batches: newBatchTracker("c"), limits: map[string]wslive.UsageLimit{}}
}

type claudeLine struct {
	Type     string          `json:"type"`
	Subtype  string          `json:"subtype"`
	Message  json.RawMessage `json:"message"`
	Response struct {
		Subtype   string `json:"subtype"`
		RequestID string `json:"request_id"`
		Error     string `json:"error"`
	} `json:"response"`
	RateLimit *struct {
		RateLimitType  string `json:"rateLimitType"`
		ResetsAt       int64  `json:"resetsAt"`
		UnifiedWindows map[string]struct {
			Utilization *float64 `json:"utilization"`
			ResetsAt    int64    `json:"resetsAt"`
		} `json:"unifiedWindows"`
	} `json:"rate_limit_info"`
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

type claudeMessage struct {
	ID      string          `json:"id"`
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
}

func (m *claudeMapper) mapLine(line []byte, replies lineReplies) []Event {
	var l claudeLine
	if json.Unmarshal(line, &l) != nil {
		return nil
	}
	now := m.now().UnixMilli()
	switch l.Type {
	case "control_response":
		replies.answer(l.Response.RequestID, controlAnswer{ok: l.Response.Subtype == "success", err: l.Response.Error})
	case "system":
		switch l.Subtype {
		case "init":
			var init claudeInit
			if json.Unmarshal(line, &init) != nil {
				return []Event{{Fatal: errors.New("launch refused: unreadable claude init message")}}
			}
			if err := checkClaudeInit(init, m.spec); err != nil {
				return []Event{{Fatal: err}}
			}
			if init.SessionID != "" {
				return []Event{{SessionID: init.SessionID}}
			}
		case "compact_boundary":
			trigger := ""
			if l.CompactMetadata != nil && (l.CompactMetadata.Trigger == wslive.TriggerAuto || l.CompactMetadata.Trigger == wslive.TriggerManual) {
				trigger = l.CompactMetadata.Trigger
			}
			return []Event{{Live: &wslive.Event{Kind: wslive.KindCompact, Phase: wslive.CompactEnd, Trigger: trigger}}}
		}
	case "assistant":
		return m.assistant(l.Message, now)
	case "user":
		return m.toolResults(l.Message, now)
	case "rate_limit_event":
		if l.RateLimit != nil {
			for kind, w := range l.RateLimit.UnifiedWindows {
				if (kind == wslive.LimitFiveHour || kind == wslive.LimitSevenDay) && w.Utilization != nil {
					m.limits[kind] = wslive.UsageLimit{Kind: kind, Percent: *w.Utilization * 100, ResetsAt: w.ResetsAt * 1000}
				}
			}
			return []Event{m.status()}
		}
	case "result":
		var out []Event
		if b := m.batches.finish(now, true); b != nil {
			out = append(out, *b)
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
		end := Event{TurnEnd: true}
		// An interrupted turn ends as error_during_execution with an
		// aborted_* terminal reason (aborted_streaming, aborted_tools); that
		// is the interrupt asked for, not a failure.
		interrupted := strings.HasPrefix(l.TerminalState, "aborted")
		if (l.IsError || l.Subtype != "success") && !interrupted {
			msg := strings.TrimSpace(l.Result)
			if msg == "" {
				msg = strings.Join(l.Errors, "; ")
			}
			if msg == "" {
				msg = "turn failed: " + l.Subtype
			}
			end.TurnError = msg
		}
		return append(out, end)
	}
	return nil
}

func (m *claudeMapper) assistant(raw json.RawMessage, now int64) []Event {
	var msg claudeMessage
	if json.Unmarshal(raw, &msg) != nil {
		return nil
	}
	if msg.Usage != nil {
		tokens := msg.Usage.InputTokens + msg.Usage.CacheReadInputTokens + msg.Usage.CacheCreationInputTokens + msg.Usage.OutputTokens
		m.context = &wslive.UsageContext{Tokens: &tokens}
	}
	var blocks []claudeBlock
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
		case "tool_use":
			out = append(out, m.batches.addCall(msg.ID, b.ID, b.Name, argsPreview(b.Input), now)...)
		}
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
			out = append(out, m.batches.endCall(b.ToolUseID, b.IsError, now)...)
		}
	}
	return out
}

func (m *claudeMapper) status() Event {
	ev := wslive.Event{Kind: wslive.KindStatus, Cost: m.cost}
	if m.context != nil {
		c := *m.context
		c.Window = m.window
		ev.Context = &c
	}
	for _, kind := range []string{wslive.LimitFiveHour, wslive.LimitSevenDay} {
		if l, ok := m.limits[kind]; ok {
			ev.Limits = append(ev.Limits, l)
		}
	}
	return Event{Live: &ev}
}
