package wsagent

import (
	"encoding/json"
	"net/http"
	"os"
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
	eventually(t, "lead receives a's report", func() bool {
		reps := got.lead()
		return len(reps) == 1 && reps[0].Result.ID == a
	})
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
