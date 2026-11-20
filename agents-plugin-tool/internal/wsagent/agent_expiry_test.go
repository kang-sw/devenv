// agent_expiry_test.go holds child waiting expiry tests.
package wsagent

import (
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsconfig"
	"github.com/kang-sw/devenv/internal/wsdashboard"
)

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
	// Send resets expiry in RAM immediately; the stamp rides the next turn
	// boundary or graceful shutdown, not a standalone record replacement.
	if rec := h.runner.conv(a).record(); rec.LastActiveAt != clk.Now().UTC().Format(time.RFC3339) {
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

// TestChildWaitingExpiryRetriesAfterFailedClose: when expire's CloseConversation
// fails with a non-errClosed error, expire resets c.expiring so a later
// heartbeat retries and deletes the expired child.
func TestChildWaitingExpiryRetriesAfterFailedClose(t *testing.T) {
	clk := newTestClock()
	var logBuf logBuffer
	h, _ := agentHarness(t, func(o *Options) {
		o.Now = clk.Now
		o.ChildWaitingDays = 2
		o.Log = &logBuf
	})
	a, b := settledPair(t, h)
	// b was active after a, so a expires first and closes b before closing a.
	h.runner.conv(b).update(func(rec *Record) { rec.LastActiveAt = clk.Now().Add(day).UTC().Format(time.RFC3339) })

	// Inject a transient non-errClosed failure on CloseConversation(a): when
	// CloseConversation(a) finishes closing descendant b, remove a from r.convs
	// once so closeOne(a) returns errUnknown instead of closing a.
	var mu sync.Mutex
	var saved *conv
	injected := false
	stop := h.runner.Watch(func(st State) {
		if st.ID != b || st.Status != StatusClosed {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		if injected {
			return
		}
		injected = true
		h.runner.mu.Lock()
		saved = h.runner.convs[a]
		delete(h.runner.convs, a)
		h.runner.mu.Unlock()
	})
	defer stop()

	// Advance 2.5 days so a (age 2.5d >= 2d) expires while b (age 1.5d < 2d)
	// does not expire on its own and only closes inside CloseConversation(a).
	clk.advance(2*day + 12*time.Hour)
	eventually(t, "the failed expiry close is logged", func() bool {
		return strings.Contains(logBuf.String(), "agent "+a+": expire:")
	})
	mu.Lock()
	c := saved
	mu.Unlock()
	if c == nil {
		t.Fatal("failed-close injection did not capture parent conv")
	}
	// expire must clear c.expiring after the non-errClosed failure so the
	// heartbeat can retry.
	eventually(t, "expiring flag cleared for heartbeat retry", func() bool {
		var expiring bool
		if !c.do(func() { expiring = c.expiring }) {
			return false
		}
		return !expiring
	})
	if gone(h, a) {
		t.Fatal("child directory deleted despite failed close")
	}

	// Restore a into r.convs and let the next heartbeat retry expiry.
	h.runner.mu.Lock()
	h.runner.convs[a] = c
	h.runner.mu.Unlock()
	eventually(t, "heartbeat retried expiry and deleted child", func() bool {
		return gone(h, a)
	})
	if len(h.runner.AgentList(h.leadCaller())) != 0 {
		t.Fatal("the lead still lists the retried expired child")
	}
}

// agentLeadHarness runs hosted leads (claude with common permission levels,
// and a lead-only "codex" backend) whose agent.spawn children are Pi workers
// under fake holders.
