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
	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wsholder"
	"github.com/kang-sw/devenv/internal/wsrsrc"
)

func TestAgentMCPCommandUsesTheStartingPackagesLauncher(t *testing.T) {
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
	for _, name := range []string{"ws", "wsflow", "pi"} {
		t.Run(name, func(t *testing.T) {
			plugin := filepath.Join(t.TempDir(), name+" package")
			launcher := filepath.Join(plugin, "bin", "ws-mcp-launcher.py")
			if err := os.MkdirAll(filepath.Dir(launcher), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(launcher, nil, 0o644); err != nil {
				t.Fatal(err)
			}
			rsrc := filepath.Join(plugin, "rsrc")
			t.Setenv(wsrsrc.EnvRsrcRoot, rsrc)
			cmd, err := agentMCPCommand(root)
			if err != nil {
				t.Fatal(err)
			}
			if want := []string{launcher, "serve", "--stdio"}; cmd.Command != python || !reflect.DeepEqual(cmd.Args, want) {
				t.Fatalf("worker MCP = %+v, want %s %v", cmd, python, want)
			}
			if cmd.Env["WS_MCP_PROJECT_ROOT"] != root || cmd.Env[wsrsrc.EnvRsrcRoot] != rsrc {
				t.Fatalf("worker MCP env = %v; root and package must override the daemon's inherited values", cmd.Env)
			}
		})
	}
}

func TestAgentMCPCommandFailsWithoutALauncher(t *testing.T) {
	for _, name := range []string{"unset resource root", "missing launcher", "launcher is a directory"} {
		t.Run(name, func(t *testing.T) {
			rsrc := filepath.Join(t.TempDir(), "rsrc")
			if name == "unset resource root" {
				rsrc = ""
			}
			if name == "launcher is a directory" {
				if err := os.MkdirAll(filepath.Join(filepath.Dir(rsrc), "bin", "ws-mcp-launcher.py"), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			t.Setenv(wsrsrc.EnvRsrcRoot, rsrc)
			cmd, err := agentMCPCommand(t.TempDir())
			if err == nil || !strings.Contains(err.Error(), "cannot resolve ws-mcp launcher") || cmd.Command != "" {
				t.Fatalf("must refuse, never fall back to the daemon executable: command=%+v, error=%v", cmd, err)
			}
		})
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
		Mode: wsdashboard.ProxyModeMCP, Cwd: root, ProjectRoot: root, OwnerPID: os.Getpid(),
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
