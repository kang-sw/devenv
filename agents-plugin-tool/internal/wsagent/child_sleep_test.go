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

// A finished spawned child does not close: it stays open after its report
// reached its parent, its process ends on its idle timer (sleep), and the
// next message starts it again on the same id (agent_tools.go).

// shortIdle gives the builtin profiles named a one-second idle timeout
// through the working root's project config, keeping what that file
// already holds.
func (h *harness) shortIdle(names ...string) {
	h.t.Helper()
	path := filepath.Join(h.root, ".ws-workflow", "config.json")
	var file struct {
		Overrides map[string]json.RawMessage `json:"structured_overrides"`
	}
	if raw, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(raw, &file); err != nil {
			h.t.Fatal(err)
		}
	}
	values := map[string]string{}
	for k, v := range file.Overrides {
		values[k] = string(v)
	}
	builtins := BuiltinProfiles()
	for _, name := range names {
		var p Profile
		if err := json.Unmarshal(builtins[ProfilePrefix+name], &p); err != nil {
			h.t.Fatalf("builtin profile %s: %v", name, err)
		}
		secs := 1
		p.IdleTimeoutSeconds = &secs
		raw, err := json.Marshal(p)
		if err != nil {
			h.t.Fatal(err)
		}
		values[ProfilePrefix+name] = string(raw)
	}
	writeStructured(h.t, path, values)
}

// finishableWorker spawns a lead's Pi worker with a session file a Send
// can resume it on, and returns it with its process, first turn sent.
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

// stillOpen fails when id closed within a short while.
func stillOpen(t *testing.T, h *harness, id, why string) {
	t.Helper()
	time.Sleep(100 * time.Millisecond)
	if h.state(id).Status == StatusClosed {
		t.Fatalf("%s: %s closed", why, id)
	}
}

// eventuallyAsleep waits for id's process to end with id still open and
// waiting (SessionChildWaiting).
func eventuallyAsleep(t *testing.T, h *harness, id string) {
	t.Helper()
	eventually(t, id+" asleep", func() bool {
		st := h.state(id)
		return !st.Process && st.Status == StatusIdle && st.Lifecycle == wsdashboard.SessionChildWaiting
	})
}

func TestFinishedPiWorkerSleepsAfterItsReportAndSendWakesIt(t *testing.T) {
	h, got := childHarness(t)
	h.shortIdle(PiWorkerProfile)
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
	eventually(t, "the lead's report", func() bool { return len(got.lead()) == 1 })
	stillOpen(t, h, w.ID, "a finished worker whose report reached its parent")
	// The idle timer ends the process; the conversation stays open.
	eventually(t, "the idle process to stop", p.isClosed)
	eventuallyAsleep(t, h, w.ID)
	if rec := h.record(w.ID); rec.ClosedAt != "" || rec.TurnOpen {
		t.Fatalf("the sleeping worker's record = %+v", rec)
	}
	if reps := got.lead(); len(reps) != 1 || reps[0].Result.Message != "result" {
		t.Fatalf("the lead's report = %+v", reps)
	}
	// A wait while it sleeps still answers with the reported turn.
	if res, err := h.runner.Wait(context.Background(), w.ID, time.Second); err != nil || res.Status != TurnDone || res.Message != "result" {
		t.Fatalf("a wait made while it sleeps = %+v, %v", res, err)
	}

	st, err := h.runner.Send(w.ID, "more")
	if err != nil || st.Status == StatusClosed || st.ID != w.ID {
		t.Fatalf("Send to the sleeping worker = %+v, %v", st, err)
	}
	p2 := h.backend.waitProc(t, 1)
	if text := p2.nextSent(t); text != h.envelope("more") {
		t.Fatalf("the woken turn = %q", text)
	}
	if res, _ := h.runner.Wait(context.Background(), w.ID, 50*time.Millisecond); res.Status != TurnRunning {
		t.Fatalf("a wait for the new message while it runs = %+v", res)
	}
	p2.reply("second")
	eventually(t, "the second report", func() bool { return len(got.lead()) == 2 })
	if reps := got.lead(); reps[1].Result.Message != "second" || reps[1].Result.ID != w.ID {
		t.Fatalf("the lead's reports = %+v", reps)
	}
	eventuallyAsleep(t, h, w.ID)
}

// Revival admits its first message under the same fence as explicit close.
func TestPiRevivalFencesMessageAdmission(t *testing.T) {
	h, _ := childHarness(t)
	w, _ := finishableWorker(t, h)
	if _, err := h.runner.CloseConversation(w.ID); err != nil {
		t.Fatal(err)
	}
	h.runner.Close()
	admissions := 0
	h.config.sendAdmitting = func(id string) {
		if id == w.ID {
			admissions++
			if h.runner.resumeMu.TryLock() {
				h.runner.resumeMu.Unlock()
				t.Error("revival admitted a message outside the close fence")
			}
		}
	}
	h.start()
	starts := h.backend.startCount()
	if _, err := h.runner.Send(w.ID, "follow-up"); err != nil {
		t.Fatal(err)
	}
	if admissions != 1 {
		t.Fatalf("revival needed %d admissions, want 1", admissions)
	}
	if text := h.backend.waitProc(t, starts).nextSent(t); text != h.envelope("follow-up") {
		t.Fatalf("revived turn = %q", text)
	}
}

// A child whose report its parent has not taken yet keeps it, however long
// the parent's loop is busy; once the parent takes it, the child stays open.
func TestPiChildReportReachesItsBusyParentAndTheChildStaysOpen(t *testing.T) {
	h, _ := childHarness(t)
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
	eventually(t, "the report taken", func() bool { return len(h.record(gc.ID).PendingReports) == 0 })
	eventually(t, "the report steered into the worker's turn", func() bool {
		steered, _ := wp.steered()
		return len(steered) == 1 && strings.Contains(steered[0], "child result")
	})
	stillOpen(t, h, gc.ID, "a finished child whose report was taken")
	if h.state(w.ID).Status == StatusClosed {
		t.Fatal("a worker whose turn runs closed")
	}
	// A wait after the handover says the report reached the worker.
	code, body := h.tokenCall(token, "wait", map[string]any{"id": gc.ID, "timeout_s": 1})
	var res TurnResult
	if code != http.StatusOK || json.Unmarshal([]byte(body), &res) != nil || res.Status != TurnDone || !res.Delivered || res.Message != "" {
		t.Fatalf("a wait after the handover = %d %s", code, body)
	}
}

// A failed turn leaves its worker open: its failure stays on its row, and
// its parent may send it on.
func TestFailedPiWorkerStaysOpen(t *testing.T) {
	h, got := childHarness(t)
	w, p := finishableWorker(t, h)
	p.events <- Event{TurnEnd: true, TurnError: "boom"}
	eventually(t, "the failure report", func() bool { return len(got.lead()) == 1 })
	stillOpen(t, h, w.ID, "a failed worker")
	if st := h.state(w.ID); st.Error == "" {
		t.Fatalf("the failed worker's state = %+v", st)
	}
}

// A message in the inbox, even one the page wrote whose signal the loop
// has not taken, starts a finished worker's next turn.
func TestHeldMessageStartsAFinishedWorkersNextTurn(t *testing.T) {
	h, got := childHarness(t)
	w, p := finishableWorker(t, h)
	c := h.runner.conv(w.ID)
	if _, _, err := wsinbox.Send(c.inbox, wsinbox.Entry{Parts: []string{"page message"}}, h.config.Now()); err != nil {
		t.Fatal(err)
	}
	p.reply("first")
	h.runner.InboxChanged(w.ID)
	if text := p.nextSent(t); !strings.Contains(text, "page message") {
		t.Fatalf("the held message's turn = %q", text)
	}
	p.reply("second")
	eventually(t, "both reports", func() bool { return len(got.lead()) == 2 })
	if reps := got.lead(); reps[1].Result.Message != "second" {
		t.Fatalf("the lead's reports = %+v", reps)
	}
	stillOpen(t, h, w.ID, "a finished worker")
}

// Report-hold under sleep: a worker whose done turn is held for its open
// child stays awake past its idle timeout and reports nothing; the child's
// report starts the worker's next turn, and only that turn is reported.
// Both then sleep, open.
func TestHeldTurnKeepsAWorkerAwakeUntilItsChildReports(t *testing.T) {
	h, got := childHarness(t)
	h.shortIdle(PiWorkerProfile)
	w, wp, _, gc, gp := workerWithChild(t, h)
	wp.reply("interim")
	eventually(t, "the held turn", func() bool { return h.record(w.ID).HeldTurn != nil })
	// Past the one-second idle timeout: the held worker keeps its process.
	time.Sleep(1500 * time.Millisecond)
	if wp.isClosed() || !h.state(w.ID).Process {
		t.Fatal("a worker whose turn is held for its child slept")
	}
	noLeadReport(t, got, "a worker whose turn is held for its child")
	gp.reply("child result")
	if text := wp.nextSent(t); !strings.Contains(text, "child result") {
		t.Fatalf("the report's turn = %q", text)
	}
	wp.reply("final")
	eventually(t, "the worker's report", func() bool { return len(got.lead()) == 1 })
	if rep := got.lead()[0]; rep.Result.Message != "final" {
		t.Fatalf("the reported turn = %+v", rep.Result)
	}
	eventuallyAsleep(t, h, gc.ID)
	eventuallyAsleep(t, h, w.ID)
}

// A parent's pi.wait made before the child finishes takes the result; one
// made after the child fell asleep answers with it again.
func TestParentWaitAcrossTheChildsSleep(t *testing.T) {
	h, _ := childHarness(t)
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
	if _, err := h.runner.Stop(gc.ID); err != nil {
		t.Fatal(err)
	}
	eventuallyAsleep(t, h, gc.ID)
	code, body := h.tokenCall(token, "wait", map[string]any{"id": gc.ID, "timeout_s": 1})
	res = TurnResult{}
	if code != http.StatusOK || json.Unmarshal([]byte(body), &res) != nil || res.Status != TurnDone || res.Delivered || res.Message != "child result" {
		t.Fatalf("a wait made while the child sleeps = %d %s", code, body)
	}
}

// A message sent as the worker finishes is never lost: it reaches the
// worker's process, and no path deadlocks.
func TestSendRacingAFinishIsNeverLost(t *testing.T) {
	h, _ := childHarness(t)
	for i := range 10 {
		w, p := finishableWorker(t, h)
		n := h.backend.startCount()
		p.reply("done")
		// Sent mid-turn it would be steered, which this fake leaves
		// unanswered; the race is with the turn's end.
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

// A lead close can finish its descendant snapshot while a ParentLead
// spawn is still minting its key. Releasing that spawn only after the close
// forces registration past the snapshot, without timing-dependent sleeps.
func TestLeadCloseFencesConcurrentPiSpawn(t *testing.T) {
	h, _ := newFakePiRecoveryHarness(t)
	lead := h.lead(BackendPi, LeadOptions{})
	parent := ParentRef{Kind: ParentLead, ReplyID: lead.ID}
	mint := h.opts.Keys.(*fakeKeys).gateMint(t, h.root)
	type result struct {
		st  State
		err error
	}
	done := make(chan result, 1)
	go func() {
		st, err := h.runner.Spawn(SpawnSpec{Profile: PiWorkerProfile, Root: h.root, Prompt: "work", Tier: "small", Parent: &parent})
		done <- result{st, err}
	}()
	mint.awaitEntered(t)
	if _, err := h.runner.CloseConversation(lead.ID); err != nil {
		t.Fatal(err)
	}
	mint.release()
	select {
	case got := <-done:
		if got.err != errClosed {
			t.Fatalf("spawn registered after lead close = %+v, %v; want errClosed", got.st, got.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("spawn did not finish after lead close")
	}
	for _, child := range h.runner.Children(parent) {
		if child.Status != StatusClosed {
			t.Fatalf("closed lead left a live Pi child: %+v", child)
		}
		if c := h.runner.conv(child.ID); c != nil {
			t.Fatalf("closed lead left a child loop registered: %s", child.ID)
		}
	}
}

// A daemon start closes the Pi children an older daemon left open whose
// conversation parent is gone, and starts no loop for them; a finished
// child restores open and asleep, and a running one as before.
func TestStartClosesOrphanedPiChildrenButNotFinishedOnes(t *testing.T) {
	h, got := childHarness(t)
	fin, p := finishableWorker(t, h)
	p.reply("done")
	eventually(t, "the finished worker's report", func() bool { return len(got.lead()) == 1 })
	orphan := h.spawnWorker("medium")
	h.backend.waitProc(t, 1).nextSent(t)
	running := h.spawnWorker("medium")
	h.backend.waitProc(t, 2).nextSent(t)
	h.runner.Close()

	rec := h.record(orphan.ID)
	rec.Parent = &ParentRef{Kind: ParentConversation, ReplyID: "nosuchparent"}
	if err := writeRecord(wsdashboard.SessionDir(h.dir, orphan.ID), rec); err != nil {
		t.Fatal(err)
	}
	reported := len(got.lead())
	starts := h.backend.startCount()
	h.start()

	if h.runner.conv(orphan.ID) != nil {
		t.Fatal("the orphan got a loop")
	}
	if st := h.state(orphan.ID); st.Status != StatusClosed {
		t.Fatalf("the orphan restored as %+v", st)
	}
	if rec := h.record(orphan.ID); rec.ClosedAt == "" || rec.TurnOpen {
		t.Fatalf("the orphan's record = %+v", rec)
	}
	if row, err := wsdashboard.ReadRegistration(h.dir, orphan.ID); err != nil || row.EndedAt == "" {
		t.Fatalf("the orphan's row = %+v, %v", row, err)
	}
	// The finished one restores open, with a loop and no process.
	if h.runner.conv(fin.ID) == nil {
		t.Fatal("the finished worker got no loop")
	}
	if st := h.state(fin.ID); st.Status != StatusIdle || st.Process || st.Lifecycle != wsdashboard.SessionChildWaiting {
		t.Fatalf("the finished worker restored as %+v", st)
	}
	if rec := h.record(fin.ID); rec.ClosedAt != "" {
		t.Fatalf("the finished worker's record = %+v", rec)
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
			t.Fatalf("another child reported after the start: %+v", rep)
		}
	}
	if n := h.backend.startCount(); n != starts {
		t.Fatalf("the start launched %d processes", n-starts)
	}
}
