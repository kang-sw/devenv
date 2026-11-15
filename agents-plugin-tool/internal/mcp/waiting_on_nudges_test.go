package mcp

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wswhiteboard"
)

type waitingNudgeTickets struct {
	status string
	calls  int
}

func (r *waitingNudgeTickets) TicketStatus(string) (string, bool) {
	r.calls++
	return r.status, r.status != ""
}

func TestWaitingOnNudgeInjectedResolverIsLazyAndShared(t *testing.T) {
	f := newNudgeFixture(t)
	r := &waitingNudgeTickets{}
	f.n.tickets = func(root string) wswhiteboard.TicketResolver {
		if root != f.target.root {
			t.Fatal("resolver root", root)
		}
		return r
	}
	f.n.process(f.target, nudgeNow)
	f.board(t, `<wb:topic name="Work"><wb:state value="waiting" on="exec:3"/></wb:topic>`, nudgeNow)
	f.n.process(f.target, nudgeNow)
	if r.calls != 0 {
		t.Fatal("board without ticket dependencies read tickets")
	}
	f.board(t, `<wb:topic name="Work"><wb:state value="waiting" on="ticket:261115-feat-example"/></wb:topic><wb:topic name="Other"><wb:log>waiting</wb:log><wb:state value="waiting" on="ticket:261115-feat-example"/></wb:topic>`, nudgeNow)
	f.n.process(f.target, nudgeNow)
	if r.calls != 1 || len(f.queue(t)) != 0 {
		t.Fatal("unknown targets/cached resolution", r.calls)
	}
	r.status = "done"
	f.n.process(f.target, nudgeNow)
	q := f.queue(t)
	if r.calls != 2 || len(q) != 2 {
		t.Fatal("injected completion", r.calls, len(q))
	}
	for _, mail := range q {
		if !strings.Contains(mail.Content, "unblocked") || !strings.Contains(mail.Content, "ticket:261115-feat-example") {
			t.Fatal(mail.Content)
		}
	}
	f.n.process(f.target, nudgeNow)
	if len(f.queue(t)) != 2 {
		t.Fatal("same-declaration duplicate")
	}
}

func TestWaitingOnNudgeProductionTicketTreeRestartAndReblock(t *testing.T) {
	f := newNudgeFixture(t)
	stem := "261115-feat-local-example"
	base := filepath.Join(f.target.root, "ai-docs", "tickets")
	for _, status := range []string{"ready", ".done"} {
		if err := os.MkdirAll(filepath.Join(base, status), 0755); err != nil {
			t.Fatal(err)
		}
	}
	ready, done := filepath.Join(base, "ready", stem+".md"), filepath.Join(base, ".done", stem+".md")
	if err := os.WriteFile(ready, []byte("---\ntitle: Example\n---\n"), 0644); err != nil {
		t.Fatal(err)
	}
	f.board(t, `<wb:topic name="Work"><wb:state value="waiting" on="ticket:`+stem+`"/></wb:topic>`, nudgeNow)
	f.n.process(f.target, nudgeNow)
	if len(f.queue(t)) != 0 {
		t.Fatal("open ticket nudged")
	}
	if err := os.Rename(ready, done); err != nil {
		t.Fatal(err)
	}
	// Reconciliation, with no page and no whiteboard update, observes local closure.
	f.n.process(f.target, nudgeNow)
	if len(f.queue(t)) != 1 {
		t.Fatal("local done not observed")
	}
	f.n = NewWhiteboardNudges(f.n.dir, nil, io.Discard)
	f.n.process(f.target, nudgeNow)
	if len(f.queue(t)) != 1 {
		t.Fatal("restart lost receipt")
	}
	if err := os.Rename(done, ready); err != nil {
		t.Fatal(err)
	}
	f.n.process(f.target, nudgeNow)
	if err := os.Rename(ready, done); err != nil {
		t.Fatal(err)
	}
	f.n.process(f.target, nudgeNow)
	if len(f.queue(t)) != 1 {
		t.Fatal("reblock/resolve renudged")
	}
	f.board(t, `<wb:topic name="Work"><wb:state value="waiting" on="ticket:`+stem+` owner"/></wb:topic>`, nudgeNow)
	f.n.process(f.target, nudgeNow)
	if len(f.queue(t)) != 2 {
		t.Fatal("redeclaration did not nudge")
	}
}
