package main

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsagent"
	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wsrsrc"
)

// httpTokenAgents is an agent host with one live MCP session token: what
// the runner gives a hosted worker it started over HTTP.
type httpTokenAgents struct {
	token string
	hello wsdashboard.ProxyHello
	void  chan struct{}
}

func (a *httpTokenAgents) Handler() http.Handler      { return http.NotFoundHandler() }
func (a *httpTokenAgents) Start(func(string, string)) {}
func (a *httpTokenAgents) InboxChanged(string)        {}
func (a *httpTokenAgents) Close()                     {}
func (a *httpTokenAgents) ResolveMCPToken(token string) (wsdashboard.ProxyHello, <-chan struct{}, bool) {
	if token != a.token {
		return wsdashboard.ProxyHello{}, nil, false
	}
	return a.hello, a.void, true
}

// TestHostedWorkerOverHTTPSeesOnlyItsProfile: a worker whose ws-mcp is
// attached over the daemon's HTTP route gets a real ws-mcp session that
// lists and runs only its profile's ws tools, after the discovery probe
// Claude Code sends before it initializes; another token, or none, gets
// nothing.
func TestHostedWorkerOverHTTPSeesOnlyItsProfile(t *testing.T) {
	rsrc, err := filepath.Abs(filepath.Join("..", "..", "..", "agents-plugin", "rsrc"))
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	agents := &httpTokenAgents{token: "worker-session-token", void: make(chan struct{}), hello: wsdashboard.ProxyHello{
		Protocol: wsdashboard.ProxyProtocol, Mode: wsdashboard.ProxyModeMCP, Cwd: root, ProjectRoot: root,
		OwnerPID: os.Getpid(), Harness: "claude", Version: "0.50.2",
		Env: map[string]string{
			"WS_CACHE_HOME": t.TempDir(), "WS_CONFIG_HOME": t.TempDir(), wsrsrc.EnvRsrcRoot: rsrc,
			"WS_MCP_NO_DASHBOARD": "1", "WS_MCP_ALLOWED_TOOLS": "tickets.query",
			wsagent.EnvPiToken: "", wsagent.EnvPiEndpoint: "",
		},
	}}
	d, err := wsdashboard.StartDaemon(wsdashboard.DaemonOptions{Dir: t.TempDir(), Port: 0, Version: "0.50.2", LockWait: 200 * time.Millisecond,
		Proxy: &proxyHost{logw: io.Discard}, Agents: agents})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(d.Close)
	url := "http://127.0.0.1:" + strconv.Itoa(d.Endpoint().Port) + wsdashboard.ProxyHTTPPath
	client := &http.Client{Timeout: 30 * time.Second, Transport: &http.Transport{Proxy: nil}}
	post := func(token, body string) (int, string) {
		t.Helper()
		req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		raw, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(raw)
	}
	type reply struct {
		Result struct {
			ServerInfo struct {
				Name string `json:"name"`
			} `json:"serverInfo"`
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
		} `json:"result"`
		Error *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	call := func(body string) reply {
		t.Helper()
		code, raw := post(agents.token, body)
		var r reply
		if code != http.StatusOK || json.Unmarshal([]byte(raw), &r) != nil {
			t.Fatalf("POST %s = %d %s", body, code, raw)
		}
		return r
	}

	for _, token := range []string{"", "another-token"} {
		if code, _ := post(token, `{"jsonrpc":"2.0","id":0,"method":"initialize","params":{}}`); code != http.StatusUnauthorized {
			t.Fatalf("token %q: %d, want 401", token, code)
		}
	}
	if r := call(`{"jsonrpc":"2.0","id":"discover-1","method":"server/discover","params":{}}`); r.Error == nil || r.Error.Code != -32601 {
		t.Fatalf("the discovery probe is not a method ws-mcp has: %+v", r)
	}
	if r := call(`{"jsonrpc":"2.0","id":0,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"claude-code","version":"2.1.326"}}}`); r.Error != nil || r.Result.ServerInfo.Name != "ws-mcp" {
		t.Fatalf("initialize after the probe: %+v", r)
	}
	if code, raw := post(agents.token, `{"jsonrpc":"2.0","method":"notifications/initialized"}`); code != http.StatusAccepted {
		t.Fatalf("notifications/initialized = %d %s", code, raw)
	}
	r := call(`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`)
	var names []string
	for _, tool := range r.Result.Tools {
		names = append(names, tool.Name)
	}
	if !slices.Equal(names, []string{"tickets.query"}) {
		t.Fatalf("the worker lists its profile's ws tools only: %v", names)
	}
	if r := call(`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"git.commit","arguments":{}}}`); r.Error == nil || !strings.Contains(r.Error.Message, "not available") {
		t.Fatalf("a tool outside the profile is refused: %+v", r)
	}

	// The token goes void; it still resolves here, as a resolution that
	// raced the void would, and the route refuses it and ends its session.
	close(agents.void)
	if code, _ := post(agents.token, `{"jsonrpc":"2.0","id":3,"method":"tools/list","params":{}}`); code != http.StatusUnauthorized {
		t.Fatalf("a void token: %d, want 401", code)
	}
	deadline := time.Now().Add(5 * time.Second)
	for d.ProxySessions() != 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if n := d.ProxySessions(); n != 0 {
		t.Fatalf("the void token's session is still open: %d sessions", n)
	}
}
