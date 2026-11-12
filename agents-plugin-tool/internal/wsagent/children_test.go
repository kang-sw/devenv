package wsagent

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsconfig"
	"github.com/kang-sw/devenv/internal/wsdashboard"
)

// childHarness is a spawn harness whose fake backend also runs the
// pi-worker profile, takes steered reports, and whose runner has an
// endpoint, with the small, medium, and large Pi tiers set.
func childHarness(t *testing.T) (*harness, *reports) {
	t.Helper()
	h := newHarness(t, map[string]string{
		"chat": chatProfile,
		"piw":  `{"backend":"claude","model":"haiku","ws_tools":["pi.spawn","pi.list"]}`,
	})
	got := &reports{}
	h.config.Report = got.add
	h.config.Backends[BackendPi] = h.backend
	h.backend.steers = true
	h.endpoint = "http://127.0.0.1:1"
	for tier, model := range map[string]string{"small": "prov/small-model", "medium": "prov/medium-model", "large": "prov/large-model"} {
		if _, err := wsconfig.SetGlobalAgentsTierForHarness(h.config.Config, tier, "", model, PiTierHarness, "low"); err != nil {
			t.Fatal(err)
		}
	}
	h.runner.Close()
	h.start()
	return h, got
}

// spawnWorker spawns a lead's Pi worker of tier.
func (h *harness) spawnWorker(tier string) State {
	h.t.Helper()
	st, err := h.runner.Spawn(SpawnSpec{Profile: PiWorkerProfile, Root: h.root, Prompt: "work", Tier: tier, Label: "worker", Parent: &leadParent})
	if err != nil {
		h.t.Fatalf("Spawn: %v", err)
	}
	return st
}

// token is the capability token of the i-th process start, "" for none.
func (h *harness) token(i int) string {
	h.t.Helper()
	spec := h.backend.waitProcStart(h.t, i)
	if spec.WSMCP == nil {
		return ""
	}
	return spec.WSMCP.Env[EnvPiToken]
}

// tokenCall posts body to the token route's op from this machine.
func (h *harness) tokenCall(token, op string, body any) (int, string) {
	h.t.Helper()
	return h.tokenCallFrom("127.0.0.1:5555", token, op, body)
}

func (h *harness) tokenCallFrom(remote, token, op string, body any) (int, string) {
	h.t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		h.t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, RoutePrefix+"/"+tokenRoute+"/"+op, bytes.NewReader(raw))
	req.RemoteAddr = remote
	req.Host = "127.0.0.1:7777"
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.runner.Handler().ServeHTTP(rec, req)
	return rec.Code, rec.Body.String()
}

// spawnChild spawns a Pi child of the token holder through the token route.
func (h *harness) spawnChild(token, tier, prompt string) State {
	h.t.Helper()
	code, body := h.tokenCall(token, "spawn", map[string]any{"prompt": prompt, "tier": tier})
	if code != http.StatusCreated {
		h.t.Fatalf("token spawn = %d %s", code, body)
	}
	var st State
	if err := json.Unmarshal([]byte(body), &st); err != nil {
		h.t.Fatal(err)
	}
	return st
}

// leadReports are the reports to the lead parent (leadParent).
func (r *reports) lead() []TurnReport {
	var out []TurnReport
	for _, rep := range r.list() {
		if rep.Parent == leadParent {
			out = append(out, rep)
		}
	}
	return out
}

// noLeadReport fails when the lead got a report within a short while.
func noLeadReport(t *testing.T, got *reports, why string) {
	t.Helper()
	time.Sleep(60 * time.Millisecond)
	if n := len(got.lead()); n != 0 {
		t.Fatalf("%s: the lead got %d reports: %+v", why, n, got.lead())
	}
}

// noSend fails when the process was sent a turn within a short while.
func noSend(t *testing.T, p *fakeProc, why string) {
	t.Helper()
	select {
	case s := <-p.sent:
		t.Fatalf("%s: a turn was sent: %q", why, s)
	case <-time.After(60 * time.Millisecond):
	}
}

func (h *harness) record(id string) Record {
	h.t.Helper()
	return readEventually(h.t, "the record", func() (Record, error) { return readRecord(wsdashboard.SessionDir(h.dir, id)) })
}

// workerWithChild is a running worker (process 0, its first turn sent)
// with a running child it spawned (process 1).
func workerWithChild(t *testing.T, h *harness) (w State, wp *fakeProc, token string, gc State, gp *fakeProc) {
	t.Helper()
	w = h.spawnWorker("medium")
	wp = h.backend.waitProc(t, 0)
	wp.nextSent(t)
	token = h.token(0)
	gc = h.spawnChild(token, "small", "sub task")
	gp = h.backend.waitProc(t, 1)
	gp.nextSent(t)
	return w, wp, token, gc, gp
}

func TestCapabilityTokenGoesToSpawnedWorkersOnly(t *testing.T) {
	h, _ := childHarness(t)
	w := h.spawnWorker("medium")
	spec := h.backend.waitProcStart(t, 0)
	env := spec.WSMCP.Env
	first := env[EnvPiToken]
	if len(first) != 64 || env[EnvPiEndpoint] != h.endpoint || !strings.Contains(env[EnvAllowedTools], "pi.spawn") {
		t.Fatalf("a spawned worker's ws-mcp gets a token, the endpoint, and pi.*: %v", env)
	}
	if rec := h.record(w.ID); rec.Tier != "medium" || h.state(w.ID).Tier != "medium" {
		t.Fatalf("the tier is recorded: %+v", rec)
	}
	raw, err := os.ReadFile(recordPath(wsdashboard.SessionDir(h.dir, w.ID)))
	if err != nil || strings.Contains(string(raw), first) {
		t.Fatalf("the token is never persisted: %v", err)
	}

	// No recorded tier: no token.
	if _, err := h.runner.Spawn(SpawnSpec{Profile: PiWorkerProfile, Root: h.root, Prompt: "x", Parent: &leadParent}); err != nil {
		t.Fatal(err)
	}
	if tok := h.token(1); tok != "" {
		t.Fatalf("a spawn with no tier gets no token: %q", tok)
	}
	// A conversation opened through POST /agents: no token.
	c := h.create("piw")
	if _, err := h.runner.Send(c.ID, "hi"); err != nil {
		t.Fatal(err)
	}
	if spec := h.backend.waitProcStart(t, 2); spec.WSMCP.Env[EnvPiToken] != "" || spec.WSMCP.Env[EnvPiEndpoint] != "" {
		t.Fatalf("POST /agents gets no token: %v", spec.WSMCP.Env)
	}

	// A token is drawn at every process start; the last one is void.
	if code, _ := h.tokenCall(first, "list", nil); code != http.StatusOK {
		t.Fatalf("the live token lists: %d", code)
	}
	if _, err := h.runner.Stop(w.ID); err != nil {
		t.Fatal(err)
	}
	if code, _ := h.tokenCall(first, "list", nil); code != http.StatusUnauthorized {
		t.Fatalf("an ended process's token is void: %d", code)
	}
	if _, err := h.runner.Send(w.ID, "again"); err != nil {
		t.Fatal(err)
	}
	second := h.token(3)
	if second == "" || second == first {
		t.Fatalf("a new start draws a new token: %q", second)
	}
	if code, _ := h.tokenCall(second, "list", nil); code != http.StatusOK {
		t.Fatalf("the new token lists: %d", code)
	}
}

func TestTokenSpawnRecordsTheHolderAsParent(t *testing.T) {
	h, _ := childHarness(t)
	w := h.spawnWorker("medium")
	token := h.token(0)
	if err := os.Mkdir(filepath.Join(h.root, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	code, body := h.tokenCall(token, "spawn", map[string]any{"prompt": "sub task", "tier": "small", "cwd": "sub", "label": "gc"})
	if code != http.StatusCreated {
		t.Fatalf("spawn = %d %s", code, body)
	}
	var gc State
	if err := json.Unmarshal([]byte(body), &gc); err != nil {
		t.Fatal(err)
	}
	want := ParentRef{Kind: ParentConversation, ReplyID: w.ID}
	if gc.Parent == nil || *gc.Parent != want || gc.Tier != "small" || gc.Model != "prov/small-model" || gc.Effort != "low" ||
		gc.Profile != PiWorkerProfile || gc.Cwd != filepath.Join(h.root, "sub") || gc.Label != "gc" || gc.Root != h.root {
		t.Fatalf("the child = %+v", gc)
	}
	if rec := h.record(gc.ID); rec.Parent == nil || *rec.Parent != want || rec.Tier != "small" {
		t.Fatalf("the child's record = %+v", rec)
	}
	if got := h.backend.waitProc(t, 1).nextSent(t); got != "sub task" {
		t.Fatalf("the child's first turn = %q", got)
	}
	if tok := h.token(1); tok == "" || tok == token {
		t.Fatalf("the child holds a token of its own: %q", tok)
	}

	open := len(h.runner.List())
	for name, tok := range map[string]string{"missing": "", "forged": strings.Repeat("ab", 32)} {
		if code, body := h.tokenCall(tok, "spawn", map[string]any{"prompt": "x", "tier": "small"}); code != http.StatusUnauthorized {
			t.Fatalf("%s token: %d %s", name, code, body)
		}
	}
	if code, _ := h.tokenCallFrom("192.0.2.1:5555", token, "spawn", map[string]any{"prompt": "x", "tier": "small"}); code != http.StatusNotFound {
		t.Fatalf("another machine's call: %d", code)
	}
	if n := len(h.runner.List()); n != open {
		t.Fatalf("a refused call spawns nothing: %d open, was %d", n, open)
	}

	// A closed conversation's token is stale.
	if _, err := h.runner.CloseConversation(w.ID); err != nil {
		t.Fatal(err)
	}
	if code, _ := h.tokenCall(token, "list", nil); code != http.StatusUnauthorized {
		t.Fatalf("a closed worker's token: %d", code)
	}
}

func TestTokenHolderReachesItsOwnChildrenAlone(t *testing.T) {
	h, _ := childHarness(t)
	a := h.spawnWorker("medium")
	tokA := h.token(0)
	b := h.spawnWorker("medium")
	tokB := h.token(1)
	gc := h.spawnChild(tokA, "small", "a's child")

	list := func(token string) []State {
		code, body := h.tokenCall(token, "list", nil)
		var answer struct {
			Conversations []State `json:"conversations"`
		}
		if code != http.StatusOK || json.Unmarshal([]byte(body), &answer) != nil {
			t.Fatalf("list = %d %s", code, body)
		}
		return answer.Conversations
	}
	if got := list(tokA); len(got) != 1 || got[0].ID != gc.ID {
		t.Fatalf("a lists its child: %+v", got)
	}
	if got := list(tokB); len(got) != 0 {
		t.Fatalf("b lists none: %+v", got)
	}
	for _, op := range []string{"send", "stop", "transcript", "wait"} {
		code, body := h.tokenCall(tokB, op, map[string]any{"id": gc.ID, "message": "x", "timeout_s": 0.01})
		if code != http.StatusNotFound || !strings.Contains(body, "is not one of this conversation's Pi subagents") {
			t.Fatalf("b's %s on a's child = %d %s", op, code, body)
		}
		// A sibling, the holder itself, and the lead's other worker are
		// no children either.
		for _, id := range []string{b.ID, a.ID} {
			if code, _ := h.tokenCall(tokA, op, map[string]any{"id": id, "message": "x", "timeout_s": 0.01}); code != http.StatusNotFound {
				t.Fatalf("a's %s on %s = %d", op, id, code)
			}
		}
	}
	if code, body := h.tokenCall(tokA, "send", map[string]any{"id": gc.ID, "message": "more"}); code != http.StatusOK {
		t.Fatalf("a sends its child: %d %s", code, body)
	}
	if code, body := h.tokenCall(tokA, "transcript", map[string]any{"id": gc.ID}); code != http.StatusOK || !strings.Contains(body, `"records"`) {
		t.Fatalf("a reads its child: %d %s", code, body)
	}
	if code, body := h.tokenCall(tokA, "stop", map[string]any{"id": gc.ID}); code != http.StatusOK || h.state(gc.ID).Status != StatusClosed {
		t.Fatalf("a closes its child: %d %s", code, body)
	}
}

func TestTokenSpawnCapsDepthAndTier(t *testing.T) {
	h, _ := childHarness(t)
	h.spawnWorker("medium")
	token := h.token(0)
	code, body := h.tokenCall(token, "spawn", map[string]any{"prompt": "x", "tier": "large"})
	if code != http.StatusForbidden || !strings.Contains(body, `tier "large" is above this conversation's own tier "medium"`) {
		t.Fatalf("a tier above the caller's = %d %s", code, body)
	}
	if code, body := h.tokenCall(token, "spawn", map[string]any{"prompt": "x", "tier": "huge"}); code != http.StatusBadRequest {
		t.Fatalf("an unknown tier = %d %s", code, body)
	}
	gc := h.spawnChild(token, "medium", "depth two")
	// The depth-2 child keeps its token and gets the cap message.
	gcToken := h.token(1)
	if gcToken == "" {
		t.Fatalf("a depth-2 child holds a token")
	}
	code, body = h.tokenCall(gcToken, "spawn", map[string]any{"prompt": "x", "tier": "small"})
	if code != http.StatusForbidden || !strings.Contains(body, "spawn depth cap is 2") || !strings.Contains(body, "depth 2") {
		t.Fatalf("a depth-3 spawn = %d %s", code, body)
	}
	if n := len(h.runner.Children(ParentRef{Kind: ParentConversation, ReplyID: gc.ID})); n != 0 {
		t.Fatalf("a refused spawn spawns nothing: %d", n)
	}

	// A caller with no recorded tier is refused, never left uncapped.
	plain, err := h.runner.Spawn(SpawnSpec{Profile: PiWorkerProfile, Root: h.root, Prompt: "x", Parent: &leadParent})
	if err != nil {
		t.Fatal(err)
	}
	_, err = h.runner.spawnFor(h.runner.conv(plain.ID), ChildSpawn{Prompt: "x", Tier: "small"})
	if requestStatus(err) != http.StatusForbidden || !strings.Contains(err.Error(), "no recorded tier") {
		t.Fatalf("a tierless caller = %v", err)
	}
}

func TestChildReportIsSteeredIntoTheRunningParentTurn(t *testing.T) {
	h, got := childHarness(t)
	w, wp, _, gc, gp := workerWithChild(t, h)
	gp.reply("child result")
	var steered []string
	eventually(t, "the steered report", func() bool {
		steered, _ = wp.steered()
		return len(steered) == 1
	})
	if s := steered[0]; !strings.Contains(s, "[Pi subagent "+gc.ID) || !strings.Contains(s, "] turn done") || !strings.Contains(s, "child result") {
		t.Fatalf("steered report = %q", s)
	}
	if rec := h.record(gc.ID); len(rec.PendingReports) != 0 {
		t.Fatalf("a delivered report leaves the child's record: %+v", rec.PendingReports)
	}
	wp.events <- Event{SteerAnswer: &SteerAnswer{}}
	wp.events <- Event{UserTaken: steered[0]}
	wp.reply("worker done")
	eventually(t, "the worker's report", func() bool { return len(got.lead()) == 1 })
	if rep := got.lead()[0]; rep.Result.ID != w.ID || rep.Result.Status != TurnDone || rep.Result.Message != "worker done" {
		t.Fatalf("the worker's report = %+v", rep)
	}
	noSend(t, wp, "a report the run took")
	if _, cleared := wp.steered(); cleared != 0 {
		t.Fatalf("nothing to drop: %d", cleared)
	}
}

func TestChildReportPromptsAnIdleParentThatHeldItsReport(t *testing.T) {
	h, got := childHarness(t)
	w, wp, _, gc, gp := workerWithChild(t, h)
	wp.reply("interim")
	eventually(t, "the worker idle", func() bool { return h.state(w.ID).Status == StatusIdle })
	noLeadReport(t, got, "a worker with an open child")
	if res, err := h.runner.Wait(context.Background(), w.ID, 50*time.Millisecond); err != nil || res.Status != TurnRunning {
		t.Fatalf("a wait on a held worker = %+v, %v", res, err)
	}

	gp.reply("child result")
	turn := wp.nextSent(t)
	if !strings.Contains(turn, "[Pi subagent "+gc.ID) || !strings.Contains(turn, "child result") {
		t.Fatalf("the report's turn = %q", turn)
	}
	if steered, _ := wp.steered(); len(steered) != 0 {
		t.Fatalf("an idle parent is not steered: %q", steered)
	}
	noLeadReport(t, got, "the report's turn runs")
	wp.reply("final")
	eventually(t, "the worker's report", func() bool { return len(got.lead()) == 1 })
	if rep := got.lead()[0]; rep.Result.Message != "final" {
		t.Fatalf("the reported turn = %+v", rep.Result)
	}
	if res, err := h.runner.Wait(context.Background(), w.ID, time.Second); err != nil || res.Status != TurnDone || res.Message != "final" {
		t.Fatalf("a wait returns the reported turn: %+v, %v", res, err)
	}
}

func TestSteeredReportTheRunMissedGoesAsTheNextTurn(t *testing.T) {
	for _, tc := range []struct {
		name string
		// before and after are the answers to the steer before and after
		// the parent's turn ends.
		before, after *SteerAnswer
		cleared       int
	}{
		{name: "queued, not taken", before: &SteerAnswer{}, cleared: 1},
		{name: "answered after the turn", after: &SteerAnswer{}, cleared: 1},
		{name: "refused", before: &SteerAnswer{Refusal: "no"}, cleared: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h, got := childHarness(t)
			_, wp, _, gc, gp := workerWithChild(t, h)
			gp.reply("child result")
			eventually(t, "the steer", func() bool { s, _ := wp.steered(); return len(s) == 1 })
			if tc.before != nil {
				wp.events <- Event{SteerAnswer: tc.before}
			}
			wp.reply("interim")
			if tc.after != nil {
				noSend(t, wp, "a steer not yet answered")
				wp.events <- Event{SteerAnswer: tc.after}
			}
			turn := wp.nextSent(t)
			if !strings.Contains(turn, "[Pi subagent "+gc.ID) || !strings.Contains(turn, "child result") {
				t.Fatalf("the report's turn = %q", turn)
			}
			if _, cleared := wp.steered(); cleared != tc.cleared {
				t.Fatalf("dropped from the harness's queue %d times, want %d", cleared, tc.cleared)
			}
			noLeadReport(t, got, "the report's turn runs")
			wp.reply("final")
			eventually(t, "the worker's report", func() bool { return len(got.lead()) == 1 })
			if rep := got.lead()[0]; rep.Result.Message != "final" {
				t.Fatalf("the reported turn = %+v", rep.Result)
			}
		})
	}
}

func TestParentWaitIsTheReportsDelivery(t *testing.T) {
	h, got := childHarness(t)
	w, wp, token, gc, gp := workerWithChild(t, h)
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
	if a.code != http.StatusOK || json.Unmarshal([]byte(a.body), &res) != nil || res.Status != TurnDone || res.Message != "child result" || res.ID != gc.ID {
		t.Fatalf("the wait = %d %s", a.code, a.body)
	}
	time.Sleep(60 * time.Millisecond)
	if steered, _ := wp.steered(); len(steered) != 0 {
		t.Fatalf("a waited report is not steered too: %q", steered)
	}
	if rec := h.record(w.ID); len(rec.Reports) != 0 {
		t.Fatalf("a waited report is not kept for a turn: %q", rec.Reports)
	}
	if rec := h.record(gc.ID); len(rec.PendingReports) != 0 {
		t.Fatalf("a waited report is delivered: %+v", rec.PendingReports)
	}
	wp.reply("worker done")
	eventually(t, "the worker's report", func() bool { return len(got.lead()) == 1 })

	// A wait that comes after the settle takes the pending report, if it
	// is still there.
	ch := h.runner.conv(gc.ID)
	ch.update(func(rec *Record) {
		rec.PendingReports = append(rec.PendingReports, PendingReport{ID: "later", Result: TurnResult{ID: gc.ID, Status: TurnDone}})
	})
	ch.mu.Lock()
	ch.settledReport = "later"
	ch.mu.Unlock()
	if code, body := h.tokenCall(token, "wait", map[string]any{"id": gc.ID, "timeout_s": 1}); code != http.StatusOK {
		t.Fatalf("the late wait = %d %s", code, body)
	}
	if rec := h.record(gc.ID); len(rec.PendingReports) != 0 {
		t.Fatalf("the late wait takes the pending report: %+v", rec.PendingReports)
	}
}

func TestFailedWorkerTurnIsNotHeld(t *testing.T) {
	h, got := childHarness(t)
	w, wp, _, _, _ := workerWithChild(t, h)
	wp.events <- Event{TurnEnd: true, TurnError: "boom"}
	eventually(t, "the failed turn's report", func() bool { return len(got.lead()) == 1 })
	if rep := got.lead()[0]; rep.Result.ID != w.ID || rep.Result.Status != TurnFailed || rep.Result.Error != "boom" {
		t.Fatalf("the report = %+v", rep.Result)
	}
}

func TestHeldReportSurvivesARestart(t *testing.T) {
	t.Run("interrupted child", func(t *testing.T) {
		h, got := childHarness(t)
		w, wp, _, gc, _ := workerWithChild(t, h)
		wp.reply("interim")
		eventually(t, "the worker idle", func() bool { return h.state(w.ID).Status == StatusIdle })
		h.runner.Close()
		noLeadReport(t, got, "before the restart")

		h.start()
		turn := h.backend.waitProc(t, 2).nextSent(t)
		if !strings.Contains(turn, "[Pi subagent "+gc.ID) || !strings.Contains(turn, "turn interrupted") {
			t.Fatalf("the interrupted child's report resumes the worker: %q", turn)
		}
		noLeadReport(t, got, "the resumed worker's turn runs")
		h.backend.proc(2).reply("final")
		eventually(t, "the worker's report", func() bool { return len(got.lead()) == 1 })
		if rep := got.lead()[0]; rep.Result.ID != w.ID || rep.Result.Message != "final" {
			t.Fatalf("the report = %+v", rep.Result)
		}
		if rec := h.record(gc.ID); len(rec.PendingReports) != 0 || rec.TurnOpen {
			t.Fatalf("the child's report is delivered: %+v", rec)
		}
	})
	t.Run("pending report", func(t *testing.T) {
		h, got := childHarness(t)
		w, wp, _, gc, _ := workerWithChild(t, h)
		wp.reply("interim")
		eventually(t, "the worker idle", func() bool { return h.state(w.ID).Status == StatusIdle })
		h.runner.Close()
		// The child's turn settled and its report waited for the handover
		// when the daemon exited.
		dir := wsdashboard.SessionDir(h.dir, gc.ID)
		rec, err := readRecord(dir)
		if err != nil {
			t.Fatal(err)
		}
		rec.TurnOpen = false
		rec.PendingReports = []PendingReport{{ID: "p1", Result: TurnResult{ID: gc.ID, Status: TurnDone, Message: "persisted result"}}}
		if err := writeRecord(dir, rec); err != nil {
			t.Fatal(err)
		}

		h.start()
		turn := h.backend.waitProc(t, 2).nextSent(t)
		if !strings.Contains(turn, "[Pi subagent "+gc.ID) || !strings.Contains(turn, "persisted result") {
			t.Fatalf("the persisted report resumes the worker: %q", turn)
		}
		noLeadReport(t, got, "the resumed worker's turn runs")
		h.backend.proc(2).reply("final")
		eventually(t, "the worker's report", func() bool { return len(got.lead()) == 1 })
		if rep := got.lead()[0]; rep.Result.ID != w.ID || rep.Result.Message != "final" {
			t.Fatalf("the report = %+v", rep.Result)
		}
		if rec := h.record(gc.ID); len(rec.PendingReports) != 0 {
			t.Fatalf("the persisted report is delivered once: %+v", rec.PendingReports)
		}
	})
}

func TestClosingAWorkerClosesItsSubtreeDeepestFirst(t *testing.T) {
	h, _ := childHarness(t)
	w, _, _, gc, _ := workerWithChild(t, h)
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
	if _, err := h.runner.CloseConversation(w.ID); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	got := slices.Clone(order)
	mu.Unlock()
	if !slices.Equal(got, []string{gc.ID, w.ID}) {
		t.Fatalf("close order = %v, want the child first", got)
	}
	if h.state(gc.ID).Status != StatusClosed {
		t.Fatalf("the child is closed")
	}
}

func TestStoppedWorkerResumesOnItsChildsReport(t *testing.T) {
	h, got := childHarness(t)
	w, wp, _, gc, gp := workerWithChild(t, h)
	wp.reply("interim")
	eventually(t, "the worker idle", func() bool { return h.state(w.ID).Status == StatusIdle })
	if _, err := h.runner.Stop(w.ID); err != nil {
		t.Fatal(err)
	}
	if st := h.state(gc.ID); st.Status == StatusClosed || !st.Process {
		t.Fatalf("a stop does not cascade: %+v", st)
	}
	gp.reply("child result")
	turn := h.backend.waitProc(t, 2).nextSent(t)
	if !strings.Contains(turn, "[Pi subagent "+gc.ID) || !strings.Contains(turn, "child result") {
		t.Fatalf("the report resumes the stopped worker: %q", turn)
	}
	h.backend.proc(2).reply("final")
	eventually(t, "the worker's report", func() bool { return len(got.lead()) == 1 })
}

func TestReportToAClosedParentStays(t *testing.T) {
	h, _ := childHarness(t)
	w, _, _, gc, gp := workerWithChild(t, h)
	// The parent alone closes (a cascade closes its children with it).
	if _, err := h.runner.closeOne(w.ID); err != nil {
		t.Fatal(err)
	}
	gp.reply("late result")
	eventually(t, "the pending report", func() bool { return len(h.record(gc.ID).PendingReports) == 1 })
	time.Sleep(60 * time.Millisecond)
	if rec := h.record(gc.ID); len(rec.PendingReports) != 1 || rec.PendingReports[0].Result.Message != "late result" {
		t.Fatalf("the report stays in the child's record: %+v", rec.PendingReports)
	}
	if rec := h.record(w.ID); len(rec.Reports) != 0 {
		t.Fatalf("a closed parent takes no report: %q", rec.Reports)
	}
}

func TestHeldReportIsReleasedWhenTheChildCloses(t *testing.T) {
	h, got := childHarness(t)
	w, wp, _, gc, _ := workerWithChild(t, h)
	wp.reply("interim")
	eventually(t, "the worker idle", func() bool { return h.state(w.ID).Status == StatusIdle })
	noLeadReport(t, got, "a worker with an open child")
	if _, err := h.runner.CloseConversation(gc.ID); err != nil {
		t.Fatal(err)
	}
	eventually(t, "the held report", func() bool { return len(got.lead()) == 1 })
	if rep := got.lead()[0]; rep.Result.ID != w.ID || rep.Result.Message != "interim" {
		t.Fatalf("the released report = %+v", rep.Result)
	}
}
