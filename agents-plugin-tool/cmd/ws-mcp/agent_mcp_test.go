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
	"time"

	"github.com/kang-sw/devenv/internal/mcp"
	"github.com/kang-sw/devenv/internal/wsagent"
	"github.com/kang-sw/devenv/internal/wsconfig"
	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wsholder"
	"github.com/kang-sw/devenv/internal/wsmailbox"
	"github.com/kang-sw/devenv/internal/wsrsrc"
)

func TestCallFallbackRegistrationAndRenderDescriptor(t *testing.T) {
	holders := wsholder.New(wsholder.Options{Dir: t.TempDir()})
	defer holders.Close()
	runner := newAgentRunner(t.TempDir(), io.Discard, nil, holders)
	defer runner.Close()
	for name, reg := range runner.BackendRegistrations() {
		if reg.CallFallback != (name == wsagent.BackendAgy) {
			t.Fatalf("backend %q fallback = %v; only agy opts in", name, reg.CallFallback)
		}
		render := hostedRender(reg)
		if render.CallFallback != reg.CallFallback || render.ToolNameScheme != reg.ToolNameScheme {
			t.Fatalf("backend %q loses fallback or alias data in prompt rendering: %+v", name, render)
		}
	}
}

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

// TestAgentMCPHostedWorkerDescriptorThroughProxy goes through the real
// proxyHost.Accept token path (with WS_MCP_PI_TOKEN set, so opts.Agents stays
// nil) and pins:
//   - an agy worker gets sanitized underscore tool names, alwaysLoad metadata,
//     full schemas, and its full worker allowlist (not the lead's DirectTools
//     filter);
//   - Claude/Codex/Pi workers keep their dotted names and no metadata;
//   - no worker lists or can invoke call, no worker gets lead session_key
//     substitution, and agent.* calls still forward over the capability token.
func TestAgentMCPHostedWorkerDescriptorThroughProxy(t *testing.T) {
	for _, backend := range []string{wsagent.BackendAgy, wsagent.BackendClaude, wsagent.BackendCodex, wsagent.BackendPi} {
		t.Run(backend, func(t *testing.T) {
			cache, configHome, root := t.TempDir(), t.TempDir(), t.TempDir()
			t.Setenv("WS_CACHE_HOME", cache)
			t.Setenv("WS_CONFIG_HOME", configHome)
			dashDir, err := wsdashboard.Dir(cache)
			if err != nil {
				t.Fatal(err)
			}
			called := make(chan string, 1)
			route := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				called <- r.URL.Path + " " + r.Header.Get("Authorization")
				_, _ = io.WriteString(w, `{"agents":[]}`)
			}))
			defer route.Close()

			key, err := mcp.NewAgentKeyStore().Mint(root)
			if err != nil {
				t.Fatal(err)
			}
			rec := wsagent.Record{
				ReplyID:   wsmailbox.ReplyID(key),
				WSKey:     key,
				Root:      root,
				CreatedAt: time.Now().UTC().Format(time.RFC3339),
				Backend:   backend,
				Profile:   wsagent.AgentWorkerProfile(backend),
				Tier:      "medium",
				Parent:    &wsagent.ParentRef{Kind: wsagent.ParentLead, ReplyID: "lead0001"},
			}
			if err := wsagent.WriteRecordForTest(dashDir, rec); err != nil {
				t.Fatal(err)
			}

			holders := wsholder.New(wsholder.Options{Dir: t.TempDir()})
			defer holders.Close()
			runner := newAgentRunner(dashDir, io.Discard, nil, holders)
			defer runner.Close()
			host := &proxyHost{agents: runner, logw: io.Discard}
			session, err := host.Accept(wsdashboard.ProxyHello{
				Mode:        wsdashboard.ProxyModeMCP,
				Version:     "0.50.8",
				Cwd:         root,
				ProjectRoot: root,
				OwnerPID:    os.Getpid(),
				Env: map[string]string{
					"WS_CACHE_HOME":            cache,
					"WS_CONFIG_HOME":           configHome,
					wsrsrc.EnvRsrcRoot:         os.Getenv(wsrsrc.EnvRsrcRoot),
					"WS_MCP_NO_DASHBOARD":      "1",
					wsagent.EnvAllowedTools:    strings.Join(wsagent.AgentWSTools, ","),
					wsagent.EnvAgentSessionKey: key,
					wsagent.EnvPiToken:         "worker-token",
					wsagent.EnvPiEndpoint:      route.URL,
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			server := session.(*proxyMCPSession).server
			defer server.Close()

			rpcCall := func(method string, params any) map[string]any {
				t.Helper()
				req, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": method, "params": params})
				if err != nil {
					t.Fatal(err)
				}
				var out bytes.Buffer
				if err := server.ServeStdio(context.Background(), bytes.NewReader(append(req, '\n')), &out); err != nil {
					t.Fatal(err)
				}
				var resp map[string]any
				if err := json.Unmarshal(bytes.TrimSpace(out.Bytes()), &resp); err != nil {
					t.Fatalf("unmarshal %s response %q: %v", method, out.String(), err)
				}
				return resp
			}

			listResp := rpcCall("tools/list", map[string]any{})
			result, _ := listResp["result"].(map[string]any)
			rawTools, _ := result["tools"].([]any)
			if len(rawTools) != len(wsagent.AgentWSTools) {
				t.Fatalf("listed %d tools, want %d", len(rawTools), len(wsagent.AgentWSTools))
			}
			byName := make(map[string]map[string]any, len(rawTools))
			for _, item := range rawTools {
				tool, _ := item.(map[string]any)
				name, _ := tool["name"].(string)
				byName[name] = tool
			}
			if _, hasCall := byName["call"]; hasCall {
				t.Fatalf("%s worker must not list call", backend)
			}

			isAgy := backend == wsagent.BackendAgy
			for _, canonical := range wsagent.AgentWSTools {
				wantName := canonical
				if isAgy {
					wantName = strings.ReplaceAll(canonical, ".", "_")
				}
				tool, ok := byName[wantName]
				if !ok {
					t.Fatalf("%s worker missing tool %q (canonical %q)", backend, wantName, canonical)
				}
				if isAgy && strings.Contains(canonical, ".") {
					if _, dotted := byName[canonical]; dotted {
						t.Fatalf("agy worker still listed dotted name %q", canonical)
					}
				}
				if isAgy {
					meta, ok := tool["_meta"].(map[string]any)
					if !ok || meta["antigravity/alwaysLoad"] != true {
						t.Fatalf("agy worker tool %q missing alwaysLoad meta: %v", wantName, tool["_meta"])
					}
				} else if _, hasMeta := tool["_meta"]; hasMeta {
					t.Fatalf("%s worker tool %q has unexpected _meta: %v", backend, wantName, tool["_meta"])
				}
			}

			ticketsName, agentListName, noteQueryName := "tickets.query", "agent.list", "note.query"
			if isAgy {
				ticketsName, agentListName, noteQueryName = "tickets_query", "agent_list", "note_query"
			}
			schema, _ := byName[ticketsName]["inputSchema"].(map[string]any)
			props, _ := schema["properties"].(map[string]any)
			if _, ok := props["session_key"]; !ok {
				t.Fatalf("%s worker %s missing session_key in inputSchema: %v", backend, ticketsName, schema)
			}
			if _, ok := props["ticket_stem"]; !ok {
				t.Fatalf("%s worker %s missing ticket_stem in inputSchema: %v", backend, ticketsName, schema)
			}

			// call stays lead-only.
			callResp := rpcCall("tools/call", map[string]any{
				"name":      "call",
				"arguments": map[string]any{"tool": noteQueryName, "arguments": map[string]any{"session_key": key}},
			})
			errObj, _ := callResp["error"].(map[string]any)
			if msg, _ := errObj["message"].(string); !strings.Contains(msg, "not available") {
				t.Fatalf("%s worker must refuse call: %v", backend, callResp)
			}

			// agent.list (by alias on agy) still forwards on the capability token.
			agentResp := rpcCall("tools/call", map[string]any{
				"name":      agentListName,
				"arguments": map[string]any{"session_key": key},
			})
			if res, _ := agentResp["result"].(map[string]any); res == nil || res["isError"] == true {
				t.Fatalf("%s worker %s failed: %v", backend, agentListName, agentResp)
			}
			select {
			case got := <-called:
				if got != "/agents/by-token/agent.list Bearer worker-token" {
					t.Fatalf("%s worker forwarded request = %q", backend, got)
				}
			default:
				t.Fatalf("%s worker %s did not forward through the token route", backend, agentListName)
			}

			// Workers still do not get hosted-lead session_key substitution.
			missingKeyResp := rpcCall("tools/call", map[string]any{
				"name":      noteQueryName,
				"arguments": map[string]any{"layer": "machine"},
			})
			if res, _ := missingKeyResp["result"].(map[string]any); res == nil || res["isError"] != true {
				t.Fatalf("%s worker %s without session_key must fail: %v", backend, noteQueryName, missingKeyResp)
			}
			withKeyResp := rpcCall("tools/call", map[string]any{
				"name":      noteQueryName,
				"arguments": map[string]any{"session_key": key, "layer": "machine"},
			})
			if res, _ := withKeyResp["result"].(map[string]any); res == nil || res["isError"] == true {
				t.Fatalf("%s worker %s with session_key failed: %v", backend, noteQueryName, withKeyResp)
			}
		})
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
