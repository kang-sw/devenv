package wsagent

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/kang-sw/devenv/internal/claudeagents"
	"github.com/kang-sw/devenv/internal/wsharnessenv"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
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

func (b ClaudeBackend) registration() (string, BackendRegistration) {
	return BackendClaude, BackendRegistration{
		Backend:          b,
		LeadProfile:      LeadProfileClaude,
		PermissionModes:  LeadPermissionModes,
		PermissionLevels: LeadPermissionLevels(),
		Catalog:          ClaudeCatalog(),
		ManualHarness:    BackendClaude,
		PlaybookCommands: true,
		WorkerCapable:    true,
		WorkerProfile:    AgentWorkerProfile(BackendClaude),
		WorkerHTTPMCP:    true,
		PlaybookTerminology: PlaybookTerminology{
			ExploreAgent:  "the Explore agent",
			ContinueIdiom: "SendMessage(to: <agentId>)",
		},
		Profiles: []BuiltinProfileRegistration{
			supportBuiltinProfile(SupportProfileClaude, BackendClaude, SupportClaudeTools),
			agentWorkerBuiltinProfile(BackendClaude, AgentClaudeTools),
			searchAnswerBuiltinProfile(SearchAnswerProfileClaude, BackendClaude, []string{"Read", "Grep", "Glob"}),
			leadBuiltinProfile(LeadProfileClaude, BackendClaude, DefaultLeadPermissionMode),
		},
	}
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
// session's CLAUDE* variables (claudeKeptEnv aside) and inherited ws harness
// identity, which the child's ws-mcp would otherwise claim.
func claudeEnv(env []string) []string {
	return harnessEnv(wsharnessenv.Drop(env), []string{"CLAUDE"}, claudeKeptEnv, nil)
}

// claudeLeadEnv is a lead conversation's environment: env without the
// parent harness session's CLAUDE* and ws variables, plus the hosted-session
// variables.
func claudeLeadEnv(spec StartSpec) []string {
	// The session-state lines are what tell a turn nobody sent from an idle
	// harness (session_state_changed in mapLine).
	set := map[string]string{"CLAUDE_CODE_EMIT_SESSION_STATE_EVENTS": "1"}
	for name, value := range spec.Lead.Env {
		set[name] = value
	}
	return harnessEnv(wsharnessenv.Drop(spec.Env), []string{"CLAUDE"}, claudeKeptEnv, set)
}

// claudeLeadArgs is the command line of a lead conversation: print mode
// with stream-json both ways, user/project settings, hooks, memory, and MCP
// servers. No ws plugin loads (claudeLeadPluginArgs): the daemon's ws-mcp is
// the MCP server ReservedMCPServer (claudeLeadMCPArgs), whose tools run
// without a prompt, and the instruction file carries the key and the
// workflow manual, appended to the default system prompt. Other tool
// permission prompts come to the daemon (--permission-prompt-tool stdio).
// The user's tools stay, less the native subagent tool (claudeRemovedTools).
func claudeLeadArgs(spec StartSpec) ([]string, error) {
	plugins, err := claudeLeadPluginArgs(spec)
	if err != nil {
		return nil, err
	}
	servers, err := claudeLeadMCPArgs(spec)
	if err != nil {
		return nil, err
	}
	mode := spec.Lead.PermissionMode
	if mode == "" {
		mode = DefaultLeadPermissionMode
	}
	args := []string{
		"-p",
		"--input-format", "stream-json",
		"--output-format", "stream-json",
		"--verbose",
		// The reply streams into the conversation view as it is written
		// (stream_event lines; the mapper's partial).
		"--include-partial-messages",
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
	if spec.Instructions != "" {
		args = append(args, "--append-system-prompt-file", spec.Instructions)
	}
	if removed := claudeRemovedTools(spec); len(removed) > 0 {
		args = append(args, "--disallowedTools", strings.Join(removed, ","))
	}
	args = append(append(args, plugins...), servers...)
	// The allowlist goes last: the option takes every following argument.
	// The rule names the whole server, as a worker's rules name its own
	// servers (claudeAllowedRules).
	return append(args, "--allowedTools", "mcp__"+claudeNamePart(ReservedMCPServer)), nil
}

// claudeRemovedTools are the built-in tools spec's launch removes with
// --disallowedTools, over a worker's --tools and a lead's own settings.
// The variadic option receives the list as one comma-separated value.
func claudeRemovedTools(spec StartSpec) []string {
	var removed []string
	if spec.RemoveSubagents {
		removed = append(removed, claudeSubagentTool, claudeInitToolNames[claudeSubagentTool])
	}
	if spec.Lead == nil && !spec.Profile.Skills {
		// The slash-command switch disables native /compact. Deny model
		// invocation through Skill while preserving typed local commands.
		removed = append(removed, "Skill")
	}
	return removed
}

// claudeArgs is the claude command line for spec, without the executable.
func claudeArgs(spec StartSpec) ([]string, error) {
	if spec.Lead != nil {
		return claudeLeadArgs(spec)
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
	if removed := claudeRemovedTools(spec); len(removed) > 0 {
		args = append(args, "--disallowedTools", strings.Join(removed, ","))
	}
	// The allowlist goes last: the option takes every following argument.
	if allowed := claudeAllowedRules(spec); len(allowed) > 0 {
		args = append(args, "--allowedTools")
		args = append(args, allowed...)
	}
	return args, nil
}

// claudeAllowedRules are the permission rules the child runs without a
// prompt: the profile's tool entries, its ws tools (restricted to
// ChildSafeWSTools when launched at PermissionReadOnly), and every tool of its
// own MCP servers. dontAsk denies everything else.
func claudeAllowedRules(spec StartSpec) []string {
	rules := append([]string{}, spec.Profile.Tools...)
	if spec.WSMCP != nil {
		wsTools := spec.Profile.WSTools
		if spec.PermissionLevel == PermissionReadOnly {
			wsTools = childSafeWSToolsFor(spec.Profile)
		}
		for _, name := range wsTools {
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
	switch {
	case spec.WSMCP != nil && spec.MCPSession != nil:
		servers[ReservedMCPServer] = claudeHTTPServer(spec.MCPSession.URL)
	case spec.WSMCP != nil:
		servers[ReservedMCPServer] = map[string]any{"type": "stdio", "command": spec.WSMCP.Command, "args": spec.WSMCP.Args, "env": spec.WSMCP.Env}
	}
	raw, err := json.Marshal(map[string]any{"mcpServers": servers})
	return string(raw), err
}

// claudeHTTPServer is the ws-mcp server entry for the daemon's HTTP route
// at url. Claude Code expands the token from the harness's environment
// (EnvMCPSessionToken) when it connects, so the token is in no argument
// and no file.
func claudeHTTPServer(url string) map[string]any {
	return map[string]any{
		"type":    "http",
		"url":     url,
		"headers": map[string]any{"Authorization": "Bearer ${" + EnvMCPSessionToken + "}"},
	}
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
	// Version is the harness's version (claude_code_version).
	Version string `json:"claude_code_version"`
}

// claudeInitShape is the light init check of the capability catalog: the
// items whose init field does not show what their grade relies on, to be
// demoted to unsupported. It never refuses a launch (checkClaudeInit stays
// a worker's fatal check). The mapping:
//
//	tools          -> tool removal, subagent removal: the field is absent,
//	                  a tool of the profile the launch did not remove
//	                  (claudeRemovedTools) is missing under its name and its
//	                  older init name (claudeInitToolNames), or a tool the
//	                  launch removed is still listed
//	permissionMode -> approval: the field is absent
//	slash_commands -> compaction: a list without "compact"; an absent field
//	                  (an older harness) changes nothing
//	model          -> model/effort: the field is absent
//	mcp_servers    -> none: MCP injection is an admission condition, and a
//	                  worker's init check refuses a missing ws server
//	plugins        -> none
//
// Steer, interrupt, follow-up, resume, usage query and the other items have
// no init field: their grade follows the version alone.
func claudeInitShape(init claudeInit, spec StartSpec) []CapabilityItem {
	var out []CapabilityItem
	tools := init.Tools != nil
	if tools {
		listed := map[string]bool{}
		for _, name := range init.Tools {
			listed[name] = true
		}
		removed := claudeRemovedTools(spec)
		for _, entry := range spec.Profile.Tools {
			base := ToolBase(entry)
			if !slices.Contains(removed, base) && !listed[base] && !listed[claudeInitToolNames[base]] {
				tools = false
			}
		}
		for _, name := range removed {
			if listed[name] {
				tools = false
			}
		}
	}
	if !tools {
		out = append(out, ItemToolRemoval, ItemSubagentRemoval)
	}
	if init.PermissionMode == "" {
		out = append(out, ItemApproval)
	}
	if init.SlashCommands != nil && !slices.Contains(*init.SlashCommands, "compact") {
		out = append(out, ItemCompaction)
	}
	if init.Model == "" {
		out = append(out, ItemModelEffort)
	}
	return out
}

// harnessReport is the catalog report of a process's init.
func (m *claudeMapper) harnessReport(init claudeInit) Event {
	return Event{Harness: &HarnessReport{Version: init.Version, Demoted: claudeInitShape(init, m.spec)}}
}

// claudeInitToolNames maps a built-in tool name to the older name Claude's
// system/init may report for it: Claude 2.1.319 launched with --tools Agent
// lists the tool as "Task" in init while its calls are named "Agent".
var claudeInitToolNames = map[string]string{"Agent": "Task"}

// checkClaudeInit verifies that the child sees only what spec permits: its
// tools are the profile's built-in tools less the ones the launch removed
// (claudeRemovedTools), ws tools, and own servers' tools; its MCP servers
// are ws (when attached) and the profile's; every plugin is a built-in or
// one of the profile's plugin directories; and permissions run in dontAsk
// mode.
func checkClaudeInit(init claudeInit, spec StartSpec) error {
	allowed := map[string]bool{}
	for _, entry := range spec.Profile.Tools {
		base := ToolBase(entry)
		allowed[base] = true
		if legacy, ok := claudeInitToolNames[base]; ok {
			allowed[legacy] = true
		}
	}
	for _, name := range claudeRemovedTools(spec) {
		delete(allowed, name)
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
	// Read before the start, which fills the holder id for a spawn too.
	attach := spec.Holder != nil && spec.Holder.id != ""
	var args []string
	if !attach {
		args, err = claudeArgs(spec)
		if err != nil {
			return nil, err
		}
	}
	// A rejoin attaches to an already-running process; startChild
	// ignores this command. Its launch-time settings and bundle may have
	// changed since launch and must not block recovery of that process.
	cmd := exec.Command(command, append(pre, args...)...)
	cmd.Dir = spec.workDir()
	cmd.Env = claudeEnv(spec.Env)
	if spec.Lead != nil {
		cmd.Env = claudeLeadEnv(spec)
		if spec.Holder != nil {
			spec.Holder.ringSkip = claudeRingSkip
		}
	}
	cmd.Env = mcpSessionEnv(cmd.Env, spec.MCPSession)
	p, err := startChild(spec, cmd, newClaudeMapper(spec), claudeProtocol{})
	if err != nil {
		return nil, err
	}
	if s := spec.MCPSession; s != nil && s.Started != nil {
		pid := 0
		if cmd.Process != nil {
			pid = cmd.Process.Pid
		} else if _, held, ok := p.HolderInfo(); ok {
			pid = held
		}
		s.Started(cmd.Env, pid)
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
