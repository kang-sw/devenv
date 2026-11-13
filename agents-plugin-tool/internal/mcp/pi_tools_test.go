package mcp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsagent"
	"github.com/kang-sw/devenv/internal/wsconfig"
	"github.com/kang-sw/devenv/internal/wslive"
	"github.com/kang-sw/devenv/internal/wsmailbox"
	"github.com/kang-sw/devenv/internal/wswake"
)

// fakePiAgents is PiAgents over an in-memory list of children.
type fakePiAgents struct {
	mu       sync.Mutex
	n        int
	specs    []wsagent.SpawnSpec
	states   map[string]wsagent.State
	order    []string
	sent     map[string][]string
	watchers map[int]func(wsagent.State)
	watchN   int
	waited   []time.Duration
	waitRes  wsagent.TurnResult
}

func newFakePiAgents() *fakePiAgents {
	return &fakePiAgents{states: map[string]wsagent.State{}, sent: map[string][]string{}, watchers: map[int]func(wsagent.State){}}
}

func (f *fakePiAgents) Spawn(spec wsagent.SpawnSpec) (wsagent.State, error) {
	f.mu.Lock()
	f.n++
	id := fmt.Sprintf("child%d", f.n)
	parent := *spec.Parent
	st := wsagent.State{ID: id, Profile: spec.Profile, Root: spec.Root, Status: wsagent.StatusRunning, Model: spec.Model, Effort: spec.Effort, Label: spec.Label, Parent: &parent}
	if spec.Cwd != spec.Root {
		st.Cwd = spec.Cwd
	}
	f.specs = append(f.specs, spec)
	f.states[id] = st
	f.order = append(f.order, id)
	f.mu.Unlock()
	f.emit(st)
	return st, nil
}

func (f *fakePiAgents) Send(id, text string) (wsagent.State, error) {
	f.mu.Lock()
	f.sent[id] = append(f.sent[id], text)
	st := f.states[id]
	f.mu.Unlock()
	return st, nil
}

func (f *fakePiAgents) Get(id string) (wsagent.State, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	st, ok := f.states[id]
	if !ok {
		return wsagent.State{}, errors.New("unknown agent conversation")
	}
	return st, nil
}

func (f *fakePiAgents) Children(parent wsagent.ParentRef) []wsagent.State {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []wsagent.State
	for _, id := range f.order {
		if st := f.states[id]; st.Parent != nil && *st.Parent == parent {
			out = append(out, st)
		}
	}
	return out
}

func (f *fakePiAgents) CloseConversation(id string) (wsagent.State, error) {
	return f.set(id, func(st *wsagent.State) { st.Status = wsagent.StatusClosed }), nil
}

func (f *fakePiAgents) Wait(_ context.Context, id string, timeout time.Duration) (wsagent.TurnResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.waited = append(f.waited, timeout)
	res := f.waitRes
	res.ID = id
	return res, nil
}

func (f *fakePiAgents) Watch(fn func(wsagent.State)) func() {
	f.mu.Lock()
	f.watchN++
	n := f.watchN
	f.watchers[n] = fn
	f.mu.Unlock()
	return func() {
		f.mu.Lock()
		delete(f.watchers, n)
		f.mu.Unlock()
	}
}

func (f *fakePiAgents) Transcript(id string) ([]wslive.RingRecord, error) {
	at := time.Date(2026, 11, 13, 0, 0, 0, 0, time.UTC)
	return []wslive.RingRecord{
		{RingEntry: wslive.RingEntry{Kind: wslive.RingUser, Text: "do it"}, At: at},
		{RingEntry: wslive.RingEntry{Kind: wslive.RingTools, Tools: &wslive.ToolSummary{Calls: 3, Tools: []wslive.ToolTally{{Name: "bash", Calls: 3}}}}, At: at},
		{RingEntry: wslive.RingEntry{Kind: wslive.RingReply, Text: "done it"}, At: at},
	}, nil
}

// set changes a child's state and tells the watchers, as the runner does.
func (f *fakePiAgents) set(id string, change func(*wsagent.State)) wsagent.State {
	f.mu.Lock()
	st := f.states[id]
	change(&st)
	f.states[id] = st
	f.mu.Unlock()
	f.emit(st)
	return st
}

func (f *fakePiAgents) emit(st wsagent.State) {
	f.mu.Lock()
	fns := make([]func(wsagent.State), 0, len(f.watchers))
	for _, fn := range f.watchers {
		fns = append(fns, fn)
	}
	f.mu.Unlock()
	for _, fn := range fns {
		fn(st)
	}
}

func (f *fakePiAgents) watcherCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.watchers)
}

// setPiVersion makes the Pi probe answer version (or fail with err) and
// forgets the cached answer.
func setPiVersion(t *testing.T, version string, err error) {
	t.Helper()
	previousProbe, previousCache := piProbe, piVersion
	piProbe = func(context.Context) (string, error) { return version, err }
	piVersion = new(piVersionCache)
	t.Cleanup(func() { piProbe, piVersion = previousProbe, previousCache })
}

// piFixture is a lead session on a root whose launcher loaded manifests,
// with a server built over agents (nil: the direct path).
type piFixture struct {
	root   string
	key    string
	server *Server
}

func newPiFixture(t *testing.T, manifests string, agents PiAgents) piFixture {
	t.Helper()
	setupMailboxTestEnv(t)
	t.Setenv(envConfigManifests, manifests)
	root := t.TempDir()
	initGit(t, root)
	s := NewServerWithOptions(ServerOptions{Root: root, Version: "test", Agents: agents})
	t.Cleanup(s.Close)
	return piFixture{root: root, key: mailboxLogin(t, s, 1, root), server: s}
}

func wsManifests() string {
	pkg := dashboardPackages[0]
	return pkg.own + ";" + pkg.dashboard
}

var piToolNames = []string{"pi.list", "pi.send", "pi.spawn", "pi.stop", "pi.transcript", "pi.wait"}

func TestPiSendDescribesNativeSteeringWithoutChangingArguments(t *testing.T) {
	for _, def := range piToolDefinitions() {
		if def["name"] != "pi.send" {
			continue
		}
		for _, tool := range []map[string]any{def, piWorkerTool(def)} {
			desc := tool["description"].(string)
			for _, want := range []string{"With no turn running it starts the next turn now", "steers that running turn at the next tool boundary", "without cancelling in-flight tool calls", "no extra turn", "closed subagent resumes on its existing session file with the same id and settings", "missing file is refused"} {
				if !strings.Contains(desc, want) {
					t.Errorf("pi.send description lacks %q: %s", want, desc)
				}
			}
			props := tool["inputSchema"].(map[string]any)["properties"].(map[string]any)
			if len(props) != 4 || props["session_key"] == nil || props["id"] == nil || props["message"] == nil || props["format"] == nil {
				t.Fatalf("pi.send arguments changed: %v", props)
			}
		}
		return
	}
	t.Fatal("pi.send is missing")
}

func listedPiTools(s *Server) []string {
	var out []string
	for _, tool := range s.filteredTools() {
		if name, _ := tool["name"].(string); isPiTool(name) {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

// TestPiToolsGate pins the exposure gate: tools/list shows the pi.* tools
// only with the knob on, a daemon runner, and a Pi of 1.0 or later, and a
// call that comes anyway refuses with the reason.
func TestPiToolsGate(t *testing.T) {
	wsflow := dashboardPackages[1]
	for _, tc := range []struct {
		name      string
		manifests string
		agents    bool
		version   string
		probeErr  error
		refusal   string
	}{
		{name: "on", manifests: wsManifests(), agents: true, version: "1.0.4"},
		{name: "knob off", manifests: wsflow.own + ";" + wsflow.dashboard, agents: true, version: "1.0.4", refusal: DashboardPiSpawnKey + " is off"},
		{name: "pi missing", manifests: wsManifests(), agents: true, probeErr: errors.New(`exec: "pi": executable file not found in $PATH`), refusal: "no usable pi"},
		{name: "pi too old", manifests: wsManifests(), agents: true, version: "0.72.1", refusal: "need pi 1.0 or later"},
		{name: "no daemon", manifests: wsManifests(), version: "1.0.4", refusal: "not hosted by the dashboard daemon"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setPiVersion(t, tc.version, tc.probeErr)
			var agents PiAgents
			if tc.agents {
				agents = newFakePiAgents()
			}
			f := newPiFixture(t, tc.manifests, agents)
			listed := listedPiTools(f.server)
			if tc.refusal == "" {
				if strings.Join(listed, ",") != strings.Join(piToolNames, ",") {
					t.Fatalf("listed pi tools = %v, want %v", listed, piToolNames)
				}
				if out := callToolWithKey(t, f.server, 2, f.key, "pi.list", nil); out != "no Pi subagents\n" {
					t.Fatalf("pi.list = %q", out)
				}
				return
			}
			if len(listed) != 0 {
				t.Fatalf("%s: pi tools listed: %v", tc.name, listed)
			}
			if out := callToolWithKey(t, f.server, 2, f.key, "pi.list", nil); !strings.Contains(out, tc.refusal) {
				t.Fatalf("%s: a call should refuse naming %q, got %q", tc.name, tc.refusal, out)
			}
		})
	}
}

// TestPiToolsKnobReadsTheProjectAndGlobalValues: the knob resolves through
// the machine-local project and global files over the package default, so
// a wsflow project opts in, and a ws user opts out, by a tune.
func TestPiToolsKnobReadsTheProjectAndGlobalValues(t *testing.T) {
	wsflow := dashboardPackages[1]
	for _, tc := range []struct {
		name      string
		manifests string
		scope     wsconfig.Scope
		value     string
		listed    bool
	}{
		{name: "wsflow project on", manifests: wsflow.own + ";" + wsflow.dashboard, scope: wsconfig.ScopeProject, value: "true", listed: true},
		{name: "wsflow global on", manifests: wsflow.own + ";" + wsflow.dashboard, scope: wsconfig.ScopeGlobal, value: "true", listed: true},
		{name: "ws project off", manifests: wsManifests(), scope: wsconfig.ScopeProject, value: "false"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setPiVersion(t, "1.0.4", nil)
			f := newPiFixture(t, tc.manifests, newFakePiAgents())
			r := wsconfig.NewResolver(wsconfig.Options{}, nil, nil, nil)
			if err := r.Set(DashboardPiSpawnKey, tc.value, wsconfig.SetOptions{ExplicitScope: tc.scope}); err != nil {
				t.Fatal(err)
			}
			if got := len(listedPiTools(f.server)) > 0; got != tc.listed {
				t.Fatalf("listed = %v, want %v", got, tc.listed)
			}
			out := callToolWithKey(t, f.server, 2, f.key, "pi.list", nil)
			if refused := strings.Contains(out, " is off"); refused == tc.listed {
				t.Fatalf("pi.list = %q, want refused %v", out, !tc.listed)
			}
		})
	}
}

func TestPiToolsRefuseANonLeadKey(t *testing.T) {
	setPiVersion(t, "1.0.4", nil)
	f := newPiFixture(t, wsManifests(), newFakePiAgents())
	if !isLeadOnlyTool("pi.spawn") || !isLeadOnlyTool("pi.wait") {
		t.Fatalf("the pi tools are lead-only")
	}
	if out := callToolWithKey(t, f.server, 2, "no-such-key", "pi.list", nil); !strings.Contains(out, "unknown_session") {
		t.Fatalf("an unknown key is refused: %q", out)
	}
}

func piTierOptions() wsconfig.Options {
	return wsconfig.Options{ConfigHome: os.Getenv(wsconfig.EnvConfigHome), CacheHome: os.Getenv("WS_CACHE_HOME")}
}

func TestPiSpawnResolvesThePiTierAndCwd(t *testing.T) {
	setPiVersion(t, "1.0.4", nil)
	agents := newFakePiAgents()
	f := newPiFixture(t, wsManifests(), agents)

	out := callToolWithKey(t, f.server, 2, f.key, "pi.spawn", map[string]any{"prompt": "x", "tier": "small"})
	if !strings.Contains(out, `no Pi tier is set`) || !strings.Contains(out, `config.tune key "agents.tier" with harness "pi"`) {
		t.Fatalf("an empty pi bucket refuses naming the key: %q", out)
	}

	if _, err := wsconfig.SetGlobalAgentsTierForHarness(piTierOptions(), "small", "", "openai-codex/luna", "pi", "high"); err != nil {
		t.Fatal(err)
	}
	if _, err := wsconfig.SetGlobalAgentsTierForHarness(piTierOptions(), "medium", "", "sol", "pi", "medium"); err != nil {
		t.Fatal(err)
	}
	out = callToolWithKey(t, f.server, 3, f.key, "pi.spawn", map[string]any{"prompt": "x", "tier": "large"})
	for _, want := range []string{`tier "large" is not set for Pi`, "(small, medium)", `config.tune key "agents.tier" with harness "pi", tier "large"`} {
		if !strings.Contains(out, want) {
			t.Fatalf("an unset tier refuses naming %q: %q", want, out)
		}
	}
	if len(agents.specs) != 0 {
		t.Fatalf("a refused spawn spawns nothing")
	}

	if err := os.Mkdir(filepath.Join(f.root, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	// The session's root as its login recorded it (symlinks resolved).
	root, err := filepath.EvalSymlinks(f.root)
	if err != nil {
		t.Fatal(err)
	}
	out = callToolWithKey(t, f.server, 4, f.key, "pi.spawn", map[string]any{"prompt": "Fix the build\nthen report", "tier": "small", "cwd": "sub"})
	if !strings.Contains(out, "spawned Pi subagent child1 (tier small: openai-codex/luna, effort high)") || !strings.Contains(out, "completion mail") {
		t.Fatalf("pi.spawn = %q", out)
	}
	spec := agents.specs[0]
	want := wsagent.ParentRef{Kind: wsagent.ParentLead, ReplyID: wsmailbox.ReplyID(f.key)}
	if spec.Profile != wsagent.PiWorkerProfile || spec.Root != root || spec.Cwd != filepath.Join(root, "sub") || spec.Model != "openai-codex/luna" || spec.Effort != "high" ||
		spec.Prompt != "Fix the build\nthen report" || spec.Label != spec.Prompt || spec.Parent == nil || *spec.Parent != want {
		t.Fatalf("spawn spec = %+v", spec)
	}
	abs := t.TempDir()
	callToolWithKey(t, f.server, 5, f.key, "pi.spawn", map[string]any{"prompt": "y", "tier": "medium", "cwd": abs, "label": "second"})
	if spec := agents.specs[1]; spec.Cwd != abs || spec.Label != "second" || spec.Model != "sol" {
		t.Fatalf("an absolute cwd and a label pass through: %+v", spec)
	}
	callToolWithKey(t, f.server, 6, f.key, "pi.spawn", map[string]any{"prompt": "z", "tier": "light"})
	if spec := agents.specs[2]; spec.Cwd != root || spec.Model != "openai-codex/luna" {
		t.Fatalf("no cwd is the root, and a legacy tier name maps to its tier: %+v", spec)
	}
}

func TestPiToolsScopeToTheCallingLead(t *testing.T) {
	setPiVersion(t, "1.0.4", nil)
	agents := newFakePiAgents()
	f := newPiFixture(t, wsManifests(), agents)
	other := mailboxLogin(t, f.server, 7, f.root)
	if _, err := wsconfig.SetGlobalAgentsTierForHarness(piTierOptions(), "small", "", "m", "pi", "high"); err != nil {
		t.Fatal(err)
	}
	callToolWithKey(t, f.server, 2, f.key, "pi.spawn", map[string]any{"prompt": "mine", "tier": "small", "label": "mine"})
	callToolWithKey(t, f.server, 3, other, "pi.spawn", map[string]any{"prompt": "theirs", "tier": "small"})

	list := callToolWithKey(t, f.server, 4, f.key, "pi.list", nil)
	if !strings.Contains(list, "child1") || strings.Contains(list, "child2") || !strings.Contains(list, `"mine"`) {
		t.Fatalf("pi.list shows the caller's children only: %q", list)
	}
	for _, tool := range []string{"pi.send", "pi.stop", "pi.transcript", "pi.wait"} {
		args := map[string]any{"id": "child2", "message": "hi"}
		if tool != "pi.send" {
			delete(args, "message")
		}
		if out := callToolWithKey(t, f.server, 5, f.key, tool, args); !strings.Contains(out, "is not one of this session's Pi subagents") {
			t.Fatalf("%s on another lead's child is refused: %q", tool, out)
		}
	}
	if len(agents.sent["child2"]) != 0 {
		t.Fatalf("a refused send reached the child")
	}

	if out := callToolWithKey(t, f.server, 6, f.key, "pi.send", map[string]any{"id": "child1", "message": "more"}); !strings.Contains(out, "steers its running turn at the next tool boundary") || !strings.Contains(out, "completion mail covers the message") {
		t.Fatalf("pi.send = %q", out)
	}
	if got := agents.sent["child1"]; len(got) != 1 || got[0] != "more" {
		t.Fatalf("sent = %v", got)
	}
	if out := callToolWithKey(t, f.server, 7, f.key, "pi.transcript", map[string]any{"id": "child1"}); !strings.Contains(out, "user:\ndo it") || !strings.Contains(out, "tools: 3 calls (bash x3)") || !strings.Contains(out, "reply:\ndone it") {
		t.Fatalf("pi.transcript = %q", out)
	}

	agents.waitRes = wsagent.TurnResult{Status: wsagent.TurnDone, Message: "the answer"}
	if out := callToolWithKey(t, f.server, 8, f.key, "pi.wait", map[string]any{"id": "child1"}); !strings.Contains(out, "turn done") || !strings.Contains(out, "the answer") {
		t.Fatalf("pi.wait = %q", out)
	}
	callToolWithKey(t, f.server, 9, f.key, "pi.wait", map[string]any{"id": "child1", "timeout_s": 5000})
	callToolWithKey(t, f.server, 10, f.key, "pi.wait", map[string]any{"id": "child1", "timeout_s": 2})
	if w := agents.waited; len(w) != 3 || w[0] != piWaitDefault || w[1] != piWaitMax || w[2] != 2*time.Second {
		t.Fatalf("wait timeouts = %v, want the default, the cap, and the asked", w)
	}
	agents.waitRes = wsagent.TurnResult{Status: wsagent.TurnRunning}
	if out := callToolWithKey(t, f.server, 11, f.key, "pi.wait", map[string]any{"id": "child1"}); !strings.Contains(out, "still running") {
		t.Fatalf("a timed-out wait = %q", out)
	}

	if out := callToolWithKey(t, f.server, 12, f.key, "pi.stop", map[string]any{"id": "child1"}); !strings.Contains(out, "closed Pi subagent child1") {
		t.Fatalf("pi.stop = %q", out)
	}
	if st, _ := agents.Get("child1"); st.Status != wsagent.StatusClosed {
		t.Fatalf("pi.stop closes the child: %+v", st)
	}
}

func TestPiTurnReporterMailsTheParent(t *testing.T) {
	setupMailboxTestEnv(t)
	root := t.TempDir()
	initGit(t, root)
	s := NewServer(root, "test")
	key := mailboxLogin(t, s, 1, root)
	sub := wswake.Mailbox.Subscribe()
	defer sub.Close()

	report := PiTurnReporter(io.Discard)
	report(wsagent.TurnReport{
		Parent: wsagent.ParentRef{Kind: "pi", ReplyID: wsmailbox.ReplyID(key)},
		Result: wsagent.TurnResult{ID: "childone", Status: wsagent.TurnDone},
	})
	select {
	case <-sub.Hard():
		t.Fatalf("a parent that is no lead gets no mail")
	default:
	}
	report(wsagent.TurnReport{
		Parent: wsagent.ParentRef{Kind: wsagent.ParentLead, ReplyID: wsmailbox.ReplyID(key)},
		Child:  wsagent.State{ID: "childone", Label: "worker"},
		Result: wsagent.TurnResult{ID: "childone", Status: wsagent.TurnDone, Message: "all green"},
	})
	select {
	case <-sub.Hard():
	case <-time.After(time.Second):
		t.Fatalf("the report did not wake the mailbox waits")
	}
	report(wsagent.TurnReport{
		Parent: wsagent.ParentRef{Kind: wsagent.ParentLead, ReplyID: wsmailbox.ReplyID(key)},
		Result: wsagent.TurnResult{ID: "childone", Status: wsagent.TurnInterrupted, Error: "the dashboard daemon exited while the turn ran"},
	})
	out := callToolWithKey(t, s, 2, key, "mailbox.recv", nil)
	for _, want := range []string{`system: [Pi subagent childone "worker"] turn done`, "all green", "system: [Pi subagent childone] turn interrupted: the dashboard daemon exited while the turn ran"} {
		if !strings.Contains(out, want) {
			t.Fatalf("mailbox.recv lacks %q: %q", want, out)
		}
	}
	if strings.Count(out, "[Pi subagent") != 2 {
		t.Fatalf("one mail per lead report: %q", out)
	}
}

// liveAgents is the agents of key's live state in s.
func liveAgents(t *testing.T, s *Server, key string) map[string]wslive.Agent {
	t.Helper()
	ls := s.live.existing(key)
	if ls == nil {
		t.Fatalf("no live session")
	}
	ls.mu.Lock()
	snap := ls.state.Snapshot(time.Now().UnixMilli())
	ls.mu.Unlock()
	out := map[string]wslive.Agent{}
	for _, a := range snap.Agents {
		out[a.ID] = a
	}
	return out
}

// TestPiRowsFeedTheParentsLiveStateAndReplay pins the agents-panel rows: a
// child's changes reach its parent's live state only, marked pi, and a new
// server (a reconnect) and an adapter reset get the open children's rows
// back from the runner.
func TestPiRowsFeedTheParentsLiveStateAndReplay(t *testing.T) {
	f := newLiveFixture(t, true)
	fastLive(t, 20*time.Millisecond)
	setPiVersion(t, "1.0.4", nil)
	agents := newFakePiAgents()
	s := NewServerWithOptions(ServerOptions{Root: f.root, Version: "test", Agents: agents})
	intake(t, s, f.keyA, map[string]any{"kind": "reset"})
	intake(t, s, f.keyB, map[string]any{"kind": "reset"})

	mine := wsagent.ParentRef{Kind: wsagent.ParentLead, ReplyID: f.replyA}
	child, _ := agents.Spawn(wsagent.SpawnSpec{Root: f.root, Cwd: f.root, Model: "m", Effort: "high", Label: "worker", Parent: &mine})
	theirs := wsagent.ParentRef{Kind: wsagent.ParentLead, ReplyID: f.replyB}
	agents.Spawn(wsagent.SpawnSpec{Root: f.root, Cwd: f.root, Label: "other", Parent: &theirs})
	row := liveAgents(t, s, f.keyA)["pi:"+child.ID]
	if row.AgentKind != wslive.AgentKindPi || row.Status != wslive.StatusRunning || row.Label != "worker" || row.Model != "m" || row.Effort != "high" {
		t.Fatalf("the spawned child's row = %+v", row)
	}
	if rows := liveAgents(t, s, f.keyA); len(rows) != 1 {
		t.Fatalf("another lead's child is not this lead's row: %+v", rows)
	}
	nested := wsagent.ParentRef{Kind: wsagent.ParentConversation, ReplyID: child.ID}
	grandchild, _ := agents.Spawn(wsagent.SpawnSpec{Root: f.root, Cwd: f.root, Label: "grandchild", Parent: &nested})
	if row := liveAgents(t, s, f.keyA)["pi:"+grandchild.ID]; row.ParentID != "pi:"+child.ID || row.Status != wslive.StatusRunning {
		t.Fatalf("the grandchild nests under its spawner: %+v", row)
	}
	if rows := liveAgents(t, s, f.keyB); len(rows) != 1 {
		t.Fatalf("another lead cannot see the descendant: %+v", rows)
	}
	agents.set(grandchild.ID, func(st *wsagent.State) { st.Status = wsagent.StatusIdle })
	if row := liveAgents(t, s, f.keyA)["pi:"+grandchild.ID]; row.Status != wslive.StatusDone {
		t.Fatalf("a grandchild settles its row: %+v", row)
	}
	agents.set(child.ID, func(st *wsagent.State) { st.Status = wsagent.StatusIdle })
	if row := liveAgents(t, s, f.keyA)["pi:"+child.ID]; row.Status != wslive.StatusDone {
		t.Fatalf("a settled turn ends the row: %+v", row)
	}
	agents.set(child.ID, func(st *wsagent.State) { st.Error = "boom" })
	if row := liveAgents(t, s, f.keyA)["pi:"+child.ID]; row.Status != wslive.StatusFailed {
		t.Fatalf("a failed turn fails the row: %+v", row)
	}

	// A reconnect: the old server ends, the new one's first intake seeds.
	s.Close()
	if n := agents.watcherCount(); n != 0 {
		t.Fatalf("a closed server stops watching the runner: %d watchers", n)
	}
	s2 := NewServerWithOptions(ServerOptions{Root: f.root, Version: "test", Agents: agents})
	defer s2.Close()
	agents.set(child.ID, func(st *wsagent.State) { st.Error, st.Status = "", wsagent.StatusRunning })
	intake(t, s2, f.keyA, map[string]any{"kind": "turn_start"})
	if row := liveAgents(t, s2, f.keyA)["pi:"+child.ID]; row.AgentKind != wslive.AgentKindPi || row.Status != wslive.StatusRunning {
		t.Fatalf("the row replays after a reconnect: %+v", row)
	}
	// An adapter reset clears the tree; the runner's rows come back.
	intake(t, s2, f.keyA, map[string]any{"kind": "reset"}, map[string]any{"kind": "agent", "agentId": "native"})
	rows := liveAgents(t, s2, f.keyA)
	if rows["pi:"+child.ID].AgentKind != wslive.AgentKindPi || rows["native"].ID == "" || rows["pi:"+grandchild.ID].ParentID != "pi:"+child.ID || rows["pi:"+grandchild.ID].Status != wslive.StatusDone {
		t.Fatalf("after a reset the pi row is back beside the adapter's: %+v", rows)
	}

	// Closing settles displayed rows; a new state seeds no closed tree.
	agents.set(grandchild.ID, func(st *wsagent.State) { st.Status = wsagent.StatusClosed })
	agents.set(child.ID, func(st *wsagent.State) { st.Status = wsagent.StatusClosed })
	s2.Close()
	s3 := NewServerWithOptions(ServerOptions{Root: f.root, Version: "test", Agents: agents})
	defer s3.Close()
	intake(t, s3, f.keyA, map[string]any{"kind": "turn_start"})
	if _, ok := liveAgents(t, s3, f.keyA)["pi:"+child.ID]; ok {
		t.Fatalf("a closed child is not seeded")
	}
}
