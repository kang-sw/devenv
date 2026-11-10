package wsagent

import (
	"errors"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wsinbox"
)

var errStartRefused = errors.New("start refused")

// returns runs fn on its own goroutine and fails the test when fn has not
// returned within a generous bound: the path under test must not wait on
// the blocked one at all, so reaching the bound means it does.
func returns(t *testing.T, what string, fn func()) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		fn()
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatalf("%s waited on the blocked path", what)
	}
}

// TestStartConvBlocksNoOtherConversation: while one conversation's start
// writes its registration (and tells the daemon), Get, List, and act on
// another conversation do not wait for it.
func TestStartConvBlocksNoOtherConversation(t *testing.T) {
	h := newHarness(t, map[string]string{"chat": chatProfile})
	h.runner.Close()
	var mu sync.Mutex
	armed, known := false, ""
	entered, release := make(chan struct{}, 1), make(chan struct{})
	notify := func(replyID, _ string) {
		mu.Lock()
		block := armed && replyID != known
		if block {
			armed = false
		}
		mu.Unlock()
		if block {
			entered <- struct{}{}
			<-release
		}
	}
	h.runner = NewRunner(h.config)
	h.runner.Start(notify)
	t.Cleanup(h.runner.Close)
	a := h.create("chat")

	mu.Lock()
	armed, known = true, a.ID
	mu.Unlock()
	created := make(chan struct{})
	go func() {
		defer close(created)
		h.create("chat")
	}()
	<-entered
	returns(t, "Get", func() { h.state(a.ID) })
	returns(t, "List", func() { h.runner.List() })
	returns(t, "act", func() {
		if _, err := h.runner.Stop(a.ID); err != nil {
			t.Error(err)
		}
	})
	close(release)
	<-created
	if n := len(h.runner.List()); n != 2 {
		t.Fatalf("List = %d conversations, want 2", n)
	}
}

// TestSendReturnsWhileTheProcessStarts: a send returns once its message is
// held, while the loop is busy starting the process, and the messages held
// meanwhile go out as the first turn.
func TestSendReturnsWhileTheProcessStarts(t *testing.T) {
	h := newHarness(t, map[string]string{"chat": chatProfile})
	gate := h.backend.gateStarts(t)
	id := h.create("chat").ID
	returns(t, "the first send", func() {
		if st, err := h.runner.Send(id, "one"); err != nil || st.Held != 1 {
			t.Errorf("Send = %+v, %v; want the message held", st, err)
		}
	})
	gate.awaitEntered(t)
	returns(t, "a send while the process starts", func() {
		if _, err := h.runner.Send(id, "two"); err != nil {
			t.Error(err)
		}
	})
	returns(t, "Get while the process starts", func() { h.state(id) })
	gate.release()
	if got := h.backend.waitProc(t, 0).nextSent(t); got != "one\n\ntwo" {
		t.Fatalf("first turn = %q, want both held messages", got)
	}
}

// TestSendRacingCloseNeverLandsInAClosedInbox: a send made while a close
// is underway is refused, never written into the inbox the close is
// clearing, and the closed conversation holds nothing.
func TestSendRacingCloseNeverLandsInAClosedInbox(t *testing.T) {
	h := newHarness(t, map[string]string{"chat": chatProfile})
	id := h.create("chat").ID
	h.runner.Send(id, "one")
	p := h.backend.waitProc(t, 0)
	p.nextSent(t)
	gate := p.gateClose(t)
	closed := make(chan error, 1)
	go func() {
		_, err := h.runner.CloseConversation(id)
		closed <- err
	}()
	gate.awaitEntered(t)
	returns(t, "a send during a close", func() {
		if _, err := h.runner.Send(id, "late"); requestStatus(err) != http.StatusConflict {
			t.Errorf("a send during a close = %v, want a 409", err)
		}
	})
	gate.release()
	if err := <-closed; err != nil {
		t.Fatal(err)
	}
	if held, err := wsinbox.Held(wsinbox.Dir(wsdashboard.SessionDir(h.dir, id))); err != nil || len(held) != 0 {
		t.Fatalf("the closed conversation's inbox holds %d messages (%v), want none", len(held), err)
	}
}

// TestSupportSendWaitsOnNoLoopAndNoOtherStore: a store's send returns while
// its conversation's loop is busy starting the process, and another
// store's send does not wait on a store whose conversation is opening.
func TestSupportSendWaitsOnNoLoopAndNoOtherStore(t *testing.T) {
	sh := newSupportHarness(t)
	gate := sh.claude.gateStarts(t)
	returns(t, "the first send", func() { sh.send("alpha", "one") })
	gate.awaitEntered(t)
	returns(t, "a send while the process starts", func() { sh.send("alpha", "two") })
	gate.release()
	if got := sh.claude.waitProc(t, 0).nextSent(t); got != "one\n\ntwo" {
		t.Fatalf("first turn = %q", got)
	}

	// alpha's next conversation blocks while its key is minted, inside
	// alpha's store lock.
	mint := sh.keys.gateMint(t, sh.stores["alpha"])
	renewed := make(chan error, 1)
	go func() {
		_, err := sh.support.New("alpha")
		renewed <- err
	}()
	mint.awaitEntered(t)
	returns(t, "another store's send", func() { sh.send("beta", "hello") })
	returns(t, "another store's read", func() { sh.storeConv("beta") })
	mint.release()
	if err := <-renewed; err != nil {
		t.Fatal(err)
	}
}

// TestSupportStartFailureShowsOnTheConversation: a send whose process
// cannot start still answers with the held message; the failure reaches
// the conversation's State.Error.
func TestSupportStartFailureShowsOnTheConversation(t *testing.T) {
	sh := newSupportHarness(t)
	sh.claude.mu.Lock()
	sh.claude.startErr = errStartRefused
	sh.claude.mu.Unlock()
	st := sh.send("alpha", "hello")
	if st.Held != 1 {
		t.Fatalf("send = %+v, want the message held", st)
	}
	eventually(t, "the start failure", func() bool {
		c := sh.storeConv("alpha")
		return c != nil && strings.Contains(c.Error, errStartRefused.Error())
	})
}
