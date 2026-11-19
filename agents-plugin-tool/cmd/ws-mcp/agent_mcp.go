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
	"github.com/kang-sw/devenv/internal/wslive"
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

// hostedPackageEnv is the configured hosted package's ws-mcp environment
// (wsagent.HostedPluginEnvironment of its staged copy), as
// agentMCPCommand gives it.
func hostedPackageEnv() (map[string]string, error) {
	name, _, err := wsagent.ResolveHostedPackage()
	if err != nil {
		return nil, err
	}
	stateDir, err := wsdashboard.Dir("")
	if err != nil {
		return nil, err
	}
	plugin, err := wspackage.StageHosted(name, stateDir)
	if err != nil {
		return nil, err
	}
	return wsagent.HostedPluginEnvironment(name, plugin)
}

// hostedRenderEnv is the environment a hosted lead's ws-mcp serves dir
// under (the daemon's own, the configured hosted package's, and no mailbox
// identity, as agentMCPCommand and the lead's launch give it), resolved
// against dir as a hello from that ws-mcp would be (callerHello).
func hostedRenderEnv(dir string) ([]string, error) {
	pkg, err := hostedPackageEnv()
	if err != nil {
		return nil, err
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
	return mcp.EnvFromMap(hello.Env), nil
}

// renderLeadManual is the daemon's wsagent.Options.LeadManual: the workflow
// manual of the hosted lead whose key is key, rendered under
// hostedRenderEnv(dir) as render (the lead backend's registration) says.
func renderLeadManual(render mcp.HostedRender, dir, key string) (string, error) {
	env, err := hostedRenderEnv(dir)
	if err != nil {
		return "", err
	}
	return mcp.RenderLeadManual(env, dir, render, key)
}

// renderLeadPlaybook is the daemon's wsagent.Options.ExpandCommand: the
// body of playbook name for the hosted lead whose key is key, rendered as
// renderLeadManual renders its manual.
func renderLeadPlaybook(render mcp.HostedRender, dir, key, name string) (string, error) {
	env, err := hostedRenderEnv(dir)
	if err != nil {
		return "", err
	}
	return mcp.RenderLeadPlaybook(env, dir, render, key, name)
}

// hostedPlaybookCommands is the daemon's wsagent.Options.PlaybookCommands:
// the lead playbook commands of the configured hosted package.
func hostedPlaybookCommands() ([]wslive.SlashCommand, error) {
	pkg, err := hostedPackageEnv()
	if err != nil {
		return nil, err
	}
	return mcp.HostedPlaybookCommands(mcp.EnvFromMap(pkg))
}
