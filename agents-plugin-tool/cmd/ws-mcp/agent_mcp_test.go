package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	goruntime "runtime"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/mcp"
	"github.com/kang-sw/devenv/internal/wsagent"
	"github.com/kang-sw/devenv/internal/wsconfig"
	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wsholder"
	"github.com/kang-sw/devenv/internal/wsrsrc"
)

func TestAgentMCPCommandUsesEmbeddedLauncherWithoutPlugin(t *testing.T) {
	t.Setenv("WS_CACHE_HOME", t.TempDir())
	t.Setenv("WS_CONFIG_HOME", t.TempDir())
	python := filepath.Join(t.TempDir(), "chosen python")
	if goruntime.GOOS == "windows" {
		python += ".exe"
	}
	if err := os.WriteFile(python, nil, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv(wsholder.PythonEnv, python)
	t.Setenv("WS_MCP_PROJECT_ROOT", filepath.Join(t.TempDir(), "lead"))
	root := filepath.Join(t.TempDir(), "worker worktree")
	if os.Getenv("WS_TEST_EMBEDDED_PACKAGES") == "1" {
		t.Setenv("WS_RSRC_DEV_CHECKOUT", "")
	}
	for _, name := range []string{"ws", "wsflow"} {
		t.Run(name, func(t *testing.T) {
			r := wsconfig.NewResolver(wsconfig.Options{}, nil, nil, nil)
			if err := r.Set(wsconfig.HostedPluginPackageKey, name, wsconfig.SetOptions{}); err != nil {
				t.Fatal(err)
			}
			t.Setenv(wsrsrc.EnvRsrcRoot, filepath.Join(t.TempDir(), "missing", "rsrc"))
			cmd, err := agentMCPCommand(root, "0.50.1")
			if err != nil {
				t.Fatal(err)
			}
			launcher := filepath.Join(cmd.Plugin, "bin", "ws-mcp-launcher.py")
			if want := []string{launcher, "serve", "--stdio"}; cmd.Command != python || !reflect.DeepEqual(cmd.Args, want) {
				t.Fatalf("worker MCP = %+v, want %s %v", cmd, python, want)
			}
			if _, err := os.Stat(launcher); err != nil {
				t.Fatal(err)
			}
			if cmd.Env["WS_MCP_HOSTED_PACKAGE"] != "1" {
				t.Fatal("missing launch-only hosted marker")
			}
			if cmd.Env["WS_MCP_PROJECT_ROOT"] != root || cmd.Env[wsrsrc.EnvRsrcRoot] != filepath.Join(cmd.Plugin, "rsrc") {
				t.Fatalf("worker MCP env = %v; root and package must override the daemon's inherited values", cmd.Env)
			}
		})
	}
}

func TestAgentMCPCommandFailsWithoutBuildInputs(t *testing.T) {
	t.Setenv("WS_CACHE_HOME", t.TempDir())
	t.Setenv("WS_CONFIG_HOME", t.TempDir())
	t.Setenv("WS_RSRC_DEV_CHECKOUT", t.TempDir())
	cmd, err := agentMCPCommand(t.TempDir(), "0.50.1")
	if err == nil || !strings.Contains(err.Error(), "cannot stage ws-mcp launcher") || cmd.Command != "" {
		t.Fatalf("must refuse, never use daemon executable: %+v, %v", cmd, err)
	}
}

// A launcher-hosted worker must retain its token-scoped pi.* forwarding,
// rather than being treated as a lead merely because the daemon has a runner.
func TestAgentMCPHostedWorkerKeepsTokenForwarding(t *testing.T) {
	cache, root := t.TempDir(), t.TempDir()
	t.Setenv("WS_CACHE_HOME", cache)
	t.Setenv("WS_CONFIG_HOME", t.TempDir())
	called := make(chan string, 1)
	route := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called <- r.URL.Path + " " + r.Header.Get("Authorization")
		_, _ = io.WriteString(w, `{"conversations":[]}`)
	}))
	defer route.Close()
	key, err := mcp.NewAgentKeyStore().Mint(root)
	if err != nil {
		t.Fatal(err)
	}
	holders := wsholder.New(wsholder.Options{Dir: t.TempDir()})
	defer holders.Close()
	runner := newAgentRunner(t.TempDir(), io.Discard, nil, holders)
	defer runner.Close()
	host := &proxyHost{agents: runner, logw: io.Discard}
	session, err := host.Accept(wsdashboard.ProxyHello{
		Mode: wsdashboard.ProxyModeMCP, Version: "0.50.1", Cwd: root, ProjectRoot: root, OwnerPID: os.Getpid(),
		Env: map[string]string{
			"WS_CACHE_HOME": cache, "WS_CONFIG_HOME": os.Getenv("WS_CONFIG_HOME"),
			wsrsrc.EnvRsrcRoot: os.Getenv(wsrsrc.EnvRsrcRoot), "WS_MCP_NO_DASHBOARD": "1",
			"WS_MCP_ALLOWED_TOOLS": "pi.list", wsagent.EnvPiToken: "worker-token", wsagent.EnvPiEndpoint: route.URL,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	server := session.(*proxyMCPSession).server
	defer server.Close()
	request, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "tools/call",
		"params": map[string]any{"name": "pi.list", "arguments": map[string]any{"session_key": key}},
	})
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := server.ServeStdio(context.Background(), bytes.NewReader(append(request, '\n')), &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "no Pi subagents") {
		t.Fatalf("hosted worker's pi.list must forward on its token, not require a lead key: %s", &out)
	}
	select {
	case got := <-called:
		if got != "/agents/by-token/list Bearer worker-token" {
			t.Fatalf("forwarded request = %q", got)
		}
	default:
		t.Fatal("hosted worker never called the token route")
	}
}

func TestAgentMCPPythonResolution(t *testing.T) {
	for _, tc := range []struct {
		name, preferred string
		windows         bool
		available       map[string]string
		want            string
		prefix, lookups []string
	}{
		{name: "launcher interpreter", preferred: "/chosen/python", available: map[string]string{"/chosen/python": "/chosen/python"}, want: "/chosen/python", lookups: []string{"/chosen/python"}},
		{name: "Windows launcher interpreter avoids Store alias", preferred: "chosen-python.exe", windows: true, available: map[string]string{"chosen-python.exe": "chosen-python.exe", "python3": "store-alias.exe"}, want: "chosen-python.exe", lookups: []string{"chosen-python.exe"}},
		{name: "Linux python3", available: map[string]string{"python3": "/bin/python3"}, want: "/bin/python3", lookups: []string{"python3"}},
		{name: "Windows python3", windows: true, available: map[string]string{"python3": "python3.exe", "py": "py.exe"}, want: "python3.exe", lookups: []string{"python3"}},
		{name: "Windows py fallback", windows: true, available: map[string]string{"py": "py.exe"}, want: "py.exe", prefix: []string{"-3"}, lookups: []string{"python3", "py"}},
		{name: "Linux does not use py", available: map[string]string{"py": "py"}, lookups: []string{"python3"}},
		{name: "missing Python", windows: true, lookups: []string{"python3", "py"}},
		{name: "invalid explicit Python fails", preferred: "/missing/python", available: map[string]string{"python3": "/bin/python3"}, lookups: []string{"/missing/python"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var looked []string
			lookup := func(name string) (string, error) {
				looked = append(looked, name)
				if path := tc.available[name]; path != "" {
					return path, nil
				}
				return "", errors.New("not found")
			}
			path, prefix, err := agentMCPPython(tc.preferred, tc.windows, lookup)
			if path != tc.want || !reflect.DeepEqual(prefix, tc.prefix) || !reflect.DeepEqual(looked, tc.lookups) || (err != nil) != (tc.want == "") {
				t.Fatalf("Python = %q %v, %v; looked up %v; want %+v", path, prefix, err, looked, tc)
			}
			if err != nil && !strings.Contains(err.Error(), "ws-mcp launcher") {
				t.Fatalf("Python refusal must name the launcher: %v", err)
			}
		})
	}
}

// TestAgentRunnerCatalogs pins the independent lead catalogs and worker
// descriptors. Pi stays handle-only until it has a catalog of its own.
func TestAgentRunnerCatalogs(t *testing.T) {
	holders := wsholder.New(wsholder.Options{Dir: t.TempDir()})
	defer holders.Close()
	runner := newAgentRunner(t.TempDir(), io.Discard, nil, holders)
	defer runner.Close()
	regs := runner.BackendRegistrations()
	if !reflect.DeepEqual(regs[wsagent.BackendClaude].Catalog, wsagent.ClaudeCatalog()) {
		t.Errorf("claude catalog = %+v, want the reference profile", regs[wsagent.BackendClaude].Catalog)
	}
	if regs[wsagent.BackendPi].Catalog != nil {
		t.Errorf("pi has a catalog profile; it stays handle-only until its own profile")
	}
	if !reflect.DeepEqual(regs[wsagent.BackendCodex].Catalog, wsagent.CodexCatalog()) {
		t.Error("codex must retain its lead and worker catalog")
	}
	if !reflect.DeepEqual(regs[wsagent.BackendAgy].Catalog, wsagent.AgyCatalog()) {
		t.Error("agy must retain its lead and worker catalog")
	}
	for _, want := range []struct {
		backend, lead, worker string
		capable, http, auto   bool
	}{
		{wsagent.BackendClaude, wsagent.LeadProfileClaude, wsagent.AgentWorkerProfile(wsagent.BackendClaude), true, true, false},
		{wsagent.BackendPi, wsagent.LeadProfilePi, wsagent.AgentWorkerProfile(wsagent.BackendPi), true, false, false},
		{wsagent.BackendAgy, wsagent.LeadProfileAgy, wsagent.AgentWorkerProfile(wsagent.BackendAgy), true, true, true},
		{wsagent.BackendCodex, wsagent.LeadProfileCodex, wsagent.AgentWorkerProfile(wsagent.BackendCodex), true, true, true},
	} {
		reg := regs[want.backend]
		if reg.LeadProfile != want.lead || reg.WorkerProfile != want.worker || reg.WorkerCapable != want.capable || reg.WorkerHTTPMCP != want.http || reg.WorkerAutoApprove != want.auto {
			t.Errorf("%s descriptor = %+v; want %+v", want.backend, reg, want)
		}
	}
}
