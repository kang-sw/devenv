package wsagent

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsdashboard"
)

// mailHarness is agentHarness with a mailbox the test delivers into.
func mailHarness(t *testing.T, configure func(*Options)) (*harness, *fakeMail) {
	t.Helper()
	mail := newFakeMail()
	h, _ := agentHarness(t, func(o *Options) {
		o.Mail = mail
		if configure != nil {
			configure(o)
		}
	})
	return h, mail
}

// keyOf is conversation id's ws session key: its mailbox address's key.
func (h *harness) keyOf(id string) string {
	h.t.Helper()
	key := h.record(id).WSKey
	if key == "" {
		h.t.Fatalf("%s has no key", id)
	}
	return key
}

// Every spawned child's mailbox is watched, awake or asleep, and mail to a
// running child reaches its turn as a steer, tagged with how its sender
// relates to it through the spawn lineage; system mail carries no tag.
func TestMailIsSteeredIntoARunningTurnWithItsRelation(t *testing.T) {
	h, mail := mailHarness(t, nil)
	a, ap := h.agentRunning(h.leadCaller(), AgentSpawnRequest{Prompt: "parent work", Tier: "medium", Authority: "workdir-write"})
	b, bp := h.agentRunning(h.holderOf(a), AgentSpawnRequest{Prompt: "child work", Tier: "small", Authority: "workdir-write"})
	aKey, bKey := h.keyOf(a), h.keyOf(b)
	eventually(t, "both mailboxes watched", func() bool { return mail.watched(aKey) && mail.watched(bKey) })

	mail.deliverFrom(bKey, a, "from the parent")
	mail.deliverFrom(bKey, "peerreplyid", "from a peer")
	mail.deliver(bKey, "a system notice")
	mail.deliverFrom(aKey, b, "from the child")
	mail.deliverFrom(aKey, leadParent.ReplyID, "from the lead")
	eventually(t, "b's steers", func() bool {
		got, _ := bp.steered()
		return slices.Equal(got, []string{"(parent) from the parent", "(peer) from a peer", "a system notice"})
	})
	eventually(t, "a's steers", func() bool {
		got, _ := ap.steered()
		return slices.Equal(got, []string{"(child) from the child", "(parent) from the lead"})
	})
	for _, id := range []string{a, b} {
		if rec := h.record(id); len(rec.Mail) != 0 {
			t.Fatalf("%s: steered mail rides the running turn, not the next: %q", id, rec.Mail)
		}
	}
}

// Mail waits for the next turn where the running turn cannot take it: the
// harness's catalog profile grades steer unsupported (a Claude worker), or
// the process has no steer handle.
func TestMailWaitsForTheNextTurnWithoutSteer(t *testing.T) {
	h, mail := mailHarness(t, nil)
	a, ap := h.agentRunning(h.leadCaller(), AgentSpawnRequest{Prompt: "graded", Tier: "medium", Authority: "workdir-write"})
	c := h.runner.conv(a)
	c.setStatus(func() {
		caps := ClaudeCatalog().Resolve(RoleWorker, "")
		c.caps = &caps
	})
	mail.deliverFrom(h.keyOf(a), leadParent.ReplyID, "after this turn")
	eventually(t, "the mail kept for the next turn", func() bool {
		return slices.Equal(h.record(a).Mail, []string{"(parent) after this turn"})
	})
	if got, _ := ap.steered(); len(got) != 0 {
		t.Fatalf("a graded-unsupported steer was used: %q", got)
	}
	ap.reply("first done")
	if next := ap.nextSent(t); !strings.Contains(next, "(parent) after this turn") {
		t.Fatalf("the next turn = %q", next)
	}
	eventually(t, "the delivered mail dropped", func() bool { return len(h.record(a).Mail) == 0 })

	h.backend.mu.Lock()
	h.backend.steers = false
	h.backend.mu.Unlock()
	b, bp := h.agentRunning(h.leadCaller(), AgentSpawnRequest{Prompt: "no handle", Tier: "small", Authority: "read-only"})
	mail.deliverFrom(h.keyOf(b), "peerreplyid", "when you can")
	eventually(t, "the mail kept for the next turn", func() bool {
		return slices.Equal(h.record(b).Mail, []string{"(peer) when you can"})
	})
	bp.reply("done")
	if next := bp.nextSent(t); !strings.Contains(next, "(peer) when you can") {
		t.Fatalf("the next turn = %q", next)
	}
}

// Mail to a sleeping child wakes it even when its parent has the width
// cap's number of awake children.
func TestMailWakesASleepingChildPastTheWidthCap(t *testing.T) {
	h, mail := mailHarness(t, nil)
	lead := h.leadCaller()
	var ids []string
	for i := range MaxSpawnWidth {
		ids = append(ids, h.agentSpawn(lead, AgentSpawnRequest{Prompt: fmt.Sprintf("job %d", i), Tier: "small", Authority: "read-only"}))
	}
	eventually(t, "every child's process", func() bool { return h.backend.startCount() == MaxSpawnWidth })
	sleeper := ids[0]
	h.asleep(sleeper)
	h.agentSpawn(lead, AgentSpawnRequest{Prompt: "the hundredth", Tier: "small", Authority: "read-only"})
	eventually(t, "the parent at the cap", func() bool {
		return h.runner.awakeChildren(leadParent) == MaxSpawnWidth && h.backend.startCount() == MaxSpawnWidth+1
	})
	starts := h.backend.startCount()
	mail.deliverFrom(h.keyOf(sleeper), leadParent.ReplyID, "wake past the cap")
	p := h.backend.waitProc(t, starts)
	if text := p.nextSent(t); !strings.Contains(text, "(parent) wake past the cap") {
		t.Fatalf("the woken turn = %q", text)
	}
	if n := h.runner.awakeChildren(leadParent); n != MaxSpawnWidth+1 {
		t.Fatalf("awake children after the wake = %d", n)
	}
}

// A refused wake keeps the claimed mail with the sleeping child, across a
// daemon restart, and tells each sender of a claim once; mail that waited
// in the mailbox while no daemon ran is claimed at start; the next
// admitted wake delivers it all.
func TestRefusedMailWakeKeepsTheMailAcrossARestart(t *testing.T) {
	refusals := &wakeRefusals{}
	h, mail := mailHarness(t, func(o *Options) { o.WakeRefused = refusals.add })
	a, b := settledPair(t, h)
	bKey := h.keyOf(b)
	if err := writePermissionSelection(wsdashboard.SessionDir(h.dir, a), PermissionReadOnly); err != nil {
		t.Fatal(err)
	}
	starts := h.backend.startCount()
	mail.deliverFrom(bKey, a, "first")
	eventually(t, "the refused start", func() bool { return strings.Contains(h.state(b).Error, "current level read-only") })
	mail.deliverFrom(bKey, "peerreplyid", "from a peer")
	eventually(t, "both kept", func() bool { return len(h.record(b).Mail) == 2 })
	eventually(t, "both senders told", func() bool { return len(refusals.list()) == 2 })
	got := refusals.list()
	if got[0][0] != a || got[1][0] != "peerreplyid" || got[0][1] != b || !strings.Contains(got[1][2], "read-only") {
		t.Fatalf("the notices = %v", got)
	}
	if st := h.state(b); st.Process || st.Lifecycle != wsdashboard.SessionChildWaiting || AgentStatus(st) != AgentWaiting {
		t.Fatalf("the refused child = %+v", st)
	}
	if n := h.runner.awakeChildren(ParentRef{Kind: ParentConversation, ReplyID: a}); n != 0 {
		t.Fatalf("a refused child counts as awake: %d", n)
	}

	h.runner.Close()
	mail.deliverFrom(bKey, "otherreplyid", "while down")
	h.start()
	eventually(t, "the waiting mail claimed at start", func() bool { return len(h.record(b).Mail) == 3 })
	time.Sleep(100 * time.Millisecond)
	if n := h.backend.startCount(); n != starts {
		t.Fatalf("a refused wake started %d processes", n-starts)
	}
	if got := refusals.list(); len(got) != 3 || got[2][0] != "otherreplyid" {
		t.Fatalf("a restart tells only the new sender: %v", got)
	}

	if err := writePermissionSelection(wsdashboard.SessionDir(h.dir, a), PermissionWorkdirWrite); err != nil {
		t.Fatal(err)
	}
	mail.deliverFrom(bKey, a, "second")
	p := h.backend.waitProc(t, starts)
	text := p.nextSent(t)
	for _, want := range []string{"(parent) first", "(peer) from a peer", "(peer) while down", "(parent) second"} {
		if !strings.Contains(text, want) {
			t.Fatalf("the woken turn %q lacks %q", text, want)
		}
	}
	if got := refusals.list(); len(got) != 3 {
		t.Fatalf("an admitted wake told a sender: %v", got)
	}
}

// Mail resets a sleeping child's waiting expiry, even when its wake is
// refused and it stays asleep.
func TestMailResetsTheChildWaitingClock(t *testing.T) {
	h, clk := expiryHarness(t)
	mail := newFakeMail()
	h.config.Mail = mail
	h.runner.Close()
	h.start()
	a, b := settledPair(t, h)
	// a stays well inside its own expiry, so only b's clock is measured.
	h.runner.conv(a).update(func(rec *Record) { rec.LastActiveAt = clk.Now().Add(30 * day).UTC().Format(time.RFC3339) })
	if err := writePermissionSelection(wsdashboard.SessionDir(h.dir, a), PermissionReadOnly); err != nil {
		t.Fatal(err)
	}
	clk.advance(day + 12*time.Hour)
	mail.deliverFrom(h.keyOf(b), a, "keep waiting")
	eventually(t, "the refused wake", func() bool { return len(h.record(b).Mail) == 1 && h.state(b).Error != "" })
	if rec := h.record(b); rec.LastActiveAt != clk.Now().UTC().Format(time.RFC3339) {
		t.Fatalf("the mail's LastActiveAt = %q, want %q", rec.LastActiveAt, clk.Now().UTC().Format(time.RFC3339))
	}
	// Two and a half days after the settle, one after the mail.
	clk.advance(day)
	time.Sleep(200 * time.Millisecond)
	if gone(h, b) {
		t.Fatal("a child mailed a day ago expired")
	}
	clk.advance(day + time.Hour)
	eventually(t, "the child expired with its mail", func() bool { return gone(h, b) })
	if gone(h, a) {
		t.Fatal("the parent expired with its child")
	}
}

func TestChildMailEnvelopeKeepsParentRelation(t *testing.T) {
	h, _ := mailHarness(t, nil)
	a, b := settledPair(t, h)
	card := h.card()
	for _, tc := range []struct{ target, sender, want string }{
		{a, leadParent.ReplyID, "parent"},
		{a, b, "child"},
		{b, a, "parent"},
		{b, "someoneelse", "peer"},
		{card, leadParent.ReplyID, "peer"},
		{b, "", ""},
	} {
		if got := h.runner.mailRelation(h.record(tc.target), tc.sender); got != tc.want {
			t.Fatalf("relation of %q to %s = %q, want %q", tc.sender, tc.target, got, tc.want)
		}
	}
}
