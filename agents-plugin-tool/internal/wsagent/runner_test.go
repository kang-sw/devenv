package wsagent

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wslive"
)

const chatProfile = `{"backend":"claude","model":"haiku","tools":["Read"],"instructions":"agent.md"}`

func requestStatus(err error) int {
	var re *requestError
	if errors.As(err, &re) {
		return re.status
	}
	return 0
}

func TestCreateWritesRecordRegistrationAndSnapshot(t *testing.T) {
	h := newHarness(t, map[string]string{"chat": chatProfile})
	st := h.create("chat")
	if st.Status != StatusIdle || st.Process || st.Model != "haiku" || st.Page != "/s/"+st.ID {
		t.Fatalf("a new conversation is idle with no process: %+v", st)
	}
	dir := wsdashboard.SessionDir(h.dir, st.ID)
	info, err := os.Stat(recordPath(dir))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("the record should be private: %v %v", info, err)
	}
	rec, err := readRecord(dir)
	if err != nil || rec.WSKey == "" || rec.Profile != "chat" || rec.Backend != suiteBackend || rec.Root != h.root {
		t.Fatalf("record = %+v, %v", rec, err)
	}
	reg, err := wsdashboard.ReadRegistration(h.dir, st.ID)
	if err != nil || !reg.Agent || !reg.Unlisted || reg.Project != h.root || !reg.Live(time.Now()) {
		t.Fatalf("registration should mark a live, unlisted agent session: %+v, %v", reg, err)
	}
	if !wslive.AdapterPresent(wslive.SnapshotPath(dir), time.Now()) {
		t.Fatalf("an open conversation's snapshot should name its adapter")
	}
	if h.backend.startCount() != 0 {
		t.Fatalf("create should not start a process")
	}
}

func TestCreateRefusesBadLaunch(t *testing.T) {
	h := newHarness(t, map[string]string{
		"other":   `{"backend":"@other"}`,
		"noinstr": `{"backend":"claude","instructions":"missing.md"}`,
	})
	for profile, want := range map[string]string{"absent": "unknown agent profile", "other": fmt.Sprintf("backend %q is not available", otherBackend()), "noinstr": "instruction file"} {
		_, err := h.runner.Create(profile, h.root)
		if err == nil || requestStatus(err) != http.StatusBadRequest || !strings.Contains(err.Error(), want) {
			t.Errorf("Create(%s) = %v, want a 400 naming %q", profile, err, want)
		}
	}
	if _, err := h.runner.Create("noinstr", "relative/root"); requestStatus(err) != http.StatusBadRequest {
		t.Errorf("a relative root should be refused, got %v", err)
	}
}

func TestSendRunsTurnAndHeldSendsMerge(t *testing.T) {
	h := newHarness(t, map[string]string{"chat": chatProfile})
	id := h.create("chat").ID
	if _, err := h.runner.Send(id, "first"); err != nil {
		t.Fatal(err)
	}
	eventually(t, "process start", func() bool { return h.backend.startCount() == 1 })
	p := h.backend.proc(0)
	if got := p.nextSent(t); got != "first" {
		t.Fatalf("first turn = %q", got)
	}
	spec := h.backend.start(0)
	if spec.Root != h.root || spec.Model != "haiku" || spec.SessionID != "" || spec.WSMCP != nil {
		t.Fatalf("start spec = %+v", spec)
	}
	instr, err := os.ReadFile(spec.Instructions)
	if err != nil || !strings.Contains(string(instr), "Answer questions") {
		t.Fatalf("the composed instructions should hold the profile's file: %q, %v", instr, err)
	}
	if st := h.state(id); st.Status != StatusRunning || !st.Process {
		t.Fatalf("a sent turn runs: %+v", st)
	}

	for _, text := range []string{"second", "third"} {
		st, err := h.runner.Send(id, text)
		if err != nil {
			t.Fatal(err)
		}
		if st.Status != StatusRunning {
			t.Fatalf("a send during a turn is held, the turn keeps running: %+v", st)
		}
	}
	if st := h.state(id); st.Held != 1 {
		t.Fatalf("two held sends should merge into one held message, held %d", st.Held)
	}
	p.events <- Event{SessionID: "backend-1"}
	p.reply("answer one")
	if got := p.nextSent(t); got != "second\n\nthird" {
		t.Fatalf("the held sends should be the next turn as one message, got %q", got)
	}
	p.reply("answer two")
	eventually(t, "idle", func() bool { return h.state(id).Status == StatusIdle })

	ring, err := wslive.ReadRing(wslive.RingDir(wsdashboard.SessionDir(h.dir, id)))
	if err != nil {
		t.Fatal(err)
	}
	var texts []string
	for _, r := range ring {
		if r.Kind == wslive.RingUser || r.Kind == wslive.RingReply {
			texts = append(texts, r.Kind+":"+r.Text)
		}
	}
	want := "user:first reply:answer one user:second\n\nthird reply:answer two"
	if strings.Join(texts, " ") != want {
		t.Fatalf("ring = %q, want %q", strings.Join(texts, " "), want)
	}
	rec, _ := readRecord(wsdashboard.SessionDir(h.dir, id))
	if rec.SessionID != "backend-1" {
		t.Fatalf("the backend session id should be persisted, got %q", rec.SessionID)
	}
}

func TestIdleTimerEndsProcessAndNextSendResumes(t *testing.T) {
	h := newHarness(t, map[string]string{"chat": `{"backend":"claude","idle_timeout_seconds":1}`})
	id := h.create("chat").ID
	h.runner.Send(id, "hello")
	eventually(t, "start", func() bool { return h.backend.startCount() == 1 })
	p := h.backend.proc(0)
	p.nextSent(t)
	p.events <- Event{SessionID: "backend-1"}
	p.reply("hi")
	eventually(t, "idle exit", func() bool { return p.isClosed() && !h.state(id).Process })

	h.runner.Send(id, "again")
	eventually(t, "resume", func() bool { return h.backend.startCount() == 2 })
	if got := h.backend.start(1).SessionID; got != "backend-1" {
		t.Fatalf("the next send should resume the backend session, got %q", got)
	}
	if got := h.backend.proc(1).nextSent(t); got != "again" {
		t.Fatalf("resumed turn = %q", got)
	}
}

func TestIdleTimeoutZeroNeverExits(t *testing.T) {
	h := newHarness(t, map[string]string{"chat": `{"backend":"claude","idle_timeout_seconds":0}`})
	id := h.create("chat").ID
	h.runner.Send(id, "hello")
	eventually(t, "start", func() bool { return h.backend.startCount() == 1 })
	p := h.backend.proc(0)
	p.nextSent(t)
	p.reply("hi")
	time.Sleep(1200 * time.Millisecond)
	if p.isClosed() || !h.state(id).Process {
		t.Fatalf("idle_timeout_seconds 0 should keep the process")
	}
}

func TestRecordSurvivesDaemonRestart(t *testing.T) {
	h := newHarness(t, map[string]string{"chat": `{"backend":"claude","ws_tools":["tickets.query"]}`})
	id := h.create("chat").ID
	h.runner.Send(id, "hello")
	eventually(t, "start", func() bool { return h.backend.startCount() == 1 })
	p := h.backend.proc(0)
	p.nextSent(t)
	p.events <- Event{SessionID: "backend-7"}
	eventually(t, "session id", func() bool {
		rec, _ := readRecord(wsdashboard.SessionDir(h.dir, id))
		return rec.SessionID == "backend-7"
	})
	// A held send survives the restart too.
	h.runner.Send(id, "held over")
	h.runner.SetSettings(id, ptr("sonnet"), nil)
	h.runner.Close()
	if !p.isClosed() {
		t.Fatalf("the daemon's shutdown should end the process")
	}

	h.start()
	st := h.state(id)
	if st.Status != StatusIdle || st.Process || st.Held != 1 || st.Model != "sonnet" {
		t.Fatalf("after a restart the conversation is open, idle, processless, still holding its send: %+v", st)
	}
	if h.backend.startCount() != 1 {
		t.Fatalf("a restart must not start processes (lazy resume)")
	}
	h.runner.Send(id, "new")
	eventually(t, "resume", func() bool { return h.backend.startCount() == 2 })
	spec := h.backend.start(1)
	if spec.SessionID != "backend-7" || spec.Model != "sonnet" {
		t.Fatalf("the resumed process should carry the recorded session and model: %+v", spec)
	}
	if got := h.backend.proc(1).nextSent(t); got != "held over\n\nnew" {
		t.Fatalf("the held send should merge with the next one, got %q", got)
	}
	rec, _ := readRecord(wsdashboard.SessionDir(h.dir, id))
	instr, _ := os.ReadFile(spec.Instructions)
	if !strings.Contains(string(instr), rec.WSKey) {
		t.Fatalf("the agent should get its ws session key in its instructions")
	}
	if spec.WSMCP == nil || spec.WSMCP.Env[EnvAllowedTools] != "tickets.query" || spec.WSMCP.Env[EnvNoDashboard] != "1" {
		t.Fatalf("ws-mcp should carry the allowlist and the dashboard off: %+v", spec.WSMCP)
	}
	for _, name := range []string{"WS_MAILBOX", "WS_MAILBOX_AUTO"} {
		if v, ok := spec.WSMCP.Env[name]; !ok || v != "" {
			t.Fatalf("ws-mcp must not inherit a mailbox identity: %s=%q (set %v)", name, v, ok)
		}
	}
	if len(h.keys.ensured) != 2 || h.keys.ensured[1] != rec.WSKey {
		t.Fatalf("every start should keep the same key usable: %v", h.keys.ensured)
	}
}

func TestStopKeepsHeldAndResumes(t *testing.T) {
	h := newHarness(t, map[string]string{"chat": chatProfile})
	id := h.create("chat").ID
	h.runner.Send(id, "one")
	eventually(t, "start", func() bool { return h.backend.startCount() == 1 })
	p := h.backend.proc(0)
	p.nextSent(t)
	p.events <- Event{SessionID: "s1"}
	h.runner.Send(id, "two")
	st, err := h.runner.Stop(id)
	if err != nil {
		t.Fatal(err)
	}
	if !p.isClosed() || st.Process || st.Status != StatusIdle || st.Held != 1 {
		t.Fatalf("stop ends the process and the turn, keeping the held send: %+v", st)
	}
	h.runner.Send(id, "three")
	eventually(t, "resume", func() bool { return h.backend.startCount() == 2 })
	if h.backend.start(1).SessionID != "s1" {
		t.Fatalf("after stop the next send resumes the session")
	}
	if got := h.backend.proc(1).nextSent(t); got != "two\n\nthree" {
		t.Fatalf("got %q", got)
	}
}

func TestCloseEndsConversationForGood(t *testing.T) {
	h := newHarness(t, map[string]string{"chat": chatProfile})
	id := h.create("chat").ID
	h.runner.Send(id, "one")
	eventually(t, "start", func() bool { return h.backend.startCount() == 1 })
	p := h.backend.proc(0)
	p.nextSent(t)
	h.runner.Send(id, "held")
	st, err := h.runner.CloseConversation(id)
	if err != nil {
		t.Fatal(err)
	}
	if st.Status != StatusClosed || st.ClosedAt == "" || !p.isClosed() || st.Held != 0 {
		t.Fatalf("close ends the process and drops held sends: %+v", st)
	}
	if _, err := h.runner.Send(id, "more"); requestStatus(err) != http.StatusConflict {
		t.Fatalf("a send to a closed conversation is refused, got %v", err)
	}
	dir := wsdashboard.SessionDir(h.dir, id)
	if ring, err := wslive.ReadRing(wslive.RingDir(dir)); err != nil || len(ring) == 0 {
		t.Fatalf("the closed conversation's ring stays readable: %v", err)
	}
	if reg, _ := wsdashboard.ReadRegistration(h.dir, id); reg.EndedAt == "" {
		t.Fatalf("the registration should be ended")
	}
	if wslive.AdapterPresent(wslive.SnapshotPath(dir), time.Now()) {
		t.Fatalf("a closed conversation's page should refuse input")
	}

	h.runner.Close()
	h.start()
	if st := h.state(id); st.Status != StatusClosed {
		t.Fatalf("closed stays closed across a restart: %+v", st)
	}
	if _, err := h.runner.Send(id, "more"); requestStatus(err) != http.StatusConflict {
		t.Fatalf("got %v", err)
	}
	if len(h.runner.List()) != 1 {
		t.Fatalf("List should still report the closed conversation")
	}
}

func TestInterruptTakesBackHeldAndStopsTurn(t *testing.T) {
	h := newHarness(t, map[string]string{"chat": chatProfile})
	id := h.create("chat").ID
	h.runner.Send(id, "one")
	eventually(t, "start", func() bool { return h.backend.startCount() == 1 })
	p := h.backend.proc(0)
	p.nextSent(t)
	h.runner.Send(id, "held")
	st, returned, err := h.runner.Interrupt(id)
	if err != nil {
		t.Fatal(err)
	}
	if len(returned) != 1 || returned[0] != "held" || st.Held != 0 {
		t.Fatalf("interrupt returns the held text: %v %+v", returned, st)
	}
	p.mu.Lock()
	n := p.interrupts
	p.mu.Unlock()
	if n != 1 {
		t.Fatalf("the running turn should be interrupted once, got %d", n)
	}
	p.events <- Event{TurnEnd: true}
	eventually(t, "idle", func() bool { return h.state(id).Status == StatusIdle })
	select {
	case s := <-p.sent:
		t.Fatalf("nothing should be submitted after an interrupt, got %q", s)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestSettingsApplyAtTurnBoundary(t *testing.T) {
	h := newHarness(t, map[string]string{"chat": chatProfile})
	id := h.create("chat").ID
	h.runner.Send(id, "one")
	eventually(t, "start", func() bool { return h.backend.startCount() == 1 })
	p := h.backend.proc(0)
	p.nextSent(t)
	st, err := h.runner.SetSettings(id, ptr("sonnet"), nil)
	if err != nil || st.Model != "sonnet" {
		t.Fatalf("SetSettings: %+v %v", st, err)
	}
	p.mu.Lock()
	early := len(p.models)
	p.mu.Unlock()
	if early != 0 {
		t.Fatalf("a model change must wait for the turn boundary")
	}
	p.reply("done")
	eventually(t, "in-stream model", func() bool {
		p.mu.Lock()
		defer p.mu.Unlock()
		return len(p.models) == 1 && p.models[0] == "sonnet"
	})
	if p.isClosed() {
		t.Fatalf("an in-stream model change keeps the process")
	}

	// Effort has no in-stream control: the process restarts on the session.
	if _, err := h.runner.SetSettings(id, nil, ptr("high")); err != nil {
		t.Fatal(err)
	}
	eventually(t, "restart for effort", func() bool { return p.isClosed() })
	h.runner.Send(id, "two")
	eventually(t, "restart", func() bool { return h.backend.startCount() == 2 })
	if spec := h.backend.start(1); spec.Effort != "high" || spec.Model != "sonnet" {
		t.Fatalf("the restarted process carries the new settings: %+v", spec)
	}
	if _, err := h.runner.SetSettings(id, ptr("bad model!"), nil); requestStatus(err) != http.StatusBadRequest {
		t.Fatalf("a malformed model is refused, got %v", err)
	}
}

func TestFailuresSurfaceInState(t *testing.T) {
	h := newHarness(t, map[string]string{"chat": chatProfile})
	id := h.create("chat").ID
	h.backend.mu.Lock()
	h.backend.startErr = errors.New("claude not found")
	h.backend.mu.Unlock()
	h.runner.Send(id, "one")
	eventually(t, "error", func() bool { return strings.Contains(h.state(id).Error, "claude not found") })
	if st := h.state(id); st.Held != 1 || st.Status != StatusIdle {
		t.Fatalf("a failed start keeps the message held: %+v", st)
	}

	h.backend.mu.Lock()
	h.backend.startErr = nil
	h.backend.mu.Unlock()
	h.runner.Send(id, "two")
	eventually(t, "start", func() bool { return h.backend.startCount() == 1 })
	p := h.backend.proc(0)
	if got := p.nextSent(t); got != "one\n\ntwo" {
		t.Fatalf("got %q", got)
	}
	p.events <- Event{Fatal: errors.New("launch refused: claude reports tools outside the profile: Write")}
	eventually(t, "refusal", func() bool {
		st := h.state(id)
		return !st.Process && st.Status == StatusIdle && strings.Contains(st.Error, "launch refused")
	})

	h.runner.Send(id, "three")
	eventually(t, "restart", func() bool { return h.backend.startCount() == 2 })
	p = h.backend.proc(1)
	p.nextSent(t)
	p.exit()
	eventually(t, "crash", func() bool {
		st := h.state(id)
		return !st.Process && st.Status == StatusIdle && st.Error != ""
	})
}

func ptr(s string) *string { return &s }

// TestRunnerSuiteOnPi runs the runner suite again with every profile on
// the pi backend: the runner treats the backends alike through Process.
func TestRunnerSuiteOnPi(t *testing.T) {
	suiteBackend = BackendPi
	t.Cleanup(func() { suiteBackend = BackendClaude })
	for name, test := range map[string]func(*testing.T){
		"CreateWritesRecordRegistrationAndSnapshot": TestCreateWritesRecordRegistrationAndSnapshot,
		"CreateRefusesBadLaunch":                    TestCreateRefusesBadLaunch,
		"SendRunsTurnAndHeldSendsMerge":             TestSendRunsTurnAndHeldSendsMerge,
		"IdleTimerEndsProcessAndNextSendResumes":    TestIdleTimerEndsProcessAndNextSendResumes,
		"IdleTimeoutZeroNeverExits":                 TestIdleTimeoutZeroNeverExits,
		"RecordSurvivesDaemonRestart":               TestRecordSurvivesDaemonRestart,
		"StopKeepsHeldAndResumes":                   TestStopKeepsHeldAndResumes,
		"CloseEndsConversationForGood":              TestCloseEndsConversationForGood,
		"InterruptTakesBackHeldAndStopsTurn":        TestInterruptTakesBackHeldAndStopsTurn,
		"SettingsApplyAtTurnBoundary":               TestSettingsApplyAtTurnBoundary,
		"FailuresSurfaceInState":                    TestFailuresSurfaceInState,
		"RESTRoutesEndToEnd":                        TestRESTRoutesEndToEnd,
		"RESTWritesRefuseOtherOrigins":              TestRESTWritesRefuseOtherOrigins,
	} {
		t.Run(name, test)
	}
}
