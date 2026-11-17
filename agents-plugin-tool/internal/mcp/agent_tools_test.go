package mcp

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsagent"
	"github.com/kang-sw/devenv/internal/wsconfig"
	"github.com/kang-sw/devenv/internal/wsmailbox"
	"github.com/kang-sw/devenv/internal/wswake"
)

// fakeAgentSide is fakePiAgents' agent.* side: it records each caller and
// spawn request and answers with what the test set.
type fakeAgentSide struct {
	callers   []wsagent.AgentCaller
	requests  []wsagent.AgentSpawnRequest
	addresses []string
	err       error
	children  []wsagent.AgentChild
	tiers     wsagent.AgentTiers
	returned  []string
}

func (f *fakePiAgents) record(caller wsagent.AgentCaller) {
	f.mu.Lock()
	f.agent.callers = append(f.agent.callers, caller)
	f.mu.Unlock()
}

func (f *fakePiAgents) AgentSpawn(caller wsagent.AgentCaller, req wsagent.AgentSpawnRequest) (wsagent.AgentSpawnResult, error) {
	f.record(caller)
	f.mu.Lock()
	defer f.mu.Unlock()
	f.agent.requests = append(f.agent.requests, req)
	if f.agent.err != nil {
		return wsagent.AgentSpawnResult{}, f.agent.err
	}
	return wsagent.AgentSpawnResult{Address: "id:kid1", Status: wsagent.AgentStarting}, nil
}

func (f *fakePiAgents) AgentInterrupt(caller wsagent.AgentCaller, address string) (wsagent.AgentInterruptResult, error) {
	f.record(caller)
	f.mu.Lock()
	defer f.mu.Unlock()
	f.agent.addresses = append(f.agent.addresses, address)
	if f.agent.err != nil {
		return wsagent.AgentInterruptResult{}, f.agent.err
	}
	return wsagent.AgentInterruptResult{Address: address, Status: wsagent.AgentIdle, Returned: f.agent.returned}, nil
}

func (f *fakePiAgents) AgentList(caller wsagent.AgentCaller) []wsagent.AgentChild {
	f.record(caller)
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.agent.children
}

func (f *fakePiAgents) AgentTiers(caller wsagent.AgentCaller) (wsagent.AgentTiers, error) {
	f.record(caller)
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.agent.tiers, f.agent.err
}

var agentToolNames = []string{"agent.interrupt", "agent.list", "agent.spawn", "agent.tiers"}

func listedAgentTools(s *Server) []string {
	var out []string
	for _, tool := range s.filteredTools() {
		if name, _ := tool["name"].(string); isAgentTool(name) {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

// agentLeadFixture is a lead session on a Claude-hosted server over agents
// (nil: no runner).
func agentLeadFixture(t *testing.T, manifests string, agents PiAgents) piFixture {
	t.Helper()
	setupMailboxTestEnv(t)
	t.Setenv(envConfigManifests, manifests)
	root := t.TempDir()
	initGit(t, root)
	s := NewServerWithOptions(ServerOptions{Root: root, Version: "test", Agents: agents, Harness: "claude"})
	t.Cleanup(s.Close)
	return piFixture{root: root, key: mailboxLogin(t, s, 1, root), server: s}
}

// TestAgentToolsListedWhereARunnerAnswers: a lead server lists the agent.*
// tools with the daemon's runner, whatever dashboard.pi_spawn_enabled and
// the Pi install say, and in the agentless product mode too; without a
// runner (or a token) it lists none, and a call refuses naming why.
func TestAgentToolsListedWhereARunnerAnswers(t *testing.T) {
	setPiVersion(t, "", errors.New("no pi here"))
	wsflow := dashboardPackages[1]
	for _, tc := range []struct {
		name      string
		manifests string
		noAgent   bool
		agents    bool
	}{
		{name: "ws runner", manifests: wsManifests(), agents: true},
		{name: "wsflow runner, pi knob off", manifests: wsflow.own + ";" + wsflow.dashboard, agents: true},
		{name: "agentless mode runner", manifests: wsflow.own + ";" + wsflow.dashboard, noAgent: true, agents: true},
		{name: "no runner", manifests: wsManifests()},
		{name: "agentless mode no runner", manifests: wsManifests(), noAgent: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.noAgent {
				t.Setenv(envNoAgent, "1")
			}
			var agents PiAgents
			if tc.agents {
				agents = newFakePiAgents()
			}
			f := agentLeadFixture(t, tc.manifests, agents)
			listed := listedAgentTools(f.server)
			if !tc.agents {
				if len(listed) != 0 {
					t.Fatalf("no runner lists no agent.* tool: %v", listed)
				}
				if out := callToolWithKey(t, f.server, 2, f.key, "agent.list", nil); !strings.Contains(out, "agent.list: spawned agents run in the dashboard daemon's agent runner") {
					t.Fatalf("a call without a runner refuses naming why: %q", out)
				}
				return
			}
			if strings.Join(listed, ",") != strings.Join(agentToolNames, ",") {
				t.Fatalf("listed agent tools = %v, want %v", listed, agentToolNames)
			}
			if out := callToolWithKey(t, f.server, 2, f.key, "agent.list", nil); out != "no child agents\n" {
				t.Fatalf("agent.list = %q", out)
			}
		})
	}
}

func TestAgentToolsAreLeadOnlyToAKey(t *testing.T) {
	f := agentLeadFixture(t, wsManifests(), newFakePiAgents())
	for _, name := range agentToolNames {
		if !isLeadOnlyTool(name) {
			t.Fatalf("%s is lead-only to a key", name)
		}
	}
	delegate, err := f.server.sessions.mint(f.root, roleDelegate, "")
	if err != nil {
		t.Fatal(err)
	}
	if raw := callToolOnce(t, f.server, 2, "agent.list", map[string]any{"session_key": delegate}); !strings.Contains(raw, "tool not available in current") {
		t.Fatalf("a delegate key without a token is refused: %s", raw)
	}
}

// TestAgentLeadPassesItsCallerAndMapsErrors: a lead's agent.* calls reach
// the runner with the lead as caller (its reply-id as parent, its root,
// its harness, its config), and the runner's answers and refusals come
// back as text or JSON.
func TestAgentLeadPassesItsCallerAndMapsErrors(t *testing.T) {
	agents := newFakePiAgents()
	f := agentLeadFixture(t, wsManifests(), agents)
	root, err := filepath.EvalSymlinks(f.root)
	if err != nil {
		t.Fatal(err)
	}

	args := map[string]any{"prompt": "Fix it\nthen report", "tier": "pi.small", "authority": "read-only", "system_prompt_path": "p.md", "worktree": "wt", "label": "fixer"}
	out := callToolWithKey(t, f.server, 2, f.key, "agent.spawn", args)
	for _, want := range []string{"spawned agent id:kid1 (starting)", "completion mail", `mailbox.send(to: "id:kid1")`} {
		if !strings.Contains(out, want) {
			t.Fatalf("agent.spawn lacks %q: %q", want, out)
		}
	}
	caller := agents.agent.callers[0]
	want := wsagent.ParentRef{Kind: wsagent.ParentLead, ReplyID: wsmailbox.ReplyID(f.key)}
	if caller.Parent != want || caller.Root != root || caller.WorkDir != "" || caller.Harness != "claude" ||
		caller.Config.RepoRoot != root || caller.Config.ConfigHome != os.Getenv(wsconfig.EnvConfigHome) {
		t.Fatalf("caller = %+v", caller)
	}
	if req := agents.agent.requests[0]; req != (wsagent.AgentSpawnRequest{Prompt: "Fix it\nthen report", Tier: "pi.small", Authority: "read-only", SystemPromptPath: "p.md", Worktree: "wt", Label: "fixer"}) {
		t.Fatalf("request = %+v", req)
	}
	raw := callToolWithKey(t, f.server, 3, f.key, "agent.spawn", map[string]any{"prompt": "x", "tier": "small", "authority": "full", "system_prompt": "be brief", "cwd": "sub", "format": "json"})
	var res wsagent.AgentSpawnResult
	if err := json.Unmarshal([]byte(raw), &res); err != nil || res.Address != "id:kid1" || res.Status != wsagent.AgentStarting {
		t.Fatalf("agent.spawn json = %q (%v)", raw, err)
	}
	if req := agents.agent.requests[1]; req.SystemPrompt != "be brief" || req.Cwd != "sub" || req.Authority != "full" {
		t.Fatalf("request = %+v", req)
	}
	if raw := callToolOnce(t, f.server, 4, "agent.spawn", map[string]any{"session_key": f.key, "prompt": "x", "tier": "small"}); !strings.Contains(raw, "authority") {
		t.Fatalf("authority is a required argument: %s", raw)
	}

	agents.agent.err = errors.New(`agent.spawn: authority full is above this session's own level workdir-write, which caps its children; pick workdir-write or lower`)
	if out := callToolWithKey(t, f.server, 5, f.key, "agent.spawn", map[string]any{"prompt": "x", "tier": "small", "authority": "full"}); out != agents.agent.err.Error() {
		t.Fatalf("a runner refusal is passed through once: %q", out)
	}
	agents.agent.err = errors.New("agent.interrupt: id:other is not an agent this session spawned (agent.list lists them)")
	if out := callToolWithKey(t, f.server, 6, f.key, "agent.interrupt", map[string]any{"address": "id:other"}); out != agents.agent.err.Error() {
		t.Fatalf("an ownership error surfaces: %q", out)
	}
	agents.agent.err = errors.New("tiers unreadable")
	if out := callToolWithKey(t, f.server, 7, f.key, "agent.tiers", nil); out != "agent.tiers: tiers unreadable" {
		t.Fatalf("an unprefixed error is named by the tool: %q", out)
	}
	agents.agent.err = nil

	agents.agent.returned = []string{"second\nmessage"}
	if out := callToolWithKey(t, f.server, 8, f.key, "agent.interrupt", map[string]any{"address": "id:kid1"}); !strings.Contains(out, "interrupted agent id:kid1 (idle); it stays") || !strings.Contains(out, "- second message") {
		t.Fatalf("agent.interrupt = %q", out)
	}
	if got := agents.agent.addresses; len(got) != 2 || got[1] != "id:kid1" {
		t.Fatalf("addresses = %v", got)
	}

	agents.agent.children = []wsagent.AgentChild{{Address: "id:kid1", Label: "fixer", Tier: "pi.small", Authority: wsagent.PermissionReadOnly, State: wsagent.AgentWaiting, LastReport: "done: all green", Model: "m", Effort: "low", Cwd: root}}
	if out := callToolWithKey(t, f.server, 9, f.key, "agent.list", nil); !strings.Contains(out, "id:kid1  waiting  pi.small  read-only  m/low  "+root+`  "fixer"`) || !strings.Contains(out, "last report: done: all green") {
		t.Fatalf("agent.list = %q", out)
	}
	agents.agent.tiers = wsagent.AgentTiers{Harness: "claude", Native: []wsagent.AgentTierEntry{{Tier: "small", Model: "haiku"}},
		Ext:     []wsagent.AgentTierEntry{{Tier: "pi.small", Model: "luna", Effort: "high"}},
		Aliases: []wsagent.AgentAliasEntry{{Alias: "mockup", Target: "pi.large", Error: `tier "large" is not set for pi`}}}
	out = callToolWithKey(t, f.server, 10, f.key, "agent.tiers", nil)
	for _, want := range []string{"native (bare tiers on this session's harness, claude):\n  small: haiku/-", "  pi.small: luna/high", `mockup -> pi.large: not spawnable now: tier "large" is not set for pi`} {
		if !strings.Contains(out, want) {
			t.Fatalf("agent.tiers lacks %q: %q", want, out)
		}
	}
	for _, c := range agents.agent.callers {
		if c.Parent != want {
			t.Fatalf("every call's caller is the lead: %+v", c)
		}
	}
}

// fakeAgentRoute is the daemon's token route for the agent.* ops.
type fakeAgentRoute struct {
	mu    sync.Mutex
	calls []fakeTokenCall
}

func (f *fakeAgentRoute) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	raw, _ := io.ReadAll(r.Body)
	var body map[string]any
	_ = json.Unmarshal(raw, &body)
	f.mu.Lock()
	f.calls = append(f.calls, fakeTokenCall{path: r.URL.Path, auth: r.Header.Get("Authorization"), body: body})
	f.mu.Unlock()
	if r.Header.Get("Authorization") != "Bearer tok123" {
		http.Error(w, "the capability token is not valid", http.StatusUnauthorized)
		return
	}
	answer := func(status int, v any) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(v)
	}
	switch strings.TrimPrefix(r.URL.Path, wsagent.RoutePrefix+"/by-token/") {
	case "agent.spawn":
		answer(http.StatusCreated, wsagent.AgentSpawnResult{Address: "id:kid9", Status: wsagent.AgentStarting})
	case "agent.interrupt":
		if body["address"] != "id:kid9" {
			http.Error(w, "agent.interrupt: "+body["address"].(string)+" is not an agent this session spawned (agent.list lists them)", http.StatusNotFound)
			return
		}
		answer(http.StatusOK, wsagent.AgentInterruptResult{Address: "id:kid9", Status: wsagent.AgentIdle, Returned: []string{}})
	case "agent.list":
		answer(http.StatusOK, map[string]any{"agents": []wsagent.AgentChild{{Address: "id:kid9", State: wsagent.AgentRunning, Tier: "pi.small"}}})
	case "agent.tiers":
		answer(http.StatusOK, wsagent.AgentTiers{Harness: "pi", Native: []wsagent.AgentTierEntry{{Tier: "small", Model: "luna"}}})
	default:
		http.NotFound(w, r)
	}
}

func (f *fakeAgentRoute) last() fakeTokenCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls[len(f.calls)-1]
}

// TestAgentWorkerForwardsOnItsToken: a spawned agent's ws-mcp (no runner,
// a delegate key, its loadout as the allowlist) lists the agent.* tools
// in a worker's words and forwards each call to the token route on its
// token; a daemon refusal reaches it as is.
func TestAgentWorkerForwardsOnItsToken(t *testing.T) {
	setupMailboxTestEnv(t)
	route := &fakeAgentRoute{}
	daemon := httptest.NewServer(route)
	defer daemon.Close()
	root := t.TempDir()
	initGit(t, root)

	// The loadout without agent.* (a pi.spawn worker) lists none.
	t.Setenv(wsagent.EnvPiToken, "tok123")
	t.Setenv(wsagent.EnvPiEndpoint, daemon.URL+"/")
	t.Setenv(envAllowedTools, strings.Join(wsagent.PiWorkerWSTools, ","))
	other := NewServer(root, "test")
	defer other.Close()
	if got := listedAgentTools(other); len(got) != 0 {
		t.Fatalf("an allowlist without agent.* lists none: %v", got)
	}

	t.Setenv(envAllowedTools, strings.Join(wsagent.AgentWSTools, ","))
	s := NewServer(root, "test")
	defer s.Close()
	key, err := s.sessions.mint(root, roleDelegate, "")
	if err != nil {
		t.Fatal(err)
	}
	if got := listedAgentTools(s); strings.Join(got, ",") != strings.Join(agentToolNames, ",") {
		t.Fatalf("listed = %v", got)
	}
	for _, tool := range s.filteredTools() {
		name, _ := tool["name"].(string)
		if !isAgentTool(name) {
			continue
		}
		desc, _ := tool["description"].(string)
		if strings.Contains(desc, "this session") || strings.Contains(desc, "pi.send") || strings.Contains(desc, "mailbox:") {
			t.Fatalf("%s reads as a lead's: %q", name, desc)
		}
		props, _ := tool["inputSchema"].(map[string]any)["properties"].(map[string]any)
		if k := props["session_key"].(map[string]string)["description"]; !strings.Contains(k, "Your own ws session key") {
			t.Fatalf("%s session_key reads as a lead's: %q", name, k)
		}
		if name == "agent.spawn" {
			if cwd := props["cwd"].(map[string]string)["description"]; !strings.Contains(cwd, "relative to your own working directory") {
				t.Fatalf("agent.spawn cwd reads as a lead's: %q", cwd)
			}
		}
	}

	out := callToolWithKey(t, s, 2, key, "agent.spawn", map[string]any{"prompt": "sub task", "tier": "small", "authority": "read-only", "system_prompt": "sys", "cwd": "sub", "label": "sub"})
	if !strings.Contains(out, "spawned agent id:kid9 (starting)") || !strings.Contains(out, "steered into your running turn") || !strings.Contains(out, `mailbox.send(to: "id:kid9")`) || strings.Contains(out, "pi.send") {
		t.Fatalf("agent.spawn = %q", out)
	}
	call := route.last()
	if call.path != wsagent.RoutePrefix+"/by-token/agent.spawn" || call.auth != "Bearer tok123" || call.body["prompt"] != "sub task" || call.body["tier"] != "small" ||
		call.body["authority"] != "read-only" || call.body["system_prompt"] != "sys" || call.body["cwd"] != "sub" || call.body["label"] != "sub" {
		t.Fatalf("forwarded spawn = %+v", call)
	}
	if out := callToolWithKey(t, s, 3, key, "agent.interrupt", map[string]any{"address": "id:nope"}); out != "agent.interrupt: id:nope is not an agent this session spawned (agent.list lists them)" {
		t.Fatalf("an ownership refusal reaches the worker once: %q", out)
	}
	if out := callToolWithKey(t, s, 4, key, "agent.interrupt", map[string]any{"address": "id:kid9"}); !strings.Contains(out, "interrupted agent id:kid9 (idle)") || route.last().path != wsagent.RoutePrefix+"/by-token/agent.interrupt" {
		t.Fatalf("agent.interrupt = %q", out)
	}
	if out := callToolWithKey(t, s, 5, key, "agent.list", nil); !strings.Contains(out, "id:kid9  running  pi.small") {
		t.Fatalf("agent.list = %q", out)
	}
	if out := callToolWithKey(t, s, 6, key, "agent.tiers", map[string]any{"format": "json"}); !strings.Contains(out, `"harness":"pi"`) || route.last().path != wsagent.RoutePrefix+"/by-token/agent.tiers" {
		t.Fatalf("agent.tiers = %q", out)
	}
	// The token authorizes agent.* alone: a lead-only tool stays refused.
	if raw := callToolOnce(t, s, 7, "workflow_state", map[string]any{"session_key": key}); !strings.Contains(raw, "tool not available in current") {
		t.Fatalf("a lead-only tool with a token: %s", raw)
	}
}

// TestReadOnlyAgentRefusesMutatingTools: a read-only agent's ws-mcp, from
// its process env or a hosted session's hello env, refuses the loadout's
// mutating tools naming the level; mailbox.send and every tool of a
// workdir-write or full agent pass the gate.
func TestReadOnlyAgentRefusesMutatingTools(t *testing.T) {
	setupMailboxTestEnv(t)
	root := t.TempDir()
	initGit(t, root)
	mutating := []string{"git.commit", "tickets.create_empty", "tickets.acquire", "tickets.release", "tickets.close", "tickets.move"}
	servers := map[string]func(t *testing.T, level string) *Server{
		"process env": func(t *testing.T, level string) *Server {
			t.Setenv(wsagent.EnvPermissionLevel, level)
			return NewServer(root, "test")
		},
		"hosted env": func(t *testing.T, level string) *Server {
			t.Setenv(wsagent.EnvPermissionLevel, "")
			return NewServerWithOptions(ServerOptions{Root: root, Version: "test", Env: append(os.Environ(), wsagent.EnvPermissionLevel+"="+level)})
		},
	}
	for name, build := range servers {
		t.Run(name, func(t *testing.T) {
			for _, level := range []string{"read-only", "workdir-write", "full", ""} {
				s := build(t, level)
				for _, tool := range append(mutating, "mailbox.send", "git.status") {
					raw := callToolOnce(t, s, 1, tool, map[string]any{"session_key": "k"})
					refused := strings.Contains(raw, "permission denied")
					if want := level == "read-only" && tool != "mailbox.send" && tool != "git.status"; refused != want {
						t.Fatalf("%s at %q: refused %v, want %v: %s", tool, level, refused, want, raw)
					}
					if refused && (!strings.Contains(raw, "authority read-only") || !strings.Contains(raw, "the session that spawned it chose") || !strings.Contains(raw, `"isError":true`)) {
						t.Fatalf("%s refusal names the level and its chooser: %s", tool, raw)
					}
				}
				s.Close()
			}
		})
	}
}

func TestConfigTuneAgentKeys(t *testing.T) {
	setupMailboxTestEnv(t)
	root := t.TempDir()
	initGit(t, root)
	s := NewServer(root, "test")
	defer s.Close()
	key := mailboxLogin(t, s, 1, root)

	if out := callToolWithKey(t, s, 2, key, "config.get", map[string]any{"key": wsconfig.ItemAgentChildWaitingDays}); !strings.Contains(out, "15") {
		t.Fatalf("child_waiting_days builtin = %q", out)
	}
	for i, tc := range []struct {
		key, value, want string
		ok               bool
	}{
		{wsconfig.ItemAgentTierAliases, "mockup=pi.large  Quick=claude.small", "agent.tier_aliases: mockup=pi.large  Quick=claude.small [scope:global]", true},
		{wsconfig.ItemAgentTierAliases, "mockup", `is not name=<harness>.<tier>`, false},
		{wsconfig.ItemAgentTierAliases, "mockup=pi.huge", "target must be <harness>.<tier>", false},
		{wsconfig.ItemAgentChildWaitingDays, "07", "agent.child_waiting_days: 7 [scope:global]", true},
		{wsconfig.ItemAgentChildWaitingDays, "0", "a whole number of days, at least 1", false},
		{wsconfig.ItemAgentChildWaitingDays, "two", "a whole number of days, at least 1", false},
	} {
		out := callToolWithKey(t, s, 10+i, key, "config.tune", map[string]any{"key": tc.key, "value": tc.value})
		if !strings.Contains(out, tc.want) || strings.HasPrefix(out, "config.tune:") == tc.ok {
			t.Fatalf("config.tune %s=%q = %q, want ok %v with %q", tc.key, tc.value, out, tc.ok, tc.want)
		}
	}
	resolver := wsconfig.NewResolver(wsconfig.Options{}, nil, nil, nil)
	aliases, err := resolver.Get("", wsconfig.ItemAgentTierAliases)
	if err != nil || aliases.Value != "mockup=pi.large  Quick=claude.small" {
		t.Fatalf("stored aliases = %+v, %v", aliases, err)
	}
	if out := callToolWithKey(t, s, 20, key, "config.tune", map[string]any{"key": wsconfig.ItemAgentChildWaitingDays, "value": "3", "scope": "project"}); !strings.Contains(out, "global") {
		t.Fatalf("a project write of a global-only key is refused: %q", out)
	}
	if out := callToolWithKey(t, s, 21, key, "config.tune", map[string]any{"key": wsconfig.ItemAgentChildWaitingDays, "reset": true}); !strings.Contains(out, "agent.child_waiting_days: 15") {
		t.Fatalf("reset = %q", out)
	}
	for _, k := range []string{wsconfig.ItemAgentTierAliases, wsconfig.ItemAgentChildWaitingDays} {
		entry, ok := resolveConfigEntryForKey(k)
		if !ok || entry.Key != k || !entry.RequiresLeadAuthority || !entry.GlobalOnly() || !entry.NoAgentVisible || entry.Structured {
			t.Fatalf("%s entry = %+v, %v", k, entry, ok)
		}
	}
}

func TestAgentWakeRefusedMailsTheSender(t *testing.T) {
	setupMailboxTestEnv(t)
	root := t.TempDir()
	initGit(t, root)
	s := NewServer(root, "test")
	defer s.Close()
	key := mailboxLogin(t, s, 1, root)
	sub := wswake.Mailbox.Subscribe()
	defer sub.Close()

	AgentWakeRefused(io.Discard)(wsmailbox.ReplyID(key), "kid1", "permission admission: authority full is above the parent's current level workdir-write")
	select {
	case <-sub.Hard():
	case <-time.After(time.Second):
		t.Fatalf("the notice did not wake the mailbox waits")
	}
	out := callToolWithKey(t, s, 2, key, "mailbox.recv", nil)
	for _, want := range []string{"agent:kid1", "The wake of agent id:kid1 was refused: permission admission: authority full is above", "stays queued for it"} {
		if !strings.Contains(out, want) {
			t.Fatalf("mailbox.recv lacks %q: %q", want, out)
		}
	}
	if strings.Count(out, "The wake of agent") != 1 {
		t.Fatalf("one mail per refusal: %q", out)
	}
}
