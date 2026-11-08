// Package wsagent is the dashboard daemon's agent runner: it starts headless
// harness processes (Claude Code, Pi) from named profiles, feeds them user
// turns, and records their output in a dashboard session directory through
// internal/wslive, so the daemon's conversation renderer and SSE serve them
// unchanged. Consumers drive conversations over the daemon's REST routes
// (http.go).
//
// The package depends on wsconfig, wsdashboard, wslive, and wsinbox, never
// on internal/mcp (which imports wsdashboard): ws-mcp code that needs the
// profile family imports this package, and the daemon command wires the
// runner in with the session-key minting it needs.
package wsagent

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/kang-sw/devenv/internal/wsconfig"
)

// ProfilePrefix is the config key family one agent profile lives under:
// agent.profile.<name>. A profile is a structured value (a config file's
// structured_overrides member), resolved through a conversation's working
// root: project, then repo (the root's committed file), then global. No
// profile is built in.
const ProfilePrefix = "agent.profile."

// Backend names a profile may select.
const (
	BackendClaude = "claude"
	BackendPi     = "pi"
)

// DefaultIdleTimeout ends a sustained process that has had no running turn
// and no input for this long, when its profile sets no idle_timeout_seconds.
const DefaultIdleTimeout = 10 * time.Minute

// ReservedMCPServer is the MCP server name the runner gives the attached
// ws-mcp; a profile's own mcp_servers may not use it.
const ReservedMCPServer = "ws"

var (
	profileNamePattern = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
	modelPattern       = regexp.MustCompile(`^[A-Za-z0-9._:/\[\]-]{1,128}$`)
	effortPattern      = regexp.MustCompile(`^[a-z]{1,16}$`)
	toolBasePattern    = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{0,63}$`)
	wsToolPattern      = regexp.MustCompile(`^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)*$`)
	serverNamePattern  = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
)

// Profile is the normative profile shape. Field names are a committed
// on-disk format (structured_overrides in a config file); do not rename
// them. Every capability defaults off: no built-in tool, no ws tool, no MCP
// server, no plugin, no skill.
type Profile struct {
	Description string `json:"description,omitempty"`
	// Backend is the harness: BackendClaude or BackendPi.
	Backend string `json:"backend"`
	// Model and Effort are the conversation's starting values; empty leaves
	// the harness default.
	Model  string `json:"model,omitempty"`
	Effort string `json:"effort,omitempty"`
	// Tools is the built-in tool allowlist. An entry is a tool name or, for
	// claude, a permission rule ("Bash(git status)"): the tool before the
	// parenthesis is made visible and the whole rule is allowed.
	Tools []string `json:"tools,omitempty"`
	// WSTools is the ws tool allowlist, by ws tool name ("tickets.query").
	// Any entry attaches ws-mcp, which advertises and accepts only these.
	WSTools []string `json:"ws_tools,omitempty"`
	// MCPServers are further MCP servers the agent may use, by server name;
	// each value is the harness's server object. Every tool of a listed
	// server is allowed.
	MCPServers map[string]json.RawMessage `json:"mcp_servers,omitempty"`
	// Plugins are plugin directories (claude) or extension paths (pi) to
	// load; a relative path resolves against the working root.
	Plugins []string `json:"plugins,omitempty"`
	// Skills keeps the harness's skills and slash commands on.
	Skills bool `json:"skills,omitempty"`
	// Instructions is the instruction file appended to the harness's own
	// system prompt; a relative path resolves against the working root. It
	// holds task instructions, never a harness persona.
	Instructions string `json:"instructions,omitempty"`
	// IdleTimeoutSeconds ends the sustained process after that long with no
	// running turn and no input; 0 never ends it, and absent is
	// DefaultIdleTimeout.
	IdleTimeoutSeconds *int `json:"idle_timeout_seconds,omitempty"`
	// ListInIndex lists the profile's conversations in the dashboard
	// session index; they are hidden from it by default.
	ListInIndex bool `json:"list_in_index,omitempty"`
}

// IdleTimeout is the profile's idle timeout; zero means never.
func (p Profile) IdleTimeout() time.Duration {
	if p.IdleTimeoutSeconds == nil {
		return DefaultIdleTimeout
	}
	return time.Duration(*p.IdleTimeoutSeconds) * time.Second
}

// InstructionsPath is the instruction file's absolute path under root, or
// "" when the profile names none.
func (p Profile) InstructionsPath(root string) string {
	return resolveUnder(root, p.Instructions)
}

// PluginPaths are the profile's plugin paths, each absolute under root.
func (p Profile) PluginPaths(root string) []string {
	out := make([]string, 0, len(p.Plugins))
	for _, path := range p.Plugins {
		out = append(out, resolveUnder(root, path))
	}
	return out
}

func resolveUnder(root, path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	path = filepath.FromSlash(path)
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Join(root, path)
}

// ToolBase is the tool name a Tools entry makes visible: the entry up to a
// permission rule's parenthesis.
func ToolBase(entry string) string {
	if i := strings.IndexByte(entry, '('); i >= 0 {
		return strings.TrimSpace(entry[:i])
	}
	return strings.TrimSpace(entry)
}

// MCPServerNames are the profile's MCP server names, sorted.
func (p Profile) MCPServerNames() []string {
	names := make([]string, 0, len(p.MCPServers))
	for name := range p.MCPServers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// ParseProfile strictly decodes and validates one profile value.
func ParseProfile(raw json.RawMessage) (Profile, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return Profile{}, fmt.Errorf("value must be a JSON object")
	}
	var p Profile
	dec := json.NewDecoder(bytes.NewReader(trimmed))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&p); err != nil {
		return Profile{}, fmt.Errorf("value does not match the agent profile schema: %w", err)
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return Profile{}, fmt.Errorf("value must be a single JSON object")
	}
	if err := p.validate(); err != nil {
		return Profile{}, err
	}
	return p, nil
}

func (p Profile) validate() error {
	switch p.Backend {
	case BackendClaude, BackendPi:
	case "":
		return fmt.Errorf("backend is required: %q or %q", BackendClaude, BackendPi)
	default:
		return fmt.Errorf("backend must be %q or %q; got %q", BackendClaude, BackendPi, p.Backend)
	}
	if p.Model != "" && !modelPattern.MatchString(p.Model) {
		return fmt.Errorf("model %q must match %s", p.Model, modelPattern.String())
	}
	if p.Effort != "" && !effortPattern.MatchString(p.Effort) {
		return fmt.Errorf("effort %q must be a lowercase word", p.Effort)
	}
	for i, entry := range p.Tools {
		if err := validateToolEntry(entry); err != nil {
			return fmt.Errorf("tools[%d]: %w", i, err)
		}
	}
	for i, name := range p.WSTools {
		if !wsToolPattern.MatchString(name) {
			return fmt.Errorf("ws_tools[%d] %q must be a ws tool name such as tickets.query", i, name)
		}
	}
	for name, value := range p.MCPServers {
		if !serverNamePattern.MatchString(name) {
			return fmt.Errorf("mcp_servers name %q must match %s", name, serverNamePattern.String())
		}
		if name == ReservedMCPServer {
			return fmt.Errorf("mcp_servers may not name %q: the runner attaches ws-mcp under it when ws_tools lists any tool", ReservedMCPServer)
		}
		if trimmed := bytes.TrimSpace(value); len(trimmed) == 0 || trimmed[0] != '{' {
			return fmt.Errorf("mcp_servers.%s must be a server object", name)
		}
	}
	for i, path := range p.Plugins {
		if strings.TrimSpace(path) == "" {
			return fmt.Errorf("plugins[%d] must be non-empty", i)
		}
	}
	if p.Instructions != "" && strings.TrimSpace(p.Instructions) == "" {
		return fmt.Errorf("instructions must name a file")
	}
	if p.IdleTimeoutSeconds != nil && *p.IdleTimeoutSeconds < 0 {
		return fmt.Errorf("idle_timeout_seconds must be 0 (never) or more")
	}
	return nil
}

func validateToolEntry(entry string) error {
	if strings.TrimSpace(entry) != entry || entry == "" {
		return fmt.Errorf("tool entry %q must be non-empty with no surrounding space", entry)
	}
	if strings.ContainsAny(entry, ",\n\r\t") {
		return fmt.Errorf("tool entry %q may not contain a comma or a control character", entry)
	}
	if !toolBasePattern.MatchString(ToolBase(entry)) {
		return fmt.Errorf("tool entry %q must start with a tool name", entry)
	}
	if strings.Contains(entry, "(") && !strings.HasSuffix(entry, ")") {
		return fmt.Errorf("tool rule %q must end with ')'", entry)
	}
	return nil
}

// CheckProfileScope refuses a committed (repo-scope) profile that starts
// programs. The repo file comes with a checkout anyone with push access
// writes, and a conversation at that root runs as the daemon's user, so the
// fields that run a command without the model (MCP servers, plugin
// directories with their hooks) belong to the machine-local scopes.
func CheckProfileScope(p Profile, scope wsconfig.Scope) error {
	if scope != wsconfig.ScopeRepo {
		return nil
	}
	var fields []string
	if len(p.MCPServers) > 0 {
		fields = append(fields, "mcp_servers")
	}
	if len(p.Plugins) > 0 {
		fields = append(fields, "plugins")
	}
	if len(fields) == 0 {
		return nil
	}
	return fmt.Errorf("a repo-scope profile may not set %s: a committed profile may not start programs; define it in project or global scope", strings.Join(fields, " or "))
}

// ValidateProfileName enforces the family's name rule.
func ValidateProfileName(name string) error {
	if !profileNamePattern.MatchString(name) {
		return fmt.Errorf("agent profile name %q must match %s", name, profileNamePattern.String())
	}
	return nil
}

// ResolvedProfile is one profile after layered resolution: the winning
// scope's whole value and the file it came from.
type ResolvedProfile struct {
	Name       string         `json:"name"`
	Scope      wsconfig.Scope `json:"scope"`
	Source     string         `json:"source,omitempty"`
	Definition Profile        `json:"definition"`
}

// ResolveProfiles resolves the whole family through resolver. Any invalid
// entry fails the resolution, naming the file and key.
func ResolveProfiles(resolver *wsconfig.Resolver) ([]ResolvedProfile, error) {
	values, err := resolver.ListStructured(ProfilePrefix, nil)
	if err != nil {
		return nil, err
	}
	out := make([]ResolvedProfile, 0, len(values))
	for _, v := range values {
		p, err := resolvedValue(v)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}

// ResolveProfile resolves one profile through root's config layers (the
// project and global files, and root's committed repo file). Only the named
// profile is validated, so a broken unrelated profile never blocks a
// launch. An unknown name fails, listing the names that resolve.
func ResolveProfile(opts wsconfig.Options, root, name string) (ResolvedProfile, error) {
	if err := ValidateProfileName(name); err != nil {
		return ResolvedProfile{}, err
	}
	opts.RepoRoot = root
	resolver := wsconfig.NewResolver(opts, nil, nil, nil)
	values, err := resolver.ListStructured(ProfilePrefix, nil)
	if err != nil {
		return ResolvedProfile{}, err
	}
	var names []string
	for _, v := range values {
		if v.Key == ProfilePrefix+name {
			return resolvedValue(v)
		}
		names = append(names, strings.TrimPrefix(v.Key, ProfilePrefix))
	}
	if len(names) == 0 {
		return ResolvedProfile{}, fmt.Errorf("unknown agent profile %q: no agent profile resolves for %s", name, root)
	}
	return ResolvedProfile{}, fmt.Errorf("unknown agent profile %q for %s; known: %s", name, root, strings.Join(names, ", "))
}

func resolvedValue(v wsconfig.StructuredValue) (ResolvedProfile, error) {
	name := strings.TrimPrefix(v.Key, ProfilePrefix)
	where := string(v.Scope) + " scope"
	if v.Source != "" {
		where = fmt.Sprintf("%s (%s scope)", v.Source, v.Scope)
	}
	if err := ValidateProfileName(name); err != nil {
		return ResolvedProfile{}, fmt.Errorf("invalid agent profile key %q in %s: %w", v.Key, where, err)
	}
	p, err := ParseProfile(v.Value)
	if err == nil {
		err = CheckProfileScope(p, v.Scope)
	}
	if err != nil {
		return ResolvedProfile{}, fmt.Errorf("invalid agent profile key %q in %s: %w", v.Key, where, err)
	}
	return ResolvedProfile{Name: name, Scope: v.Scope, Source: v.Source, Definition: p}, nil
}

// ProfileValueSchema is the JSON Schema config.list publishes for the
// family's value. ParseProfile enforces the same rules; keep the two in step.
func ProfileValueSchema() map[string]any {
	stringList := func(desc string) map[string]any {
		return map[string]any{"type": "array", "items": map[string]any{"type": "string", "minLength": 1}, "description": desc}
	}
	return map[string]any{
		"$schema":              "https://json-schema.org/draft/2020-12/schema",
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"backend"},
		"properties": map[string]any{
			"description": map[string]any{"type": "string"},
			"backend":     map[string]any{"enum": []string{BackendClaude, BackendPi}, "description": "The harness the conversation runs in."},
			"model":       map[string]any{"type": "string", "pattern": modelPattern.String(), "description": "Starting model; absent leaves the harness default."},
			"effort":      map[string]any{"type": "string", "pattern": effortPattern.String(), "description": "Starting reasoning effort; absent leaves the harness default."},
			"tools":       stringList("Built-in tool allowlist: tool names, or claude permission rules such as \"Bash(git status)\". Absent allows none; everything not listed is denied without a prompt."),
			"ws_tools":    stringList("ws tool allowlist by ws tool name (tickets.query). Any entry attaches ws-mcp, which advertises and accepts only these."),
			"mcp_servers": map[string]any{
				"type":                 "object",
				"description":          "Further MCP servers the agent may use, by name (not \"ws\"); each value is the harness's server object. Absent allows none. Not allowed in a repo-scope profile.",
				"additionalProperties": map[string]any{"type": "object"},
			},
			"plugins":              stringList("Plugin directories (claude) or extension paths (pi) to load; relative paths resolve against the working root. Absent loads none. Not allowed in a repo-scope profile."),
			"skills":               map[string]any{"type": "boolean", "description": "Keep the harness's skills and slash commands on; off by default."},
			"instructions":         map[string]any{"type": "string", "minLength": 1, "description": "Task instruction file appended to the harness's own system prompt; a relative path resolves against the working root."},
			"idle_timeout_seconds": map[string]any{"type": "integer", "minimum": 0, "description": "End the sustained process after this many seconds with no running turn and no input; 0 never ends it; absent is 600."},
			"list_in_index":        map[string]any{"type": "boolean", "description": "List this profile's conversations in the dashboard session index; hidden by default."},
		},
	}
}
