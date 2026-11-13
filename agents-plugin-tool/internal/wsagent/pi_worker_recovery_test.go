package wsagent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsconfig"
	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wsholder"
	"github.com/kang-sw/devenv/internal/wsmailbox"
)

// Substitute only the executable at the real holder's spawn boundary. The
// production Pi backend still builds its restricted argv/bridge and maps RPC.
type recoveryPythonHost struct {
	ManagerHost
	python, script string
}

func (h recoveryPythonHost) Spawn(id string, spec wsholder.SpawnSpec) (wsholder.ChildStatus, error) {
	spec.Argv = append([]string{h.python, "-I", "-u", h.script}, spec.Argv[1:]...)
	return h.ManagerHost.Spawn(id, spec)
}

// The two gates separate agent_end from agent_settled. stdin remains open
// through daemon downtime; the holder, not the daemon, owns these pipes.
const recoveryPythonPi = `
import json, os, sys, threading, time
session = sys.argv[sys.argv.index("--session") + 1]
os.makedirs(os.path.dirname(session), exist_ok=True)
session_id = "fake-pi-" + str(os.getpid())
lock = threading.Lock()
running = False
messages = []
def emit(value):
    with lock:
        print(json.dumps(value), flush=True)
def mark(suffix, value):
    with open(session + suffix, "w") as f:
        json.dump(value, f)
def gate(suffix):
    until = time.monotonic() + 60
    while not os.path.exists(session + suffix):
        if time.monotonic() >= until:
            os._exit(2)
        time.sleep(0.01)
def complete():
    global running, messages
    gate(".finish")
    message = {"role":"assistant", "content":[{"type":"text", "text":"held result"}], "stopReason":"stop"}
    messages = [message]
    with open(session, "a") as f:
        f.write(json.dumps(message) + "\n")
    emit({"type":"message_end", "message":message})
    emit({"type":"agent_end", "messages":[message]})
    mark(".ended", True)
    gate(".settle")
    running = False
    emit({"type":"agent_settled"})
    mark(".settled", True)
with open(session + ".starts", "a") as f:
    f.write(json.dumps({"pid":os.getpid(), "session":session, "argv":sys.argv[1:]}) + "\n")
with open(session, "a") as f:
    f.write(json.dumps({"type":"session", "id":session_id}) + "\n")
emit({"type":"response", "command":"get_session_stats", "success":True, "data":{"sessionId":session_id}})
for line in sys.stdin:
    d = json.loads(line)
    with open(session + ".commands", "a") as f:
        f.write(line)
    kind = d.get("type")
    data = {}
    if kind == "prompt":
        running = True
        emit({"type":"response", "command":"prompt", "success":True, "data":{"disposition":"queued"}})
        emit({"type":"agent_start"})
        threading.Thread(target=complete, daemon=True).start()
    else:
        if kind == "get_state": data = {"isStreaming":running, "isCompacting":False}
        if kind == "get_messages": data = {"messages":messages}
        if kind == "get_session_stats": data = {"sessionId":session_id}
        emit({"type":"response", "id":d.get("id", ""), "command":kind, "success":True, "data":data})
`

type piRecoveryHarness struct {
	t       *testing.T
	base    string
	python  string
	manager *wsholder.Manager
	runner  *Runner
}

func newPiRecoveryHarness(t *testing.T) *piRecoveryHarness {
	t.Helper()
	h := &piRecoveryHarness{t: t, base: t.TempDir(), python: lookPython(t)}
	if err := os.MkdirAll(filepath.Join(h.base, "root"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(h.base, "fake-pi.py"), []byte(recoveryPythonPi), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if h.runner != nil {
			h.runner.Close()
		}
		if h.manager == nil {
			h.manager = wsholder.New(wsholder.Options{Dir: filepath.Join(h.base, "holders"), Python: h.python})
		}
		h.manager.Scan()
		for _, holder := range h.manager.List().Holders {
			if err := h.manager.Stop(holder.ID); err != nil {
				if recoveryStoppedLinuxZombie(filepath.Join(h.base, "holders", holder.ID), holder.PID, err) {
					t.Logf("isolated holder %s stopped and removed its descriptor, but Linux has not reaped its zombie (pid %d)", holder.ID, holder.PID)
					continue
				}
				t.Errorf("cleanup isolated holder %s: %v", holder.ID, err)
			}
		}
		h.manager.Close()
	})
	return h
}

// Some Linux test hosts do not reap orphaned holders. This predates worker
// recovery: tolerate only Stop's liveness timeout for a verified zombie whose
// holder already removed its descriptor, never a live or unidentified process.
func recoveryStoppedLinuxZombie(dir string, pid int, stopErr error) bool {
	if runtime.GOOS != "linux" || pid <= 0 || stopErr.Error() != fmt.Sprintf("holder process %d did not end", pid) {
		return false
	}
	raw, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "stat"))
	if err != nil {
		return false
	}
	// comm is parenthesized and can itself contain spaces and parentheses.
	end := strings.LastIndexByte(string(raw), ')')
	if end < 0 {
		return false
	}
	fields := strings.Fields(string(raw)[end+1:])
	if len(fields) == 0 || fields[0] != "Z" {
		return false
	}
	_, err = wsholder.ReadDescriptor(dir)
	return os.IsNotExist(err)
}

func (h *piRecoveryHarness) start() {
	h.manager = wsholder.New(wsholder.Options{Dir: filepath.Join(h.base, "holders"), Python: h.python})
	host := recoveryPythonHost{ManagerHost: ManagerHost{h.manager}, python: h.python, script: filepath.Join(h.base, "fake-pi.py")}
	h.runner = NewRunner(Options{
		Dir: filepath.Join(h.base, "dashboard"), Holders: host,
		Backends: map[string]Backend{BackendPi: PiBackend{Command: h.python}},
		Keys:     &fakeKeys{}, Config: wsconfig.Options{CacheHome: filepath.Join(h.base, "cache"), ConfigHome: filepath.Join(h.base, "config")},
		WSMCP: func(string) (MCPCommand, error) { return MCPCommand{Command: "unused-ws-mcp"}, nil },
		Report: func(rep TurnReport) {
			if rep.Parent.Kind == ParentLead {
				// The same append used by PiTurnReporter, without importing mcp.
				if err := wsmailbox.PostSystem(filepath.Join(h.base, "mailbox.json"), rep.Parent.ReplyID, ReportText(rep), time.Now()); err != nil {
					h.t.Errorf("completion mail: %v", err)
				}
			}
		},
		heartbeat: 20 * time.Millisecond,
	})
	h.runner.SetEndpoint("http://127.0.0.1:1")
	h.runner.Start(nil)
}

func (h *piRecoveryHarness) stopDaemon() {
	h.runner.Close()
	h.manager.Close()
	h.runner, h.manager = nil, nil
}

func (h *piRecoveryHarness) spawn() Record {
	h.t.Helper()
	st, err := h.runner.Spawn(SpawnSpec{Profile: PiWorkerProfile, Root: filepath.Join(h.base, "root"), Prompt: "keep working", Tier: "medium", Parent: &leadParent})
	if err != nil {
		h.t.Fatal(err)
	}
	var rec Record
	eventually(h.t, "held worker and persisted session", func() bool {
		rec, err = readRecord(wsdashboard.SessionDir(filepath.Join(h.base, "dashboard"), st.ID))
		return err == nil && rec.HolderID != "" && rec.HostPID > 0 && rec.SessionID != "" && rec.TurnOpen && rec.HolderSeq > 0
	})
	return rec
}

func (h *piRecoveryHarness) gate(session, suffix string) {
	h.t.Helper()
	if err := os.WriteFile(session+suffix, []byte("ready"), 0o600); err != nil {
		h.t.Fatal(err)
	}
}

func (h *piRecoveryHarness) awaitFile(path string) {
	h.t.Helper()
	eventually(h.t, filepath.Base(path), func() bool { _, err := os.Stat(path); return err == nil })
}

func (h *piRecoveryHarness) mail() []wsmailbox.Envelope {
	h.t.Helper()
	store, err := wsmailbox.LoadReplyStore(filepath.Join(h.base, "mailbox.json"))
	if err != nil {
		h.t.Fatal(err)
	}
	return store.Queues[leadParent.ReplyID]
}

// This subprocess exits itself without Runner.Close, Manager.Close, sequence
// flushing, or test cleanup. Only its isolated holder/child is left running.
func TestPiWorkerAbruptLossHelper(t *testing.T) {
	base := os.Getenv("WS_TEST_PI_RECOVERY_BASE")
	if base == "" {
		t.Skip("isolated abrupt-loss helper")
	}
	h := &piRecoveryHarness{t: t, base: base, python: os.Getenv("WS_TEST_PI_RECOVERY_PYTHON")}
	h.start()
	rec := h.spawn()
	if err := os.WriteFile(filepath.Join(base, "ready"), []byte(rec.ReplyID), 0o600); err != nil {
		t.Fatal(err)
	}
	os.Exit(0)
}

func TestPiDaemonRestartMidTurnCompletesWithOneCompletionMail(t *testing.T) {
	for _, scenario := range []string{"mid-turn", "finished-during-downtime", "abrupt-loss"} {
		t.Run(scenario, func(t *testing.T) {
			h := newPiRecoveryHarness(t)
			var before Record
			if scenario == "abrupt-loss" {
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestPiWorkerAbruptLossHelper$", "-test.count=1")
				cmd.Env = append(os.Environ(), "WS_TEST_PI_RECOVERY_BASE="+h.base, "WS_TEST_PI_RECOVERY_PYTHON="+h.python)
				if out, err := cmd.CombinedOutput(); err != nil {
					t.Fatalf("isolated daemon helper: %v\n%s", err, out)
				}
				id, err := os.ReadFile(filepath.Join(h.base, "ready"))
				if err != nil {
					t.Fatal(err)
				}
				before, err = readRecord(wsdashboard.SessionDir(filepath.Join(h.base, "dashboard"), string(id)))
				if err != nil {
					t.Fatal(err)
				}
			} else {
				h.start()
				before = h.spawn()
				h.stopDaemon()
			}
			session := piSessionFile(wsdashboard.SessionDir(filepath.Join(h.base, "dashboard"), before.ReplyID))
			if scenario == "finished-during-downtime" {
				h.gate(session, ".finish")
				h.gate(session, ".settle")
				h.awaitFile(session + ".settled")
			}
			h.start()
			c := h.runner.conv(before.ReplyID)
			eventually(t, "replay completion", func() bool {
				replaying := true
				c.do(func() { replaying = c.replaying })
				return !replaying
			})
			after := c.record()
			if after.HolderID != before.HolderID || after.HostPID != before.HostPID || after.SessionID != before.SessionID || after.WSKey != before.WSKey {
				t.Fatal("restart changed holder, child PID, backend session, or delegate identity")
			}
			client := h.manager.Client(before.HolderID)
			if client == nil {
				t.Fatal("recorded holder was not reattached")
			}
			status, err := client.Status()
			if err != nil || status.Child.PID != before.HostPID || status.Child.State != "running" {
				t.Fatalf("rejoined the same live child: %+v, %v", status.Child, err)
			}
			if scenario != "finished-during-downtime" {
				h.gate(session, ".finish")
				h.awaitFile(session + ".ended")
				res, err := h.runner.Wait(context.Background(), before.ReplyID, 30*time.Millisecond)
				if err != nil || res.Status != TurnRunning || len(h.mail()) != 0 {
					t.Fatalf("agent_end must not settle the turn: %+v, %v", res, err)
				}
				h.gate(session, ".settle")
			}
			res, err := h.runner.Wait(context.Background(), before.ReplyID, 5*time.Second)
			if err != nil || res.Status != TurnDone || res.Message != "held result" || res.Error != "" {
				t.Fatalf("recovered wait: %+v, %v", res, err)
			}
			eventually(t, "exactly one completion mail", func() bool { return len(h.mail()) == 1 })
			c.do(func() {}) // Include report append/mark and any queued replay events.
			mail := h.mail()
			if len(mail) != 1 || !strings.Contains(mail[0].Content, "held result") || strings.Contains(mail[0].Content, TurnInterrupted) {
				t.Fatalf("completion mail = %+v", mail)
			}
			starts, err := os.ReadFile(session + ".starts")
			if err != nil || bytes.Count(starts, []byte("\n")) != 1 {
				t.Fatalf("child launched more than once: %s, %v", starts, err)
			}
			commands, err := os.ReadFile(session + ".commands")
			if err != nil {
				t.Fatal(err)
			}
			prompts := 0
			for _, line := range bytes.Split(bytes.TrimSpace(commands), []byte("\n")) {
				var command struct{ Type string }
				if json.Unmarshal(line, &command) == nil && command.Type == "prompt" {
					prompts++
				}
			}
			if prompts != 1 {
				t.Fatalf("the original prompt was repeated: %d prompts", prompts)
			}
		})
	}
}

func newFakePiRecoveryHarness(t *testing.T) (*leadHarness, *reports) {
	t.Helper()
	h := newLeadHarness(t)
	got := &reports{}
	h.opts.Report = got.add
	h.opts.WSMCP = func(string) (MCPCommand, error) { return MCPCommand{Command: "/unused/ws-mcp"}, nil }
	for tier, model := range map[string]string{"small": "prov/small", "medium": "prov/medium"} {
		if _, err := wsconfig.SetGlobalAgentsTierForHarness(h.opts.Config, tier, "", model, PiTierHarness, "low"); err != nil {
			t.Fatal(err)
		}
	}
	h.start()
	h.runner.SetEndpoint("http://127.0.0.1:1")
	return h, got
}

func spawnRecoveryWorker(t *testing.T, h *leadHarness) (State, string) {
	t.Helper()
	worker, err := h.runner.Spawn(SpawnSpec{Profile: PiWorkerProfile, Root: h.root, Prompt: "work", Tier: "medium", Parent: &leadParent})
	if err != nil {
		t.Fatal(err)
	}
	holder := h.holder(worker.ID)
	h.holders.out(holder, `{"type":"agent_start"}`)
	eventually(t, "persisted worker stream", func() bool { return h.record(worker.ID).HolderSeq > 0 })
	return worker, holder
}

func recoveryPiReply(text string) []string {
	return []string{jsonLine(map[string]any{"type": "message_end", "message": map[string]any{"role": "assistant", "content": []map[string]any{{"type": "text", "text": text}}, "stopReason": "stop"}}), `{"type":"agent_settled"}`}
}

func recoveryTokenCall(t *testing.T, endpoint, token, op string, body any) (int, string) {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint+RoutePrefix+"/by-token/"+op, bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	answer, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, string(answer)
}

func TestPiNestedDaemonRestartRejoinsBothDepths(t *testing.T) {
	type tokenReply struct {
		code int
		body string
	}
	h, got := newFakePiRecoveryHarness(t)
	old := httptest.NewServer(h.runner.Handler())
	defer old.Close()
	h.runner.SetEndpoint(old.URL)
	worker, workerHolder := spawnRecoveryWorker(t, h)
	workerBefore := h.record(worker.ID)
	code, body := recoveryTokenCall(t, old.URL, workerBefore.Capability, "spawn", map[string]any{"prompt": "leaf work", "tier": "small"})
	var leaf State
	if code != http.StatusCreated || json.Unmarshal([]byte(body), &leaf) != nil {
		t.Fatalf("spawn leaf: %d %s", code, body)
	}
	leafHolder := h.holder(leaf.ID)
	h.holders.out(leafHolder, `{"type":"agent_start"}`)
	eventually(t, "persisted leaf stream", func() bool { return h.record(leaf.ID).HolderSeq > 0 })
	leafBefore := h.record(leaf.ID)
	// The parent's done result must remain held while its leaf still runs.
	h.holders.out(workerHolder, recoveryPiReply("interim parent result")...)
	eventually(t, "parent result held for leaf", func() bool { return h.record(worker.ID).HeldTurn != nil })
	waitReply := make(chan tokenReply, 1)
	go func() {
		code, body := recoveryTokenCall(t, old.URL, workerBefore.Capability, "wait", map[string]any{"id": leaf.ID, "timeout_s": 30})
		waitReply <- tokenReply{code, body}
	}()
	child := h.runner.conv(leaf.ID)
	eventually(t, "active nested wait", func() bool {
		child.mu.Lock()
		defer child.mu.Unlock()
		return len(child.parentWaits) > 0
	})
	h.runner.Close()
	select {
	case reply := <-waitReply:
		if reply.code != http.StatusServiceUnavailable || !strings.Contains(reply.body, "reissue the wait") || strings.Contains(reply.body, `"status":"closed"`) {
			t.Fatalf("daemon detach must not close the child's turn: %d %s", reply.code, reply.body)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("nested wait did not return on daemon detach")
	}
	h.holders.restart()
	entered, release := h.gateScan()
	h.runner = NewRunner(h.opts)
	arrived := make(chan struct{}, 1)
	next := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		select {
		case arrived <- struct{}{}:
		default:
		}
		h.runner.Handler().ServeHTTP(w, req)
	}))
	defer next.Close()
	defer release()
	if next.URL == old.URL {
		t.Fatal("test requires an endpoint-port change")
	}
	old.Close()
	h.runner.SetEndpoint(next.URL)
	h.runner.Start(nil)
	t.Cleanup(h.runner.Close)
	// Cleanup must release the scan before the runner's cleanup waits for it.
	t.Cleanup(release)
	wait(t, "initial holder scan gate", entered)
	listReply := make(chan tokenReply, 1)
	go func() {
		code, body := recoveryTokenCall(t, next.URL, workerBefore.Capability, "list", nil)
		listReply <- tokenReply{code, body}
	}()
	wait(t, "nested call reached successor", arrived)
	select {
	case reply := <-listReply:
		t.Fatalf("nested call escaped the initial scan: %d %s", reply.code, reply.body)
	case <-time.After(30 * time.Millisecond):
	}
	release()
	eventually(t, "both persisted capabilities restored", func() bool {
		h.runner.mu.Lock()
		defer h.runner.mu.Unlock()
		return h.runner.tokens[workerBefore.Capability] == worker.ID && h.runner.tokens[leafBefore.Capability] == leaf.ID
	})
	for _, before := range []Record{workerBefore, leafBefore} {
		after := h.record(before.ReplyID)
		if after.HolderID != before.HolderID || after.HostPID != before.HostPID || after.Capability != before.Capability || after.Tier != before.Tier {
			t.Fatal("nested rejoin changed process, capability, or tier")
		}
	}
	select {
	case reply := <-listReply:
		if reply.code != http.StatusOK || !strings.Contains(reply.body, leaf.ID) {
			t.Fatalf("restored worker token: %d %s", reply.code, reply.body)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("nested call was not released after holder validation")
	}
	if code, body := recoveryTokenCall(t, next.URL, leafBefore.Capability, "spawn", map[string]any{"prompt": "too deep", "tier": "small"}); code != http.StatusForbidden || !strings.Contains(body, "depth") {
		t.Fatalf("depth-2 cap after rejoin: %d %s", code, body)
	}
	if len(got.lead()) != 0 {
		t.Fatalf("parent result escaped its child hold: %+v", got.lead())
	}
	h.holders.out(leafHolder, recoveryPiReply("leaf recovered")...)
	eventually(t, "leaf report starts parent's next turn", func() bool {
		prompts := h.holders.inputs(workerHolder, "prompt")
		return len(prompts) == 2 && strings.Contains(prompts[1], "leaf recovered")
	})
	if len(h.record(leaf.ID).PendingReports) != 0 || len(got.lead()) != 0 {
		t.Fatal("nested report was lost, or the interim parent result was published")
	}
	h.holders.out(workerHolder, `{"type":"agent_start"}`)
	h.holders.out(workerHolder, recoveryPiReply("parent recovered")...)
	eventually(t, "one final parent report", func() bool { return len(got.lead()) == 1 })
	if rep := got.lead()[0]; rep.Result.Status != TurnDone || rep.Result.Message != "parent recovered" {
		t.Fatalf("recovered parent report: %+v", rep.Result)
	}
	if h.holders.opCount("launch") != 2 || h.holders.opCount("spawn") != 2 {
		t.Fatalf("nested restart launched another process: %v", h.holders.opList())
	}
}

func TestPiSteerAcknowledgmentsResumeAfterAppliedSequence(t *testing.T) {
	h, _ := newFakePiRecoveryHarness(t)
	worker, holder := spawnRecoveryWorker(t, h)
	c := h.runner.conv(worker.ID)
	c.do(func() {
		c.takeReport("first report")
		c.takeReport("second report")
	})
	h.holders.out(holder,
		`{"type":"response","command":"steer","success":true}`,
		`{"type":"message_end","message":{"role":"user","content":"first report"}}`)
	eventually(t, "first steer acknowledged and taken", func() bool {
		rec := h.record(worker.ID)
		return rec.SteerAnswers == 1 && rec.SteerAnswerSeq > 0 && len(rec.Steered) == 1 && rec.Steered[0].Seq == 2
	})
	c.do(c.persistSeq)
	h.restart()
	c = h.runner.conv(worker.ID)
	eventually(t, "steer replay done", func() bool {
		done := false
		c.do(func() { done = !c.replaying })
		return done
	})
	var seq, answers int
	c.do(func() { seq, answers = c.steerSeq, c.steerAnswers })
	if seq != 2 || answers != 1 {
		t.Fatalf("replay lost counters or reapplied the first ack: seq=%d answers=%d", seq, answers)
	}
	// This is the outstanding second response, not a third response merely
	// because the first acknowledgment reappeared in the holder's replay.
	h.holders.out(holder, `{"type":"response","command":"steer","success":false,"error":"not queued"}`)
	eventually(t, "second report returned to the next turn", func() bool {
		rec := h.record(worker.ID)
		return rec.SteerAnswers == 2 && len(rec.Steered) == 0 && len(rec.Reports) == 1 && rec.Reports[0] == "second report"
	})
	if h.holders.opCount("launch") != 1 || len(h.holders.inputs(holder, "steer")) != 2 {
		t.Fatal("acknowledgment recovery restarted the process or resent a steer")
	}
}

func TestPiGapRecoveryErrorWithoutMessageStillFails(t *testing.T) {
	h, got := newFakePiRecoveryHarness(t)
	worker, holder := spawnRecoveryWorker(t, h)
	startedAt := h.record(worker.ID).TurnStartedAt
	if startedAt == 0 {
		t.Fatal("the open turn has no persisted start time")
	}
	h.runner.Close()
	h.holders.restart()
	// Keep one non-boundary record so replay reports a wrapped ring and
	// asks the surviving child for its current state and saved messages.
	h.holders.out(holder, `{"type":"response","command":"get_session_stats","success":true,"data":{"sessionId":"saved-session"}}`)
	h.holders.trim(holder, h.holders.records(holder)-1)
	h.start()
	answer := func(command string, data any) {
		t.Helper()
		var inputs []string
		eventually(t, command+" probe", func() bool {
			inputs = h.holders.inputs(holder, command)
			return len(inputs) > 0
		})
		var request struct{ ID string }
		if err := json.Unmarshal([]byte(inputs[len(inputs)-1]), &request); err != nil {
			t.Fatal(err)
		}
		h.holders.out(holder, jsonLine(map[string]any{"type": "response", "id": request.ID, "command": command, "success": true, "data": data}))
	}
	answer("get_state", map[string]any{"isStreaming": false, "isCompacting": false})
	answer("get_messages", map[string]any{"messages": []any{map[string]any{
		"role": "assistant", "timestamp": startedAt, "stopReason": "error",
		"content": []any{map[string]any{"type": "text", "text": "partial reply"}},
	}}})
	res, err := h.runner.Wait(context.Background(), worker.ID, time.Second)
	if err != nil || res.Status != TurnFailed || res.Error != "turn failed" || res.Message != "partial reply" {
		t.Fatalf("a saved error with no errorMessage became success: %+v, %v", res, err)
	}
	eventually(t, "one failed recovery report", func() bool { return len(got.lead()) == 1 })
	if got.lead()[0].Result.Status != TurnFailed || h.holders.opCount("launch") != 1 {
		t.Fatal("gap recovery reported success or started another process")
	}
}

func TestPiProcessGoneRecoveryFallbacks(t *testing.T) {
	for _, scenario := range []string{"missing-holder", "child-exited", "legacy-rewritten"} {
		t.Run(scenario, func(t *testing.T) {
			h, got := newFakePiRecoveryHarness(t)
			worker, holder := spawnRecoveryWorker(t, h)
			h.runner.Close()
			h.holders.restart()
			unrelated := ""
			switch scenario {
			case "missing-holder":
				h.holders.vanish(holder)
			case "child-exited":
				h.holders.exit(holder, 7)
			case "legacy-rewritten":
				// Model an older writer's struct round-trip: recovery members
				// disappear while the legacy record and schema stay readable.
				type legacyRecord struct {
					SchemaVersion int        `json:"schema_version"`
					ReplyID       string     `json:"reply_id"`
					Profile       string     `json:"profile"`
					Backend       string     `json:"backend"`
					Root          string     `json:"root"`
					WSKey         string     `json:"ws_session_key"`
					SessionID     string     `json:"backend_session_id,omitempty"`
					CreatedAt     string     `json:"created_at"`
					Parent        *ParentRef `json:"parent,omitempty"`
					Tier          string     `json:"tier,omitempty"`
					TurnOpen      bool       `json:"turn_open,omitempty"`
				}
				path := recordPath(wsdashboard.SessionDir(h.dir, worker.ID))
				raw, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				var legacy legacyRecord
				if err := json.Unmarshal(raw, &legacy); err != nil {
					t.Fatal(err)
				}
				raw, err = json.Marshal(legacy)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, raw, 0o600); err != nil {
					t.Fatal(err)
				}
				if legacy.SchemaVersion != recordSchemaVersion || strings.Contains(string(raw), "holder_id") {
					t.Fatal("legacy fixture retained recovery metadata or changed schema")
				}
				unrelated = h.holders.add(holderSession{Kind: "terminal", ReplyID: "other"})
			}
			if scenario == "missing-holder" {
				entered, release := h.gateScan()
				defer release()
				h.start()
				wait(t, "delayed holder scan", entered)
				res, err := h.runner.Wait(context.Background(), worker.ID, 30*time.Millisecond)
				if err != nil || res.Status != TurnRunning || len(got.lead()) != 0 || h.holders.opCount("launch") != 1 {
					t.Fatalf("an incomplete scan was mistaken for process loss: %+v, %v", res, err)
				}
				release()
			} else {
				h.start()
			}
			res, err := h.runner.Wait(context.Background(), worker.ID, time.Second)
			if err != nil || res.Status != TurnInterrupted || res.Error == "" {
				t.Fatalf("process-gone wait: %+v, %v", res, err)
			}
			h.quiesce(worker.ID)
			st := h.state(worker.ID)
			if st.Status != StatusIdle || st.Process || st.Error != res.Error || h.record(worker.ID).TurnOpen || len(got.lead()) != 1 {
				t.Fatalf("interrupted once, then idle/error/no process: %+v", st)
			}
			if scenario == "legacy-rewritten" {
				eventually(t, "downgrade's worker orphan swept", func() bool { return h.holders.opCount("stop "+holder) == 1 })
				if h.holders.opCount("stop "+unrelated) != 0 {
					t.Fatal("worker orphan sweep stopped another holder kind")
				}
			}
			h.send(worker.ID, "follow-up")
			next := h.holder(worker.ID)
			prompts := h.holders.inputs(next, "prompt")
			if next == holder || len(prompts) != 1 || !strings.Contains(prompts[0], "follow-up") {
				t.Fatalf("follow-up was queued but not submitted on a new process: %q", prompts)
			}
			h.holders.out(next, `{"type":"agent_start"}`)
			h.holders.out(next, recoveryPiReply("recovered follow-up")...)
			res, err = h.runner.Wait(context.Background(), worker.ID, time.Second)
			if err != nil || res.Status != TurnDone || res.Message != "recovered follow-up" {
				t.Fatalf("follow-up wait: %+v, %v", res, err)
			}
			eventually(t, "one interruption and one follow-up completion", func() bool { return len(got.lead()) == 2 })
		})
	}
}
