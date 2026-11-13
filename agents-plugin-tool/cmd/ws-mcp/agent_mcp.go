package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"strings"

	"github.com/kang-sw/devenv/internal/wsagent"
	"github.com/kang-sw/devenv/internal/wsholder"
	"github.com/kang-sw/devenv/internal/wsrsrc"
)

// agentMCPCommand uses the package that started the daemon, not its staged
// executable. The launcher supplies WS_RSRC_ROOT and its own Python interpreter
// at daemon-start; neither is inferred from the worker's project directory.
func agentMCPCommand(root string) (wsagent.MCPCommand, error) {
	rsrc, err := wsrsrc.ResolveRoot()
	if err != nil {
		return wsagent.MCPCommand{}, fmt.Errorf("resolve ws-mcp launcher: %w", err)
	}
	rsrc, err = filepath.Abs(rsrc)
	if err != nil {
		return wsagent.MCPCommand{}, fmt.Errorf("resolve ws-mcp launcher resource root: %w", err)
	}
	launcher := filepath.Join(filepath.Dir(rsrc), "bin", "ws-mcp-launcher.py")
	if info, err := os.Stat(launcher); err != nil || !info.Mode().IsRegular() {
		return wsagent.MCPCommand{}, fmt.Errorf("cannot resolve ws-mcp launcher at %s (from %s); start the daemon through the plugin's bin/ws-mcp-launcher.py", launcher, wsrsrc.EnvRsrcRoot)
	}
	python, prefix, err := agentMCPPython(os.Getenv(wsholder.PythonEnv), goruntime.GOOS == "windows", exec.LookPath)
	if err != nil {
		return wsagent.MCPCommand{}, err
	}
	return wsagent.MCPCommand{
		Command: python,
		// --root would disable the launcher's exact serve --stdio proxy
		// match. Override the daemon's inherited project root in env instead.
		Args: append(prefix, launcher, "serve", "--stdio"),
		Env:  map[string]string{"WS_MCP_PROJECT_ROOT": root, wsrsrc.EnvRsrcRoot: rsrc},
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
