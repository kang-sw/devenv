package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"strings"

	"github.com/kang-sw/devenv/internal/mcp"
	"github.com/kang-sw/devenv/internal/wsagent"
	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wsholder"
	"github.com/kang-sw/devenv/internal/wsrsrc"
)

// agentMCPCommand uses the configured hosted package, not the daemon's staged
// executable or the worker's project directory. The starter supplies Python.
func agentMCPCommand(root, daemonVersion string) (wsagent.MCPCommand, error) {
	plugin, warning, err := wsagent.ResolveHostedPlugin(daemonVersion)
	if err != nil {
		return wsagent.MCPCommand{}, fmt.Errorf("cannot resolve ws-mcp launcher: %w", err)
	}
	launcher := filepath.Join(plugin, "bin", "ws-mcp-launcher.py")
	if info, err := os.Stat(launcher); err != nil || !info.Mode().IsRegular() {
		return wsagent.MCPCommand{}, fmt.Errorf("cannot resolve ws-mcp launcher at %s (from %s); start the daemon through the plugin's bin/ws-mcp-launcher.py", launcher, wsrsrc.EnvRsrcRoot)
	}
	python, prefix, err := agentMCPPython(os.Getenv(wsholder.PythonEnv), goruntime.GOOS == "windows", exec.LookPath)
	if err != nil {
		return wsagent.MCPCommand{}, err
	}
	env, err := wsagent.HostedPluginEnvironment(plugin)
	if err != nil {
		return wsagent.MCPCommand{}, err
	}
	env["WS_MCP_PROJECT_ROOT"] = root
	return wsagent.MCPCommand{
		Command:       python,
		PluginWarning: warning,
		Plugin:        plugin,
		// --root would disable the launcher's exact serve --stdio proxy
		// match. Override the daemon's inherited project root in env instead.
		Args: append(prefix, launcher, "serve", "--stdio"),
		Env:  env,
	}, nil
}

// agentMCPPython keeps the launcher's interpreter when known (including on
// Windows, where python3 may be a Store alias), else uses the harness configs'
// python3 command, with the Windows Python launcher as a fallback.
func agentMCPPython(preferred string, windows bool, lookPath func(string) (string, error)) (string, []string, error) {
	if preferred = strings.TrimSpace(preferred); preferred != "" {
		path, err := lookPath(preferred)
		if err != nil {
			return "", nil, fmt.Errorf("ws-mcp launcher Python from %s: %w", wsholder.PythonEnv, err)
		}
		return path, nil, nil
	}
	if path, err := lookPath("python3"); err == nil {
		return path, nil, nil
	}
	if windows {
		if path, err := lookPath("py"); err == nil {
			return path, []string{"-3"}, nil
		}
	}
	return "", nil, fmt.Errorf("no Python interpreter for ws-mcp launcher: %s is unset and python3 (or py on Windows) is not on PATH", wsholder.PythonEnv)
}

// renderLeadManual is the daemon's wsagent.Options.LeadManual: the workflow
// manual of the hosted lead whose key is key, rendered under the environment
// its ws-mcp gets (the daemon's own, the configured hosted package's, and no
// mailbox identity, as agentMCPCommand and the lead's launch give it) and
// resolved against dir as a hello from that ws-mcp would be (callerHello).
// A Claude lead's connection is detected as claude; agy and a Pi lead on
// its MCP fallback are detected as no harness.
func renderLeadManual(backend, dir, key string) (string, error) {
	plugin, _, err := wsagent.ResolveHostedPlugin(version)
	if err != nil {
		return "", err
	}
	pkg, err := wsagent.HostedPluginEnvironment(plugin)
	if err != nil {
		return "", err
	}
	env := map[string]string{}
	for _, kv := range os.Environ() {
		if name, value, ok := strings.Cut(kv, "="); ok {
			env[name] = value
		}
	}
	for name, value := range pkg {
		env[name] = value
	}
	env["WS_MCP_PROJECT_ROOT"] = dir
	env["WS_MAILBOX"], env["WS_MAILBOX_AUTO"] = "", ""
	hello := callerHello(wsdashboard.ProxyHello{Env: env, Cwd: dir})
	harness := ""
	if backend == wsagent.BackendClaude {
		harness = "claude"
	}
	return mcp.RenderLeadManual(mcp.EnvFromMap(hello.Env), dir, harness, key)
}
