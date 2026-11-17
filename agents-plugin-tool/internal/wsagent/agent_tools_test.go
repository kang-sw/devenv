package wsagent

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsconfig"
	"github.com/kang-sw/devenv/internal/wsdashboard"
)

// agentHarness is childHarness with both worker harnesses spawnable for
// agent.*: lookPath finds the fake's executable, and claude's small,
// medium, and large tiers are set beside Pi's. configure, when not nil,
// edits the runner's options before a restart.
func agentHarness(t *testing.T, configure func(*Options)) (*harness, *reports) {
	t.Helper()
	h, got := childHarness(t)
	setLookPath(t, func(file string) (string, error) { return file, nil })
	for tier, model := range map[string]string{"small": "claude-small", "medium": "claude-medium", "large": "claude-large"} {
		if _, err := wsconfig.SetGlobalAgentsTierForHarness(h.config.Config, tier, "", model, BackendClaude, "high"); err != nil {
			t.Fatal(err)
		}
	}
	if configure != nil {
		configure(&h.config)
		h.runner.Close()
		h.start()
	}
	return h, got
}

// leadCaller is the agent.* caller of the harness's lead parent, a claude
// lead with no Runner record.
func (h *harness) leadCaller() AgentCaller {
	return LeadCaller(leadParent.ReplyID, h.root, BackendClaude, h.config.Config)
}

// holderOf is the agent.* caller of spawned conversation id.
func (h *harness) holderOf(id string) AgentCaller {
	h.t.Helper()
	c := h.runner.conv(id)
	if c == nil {
		h.t.Fatalf("%s has no loop", id)
	}
	return h.runner.holderCaller(c.record())
}

// agentSpawn spawns through agent.spawn and returns the child's id.
func (h *harness) agentSpawn(caller AgentCaller, req AgentSpawnRequest) string {
	h.t.Helper()
	res, err := h.runner.AgentSpawn(caller, req)
	if err != nil {
		h.t.Fatalf("AgentSpawn(%+v): %v", req, err)
	}
	return agentID(res.Address)
}

// agentRunning spawns through agent.spawn and waits for the child's
// process to take its first turn.
func (h *harness) agentRunning(caller AgentCaller, req AgentSpawnRequest) (string, *fakeProc) {
	h.t.Helper()
	n := h.backend.startCount()
	id := h.agentSpawn(caller, req)
	p := h.backend.waitProc(h.t, n)
	p.nextSent(h.t)
	return id, p
}

// asleep stops id's process (as its idle timer would) and waits until it
// is waiting.
func (h *harness) asleep(id string) {
	h.t.Helper()
	if _, err := h.runner.Stop(id); err != nil {
		h.t.Fatal(err)
	}
	eventuallyAsleep(h.t, h, id)
}

// settledPair is a lead's child a (process 0) with a child b of its own,
// each of whose turn settled done and reported, both asleep.
func settledPair(t *testing.T, h *harness) (a, b string) {
	t.Helper()
	a, ap := h.agentRunning(h.leadCaller(), AgentSpawnRequest{Prompt: "parent work", Tier: "medium", Authority: "workdir-write"})
	b, bp := h.agentRunning(h.holderOf(a), AgentSpawnRequest{Prompt: "child work", Tier: "small", Authority: "workdir-write"})
	ap.reply("interim")
	eventually(t, "a's turn held for b", func() bool { return h.record(a).HeldTurn != nil })
	bp.reply("b done")
	if text := ap.nextSent(t); !strings.Contains(text, "b done") {
		t.Fatalf("b's report turn = %q", text)
	}
	ap.reply("a done")
	eventually(t, "a's turn settled", func() bool {
		rec := h.record(a)
		return rec.LastTurn != nil && rec.LastTurn.Result.Message == "a done" && !rec.TurnOpen
	})
	h.asleep(b)
	h.asleep(a)
	return a, b
}

func agentRefused(t *testing.T, err error, status int, wants ...string) {
	t.Helper()
	if err == nil || requestStatus(err) != status {
		t.Fatalf("err = %v (status %d), want status %d", err, requestStatus(err), status)
	}
	for _, want := range wants {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("err = %q, want it to name %q", err, want)
		}
	}
}

func findAgent(t *testing.T, list []AgentChild, id string) AgentChild {
	t.Helper()
	for _, c := range list {
		if c.Address == AgentAddress(id) {
			return c
		}
	}
	t.Fatalf("%s is not in %+v", id, list)
	return AgentChild{}
}

func instructionsOf(t *testing.T, spec StartSpec) string {
	t.Helper()
	if spec.Instructions == "" {
		t.Fatal("the start got no instruction file")
	}
	raw, err := os.ReadFile(spec.Instructions)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// wakeRefusals records Options.WakeRefused calls.
type wakeRefusals struct {
	mu  sync.Mutex
	got [][3]string
}

func (w *wakeRefusals) add(from, id, reason string) {
	w.mu.Lock()
	w.got = append(w.got, [3]string{from, id, reason})
	w.mu.Unlock()
}

func (w *wakeRefusals) list() [][3]string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return slices.Clone(w.got)
}

// testClock is a clock a test moves.
type testClock struct {
	mu  sync.Mutex
	now time.Time
}

func newTestClock() *testClock { return &testClock{now: time.Now()} }

func (c *testClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *testClock) advance(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	c.mu.Unlock()
}

func gone(h *harness, id string) bool {
	_, err := h.runner.Get(id)
	_, statErr := os.Stat(wsdashboard.SessionDir(h.dir, id))
	return err != nil && os.IsNotExist(statErr)
}

const day = 24 * time.Hour

func TestAgentSpawnAndListDescribeTheChild(t *testing.T) {
	h, got := agentHarness(t, nil)
	lead := h.leadCaller()
	res, err := h.runner.AgentSpawn(lead, AgentSpawnRequest{Prompt: "investigate the cache\nin detail", Tier: "medium", Authority: "workdir-write", Label: "probe"})
	if err != nil {
		t.Fatal(err)
	}
	a := agentID(res.Address)
	if res.Address != "id:"+a || !wsdashboard.ValidReplyID(a) || (res.Status != AgentStarting && res.Status != AgentRunning) {
		t.Fatalf("spawn answer = %+v", res)
	}
	ap := h.backend.waitProc(t, 0)
	if text := ap.nextSent(t); !strings.Contains(text, "investigate the cache") {
		t.Fatalf("the first turn = %q", text)
	}
	b, _ := h.agentRunning(lead, AgentSpawnRequest{Prompt: "check pi\nmore", Tier: "pi.small", Authority: "read-only"})
	if st := h.state(a); st.Backend != BackendClaude || st.Profile != AgentWorkerProfile(BackendClaude) || st.Parent == nil || *st.Parent != leadParent || st.Tier != "medium" {
		t.Fatalf("a's state = %+v", st)
	}
	if st := h.state(b); st.Backend != BackendPi || st.Profile != AgentWorkerProfile(BackendPi) {
		t.Fatalf("b's state = %+v", st)
	}

	list := h.runner.AgentList(lead)
	if len(list) != 2 {
		t.Fatalf("list = %+v", list)
	}
	ca, cb := findAgent(t, list, a), findAgent(t, list, b)
	if ca.Label != "probe" || ca.Tier != "claude.medium" || ca.Authority != PermissionWorkdirWrite || ca.Model != "claude-medium" || ca.Effort != "high" ||
		ca.Cwd != h.root || ca.CreatedAt == "" || ca.LastReport != "" {
		t.Fatalf("a's entry = %+v", ca)
	}
	// An omitted label is the prompt's first line.
	if cb.Label != "check pi" || cb.Tier != "pi.small" || cb.Authority != PermissionReadOnly || cb.Model != "prov/small-model" || cb.Effort != "low" {
		t.Fatalf("b's entry = %+v", cb)
	}
	eventually(t, "a running", func() bool { return findAgent(t, h.runner.AgentList(lead), a).State == AgentRunning })

	ap.reply("found the leak\nlong details")
	eventually(t, "a's report", func() bool {
		c := findAgent(t, h.runner.AgentList(lead), a)
		return c.LastReport == "done: found the leak" && c.State == AgentIdle
	})
	if reps := got.lead(); len(reps) != 1 || reps[0].Result.ID != a {
		t.Fatalf("the lead's reports = %+v", reps)
	}
	// No auto-close: the finished child stays open.
	stillOpen(t, h, a, "a finished agent.spawn child")
	if other := h.runner.AgentList(LeadCaller("otherleadreplyid", h.root, BackendClaude, h.config.Config)); len(other) != 0 {
		t.Fatalf("another lead lists %+v", other)
	}
}

func TestAgentInterruptKeepsTheChild(t *testing.T) {
	h, _ := agentHarness(t, nil)
	lead := h.leadCaller()
	a, ap := h.agentRunning(lead, AgentSpawnRequest{Prompt: "long job", Tier: "small", Authority: "read-only"})
	if _, err := h.runner.Send(a, "queued follow-up"); err != nil {
		t.Fatal(err)
	}
	eventually(t, "the follow-up held", func() bool { return h.state(a).Held > 0 })
	res, err := h.runner.AgentInterrupt(lead, AgentAddress(a))
	if err != nil {
		t.Fatal(err)
	}
	if res.Address != AgentAddress(a) || len(res.Returned) != 1 || !strings.Contains(res.Returned[0], "queued follow-up") || res.Status == AgentClosed {
		t.Fatalf("interrupt = %+v", res)
	}
	eventually(t, "the turn interrupted", func() bool {
		ap.mu.Lock()
		defer ap.mu.Unlock()
		return ap.interrupts == 1
	})
	ap.events <- Event{TurnEnd: true, TurnError: "interrupted"}
	stillOpen(t, h, a, "an interrupted child")
	// Nothing left to hand back: an empty list, not null.
	if res, err := h.runner.AgentInterrupt(lead, AgentAddress(a)); err != nil || res.Returned == nil || len(res.Returned) != 0 {
		t.Fatalf("a second interrupt = %+v, %v", res, err)
	}
	if _, err := h.runner.Deliver(a, Delivery{Text: "go on"}); err != nil {
		t.Fatal(err)
	}
	if text := ap.nextSent(t); !strings.Contains(text, "go on") {
		t.Fatalf("the next turn = %q", text)
	}
}

// Ownership: agent.* control reaches the caller's direct children alone.
func TestAgentControlReachesOwnChildrenAlone(t *testing.T) {
	h, _ := agentHarness(t, nil)
	lead := h.leadCaller()
	a, _ := h.agentRunning(lead, AgentSpawnRequest{Prompt: "parent", Tier: "medium", Authority: "workdir-write"})
	tokenA := h.token(0)
	b, _ := h.agentRunning(h.holderOf(a), AgentSpawnRequest{Prompt: "child", Tier: "small", Authority: "workdir-write"})
	tokenB := h.token(1)
	other := LeadCaller("otherleadreplyid", h.root, BackendClaude, h.config.Config)

	for name, tc := range map[string]struct {
		caller AgentCaller
		id     string
	}{
		"another lead's child": {other, a},
		"a grandchild":         {lead, b},
		"the caller's parent":  {h.holderOf(b), a},
		"the caller itself":    {h.holderOf(a), a},
		"no such agent":        {lead, "nosuchagent"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := h.runner.AgentInterrupt(tc.caller, AgentAddress(tc.id))
			agentRefused(t, err, http.StatusNotFound, "is not an agent this session spawned")
		})
	}
	if list := h.runner.AgentList(lead); len(list) != 1 || list[0].Address != AgentAddress(a) {
		t.Fatalf("the lead lists %+v", list)
	}
	if _, err := h.runner.AgentInterrupt(h.holderOf(a), AgentAddress(b)); err != nil {
		t.Fatalf("a interrupts its own child: %v", err)
	}

	// The same through the token route.
	if code, body := h.tokenCall(tokenA, "agent.interrupt", map[string]any{"address": AgentAddress(b)}); code != http.StatusOK {
		t.Fatalf("a's token interrupts b: %d %s", code, body)
	}
	if code, body := h.tokenCall(tokenB, "agent.interrupt", map[string]any{"address": AgentAddress(a)}); code != http.StatusNotFound || !strings.Contains(body, "is not an agent this session spawned") {
		t.Fatalf("b's token interrupts its parent: %d %s", code, body)
	}
	var listed struct {
		Agents []AgentChild `json:"agents"`
	}
	if code, body := h.tokenCall(tokenA, "agent.list", nil); code != http.StatusOK || json.Unmarshal([]byte(body), &listed) != nil || len(listed.Agents) != 1 || listed.Agents[0].Address != AgentAddress(b) {
		t.Fatalf("a's token lists = %d %s", code, body)
	}
	if code, body := h.tokenCall(tokenB, "agent.list", nil); code != http.StatusOK || json.Unmarshal([]byte(body), &listed) != nil || len(listed.Agents) != 0 {
		t.Fatalf("b's token lists = %d %s", code, body)
	}
}

func TestAgentChildSleepsOnItsIdleTimerAndADeliveryWakesIt(t *testing.T) {
	refusals := &wakeRefusals{}
	h, got := agentHarness(t, func(o *Options) { o.WakeRefused = refusals.add })
	h.shortIdle(AgentWorkerProfile(BackendClaude))
	lead := h.leadCaller()
	a, ap := h.agentRunning(lead, AgentSpawnRequest{Prompt: "work", Tier: "small", Authority: "workdir-write"})
	ap.reply("done")
	eventually(t, "the report", func() bool { return len(got.lead()) == 1 })
	eventually(t, "the idle process to stop", ap.isClosed)
	eventuallyAsleep(t, h, a)
	if st := findAgent(t, h.runner.AgentList(lead), a).State; st != AgentWaiting {
		t.Fatalf("a sleeping child's state = %q", st)
	}
	st, err := h.runner.Deliver(a, Delivery{From: "senderreplyid", Text: "wake up"})
	if err != nil || st.ID != a {
		t.Fatalf("Deliver = %+v, %v", st, err)
	}
	p := h.backend.waitProc(t, 1)
	if text := p.nextSent(t); !strings.Contains(text, "wake up") {
		t.Fatalf("the woken turn = %q", text)
	}
	if st := h.state(a); st.Lifecycle == wsdashboard.SessionChildWaiting || !st.Process {
		t.Fatalf("the woken child = %+v", st)
	}
	if got := refusals.list(); len(got) != 0 {
		t.Fatalf("an admitted wake told the sender: %v", got)
	}
}

// A wake whose admission the parent's current level refuses keeps the mail
// queued and the child asleep, and tells the sender once; once the parent's
// level allows it again, the next delivery wakes the child with both.
func TestRefusedWakeKeepsTheMailAndTellsTheSenderOnce(t *testing.T) {
	refusals := &wakeRefusals{}
	h, _ := agentHarness(t, func(o *Options) { o.WakeRefused = refusals.add })
	a, b := settledPair(t, h)
	parentOfB := ParentRef{Kind: ParentConversation, ReplyID: a}
	if err := writePermissionSelection(wsdashboard.SessionDir(h.dir, a), PermissionReadOnly); err != nil {
		t.Fatal(err)
	}
	starts := h.backend.startCount()
	if _, err := h.runner.Deliver(b, Delivery{From: "senderreplyid", Text: "first message"}); err != nil {
		t.Fatalf("a refused wake's delivery = %v", err)
	}
	eventually(t, "the refused start", func() bool {
		return strings.Contains(h.state(b).Error, "authority workdir-write is above the parent's current level read-only")
	})
	time.Sleep(100 * time.Millisecond)
	st := h.state(b)
	if st.Held == 0 || st.Process || st.Lifecycle != wsdashboard.SessionChildWaiting || AgentStatus(st) != AgentWaiting {
		t.Fatalf("the refused child = %+v", st)
	}
	if n := h.backend.startCount(); n != starts {
		t.Fatalf("a refused wake started %d processes", n-starts)
	}
	if n := h.runner.awakeChildren(parentOfB); n != 0 {
		t.Fatalf("a refused child counts as awake: %d", n)
	}
	if got := refusals.list(); len(got) != 1 || got[0][0] != "senderreplyid" || got[0][1] != b || !strings.Contains(got[0][2], "read-only") {
		t.Fatalf("the sender's notices = %v", got)
	}

	if err := writePermissionSelection(wsdashboard.SessionDir(h.dir, a), PermissionWorkdirWrite); err != nil {
		t.Fatal(err)
	}
	if _, err := h.runner.Deliver(b, Delivery{From: "senderreplyid", Text: "second message"}); err != nil {
		t.Fatal(err)
	}
	p := h.backend.waitProc(t, starts)
	text := p.nextSent(t)
	if !strings.Contains(text, "first message") {
		t.Fatalf("the woken turn lost the held mail: %q", text)
	}
	if !strings.Contains(text, "second message") {
		p.reply("first done")
		if next := p.nextSent(t); !strings.Contains(next, "second message") {
			t.Fatalf("the second message's turn = %q", next)
		}
	}
	if got := refusals.list(); len(got) != 1 {
		t.Fatalf("an admitted wake told the sender: %v", got)
	}
}

func TestAgentSpawnDepthCapThroughTheTokenRoute(t *testing.T) {
	h, _ := agentHarness(t, nil)
	a, _ := h.agentRunning(h.leadCaller(), AgentSpawnRequest{Prompt: "depth 1", Tier: "medium", Authority: "workdir-write"})
	token := h.token(0)
	if code, body := h.tokenCall(token, "agent.tiers", nil); code != http.StatusOK || !strings.Contains(body, `"harness":"claude"`) {
		t.Fatalf("agent.tiers by token = %d %s", code, body)
	}
	last := a
	for depth := 2; depth <= MaxSpawnDepth; depth++ {
		n := h.backend.startCount()
		code, body := h.tokenCall(token, "agent.spawn", map[string]any{"prompt": fmt.Sprintf("depth %d", depth), "tier": "small", "authority": "workdir-write"})
		var res AgentSpawnResult
		if code != http.StatusCreated || json.Unmarshal([]byte(body), &res) != nil {
			t.Fatalf("depth-%d spawn = %d %s", depth, code, body)
		}
		id := agentID(res.Address)
		if st := h.state(id); st.Parent == nil || *st.Parent != (ParentRef{Kind: ParentConversation, ReplyID: last}) {
			t.Fatalf("depth-%d child's parent = %+v", depth, st.Parent)
		}
		h.backend.waitProc(t, n).nextSent(t)
		if token = h.token(n); token == "" {
			t.Fatalf("a depth-%d child holds no token", depth)
		}
		last = id
	}
	code, body := h.tokenCall(token, "agent.spawn", map[string]any{"prompt": "too deep", "tier": "small", "authority": "read-only"})
	if code != http.StatusForbidden || !strings.Contains(body, "agent.spawn: the spawn depth cap is 4") || !strings.Contains(body, "at depth 4") {
		t.Fatalf("a depth-5 spawn = %d %s", code, body)
	}
	if n := len(h.runner.Children(ParentRef{Kind: ParentConversation, ReplyID: last})); n != 0 {
		t.Fatalf("a refused spawn spawned %d", n)
	}
}

// The width cap counts awake direct children: the 100th awake one is
// refused, a sleeping one does not count, and a delivery wakes a sleeping
// child past the cap.
func TestAgentSpawnWidthCapCountsAwakeChildren(t *testing.T) {
	h, _ := agentHarness(t, nil)
	lead := h.leadCaller()
	var ids []string
	for i := range MaxSpawnWidth {
		ids = append(ids, h.agentSpawn(lead, AgentSpawnRequest{Prompt: fmt.Sprintf("job %d", i), Tier: "small", Authority: "read-only"}))
	}
	eventually(t, "every child's process", func() bool { return h.backend.startCount() == MaxSpawnWidth })
	if n := h.runner.awakeChildren(leadParent); n != MaxSpawnWidth {
		t.Fatalf("awake children = %d", n)
	}
	_, err := h.runner.AgentSpawn(lead, AgentSpawnRequest{Prompt: "one too many", Tier: "small", Authority: "read-only"})
	agentRefused(t, err, http.StatusForbidden, "the spawn width cap is 99 awake direct children")
	if n := len(h.runner.AgentList(lead)); n != MaxSpawnWidth {
		t.Fatalf("a refused spawn spawned: %d children", n)
	}

	sleeper := ids[0]
	h.asleep(sleeper)
	eventually(t, "the sleeper not counted", func() bool { return h.runner.awakeChildren(leadParent) == MaxSpawnWidth-1 })
	h.agentSpawn(lead, AgentSpawnRequest{Prompt: "the hundredth", Tier: "small", Authority: "read-only"})
	eventually(t, "the parent at the cap", func() bool { return h.runner.awakeChildren(leadParent) == MaxSpawnWidth })
	starts := h.backend.startCount()
	if _, err := h.runner.Deliver(sleeper, Delivery{From: "senderreplyid", Text: "wake past the cap"}); err != nil {
		t.Fatalf("a wake at the cap = %v", err)
	}
	eventually(t, "the sleeper's process", func() bool { return h.state(sleeper).Process })
	if h.backend.startCount() <= starts {
		t.Fatal("the wake started no process")
	}
	if n := h.runner.awakeChildren(leadParent); n != MaxSpawnWidth+1 {
		t.Fatalf("awake children after the wake = %d", n)
	}
}

func setTierAliases(t *testing.T, cfg wsconfig.Options, value string) {
	t.Helper()
	resolver := wsconfig.NewResolver(cfg, nil, nil, nil)
	if err := resolver.Set(wsconfig.ItemAgentTierAliases, value, wsconfig.SetOptions{ExplicitScope: wsconfig.ScopeGlobal}); err != nil {
		t.Fatal(err)
	}
}

func TestAgentSpawnResolvesTiers(t *testing.T) {
	h, _ := agentHarness(t, func(o *Options) {
		o.Backends["codex"] = BackendRegistration{Backend: o.Backends[BackendClaude].Backend}
	})
	setTierAliases(t, h.config.Config, "quick=pi.small big=pi.xlarge far=agy.large")
	lead := h.leadCaller()
	for _, tc := range []struct{ tier, backend, model string }{
		{"medium", BackendClaude, "claude-medium"},
		{"pi.small", BackendPi, "prov/small-model"},
		{"CLAUDE.Large", BackendClaude, "claude-large"},
		{"quick", BackendPi, "prov/small-model"},
	} {
		id := h.agentSpawn(lead, AgentSpawnRequest{Prompt: "x", Tier: tc.tier, Authority: "read-only"})
		if st := h.state(id); st.Backend != tc.backend || st.Model != tc.model {
			t.Fatalf("tier %q spawned %+v", tc.tier, st)
		}
	}
	spawned := len(h.runner.AgentList(lead))
	codex := LeadCaller(leadParent.ReplyID, h.root, "codex", h.config.Config)
	for _, tc := range []struct {
		caller AgentCaller
		tier   string
		why    string
	}{
		{lead, "", "tier is required"},
		{lead, "huge", `tier "huge" is neither a tier, <harness>.<tier>, nor an alias`},
		{lead, "pi.xlarge", "pi.xlarge has no model in agents.tier"},
		{lead, "pi.huge", "names no tier"},
		{lead, "agy.large", `harness "agy" is not registered in this daemon`},
		{lead, "big", "pi.xlarge has no model"},
		{lead, "far", `harness "agy" is not registered`},
		{codex, "small", `harness "codex" is registered for hosted leads only`},
		{lead, "codex.small", `harness "codex" is registered for hosted leads only`},
	} {
		_, err := h.runner.AgentSpawn(tc.caller, AgentSpawnRequest{Prompt: "x", Tier: tc.tier, Authority: "read-only"})
		agentRefused(t, err, http.StatusBadRequest, tc.why, "valid tiers: ")
		msg := err.Error()
		for _, want := range []string{"claude.small", "claude.large", "pi.small", "pi.large", "quick"} {
			if !strings.Contains(msg, want) {
				t.Fatalf("tier %q: the valid list lacks %s: %s", tc.tier, want, msg)
			}
		}
		if strings.Contains(msg, "big") && tc.tier != "big" || strings.Contains(msg, "far") && tc.tier != "far" || strings.Contains(msg, "codex.") && !strings.Contains(tc.why, "codex") {
			t.Fatalf("tier %q: the valid list names an unusable tier: %s", tc.tier, msg)
		}
		native := strings.Contains(msg, "valid tiers: small, medium, large, xlarge, claude.small")
		if native != (tc.caller.Harness == BackendClaude) {
			t.Fatalf("tier %q: bare tiers listed for harness %s: %s", tc.tier, tc.caller.Harness, msg)
		}
	}
	if n := len(h.runner.AgentList(lead)); n != spawned {
		t.Fatalf("a refused tier spawned: %d children, was %d", n, spawned)
	}
}

func TestAgentTiersListsNativeExtAndAliases(t *testing.T) {
	h, _ := agentHarness(t, nil)
	setTierAliases(t, h.config.Config, "quick=pi.small big=pi.xlarge far=agy.large")
	got, err := h.runner.AgentTiers(h.leadCaller())
	if err != nil {
		t.Fatal(err)
	}
	tierNames := func(es []AgentTierEntry) []string {
		var out []string
		for _, e := range es {
			out = append(out, e.Tier)
		}
		return out
	}
	// claude's xlarge is the builtin floor's; the test set the others.
	if got.Harness != BackendClaude || !slices.Equal(tierNames(got.Native), []string{"small", "medium", "large", "xlarge"}) ||
		got.Native[1] != (AgentTierEntry{Tier: "medium", Model: "claude-medium", Effort: "high"}) {
		t.Fatalf("native = %+v", got)
	}
	if !slices.Equal(tierNames(got.Ext), []string{"claude.small", "claude.medium", "claude.large", "claude.xlarge", "pi.small", "pi.medium", "pi.large"}) ||
		got.Ext[4] != (AgentTierEntry{Tier: "pi.small", Model: "prov/small-model", Effort: "low"}) {
		t.Fatalf("ext = %+v", got.Ext)
	}
	wantAliases := []AgentAliasEntry{
		{Alias: "big", Target: "pi.xlarge", Error: `tier "xlarge" is not set for pi`},
		{Alias: "far", Target: "agy.large", Error: `harness "agy" has no spawnable tier now`},
		{Alias: "quick", Target: "pi.small", Model: "prov/small-model", Effort: "low"},
	}
	if !slices.Equal(got.Aliases, wantAliases) || len(got.Unavailable) != 0 {
		t.Fatalf("aliases = %+v, unavailable = %v", got.Aliases, got.Unavailable)
	}
	// On a Pi caller the bare tiers are Pi's.
	pi, err := h.runner.AgentTiers(LeadCaller(leadParent.ReplyID, h.root, BackendPi, h.config.Config))
	if err != nil || !slices.Equal(tierNames(pi.Native), []string{"small", "medium", "large"}) || pi.Native[0].Model != "prov/small-model" {
		t.Fatalf("a pi caller's tiers = %+v, %v", pi, err)
	}

	// A worker harness that cannot spawn now is named, with why, and its
	// tiers leave the lists.
	h.config.ChildCheck = func() error { return errors.New("pi helper missing") }
	h.runner.Close()
	h.start()
	got, err = h.runner.AgentTiers(h.leadCaller())
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got.Unavailable, []string{"pi: pi helper missing"}) || !slices.Equal(tierNames(got.Ext), []string{"claude.small", "claude.medium", "claude.large", "claude.xlarge"}) ||
		got.Aliases[2].Error == "" {
		t.Fatalf("tiers with pi unavailable = %+v", got)
	}
	_, err = h.runner.AgentSpawn(h.leadCaller(), AgentSpawnRequest{Prompt: "x", Tier: "pi.small", Authority: "read-only"})
	agentRefused(t, err, http.StatusBadRequest, "pi helper missing", "valid tiers: small, medium, large, xlarge, claude.small, claude.medium, claude.large, claude.xlarge")
	if strings.Contains(err.Error(), "pi.small,") || strings.Contains(err.Error(), "quick") {
		t.Fatalf("an unavailable harness's tiers are listed: %v", err)
	}

	// No worker harness installed: nothing is spawnable.
	setLookPath(t, func(file string) (string, error) { return "", errors.New("not found") })
	got, err = h.runner.AgentTiers(h.leadCaller())
	if err != nil || len(got.Native)+len(got.Ext) != 0 || len(got.Unavailable) != 2 || !strings.Contains(got.Unavailable[0], "fake is not installed") {
		t.Fatalf("tiers with nothing installed = %+v, %v", got, err)
	}
	_, err = h.runner.AgentSpawn(h.leadCaller(), AgentSpawnRequest{Prompt: "x", Tier: "small", Authority: "read-only"})
	agentRefused(t, err, http.StatusBadRequest, "no tier is spawnable now")
}

func TestAgentSystemPromptReachesEveryStart(t *testing.T) {
	h, _ := agentHarness(t, nil)
	lead := h.leadCaller()
	a, ap := h.agentRunning(lead, AgentSpawnRequest{Prompt: "go", Tier: "small", Authority: "read-only", SystemPrompt: "You are the cache probe."})
	if text := instructionsOf(t, h.backend.start(0)); !strings.Contains(text, "You are the cache probe.") {
		t.Fatalf("the first start's instructions = %q", text)
	}
	if raw, err := os.ReadFile(filepath.Join(wsdashboard.SessionDir(h.dir, a), systemPromptFile)); err != nil || string(raw) != "You are the cache probe." {
		t.Fatalf("the session copy = %q, %v", raw, err)
	}
	if rec := h.record(a); !rec.SystemPrompt {
		t.Fatalf("the record does not mark the system prompt: %+v", rec)
	}
	ap.reply("done")
	eventually(t, "a's turn settled", func() bool { return h.state(a).Status == StatusIdle })
	h.asleep(a)
	// A new segment after sleep re-injects it.
	if _, err := h.runner.Deliver(a, Delivery{Text: "again"}); err != nil {
		t.Fatal(err)
	}
	h.backend.waitProc(t, 1).nextSent(t)
	if text := instructionsOf(t, h.backend.start(1)); !strings.Contains(text, "You are the cache probe.") {
		t.Fatalf("the start after sleep = %q", text)
	}
	// After a daemon restart it is read from the session directory.
	h.runner.Close()
	h.start()
	if _, err := h.runner.Deliver(a, Delivery{Text: "after the restart"}); err != nil {
		t.Fatal(err)
	}
	p := h.backend.waitProc(t, 2)
	if text := p.nextSent(t); !strings.Contains(text, "after the restart") {
		t.Fatalf("the turn after the restart = %q", text)
	}
	if text := instructionsOf(t, h.backend.start(2)); !strings.Contains(text, "You are the cache probe.") {
		t.Fatalf("the start after the restart = %q", text)
	}

	// system_prompt_path is read once, at spawn, relative to the caller's
	// directory: deleting the source does not matter.
	src := filepath.Join(h.root, "prompts", "probe.md")
	if err := os.MkdirAll(filepath.Dir(src), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(src, []byte("Path-sourced prompt."), 0o644); err != nil {
		t.Fatal(err)
	}
	n := h.backend.startCount()
	h.agentSpawn(lead, AgentSpawnRequest{Prompt: "go", Tier: "small", Authority: "read-only", SystemPromptPath: "prompts/probe.md"})
	if err := os.Remove(src); err != nil {
		t.Fatal(err)
	}
	h.backend.waitProc(t, n).nextSent(t)
	if text := instructionsOf(t, h.backend.start(n)); !strings.Contains(text, "Path-sourced prompt.") {
		t.Fatalf("the path prompt's start = %q", text)
	}

	_, err := h.runner.AgentSpawn(lead, AgentSpawnRequest{Prompt: "x", Tier: "small", Authority: "read-only", SystemPrompt: "a", SystemPromptPath: "b"})
	agentRefused(t, err, http.StatusBadRequest, "not both")
	_, err = h.runner.AgentSpawn(lead, AgentSpawnRequest{Prompt: "x", Tier: "small", Authority: "read-only", SystemPromptPath: "prompts/probe.md"})
	agentRefused(t, err, http.StatusBadRequest, "system_prompt_path")
}

func TestAgentSpawnCapsAuthorityAndTier(t *testing.T) {
	h, _ := agentHarness(t, nil)
	lead := h.leadCaller()
	_, err := h.runner.AgentSpawn(lead, AgentSpawnRequest{Prompt: "x", Tier: "small"})
	agentRefused(t, err, http.StatusBadRequest, "authority is required")
	_, err = h.runner.AgentSpawn(lead, AgentSpawnRequest{Prompt: "x", Tier: "small", Authority: "admin"})
	agentRefused(t, err, http.StatusBadRequest, "authority must be read-only, workdir-write, or full")

	// A lead parent with no Runner record caps at workdir-write.
	_, err = h.runner.AgentSpawn(lead, AgentSpawnRequest{Prompt: "x", Tier: "small", Authority: "full"})
	agentRefused(t, err, http.StatusForbidden, "authority full is above this session's own level workdir-write")
	w, _ := h.agentRunning(lead, AgentSpawnRequest{Prompt: "writer", Tier: "medium", Authority: "workdir-write"})
	r, _ := h.agentRunning(lead, AgentSpawnRequest{Prompt: "reader", Tier: "small", Authority: "read-only"})
	if lvl := childLevel(wsdashboard.SessionDir(h.dir, w)); lvl != PermissionWorkdirWrite {
		t.Fatalf("the child's stored level = %q", lvl)
	}

	// A conversation parent caps at its own explicit level: at it is
	// admitted, above it refused.
	_, err = h.runner.AgentSpawn(h.holderOf(w), AgentSpawnRequest{Prompt: "x", Tier: "small", Authority: "full"})
	agentRefused(t, err, http.StatusForbidden, "above this session's own level workdir-write")
	h.agentSpawn(h.holderOf(w), AgentSpawnRequest{Prompt: "x", Tier: "small", Authority: "workdir-write"})
	_, err = h.runner.AgentSpawn(h.holderOf(r), AgentSpawnRequest{Prompt: "x", Tier: "small", Authority: "workdir-write"})
	agentRefused(t, err, http.StatusForbidden, "above this session's own level read-only")
	h.agentSpawn(h.holderOf(r), AgentSpawnRequest{Prompt: "x", Tier: "small", Authority: "read-only"})

	// A nested spawn's tier may not pass its holder's.
	_, err = h.runner.AgentSpawn(h.holderOf(r), AgentSpawnRequest{Prompt: "x", Tier: "medium", Authority: "read-only"})
	agentRefused(t, err, http.StatusForbidden, `tier "medium" is above this conversation's own tier "small"`)
	_, err = h.runner.AgentSpawn(h.holderOf(w), AgentSpawnRequest{Prompt: "x", Tier: "pi.large", Authority: "read-only"})
	agentRefused(t, err, http.StatusForbidden, `tier "large" is above this conversation's own tier "medium"`)

	// A conversation parent with no record has no level: admission fails.
	ghost := AgentCaller{Parent: ParentRef{Kind: ParentConversation, ReplyID: "nosuchconversation"}, Root: h.root, Harness: BackendClaude, Config: h.config.Config}
	_, err = h.runner.AgentSpawn(ghost, AgentSpawnRequest{Prompt: "x", Tier: "small", Authority: "read-only"})
	agentRefused(t, err, http.StatusForbidden, "permission admission", "has no record")
}

func TestAgentChildGetsATokenAndItsAuthority(t *testing.T) {
	h, _ := agentHarness(t, nil)
	lead := h.leadCaller()
	for i, tc := range []struct {
		tier    string
		level   PermissionLevel
		backend string
	}{
		{"small", PermissionWorkdirWrite, BackendClaude},
		{"pi.small", PermissionReadOnly, BackendPi},
	} {
		h.agentRunning(lead, AgentSpawnRequest{Prompt: "x", Tier: tc.tier, Authority: string(tc.level)})
		spec := h.backend.start(i)
		if spec.WSMCP == nil {
			t.Fatalf("%s: no ws-mcp", tc.tier)
		}
		env := spec.WSMCP.Env
		tools := strings.Split(env[EnvAllowedTools], ",")
		if len(env[EnvPiToken]) != 64 || env[EnvPiEndpoint] != h.endpoint || env[EnvPermissionLevel] != string(tc.level) ||
			!slices.Contains(tools, "agent.spawn") || !slices.Contains(tools, "mailbox.send") || slices.Contains(tools, "pi.spawn") || spec.Profile.Backend != tc.backend {
			t.Fatalf("%s: the child's start = %+v, env %v", tc.tier, spec, env)
		}
		if code, body := h.tokenCall(env[EnvPiToken], "agent.list", nil); code != http.StatusOK {
			t.Fatalf("%s: the child's token lists = %d %s", tc.tier, code, body)
		}
	}
}

// The child's explicit level survives a daemon restart, and its next start
// is admitted and carries it again.
func TestAgentChildLevelSurvivesARestart(t *testing.T) {
	h, _ := agentHarness(t, nil)
	lead := h.leadCaller()
	r, rp := h.agentRunning(lead, AgentSpawnRequest{Prompt: "x", Tier: "small", Authority: "read-only"})
	rp.reply("done")
	eventually(t, "the turn settled", func() bool { return h.state(r).Status == StatusIdle })
	h.runner.Close()
	h.start()
	if c := findAgent(t, h.runner.AgentList(lead), r); c.Authority != PermissionReadOnly {
		t.Fatalf("after the restart = %+v", c)
	}
	if _, err := h.runner.Deliver(r, Delivery{Text: "again"}); err != nil {
		t.Fatal(err)
	}
	h.backend.waitProc(t, 1).nextSent(t)
	if lvl := h.backend.start(1).WSMCP.Env[EnvPermissionLevel]; lvl != string(PermissionReadOnly) {
		t.Fatalf("the restarted child's level = %q", lvl)
	}
}

// A child whose conversation parent's record is gone after a restart is
// refused at its next start: no guessed ceiling.
func TestAgentChildOfAGoneParentIsRefusedAtItsNextStart(t *testing.T) {
	refusals := &wakeRefusals{}
	h, _ := agentHarness(t, func(o *Options) { o.WakeRefused = refusals.add })
	a, b := settledPair(t, h)
	h.runner.Close()
	if err := os.RemoveAll(wsdashboard.SessionDir(h.dir, a)); err != nil {
		t.Fatal(err)
	}
	h.start()
	starts := h.backend.startCount()
	if _, err := h.runner.Deliver(b, Delivery{From: "senderreplyid", Text: "wake"}); err != nil {
		t.Fatal(err)
	}
	eventually(t, "the refused start", func() bool { return strings.Contains(h.state(b).Error, "has no record") })
	time.Sleep(60 * time.Millisecond)
	if n := h.backend.startCount(); n != starts {
		t.Fatalf("a refused start launched %d", n-starts)
	}
	if got := refusals.list(); len(got) != 1 || !strings.Contains(got[0][2], "has no record") {
		t.Fatalf("the sender's notices = %v", got)
	}
}

// expiryHarness is agentHarness on a clock the test moves, with a two-day
// child-agent-waiting expiry.
func expiryHarness(t *testing.T) (*harness, *testClock) {
	t.Helper()
	clk := newTestClock()
	h, _ := agentHarness(t, func(o *Options) {
		o.Now = clk.Now
		o.ChildWaitingDays = 2
	})
	return h, clk
}

// card is a conversation opened as a whiteboard agent card is: no spawn
// parent (ParentRef), settled and asleep.
func (h *harness) card() string {
	h.t.Helper()
	n := h.backend.startCount()
	id := h.create("chat").ID
	if _, err := h.runner.Send(id, "card work"); err != nil {
		h.t.Fatal(err)
	}
	p := h.backend.waitProc(h.t, n)
	p.nextSent(h.t)
	p.reply("card done")
	eventually(h.t, "the card's turn settled", func() bool { return h.state(id).Status == StatusIdle })
	if _, err := h.runner.Stop(id); err != nil {
		h.t.Fatal(err)
	}
	eventually(h.t, "the card stopped", func() bool { return !h.state(id).Process })
	return id
}

func TestChildWaitingPastItsExpiryIsDeletedDeepestFirst(t *testing.T) {
	h, clk := expiryHarness(t)
	a, b := settledPair(t, h)
	card := h.card()
	if st := h.state(card); st.Lifecycle == wsdashboard.SessionChildWaiting {
		t.Fatalf("a card is no waiting child: %+v", st)
	}
	// b was active after a, so a expires first and takes b with it.
	h.runner.conv(b).update(func(rec *Record) { rec.LastActiveAt = clk.Now().Add(day).UTC().Format(time.RFC3339) })
	var mu sync.Mutex
	var order []string
	stop := h.runner.Watch(func(st State) {
		mu.Lock()
		defer mu.Unlock()
		if st.Status == StatusClosed && !slices.Contains(order, st.ID) {
			order = append(order, st.ID)
		}
	})
	defer stop()

	clk.advance(day + 12*time.Hour)
	time.Sleep(200 * time.Millisecond)
	if gone(h, a) || gone(h, b) {
		t.Fatal("a child was deleted before its expiry")
	}
	clk.advance(day)
	eventually(t, "the expired subtree deleted", func() bool { return gone(h, a) && gone(h, b) })
	mu.Lock()
	closed := slices.Clone(order)
	mu.Unlock()
	if !slices.Equal(closed, []string{b, a}) {
		t.Fatalf("close order = %v, want the child first", closed)
	}
	if len(h.runner.AgentList(h.leadCaller())) != 0 {
		t.Fatal("the lead still lists a deleted child")
	}
	// A card is never expired or cascaded.
	clk.advance(30 * day)
	time.Sleep(200 * time.Millisecond)
	if st := h.state(card); st.Status == StatusClosed {
		t.Fatalf("the card = %+v", st)
	}
	if _, err := os.Stat(wsdashboard.SessionDir(h.dir, card)); err != nil {
		t.Fatalf("the card's directory: %v", err)
	}
}

func TestChildWaitingExpiryRestartsOnActivity(t *testing.T) {
	h, clk := expiryHarness(t)
	a, ap := h.agentRunning(h.leadCaller(), AgentSpawnRequest{Prompt: "x", Tier: "small", Authority: "read-only"})
	ap.reply("done")
	eventually(t, "the turn settled", func() bool { return h.state(a).Status == StatusIdle })
	h.asleep(a)
	clk.advance(day + 12*time.Hour)
	if _, err := h.runner.Deliver(a, Delivery{Text: "more"}); err != nil {
		t.Fatal(err)
	}
	if rec := h.record(a); rec.LastActiveAt != clk.Now().UTC().Format(time.RFC3339) {
		t.Fatalf("a send's LastActiveAt = %q", rec.LastActiveAt)
	}
	p := h.backend.waitProc(t, 1)
	p.nextSent(t)
	p.reply("more done")
	eventually(t, "the second turn settled", func() bool { return h.state(a).Status == StatusIdle })
	h.asleep(a)
	// Two and a half days after the spawn, one after the last activity.
	clk.advance(day)
	time.Sleep(200 * time.Millisecond)
	if gone(h, a) {
		t.Fatal("a child active a day ago expired")
	}
	clk.advance(day + time.Hour)
	eventually(t, "the child expired", func() bool { return gone(h, a) })
}

// An expiring child with an awake descendant is kept.
func TestChildWaitingWithAnAwakeDescendantIsKept(t *testing.T) {
	h, clk := expiryHarness(t)
	a, ap := h.agentRunning(h.leadCaller(), AgentSpawnRequest{Prompt: "x", Tier: "medium", Authority: "workdir-write"})
	ap.reply("done")
	eventually(t, "the turn settled", func() bool { return h.state(a).Status == StatusIdle })
	h.asleep(a)
	b, _ := h.agentRunning(h.holderOf(a), AgentSpawnRequest{Prompt: "long", Tier: "small", Authority: "read-only"})
	// A sibling as old with no descendant shows the sweep ran.
	s, sp := h.agentRunning(h.leadCaller(), AgentSpawnRequest{Prompt: "x", Tier: "small", Authority: "read-only"})
	sp.reply("done")
	eventually(t, "the sibling's turn settled", func() bool { return h.state(s).Status == StatusIdle })
	h.asleep(s)
	clk.advance(3 * day)
	eventually(t, "the sibling expired", func() bool { return gone(h, s) })
	time.Sleep(200 * time.Millisecond)
	if gone(h, a) || gone(h, b) {
		t.Fatal("a waiting child with an awake descendant was deleted")
	}
	if st := h.state(a); st.Status == StatusClosed || st.Lifecycle != wsdashboard.SessionChildWaiting {
		t.Fatalf("the kept child = %+v", st)
	}
}

func TestDaemonStartDeletesAnExpiredSubtreeWithoutALoop(t *testing.T) {
	h, clk := expiryHarness(t)
	a, b := settledPair(t, h)
	card := h.card()
	young, yp := h.agentRunning(h.leadCaller(), AgentSpawnRequest{Prompt: "x", Tier: "small", Authority: "read-only"})
	yp.reply("done")
	eventually(t, "the young child's turn settled", func() bool { return h.state(young).Status == StatusIdle })
	h.runner.conv(young).update(func(rec *Record) { rec.LastActiveAt = clk.Now().Add(2 * day).UTC().Format(time.RFC3339) })
	h.runner.Close()
	clk.advance(3 * day)
	h.start()
	for _, id := range []string{a, b} {
		if h.runner.conv(id) != nil || !gone(h, id) {
			t.Fatalf("%s survived the start", id)
		}
	}
	for _, id := range []string{card, young} {
		if h.runner.conv(id) == nil {
			t.Fatalf("%s got no loop", id)
		}
		if st := h.state(id); st.Status == StatusClosed {
			t.Fatalf("%s = %+v", id, st)
		}
	}
}

// expiredAtStart picks each quiet subtree past its expiry, deepest first;
// a busy descendant keeps its ancestors, a recent or busy child is kept,
// and a conversation with no spawn parent is never picked.
func TestExpiredAtStartPicksQuietExpiredSubtrees(t *testing.T) {
	now := time.Now()
	r := NewRunner(Options{Dir: t.TempDir(), Now: func() time.Time { return now }, ChildWaitingDays: 2})
	old, recent := now.Add(-3*day).UTC().Format(time.RFC3339), now.Add(-day).UTC().Format(time.RFC3339)
	lead := func(id, at string) Record {
		return Record{ReplyID: id, Parent: &leadParent, CreatedAt: at}
	}
	child := func(id, parent, at string) Record {
		return Record{ReplyID: id, Parent: &ParentRef{Kind: ParentConversation, ReplyID: parent}, CreatedAt: at}
	}
	busy := child("busy", "c", old)
	busy.TurnOpen = true
	pending := lead("pending", old)
	pending.Sends = 1
	refreshed := lead("refreshed", old)
	refreshed.LastActiveAt = recent
	open := []Record{
		lead("a", old), child("b", "a", old),
		lead("c", old), busy, child("e", "busy", old),
		lead("fresh", recent), refreshed, pending,
		{ReplyID: "card", CreatedAt: old},
		{ReplyID: "leadconv", Lead: true, CreatedAt: old},
	}
	if got := r.expiredAtStart(open); !slices.Equal(got, []string{"b", "a", "e"}) {
		t.Fatalf("expiredAtStart = %v, want [b a e]", got)
	}
}

func TestChildWaitingExpiryReadsItsConfig(t *testing.T) {
	cfg := testConfig(t)
	if got := NewRunner(Options{Config: cfg}).childWaitingExpiry(); got != DefaultChildWaitingDays*day {
		t.Fatalf("the default expiry = %v", got)
	}
	resolver := wsconfig.NewResolver(cfg, nil, nil, nil)
	if err := resolver.Set(wsconfig.ItemAgentChildWaitingDays, "3", wsconfig.SetOptions{ExplicitScope: wsconfig.ScopeGlobal}); err != nil {
		t.Fatal(err)
	}
	if got := NewRunner(Options{Config: cfg}).childWaitingExpiry(); got != 3*day {
		t.Fatalf("the configured expiry = %v", got)
	}
	if got := NewRunner(Options{Config: cfg, ChildWaitingDays: 1}).childWaitingExpiry(); got != day {
		t.Fatalf("the override = %v", got)
	}
}

// agentLeadHarness runs hosted leads (claude with common permission levels,
// and a lead-only "codex" backend) whose agent.spawn children are Pi workers
// under fake holders.
func agentLeadHarness(t *testing.T) (*leadHarness, *wakeRefusals) {
	t.Helper()
	h := newLeadHarness(t)
	refusals := &wakeRefusals{}
	got := &reports{}
	h.opts.Report = got.add
	h.opts.WakeRefused = refusals.add
	h.opts.WSMCP = func(string) (MCPCommand, error) { return MCPCommand{Command: "/unused/ws-mcp"}, nil }
	claude := h.opts.Backends[BackendClaude]
	claude.PermissionLevels = LeadPermissionLevels()
	h.opts.Backends[BackendClaude] = claude
	h.opts.Backends["codex"] = BackendRegistration{Backend: &fakeBackend{}, LeadProfile: "codex-lead"}
	h.globalProfile("codex-lead", `{"backend":"codex","lead":true}`)
	for tier, model := range map[string]string{"small": "prov/small", "medium": "prov/medium"} {
		if _, err := wsconfig.SetGlobalAgentsTierForHarness(h.opts.Config, tier, "", model, PiTierHarness, "low"); err != nil {
			t.Fatal(err)
		}
	}
	h.start()
	h.runner.SetEndpoint("http://127.0.0.1:1")
	return h, refusals
}

func (h *leadHarness) caller(lead State) AgentCaller {
	return LeadCaller(lead.ID, h.root, lead.Backend, h.opts.Config)
}

func TestAgentSpawnCapsAtAHostedLeadsLevel(t *testing.T) {
	h, _ := agentLeadHarness(t)
	spawn := func(caller AgentCaller, level PermissionLevel) (string, error) {
		res, err := h.runner.AgentSpawn(caller, AgentSpawnRequest{Prompt: "x", Tier: "pi.small", Authority: string(level)})
		return agentID(res.Address), err
	}

	// A legacy-derived claude lead caps at its mode's level: full for
	// bypassPermissions, admitted at create and at the process start.
	full := h.lead(BackendClaude, LeadOptions{PermissionMode: "bypassPermissions"})
	id, err := spawn(h.caller(full), PermissionFull)
	if err != nil {
		t.Fatalf("a full child of a full lead: %v", err)
	}
	h.holder(id)
	if lvl := childLevel(wsdashboard.SessionDir(h.dir, id)); lvl != PermissionFull {
		t.Fatalf("the child's level = %q", lvl)
	}
	if st := h.state(id); st.Error != "" {
		t.Fatalf("the full child's start = %+v", st)
	}

	// auto: workdir-write.
	auto := h.lead(BackendClaude, LeadOptions{PermissionMode: "auto"})
	_, err = spawn(h.caller(auto), PermissionFull)
	agentRefused(t, err, http.StatusForbidden, "authority full is above this session's own level workdir-write")
	if _, err := spawn(h.caller(auto), PermissionWorkdirWrite); err != nil {
		t.Fatal(err)
	}
	// Its explicit level (the sidecar) is its current one.
	if err := writePermissionSelection(wsdashboard.SessionDir(h.dir, auto.ID), PermissionReadOnly); err != nil {
		t.Fatal(err)
	}
	_, err = spawn(h.caller(auto), PermissionWorkdirWrite)
	agentRefused(t, err, http.StatusForbidden, "above this session's own level read-only")

	// A lead with no common level on its backend, and a lead reply-id with
	// no Runner record, cap at workdir-write.
	codex := h.lead("codex", LeadOptions{})
	_, err = spawn(h.caller(codex), PermissionFull)
	agentRefused(t, err, http.StatusForbidden, "above this session's own level workdir-write")
	if _, err := spawn(h.caller(codex), PermissionWorkdirWrite); err != nil {
		t.Fatal(err)
	}
	_, err = h.runner.AgentSpawn(h.caller(codex), AgentSpawnRequest{Prompt: "x", Tier: "small", Authority: "read-only"})
	// No bare tier is listed for a lead-only harness.
	agentRefused(t, err, http.StatusBadRequest, `harness "codex" is registered for hosted leads only`, "valid tiers: claude.small", "pi.small, pi.medium")
	ghost := LeadCaller("noleadrecordhere", h.root, BackendClaude, h.opts.Config)
	_, err = h.runner.AgentSpawn(ghost, AgentSpawnRequest{Prompt: "x", Tier: "pi.small", Authority: "full"})
	agentRefused(t, err, http.StatusForbidden, "above this session's own level workdir-write")
}

// A hosted lead that lowered its level refuses its sleeping child's wake;
// the mail waits, and the wake after the lead's level is back takes it.
func TestRefusedWakeUnderAHostedLead(t *testing.T) {
	h, refusals := agentLeadHarness(t)
	lead := h.lead(BackendClaude, LeadOptions{PermissionMode: "auto"})
	res, err := h.runner.AgentSpawn(h.caller(lead), AgentSpawnRequest{Prompt: "work", Tier: "pi.small", Authority: "workdir-write"})
	if err != nil {
		t.Fatal(err)
	}
	x := agentID(res.Address)
	first := h.holder(x)
	h.holders.out(first, `{"type":"agent_start"}`)
	h.holders.out(first, recoveryPiReply("x done")...)
	eventually(t, "x's turn settled", func() bool { rec := h.record(x); return rec.LastTurn != nil && !rec.TurnOpen })
	if _, err := h.runner.Stop(x); err != nil {
		t.Fatal(err)
	}
	eventually(t, "x asleep", func() bool {
		st := h.state(x)
		return !st.Process && st.Lifecycle == wsdashboard.SessionChildWaiting
	})

	if err := writePermissionSelection(wsdashboard.SessionDir(h.dir, lead.ID), PermissionReadOnly); err != nil {
		t.Fatal(err)
	}
	launches := h.holders.opCount("launch")
	if _, err := h.runner.Deliver(x, Delivery{From: lead.ID, Text: "held mail"}); err != nil {
		t.Fatal(err)
	}
	eventually(t, "the refused start", func() bool { return strings.Contains(h.state(x).Error, "current level read-only") })
	time.Sleep(60 * time.Millisecond)
	if n := h.holders.opCount("launch"); n != launches {
		t.Fatalf("a refused wake launched %d", n-launches)
	}
	if st := h.state(x); st.Held == 0 || st.Lifecycle != wsdashboard.SessionChildWaiting {
		t.Fatalf("the refused child = %+v", st)
	}
	if got := refusals.list(); len(got) != 1 || got[0][0] != lead.ID || got[0][1] != x {
		t.Fatalf("the sender's notices = %v", got)
	}

	if err := writePermissionSelection(wsdashboard.SessionDir(h.dir, lead.ID), PermissionWorkdirWrite); err != nil {
		t.Fatal(err)
	}
	if _, err := h.runner.Deliver(x, Delivery{From: lead.ID, Text: "second mail"}); err != nil {
		t.Fatal(err)
	}
	eventually(t, "the admitted wake", func() bool { return h.holders.opCount("launch") == launches+1 })
	next := h.holder(x)
	eventually(t, "both mails taken", func() bool {
		all := strings.Join(h.holders.inputs(next, "prompt"), "\n")
		return strings.Contains(all, "held mail") && strings.Contains(all, "second mail")
	})
	if got := refusals.list(); len(got) != 1 {
		t.Fatalf("an admitted wake told the sender: %v", got)
	}
}
