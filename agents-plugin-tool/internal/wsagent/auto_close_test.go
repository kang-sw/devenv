package wsagent

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wsinbox"
)

// autoCloseHarness is childHarness with finished Pi children closing on
// their own, as in production.
func autoCloseHarness(t *testing.T) (*harness, *reports) {
	t.Helper()
	h, got := childHarness(t)
	h.config.keepFinished = false
	h.runner.Close()
	h.start()
	return h, got
}

// finishableWorker spawns a lead's Pi worker with a session file a Send
// can reopen it on, and returns it with its process, first turn sent.
func finishableWorker(t *testing.T, h *harness) (State, *fakeProc) {
	t.Helper()
	n := h.backend.startCount()
	w := h.spawnWorker("medium")
	p := h.backend.waitProc(t, n)
	p.nextSent(t)
	file := piSessionFile(wsdashboard.SessionDir(h.dir, w.ID))
	if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte("persisted conversation\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return w, p
}

func eventuallyClosed(t *testing.T, h *harness, id string) {
	t.Helper()
	eventually(t, id+" closed on its own", func() bool { return h.state(id).Status == StatusClosed })
}

func stillOpen(t *testing.T, h *harness, id, why string) {
	t.Helper()
	time.Sleep(100 * time.Millisecond)
	if h.state(id).Status == StatusClosed {
		t.Fatalf("%s: %s closed", why, id)
	}
}

func TestFinishedPiWorkerClosesAfterItsReportAndSendReopensIt(t *testing.T) {
	h, got := autoCloseHarness(t)
	w, p := finishableWorker(t, h)
	waited := make(chan TurnResult, 1)
	go func() {
		res, _ := h.runner.Wait(context.Background(), w.ID, 5*time.Second)
		waited <- res
	}()
	time.Sleep(20 * time.Millisecond)
	p.reply("result")
	if res := <-waited; res.Status != TurnDone || res.Message != "result" {
		t.Fatalf("a wait made before the finish = %+v", res)
	}
	eventuallyClosed(t, h, w.ID)
	if !p.isClosed() {
		t.Fatal("the closed worker's process still runs")
	}
	if rec := h.record(w.ID); rec.ClosedAt == "" || rec.TurnOpen {
		t.Fatalf("closed record = %+v", rec)
	}
	if reps := got.lead(); len(reps) != 1 || reps[0].Result.Message != "result" {
		t.Fatalf("the lead's report = %+v", reps)
	}
	// A wait after the close still answers with the reported turn.
	if res, err := h.runner.Wait(context.Background(), w.ID, time.Second); err != nil || res.Status != TurnDone || res.Message != "result" {
		t.Fatalf("a wait made after the close = %+v, %v", res, err)
	}

	st, err := h.runner.Send(w.ID, "more")
	if err != nil || st.Status == StatusClosed {
		t.Fatalf("Send to the closed worker = %+v, %v", st, err)
	}
	p2 := h.backend.waitProc(t, 1)
	if text := p2.nextSent(t); text != h.envelope("more") {
		t.Fatalf("reopened turn = %q", text)
	}
	if res, _ := h.runner.Wait(context.Background(), w.ID, 50*time.Millisecond); res.Status != TurnRunning {
		t.Fatalf("a wait for the new message while it runs = %+v", res)
	}
	p2.reply("second")
	eventuallyClosed(t, h, w.ID)
	if reps := got.lead(); len(reps) != 2 || reps[1].Result.Message != "second" {
		t.Fatalf("the lead's reports = %+v", reps)
	}
}

// A child whose report its parent has not taken yet stays open, however
// long the parent's loop is busy, and closes once the parent takes it.
func TestPiChildStaysOpenUntilItsParentTakesItsReport(t *testing.T) {
	h, _ := autoCloseHarness(t)
	w, wp, token, gc, gp := workerWithChild(t, h)
	parent := h.runner.conv(w.ID)
	block, entered := make(chan struct{}), make(chan struct{})
	var once sync.Once
	release := func() { once.Do(func() { close(block) }) }
	t.Cleanup(release)
	go parent.do(func() {
		close(entered)
		<-block
	})
	<-entered
	gp.reply("child result")
	eventually(t, "the pending report", func() bool { return len(h.record(gc.ID).PendingReports) == 1 })
	stillOpen(t, h, gc.ID, "a child whose report waits for its parent")
	release()
	eventuallyClosed(t, h, gc.ID)
	if rec := h.record(gc.ID); len(rec.PendingReports) != 0 {
		t.Fatalf("the closed child's report was never taken: %+v", rec.PendingReports)
	}
	eventually(t, "the report steered into the worker's turn", func() bool {
		steered, _ := wp.steered()
		return len(steered) == 1 && strings.Contains(steered[0], "child result")
	})
	if h.state(w.ID).Status == StatusClosed {
		t.Fatal("a worker whose turn runs closed")
	}
	// A wait after the close says the report reached the worker, as for
	// an open child.
	code, body := h.tokenCall(token, "wait", map[string]any{"id": gc.ID, "timeout_s": 1})
	var res TurnResult
	if code != http.StatusOK || json.Unmarshal([]byte(body), &res) != nil || res.Status != TurnDone || !res.Delivered || res.Message != "" {
		t.Fatalf("a wait after the close = %d %s", code, body)
	}
}

// A failed turn leaves its worker open: its failure stays on its row, and
// its parent may send it on.
func TestFailedPiWorkerStaysOpen(t *testing.T) {
	h, got := autoCloseHarness(t)
	w, p := finishableWorker(t, h)
	p.events <- Event{TurnEnd: true, TurnError: "boom"}
	eventually(t, "the failure report", func() bool { return len(got.lead()) == 1 })
	stillOpen(t, h, w.ID, "a failed worker")
	if st := h.state(w.ID); st.Error == "" {
		t.Fatalf("the failed worker's state = %+v", st)
	}
}

// A message in the inbox keeps a finished worker open, even one the page
// wrote whose signal the loop has not taken; it closes after that turn.
func TestHeldMessageKeepsAFinishedWorkerOpen(t *testing.T) {
	h, got := autoCloseHarness(t)
	w, p := finishableWorker(t, h)
	c := h.runner.conv(w.ID)
	if _, _, err := wsinbox.Send(c.inbox, wsinbox.Entry{Parts: []string{"page message"}}, h.config.Now()); err != nil {
		t.Fatal(err)
	}
	p.reply("first")
	stillOpen(t, h, w.ID, "a worker with a held message")
	h.runner.InboxChanged(w.ID)
	if text := p.nextSent(t); !strings.Contains(text, "page message") {
		t.Fatalf("the held message's turn = %q", text)
	}
	p.reply("second")
	eventuallyClosed(t, h, w.ID)
	if reps := got.lead(); len(reps) != 2 || reps[1].Result.Message != "second" {
		t.Fatalf("the lead's reports = %+v", reps)
	}
}

// A worker whose done turn is held for its open child stays open; the
// child closes once its report reached the worker, and the worker after
// the turn that report starts.
func TestHeldTurnKeepsAWorkerOpenUntilItsChildReports(t *testing.T) {
	h, got := autoCloseHarness(t)
	w, wp, _, gc, gp := workerWithChild(t, h)
	wp.reply("interim")
	eventually(t, "the held turn", func() bool { return h.record(w.ID).HeldTurn != nil })
	stillOpen(t, h, w.ID, "a worker whose turn is held for its child")
	gp.reply("child result")
	eventuallyClosed(t, h, gc.ID)
	if text := wp.nextSent(t); !strings.Contains(text, "child result") {
		t.Fatalf("the report's turn = %q", text)
	}
	if h.state(w.ID).Status == StatusClosed {
		t.Fatal("a worker whose turn runs closed")
	}
	wp.reply("final")
	eventuallyClosed(t, h, w.ID)
	eventually(t, "the worker's report", func() bool {
		reps := got.lead()
		return len(reps) > 0 && reps[len(reps)-1].Result.Message == "final"
	})
}

// A parent's pi.wait made before the child finishes takes the result; one
// made after the child closed answers with it again, as for an open child
// whose report a wait took.
func TestParentWaitAcrossAutoClose(t *testing.T) {
	h, _ := autoCloseHarness(t)
	_, _, token, gc, gp := workerWithChild(t, h)
	type answer struct {
		code int
		body string
	}
	done := make(chan answer, 1)
	go func() {
		code, body := h.tokenCall(token, "wait", map[string]any{"id": gc.ID, "timeout_s": 5})
		done <- answer{code, body}
	}()
	c := h.runner.conv(gc.ID)
	eventually(t, "the parent's wait", func() bool {
		c.mu.Lock()
		defer c.mu.Unlock()
		return len(c.parentWaits) == 1
	})
	gp.reply("child result")
	a := <-done
	var res TurnResult
	if a.code != http.StatusOK || json.Unmarshal([]byte(a.body), &res) != nil || res.Status != TurnDone || res.Message != "child result" {
		t.Fatalf("the wait = %d %s", a.code, a.body)
	}
	eventuallyClosed(t, h, gc.ID)
	code, body := h.tokenCall(token, "wait", map[string]any{"id": gc.ID, "timeout_s": 1})
	res = TurnResult{}
	if code != http.StatusOK || json.Unmarshal([]byte(body), &res) != nil || res.Status != TurnDone || res.Delivered || res.Message != "child result" {
		t.Fatalf("a wait after the close = %d %s", code, body)
	}
}

// A message sent as the worker finishes is never lost: it lands before
// the close and keeps the worker open, or the close came first and the
// send reopens it. No path deadlocks.
func TestSendRacingAnAutoCloseIsNeverLost(t *testing.T) {
	h, _ := autoCloseHarness(t)
	for i := range 10 {
		w, p := finishableWorker(t, h)
		n := h.backend.startCount()
		p.reply("done")
		// Sent mid-turn it would be steered, which this fake leaves
		// unanswered; the race is with the close the turn's end starts.
		eventually(t, "the turn's end", func() bool { return h.state(w.ID).Status != StatusRunning })
		if _, err := h.runner.Send(w.ID, "follow-up"); err != nil {
			t.Fatalf("round %d: Send = %v", i, err)
		}
		var text string
		deadline := time.Now().Add(5 * time.Second)
		for text == "" {
			if time.Now().After(deadline) {
				t.Fatalf("round %d: the follow-up reached no process", i)
			}
			select {
			case text = <-p.sent:
				continue
			default:
			}
			if h.backend.startCount() > n {
				select {
				case text = <-h.backend.proc(n).sent:
					continue
				default:
				}
			}
			time.Sleep(5 * time.Millisecond)
		}
		if text != h.envelope("follow-up") {
			t.Fatalf("round %d: the follow-up's turn = %q", i, text)
		}
	}
}

func TestClosingALeadClosesItsPiChildren(t *testing.T) {
	h, _ := newFakePiRecoveryHarness(t)
	lead := h.lead(BackendPi, LeadOptions{})
	spawn := func(parent ParentRef) (State, string) {
		st, err := h.runner.Spawn(SpawnSpec{Profile: PiWorkerProfile, Root: h.root, Prompt: "work", Tier: "small", Parent: &parent})
		if err != nil {
			t.Fatal(err)
		}
		return st, h.holder(st.ID)
	}
	child, holder := spawn(ParentRef{Kind: ParentLead, ReplyID: lead.ID})
	other, _ := spawn(leadParent)
	if _, err := h.runner.CloseConversation(lead.ID); err != nil {
		t.Fatal(err)
	}
	if st := h.state(child.ID); st.Status != StatusClosed {
		t.Fatalf("the closed lead's Pi child = %+v", st)
	}
	eventually(t, "the child's holder stopped", func() bool { return h.holders.opCount("stop "+holder) == 1 })
	if st := h.state(other.ID); st.Status == StatusClosed {
		t.Fatalf("another lead's Pi child closed: %+v", st)
	}
}

// A daemon start closes the Pi children an older daemon left open that
// are finished or whose conversation parent is gone, and starts no loop
// for them; any other restores as before.
func TestStartClosesFinishedAndOrphanedPiChildren(t *testing.T) {
	h, got := autoCloseHarness(t)
	fin, p := finishableWorker(t, h)
	p.reply("done")
	eventuallyClosed(t, h, fin.ID)
	orphan := h.spawnWorker("medium")
	h.backend.waitProc(t, 1).nextSent(t)
	running := h.spawnWorker("medium")
	h.backend.waitProc(t, 2).nextSent(t)
	h.runner.Close()

	rewrite := func(id string, edit func(rec *Record)) {
		t.Helper()
		rec := h.record(id)
		edit(&rec)
		if err := writeRecord(wsdashboard.SessionDir(h.dir, id), rec); err != nil {
			t.Fatal(err)
		}
	}
	rewrite(fin.ID, func(rec *Record) { rec.ClosedAt = "" })
	rewrite(orphan.ID, func(rec *Record) { rec.Parent = &ParentRef{Kind: ParentConversation, ReplyID: "nosuchparent"} })
	reported := len(got.lead())
	starts := h.backend.startCount()
	h.start()

	for _, id := range []string{fin.ID, orphan.ID} {
		if h.runner.conv(id) != nil {
			t.Fatalf("%s got a loop", id)
		}
		if st := h.state(id); st.Status != StatusClosed {
			t.Fatalf("%s restored as %+v", id, st)
		}
		if rec := h.record(id); rec.ClosedAt == "" || rec.TurnOpen {
			t.Fatalf("%s record = %+v", id, rec)
		}
		if row, err := wsdashboard.ReadRegistration(h.dir, id); err != nil || row.EndedAt == "" {
			t.Fatalf("%s row = %+v, %v", id, row, err)
		}
	}
	// The running one restored with a loop: it reports its cut-off turn.
	eventually(t, "the cut-off turn's report", func() bool {
		for _, rep := range got.lead()[reported:] {
			if rep.Result.ID == running.ID && rep.Result.Status == TurnInterrupted {
				return true
			}
		}
		return false
	})
	for _, rep := range got.lead()[reported:] {
		if rep.Result.ID != running.ID {
			t.Fatalf("a closed child reported after the start: %+v", rep)
		}
	}
	if n := h.backend.startCount(); n != starts {
		t.Fatalf("the start launched %d processes", n-starts)
	}
}
