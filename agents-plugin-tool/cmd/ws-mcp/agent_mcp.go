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
	"github.com/kang-sw/devenv/internal/wspackage"
)

// agentMCPCommand uses the configured hosted package, not the daemon's staged
// executable or the worker's project directory. The starter supplies Python.
func agentMCPCommand(root, _ string) (wsagent.MCPCommand, error) {
	name, warning, err := wsagent.ResolveHostedPackage()
	if err != nil {
		return wsagent.MCPCommand{}, fmt.Errorf("cannot resolve ws-mcp launcher: %w", err)
	}
	stateDir, err := wsdashboard.Dir("")
	if err != nil {
		return wsagent.MCPCommand{}, err
	}
	plugin, err := wspackage.StageHosted(name, stateDir)
	if err != nil {
		return wsagent.MCPCommand{}, fmt.Errorf("cannot stage ws-mcp launcher: %w", err)
	}
	launcher := filepath.Join(plugin, "bin", "ws-mcp-launcher.py")
	python, prefix, err := agentMCPPython(os.Getenv(wsholder.PythonEnv), goruntime.GOOS == "windows", exec.LookPath)
	if err != nil {
		return wsagent.MCPCommand{}, err
	}
	env, err := wsagent.HostedPluginEnvironment(name, plugin)
	if err != nil {
		return wsagent.MCPCommand{}, err
	}
	env["WS_MCP_PROJECT_ROOT"] = root
	env["WS_MCP_HOSTED_PACKAGE"] = "1"
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
// harness is the lead backend's declared ManualHarness (wsagent.BackendRegistration),
// "" for a backend that declares none, such as a Pi lead on its MCP fallback.
func renderLeadManual(harness, dir, key string) (string, error) {
	name, _, err := wsagent.ResolveHostedPackage()
	if err != nil {
		return "", err
	}
	stateDir, err := wsdashboard.Dir("")
	if err != nil {
		return "", err
	}
	plugin, err := wspackage.StageHosted(name, stateDir)
	if err != nil {
		return "", err
	}
	pkg, err := wsagent.HostedPluginEnvironment(name, plugin)
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
	return mcp.RenderLeadManual(mcp.EnvFromMap(hello.Env), dir, harness, key)
}
