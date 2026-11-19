package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsagent"
	"github.com/kang-sw/devenv/internal/wsconfig"
	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wsholder"
	"github.com/kang-sw/devenv/internal/wsrsrc"
)

func hostedPackageFixture(t *testing.T, name string) string {
	t.Helper()
	root := t.TempDir()
	for _, dir := range []string{".claude-plugin", "bin", "rsrc"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	env := map[string]string{
		"WS_MCP_CLI_NAME":         name + "-cli",
		"WS_MCP_CONFIG_MANIFESTS": "${CLAUDE_PLUGIN_ROOT}/config-manifest.json;${CLAUDE_PLUGIN_ROOT}/dashboard-config-manifest.json",
	}
	if name == "wsflow" {
		env["WS_MCP_NAMESPACE"], env["WS_MCP_NO_AGENT"], env["WS_MCP_SETUP_TOOL"] = name, "1", "setup"
	}
	raw, _ := json.Marshal(map[string]any{"name": name, "version": "0.50.1", "mcpServers": map[string]any{name: map[string]any{"env": env}}})
	if err := os.WriteFile(filepath.Join(root, ".claude-plugin", "plugin.json"), raw, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "bin", "ws-mcp-launcher.py"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestHostedPackageHellosDoNotSelectHostedInputs(t *testing.T) {
	t.Setenv("WS_CACHE_HOME", t.TempDir())
	t.Setenv("WS_CONFIG_HOME", t.TempDir())
	dir, err := wsdashboard.Dir("")
	if err != nil {
		t.Fatal(err)
	}
	host := &proxyHost{dir: dir, logw: io.Discard}
	roots := map[string]string{}
	for _, name := range []string{"ws", "wsflow"} {
		roots[name] = hostedPackageFixture(t, name)
		// External hellos retain their own policy, without registering roots.
		hello := wsdashboard.ProxyHello{Mode: wsdashboard.ProxyModeCLI, Version: "0.50.1", Cwd: roots[name], Env: map[string]string{wsrsrc.EnvRsrcRoot: "rsrc", "WS_MCP_NAMESPACE": name}}
		session, err := host.Accept(hello)
		if err != nil {
			t.Fatal(err)
		}
		caller := session.(*proxyCLISession).hello
		if caller.Env["WS_MCP_NAMESPACE"] != name || caller.Env[wsrsrc.EnvRsrcRoot] != filepath.Join(roots[name], "rsrc") {
			t.Fatalf("hello policy was replaced: %+v", caller.Env)
		}
	}
	// A replacement still selects build-owned packages, never learned roots.
	if os.Getenv("WS_TEST_EMBEDDED_PACKAGES") == "1" {
		t.Setenv("WS_RSRC_DEV_CHECKOUT", "")
	}
	host = &proxyHost{dir: dir, logw: io.Discard}
	python := filepath.Join(t.TempDir(), "python")
	if runtime.GOOS == "windows" {
		python += ".exe"
	}
	if err := os.WriteFile(python, nil, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv(wsholder.PythonEnv, python)
	workerRoot := t.TempDir()
	for _, starter := range []string{"ws", "wsflow"} {
		t.Setenv(wsrsrc.EnvRsrcRoot, filepath.Join(roots[starter], "rsrc"))
		for _, selected := range []string{"", "ws", "wsflow"} {
			r := wsconfig.NewResolver(wsconfig.Options{}, nil, nil, nil)
			if selected == "" {
				err = r.Unset(wsconfig.HostedPluginPackageKey, wsconfig.SetOptions{})
			} else {
				err = r.Set(wsconfig.HostedPluginPackageKey, selected, wsconfig.SetOptions{})
			}
			if err != nil {
				t.Fatal(err)
			}
			want := selected
			if want == "" {
				want = "wsflow"
			}
			cmd, err := agentMCPCommand(workerRoot, "0.50.1")
			if err != nil {
				t.Fatal(err)
			}
			if cmd.Args[0] != filepath.Join(cmd.Plugin, "bin", "ws-mcp-launcher.py") || cmd.Plugin == roots[want] || cmd.PluginWarning != "" || cmd.Env["WS_MCP_PROJECT_ROOT"] != workerRoot {
				t.Fatalf("%s/%s: %+v", starter, selected, cmd)
			}
			if cmd.Env["WS_MCP_CLI_NAME"] != want+"-cli" || cmd.Env["WS_SKILLS_ROOT"] != filepath.Join(cmd.Plugin, "skills") || !strings.Contains(cmd.Env["WS_MCP_CONFIG_MANIFESTS"], cmd.Plugin) {
				t.Fatalf("wrong package policy: %v", cmd.Env)
			}
			if want == "wsflow" && (cmd.Env["WS_MCP_NAMESPACE"] != want || cmd.Env["WS_MCP_NO_AGENT"] != "1") {
				t.Fatalf("missing wsflow policy: %v", cmd.Env)
			}
			if want == "ws" && (cmd.Env["WS_MCP_NAMESPACE"] != "" || cmd.Env["WS_MCP_NO_AGENT"] != "") {
				t.Fatalf("starter policy leaked into ws: %v", cmd.Env)
			}
		}
	}
	// An unrelated hello remains usable but cannot change hosted selection.
	bad := hostedPackageFixture(t, "unrelated")
	if _, err := host.Accept(wsdashboard.ProxyHello{Mode: wsdashboard.ProxyModeCLI, Version: "0.50.1", Cwd: bad, Env: map[string]string{wsrsrc.EnvRsrcRoot: filepath.Join(bad, "rsrc"), "WS_MCP_NAMESPACE": "wsflow"}}); err != nil {
		t.Fatal(err)
	}
	root, warning, err := wsagent.ResolveHostedPackage()
	if err != nil || root != "wsflow" || warning != "" {
		t.Fatalf("unrelated hello changed hosted selection: %q %q %v", root, warning, err)
	}
}

func TestHostedPackageSelectionLeavesNonHostedHelloPolicy(t *testing.T) {
	t.Setenv("WS_CONFIG_HOME", t.TempDir())
	t.Setenv("WS_CACHE_HOME", t.TempDir())
	r := wsconfig.NewResolver(wsconfig.Options{}, nil, nil, nil)
	for _, selected := range []string{"ws", "wsflow"} {
		if err := r.Set(wsconfig.HostedPluginPackageKey, selected, wsconfig.SetOptions{}); err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{"ws", "wsflow"} {
			plugin := hostedPackageFixture(t, name)
			for file, body := range map[string]string{
				"config-manifest.json":           `{"schema_version":1,"namespace":"fixture.","keys":[{"key":"fixture.owner","type":"string","default":"` + name + `","description":"d"}]}`,
				"dashboard-config-manifest.json": `{"schema_version":1,"namespace":"dashboard.","keys":[{"key":"dashboard.enabled","type":"boolean","default":false,"description":"d"}]}`,
			} {
				if err := os.WriteFile(filepath.Join(plugin, file), []byte(body), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			// This is an external connection: its own manifest paths remain
			// caller-owned, not the hosted build-owned environment helper.
			env := map[string]string{
				"WS_RSRC_ROOT":            filepath.Join(plugin, "rsrc"),
				"WS_MCP_CONFIG_MANIFESTS": filepath.Join(plugin, "config-manifest.json") + ";" + filepath.Join(plugin, "dashboard-config-manifest.json"),
			}
			if name == "wsflow" {
				env["WS_MCP_NAMESPACE"], env["WS_MCP_NO_AGENT"] = name, "1"
			}
			env["WS_CONFIG_HOME"], env["WS_CACHE_HOME"] = os.Getenv("WS_CONFIG_HOME"), os.Getenv("WS_CACHE_HOME")
			host := &proxyHost{dir: t.TempDir(), logw: io.Discard}
			session, err := host.Accept(wsdashboard.ProxyHello{Mode: wsdashboard.ProxyModeMCP, Version: "0.50.1", Cwd: t.TempDir(), Env: env, OwnerPID: os.Getpid()})
			if err != nil {
				t.Fatal(err)
			}
			server := session.(*proxyMCPSession).server
			var output bytes.Buffer
			requests := "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"tools/list\"}\n" +
				"{\"jsonrpc\":\"2.0\",\"id\":2,\"method\":\"tools/call\",\"params\":{\"name\":\"config.get\",\"arguments\":{\"key\":\"fixture.owner\"}}}\n" +
				"{\"jsonrpc\":\"2.0\",\"id\":3,\"method\":\"tools/call\",\"params\":{\"name\":\"exec.spawn\",\"arguments\":{\"cmd\":\"unused-no-key\"}}}\n"
			err = server.ServeStdio(context.Background(), strings.NewReader(requests), &output)
			server.Close()
			if err != nil {
				t.Fatal(err)
			}
			results := map[int]json.RawMessage{}
			decoder := json.NewDecoder(&output)
			for range 3 {
				var response struct {
					ID     int             `json:"id"`
					Result json.RawMessage `json:"result"`
					Error  json.RawMessage `json:"error"`
				}
				if err := decoder.Decode(&response); err != nil {
					t.Fatal(err)
				}
				results[response.ID] = append(response.Result, response.Error...)
			}
			if !strings.Contains(string(results[2]), name) {
				t.Fatalf("%s setting changed %s manifest: %s", selected, name, results[2])
			}
			if (name == "wsflow") != strings.Contains(string(results[3]), "agentless mode disables") {
				t.Fatalf("%s setting changed %s no-agent profile: %s", selected, name, results[3])
			}
		}
	}
}

func TestAgentMCPSelectedPackageNeedsNoInstalledCopy(t *testing.T) {
	t.Setenv("WS_CACHE_HOME", t.TempDir())
	t.Setenv("WS_CONFIG_HOME", t.TempDir())
	ws := hostedPackageFixture(t, "ws")
	t.Setenv(wsrsrc.EnvRsrcRoot, filepath.Join(ws, "rsrc"))
	python := filepath.Join(t.TempDir(), "python")
	if runtime.GOOS == "windows" {
		python += ".exe"
	}
	if err := os.WriteFile(python, nil, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv(wsholder.PythonEnv, python)
	cmd, err := agentMCPCommand(t.TempDir(), "0.50.1")
	if err != nil || cmd.Plugin == ws || cmd.PluginWarning != "" || cmd.Env["WS_MCP_NAMESPACE"] != "wsflow" {
		t.Fatalf("fallback = %+v, %v", cmd, err)
	}
}
