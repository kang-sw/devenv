package wsagent

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wsinbox"
)

// reports collects Options.Report's calls.
type reports struct {
	mu  sync.Mutex
	got []TurnReport
}

func (r *reports) add(rep TurnReport) {
	r.mu.Lock()
	r.got = append(r.got, rep)
	r.mu.Unlock()
}

func (r *reports) list() []TurnReport {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]TurnReport(nil), r.got...)
}

// spawnHarness is a harness whose runner reports to the returned reports.
func spawnHarness(t *testing.T) (*harness, *reports) {
	t.Helper()
	h := newHarness(t, map[string]string{"chat": chatProfile})
	got := &reports{}
	h.config.Report = got.add
	h.runner.Close()
	h.start()
	return h, got
}

var leadParent = ParentRef{Kind: ParentLead, ReplyID: "parentreplyid"}

func (h *harness) spawn(prompt string, parent ParentRef) State {
	h.t.Helper()
	st, err := h.runner.Spawn(SpawnSpec{Profile: "chat", Root: h.root, Prompt: prompt, Parent: &parent})
	if err != nil {
		h.t.Fatalf("Spawn: %v", err)
	}
	return st
}

func TestSpawnRecordsParentCwdAndSendsPrompt(t *testing.T) {
	h, _ := spawnHarness(t)
	sub := filepath.Join(h.root, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	st, err := h.runner.Spawn(SpawnSpec{Profile: "chat", Root: h.root, Cwd: sub, Model: "sonnet", Effort: "high", Prompt: "do the thing", Label: "  \n worker one\nmore", Parent: &leadParent})
	if err != nil {
		t.Fatal(err)
	}
	if st.Parent == nil || *st.Parent != leadParent || st.Cwd != sub || st.Label != "worker one" || st.Model != "sonnet" || st.Effort != "high" {
		t.Fatalf("spawned state = %+v", st)
	}
	rec, err := readRecord(wsdashboard.SessionDir(h.dir, st.ID))
	if err != nil || rec.Parent == nil || *rec.Parent != leadParent || rec.Cwd != sub || rec.Lead {
		t.Fatalf("record = %+v, %v", rec, err)
	}
	if got := h.backend.waitProc(t, 0).nextSent(t); got != "do the thing" {
		t.Fatalf("first turn = %q", got)
	}
	if spec := h.backend.start(0); spec.Cwd != sub || spec.Root != h.root || spec.Model != "sonnet" {
		t.Fatalf("the child runs in its cwd on its root: %+v", spec)
	}
	eventually(t, "the turn open on the record", func() bool {
		rec, _ := readRecord(wsdashboard.SessionDir(h.dir, st.ID))
		return rec.TurnOpen
	})

	// Without a cwd the child runs in its root, and the record names none.
	other := h.spawn("again", leadParent)
	if other.Cwd != "" {
		t.Fatalf("a spawn without a cwd records none: %+v", other)
	}
	if spec := h.backend.waitProcStart(t, 1); spec.Cwd != "" {
		t.Fatalf("a spawn without a cwd runs in its root: %+v", spec)
	}
}

// waitProcStart waits for the i-th start and returns its spec.
func (b *fakeBackend) waitProcStart(t *testing.T, i int) StartSpec {
	t.Helper()
	b.waitProc(t, i)
	return b.start(i)
}

func TestSpawnRefusesBadSpec(t *testing.T) {
	h, _ := spawnHarness(t)
	file := filepath.Join(h.root, "agent.md")
	for name, spec := range map[string]SpawnSpec{
		"empty prompt":  {Profile: "chat", Root: h.root, Prompt: "  "},
		"relative cwd":  {Profile: "chat", Root: h.root, Prompt: "x", Cwd: "sub"},
		"missing cwd":   {Profile: "chat", Root: h.root, Prompt: "x", Cwd: filepath.Join(h.root, "absent")},
		"file cwd":      {Profile: "chat", Root: h.root, Prompt: "x", Cwd: file},
		"bad parent":    {Profile: "chat", Root: h.root, Prompt: "x", Parent: &ParentRef{Kind: ParentLead, ReplyID: "not a reply id"}},
		"kindless":      {Profile: "chat", Root: h.root, Prompt: "x", Parent: &ParentRef{ReplyID: "parentreplyid"}},
		"bad model":     {Profile: "chat", Root: h.root, Prompt: "x", Model: "bad model!"},
		"relative root": {Profile: "chat", Root: "rel", Prompt: "x"},
	} {
		if _, err := h.runner.Spawn(spec); requestStatus(err) != http.StatusBadRequest {
			t.Errorf("%s: Spawn = %v, want a 400", name, err)
		}
	}
	if n := len(h.runner.List()); n != 0 {
		t.Fatalf("a refused spawn opens nothing, %d open", n)
	}
}

func TestSpawnReportsSettledTurns(t *testing.T) {
	h, got := spawnHarness(t)
	var seen []State
	var mu sync.Mutex
	stop := h.runner.Watch(func(st State) {
		mu.Lock()
		seen = append(seen, st)
		mu.Unlock()
	})
	defer stop()

	plain := h.create("chat").ID
	h.runner.Send(plain, "no parent")
	h.backend.waitProc(t, 0).nextSent(t)
	h.backend.proc(0).reply("unreported")
	eventually(t, "the plain turn to end", func() bool { return h.state(plain).Status == StatusIdle })

	id := h.spawn("first", leadParent).ID
	p := h.backend.waitProc(t, 1)
	p.nextSent(t)
	p.events <- Event{Live: nil, SessionID: "s-1"}
	p.reply("the answer")
	eventually(t, "one report", func() bool { return len(got.list()) == 1 })
	rep := got.list()[0]
	if rep.Parent != leadParent || rep.Result.ID != id || rep.Result.Status != TurnDone || rep.Result.Message != "the answer" || rep.Child.ID != id {
		t.Fatalf("report = %+v", rep)
	}
	rec, _ := readRecord(wsdashboard.SessionDir(h.dir, id))
	if rec.TurnOpen {
		t.Fatalf("a settled turn is not open on the record")
	}

	// A process lost mid-turn settles the turn as failed.
	h.runner.Send(id, "second")
	p.nextSent(t)
	p.exit()
	eventually(t, "the failure report", func() bool { return len(got.list()) == 2 })
	if rep := got.list()[1]; rep.Result.Status != TurnFailed || rep.Result.Error == "" || rep.Result.Message != "" {
		t.Fatalf("lost process report = %+v", rep)
	}

	// A start that fails settles the turn as failed too.
	h.backend.mu.Lock()
	h.backend.startErr = errNoPi
	h.backend.mu.Unlock()
	h.runner.Send(id, "third")
	eventually(t, "the start failure report", func() bool { return len(got.list()) == 3 })
	if rep := got.list()[2]; rep.Result.Status != TurnFailed || !strings.Contains(rep.Result.Error, errNoPi.Error()) {
		t.Fatalf("start failure report = %+v", rep)
	}

	mu.Lock()
	defer mu.Unlock()
	for _, st := range seen {
		if st.ID == plain {
			t.Fatalf("a conversation without a parent is not watched")
		}
	}
	if len(seen) < 4 {
		t.Fatalf("the watcher should see the spawn, the turns, and the settles: %d states", len(seen))
	}
}

var errNoPi = os.ErrNotExist

func TestWaitWaitsForTheTurnAfterTheLastSend(t *testing.T) {
	h, _ := spawnHarness(t)
	id := h.spawn("first", leadParent).ID
	p := h.backend.waitProc(t, 0)
	p.nextSent(t)

	if res, err := h.runner.Wait(context.Background(), id, 50*time.Millisecond); err != nil || res.Status != TurnRunning {
		t.Fatalf("a wait that times out reports running: %+v, %v", res, err)
	}
	done := make(chan TurnResult, 1)
	go func() {
		res, _ := h.runner.Wait(context.Background(), id, 5*time.Second)
		done <- res
	}()
	p.reply("one")
	if res := <-done; res.Status != TurnDone || res.Message != "one" {
		t.Fatalf("the wait returns the settled turn: %+v", res)
	}
	if res, _ := h.runner.Wait(context.Background(), id, time.Second); res.Message != "one" {
		t.Fatalf("a turn settled before the wait returns at once: %+v", res)
	}

	h.runner.Send(id, "second")
	if res, _ := h.runner.Wait(context.Background(), id, 100*time.Millisecond); res.Status != TurnRunning {
		t.Fatalf("after a send the wait must not return the previous turn: %+v", res)
	}
	p.nextSent(t)
	// A send held during the turn: the wait needs the turn that takes it.
	h.runner.Send(id, "third")
	go func() {
		res, _ := h.runner.Wait(context.Background(), id, 5*time.Second)
		done <- res
	}()
	p.reply("two")
	select {
	case res := <-done:
		t.Fatalf("the turn before the held send answered the wait: %+v", res)
	case <-time.After(100 * time.Millisecond):
	}
	if got := p.nextSent(t); got != "third" {
		t.Fatalf("the held send's turn = %q", got)
	}
	p.reply("three")
	if res := <-done; res.Message != "three" {
		t.Fatalf("the wait returns the held send's turn: %+v", res)
	}

	ctx, cancel := context.WithCancel(context.Background())
	h.runner.Send(id, "fourth")
	cancel()
	if res, _ := h.runner.Wait(ctx, id, 5*time.Second); res.Status != TurnRunning {
		t.Fatalf("an ended context ends the wait as running: %+v", res)
	}
	if _, err := h.runner.Wait(context.Background(), "nosuchagent", time.Second); err == nil {
		t.Fatalf("a wait on an unknown id fails")
	}
}

func TestCloseReportsNothingAndEndsWaits(t *testing.T) {
	h, got := spawnHarness(t)
	id := h.spawn("first", leadParent).ID
	p := h.backend.waitProc(t, 0)
	p.nextSent(t)
	done := make(chan TurnResult, 1)
	go func() {
		res, _ := h.runner.Wait(context.Background(), id, 5*time.Second)
		done <- res
	}()
	time.Sleep(20 * time.Millisecond)
	if _, err := h.runner.CloseConversation(id); err != nil {
		t.Fatal(err)
	}
	if res := <-done; res.Status != TurnClosed {
		t.Fatalf("a close ends a wait as closed: %+v", res)
	}
	time.Sleep(50 * time.Millisecond)
	if n := len(got.list()); n != 0 {
		t.Fatalf("a closed conversation reports nothing: %+v", got.list())
	}
	if res, _ := h.runner.Wait(context.Background(), id, time.Second); res.Status != TurnClosed {
		t.Fatalf("a wait on a closed conversation: %+v", res)
	}
	if st := h.runner.Children(leadParent); len(st) != 1 || st[0].Status != StatusClosed || st[0].Parent == nil {
		t.Fatalf("a closed child is still listed with its parent: %+v", st)
	}
	if _, err := h.runner.Transcript(id); err != nil {
		t.Fatalf("a closed child's transcript stays readable: %v", err)
	}
}

func TestDaemonExitReportsTheCutTurnAsInterrupted(t *testing.T) {
	h, got := spawnHarness(t)
	id := h.spawn("first", leadParent).ID
	h.backend.waitProc(t, 0).nextSent(t)
	eventually(t, "the turn open on the record", func() bool {
		rec, _ := readRecord(wsdashboard.SessionDir(h.dir, id))
		return rec.TurnOpen
	})
	h.runner.Close()
	if n := len(got.list()); n != 0 {
		t.Fatalf("the exiting daemon reports nothing: %+v", got.list())
	}

	h.start()
	eventually(t, "the interrupted report", func() bool { return len(got.list()) == 1 })
	rep := got.list()[0]
	if rep.Result.Status != TurnInterrupted || rep.Result.Error == "" || rep.Parent != leadParent || rep.Result.ID != id {
		t.Fatalf("report = %+v", rep)
	}
	rec, _ := readRecord(wsdashboard.SessionDir(h.dir, id))
	if rec.TurnOpen {
		t.Fatalf("the reported turn is no longer open")
	}
	h.runner.Close()
	h.start()
	time.Sleep(50 * time.Millisecond)
	if n := len(got.list()); n != 1 {
		t.Fatalf("a turn is reported once: %+v", got.list())
	}
}

func TestChildrenAreScopedToTheParent(t *testing.T) {
	h, _ := spawnHarness(t)
	other := ParentRef{Kind: ParentLead, ReplyID: "otherreplyid"}
	a := h.spawn("one", leadParent).ID
	b := h.spawn("two", leadParent).ID
	h.spawn("three", other)
	h.create("chat")
	var ids []string
	for _, st := range h.runner.Children(leadParent) {
		ids = append(ids, st.ID)
	}
	if len(ids) != 2 || !(ids[0] == a && ids[1] == b || ids[0] == b && ids[1] == a) {
		t.Fatalf("Children = %v, want %s and %s", ids, a, b)
	}
}

func TestVersionAtLeast(t *testing.T) {
	for v, want := range map[string]bool{"1.0.4": true, "v1.0": true, "2.3.1": true, "1.1.0": true, "0.9.9": false, "0.72.1": false, "": false, "pi": false} {
		if got := VersionAtLeast(v, 1, 0); got != want {
			t.Errorf("VersionAtLeast(%q, 1, 0) = %v", v, got)
		}
	}
}

func TestPiVersionRunsPi(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("a shell script stands in for pi")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "pi"), []byte("#!/bin/sh\necho \"pi 1.0.4\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	got, err := PiVersion(context.Background())
	if err != nil || got != "1.0.4" {
		t.Fatalf("PiVersion = %q, %v", got, err)
	}
	t.Setenv("PATH", t.TempDir())
	if _, err := PiVersion(context.Background()); err == nil {
		t.Fatalf("a missing pi fails")
	}
}

// TestCommandTurnDoesNotAnswerAWaitForTheTextBehindIt: a leading slash
// command drained with a sent message runs as its own turn, which covers
// none of that message: the wait for it takes the text's turn.
func TestCommandTurnDoesNotAnswerAWaitForTheTextBehindIt(t *testing.T) {
	h, _ := spawnHarness(t)
	id := h.spawn("first", leadParent).ID
	p := h.backend.waitProc(t, 0)
	p.nextSent(t)
	p.reply("one")
	eventually(t, "the first turn settled", func() bool { return h.state(id).Status == StatusIdle })

	inbox := wsinbox.Dir(wsdashboard.SessionDir(h.dir, id))
	if _, _, err := wsinbox.Send(inbox, wsinbox.Entry{Command: &wsinbox.Command{Name: "compact"}}, time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := h.runner.Send(id, "the text"); err != nil {
		t.Fatal(err)
	}
	done := make(chan TurnResult, 1)
	go func() {
		res, _ := h.runner.Wait(context.Background(), id, 5*time.Second)
		done <- res
	}()
	if got := p.nextSent(t); got != "/compact" {
		t.Fatalf("the command's turn = %q", got)
	}
	p.reply("compacted")
	select {
	case res := <-done:
		t.Fatalf("the command's turn answered the wait for the text: %+v", res)
	case <-time.After(100 * time.Millisecond):
	}
	if got := p.nextSent(t); got != "the text" {
		t.Fatalf("the text's turn = %q", got)
	}
	p.reply("answered")
	if res := <-done; res.Message != "answered" {
		t.Fatalf("the wait takes the text's turn: %+v", res)
	}
}
