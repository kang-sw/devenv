package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/mcp"
	"github.com/kang-sw/devenv/internal/wsagent"
	"github.com/kang-sw/devenv/internal/wsconfig"
)

type cliTokenBackend struct{ starts chan wsagent.StartSpec }

func (b *cliTokenBackend) Start(_ context.Context, spec wsagent.StartSpec) (wsagent.Process, error) {
	b.starts <- spec
	return &cliTokenProcess{events: make(chan wsagent.Event)}, nil
}

type cliTokenProcess struct {
	events chan wsagent.Event
	once   sync.Once
}

func (p *cliTokenProcess) Events() <-chan wsagent.Event { return p.events }
func (p *cliTokenProcess) Send(string) error             { return nil }
func (p *cliTokenProcess) Interrupt() error              { return nil }
func (p *cliTokenProcess) SetModel(string) error         { return nil }
func (p *cliTokenProcess) SetEffort(string) error        { return nil }
func (p *cliTokenProcess) Close() error {
	p.once.Do(func() { close(p.events) })
	return nil
}

func TestHostedCLIWorkerTokenRouting(t *testing.T) {
	hello := cliHello(t)
	t.Setenv("WS_CACHE_HOME", hello.Env["WS_CACHE_HOME"])
	t.Setenv("WS_CONFIG_HOME", hello.Env["WS_CONFIG_HOME"])
	keys := mcp.NewAgentKeyStore()
	key, err := keys.Mint(hello.Cwd)
	if err != nil {
		t.Fatal(err)
	}
	config := wsconfig.Options{ConfigHome: hello.Env["WS_CONFIG_HOME"], CacheHome: hello.Env["WS_CACHE_HOME"]}
	for _, tier := range []string{"small", "medium", "large"} {
		if _, err := wsconfig.SetGlobalAgentsTierForHarness(config, tier, "", "provider/"+tier, wsagent.PiTierHarness, "low"); err != nil {
			t.Fatal(err)
		}
	}
	backend := &cliTokenBackend{starts: make(chan wsagent.StartSpec, 8)}
	runner := wsagent.NewRunner(wsagent.Options{Dir: t.TempDir(), Config: config, Keys: keys,
		Backends: map[string]wsagent.Backend{wsagent.BackendPi: backend},
		WSMCP:    func(string) (wsagent.MCPCommand, error) { return wsagent.MCPCommand{Command: "unused"}, nil },
	})
	daemon := httptest.NewServer(runner.Handler())
	defer daemon.Close()
	runner.SetEndpoint(daemon.URL)
	runner.Start(nil)
	defer runner.Close()
	parent := wsagent.ParentRef{Kind: wsagent.ParentLead, ReplyID: "parentreplyid"}
	spawnWorker := func() string {
		t.Helper()
		_, err := runner.Spawn(wsagent.SpawnSpec{Profile: wsagent.PiWorkerProfile, Root: hello.Cwd, Prompt: "work", Tier: "medium", Parent: &parent})
		if err != nil {
			t.Fatal(err)
		}
		select {
		case spec := <-backend.starts:
			if spec.WSMCP == nil || spec.WSMCP.Env[wsagent.EnvPiToken] == "" {
				t.Fatal("worker has no capability token")
			}
			return spec.WSMCP.Env[wsagent.EnvPiToken]
		case <-time.After(5 * time.Second):
			t.Fatal("worker did not start")
			return ""
		}
	}
	tokenA, tokenB := spawnWorker(), spawnWorker()
	hello.Env[wsagent.EnvPiEndpoint] = daemon.URL
	hello.Env[wsagent.EnvPiToken] = tokenA
	hello.Env["WS_MCP_ALLOWED_TOOLS"] = "pi.spawn,pi.list,pi.transcript,pi.wait"
	host := &proxyHost{agents: runner, logw: io.Discard}
	out, stderr, code := hostedCLIResultWithHost(t, host, hello, "tools", "pi.spawn")
	if code != 0 || !strings.Contains(out, "Your own ws session key") || !strings.Contains(out, "relative to your own working directory") {
		t.Fatalf("worker schema: %q %q %d", out, stderr, code)
	}
	hello.Env[wsagent.EnvPiToken] = "  "
	leadSchema, stderr, code := hostedCLIResultWithHost(t, host, hello, "tools", "pi.spawn")
	if strings.Contains(leadSchema, "Your own ws session key") || (code != 0 && !strings.Contains(stderr, "tool not found: pi.spawn")) {
		t.Fatalf("lead schema: %q %q %d", leadSchema, stderr, code)
	}
	hello.Env[wsagent.EnvPiToken] = tokenA
	call := func(tool, args string) (string, string, int) {
		t.Helper()
		return hostedCLIResultWithHost(t, host, hello, "call", tool, args)
	}
	out, stderr, code = call("pi.spawn", fmt.Sprintf(`{"session_key":%q,"prompt":"child","tier":"small","format":"json"}`, key))
	if code != 0 {
		t.Fatalf("spawn: %q %q %d", out, stderr, code)
	}
	var child struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(out), &child); err != nil || child.ID == "" {
		t.Fatalf("child: %q %v", out, err)
	}
	out, stderr, code = call("pi.list", fmt.Sprintf(`{"session_key":%q}`, key))
	if code != 0 || !strings.Contains(out, child.ID) {
		t.Fatalf("list: %q %q %d", out, stderr, code)
	}
	args := fmt.Sprintf(`{"session_key":%q,"id":%q}`, key, child.ID)
	out, stderr, code = call("pi.transcript", args)
	if code != 0 {
		t.Fatalf("owned transcript: %q %q %d", out, stderr, code)
	}
	waitArgs := fmt.Sprintf(`{"session_key":%q,"id":%q,"timeout_s":0.01}`, key, child.ID)
	out, stderr, code = call("pi.wait", waitArgs)
	if code != 0 {
		t.Fatalf("owned wait: %q %q %d", out, stderr, code)
	}
	hello.Env[wsagent.EnvPiToken] = tokenB
	for _, tc := range []struct{ tool, args string }{{"pi.transcript", args}, {"pi.wait", waitArgs}} {
		out, stderr, code = call(tc.tool, tc.args)
		if code == 0 || !strings.Contains(stderr, "is not one of this conversation's Pi subagents") {
			t.Fatalf("foreign %s: %q %q %d", tc.tool, out, stderr, code)
		}
	}
	out, stderr, code = call("pi.spawn", fmt.Sprintf(`{"session_key":%q,"prompt":"too high","tier":"large"}`, key))
	if code == 0 || !strings.Contains(stderr, "above this conversation's own tier") {
		t.Fatalf("tier cap: %q %q %d", out, stderr, code)
	}
}
