package wsagent

import (
	"encoding/json"
	"net/http"
	"slices"
	"testing"
)

type idleAnswer struct {
	Sessions       []IdleLead `json:"sessions"`
	Kept           int        `json:"kept"`
	Uncontrollable int        `json:"uncontrollable"`
}

type sleepAnswer struct {
	Slept   []string `json:"slept"`
	Skipped []string `json:"skipped"`
}

// settledLead is a lead whose first turn ended: process alive, nothing running.
func (h *leadHarness) settledLead() (id, holder string) {
	h.t.Helper()
	id, holder = h.runLead("go")
	h.holders.out(holder, stateLine(HarnessRunning), initLine("sess-1", "auto"), replyLine("u1", "ok"), resultLine(), stateLine(HarnessIdle))
	h.idle(id)
	return id, holder
}

func (h *leadHarness) idleList(v viewer) idleAnswer {
	h.t.Helper()
	r := h.route(v, http.MethodGet, "/agents/idle", nil)
	var out idleAnswer
	if r.status != http.StatusOK || json.Unmarshal([]byte(r.body), &out) != nil {
		h.t.Fatalf("GET /agents/idle = %d %s", r.status, r.body)
	}
	return out
}

func (h *leadHarness) sleepIdle(v viewer, ids ...string) (int, sleepAnswer) {
	h.t.Helper()
	r := h.route(v, http.MethodPost, "/agents/sleep-idle", map[string]any{"ids": ids})
	var out sleepAnswer
	if r.status == http.StatusOK && json.Unmarshal([]byte(r.body), &out) != nil {
		h.t.Fatalf("sleep-idle answer %q", r.body)
	}
	return r.status, out
}

// TestIdleListOffersIdleControllableLeadsOnly: a settled lead is listed; one
// waiting on an approval and one at work are counted as kept; a viewer
// without the lead controls sees none offered and the idle one excluded.
func TestIdleListOffersIdleControllableLeadsOnly(t *testing.T) {
	h := newLeadHarness(t).start()
	idleID, _ := h.settledLead()
	waitID, _ := h.waitingLead()
	busyID, busyHolder := h.runLead("work")
	h.holders.out(busyHolder, stateLine(HarnessRunning))

	got := h.idleList(localBrowser)
	if len(got.Sessions) != 1 || got.Sessions[0].ID != idleID || got.Sessions[0].Backend != BackendClaude || got.Kept != 2 || got.Uncontrollable != 0 {
		t.Fatalf("local list = %+v, want only %s with 2 kept (not %s, %s)", got, idleID, waitID, busyID)
	}
	lan := h.idleList(lanBrowser)
	if len(lan.Sessions) != 0 || lan.Uncontrollable != 1 || lan.Kept != 2 {
		t.Fatalf("LAN list = %+v, want none offered and 1 excluded", lan)
	}
	if r := h.route(localBrowser, http.MethodPost, "/agents/idle", map[string]any{}); r.status != http.StatusMethodNotAllowed {
		t.Fatalf("POST /agents/idle = %d, want 405", r.status)
	}
}

// TestSleepIdleStopsIdleAndSkipsWhatBecameBusy: the stop re-checks idleness
// per id; a session that started a turn after the list was read, a waiting
// one, and an unknown id are skipped and left running, and the stopped one
// resumes on its next message.
func TestSleepIdleStopsIdleAndSkipsWhatBecameBusy(t *testing.T) {
	h := newLeadHarness(t).start()
	a, holderA := h.settledLead()
	b, holderB := h.settledLead()
	waiting, holderW := h.waitingLead()
	listed := h.idleList(localBrowser)
	if len(listed.Sessions) != 2 {
		t.Fatalf("list = %+v", listed)
	}
	// b starts a turn after the dialog listed it.
	h.send(b, "again")
	h.holders.out(holderB, stateLine(HarnessRunning))
	eventually(t, "b's turn", func() bool { return h.state(b).Status == StatusRunning })

	status, got := h.sleepIdle(localBrowser, a, b, waiting, "nosuchsession")
	if status != http.StatusOK || !slices.Equal(got.Slept, []string{a}) || !slices.Equal(got.Skipped, []string{b, waiting, "nosuchsession"}) {
		t.Fatalf("sleep-idle = %d %+v", status, got)
	}
	if st := h.state(a); st.Process || st.Status != StatusIdle {
		t.Fatalf("a not stopped: %+v", st)
	}
	if h.holders.opCount("stop "+holderA) != 1 || h.holders.opCount("stop "+holderB) != 0 || h.holders.opCount("stop "+holderW) != 0 {
		t.Fatalf("holder calls %v", h.holders.opList())
	}
	if st := h.state(b); !st.Process {
		t.Fatalf("b was stopped: %+v", st)
	}
	if again := h.idleList(localBrowser); len(again.Sessions) != 0 {
		t.Fatalf("a stopped session is still offered: %+v", again)
	}
	h.send(a, "wake")
	eventually(t, "a resumes on the next message", func() bool { return h.state(a).Process })
}
