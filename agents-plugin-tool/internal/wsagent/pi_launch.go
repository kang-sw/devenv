// Pi launch arguments, MCP setup, and process startup.

package wsagent

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/kang-sw/devenv/internal/wsharnessenv"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

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
			"cwd":      spec.workDir(),
			"exposure": "direct",
			"timeout":  piMCPTimeout.Seconds(),
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

// piExtensionPackage is the package.json name of the native ws Pi extension,
// the extension that registers the execution gateway and bridges daemon tools.
const piExtensionPackage = "ws-pi-bridge"

// PiMCPOnly reports whether rec is a Pi conversation that reaches ws only
// through Pi's builtin MCP client, without the native ws Pi extension and so
// without the tools that extension registers. A spawned conversation is
// MCP-only unless def's plugins load the extension, because piArgs starts it
// with --no-extensions; a hosted lead is MCP-only when piLeadLaunch fell back
// to the MCP bridge, identified by its extension-unavailable PluginWarning.
// Other launch warnings do not imply the native extension is absent.
// def is the conversation's launched profile; a nil def
// loads no plugin. Records of other backends are never MCP-only.
func PiMCPOnly(rec Record, def *Profile) bool {
	if rec.Backend != BackendPi {
		return false
	}
	if rec.Lead {
		return strings.Contains(rec.PluginWarning, piExtensionFallbackMarker)
	}
	if def == nil {
		return true
	}
	for _, path := range def.PluginPaths(rec.Root) {
		if isPiExtensionPath(path) {
			return false
		}
	}
	return true
}

// isPiExtensionPath reports whether the nearest package.json at or above path
// names the native ws Pi extension package; an extension path may name the
// package directory or any entry file inside it.
func isPiExtensionPath(path string) bool {
	if path == "" {
		return false
	}
	for dir := filepath.Clean(path); ; {
		if raw, err := os.ReadFile(filepath.Join(dir, "package.json")); err == nil {
			var manifest struct {
				Name string `json:"name"`
			}
			return json.Unmarshal(raw, &manifest) == nil && manifest.Name == piExtensionPackage
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return false
		}
		dir = parent
	}
}

// piLeadArgs is the base command line of a lead conversation. Plugin
// isolation is added by preparePiLeadLaunch; skills, settings and context
// files keep Pi's ordinary behavior. A terminal takeover keeps its session
// file (spec.SessionFile), in Pi's own session directory for it.
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
// and without inherited ws harness identity.
func piEnv(env []string) []string {
	return harnessEnv(wsharnessenv.Drop(env), []string{"WS_PI_"}, nil, map[string]string{"PI_SKIP_VERSION_CHECK": "1"})
}

// piWorkerEnv marks a daemon worker for an explicitly loaded native extension:
// it must report its own RPC task counts, not start the owner's intake feed.
func piWorkerEnv(env []string) []string {
	return append(piEnv(env), "WS_PI_SPAWN_ROLE=worker")
}

// piLeadEnv is a lead conversation's environment without the Pi and ws
// variables of the session that started the daemon, plus the hosted-session
// variables.
func piLeadEnv(spec StartSpec) []string {
	set := map[string]string{"PI_SKIP_VERSION_CHECK": "1"}
	for name, value := range spec.Lead.Env {
		set[name] = value
	}
	return harnessEnv(wsharnessenv.Drop(spec.Env), []string{"WS_PI_"}, nil, set)
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
	cmd.Env = piWorkerEnv(spec.Env)
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
	attach := spec.Holder != nil && spec.Holder.id != ""
	args := append(pre, piLeadArgs(spec)...)
	if !attach {
		command, args, err = preparePiLeadLaunch(command, pre, spec)
		if err != nil {
			return nil, fmt.Errorf("pi lead launch: %w", err)
		}
	}
	cmd := exec.Command(command, args...)
	cmd.Dir = spec.workDir()
	cmd.Env = piLeadEnv(spec)
	m := newPiMapper(spec)
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
